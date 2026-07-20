package integration

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"new-api-pilot/internal/ops"
	"new-api-pilot/migrations"
	"new-api-pilot/model"
)

func TestA22RestoreSafetyRejectsPreImportIdentityAndMigrationMismatch(t *testing.T) {
	if os.Getenv("A22_RESTORE_SAFETY") != "true" {
		t.Skip("A22_RESTORE_SAFETY is not enabled")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required for restore safety integration: %v", err)
	}
	databaseName := requireA22RestoreEnvironment(t, "A22_RESTORE_DATABASE")
	primaryDSN := requireA22RestoreEnvironment(t, "A22_RESTORE_PRIMARY_DSN")
	secondaryDSN := requireA22RestoreEnvironment(t, "A22_RESTORE_SECONDARY_DSN")
	defaultsFile := requireA22RestoreEnvironment(t, "A22_RESTORE_MYSQL_DEFAULTS_FILE")
	binaryPath := requireA22RestoreEnvironment(t, "A22_RESTORE_NEW_API_PILOT_BIN")
	restoreScript := requireA22RestoreEnvironment(t, "A22_RESTORE_SCRIPT")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	open := func(dsn string) *model.Database {
		t.Helper()
		database, openErr := model.Open(ctx, model.Options{
			DSN: dsn, MaxIdle: 1, MaxOpen: 2, MaxLifetime: time.Minute,
		})
		if openErr != nil {
			t.Fatalf("open restore safety database: %v", openErr)
		}
		t.Cleanup(func() { _ = database.Close() })
		return database
	}
	primary := open(primaryDSN)
	secondary := open(secondaryDSN)
	primaryIdentity, err := ops.RunDatabaseIdentity(ctx, primary.SQL)
	if err != nil {
		t.Fatalf("read primary identity: %v", err)
	}
	secondaryIdentity, err := ops.RunDatabaseIdentity(ctx, secondary.SQL)
	if err != nil {
		t.Fatalf("read secondary identity: %v", err)
	}
	if primaryIdentity.Database != databaseName || secondaryIdentity.Database != databaseName ||
		primaryIdentity.ServerUUID == secondaryIdentity.ServerUUID {
		t.Fatalf("restore safety topology is invalid: primary=%#v secondary=%#v", primaryIdentity, secondaryIdentity)
	}
	assertA22RestoreDatabaseEmpty(t, ctx, primary, databaseName)
	assertA22RestoreDatabaseEmpty(t, ctx, secondary, databaseName)

	key := []byte("01234567890123456789012345678901")
	encodedKey := base64.StdEncoding.EncodeToString(key)
	keyDigest := sha256.Sum256(key)
	keyID := hex.EncodeToString(keyDigest[:])

	t.Run("different server UUID with same database name", func(t *testing.T) {
		manifestPath := writeA22RestoreManifest(t, t.TempDir(), databaseName, keyID, primaryIdentity.ServerUUID, false)
		releaseRoot := filepath.Join(t.TempDir(), "release")
		output, runErr := runA22RestoreScript(t, bash, restoreScript, manifestPath, releaseRoot, defaultsFile,
			binaryPath, databaseName, secondaryDSN, encodedKey)
		if runErr == nil || !strings.Contains(output, "must identify the same MySQL server") {
			t.Fatalf("cross-server restore output=%q error=%v", output, runErr)
		}
		assertA22RestoreDatabaseEmpty(t, ctx, primary, databaseName)
		assertA22RestoreDatabaseEmpty(t, ctx, secondary, databaseName)
		assertA22NoReleaseGate(t, releaseRoot)
	})

	t.Run("valid-format migration checksum tamper with rewritten sidecar", func(t *testing.T) {
		manifestPath := writeA22RestoreManifest(t, t.TempDir(), databaseName, keyID, primaryIdentity.ServerUUID, true)
		releaseRoot := filepath.Join(t.TempDir(), "release")
		output, runErr := runA22RestoreScript(t, bash, restoreScript, manifestPath, releaseRoot, defaultsFile,
			binaryPath, databaseName, primaryDSN, encodedKey)
		if runErr == nil || !strings.Contains(output, "backup manifest preflight failed") {
			t.Fatalf("migration-tamper restore output=%q error=%v", output, runErr)
		}
		assertA22RestoreDatabaseEmpty(t, ctx, primary, databaseName)
		assertA22RestoreDatabaseEmpty(t, ctx, secondary, databaseName)
		assertA22NoReleaseGate(t, releaseRoot)
	})
}

