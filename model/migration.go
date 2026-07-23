package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
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
	if version != "0001_initial_schema" {
		return false, fmt.Errorf("migration %s has no registered DDL postconditions", version)
	}
	if index < 0 || index >= len(initialMigrationTableOrder) {
		return false, fmt.Errorf("no postcondition for DDL statement %d", index+1)
	}
	return verifyMigrationTable(ctx, connection, initialMigrationTableOrder[index])
}

func verifyMigrationTable(ctx context.Context, connection *sql.Conn, table string) (bool, error) {
	var count int
	err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ? AND table_type = 'BASE TABLE' AND engine = 'InnoDB'`, table).Scan(&count)
	return count == 1, err
}

var initialMigrationTableOrder = []string{
	"schema_migration", "schema_migration_progress", "platform_user", "site", "site_channel",
	"site_monitoring_pause", "site_capability", "customer", "account", "collection_cursor",
	"collection_run", "collection_window", "collection_run_window", "aggregation_bucket_lock", "usage_fact_hourly",
	"usage_fact_daily", "account_stat_hourly", "account_stat_daily", "customer_stat_hourly", "customer_stat_daily",
	"site_stat_hourly", "site_stat_daily", "global_stat_hourly", "global_stat_daily", "model_stat_hourly",
	"model_stat_daily", "channel_stat_hourly", "channel_stat_daily", "site_instance", "site_instance_status_minutely",
	"site_instance_status_hourly", "site_instance_status_daily", "site_status_minutely", "site_status_hourly", "site_status_daily",
	"alert_rule", "alert_event", "alert_delivery", "export_job", "platform_setting",
	"alert_evaluation_cursor", "encryption_reencrypt_job", "encryption_reencrypt_item", "upstream_log_fact", "upstream_log_collection_state",
	"site_user_inventory", "site_user_inventory_hourly", "site_channel_inventory", "site_channel_inventory_hourly", "site_performance_metric_bucket",
	"site_performance_collection_state", "site_topup_order", "site_topup_collection_state", "site_redemption", "site_redemption_collection_state",
	"site_upstream_task", "site_upstream_task_collection_state", "site_model_meta", "site_model_meta_collection_state", "site_channel_model_mapping",
	"site_subscription_plan", "site_subscription_plan_collection_state", "site_group_catalog", "site_pricing_catalog", "pricing_group_collection_state",
	"site_system_task", "site_system_task_collection_state", "data_maintenance_state", "site_instance_lifecycle",
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
