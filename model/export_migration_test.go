package model

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestMySQLExportClaimLeaseMigrationRecoversEveryCommitGap(t *testing.T) {
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
	priorChecksums := readMigrationChecksums(t, ctx, database.SQL, []string{"0001_initial_schema"})

	faults := []struct {
		name         string
		afterIndex   int
		beforeRecord bool
	}{
		{name: "after_claim_token_alter", afterIndex: 0},
		{name: "after_lease_alter", afterIndex: 1},
		{name: "after_claim_index_alter", afterIndex: 2},
		{name: "before_final_record", afterIndex: -1, beforeRecord: true},
	}
	for caseIndex, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			version := fmt.Sprintf("992%d_export_claim_%s", caseIndex, fault.name)
			table := fmt.Sprintf("export_claim_recovery_fixture_%d", caseIndex)
			cleanupExportClaimMigrationFixture(t, ctx, database.SQL, version, table)
			t.Cleanup(func() {
				cleanupExportClaimMigrationFixture(t, context.Background(), database.SQL, version, table)
			})
			if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
  id BIGINT NOT NULL AUTO_INCREMENT,
  status VARCHAR(16) NOT NULL,
  next_attempt_at BIGINT NOT NULL,
  heartbeat_at BIGINT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, table)); err != nil {
				t.Fatalf("create export claim fixture: %v", err)
			}
			migration := exportClaimFixtureMigration(table)
			migrationFS := migrationTestFSWithAdditional(t, version+".sql", migration)
			runner := &MigrationRunner{
				DB: database.SQL, FS: migrationFS,
				Now:                    func() time.Time { return time.Unix(1_752_400_800, 0) },
				verifyDDLPostcondition: exportClaimFixtureVerifier(table),
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
				t.Fatal("fault-injected export claim migration unexpectedly succeeded")
			}

			recovery := &MigrationRunner{
				DB: database.SQL, FS: migrationFS, Now: runner.Now,
				verifyDDLPostcondition: exportClaimFixtureVerifier(table),
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("recover export claim migration after %s: %v", fault.name, err)
			}
			if err := recovery.Run(ctx); err != nil {
				t.Fatalf("idempotent export claim migration after %s: %v", fault.name, err)
			}
			assertRecoveredExportClaimMigration(t, ctx, database.SQL, version, table)
		})
	}
	afterChecksums := readMigrationChecksums(t, ctx, database.SQL, []string{"0001_initial_schema"})
	if !reflect.DeepEqual(afterChecksums, priorChecksums) {
		t.Fatalf("prior migration checksums changed: before=%v after=%v", priorChecksums, afterChecksums)
	}
}

func exportClaimFixtureMigration(table string) string {
	return fmt.Sprintf(`ALTER TABLE %s
  ADD COLUMN claim_token VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER heartbeat_at;
ALTER TABLE %s
  ADD COLUMN lease_expires_at BIGINT NULL AFTER claim_token;
ALTER TABLE %s
  ADD KEY idx_export_job_claim (status, lease_expires_at, next_attempt_at, id);`, table, table, table)
}

func exportClaimFixtureVerifier(table string) func(context.Context, *sql.Conn, string, int) (bool, error) {
	return func(ctx context.Context, connection *sql.Conn, _ string, stage int) (bool, error) {
		if stage < 0 || stage > 2 {
			return false, fmt.Errorf("no export claim fixture postcondition for statement %d", stage+1)
		}
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
WHERE table_schema = DATABASE() AND table_name = ?
  AND column_name IN ('claim_token', 'lease_expires_at')`, table)
		if err != nil {
			return false, err
		}
		defer func() { _ = rows.Close() }()
		states := map[string]columnState{}
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
			return false, fmt.Errorf("claim token schema mismatch: %#v", claim)
		}
		if stage == 0 {
			return true, nil
		}
		lease, exists := states["lease_expires_at"]
		if !exists {
			return false, nil
		}
		if lease.ColumnType != "bigint" || lease.Nullable != "YES" || lease.Charset.Valid || lease.Collation.Valid {
			return false, fmt.Errorf("lease schema mismatch: %#v", lease)
		}
		if stage == 1 {
			return true, nil
		}
		indexRows, err := connection.QueryContext(ctx, `SELECT seq_in_index, column_name, non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = 'idx_export_job_claim'
ORDER BY seq_in_index`, table)
		if err != nil {
			return false, err
		}
		defer func() { _ = indexRows.Close() }()
		columns := []string{}
		for indexRows.Next() {
			var sequence, nonUnique int
			var column string
			if err := indexRows.Scan(&sequence, &column, &nonUnique); err != nil {
				return false, err
			}
			if sequence != len(columns)+1 || nonUnique != 1 {
				return false, fmt.Errorf("claim index metadata sequence=%d non_unique=%d", sequence, nonUnique)
			}
			columns = append(columns, column)
		}
		if err := indexRows.Err(); err != nil {
			return false, err
		}
		return reflect.DeepEqual(columns, []string{"status", "lease_expires_at", "next_attempt_at", "id"}), nil
	}
}

func assertRecoveredExportClaimMigration(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	version string,
	table string,
) {
	t.Helper()
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve export migration verification connection: %v", err)
	}
	defer func() { _ = connection.Close() }()
	verified, err := exportClaimFixtureVerifier(table)(ctx, connection, version, 2)
	if err != nil || !verified {
		t.Fatalf("export claim fixture schema verified=%t err=%v", verified, err)
	}
	var applied, progress int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration WHERE version = ?", version).Scan(&applied); err != nil {
		t.Fatalf("count export claim migration: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_progress WHERE version = ?", version).Scan(&progress); err != nil {
		t.Fatalf("count export claim migration checkpoint: %v", err)
	}
	if applied != 1 || progress != 0 {
		t.Fatalf("export claim migration records applied=%d progress=%d", applied, progress)
	}
}

func cleanupExportClaimMigrationFixture(t *testing.T, ctx context.Context, db *sql.DB, version, table string) {
	t.Helper()
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration_progress WHERE version = ?", version)
	_, _ = db.ExecContext(ctx, "DELETE FROM schema_migration WHERE version = ?", version)
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil && ctx.Err() == nil {
		t.Errorf("drop export claim migration fixture %s: %v", table, err)
	}
}

func readMigrationChecksums(t *testing.T, ctx context.Context, db *sql.DB, versions []string) map[string]string {
	t.Helper()
	result := make(map[string]string, len(versions))
	for _, version := range versions {
		var checksum string
		if err := db.QueryRowContext(ctx, "SELECT checksum FROM schema_migration WHERE version = ?", version).Scan(&checksum); err != nil {
			t.Fatalf("read migration %s checksum: %v", version, err)
		}
		result[version] = checksum
	}
	return result
}
