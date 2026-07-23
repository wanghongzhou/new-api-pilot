package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"

	"new-api-pilot/migrations"
	"new-api-pilot/model"
)

const (
	a25AcceptanceID = "A25"
	a25TargetTest   = "TestA25MigrationAcceptance"
)

type a25Fixture struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Clock         struct {
		NowUnix int64 `yaml:"now_unix"`
	} `yaml:"clock"`
	MySQL struct {
		Version                     string `yaml:"version"`
		TransactionIsolation        string `yaml:"transaction_isolation"`
		Charset                     string `yaml:"charset"`
		Collation                   string `yaml:"collation"`
		MigrationChecksumTamperFail bool   `yaml:"migration_checksum_tamper_must_fail"`
	} `yaml:"mysql"`
}

type a25MigrationEvidence struct {
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

type a25VersionGateEvidence struct {
	CurrentVersion          string `json:"current_version"`
	LegacyMySQLVersion      string `json:"legacy_mysql_version"`
	MariaDBVersion          string `json:"mariadb_version"`
	CurrentAccepted         bool   `json:"current_accepted"`
	LegacyMySQLRejected     bool   `json:"legacy_mysql_rejected"`
	MariaDBRejected         bool   `json:"mariadb_rejected"`
	LegacyTablesBefore      int64  `json:"legacy_tables_before"`
	LegacyTablesAfter       int64  `json:"legacy_tables_after"`
	MariaDBTablesBefore     int64  `json:"mariadb_tables_before"`
	MariaDBTablesAfter      int64  `json:"mariadb_tables_after"`
	LegacyLockAbsentBefore  bool   `json:"legacy_lock_absent_before"`
	LegacyLockAbsentAfter   bool   `json:"legacy_lock_absent_after"`
	MariaDBLockAbsentBefore bool   `json:"mariadb_lock_absent_before"`
	MariaDBLockAbsentAfter  bool   `json:"mariadb_lock_absent_after"`
}

type a25EmptyEvidence struct {
	TablesBefore           int64  `json:"tables_before"`
	MigrationCount         int    `json:"migration_count"`
	ProgressRows           int64  `json:"progress_rows"`
	SchemaSHA256           string `json:"schema_sha256"`
	AppliedAtStable        bool   `json:"applied_at_stable"`
	IdempotentSchemaStable bool   `json:"idempotent_schema_stable"`
}

type a25UpgradeEvidence struct {
	PrefixMigrationCount  int    `json:"prefix_migration_count"`
	HistoricalRows        int64  `json:"historical_rows"`
	HistoricalSHA256      string `json:"historical_sha256"`
	HistoricalPreserved   bool   `json:"historical_preserved"`
	ForeignKeysPreserved  bool   `json:"foreign_keys_preserved"`
	BackfillScopeMigrated bool   `json:"backfill_scope_migrated"`
	SchemaSHA256          string `json:"schema_sha256"`
	MatchesAuthoritative  bool   `json:"matches_authoritative"`
}

type a25TamperEvidence struct {
	DatabaseChecksumRejected bool  `json:"database_checksum_rejected"`
	RepositorySourceRejected bool  `json:"repository_source_rejected"`
	UnknownVersionRejected   bool  `json:"unknown_version_rejected"`
	NoSchemaMutation         bool  `json:"no_schema_mutation"`
	ProgressRows             int64 `json:"progress_rows"`
}

type a25DMLFailureEvidence struct {
	InitialFailureObserved bool  `json:"initial_failure_observed"`
	CheckpointReady        bool  `json:"checkpoint_ready"`
	CheckpointIndex        int   `json:"checkpoint_index"`
	ResumeCompleted        bool  `json:"resume_completed"`
	IdempotentRowCount     int64 `json:"idempotent_row_count"`
	ProgressRows           int64 `json:"progress_rows"`
}

type a25DDLRecoveryCase struct {
	DirtyWithoutDDLReplayed bool   `json:"dirty_without_ddl_replayed"`
	DirtyWithDDLRecognized  bool   `json:"dirty_with_ddl_recognized"`
	ReplaySchemaSHA256      string `json:"replay_schema_sha256"`
	CommittedSchemaSHA256   string `json:"committed_schema_sha256"`
	ProgressRows            int64  `json:"progress_rows"`
}

type a25Report struct {
	SchemaVersion             int                    `json:"schema_version"`
	AcceptanceID              string                 `json:"acceptance_id"`
	Status                    string                 `json:"status"`
	FixturePath               string                 `json:"fixture_path"`
	FixtureSHA256             string                 `json:"fixture_sha256"`
	FixedNowUnix              int64                  `json:"fixed_now_unix"`
	RepositoryMigrations      []a25MigrationEvidence `json:"repository_migrations"`
	AuthoritativeSchemaSHA256 string                 `json:"authoritative_schema_sha256"`
	VersionGate               a25VersionGateEvidence `json:"version_gate"`
	EmptyDatabase             a25EmptyEvidence       `json:"empty_database"`
	Upgrade                   a25UpgradeEvidence     `json:"upgrade"`
	Tamper                    a25TamperEvidence      `json:"tamper"`
	DMLFailure                a25DMLFailureEvidence  `json:"dml_failure"`
	DDLRecovery               a25DDLRecoveryCase     `json:"ddl_recovery"`
}

type a25NegativeServerProbe struct {
	Version         string
	Rejected        bool
	TablesBefore    int64
	TablesAfter     int64
	LockBeforeEmpty bool
	LockAfterEmpty  bool
}

func TestA25MigrationAcceptance(t *testing.T) {
	acceptance := strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) == a25AcceptanceID
	currentDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	legacyDSN := strings.TrimSpace(os.Getenv("A25_LEGACY_DATABASE_DSN"))
	mariaDBDSN := strings.TrimSpace(os.Getenv("A25_MARIADB_DATABASE_DSN"))
	if currentDSN == "" || legacyDSN == "" || mariaDBDSN == "" {
		if acceptance {
			t.Fatal("A25 requires isolated current, MySQL 5.7, and MariaDB databases")
		}
		t.Skip("A25 isolated database DSNs are not configured")
	}
	if os.Getenv("A25_ISOLATED_MYSQL") != "true" {
		if acceptance {
			t.Fatal("A25_ISOLATED_MYSQL=true is required")
		}
		t.Skip("A25 requires isolated database servers")
	}
	evidenceDirectory := strings.TrimSpace(os.Getenv("ACCEPTANCE_EVIDENCE_DIR"))
	if acceptance {
		assertA25EvidenceDirectory(t, evidenceDirectory)
	}
	fixture, fixturePath, fixtureSHA := loadA25Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	admin := openA25Database(t, ctx, currentDSN)
	currentVersion := readA25ServerVersion(t, ctx, admin.SQL)
	if !strings.HasPrefix(currentVersion, fixture.MySQL.Version+".") {
		t.Fatalf("A25 current MySQL version=%q want %s.x", currentVersion, fixture.MySQL.Version)
	}
	if err := model.ValidateMySQLVersion(currentVersion); err != nil {
		t.Fatalf("A25 current MySQL version rejected: %v", err)
	}
	legacyProbe := probeA25UnsupportedServer(t, ctx, legacyDSN, false)
	mariaProbe := probeA25UnsupportedServer(t, ctx, mariaDBDSN, true)

	repository, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil || len(repository) != 1 {
		t.Fatalf("load A25 repository migrations count=%d err=%v", len(repository), err)
	}
	now := time.Unix(fixture.Clock.NowUnix, 0)

	empty := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_empty")
	emptyTablesBefore := countA25Tables(t, ctx, empty.SQL)
	if emptyTablesBefore != 0 {
		t.Fatalf("A25 empty scenario started with %d tables", emptyTablesBefore)
	}
	runA25Migrations(t, ctx, empty.SQL, migrations.Files, now)
	assertA25CurrentMigrationState(t, ctx, empty.SQL, repository)
	authoritativeSchema := hashA25Schema(t, ctx, empty.SQL)
	appliedBefore := readA25AppliedTimes(t, ctx, empty.SQL)
	runA25Migrations(t, ctx, empty.SQL, migrations.Files, now.Add(time.Hour))
	appliedAfter := readA25AppliedTimes(t, ctx, empty.SQL)
	emptySchemaAfter := hashA25Schema(t, ctx, empty.SQL)

	upgrade := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_upgrade")
	runA25Migrations(t, ctx, upgrade.SQL, migrations.Files, now)
	historicalRows, historicalSHA := seedA25HistoricalRows(t, ctx, upgrade.SQL, fixture.Clock.NowUnix)
	runA25Migrations(t, ctx, upgrade.SQL, migrations.Files, now.Add(time.Minute))
	assertA25CurrentMigrationState(t, ctx, upgrade.SQL, repository)
	upgradeHistoricalRows, upgradeHistoricalSHA, foreignKeysPreserved, scopeMigrated := verifyA25HistoricalRows(t, ctx, upgrade.SQL)
	upgradeSchema := hashA25Schema(t, ctx, upgrade.SQL)

	tamper := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_tamper")
	runA25Migrations(t, ctx, tamper.SQL, migrations.Files, now)
	tamperSchemaBefore := hashA25Schema(t, ctx, tamper.SQL)
	badChecksum := strings.Repeat("f", 64)
	if badChecksum == repository[0].Checksum {
		badChecksum = strings.Repeat("e", 64)
	}
	execA25(t, ctx, tamper.SQL, "UPDATE schema_migration SET checksum = ? WHERE version = ?", badChecksum, repository[0].Version)
	databaseChecksumRejected := runA25MigrationsExpect(t, ctx, tamper.SQL, migrations.Files, now, model.ErrMigrationChecksumInvalid)
	execA25(t, ctx, tamper.SQL, "UPDATE schema_migration SET checksum = ? WHERE version = ?", repository[0].Checksum, repository[0].Version)
	repositorySourceRejected := runA25MigrationsExpect(t, ctx, tamper.SQL, tamperedA25SourceFS(t, repository, repository[0].Version), now,
		model.ErrMigrationChecksumInvalid)
	execA25(t, ctx, tamper.SQL, `INSERT INTO schema_migration (version, checksum, applied_at) VALUES (?, ?, ?)`,
		"9999_unknown_acceptance", strings.Repeat("a", 64), fixture.Clock.NowUnix)
	unknownVersionRejected := runA25MigrationsExpect(t, ctx, tamper.SQL, migrations.Files, now, model.ErrMigrationSourceInvalid)
	execA25(t, ctx, tamper.SQL, "DELETE FROM schema_migration WHERE version = ?", "9999_unknown_acceptance")
	tamperSchemaAfter := hashA25Schema(t, ctx, tamper.SQL)
	tamperProgress := countA25Rows(t, ctx, tamper.SQL, "schema_migration_progress")

	dmlFailure := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_dml_failure")
	dmlSource, dmlVersion := a25DMLFailureFS(t, repository)
	dmlRunner := model.NewMigrationRunner(dmlFailure.SQL)
	dmlRunner.FS = dmlSource
	dmlRunner.Now = func() time.Time { return now }
	dmlErr := dmlRunner.Run(ctx)
	if dmlErr == nil {
		t.Fatal("A25 injected DML failure unexpectedly succeeded")
	}
	dmlCheckpointIndex, dmlCheckpointState := readA25Progress(t, ctx, dmlFailure.SQL, dmlVersion)
	execA25(t, ctx, dmlFailure.SQL, `CREATE TABLE a25_failure_target (
id BIGINT NOT NULL PRIMARY KEY, marker VARCHAR(32) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	runA25Migrations(t, ctx, dmlFailure.SQL, dmlSource, now.Add(time.Minute))
	runA25Migrations(t, ctx, dmlFailure.SQL, dmlSource, now.Add(2*time.Minute))
	dmlRows := countA25Rows(t, ctx, dmlFailure.SQL, "a25_failure_target")
	dmlProgress := countA25Rows(t, ctx, dmlFailure.SQL, "schema_migration_progress")

	dirtyReplay := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_dirty_replay")
	prepareA25InitialDirtyCheckpoint(t, ctx, dirtyReplay.SQL, repository[0], 2, false, fixture.Clock.NowUnix)
	runA25Migrations(t, ctx, dirtyReplay.SQL, migrations.Files, now.Add(time.Minute))
	assertA25CurrentMigrationState(t, ctx, dirtyReplay.SQL, repository)
	dirtyReplaySchema := hashA25Schema(t, ctx, dirtyReplay.SQL)

	dirtyCommitted := createA25ScenarioDatabase(t, ctx, admin.SQL, currentDSN, "pilot_a25_dirty_committed")
	prepareA25InitialDirtyCheckpoint(t, ctx, dirtyCommitted.SQL, repository[0], 2, true, fixture.Clock.NowUnix)
	runA25Migrations(t, ctx, dirtyCommitted.SQL, migrations.Files, now.Add(time.Minute))
	assertA25CurrentMigrationState(t, ctx, dirtyCommitted.SQL, repository)
	dirtyCommittedSchema := hashA25Schema(t, ctx, dirtyCommitted.SQL)
	dirtyProgress := countA25Rows(t, ctx, dirtyCommitted.SQL, "schema_migration_progress") +
		countA25Rows(t, ctx, dirtyReplay.SQL, "schema_migration_progress")

	if authoritativeSchema != emptySchemaAfter || authoritativeSchema != upgradeSchema ||
		authoritativeSchema != dirtyReplaySchema || authoritativeSchema != dirtyCommittedSchema {
		t.Fatalf("A25 schema hashes differ authoritative=%s empty=%s upgrade=%s replay=%s committed=%s",
			authoritativeSchema, emptySchemaAfter, upgradeSchema, dirtyReplaySchema, dirtyCommittedSchema)
	}
	if historicalRows != upgradeHistoricalRows || historicalSHA != upgradeHistoricalSHA ||
		!foreignKeysPreserved || !scopeMigrated {
		t.Fatalf("A25 historical upgrade rows=%d/%d sha=%s/%s fk=%t scope=%t",
			historicalRows, upgradeHistoricalRows, historicalSHA, upgradeHistoricalSHA, foreignKeysPreserved, scopeMigrated)
	}
	if !legacyProbe.Rejected || !mariaProbe.Rejected || legacyProbe.TablesBefore != 0 || legacyProbe.TablesAfter != 0 ||
		mariaProbe.TablesBefore != 0 || mariaProbe.TablesAfter != 0 || !legacyProbe.LockBeforeEmpty ||
		!legacyProbe.LockAfterEmpty || !mariaProbe.LockBeforeEmpty || !mariaProbe.LockAfterEmpty {
		t.Fatalf("A25 unsupported server probes legacy=%#v maria=%#v", legacyProbe, mariaProbe)
	}

	reportMigrations := make([]a25MigrationEvidence, 0, len(repository))
	for _, migration := range repository {
		reportMigrations = append(reportMigrations, a25MigrationEvidence{Version: migration.Version, Checksum: migration.Checksum})
	}
	report := a25Report{
		SchemaVersion: 1, AcceptanceID: a25AcceptanceID, Status: "passed",
		FixturePath: fixturePath, FixtureSHA256: fixtureSHA, FixedNowUnix: fixture.Clock.NowUnix,
		RepositoryMigrations: reportMigrations, AuthoritativeSchemaSHA256: authoritativeSchema,
		VersionGate: a25VersionGateEvidence{
			CurrentVersion: currentVersion, LegacyMySQLVersion: legacyProbe.Version, MariaDBVersion: mariaProbe.Version,
			CurrentAccepted: true, LegacyMySQLRejected: legacyProbe.Rejected, MariaDBRejected: mariaProbe.Rejected,
			LegacyTablesBefore: legacyProbe.TablesBefore, LegacyTablesAfter: legacyProbe.TablesAfter,
			MariaDBTablesBefore: mariaProbe.TablesBefore, MariaDBTablesAfter: mariaProbe.TablesAfter,
			LegacyLockAbsentBefore: legacyProbe.LockBeforeEmpty, LegacyLockAbsentAfter: legacyProbe.LockAfterEmpty,
			MariaDBLockAbsentBefore: mariaProbe.LockBeforeEmpty, MariaDBLockAbsentAfter: mariaProbe.LockAfterEmpty,
		},
		EmptyDatabase: a25EmptyEvidence{
			TablesBefore: emptyTablesBefore, MigrationCount: len(repository),
			ProgressRows: countA25Rows(t, ctx, empty.SQL, "schema_migration_progress"), SchemaSHA256: authoritativeSchema,
			AppliedAtStable:        equalA25AppliedTimes(appliedBefore, appliedAfter),
			IdempotentSchemaStable: authoritativeSchema == emptySchemaAfter,
		},
		Upgrade: a25UpgradeEvidence{
			PrefixMigrationCount: 1, HistoricalRows: upgradeHistoricalRows, HistoricalSHA256: upgradeHistoricalSHA,
			HistoricalPreserved:  historicalRows == upgradeHistoricalRows && historicalSHA == upgradeHistoricalSHA,
			ForeignKeysPreserved: foreignKeysPreserved, BackfillScopeMigrated: scopeMigrated,
			SchemaSHA256: upgradeSchema, MatchesAuthoritative: upgradeSchema == authoritativeSchema,
		},
		Tamper: a25TamperEvidence{
			DatabaseChecksumRejected: databaseChecksumRejected, RepositorySourceRejected: repositorySourceRejected,
			UnknownVersionRejected: unknownVersionRejected, NoSchemaMutation: tamperSchemaBefore == tamperSchemaAfter,
			ProgressRows: tamperProgress,
		},
		DMLFailure: a25DMLFailureEvidence{
			InitialFailureObserved: dmlErr != nil, CheckpointReady: dmlCheckpointState == "ready",
			CheckpointIndex: dmlCheckpointIndex, ResumeCompleted: dmlRows == 1 && dmlProgress == 0,
			IdempotentRowCount: dmlRows, ProgressRows: dmlProgress,
		},
		DDLRecovery: a25DDLRecoveryCase{
			DirtyWithoutDDLReplayed: dirtyReplaySchema == authoritativeSchema,
			DirtyWithDDLRecognized:  dirtyCommittedSchema == authoritativeSchema,
			ReplaySchemaSHA256:      dirtyReplaySchema, CommittedSchemaSHA256: dirtyCommittedSchema,
			ProgressRows: dirtyProgress,
		},
	}
	if acceptance {
		writeA25Report(t, evidenceDirectory, report)
	}
}

func loadA25Fixture(t *testing.T) (a25Fixture, string, string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read A25 fixture: %v", err)
	}
	var fixture a25Fixture
	if err := yaml.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode A25 fixture: %v", err)
	}
	if fixture.SchemaVersion != 2 || fixture.FixtureID != "F05" || fixture.Clock.NowUnix <= 0 ||
		fixture.MySQL.Version != "8.4" || fixture.MySQL.TransactionIsolation != "READ-COMMITTED" ||
		fixture.MySQL.Charset != "utf8mb4" || fixture.MySQL.Collation != "utf8mb4_unicode_ci" ||
		!fixture.MySQL.MigrationChecksumTamperFail {
		t.Fatalf("unexpected A25 fixture: %#v", fixture)
	}
	digest := sha256.Sum256(payload)
	return fixture, "testdata/design/f05-ops-capacity.yaml", hex.EncodeToString(digest[:])
}

func openA25Database(t *testing.T, ctx context.Context, dsn string) *model.Database {
	t.Helper()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 1, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open A25 database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func createA25ScenarioDatabase(t *testing.T, ctx context.Context, admin *sql.DB, baseDSN, name string) *model.Database {
	t.Helper()
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			t.Fatalf("unsafe A25 database name %q", name)
		}
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create A25 database %s: %v", name, err)
	}
	configuration, err := mysqldriver.ParseDSN(baseDSN)
	if err != nil {
		t.Fatalf("parse A25 DSN: %v", err)
	}
	configuration.DBName = name
	return openA25Database(t, ctx, configuration.FormatDSN())
}

func runA25Migrations(t *testing.T, ctx context.Context, database *sql.DB, source fs.FS, now time.Time) {
	t.Helper()
	runner := model.NewMigrationRunner(database)
	runner.FS = source
	runner.Now = func() time.Time { return now }
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("run A25 migrations: %v", err)
	}
}

func runA25MigrationsExpect(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	source fs.FS,
	now time.Time,
	want error,
) bool {
	t.Helper()
	runner := model.NewMigrationRunner(database)
	runner.FS = source
	runner.Now = func() time.Time { return now }
	err := runner.Run(ctx)
	if !errors.Is(err, want) {
		t.Fatalf("A25 migration error=%v want=%v", err, want)
	}
	return true
}

func tamperedA25SourceFS(t *testing.T, repository []model.MigrationVersion, version string) fs.FS {
	t.Helper()
	result := fstest.MapFS{}
	for _, migration := range repository {
		path := migration.Version + ".sql"
		payload, err := fs.ReadFile(migrations.Files, path)
		if err != nil {
			t.Fatalf("read A25 migration %s: %v", path, err)
		}
		if migration.Version == version {
			payload = append(append([]byte(nil), payload...), []byte("\n-- A25 source tamper\n")...)
		}
		result[path] = &fstest.MapFile{Data: payload, Mode: 0o444}
	}
	return result
}

func a25DMLFailureFS(t *testing.T, repository []model.MigrationVersion) (fs.FS, string) {
	t.Helper()
	result := fstest.MapFS{}
	for _, migration := range repository {
		path := migration.Version + ".sql"
		payload, err := fs.ReadFile(migrations.Files, path)
		if err != nil {
			t.Fatalf("read A25 migration %s: %v", path, err)
		}
		result[path] = &fstest.MapFile{Data: payload, Mode: 0o444}
	}
	const version = "0007_a25_transactional_failure"
	result[version+".sql"] = &fstest.MapFile{
		Data: []byte("INSERT INTO a25_failure_target (id, marker) VALUES (1, 'resumed');\n"), Mode: 0o444,
	}
	return result, version
}

func seedA25HistoricalRows(t *testing.T, ctx context.Context, database *sql.DB, now int64) (int64, string) {
	t.Helper()
	execA25(t, ctx, database, `INSERT INTO platform_user
(username, password_hash, display_name, role, status, must_change_password, session_version, created_at, updated_at)
VALUES ('a25-admin', 'hash', 'A25 Admin', 'admin', 1, 0, 1, ?, ?)`, now, now)
	execA25(t, ctx, database, `INSERT INTO site
(name, base_url, created_at, updated_at) VALUES ('A25 Site', 'https://a25.example.test', ?, ?)`, now, now)
	execA25(t, ctx, database, `INSERT INTO customer
(name, status, created_at, updated_at) VALUES ('A25 Customer', 'using', ?, ?)`, now, now)
	execA25(t, ctx, database, `INSERT INTO account
(site_id, customer_id, remote_user_id, remote_created_at, username, quota, used_quota, request_count,
 managed_status, created_at, updated_at)
VALUES (1, 1, 9007199254740993, ?, 'a25-user', 123456789, 456, 7, 'active', ?, ?)`, now-3600, now, now)
	execA25(t, ctx, database, `INSERT INTO collection_run
(site_id, site_config_version, task_type, target_type, target_id, trigger_type, start_timestamp, end_timestamp,
 scope, status, next_attempt_at, windows_initialized_at, created_request_id, last_request_id, created_at, updated_at)
VALUES (1, 1, 'usage_backfill', 'site', 1, 'manual', ?, ?, JSON_OBJECT('only_missing', TRUE), 'success', ?, ?, 'a25-create', 'a25-last', ?, ?)`,
		now-7200, now-3600, now, now, now, now)
	return hashA25HistoricalRows(t, ctx, database)
}

func verifyA25HistoricalRows(t *testing.T, ctx context.Context, database *sql.DB) (int64, string, bool, bool) {
	t.Helper()
	rows, digest := hashA25HistoricalRows(t, ctx, database)
	var joined int64
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM account a
JOIN site s ON s.id = a.site_id JOIN customer c ON c.id = a.customer_id
WHERE a.remote_user_id = 9007199254740993 AND s.base_url = 'https://a25.example.test' AND c.name = 'A25 Customer'`).Scan(&joined); err != nil {
		t.Fatalf("verify A25 historical foreign keys: %v", err)
	}
	var scope bool
	if err := database.QueryRowContext(ctx, `SELECT JSON_EXTRACT(scope, '$.only_missing') = TRUE
FROM collection_run WHERE task_type = 'usage_backfill' LIMIT 1`).Scan(&scope); err != nil {
		t.Fatalf("verify A25 migrated collection scope: %v", err)
	}
	return rows, digest, joined == 1, scope
}