func requireA22RestoreEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required when A22_RESTORE_SAFETY=true", name)
	}
	return value
}

func runA22RestoreScript(
	t *testing.T,
	bash string,
	restoreScript string,
	manifestPath string,
	releaseRoot string,
	defaultsFile string,
	binaryPath string,
	databaseName string,
	databaseDSN string,
	encodedKey string,
) (string, error) {
	t.Helper()
	command := exec.Command(bash, restoreScript)
	command.Env = append(os.Environ(),
		"RESTORE_MANIFEST="+manifestPath,
		"RESTORE_RELEASE_ROOT="+releaseRoot,
		"MYSQL_DEFAULTS_FILE="+defaultsFile,
		"NEW_API_PILOT_BIN="+binaryPath,
		"MYSQL_DATABASE="+databaseName,
		"DATABASE_DSN="+databaseDSN,
		"ENCRYPTION_KEY="+encodedKey,
	)
	output, err := command.CombinedOutput()
	return string(output), err
}

func writeA22RestoreManifest(
	t *testing.T,
	directory string,
	databaseName string,
	keyID string,
	serverUUID string,
	tamperMigration bool,
) string {
	t.Helper()
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	if _, err := zipper.Write([]byte("-- CHANGE REPLICATION SOURCE TO SOURCE_LOG_FILE='binlog.000001', SOURCE_LOG_POS=123;\n" +
		"CREATE TABLE a22_restore_import_must_not_run (id BIGINT NOT NULL PRIMARY KEY);\n")); err != nil {
		t.Fatalf("write restore safety dump: %v", err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatalf("close restore safety dump: %v", err)
	}
	dumpName := "database.sql.gz"
	dumpPath := filepath.Join(directory, dumpName)
	if err := os.WriteFile(dumpPath, compressed.Bytes(), 0o600); err != nil {
		t.Fatalf("write restore safety dump: %v", err)
	}
	dumpDigest := sha256.Sum256(compressed.Bytes())
	dumpSHA := hex.EncodeToString(dumpDigest[:])
	writeA22RestoreSidecar(t, dumpPath+".sha256", dumpSHA, dumpName)

	repository, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	manifestMigrations := make([]ops.ManifestMigration, 0, len(repository))
	for _, migration := range repository {
		manifestMigrations = append(manifestMigrations, ops.ManifestMigration{
			Version: migration.Version, Checksum: migration.Checksum,
		})
	}
	if tamperMigration {
		tampered := strings.Repeat("f", 64)
		if manifestMigrations[0].Checksum == tampered {
			tampered = strings.Repeat("e", 64)
		}
		manifestMigrations[0].Checksum = tampered
	}
	manifest := ops.BackupManifest{
		SchemaVersion: 1, BackupID: "backup-20260715T010203Z-a22a22a2", CreatedAtUTC: "2026-07-15T01:02:03Z",
		Database: databaseName, DumpFile: dumpName, DumpSHA256: dumpSHA, DumpSizeBytes: int64(compressed.Len()),
		ImageDigest: "sha256:" + strings.Repeat("a", 64), EncryptionKeyID: keyID,
		MySQLVersion: "8.4", ServerUUID: serverUUID,
		Source:     ops.BackupSource{LogFile: "binlog.000001", LogPosition: 123},
		Migrations: manifestMigrations, ExportFiles: "excluded_regenerable",
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal restore safety manifest: %v", err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatalf("write restore safety manifest: %v", err)
	}
	manifestDigest := sha256.Sum256(payload)
	writeA22RestoreSidecar(t, manifestPath+".sha256", hex.EncodeToString(manifestDigest[:]), "manifest.json")
	return manifestPath
}

func writeA22RestoreSidecar(t *testing.T, path string, digest string, name string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(digest+"  "+name+"\n"), 0o600); err != nil {
		t.Fatalf("write restore safety sidecar: %v", err)
	}
}

func assertA22RestoreDatabaseEmpty(t *testing.T, ctx context.Context, database *model.Database, databaseName string) {
	t.Helper()
	var count int
	if err := database.SQL.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", databaseName).Scan(&count); err != nil {
		t.Fatalf("count restore safety tables: %v", err)
	}
	if count != 0 {
		t.Fatalf("restore safety target %s contains %d tables", databaseName, count)
	}
}

func assertA22NoReleaseGate(t *testing.T, releaseRoot string) {
	t.Helper()
	entries, err := os.ReadDir(releaseRoot)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("read restore release root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("restore failure created release gate entries: %#v", entries)
	}
}
