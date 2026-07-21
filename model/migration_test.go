package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"new-api-pilot/migrations"
)

func TestSplitSQLStatements(t *testing.T) {
	input := `-- comment
CREATE TABLE first_table (value VARCHAR(20) DEFAULT ';');
/* block ; comment */
INSERT INTO first_table VALUES ('it''s fine');`
	statements, err := SplitSQLStatements(input)
	if err != nil {
		t.Fatalf("SplitSQLStatements() error = %v", err)
	}
	if len(statements) != 2 || !strings.Contains(statements[0], "DEFAULT ';'") {
		t.Fatalf("statements = %#v", statements)
	}
}

func TestInitialMigrationContainsAllDesignedTables(t *testing.T) {
	contents, err := migrations.Files.ReadFile("0001_initial_schema.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	statements, err := SplitSQLStatements(string(contents))
	if err != nil {
		t.Fatalf("parse migration: %v", err)
	}
	if len(statements) != 39 {
		t.Fatalf("migration statement count = %d, want 39", len(statements))
	}
	for _, table := range expectedTables {
		if !strings.Contains(string(contents), "CREATE TABLE IF NOT EXISTS "+table+" ") {
			t.Errorf("migration is missing table %s", table)
		}
	}
	contracts := parseInitialTableContracts(t, statements)
	if len(contracts) != len(expectedTables) {
		t.Fatalf("parsed table contract count = %d, want %d", len(contracts), len(expectedTables))
	}
	for _, table := range expectedTables {
		contract, exists := contracts[table]
		if !exists {
			t.Errorf("migration contract is missing table %s", table)
			continue
		}
		if len(contract.Columns) == 0 || len(contract.Indexes) == 0 {
			t.Errorf("migration contract for %s has no columns or indexes: %#v", table, contract)
		}
		if contract.Engine != "InnoDB" || contract.Charset != "utf8mb4" || contract.Collation != "utf8mb4_unicode_ci" {
			t.Errorf("migration contract for %s engine/charset/collation = %s/%s/%s", table, contract.Engine, contract.Charset, contract.Collation)
		}
	}
}

func TestMySQLMigrationAndSeeds(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 5, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()

	runner := NewMigrationRunner(database.SQL)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("idempotent migration run: %v", err)
	}
	schemaSummary, err := VerifyAuthoritativeSchema(ctx, database.SQL)
	if err != nil {
		t.Fatalf("verify authoritative schema contract: %v", err)
	}
	if schemaSummary.Tables != 69 || schemaSummary.ForeignKeys != 63 {
		t.Fatalf("authoritative schema summary = %#v", schemaSummary)
	}
	assertChecksumMismatchIsRejected(t, ctx, database.SQL, runner)

	seeder := NewSeeder(database.SQL)
	if err := seeder.Run(ctx); err != nil {
		t.Fatalf("first seed run: %v", err)
	}
	if err := seeder.Run(ctx); err != nil {
		t.Fatalf("idempotent seed run: %v", err)
	}
	assertExactDefaultSeeds(t, ctx, database.SQL)
}

func TestMySQLMigrationRecoversDDLAndDMLCommitGaps(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("prepare production migrations: %v", err)
	}

	faults := []struct {
		name         string
		afterIndex   int
		beforeRecord bool
	}{
		{name: "after_add_ddl", afterIndex: 0},
		{name: "after_dml", afterIndex: 1},
		{name: "after_modify_ddl", afterIndex: 2},
		{name: "before_final_record", afterIndex: -1, beforeRecord: true},
	}
	for caseIndex, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			version := fmt.Sprintf("990%d_scope_%s", caseIndex, fault.name)
			table := fmt.Sprintf("migration_scope_fixture_%d", caseIndex)
			migration := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN scope JSON NULL AFTER id;
UPDATE %s SET scope = JSON_OBJECT('only_missing', TRUE) WHERE scope IS NULL;
ALTER TABLE %s MODIFY COLUMN scope JSON NOT NULL AFTER id;`, table, table, table)
			migrationFS := migrationTestFSWithAdditional(t, version+".sql", migration)
			cleanupMigrationFixture(t, ctx, database.SQL, version, table)
			t.Cleanup(func() { cleanupMigrationFixture(t, context.Background(), database.SQL, version, table) })
			if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
  id BIGINT NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, table)); err != nil {
				t.Fatalf("create migration fixture: %v", err)
			}
			if _, err := database.SQL.ExecContext(ctx, "INSERT INTO "+table+" (id) VALUES (1)"); err != nil {
				t.Fatalf("seed migration fixture: %v", err)
			}

			runner := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: func() time.Time { return time.Unix(1_752_400_800, 0) },
				verifyDDLPostcondition: migrationFixtureVerifier(table),
			}
			runner.afterStatement = func(event MigrationStatementEvent) error {
				if event.Index != fault.afterIndex {
					return nil
				}
				return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
			}
			if fault.beforeRecord {
				runner.beforeRecord = func(event MigrationRecordEvent) error {
					return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
				}
			}
			if err := runner.Run(ctx); err == nil {
				t.Fatal("fault-injected migration unexpectedly succeeded")
			}

			recovery := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: runner.Now,
				verifyDDLPostcondition: migrationFixtureVerifier(table),
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("recover migration after %s: %v", fault.name, err)
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("idempotent migration after %s: %v", fault.name, err)
			}
			assertRecoveredMigrationFixture(t, ctx, database.SQL, version, table)
		})
	}
}

func TestMySQLAlertReliabilityMigrationRecoversEveryCommitGap(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("prepare production migrations: %v", err)
	}

	faults := []struct {
		name         string
		afterIndex   int
		beforeRecord bool
	}{
		{name: "after_cursor_create", afterIndex: 0},
		{name: "after_delivery_alter", afterIndex: 1},
		{name: "during_payload_backfill", afterIndex: 2},
		{name: "after_payload_not_null", afterIndex: 3},
		{name: "before_final_record", afterIndex: -1, beforeRecord: true},
	}
	for caseIndex, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			version := fmt.Sprintf("991%d_alert_reliability_%s", caseIndex, fault.name)
			cursorTable := fmt.Sprintf("alert_cursor_recovery_fixture_%d", caseIndex)
			deliveryTable := fmt.Sprintf("alert_delivery_recovery_fixture_%d", caseIndex)
			cleanupAlertReliabilityMigrationFixture(t, ctx, database.SQL, version, cursorTable, deliveryTable)
			t.Cleanup(func() {
				cleanupAlertReliabilityMigrationFixture(t, context.Background(), database.SQL, version, cursorTable, deliveryTable)
			})
			if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
  id BIGINT NOT NULL AUTO_INCREMENT,
  alert_event_id BIGINT NULL,
  event_type VARCHAR(16) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempt_count INT NOT NULL DEFAULT 0,
  next_retry_at BIGINT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, deliveryTable)); err != nil {
				t.Fatalf("create alert delivery recovery fixture: %v", err)
			}
			if _, err := database.SQL.ExecContext(ctx, "INSERT INTO "+deliveryTable+
				" (alert_event_id, event_type, next_retry_at) VALUES (NULL, 'test', 1), (42, 'firing', 1)"); err != nil {
				t.Fatalf("seed alert delivery recovery fixture: %v", err)
			}
			migration := alertReliabilityFixtureMigration(cursorTable, deliveryTable)
			migrationFS := migrationTestFSWithAdditional(t, version+".sql", migration)
			runner := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: func() time.Time { return time.Unix(1_752_400_800, 0) },
				verifyDDLPostcondition: alertReliabilityFixtureVerifier(cursorTable, deliveryTable),
			}
			runner.afterStatement = func(event MigrationStatementEvent) error {
				if event.Index != fault.afterIndex {
					return nil
				}
				return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
			}
			if fault.beforeRecord {
				runner.beforeRecord = func(event MigrationRecordEvent) error {
					return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
				}
			}
			if err := runner.Run(ctx); err == nil {
				t.Fatal("fault-injected alert reliability migration unexpectedly succeeded")
			}

			recovery := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: runner.Now,
				verifyDDLPostcondition: alertReliabilityFixtureVerifier(cursorTable, deliveryTable),
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("recover alert reliability migration after %s: %v", fault.name, err)
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("idempotent alert reliability migration after %s: %v", fault.name, err)
			}
			assertRecoveredAlertReliabilityFixture(t, ctx, database.SQL, version, cursorTable, deliveryTable)
		})
	}
}

func alertReliabilityFixtureMigration(cursorTable, deliveryTable string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  active_key VARCHAR(384) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  last_sample_at BIGINT NOT NULL,
  last_sample_key VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (active_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
ALTER TABLE %s
  ADD COLUMN claim_token VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER attempt_count,
  ADD COLUMN lease_expires_at BIGINT NULL AFTER claim_token,
  ADD COLUMN payload_snapshot JSON NULL AFTER lease_expires_at,
  ADD KEY idx_alert_delivery_claim (status, lease_expires_at, next_retry_at, id);
UPDATE %s
SET payload_snapshot = CASE
  WHEN event_type = 'test' THEN JSON_OBJECT('version', 1, 'kind', 'test')
  ELSE JSON_OBJECT('version', 1, 'kind', 'legacy', 'alert_event_id', CAST(alert_event_id AS CHAR), 'event_type', event_type)
END
WHERE payload_snapshot IS NULL;
ALTER TABLE %s MODIFY COLUMN payload_snapshot JSON NOT NULL AFTER lease_expires_at;`,
		cursorTable, deliveryTable, deliveryTable, deliveryTable)
}

