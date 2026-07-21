package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"time"

	"new-api-pilot/migrations"
)

const schemaMigrationBootstrap = `CREATE TABLE IF NOT EXISTS schema_migration (
  version VARCHAR(64) NOT NULL,
  checksum CHAR(64) NOT NULL,
  applied_at BIGINT NOT NULL,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const (
	MigrationLockName = "new-api-pilot:migration-runner"
	migrationReady    = "ready"
	migrationDirty    = "dirty"
)

const schemaMigrationProgressBootstrap = `CREATE TABLE IF NOT EXISTS schema_migration_progress (
  version VARCHAR(64) NOT NULL,
  checksum CHAR(64) NOT NULL,
  statement_index INT NOT NULL DEFAULT 0,
  state VARCHAR(16) NOT NULL DEFAULT 'ready',
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

type MigrationRunner struct {
	DB                     *sql.DB
	FS                     fs.FS
	Now                    func() time.Time
	afterStatement         func(MigrationStatementEvent) error
	beforeRecord           func(MigrationRecordEvent) error
	verifyDDLPostcondition func(context.Context, *sql.Conn, string, int) (bool, error)
}

type MigrationStatementEvent struct {
	Version      string
	Index        int
	Count        int
	DDL          bool
	ConnectionID int64
}

type MigrationRecordEvent struct {
	Version      string
	ConnectionID int64
}

type migrationProgress struct {
	Checksum       string
	StatementIndex int
	State          string
}

type migrationProgressExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func NewMigrationRunner(db *sql.DB) *MigrationRunner {
	return &MigrationRunner{DB: db, FS: migrations.Files, Now: time.Now}
}

func (runner *MigrationRunner) Run(ctx context.Context) error {
	if runner.DB == nil {
		return errors.New("migration database is required")
	}
	if runner.FS == nil {
		return errors.New("migration filesystem is required")
	}
	if runner.Now == nil {
		runner.Now = time.Now
	}
	if runner.verifyDDLPostcondition == nil {
		runner.verifyDDLPostcondition = verifyMigrationDDLPostcondition
	}
	connection, err := runner.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer func() { _ = connection.Close() }()
	var lockAcquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", MigrationLockName).Scan(&lockAcquired); err != nil ||
		!lockAcquired.Valid || lockAcquired.Int64 != 1 {
		return fmt.Errorf("acquire migration lock: acquired=%v: %w", lockAcquired, err)
	}
	defer func() {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", MigrationLockName)
	}()

	repository, err := LoadMigrationVersions(runner.FS)
	if err != nil {
		return err
	}
	inventory, err := inspectMigrationTableInventory(ctx, connection)
	if err != nil {
		return err
	}
	if !inventory.SchemaMigration && inventory.Tables != 0 {
		return fmt.Errorf("%w: schema_migration is missing from a non-empty database",
			ErrMigrationSourceInvalid)
	}
	if inventory.SchemaMigrationProgress && !inventory.SchemaMigration {
		return fmt.Errorf("%w: schema_migration_progress exists without schema_migration",
			ErrMigrationSourceInvalid)
	}
	applied := make([]MigrationVersion, 0)
	if inventory.SchemaMigration {
		applied, err = ReadMigrationVersions(ctx, connection)
		if err != nil {
			return err
		}
	}
	if err := ValidateMigrationVersionPrefix(repository, applied, false); err != nil {
		return err
	}
	progress := make([]migrationProgressState, 0)
	if inventory.SchemaMigrationProgress {
		progress, err = readMigrationProgressStates(ctx, connection)
		if err != nil {
			return err
		}
	}
	if len(applied) == 0 && len(progress) == 0 && inventory.NonMetadataTables != 0 {
		return fmt.Errorf("%w: non-empty schema has no applied migration or recovery checkpoint",
			ErrMigrationSourceInvalid)
	}
	if err := validateMigrationProgressSource(repository, applied, progress, runner.FS); err != nil {
		return err
	}

	if !inventory.SchemaMigration {
		if _, err := connection.ExecContext(ctx, schemaMigrationBootstrap); err != nil {
			return fmt.Errorf("bootstrap schema_migration: %w", err)
		}
	}
	if !inventory.SchemaMigrationProgress {
		if _, err := connection.ExecContext(ctx, schemaMigrationProgressBootstrap); err != nil {
			return fmt.Errorf("bootstrap schema_migration_progress: %w", err)
		}
	}

	for _, migration := range repository {
		if err := runner.apply(ctx, connection, migration.Version+".sql"); err != nil {
			return err
		}
	}
	return nil
}

func (runner *MigrationRunner) apply(ctx context.Context, connection *sql.Conn, path string) error {
	contents, err := fs.ReadFile(runner.FS, path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	version := strings.TrimSuffix(path, ".sql")
	checksum := migrationChecksum(contents)

	var appliedChecksum string
	err = connection.QueryRowContext(ctx, "SELECT checksum FROM schema_migration WHERE version = ?", version).Scan(&appliedChecksum)
	switch {
	case err == nil:
		if appliedChecksum != checksum {
			return fmt.Errorf("%w: migration %s checksum mismatch: database=%s repository=%s",
				ErrMigrationChecksumInvalid, version, appliedChecksum, checksum)
		}
		var progressChecksum string
		progressErr := connection.QueryRowContext(ctx,
			"SELECT checksum FROM schema_migration_progress WHERE version = ?", version).Scan(&progressChecksum)
		if progressErr == nil && progressChecksum != checksum {
			return fmt.Errorf("%w: migration %s stale checkpoint checksum mismatch: database=%s repository=%s",
				ErrMigrationChecksumInvalid, version, progressChecksum, checksum)
		}
		if progressErr != nil && !errors.Is(progressErr, sql.ErrNoRows) {
			return fmt.Errorf("read stale migration %s checkpoint: %w", version, progressErr)
		}
		if _, err := connection.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ?", version); err != nil {
			return fmt.Errorf("clear stale migration %s checkpoint: %w", version, err)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read migration %s state: %w", version, err)
	}

	statements, err := SplitSQLStatements(string(contents))
	if err != nil {
		return fmt.Errorf("parse migration %s: %w", version, err)
	}
	progress, err := loadOrCreateMigrationProgress(ctx, connection, version, checksum, runner.Now().Unix())
	if err != nil {
		return err
	}
	if progress.Checksum != checksum {
		return fmt.Errorf("%w: migration %s checkpoint checksum mismatch: database=%s repository=%s",
			ErrMigrationChecksumInvalid, version, progress.Checksum, checksum)
	}
	if progress.StatementIndex < 0 || progress.StatementIndex > len(statements) ||
		(progress.State != migrationReady && progress.State != migrationDirty) ||
		(progress.State == migrationDirty && progress.StatementIndex == len(statements)) {
		return fmt.Errorf("migration %s checkpoint is invalid: statement_index=%d state=%q count=%d",
			version, progress.StatementIndex, progress.State, len(statements))
	}
	var connectionID int64
	if err := connection.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&connectionID); err != nil {
		return fmt.Errorf("read migration %s connection id: %w", version, err)
	}

	for index := progress.StatementIndex; index < len(statements); index++ {
		statement := statements[index]
		ddl := isMigrationDDL(statement)
		recovering := progress.State == migrationDirty && index == progress.StatementIndex
		event := MigrationStatementEvent{
			Version: version, Index: index, Count: len(statements), DDL: ddl, ConnectionID: connectionID,
		}
		if ddl {
			if err := runner.applyDDLStatement(ctx, connection, checksum, statement, event, recovering); err != nil {
				return err
			}
		} else {
			if recovering {
				return fmt.Errorf("migration %s statement %d has an impossible transactional dirty checkpoint", version, index+1)
			}
			if err := runner.applyTransactionalStatement(ctx, connection, checksum, statement, event); err != nil {
				return err
			}
		}
		progress.State = migrationReady
	}
	if runner.beforeRecord != nil {
		if err := runner.beforeRecord(MigrationRecordEvent{Version: version, ConnectionID: connectionID}); err != nil {
			return fmt.Errorf("before recording migration %s: %w", version, err)
		}
	}
	if err := recordCompletedMigration(ctx, connection, version, checksum, runner.Now().Unix()); err != nil {
		return err
	}
	return nil
}

func (runner *MigrationRunner) applyDDLStatement(
	ctx context.Context,
	connection *sql.Conn,
	checksum string,
	statement string,
	event MigrationStatementEvent,
	recovering bool,
) error {
	if recovering {
		applied, err := runner.verifyDDLPostcondition(ctx, connection, event.Version, event.Index)
		if err != nil {
			return fmt.Errorf("verify dirty migration %s statement %d postcondition: %w", event.Version, event.Index+1, err)
		}
		if applied {
			if err := updateMigrationProgress(ctx, connection, event.Version, checksum, event.Index+1, migrationReady, runner.Now().Unix()); err != nil {
				return fmt.Errorf("recover migration %s statement %d checkpoint: %w", event.Version, event.Index+1, err)
			}
			return nil
		}
	}
	if err := updateMigrationProgress(ctx, connection, event.Version, checksum, event.Index, migrationDirty, runner.Now().Unix()); err != nil {
		return fmt.Errorf("mark migration %s statement %d dirty: %w", event.Version, event.Index+1, err)
	}
	if _, err := connection.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("apply migration %s statement %d (dirty checkpoint retained): %w", event.Version, event.Index+1, err)
	}
	if runner.afterStatement != nil {
		if err := runner.afterStatement(event); err != nil {
			return fmt.Errorf("after migration %s statement %d: %w", event.Version, event.Index+1, err)
		}
	}
	applied, err := runner.verifyDDLPostcondition(ctx, connection, event.Version, event.Index)
	if err != nil {
		return fmt.Errorf("verify migration %s statement %d postcondition: %w", event.Version, event.Index+1, err)
	}
	if !applied {
		return fmt.Errorf("migration %s statement %d completed without satisfying its postcondition", event.Version, event.Index+1)
	}
	if err := updateMigrationProgress(ctx, connection, event.Version, checksum, event.Index+1, migrationReady, runner.Now().Unix()); err != nil {
		return fmt.Errorf("checkpoint migration %s statement %d: %w", event.Version, event.Index+1, err)
	}
	return nil
}