func hashA25HistoricalRows(t *testing.T, ctx context.Context, database *sql.DB) (int64, string) {
	t.Helper()
	queries := []string{
		`SELECT CONCAT_WS('#', id, username, display_name, role, status, session_version, created_at, updated_at)
FROM platform_user ORDER BY id`,
		`SELECT CONCAT_WS('#', id, name, base_url, created_at, updated_at) FROM site ORDER BY id`,
		`SELECT CONCAT_WS('#', id, name, status, created_at, updated_at) FROM customer ORDER BY id`,
		`SELECT CONCAT_WS('#', id, site_id, customer_id, remote_user_id, remote_created_at, username,
quota, used_quota, request_count, managed_status, created_at, updated_at) FROM account ORDER BY id`,
	}
	values := make([]string, 0, 4)
	for index, query := range queries {
		rows, err := database.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("read A25 historical query %d: %v", index, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan A25 historical query %d: %v", index, err)
			}
			values = append(values, fmt.Sprintf("%d:%s", index, value))
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate A25 historical query %d: %v", index, err)
		}
		_ = rows.Close()
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return int64(len(values)), hex.EncodeToString(digest[:])
}

func assertA25CurrentMigrationState(t *testing.T, ctx context.Context, database *sql.DB, repository []model.MigrationVersion) {
	t.Helper()
	applied, err := model.ReadMigrationVersions(ctx, database)
	if err != nil {
		t.Fatalf("read A25 migrations: %v", err)
	}
	if err := model.ValidateMigrationVersionPrefix(repository, applied, true); err != nil {
		t.Fatalf("validate A25 migration state: %v", err)
	}
	if progress := countA25Rows(t, ctx, database, "schema_migration_progress"); progress != 0 {
		t.Fatalf("A25 migration progress rows=%d want=0", progress)
	}
}