func alertReliabilityFixtureVerifier(
	cursorTable string,
	deliveryTable string,
) func(context.Context, *sql.Conn, string, int) (bool, error) {
	return func(ctx context.Context, connection *sql.Conn, _ string, index int) (bool, error) {
		if index == 0 {
			var count int
			err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ? AND table_type = 'BASE TABLE'`, cursorTable).Scan(&count)
			return count == 1, err
		}
		if index != 1 && index != 3 {
			return false, fmt.Errorf("no alert reliability fixture postcondition for statement %d", index+1)
		}
		var columns, notNullPayload, indexColumns int
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ?
  AND column_name IN ('claim_token', 'lease_expires_at', 'payload_snapshot')`, deliveryTable).Scan(&columns); err != nil {
			return false, err
		}
		if columns != 3 {
			return false, nil
		}
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'idx_alert_delivery_claim'`, deliveryTable).
			Scan(&indexColumns); err != nil {
			return false, err
		}
		if indexColumns != 4 {
			return false, nil
		}
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'payload_snapshot' AND is_nullable = 'NO'`, deliveryTable).
			Scan(&notNullPayload); err != nil {
			return false, err
		}
		return index == 1 || notNullPayload == 1, nil
	}
}

func assertRecoveredAlertReliabilityFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	version string,
	cursorTable string,
	deliveryTable string,
) {
	t.Helper()
	verifier := alertReliabilityFixtureVerifier(cursorTable, deliveryTable)
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve alert reliability verification connection: %v", err)
	}
	defer func() { _ = connection.Close() }()
	if applied, err := verifier(ctx, connection, version, 3); err != nil || !applied {
		t.Fatalf("alert reliability final schema = %t, %v", applied, err)
	}
	rows, err := db.QueryContext(ctx, "SELECT JSON_UNQUOTE(JSON_EXTRACT(payload_snapshot, '$.kind')) FROM "+deliveryTable+" ORDER BY id")
	if err != nil {
		t.Fatalf("read recovered alert payload snapshots: %v", err)
	}
	defer func() { _ = rows.Close() }()
	kinds := []string{}
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scan recovered alert payload snapshot: %v", err)
		}
		kinds = append(kinds, kind)
	}
	if !reflect.DeepEqual(kinds, []string{"test", "legacy"}) {
		t.Fatalf("recovered alert payload kinds = %v", kinds)
	}
	var applied, progress int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration WHERE version = ?", version).Scan(&applied); err != nil {
		t.Fatalf("count applied alert reliability migration: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_progress WHERE version = ?", version).Scan(&progress); err != nil {
		t.Fatalf("count alert reliability migration progress: %v", err)
	}
	if applied != 1 || progress != 0 {
		t.Fatalf("alert reliability migration records applied=%d progress=%d", applied, progress)
	}
}

func cleanupAlertReliabilityMigrationFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	version string,
	cursorTable string,
	deliveryTable string,
) {
	t.Helper()
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ?", version)
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration WHERE version = ?", version)
	for _, table := range []string{cursorTable, deliveryTable} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil && ctx.Err() == nil {
			t.Errorf("drop alert reliability migration fixture %s: %v", table, err)
		}
	}
}