func (runner *MigrationRunner) applyTransactionalStatement(
	ctx context.Context,
	connection *sql.Conn,
	checksum string,
	statement string,
	event MigrationStatementEvent,
) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s statement %d: %w", event.Version, event.Index+1, err)
	}
	defer func() { _ = transaction.Rollback() }()
	if err := updateMigrationProgress(ctx, transaction, event.Version, checksum, event.Index, migrationDirty, runner.Now().Unix()); err != nil {
		return fmt.Errorf("mark migration %s statement %d dirty: %w", event.Version, event.Index+1, err)
	}
	if _, err := transaction.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("apply migration %s statement %d: %w", event.Version, event.Index+1, err)
	}
	if runner.afterStatement != nil {
		if err := runner.afterStatement(event); err != nil {
			return fmt.Errorf("after migration %s statement %d: %w", event.Version, event.Index+1, err)
		}
	}
	if err := updateMigrationProgress(ctx, transaction, event.Version, checksum, event.Index+1, migrationReady, runner.Now().Unix()); err != nil {
		return fmt.Errorf("checkpoint migration %s statement %d: %w", event.Version, event.Index+1, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %s statement %d: %w", event.Version, event.Index+1, err)
	}
	return nil
}

func loadOrCreateMigrationProgress(
	ctx context.Context,
	connection *sql.Conn,
	version, checksum string,
	now int64,
) (migrationProgress, error) {
	if _, err := connection.ExecContext(ctx, `INSERT IGNORE INTO schema_migration_progress
  (version, checksum, statement_index, state, updated_at)
VALUES (?, ?, 0, 'ready', ?)`, version, checksum, now); err != nil {
		return migrationProgress{}, fmt.Errorf("create migration %s checkpoint: %w", version, err)
	}
	var progress migrationProgress
	if err := connection.QueryRowContext(ctx, `SELECT checksum, statement_index, state
FROM schema_migration_progress WHERE version = ?`, version).
		Scan(&progress.Checksum, &progress.StatementIndex, &progress.State); err != nil {
		return migrationProgress{}, fmt.Errorf("read migration %s checkpoint: %w", version, err)
	}
	return progress, nil
}