func readA25AppliedTimes(t *testing.T, ctx context.Context, database *sql.DB) map[string]int64 {
	t.Helper()
	rows, err := database.QueryContext(ctx, "SELECT version, applied_at FROM schema_migration ORDER BY version")
	if err != nil {
		t.Fatalf("read A25 applied times: %v", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]int64)
	for rows.Next() {
		var version string
		var appliedAt int64
		if err := rows.Scan(&version, &appliedAt); err != nil {
			t.Fatalf("scan A25 applied time: %v", err)
		}
		result[version] = appliedAt
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate A25 applied times: %v", err)
	}
	return result
}

func equalA25AppliedTimes(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for version, value := range left {
		if right[version] != value {
			return false
		}
	}
	return true
}

func prepareA25InitialDirtyCheckpoint(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	migration model.MigrationVersion,
	index int,
	ddlCommitted bool,
	now int64,
) {
	t.Helper()
	applyA25MigrationStatement(t, ctx, database, migrations.Files, migration.Version, 0)
	applyA25MigrationStatement(t, ctx, database, migrations.Files, migration.Version, 1)
	if ddlCommitted {
		applyA25MigrationStatement(t, ctx, database, migrations.Files, migration.Version, index)
	}
	insertA25DirtyCheckpoint(t, ctx, database, migration, index, now)
}