func TestMySQLEncryptionReencryptMigrationRecoversEveryCommitGap(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("prepare production migrations: %v", err)
	}

	payload, err := migrations.Files.ReadFile("0005_encryption_reencrypt.sql")
	if err != nil {
		t.Fatalf("read encryption re-encryption migration: %v", err)
	}
	faults := []struct {
		name         string
		afterIndex   int
		beforeRecord bool
	}{
		{name: "after_job_create", afterIndex: 0},
		{name: "after_item_create", afterIndex: 1},
		{name: "before_final_record", afterIndex: -1, beforeRecord: true},
	}
	for caseIndex, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			version := fmt.Sprintf("992%d_encryption_reencrypt_%s", caseIndex, fault.name)
			jobTable := fmt.Sprintf("encryption_reencrypt_job_fixture_%d", caseIndex)
			itemTable := fmt.Sprintf("encryption_reencrypt_item_fixture_%d", caseIndex)
			cleanupEncryptionReencryptMigrationFixture(t, ctx, database.SQL, version, jobTable, itemTable)
			t.Cleanup(func() {
				cleanupEncryptionReencryptMigrationFixture(
					t, context.Background(), database.SQL, version, jobTable, itemTable,
				)
			})
			migration := strings.ReplaceAll(string(payload), encryptionReencryptMigrationTableOrder[0], jobTable)
			migration = strings.ReplaceAll(migration, encryptionReencryptMigrationTableOrder[1], itemTable)
			migrationFS := migrationTestFSWithAdditional(t, version+".sql", migration)
			verifier := func(ctx context.Context, connection *sql.Conn, _ string, index int) (bool, error) {
				return verifyEncryptionReencryptSchema(ctx, connection, jobTable, itemTable, index)
			}
			runner := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: func() time.Time { return time.Unix(1_752_400_800, 0) },
				verifyDDLPostcondition: verifier,
			}
			runner.afterStatement = func(event MigrationStatementEvent) error {
				if event.Index != fault.afterIndex {
					return nil
				}
				return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
			}
			if fault.beforeRecord {
				runner.beforeRecord = func(event MigrationRecordEvent) error {
					return killMySQLConnection(ctx, database.SQL, event.ConnectionID)
				}
			}
			if err := runner.Run(ctx); err == nil {
				t.Fatal("fault-injected encryption re-encryption migration unexpectedly succeeded")
			}

			recovery := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: runner.Now, verifyDDLPostcondition: verifier,
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("recover encryption re-encryption migration after %s: %v", fault.name, err)
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("idempotent encryption re-encryption migration after %s: %v", fault.name, err)
			}
			assertRecoveredEncryptionReencryptMigration(
				t, ctx, database.SQL, version, jobTable, itemTable,
			)
		})
	}
}

func assertRecoveredEncryptionReencryptMigration(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	version string,
	jobTable string,
	itemTable string,
) {
	t.Helper()
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve encryption re-encryption verification connection: %v", err)
	}
	defer func() { _ = connection.Close() }()
	for stage := range encryptionReencryptMigrationTableOrder {
		applied, err := verifyEncryptionReencryptSchema(ctx, connection, jobTable, itemTable, stage)
		if err != nil || !applied {
			t.Fatalf("encryption re-encryption schema stage %d = %t, %v", stage, applied, err)
		}
	}
	var applied, progress int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration WHERE version = ?", version).Scan(&applied); err != nil {
		t.Fatalf("count applied encryption re-encryption migration: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_progress WHERE version = ?", version).Scan(&progress); err != nil {
		t.Fatalf("count encryption re-encryption migration progress: %v", err)
	}
	if applied != 1 || progress != 0 {
		t.Fatalf("encryption re-encryption migration records applied=%d progress=%d", applied, progress)
	}
}

func cleanupEncryptionReencryptMigrationFixture(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	version string,
	jobTable string,
	itemTable string,
) {
	t.Helper()
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ?", version)
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration WHERE version = ?", version)
	for _, table := range []string{itemTable, jobTable} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil && ctx.Err() == nil {
			t.Errorf("drop encryption re-encryption migration fixture %s: %v", table, err)
		}
	}
}