func updateMigrationProgress(
	ctx context.Context,
	executor migrationProgressExecutor,
	version string,
	checksum string,
	statementIndex int,
	state string,
	now int64,
) error {
	if statementIndex < 0 || (state != migrationReady && state != migrationDirty) {
		return errors.New("invalid migration checkpoint update")
	}
	result, err := executor.ExecContext(ctx, `UPDATE schema_migration_progress
SET statement_index = ?, state = ?, updated_at = ?
WHERE version = ? AND checksum = ?`, statementIndex, state, now, version, checksum)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("migration checkpoint compare-and-set failed")
	}
	return nil
}

func recordCompletedMigration(
	ctx context.Context,
	connection *sql.Conn,
	version, checksum string,
	appliedAt int64,
) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s record: %w", version, err)
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, `INSERT INTO schema_migration (version, checksum, applied_at)
VALUES (?, ?, ?)`, version, checksum, appliedAt); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	result, err := transaction.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ? AND checksum = ?", version, checksum)
	if err != nil {
		return fmt.Errorf("clear migration %s checkpoint: %w", version, err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("clear migration %s checkpoint: rows=%d: %w", version, rows, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %s record: %w", version, err)
	}
	return nil
}

func isMigrationDDL(statement string) bool {
	fields := strings.Fields(statement)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToUpper(fields[0]) {
	case "ALTER", "CREATE", "DROP", "RENAME", "TRUNCATE":
		return true
	default:
		return false
	}
}

func verifyMigrationDDLPostcondition(ctx context.Context, connection *sql.Conn, version string, index int) (bool, error) {
	switch version {
	case "0001_initial_schema":
		if index < 0 || index >= len(initialMigrationTableOrder) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		var count int
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ? AND table_type = 'BASE TABLE'`, initialMigrationTableOrder[index]).Scan(&count); err != nil {
			return false, err
		}
		return count == 1, nil
	case "0002_collection_run_scope":
		switch index {
		case 0:
			return verifyCollectionRunScopeColumn(ctx, connection, false)
		case 2:
			return verifyCollectionRunScopeColumn(ctx, connection, true)
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0003_alert_reliability":
		switch index {
		case 0:
			return verifyAlertEvaluationCursorTable(ctx, connection)
		case 1:
			return verifyAlertDeliveryReliabilityColumns(ctx, connection, false)
		case 3:
			return verifyAlertDeliveryReliabilityColumns(ctx, connection, true)
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0004_export_claim_lease":
		if index < 0 || index > 2 {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyExportClaimLeaseSchema(ctx, connection, index)
	case "0005_encryption_reencrypt":
		if index < 0 || index >= len(encryptionReencryptMigrationTableOrder) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyEncryptionReencryptSchema(
			ctx,
			connection,
			encryptionReencryptMigrationTableOrder[0],
			encryptionReencryptMigrationTableOrder[1],
			index,
		)
	case "0006_usage_fact_capacity_indexes":
		if index != 0 {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyUsageFactCapacityIndexes(ctx, connection)
	case "0007_alert_lifecycle":
		switch index {
		case 0:
			return verifyAlertResolutionReasonColumn(ctx, connection)
		case 1:
			return verifyInstanceRetirementSchema(ctx, connection)
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0008_usage_flow_dimensions":
		switch index {
		case 0:
			return verifyUsageFlowDimensions(ctx, connection, "usage_fact_hourly", "hour_ts", "time")
		case 1:
			return verifyUsageFlowDimensions(ctx, connection, "usage_fact_daily", "date_key", "date")
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0009_upstream_log_facts":
		switch index {
		case 0:
			return verifyMigrationTable(ctx, connection, "upstream_log_fact")
		case 1:
			return verifyMigrationTable(ctx, connection, "upstream_log_collection_state")
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0010_site_user_inventory":
		switch index {
		case 0:
			return verifyMigrationTable(ctx, connection, "site_user_inventory")
		case 1:
			return verifyMigrationTable(ctx, connection, "site_user_inventory_hourly")
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0011_site_channel_inventory":
		switch index {
		case 0:
			return verifyMigrationTable(ctx, connection, "site_channel_inventory")
		case 1:
			return verifyMigrationTable(ctx, connection, "site_channel_inventory_hourly")
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0012_performance_history":
		switch index {
		case 0:
			return verifyMigrationTable(ctx, connection, "site_performance_metric_bucket")
		case 1:
			return verifyMigrationTable(ctx, connection, "site_performance_collection_state")
		default:
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
	case "0013_finance_operations":
		tables := []string{"site_topup_order", "site_topup_collection_state", "site_redemption", "site_redemption_collection_state"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0014_upstream_tasks":
		tables := []string{"site_upstream_task", "site_upstream_task_collection_state"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0015_model_catalog":
		tables := []string{"site_model_meta", "site_model_meta_collection_state", "site_channel_model_mapping"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0016_subscription_plans":
		tables := []string{"site_subscription_plan", "site_subscription_plan_collection_state"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0017_pricing_group_catalog":
		tables := []string{"site_group_catalog", "site_pricing_catalog", "pricing_group_collection_state"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0018_system_task_monitoring":
		tables := []string{"site_system_task", "site_system_task_collection_state"}
		if index < 0 || index >= len(tables) {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyMigrationTable(ctx, connection, tables[index])
	case "0019_data_maintenance":
		if index != 0 {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyDataMaintenanceTable(ctx, connection)
	case "0020_site_instance_lifecycle":
		if index < 0 || index > 1 {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifySiteInstanceLifecycleTable(ctx, connection)
	case "0021_customer_amounts":
		if index < 0 || index > 1 {
			return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
		}
		return verifyCustomerAmountsSchema(ctx, connection, index == 1)
	default:
		return false, fmt.Errorf("migration %s has no registered DDL postconditions", version)
	}
}

func verifyCustomerAmountsSchema(ctx context.Context, connection *sql.Conn, requireCheck bool) (bool, error) {
	var columns int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'customer'
  AND column_name IN ('contract_amount', 'payment_amount')
  AND column_type = 'decimal(38,10)' AND is_nullable = 'NO'`).Scan(&columns); err != nil {
		return false, err
	}
	if columns != 2 {
		return false, nil
	}
	if !requireCheck {
		return true, nil
	}
	var checks int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.check_constraints
WHERE constraint_schema = DATABASE()
  AND constraint_name = 'chk_customer_amounts_non_negative'`).Scan(&checks); err != nil {
		return false, err
	}
	return checks == 1, nil
}

func verifySiteInstanceLifecycleTable(ctx context.Context, connection *sql.Conn) (bool, error) {
	ready, err := verifyMigrationTable(ctx, connection, "site_instance_lifecycle")
	if err != nil || !ready {
		return ready, err
	}
	var columns, generated, indexes, foreignKeys, checks int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name='site_instance_lifecycle' AND column_name IN
('id','site_id','node_name','start_minute_ts','end_minute_ts','evidence_status','open_node_name','created_at','updated_at')`).Scan(&columns); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name='site_instance_lifecycle' AND column_name='open_node_name' AND extra LIKE '%STORED GENERATED%'`).Scan(&generated); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name='site_instance_lifecycle' AND index_name IN
('uk_site_instance_lifecycle_start','uk_site_instance_lifecycle_open','idx_site_instance_lifecycle_range')`).Scan(&indexes); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.referential_constraints
WHERE constraint_schema=DATABASE() AND table_name='site_instance_lifecycle' AND constraint_name='fk_site_instance_lifecycle_site' AND update_rule='RESTRICT' AND delete_rule='RESTRICT'`).Scan(&foreignKeys); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.table_constraints
WHERE constraint_schema=DATABASE() AND table_name='site_instance_lifecycle' AND constraint_type='CHECK' AND constraint_name='chk_site_instance_lifecycle_range'`).Scan(&checks); err != nil {
		return false, err
	}
	return columns == 9 && generated == 1 && indexes == 3 && foreignKeys == 1 && checks == 1, nil
}