func insertA25DirtyCheckpoint(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	migration model.MigrationVersion,
	index int,
	now int64,
) {
	t.Helper()
	execA25(t, ctx, database, `INSERT INTO schema_migration_progress
(version, checksum, statement_index, state, updated_at) VALUES (?, ?, ?, 'dirty', ?)`,
		migration.Version, migration.Checksum, index, now)
}

func applyA25MigrationStatement(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	source fs.FS,
	version string,
	index int,
) {
	t.Helper()
	payload, err := fs.ReadFile(source, version+".sql")
	if err != nil {
		t.Fatalf("read A25 DDL migration: %v", err)
	}
	statements, err := model.SplitSQLStatements(string(payload))
	if err != nil || index < 0 || index >= len(statements) {
		t.Fatalf("parse A25 DDL migration statements=%d err=%v", len(statements), err)
	}
	execA25(t, ctx, database, statements[index])
}

func readA25Progress(t *testing.T, ctx context.Context, database *sql.DB, version string) (int, string) {
	t.Helper()
	var index int
	var state string
	if err := database.QueryRowContext(ctx, `SELECT statement_index, state
FROM schema_migration_progress WHERE version = ?`, version).Scan(&index, &state); err != nil {
		t.Fatalf("read A25 migration progress: %v", err)
	}
	return index, state
}