func TestMySQLMigrationRejectsProgressChecksumMismatch(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 5, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("prepare production migrations: %v", err)
	}
	const version = "9999_progress_checksum"
	const table = "migration_checksum_fixture"
	cleanupMigrationFixture(t, ctx, database.SQL, version, table)
	defer cleanupMigrationFixture(t, context.Background(), database.SQL, version, table)
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (id BIGINT NOT NULL PRIMARY KEY)
ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, table)); err != nil {
		t.Fatalf("create checksum fixture: %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO schema_migration_progress
  (version, checksum, statement_index, state, updated_at)
VALUES (?, ?, 0, 'ready', ?)`, version, strings.Repeat("0", 64), time.Now().Unix()); err != nil {
		t.Fatalf("insert mismatched checkpoint: %v", err)
	}
	runner := &MigrationRunner{
		DB: database.SQL,
		FS: migrationTestFSWithAdditional(t, version+".sql",
			"ALTER TABLE "+table+" ADD COLUMN scope JSON NULL"),
		Now: time.Now, verifyDDLPostcondition: migrationFixtureVerifier(table),
	}
	if err := runner.Run(ctx); err == nil || !strings.Contains(err.Error(), "checkpoint checksum mismatch") {
		t.Fatalf("mismatched checkpoint error = %v", err)
	}
}

func TestMySQLMigrationWaitsForBackupAdvisoryLock(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("prepare migrations: %v", err)
	}

	backupConnection, err := database.SQL.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve backup lock connection: %v", err)
	}
	defer func() { _ = backupConnection.Close() }()
	var acquired sql.NullInt64
	if err := backupConnection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", MigrationLockName).Scan(&acquired); err != nil ||
		!acquired.Valid || acquired.Int64 != 1 {
		t.Fatalf("acquire backup migration lock = %v, %v", acquired, err)
	}
	released := false
	defer func() {
		if released {
			return
		}
		_, _ = backupConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", MigrationLockName)
	}()

	result := make(chan error, 1)
	go func() { result <- NewMigrationRunner(database.SQL).Run(ctx) }()
	select {
	case err := <-result:
		t.Fatalf("migration returned while the backup lock was held: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if _, err := backupConnection.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", MigrationLockName); err != nil {
		t.Fatalf("release backup migration lock: %v", err)
	}
	released = true
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("migration after backup lock release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("migration did not resume after backup lock release: %v", ctx.Err())
	}
}

func TestMySQLMigrationSourceGate(t *testing.T) {
	if os.Getenv("TEST_DATABASE_DSN") == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	repository, err := LoadMigrationVersions(migrations.Files)
	if err != nil {
		t.Fatalf("load repository migrations: %v", err)
	}
	if len(repository) < 3 {
		t.Fatalf("repository migration count = %d, want at least 3", len(repository))
	}

	t.Run("normal prefix and full current", func(t *testing.T) {
		database := openIsolatedMigrationSourceDatabase(t)
		prefixRunner := NewMigrationRunner(database.SQL)
		prefixRunner.FS = migrationRepositoryPrefixFS(t, len(repository)-1)
		if err := prefixRunner.Run(context.Background()); err != nil {
			t.Fatalf("apply repository prefix: %v", err)
		}
		if err := NewMigrationRunner(database.SQL).Run(context.Background()); err != nil {
			t.Fatalf("complete repository prefix: %v", err)
		}
		if err := NewMigrationRunner(database.SQL).Run(context.Background()); err != nil {
			t.Fatalf("rerun full current repository: %v", err)
		}
		applied, err := ReadMigrationVersions(context.Background(), database.SQL)
		if err != nil {
			t.Fatalf("read fully applied migrations: %v", err)
		}
		if err := ValidateMigrationVersionPrefix(repository, applied, true); err != nil {
			t.Fatalf("validate fully applied migrations: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB, []MigrationVersion)
		want   error
	}{
		{
			name: "unknown version with valid checksum",
			mutate: func(t *testing.T, db *sql.DB, _ []MigrationVersion) {
				execMigrationSourceMutation(t, db, `INSERT INTO schema_migration
  (version, checksum, applied_at) VALUES ('9999_unknown_review', ?, 1)`, strings.Repeat("a", 64))
			},
			want: ErrMigrationSourceInvalid,
		},
		{
			name: "unknown version and progress",
			mutate: func(t *testing.T, db *sql.DB, _ []MigrationVersion) {
				execMigrationSourceMutation(t, db, `INSERT INTO schema_migration
  (version, checksum, applied_at) VALUES ('9999_unknown_review', ?, 1)`, strings.Repeat("a", 64))
				execMigrationSourceMutation(t, db, `INSERT INTO schema_migration_progress
  (version, checksum, statement_index, state, updated_at)
VALUES ('9998_unknown_progress', ?, 0, 'ready', 1)`, strings.Repeat("b", 64))
			},
			want: ErrMigrationSourceInvalid,
		},
		{
			name: "unknown progress version",
			mutate: func(t *testing.T, db *sql.DB, _ []MigrationVersion) {
				execMigrationSourceMutation(t, db, `INSERT INTO schema_migration_progress
  (version, checksum, statement_index, state, updated_at)
VALUES ('9998_unknown_progress', ?, 0, 'ready', 1)`, strings.Repeat("b", 64))
			},
			want: ErrMigrationSourceInvalid,
		},
		{
			name: "missing middle version",
			mutate: func(t *testing.T, db *sql.DB, repository []MigrationVersion) {
				execMigrationSourceMutation(t, db, `INSERT INTO schema_migration
  (version, checksum, applied_at) VALUES (?, ?, 1)`,
					repository[2].Version, repository[2].Checksum)
			},
			want: ErrMigrationSourceInvalid,
		},
		{
			name: "non canonical checksum",
			mutate: func(t *testing.T, db *sql.DB, repository []MigrationVersion) {
				execMigrationSourceMutation(t, db,
					"UPDATE schema_migration SET checksum = ? WHERE version = ?",
					strings.Repeat("A", 64), repository[0].Version)
			},
			want: ErrMigrationChecksumInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := openIsolatedMigrationSourceDatabase(t)
			prefixRunner := NewMigrationRunner(database.SQL)
			prefixRunner.FS = migrationRepositoryPrefixFS(t, 1)
			if err := prefixRunner.Run(context.Background()); err != nil {
				t.Fatalf("prepare first migration prefix: %v", err)
			}
			test.mutate(t, database.SQL, repository)
			before := readMigrationSourceGuard(t, database.SQL)
			runErr := NewMigrationRunner(database.SQL).Run(context.Background())
			if !errors.Is(runErr, test.want) {
				t.Fatalf("migration source gate error = %v, want %v", runErr, test.want)
			}
			after := readMigrationSourceGuard(t, database.SQL)
			if before != after {
				t.Fatalf("rejected migration changed database: before=%#v after=%#v", before, after)
			}
			if after.ScopeColumns != 0 || after.SeedSettings != 0 {
				t.Fatalf("rejected migration executed pending DDL or seeds: %#v", after)
			}
		})
	}
}

func TestValidateMigrationVersionPrefixRejectsMalformedSequences(t *testing.T) {
	repository := []MigrationVersion{
		{Version: "0001_first", Checksum: strings.Repeat("a", 64)},
		{Version: "0002_second", Checksum: strings.Repeat("b", 64)},
	}
	tests := []struct {
		name    string
		applied []MigrationVersion
		want    error
	}{
		{name: "empty version", applied: []MigrationVersion{{Checksum: strings.Repeat("a", 64)}}, want: ErrMigrationSourceInvalid},
		{name: "duplicate", applied: []MigrationVersion{repository[0], repository[0]}, want: ErrMigrationSourceInvalid},
		{name: "out of order", applied: []MigrationVersion{repository[1], repository[0]}, want: ErrMigrationSourceInvalid},
		{name: "unknown", applied: []MigrationVersion{repository[0], {Version: "9999_unknown", Checksum: strings.Repeat("c", 64)}}, want: ErrMigrationSourceInvalid},
		{name: "invalid checksum", applied: []MigrationVersion{{Version: repository[0].Version, Checksum: strings.Repeat("A", 64)}}, want: ErrMigrationChecksumInvalid},
		{name: "checksum mismatch", applied: []MigrationVersion{{Version: repository[0].Version, Checksum: strings.Repeat("c", 64)}}, want: ErrMigrationChecksumInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateMigrationVersionPrefix(repository, test.applied, false); !errors.Is(err, test.want) {
				t.Fatalf("ValidateMigrationVersionPrefix() error = %v, want %v", err, test.want)
			}
		})
	}
	if err := ValidateMigrationVersionPrefix(repository, repository[:1], false); err != nil {
		t.Fatalf("valid repository prefix: %v", err)
	}
	if err := ValidateMigrationVersionPrefix(repository, repository, true); err != nil {
		t.Fatalf("valid complete repository: %v", err)
	}
}

type migrationSourceGuard struct {
	Tables       int64
	Migrations   int64
	Progress     int64
	ScopeColumns int64
	SeedSettings int64
}

func readMigrationSourceGuard(t *testing.T, database *sql.DB) migrationSourceGuard {
	t.Helper()
	guard := migrationSourceGuard{}
	queries := []struct {
		query string
		value *int64
	}{
		{"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'", &guard.Tables},
		{"SELECT COUNT(*) FROM schema_migration", &guard.Migrations},
		{"SELECT COUNT(*) FROM schema_migration_progress", &guard.Progress},
		{"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'collection_run' AND column_name = 'scope'", &guard.ScopeColumns},
		{"SELECT COUNT(*) FROM platform_setting", &guard.SeedSettings},
	}
	for _, item := range queries {
		if err := database.QueryRow(item.query).Scan(item.value); err != nil {
			t.Fatalf("read migration source guard: %v", err)
		}
	}
	return guard
}

func execMigrationSourceMutation(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("mutate migration source fixture: %v", err)
	}
}

func openIsolatedMigrationSourceDatabase(t *testing.T) *Database {
	t.Helper()
	configuration, err := mysqldriver.ParseDSN(os.Getenv("TEST_DATABASE_DSN"))
	if err != nil {
		t.Fatalf("parse integration DSN: %v", err)
	}
	databaseName := fmt.Sprintf("nap_msrc_%d_%d", os.Getpid(), time.Now().UnixNano())
	adminConfiguration := *configuration
	if adminDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_ADMIN_DSN")); adminDSN != "" {
		parsedAdmin, parseErr := mysqldriver.ParseDSN(adminDSN)
		if parseErr != nil {
			t.Fatalf("parse integration admin DSN: %v", parseErr)
		}
		adminConfiguration = *parsedAdmin
	}
	adminConfiguration.DBName = ""
	admin, err := sql.Open("mysql", adminConfiguration.FormatDSN())
	if err != nil {
		t.Fatalf("open migration source admin connection: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("ping migration source admin connection: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create isolated migration source database: %v", err)
	}
	testUser := strings.ReplaceAll(configuration.User, "'", "''")
	grant := "GRANT ALL PRIVILEGES ON `" + databaseName + "`.* TO '" + testUser + "'@'%'"
	if _, err := admin.ExecContext(ctx, grant); err != nil {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE `"+databaseName+"`")
		_ = admin.Close()
		t.Fatalf("grant isolated migration source database: %v", err)
	}
	isolateConfiguration := *configuration
	isolateConfiguration.DBName = databaseName
	database, err := Open(ctx, Options{
		DSN: isolateConfiguration.FormatDSN(), MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute,
	})
	if err != nil {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE `"+databaseName+"`")
		_ = admin.Close()
		t.Fatalf("open isolated migration source database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupContext, "DROP DATABASE IF EXISTS `"+databaseName+"`"); err != nil {
			t.Errorf("drop isolated migration source database: %v", err)
		}
		_ = admin.Close()
	})
	return database
}

func migrationRepositoryPrefixFS(t *testing.T, count int) fstest.MapFS {
	t.Helper()
	paths, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatalf("list repository migrations: %v", err)
	}
	sort.Strings(paths)
	if count < 1 || count > len(paths) {
		t.Fatalf("migration repository prefix count = %d, available = %d", count, len(paths))
	}
	result := make(fstest.MapFS, count)
	for _, path := range paths[:count] {
		payload, err := fs.ReadFile(migrations.Files, path)
		if err != nil {
			t.Fatalf("read repository migration %s: %v", path, err)
		}
		result[path] = &fstest.MapFile{Data: payload}
	}
	return result
}

func migrationTestFSWithAdditional(t *testing.T, path, contents string) fstest.MapFS {
	t.Helper()
	paths, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatalf("list repository migrations: %v", err)
	}
	result := make(fstest.MapFS, len(paths)+1)
	for _, repositoryPath := range paths {
		payload, err := fs.ReadFile(migrations.Files, repositoryPath)
		if err != nil {
			t.Fatalf("read repository migration %s: %v", repositoryPath, err)
		}
		result[repositoryPath] = &fstest.MapFile{Data: payload}
	}
	if _, exists := result[path]; exists {
		t.Fatalf("additional migration duplicates repository path %s", path)
	}
	result[path] = &fstest.MapFile{Data: []byte(contents)}
	return result
}

func killMySQLConnection(ctx context.Context, db *sql.DB, connectionID int64) error {
	_, err := db.ExecContext(ctx, "KILL CONNECTION "+fmt.Sprintf("%d", connectionID))
	return err
}

func migrationFixtureVerifier(table string) func(context.Context, *sql.Conn, string, int) (bool, error) {
	return func(ctx context.Context, connection *sql.Conn, _ string, index int) (bool, error) {
		if index != 0 && index != 2 {
			return false, fmt.Errorf("no fixture postcondition for statement %d", index+1)
		}
		var dataType, nullable, previous string
		err := connection.QueryRowContext(ctx, `SELECT c.data_type, c.is_nullable,
       COALESCE((SELECT p.column_name FROM information_schema.columns p
                 WHERE p.table_schema = c.table_schema AND p.table_name = c.table_name
                   AND p.ordinal_position = c.ordinal_position - 1), '')
FROM information_schema.columns c
WHERE c.table_schema = DATABASE() AND c.table_name = ? AND c.column_name = 'scope'`, table).
			Scan(&dataType, &nullable, &previous)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if dataType != "json" || previous != "id" {
			return false, fmt.Errorf("fixture scope mismatch: type=%s previous=%s", dataType, previous)
		}
		return index == 0 || nullable == "NO", nil
	}
}

func assertRecoveredMigrationFixture(t *testing.T, ctx context.Context, db *sql.DB, version, table string) {
	t.Helper()
	var onlyMissing string
	if err := db.QueryRowContext(ctx, "SELECT JSON_UNQUOTE(JSON_EXTRACT(scope, '$.only_missing')) FROM "+table+" WHERE id = 1").
		Scan(&onlyMissing); err != nil || onlyMissing != "true" {
		t.Fatalf("recovered fixture value = %q, %v", onlyMissing, err)
	}
	var applied, progress int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration WHERE version = ?", version).Scan(&applied); err != nil {
		t.Fatalf("count applied migration: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_progress WHERE version = ?", version).Scan(&progress); err != nil {
		t.Fatalf("count migration progress: %v", err)
	}
	if applied != 1 || progress != 0 {
		t.Fatalf("migration records applied=%d progress=%d", applied, progress)
	}
}

func cleanupMigrationFixture(t *testing.T, ctx context.Context, db *sql.DB, version, table string) {
	t.Helper()
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ?", version)
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration WHERE version = ?", version)
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil && ctx.Err() == nil {
		t.Errorf("drop migration fixture %s: %v", table, err)
	}
}

func applyCollectionRunScopeContract(contracts map[string]tableContract) {
	contract := contracts["collection_run"]
	scope := columnContract{Name: "scope", ColumnType: "json", IsNullable: "NO"}
	columns := make([]columnContract, 0, len(contract.Columns)+1)
	for _, column := range contract.Columns {
		columns = append(columns, column)
		if column.Name == "end_timestamp" {
			columns = append(columns, scope)
		}
	}
	contract.Columns = columns
	contracts["collection_run"] = contract
}

func applyAlertReliabilityContract(contracts map[string]tableContract) {
	delivery := contracts["alert_delivery"]
	columns := make([]columnContract, 0, len(delivery.Columns)+3)
	for _, column := range delivery.Columns {
		columns = append(columns, column)
		if column.Name == "attempt_count" {
			columns = append(columns,
				columnContract{
					Name: "claim_token", ColumnType: "varchar(64)", IsNullable: "YES",
					CharacterSet: sql.NullString{String: "ascii", Valid: true},
					Collation:    sql.NullString{String: "ascii_bin", Valid: true},
				},
				columnContract{Name: "lease_expires_at", ColumnType: "bigint", IsNullable: "YES"},
				columnContract{Name: "payload_snapshot", ColumnType: "json", IsNullable: "NO"},
			)
		}
	}
	delivery.Columns = columns
	delivery.Indexes["idx_alert_delivery_claim"] = indexContract{
		Columns: []string{"status", "lease_expires_at", "next_retry_at", "id"},
	}
	contracts["alert_delivery"] = delivery
	contracts["alert_evaluation_cursor"] = tableContract{
		Columns: []columnContract{
			{
				Name: "active_key", ColumnType: "varchar(384)", IsNullable: "NO",
				CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
				Collation:    sql.NullString{String: "utf8mb4_bin", Valid: true},
			},
			{Name: "last_sample_at", ColumnType: "bigint", IsNullable: "NO"},
			{
				Name: "last_sample_key", ColumnType: "varchar(255)", IsNullable: "NO",
				CharacterSet: sql.NullString{String: "utf8mb4", Valid: true},
				Collation:    sql.NullString{String: "utf8mb4_bin", Valid: true},
			},
			{Name: "created_at", ColumnType: "bigint", IsNullable: "NO"},
			{Name: "updated_at", ColumnType: "bigint", IsNullable: "NO"},
		},
		Indexes:     map[string]indexContract{"PRIMARY": {Unique: true, Columns: []string{"active_key"}}},
		ForeignKeys: map[string]foreignKeyContract{},
		Engine:      "InnoDB", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci",
	}
}

func applyExportClaimLeaseContract(contracts map[string]tableContract) {
	exportJob := contracts["export_job"]
	columns := make([]columnContract, 0, len(exportJob.Columns)+2)
	for _, column := range exportJob.Columns {
		columns = append(columns, column)
		if column.Name == "heartbeat_at" {
			columns = append(columns,
				columnContract{
					Name: "claim_token", ColumnType: "varchar(64)", IsNullable: "YES",
					CharacterSet: sql.NullString{String: "ascii", Valid: true},
					Collation:    sql.NullString{String: "ascii_bin", Valid: true},
				},
				columnContract{Name: "lease_expires_at", ColumnType: "bigint", IsNullable: "YES"},
			)
		}
	}
	exportJob.Columns = columns
	exportJob.Indexes["idx_export_job_claim"] = indexContract{
		Columns: []string{"status", "lease_expires_at", "next_attempt_at", "id"},
	}
	contracts["export_job"] = exportJob
}

func applyEncryptionReencryptContract(t *testing.T, contracts map[string]tableContract) {
	t.Helper()
	payload, err := migrations.Files.ReadFile("0005_encryption_reencrypt.sql")
	if err != nil {
		t.Fatalf("read encryption re-encryption migration contract: %v", err)
	}
	statements, err := SplitSQLStatements(string(payload))
	if err != nil {
		t.Fatalf("parse encryption re-encryption migration contract: %v", err)
	}
	additional := parseInitialTableContracts(t, statements)
	for name, contract := range additional {
		if _, exists := contracts[name]; exists {
			t.Fatalf("encryption re-encryption migration duplicates table contract %s", name)
		}
		contracts[name] = contract
	}
}

func applyMigrationProgressContract(contracts map[string]tableContract) {
	contracts["schema_migration_progress"] = tableContract{
		Columns: []columnContract{
			{Name: "version", ColumnType: "varchar(64)", IsNullable: "NO", CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "checksum", ColumnType: "char(64)", IsNullable: "NO", CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "statement_index", ColumnType: "int", IsNullable: "NO", Default: sql.NullString{String: "0", Valid: true}},
			{Name: "state", ColumnType: "varchar(16)", IsNullable: "NO", Default: sql.NullString{String: "ready", Valid: true}, CharacterSet: sql.NullString{String: "utf8mb4", Valid: true}, Collation: sql.NullString{String: "utf8mb4_unicode_ci", Valid: true}},
			{Name: "updated_at", ColumnType: "bigint", IsNullable: "NO"},
		},
		Indexes:     map[string]indexContract{"PRIMARY": {Unique: true, Columns: []string{"version"}}},
		ForeignKeys: map[string]foreignKeyContract{},
		Engine:      "InnoDB", Charset: "utf8mb4", Collation: "utf8mb4_unicode_ci",
	}
}

func assertChecksumMismatchIsRejected(t *testing.T, ctx context.Context, db *sql.DB, runner *MigrationRunner) {
	t.Helper()
	for _, version := range []string{"0001_initial_schema", "0005_encryption_reencrypt", "0006_usage_fact_capacity_indexes"} {
		var checksum string
		if err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migration WHERE version = ?", version).Scan(&checksum); err != nil {
			t.Fatalf("read migration %s checksum: %v", version, err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE schema_migration SET checksum = ? WHERE version = ?", strings.Repeat("0", 64), version); err != nil {
			t.Fatalf("tamper migration %s checksum: %v", version, err)
		}
		runErr := runner.Run(ctx)
		if _, err := db.ExecContext(context.Background(), "UPDATE schema_migration SET checksum = ? WHERE version = ?", checksum, version); err != nil {
			t.Fatalf("restore migration %s checksum: %v", version, err)
		}
		if runErr == nil || !strings.Contains(runErr.Error(), "checksum mismatch") {
			t.Fatalf("tampered migration %s run error = %v", version, runErr)
		}
	}
}

const mysqlIntegrationLockName = "new-api-pilot-platform-user-integration"

type tableContract struct {
	Columns     []columnContract
	Indexes     map[string]indexContract
	ForeignKeys map[string]foreignKeyContract
	Engine      string
	Charset     string
	Collation   string
}

type columnContract struct {
	Name                 string
	ColumnType           string
	IsNullable           string
	Default              sql.NullString
	Extra                string
	GenerationExpression string
	CharacterSet         sql.NullString
	Collation            sql.NullString
}

type indexContract struct {
	Unique  bool
	Columns []string
}

type foreignKeyContract struct {
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	UpdateRule        string
	DeleteRule        string
}

var (
	createTableContractPattern = regexp.MustCompile(`(?s)^CREATE TABLE IF NOT EXISTS ([a-z0-9_]+) \((.*)\) ENGINE=([A-Za-z0-9]+) DEFAULT CHARSET=([a-z0-9_]+) COLLATE=([a-z0-9_]+)$`)
	columnNamePattern          = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	columnDefaultPattern       = regexp.MustCompile(`\bDEFAULT\s+('(?:''|[^'])*'|[^\s,]+)`)
	columnCharsetPattern       = regexp.MustCompile(`\bCHARACTER SET\s+([a-z0-9_]+)`)
	columnCollationPattern     = regexp.MustCompile(`\bCOLLATE\s+([a-z0-9_]+)`)
	primaryIndexPattern        = regexp.MustCompile(`(?s)PRIMARY KEY\s*\(([^)]+)\)`)
	uniqueIndexPattern         = regexp.MustCompile(`(?s)UNIQUE KEY\s+([a-z0-9_]+)\s*\(([^)]+)\)`)
	secondaryIndexPattern      = regexp.MustCompile(`(?m)^\s*KEY\s+([a-z0-9_]+)\s*\(([^)]+)\)`)
	foreignKeyPattern          = regexp.MustCompile(`(?s)CONSTRAINT\s+([a-z0-9_]+)\s+FOREIGN KEY\s*\(([^)]+)\)\s+REFERENCES\s+([a-z0-9_]+)\s*\(([^)]+)\)\s+ON UPDATE\s+(RESTRICT|CASCADE|SET NULL|NO ACTION)\s+ON DELETE\s+(RESTRICT|CASCADE|SET NULL|NO ACTION)`)
)