func verifyDataMaintenanceTable(ctx context.Context, connection *sql.Conn) (bool, error) {
	ready, err := verifyMigrationTable(ctx, connection, "data_maintenance_state")
	if err != nil || !ready {
		return ready, err
	}
	var columns, indexes, foreignKeys, checks int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'data_maintenance_state'
  AND column_name IN ('operation_id','scope_key','scope_revision','date_key','status','cursor_kind','cursor_site_id',
    'cursor_node_name','cursor_bucket_start','cursor_id','site_id','site_config_version','request_id',
    'run_id','error_code','attempt_count','next_attempt_at','last_attempt_at','last_success_at',
    'last_failure_at','created_at','updated_at')`).Scan(&columns); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'data_maintenance_state'
  AND index_name IN ('uk_data_maintenance_scope','idx_data_maintenance_due','idx_data_maintenance_site')`).Scan(&indexes); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.referential_constraints
WHERE constraint_schema = DATABASE() AND table_name = 'data_maintenance_state'
  AND constraint_name IN ('fk_data_maintenance_site','fk_data_maintenance_run')
  AND update_rule = 'RESTRICT' AND delete_rule IN ('RESTRICT','SET NULL')`).Scan(&foreignKeys); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.table_constraints
WHERE constraint_schema = DATABASE() AND table_name = 'data_maintenance_state'
  AND constraint_type = 'CHECK' AND constraint_name IN (
    'chk_data_maintenance_operation','chk_data_maintenance_status','chk_data_maintenance_cursor',
	'chk_data_maintenance_attempt','chk_data_maintenance_site_pair','chk_data_maintenance_shape')`).Scan(&checks); err != nil {
		return false, err
	}
	return columns == 22 && indexes == 3 && foreignKeys == 2 && checks == 6, nil
}