func hashA25Schema(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	queries := []string{
		`SELECT CONCAT_WS('|', 'table', table_name, engine, table_collation)
FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`,
		`SELECT CONCAT_WS('|', 'column', table_name, ordinal_position, column_name, column_type, is_nullable,
COALESCE(CAST(column_default AS CHAR), '<NULL>'), extra,
COALESCE(character_set_name, '<NULL>'), COALESCE(collation_name, '<NULL>'))
FROM information_schema.columns WHERE table_schema = DATABASE()`,
		`SELECT CONCAT_WS('|', 'index', table_name, index_name, non_unique, seq_in_index, column_name,
COALESCE(collation, '<NULL>'), COALESCE(CAST(sub_part AS CHAR), '<NULL>'), index_type)
FROM information_schema.statistics WHERE table_schema = DATABASE()`,
		`SELECT CONCAT_WS('|', 'fk', table_name, constraint_name, column_name, referenced_table_name,
referenced_column_name, ordinal_position)
FROM information_schema.key_column_usage
WHERE table_schema = DATABASE() AND referenced_table_name IS NOT NULL`,
	}
	values := make([]string, 0, 2048)
	for index, query := range queries {
		rows, err := database.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("read A25 schema query %d: %v", index, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan A25 schema query %d: %v", index, err)
			}
			values = append(values, value)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate A25 schema query %d: %v", index, err)
		}
		_ = rows.Close()
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:])
}