func parseInitialTableContracts(t *testing.T, statements []string) map[string]tableContract {
	t.Helper()
	contracts := make(map[string]tableContract, len(statements))
	for _, statement := range statements {
		matches := createTableContractPattern.FindStringSubmatch(strings.TrimSpace(statement))
		if len(matches) != 6 {
			t.Fatalf("cannot parse CREATE TABLE contract: %.120s", statement)
		}
		name := matches[1]
		body := matches[2]
		tableCharset := matches[4]
		tableCollation := matches[5]
		columns := make([]columnContract, 0)
		for _, line := range strings.Split(body, "\n") {
			if column, ok := parseColumnContract(line, tableCharset, tableCollation); ok {
				columns = append(columns, column)
			}
		}
		indexes := make(map[string]indexContract)
		if primary := primaryIndexPattern.FindStringSubmatch(body); len(primary) == 2 {
			indexes["PRIMARY"] = indexContract{Unique: true, Columns: parseContractColumns(primary[1])}
		}
		for _, index := range uniqueIndexPattern.FindAllStringSubmatch(body, -1) {
			indexes[index[1]] = indexContract{Unique: true, Columns: parseContractColumns(index[2])}
		}
		for _, index := range secondaryIndexPattern.FindAllStringSubmatch(body, -1) {
			indexes[index[1]] = indexContract{Unique: false, Columns: parseContractColumns(index[2])}
		}
		foreignKeys := make(map[string]foreignKeyContract)
		for _, key := range foreignKeyPattern.FindAllStringSubmatch(body, -1) {
			foreignKeys[key[1]] = foreignKeyContract{
				Columns:           parseContractColumns(key[2]),
				ReferencedTable:   key[3],
				ReferencedColumns: parseContractColumns(key[4]),
				UpdateRule:        key[5],
				DeleteRule:        key[6],
			}
		}
		for keyName, key := range foreignKeys {
			if !hasIndexPrefix(indexes, key.Columns) {
				indexes[keyName] = indexContract{Unique: false, Columns: append([]string(nil), key.Columns...)}
			}
		}
		if _, duplicate := contracts[name]; duplicate {
			t.Fatalf("duplicate CREATE TABLE contract for %s", name)
		}
		contracts[name] = tableContract{
			Columns: columns, Indexes: indexes, ForeignKeys: foreignKeys,
			Engine: matches[3], Charset: tableCharset, Collation: tableCollation,
		}
	}
	return contracts
}