func verifyMigrationTable(ctx context.Context, connection *sql.Conn, table string) (bool, error) {
	var count int
	err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ? AND table_type = 'BASE TABLE' AND engine = 'InnoDB'`, table).Scan(&count)
	return count == 1, err
}

func verifyUsageFlowDimensions(ctx context.Context, connection *sql.Conn, table, timeColumn, suffix string) (bool, error) {
	var columns int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ?
  AND column_name IN ('use_group','token_id','token_name','node_name')`, table).Scan(&columns); err != nil {
		return false, err
	}
	if columns != 4 {
		return false, nil
	}
	rows, err := connection.QueryContext(ctx, `SELECT index_name, seq_in_index, column_name
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name IN (?, ?, ?, ?)
	ORDER BY index_name, seq_in_index`, table, "uk_"+table, "idx_"+table+"_group_"+suffix,
		"idx_"+table+"_token_"+suffix, "idx_"+table+"_node_"+suffix)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	actual := map[string][]string{}
	for rows.Next() {
		var name, column string
		var sequence int
		if err := rows.Scan(&name, &sequence, &column); err != nil {
			return false, err
		}
		if sequence != len(actual[name])+1 {
			return false, nil
		}
		actual[name] = append(actual[name], column)
	}
	want := map[string][]string{
		"uk_" + table:                       {"site_id", "remote_user_id", "model_name", "channel_id", "use_group", "token_id", "node_name", timeColumn},
		"idx_" + table + "_group_" + suffix: {"site_id", "use_group", timeColumn},
		"idx_" + table + "_token_" + suffix: {"site_id", "token_id", timeColumn},
		"idx_" + table + "_node_" + suffix:  {"site_id", "node_name", timeColumn},
	}
	return reflect.DeepEqual(actual, want), rows.Err()
}