func probeA25UnsupportedServer(t *testing.T, ctx context.Context, dsn string, wantMariaDB bool) a25NegativeServerProbe {
	t.Helper()
	raw, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open A25 unsupported server: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("ping A25 unsupported server: %v", err)
	}
	version := readA25ServerVersion(t, ctx, raw)
	if wantMariaDB != strings.Contains(strings.ToLower(version), "mariadb") {
		t.Fatalf("A25 unsupported server version=%q MariaDB=%t", version, wantMariaDB)
	}
	beforeTables := countA25Tables(t, ctx, raw)
	beforeLock := readA25MigrationLock(t, ctx, raw)
	_, openErr := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 1, MaxOpen: 2, MaxLifetime: time.Minute})
	if openErr == nil {
		t.Fatalf("A25 unsupported server %q was accepted", version)
	}
	afterTables := countA25Tables(t, ctx, raw)
	afterLock := readA25MigrationLock(t, ctx, raw)
	return a25NegativeServerProbe{
		Version: version, Rejected: true, TablesBefore: beforeTables, TablesAfter: afterTables,
		LockBeforeEmpty: !beforeLock.Valid, LockAfterEmpty: !afterLock.Valid,
	}
}

func readA25ServerVersion(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	var version string
	if err := database.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatalf("read A25 server version: %v", err)
	}
	return version
}