func parseColumnContract(line, tableCharset, tableCollation string) (columnContract, bool) {
	definition := strings.TrimSuffix(strings.TrimSpace(line), ",")
	fields := strings.Fields(definition)
	if len(fields) < 2 {
		return columnContract{}, false
	}
	name := strings.Trim(fields[0], "`")
	if !columnNamePattern.MatchString(name) {
		return columnContract{}, false
	}
	columnType := strings.ToLower(fields[1])
	contract := columnContract{Name: name, ColumnType: columnType, IsNullable: "YES"}
	upperDefinition := strings.ToUpper(definition)
	if strings.Contains(upperDefinition, " NOT NULL") {
		contract.IsNullable = "NO"
	}
	if strings.Contains(upperDefinition, " AUTO_INCREMENT") {
		contract.Extra = "auto_increment"
	}
	if matched := columnDefaultPattern.FindStringSubmatch(definition); len(matched) == 2 {
		value := matched[1]
		if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		if !strings.EqualFold(value, "NULL") {
			contract.Default = sql.NullString{String: value, Valid: true}
		}
	}
	if isCharacterColumnType(columnType) {
		charset := tableCharset
		if matched := columnCharsetPattern.FindStringSubmatch(definition); len(matched) == 2 {
			charset = matched[1]
		}
		collation := tableCollation
		if matched := columnCollationPattern.FindStringSubmatch(definition); len(matched) == 2 {
			collation = matched[1]
		}
		contract.CharacterSet = sql.NullString{String: charset, Valid: true}
		contract.Collation = sql.NullString{String: collation, Valid: true}
	}
	return contract, true
}