func verifyAlertResolutionReasonColumn(ctx context.Context, connection *sql.Conn) (bool, error) {
	var alertColumn int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'alert_event'
  AND column_name = 'resolution_reason' AND column_type = 'varchar(32)' AND is_nullable = 'YES'`).Scan(&alertColumn); err != nil {
		return false, err
	}
	return alertColumn == 1, nil
}

func verifyInstanceRetirementSchema(ctx context.Context, connection *sql.Conn) (bool, error) {
	var instanceColumn, indexColumns int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'site_instance'
  AND column_name = 'retired_at' AND column_type = 'bigint' AND is_nullable = 'YES'`).Scan(&instanceColumn); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'site_instance' AND index_name = 'idx_site_instance_active'`).Scan(&indexColumns); err != nil {
		return false, err
	}
	return instanceColumn == 1 && indexColumns == 3, nil
}

func verifyUsageFactCapacityIndexes(ctx context.Context, connection *sql.Conn) (bool, error) {
	type indexColumn struct {
		Name     string
		Sequence int
		Column   string
		Unique   bool
	}
	rows, err := connection.QueryContext(ctx, `SELECT index_name, seq_in_index, column_name, non_unique = 0
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'usage_fact_hourly'
  AND index_name IN (
    'idx_usage_fact_hourly_time_user',
    'idx_usage_fact_hourly_time_model_user',
    'idx_usage_fact_hourly_time_channel_user'
  )