func readA25MigrationLock(t *testing.T, ctx context.Context, database *sql.DB) sql.NullInt64 {
	t.Helper()
	var owner sql.NullInt64
	if err := database.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?)", model.MigrationLockName).Scan(&owner); err != nil {
		t.Fatalf("read A25 migration lock: %v", err)
	}
	return owner
}

func countA25Tables(t *testing.T, ctx context.Context, database *sql.DB) int64 {
	t.Helper()
	var count int64
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`).Scan(&count); err != nil {
		t.Fatalf("count A25 tables: %v", err)
	}
	return count
}

func countA25Rows(t *testing.T, ctx context.Context, database *sql.DB, table string) int64 {
	t.Helper()
	for _, character := range table {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			t.Fatalf("unsafe A25 table %q", table)
		}
	}
	var count int64
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
		t.Fatalf("count A25 %s: %v", table, err)
	}
	return count
}

func execA25(t *testing.T, ctx context.Context, database *sql.DB, query string, arguments ...any) {
	t.Helper()
	if _, err := database.ExecContext(ctx, query, arguments...); err != nil {
		t.Fatalf("execute A25 SQL: %v", err)
	}
}

func assertA25EvidenceDirectory(t *testing.T, directory string) {
	t.Helper()
	if directory == "" || !filepath.IsAbs(directory) {
		t.Fatal("A25 evidence directory must be an existing absolute path")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		t.Fatalf("A25 evidence directory is invalid: %v", err)
	}
}

func writeA25Report(t *testing.T, directory string, report a25Report) {
	t.Helper()
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal A25 report: %v", err)
	}
	payload = append(payload, '\n')
	temporary := filepath.Join(directory, "a25-report.json.tmp")
	final := filepath.Join(directory, "a25-report.json")
	if err := os.WriteFile(temporary, payload, 0o640); err != nil {
		t.Fatalf("write A25 report: %v", err)
	}
	if err := os.Rename(temporary, final); err != nil {
		t.Fatalf("publish A25 report: %v", err)
	}
}