func isCharacterColumnType(columnType string) bool {
	baseType := columnType
	if index := strings.IndexByte(baseType, '('); index >= 0 {
		baseType = baseType[:index]
	}
	switch baseType {
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		return true
	default:
		return false
	}
}

func hasIndexPrefix(indexes map[string]indexContract, columns []string) bool {
	for _, index := range indexes {
		if len(index.Columns) < len(columns) {
			continue
		}
		matches := true
		for position, column := range columns {
			if index.Columns[position] != column {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func parseContractColumns(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.Trim(strings.TrimSpace(part), "`"))
	}
	return result
}

func acquireMySQLIntegrationLock(t *testing.T, ctx context.Context, db *sql.DB) *sql.Conn {
	t.Helper()
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve MySQL integration lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", mysqlIntegrationLockName).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		t.Fatalf("acquire MySQL integration lock = %v, %v", acquired, err)
	}
	return connection
}

func assertSchemaContracts(t *testing.T, ctx context.Context, db *sql.DB, expected map[string]tableContract) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list migrated tables: %v", err)
	}
	actualTables := make([]string, 0, len(expected))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatalf("scan migrated table: %v", err)
		}
		actualTables = append(actualTables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close migrated table rows: %v", err)
	}
	expectedTables := make([]string, 0, len(expected))
	for name := range expected {
		expectedTables = append(expectedTables, name)
	}
	sort.Strings(expectedTables)
	if !reflect.DeepEqual(actualTables, expectedTables) {
		t.Fatalf("migrated tables = %v, want %v", actualTables, expectedTables)
	}

	for _, name := range expectedTables {
		actual := readLiveTableContract(t, ctx, db, name)
		contract := expected[name]
		if !reflect.DeepEqual(actual, contract) {
			t.Errorf("table %s contract mismatch\nactual: %#v\nwant:   %#v", name, actual, contract)
		}
	}
}