ORDER BY index_name, seq_in_index`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	actual := make(map[string][]indexColumn, 3)
	for rows.Next() {
		var column indexColumn
		if err := rows.Scan(&column.Name, &column.Sequence, &column.Column, &column.Unique); err != nil {
			return false, err
		}
		actual[column.Name] = append(actual[column.Name], column)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	want := map[string][]string{
		"idx_usage_fact_hourly_time_user":         {"hour_ts", "site_id", "remote_user_id"},
		"idx_usage_fact_hourly_time_model_user":   {"hour_ts", "site_id", "model_name", "remote_user_id"},
		"idx_usage_fact_hourly_time_channel_user": {"hour_ts", "site_id", "channel_id", "remote_user_id"},
	}
	if len(actual) != len(want) {
		return false, nil
	}
	for name, columns := range want {
		states := actual[name]
		if len(states) != len(columns) {
			return false, nil
		}
		for position, state := range states {
			if state.Sequence != position+1 || state.Column != columns[position] || state.Unique {
				return false, fmt.Errorf("usage fact capacity index postcondition mismatch for %s: %#v", name, states)
			}
		}
	}
	return true, nil
}

func verifyEncryptionReencryptSchema(
	ctx context.Context,
	connection *sql.Conn,
	jobTable string,
	itemTable string,
	stage int,
) (bool, error) {
	table := jobTable
	columns := []string{
		"id", "old_key_id", "new_key_id", "active_key", "state", "inventory_hash",
		"total_items", "staged_items", "created_at", "updated_at",
	}
	if stage == 1 {
		table = itemTable
		columns = []string{
			"job_id", "row_type", "row_id", "aad_identity", "source_hash",
			"new_ciphertext", "needs_update", "created_at",
		}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	arguments := make([]any, 0, len(columns)+1)
	arguments = append(arguments, table)
	for _, column := range columns {
		arguments = append(arguments, column)
	}
	var count int
	query := `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name IN (` + placeholders + `)`
	if err := connection.QueryRowContext(ctx, query, arguments...).Scan(&count); err != nil {
		return false, err
	}
	if count != len(columns) {
		return false, nil
	}
	var indexColumns int
	if stage == 0 {
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ?
  AND index_name = 'uk_encryption_reencrypt_active' AND column_name = 'active_key' AND non_unique = 0`, jobTable).
			Scan(&indexColumns); err != nil {
			return false, err
		}
		return indexColumns == 1, nil
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ?
  AND index_name = 'PRIMARY' AND column_name IN ('job_id','row_type','row_id') AND non_unique = 0`, itemTable).
		Scan(&indexColumns); err != nil {
		return false, err
	}
	return indexColumns == 3, nil
}

func verifyExportClaimLeaseSchema(ctx context.Context, connection *sql.Conn, stage int) (bool, error) {
	type columnState struct {
		Name       string
		ColumnType string
		Nullable   string
		Charset    sql.NullString
		Collation  sql.NullString
	}
	rows, err := connection.QueryContext(ctx, `SELECT column_name, column_type, is_nullable,
       character_set_name, collation_name
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'export_job'
  AND column_name IN ('claim_token', 'lease_expires_at')`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	states := make(map[string]columnState, 2)
	for rows.Next() {
		var state columnState
		if err := rows.Scan(&state.Name, &state.ColumnType, &state.Nullable, &state.Charset, &state.Collation); err != nil {
			return false, err
		}
		states[state.Name] = state
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	claim, exists := states["claim_token"]
	if !exists {
		return false, nil
	}
	if claim.ColumnType != "varchar(64)" || claim.Nullable != "YES" ||
		claim.Charset.String != "ascii" || claim.Collation.String != "ascii_bin" {
		return false, fmt.Errorf("export claim lease column postcondition mismatch: %#v", states)
	}
	if stage == 0 {
		return true, nil
	}
	lease, exists := states["lease_expires_at"]
	if !exists {
		return false, nil
	}
	if lease.ColumnType != "bigint" || lease.Nullable != "YES" || lease.Charset.Valid || lease.Collation.Valid {
		return false, fmt.Errorf("export claim lease column postcondition mismatch: %#v", states)
	}
	if stage == 1 {
		return true, nil
	}
	type indexState struct {
		Sequence int
		Column   string
		Unique   bool
	}
	indexRows, err := connection.QueryContext(ctx, `SELECT seq_in_index, column_name, non_unique = 0
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'export_job'
	  AND index_name = 'idx_export_job_claim'
ORDER BY seq_in_index`)
	if err != nil {
		return false, err
	}
	defer func() { _ = indexRows.Close() }()
	indexes := make([]indexState, 0, 4)
	for indexRows.Next() {
		var state indexState
		if err := indexRows.Scan(&state.Sequence, &state.Column, &state.Unique); err != nil {
			return false, err
		}
		indexes = append(indexes, state)
	}
	if err := indexRows.Err(); err != nil {
		return false, err
	}
	want := []string{"status", "lease_expires_at", "next_attempt_at", "id"}
	if len(indexes) != len(want) {
		return false, nil
	}
	for position, state := range indexes {
		if state.Sequence != position+1 || state.Column != want[position] || state.Unique {
			return false, fmt.Errorf("export claim index postcondition mismatch: %#v", indexes)
		}
	}
	return true, nil
}

func verifyAlertEvaluationCursorTable(ctx context.Context, connection *sql.Conn) (bool, error) {
	var count int
	err := connection.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = 'alert_evaluation_cursor'
  AND table_type = 'BASE TABLE' AND engine = 'InnoDB'`).Scan(&count)
	if err != nil || count != 1 {
		return false, err
	}
	var columns, primaryColumns int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'alert_evaluation_cursor'
  AND column_name IN ('active_key', 'last_sample_at', 'last_sample_key', 'created_at', 'updated_at')`).Scan(&columns); err != nil {
		return false, err
	}
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'alert_evaluation_cursor'
  AND index_name = 'PRIMARY' AND column_name = 'active_key' AND non_unique = 0`).Scan(&primaryColumns); err != nil {
		return false, err
	}
	return columns == 5 && primaryColumns == 1, nil
}