func readLiveTableContract(t *testing.T, ctx context.Context, db *sql.DB, table string) tableContract {
	t.Helper()
	var engine, charset, collation string
	if err := db.QueryRowContext(ctx, `SELECT t.engine, c.character_set_name, t.table_collation
FROM information_schema.tables t
JOIN information_schema.collation_character_set_applicability c
  ON c.collation_name = t.table_collation
WHERE t.table_schema = DATABASE() AND t.table_name = ?`, table).Scan(&engine, &charset, &collation); err != nil {
		t.Fatalf("read table %s engine/collation: %v", table, err)
	}
	columnRows, err := db.QueryContext(ctx, `SELECT column_name, column_type, is_nullable, column_default,
       extra, generation_expression, character_set_name, collation_name
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ?
ORDER BY ordinal_position`, table)
	if err != nil {
		t.Fatalf("read table %s columns: %v", table, err)
	}
	columns := make([]columnContract, 0)
	for columnRows.Next() {
		var column columnContract
		if err := columnRows.Scan(
			&column.Name, &column.ColumnType, &column.IsNullable, &column.Default,
			&column.Extra, &column.GenerationExpression, &column.CharacterSet, &column.Collation,
		); err != nil {
			_ = columnRows.Close()
			t.Fatalf("scan table %s column: %v", table, err)
		}
		columns = append(columns, column)
	}
	if err := columnRows.Close(); err != nil {
		t.Fatalf("close table %s column rows: %v", table, err)
	}

	indexes := make(map[string]indexContract)
	indexRows, err := db.QueryContext(ctx, `SELECT index_name, non_unique, column_name
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ?
ORDER BY index_name, seq_in_index`, table)
	if err != nil {
		t.Fatalf("read table %s indexes: %v", table, err)
	}
	for indexRows.Next() {
		var name, column string
		var nonUnique int
		if err := indexRows.Scan(&name, &nonUnique, &column); err != nil {
			_ = indexRows.Close()
			t.Fatalf("scan table %s index: %v", table, err)
		}
		contract := indexes[name]
		contract.Unique = nonUnique == 0
		contract.Columns = append(contract.Columns, column)
		indexes[name] = contract
	}
	if err := indexRows.Close(); err != nil {
		t.Fatalf("close table %s index rows: %v", table, err)
	}

	foreignKeys := make(map[string]foreignKeyContract)
	foreignKeyRows, err := db.QueryContext(ctx, `SELECT k.constraint_name, k.column_name, k.referenced_table_name,
       k.referenced_column_name, r.update_rule, r.delete_rule
FROM information_schema.key_column_usage k
JOIN information_schema.referential_constraints r
  ON r.constraint_schema = k.constraint_schema
 AND r.table_name = k.table_name
 AND r.constraint_name = k.constraint_name
WHERE k.table_schema = DATABASE()
  AND k.table_name = ?
  AND k.referenced_table_name IS NOT NULL
ORDER BY k.constraint_name, k.ordinal_position`, table)
	if err != nil {
		t.Fatalf("read table %s foreign keys: %v", table, err)
	}
	for foreignKeyRows.Next() {
		var name, column, referencedTable, referencedColumn, updateRule, deleteRule string
		if err := foreignKeyRows.Scan(&name, &column, &referencedTable, &referencedColumn, &updateRule, &deleteRule); err != nil {
			_ = foreignKeyRows.Close()
			t.Fatalf("scan table %s foreign key: %v", table, err)
		}
		contract := foreignKeys[name]
		contract.Columns = append(contract.Columns, column)
		contract.ReferencedTable = referencedTable
		contract.ReferencedColumns = append(contract.ReferencedColumns, referencedColumn)
		contract.UpdateRule = updateRule
		contract.DeleteRule = deleteRule
		foreignKeys[name] = contract
	}
	if err := foreignKeyRows.Close(); err != nil {
		t.Fatalf("close table %s foreign key rows: %v", table, err)
	}
	return tableContract{
		Columns: columns, Indexes: indexes, ForeignKeys: foreignKeys,
		Engine: engine, Charset: charset, Collation: collation,
	}
}

func assertExactDefaultSeeds(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if len(defaultSettings) != 26 {
		t.Fatalf("default setting definition count = %d, want 26", len(defaultSettings))
	}
	if len(defaultAlertRules) != 25 {
		t.Fatalf("default alert rule definition count = %d, want 25", len(defaultAlertRules))
	}
	seenSettings := make(map[string]struct{}, len(defaultSettings))
	for _, setting := range defaultSettings {
		if _, duplicate := seenSettings[setting.Key]; duplicate {
			t.Fatalf("duplicate default setting definition %s", setting.Key)
		}
		seenSettings[setting.Key] = struct{}{}
		var matches int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*)
FROM platform_setting
WHERE setting_key = ? AND setting_value = ? AND value_type = ? AND is_secret = ?`,
			setting.Key, setting.Value, setting.Type, setting.Secret,
		).Scan(&matches); err != nil {
			t.Fatalf("verify default setting %s: %v", setting.Key, err)
		}
		if matches != 1 {
			t.Errorf("default setting %s exact match count = %d", setting.Key, matches)
		}
	}

	seenRules := make(map[string]struct{}, len(defaultAlertRules))
	for _, rule := range defaultAlertRules {
		identity := rule.Key + ":" + rule.Level
		if _, duplicate := seenRules[identity]; duplicate {
			t.Fatalf("duplicate default alert rule definition %s", identity)
		}
		seenRules[identity] = struct{}{}
		var matches int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*)
FROM alert_rule
WHERE rule_key = ? AND name = ? AND enabled = 1 AND level = ? AND metric = ?
  AND compare_operator = ? AND threshold_value = CAST(? AS DECIMAL(30,10))
  AND for_times = ? AND scope_type = 'global' AND scope_id = 0`,
			rule.Key, rule.Name, rule.Level, rule.Metric, rule.Operator, rule.Threshold, rule.ForTimes,
		).Scan(&matches); err != nil {
			t.Fatalf("verify default alert rule %s: %v", identity, err)
		}
		if matches != 1 {
			t.Errorf("default alert rule %s exact match count = %d", identity, matches)
		}
	}
}

var expectedTables = []string{
	"schema_migration", "platform_user", "site", "site_channel", "site_monitoring_pause",
	"site_capability", "customer", "account", "collection_cursor", "collection_run",
	"collection_window", "collection_run_window", "aggregation_bucket_lock", "usage_fact_hourly",
	"usage_fact_daily", "account_stat_hourly", "account_stat_daily", "customer_stat_hourly",
	"customer_stat_daily", "site_stat_hourly", "site_stat_daily", "global_stat_hourly",
	"global_stat_daily", "model_stat_hourly", "model_stat_daily", "channel_stat_hourly",
	"channel_stat_daily", "site_instance", "site_instance_status_minutely",
	"site_instance_status_hourly", "site_instance_status_daily", "site_status_minutely",
	"site_status_hourly", "site_status_daily", "alert_rule", "alert_event", "alert_delivery",
	"export_job", "platform_setting",
}