func verifyAlertDeliveryReliabilityColumns(
	ctx context.Context,
	connection *sql.Conn,
	requirePayloadNotNull bool,
) (bool, error) {
	type columnState struct {
		Name     string
		DataType string
		Nullable string
	}
	rows, err := connection.QueryContext(ctx, `SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'alert_delivery'
  AND column_name IN ('claim_token', 'lease_expires_at', 'payload_snapshot')`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	states := make(map[string]columnState, 3)
	for rows.Next() {
		var state columnState
		if err := rows.Scan(&state.Name, &state.DataType, &state.Nullable); err != nil {
			return false, err
		}
		states[state.Name] = state
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(states) != 3 {
		return false, nil
	}
	if states["claim_token"].DataType != "varchar" || states["claim_token"].Nullable != "YES" ||
		states["lease_expires_at"].DataType != "bigint" || states["lease_expires_at"].Nullable != "YES" ||
		states["payload_snapshot"].DataType != "json" {
		return false, fmt.Errorf("alert delivery reliability column postcondition mismatch: %#v", states)
	}
	if requirePayloadNotNull && states["payload_snapshot"].Nullable != "NO" {
		return false, nil
	}
	var indexColumns int
	if err := connection.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'alert_delivery'
  AND index_name = 'idx_alert_delivery_claim'
  AND column_name IN ('status', 'lease_expires_at', 'next_retry_at', 'id')`).Scan(&indexColumns); err != nil {
		return false, err
	}
	return indexColumns == 4, nil
}

func verifyCollectionRunScopeColumn(ctx context.Context, connection *sql.Conn, requireNotNull bool) (bool, error) {
	var dataType, nullable, previousColumn string
	var defaultValue sql.NullString
	err := connection.QueryRowContext(ctx, `SELECT c.data_type, c.is_nullable, c.column_default,
       COALESCE((SELECT previous.column_name FROM information_schema.columns previous
                 WHERE previous.table_schema = c.table_schema AND previous.table_name = c.table_name
                   AND previous.ordinal_position = c.ordinal_position - 1), '')
FROM information_schema.columns c
WHERE c.table_schema = DATABASE() AND c.table_name = 'collection_run' AND c.column_name = 'scope'`).
		Scan(&dataType, &nullable, &defaultValue, &previousColumn)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if dataType != "json" || defaultValue.Valid || previousColumn != "end_timestamp" ||
		(nullable != "YES" && nullable != "NO") {
		return false, fmt.Errorf("scope column postcondition mismatch: type=%s nullable=%s default=%v previous=%s",
			dataType, nullable, defaultValue, previousColumn)
	}
	if requireNotNull && nullable != "NO" {
		return false, nil
	}
	return true, nil
}

var initialMigrationTableOrder = []string{
	"schema_migration", "platform_user", "site", "site_channel", "site_monitoring_pause",
	"site_capability", "customer", "account", "collection_cursor", "collection_run",
	"collection_window", "collection_run_window", "aggregation_bucket_lock", "usage_fact_hourly",
	"usage_fact_daily", "account_stat_hourly", "account_stat_daily", "customer_stat_hourly",
	"customer_stat_daily", "site_stat_hourly", "site_stat_daily", "global_stat_hourly",
	"global_stat_daily", "model_stat_hourly", "model_stat_daily", "channel_stat_hourly",
	"channel_stat_daily", "site_instance", "site_instance_status_minutely", "site_instance_status_hourly",
	"site_instance_status_daily", "site_status_minutely", "site_status_hourly", "site_status_daily",
	"alert_rule", "alert_event", "alert_delivery", "export_job", "platform_setting",
}

var encryptionReencryptMigrationTableOrder = []string{
	"encryption_reencrypt_job",
	"encryption_reencrypt_item",
}

func SplitSQLStatements(input string) ([]string, error) {
	statements := make([]string, 0)
	var current strings.Builder
	var quote rune
	escaped := false
	inLineComment := false
	inBlockComment := false
	runes := []rune(input)

	flush := func() {
		statement := strings.TrimSpace(current.String())
		if statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}

	for index := 0; index < len(runes); index++ {
		character := runes[index]
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}

		if inLineComment {
			if character == '\n' {
				inLineComment = false
				current.WriteRune(character)
			}
			continue
		}
		if inBlockComment {
			if character == '*' && next == '/' {
				inBlockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			current.WriteRune(character)
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if character == quote {
				if next == quote && quote != '`' {
					current.WriteRune(next)
					index++
					continue
				}
				quote = 0
			}
			continue
		}

		switch {
		case character == '-' && next == '-' && (index+2 >= len(runes) || runes[index+2] == ' ' || runes[index+2] == '\t'):
			inLineComment = true
			index++
		case character == '#':
			inLineComment = true
		case character == '/' && next == '*':
			inBlockComment = true
			index++
		case character == '\'' || character == '"' || character == '`':
			quote = character
			current.WriteRune(character)
		case character == ';':
			flush()
		default:
			current.WriteRune(character)
		}
	}
	if quote != 0 || inBlockComment {
		return nil, errors.New("unterminated SQL quote or block comment")
	}
	flush()
	return statements, nil
}
