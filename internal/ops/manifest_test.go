package ops

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"new-api-pilot/migrations"
	"new-api-pilot/model"
)

const manifestTestKeyID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidateBackupManifest(t *testing.T) {
	directory := t.TempDir()
	manifestPath := writeManifestFixture(t, directory)
	validated, err := ValidateBackupManifest(manifestPath, manifestTestKeyID)
	if err != nil {
		t.Fatalf("ValidateBackupManifest() error = %v", err)
	}
	if validated.Manifest.BackupID != "backup-20260714T010203Z-01234567" || validated.ManifestHash == "" {
		t.Fatalf("unexpected validated manifest: %#v", validated)
	}
}

func TestRunManifestPreflightReturnsStableJSONReport(t *testing.T) {
	manifestPath := writeManifestFixture(t, t.TempDir())
	report, err := RunManifestPreflight(manifestPath, manifestTestKeyID)
	if err != nil || report.Status != "success" || report.Command != "verify-backup" ||
		report.Mode != "manifest-only" || report.Summary.Failed != 0 {
		t.Fatalf("manifest preflight report = %#v, error = %v", report, err)
	}

	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["image_digest"] = "bad"
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode corrupt manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatalf("write corrupt manifest: %v", err)
	}
	writeSidecar(t, manifestPath+".sha256", sha256Hex(payload), "manifest.json")
	report, err = RunManifestPreflight(manifestPath, manifestTestKeyID)
	if err == nil || report.Status != "failed" || report.Error == nil ||
		report.Error.Code != "VERIFY_MANIFEST_INVALID" {
		t.Fatalf("corrupt manifest preflight report = %#v, error = %v", report, err)
	}
}

func TestRunManifestPreflightRejectsCompleteManifestContract(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "unknown field", mutate: func(manifest map[string]any) { manifest["unexpected"] = true }},
		{name: "created at", mutate: func(manifest map[string]any) { manifest["created_at_utc"] = "not-utc" }},
		{name: "database", mutate: func(manifest map[string]any) { manifest["database"] = "bad/name" }},
		{name: "image digest", mutate: func(manifest map[string]any) { manifest["image_digest"] = "bad" }},
		{name: "mysql identity", mutate: func(manifest map[string]any) { manifest["server_uuid"] = "" }},
		{name: "source coordinate", mutate: func(manifest map[string]any) {
			manifest["source"] = map[string]any{"log_file": "binlog.999999", "log_position": float64(123)}
		}},
		{name: "migration ordering", mutate: func(manifest map[string]any) {
			manifest["schema_migrations"] = []any{
				map[string]any{"version": "0002_second", "checksum": strings.Repeat("c", 64)},
				map[string]any{"version": "0001_first", "checksum": strings.Repeat("b", 64)},
			}
		}},
		{name: "export policy", mutate: func(manifest map[string]any) { manifest["export_files"] = "included" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifestPath := writeManifestFixture(t, t.TempDir())
			payload, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(payload, &manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			test.mutate(manifest)
			payload, err = json.Marshal(manifest)
			if err != nil {
				t.Fatalf("encode manifest: %v", err)
			}
			if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			writeSidecar(t, manifestPath+".sha256", sha256Hex(payload), "manifest.json")
			report, err := RunManifestPreflight(manifestPath, manifestTestKeyID)
			if err == nil || report.Error == nil || report.Error.Code != "VERIFY_MANIFEST_INVALID" {
				t.Fatalf("manifest preflight report = %#v, error = %v", report, err)
			}
		})
	}

	manifestPath := writeManifestFixture(t, t.TempDir())
	report, err := RunManifestPreflight(manifestPath, strings.Repeat("f", 64))
	if err == nil || report.Error == nil || report.Error.Code != "VERIFY_MANIFEST_INVALID" {
		t.Fatalf("wrong-key manifest preflight report = %#v, error = %v", report, err)
	}
}

func TestRunManifestPreflightRejectsRepositoryMigrationChecksumTamper(t *testing.T) {
	manifestPath := writeManifestFixture(t, t.TempDir())
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Migrations) == 0 {
		t.Fatal("manifest fixture contains no migrations")
	}
	tampered := strings.Repeat("f", 64)
	if manifest.Migrations[0].Checksum == tampered {
		tampered = strings.Repeat("e", 64)
	}
	manifest.Migrations[0].Checksum = tampered
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode tampered manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatalf("write tampered manifest: %v", err)
	}
	writeSidecar(t, manifestPath+".sha256", sha256Hex(payload), "manifest.json")

	report, err := RunManifestPreflight(manifestPath, manifestTestKeyID)
	if err == nil || report.Status != "failed" || report.Error == nil ||
		report.Error.Code != "VERIFY_MIGRATION_INVALID" || report.Summary.Passed != 1 || report.Summary.Failed != 1 {
		t.Fatalf("migration checksum tamper report = %#v, error = %v", report, err)
	}
}

func TestValidateBackupManifestRejectsRelativePath(t *testing.T) {
	if _, err := ValidateBackupManifest("manifest.json", manifestTestKeyID); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path error = %v", err)
	}
}

func TestValidateBackupManifestRejectsDumpCorruption(t *testing.T) {
	directory := t.TempDir()
	manifestPath := writeManifestFixture(t, directory)
	dumpPath := filepath.Join(directory, "database.sql.gz")
	if err := os.WriteFile(dumpPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt dump: %v", err)
	}
	if _, err := ValidateBackupManifest(manifestPath, manifestTestKeyID); err == nil || !strings.Contains(err.Error(), "checksum or size") {
		t.Fatalf("corrupt dump error = %v", err)
	}
}

func TestValidateBackupManifestRejectsSymlinkComponent(t *testing.T) {
	realDirectory := t.TempDir()
	writeManifestFixture(t, realDirectory)
	root := t.TempDir()
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	_, err := ValidateBackupManifest(filepath.Join(linkedDirectory, "manifest.json"), manifestTestKeyID)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("symlink path error = %v", err)
	}
}

func TestReadBoundedFileRejectsOversizedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if _, err := readBoundedFile(path, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized payload error = %v", err)
	}
	payload, err := readBoundedFile(path, 5)
	if err != nil || string(payload) != "12345" {
		t.Fatalf("bounded payload = %q, %v", payload, err)
	}
}

func writeManifestFixture(t *testing.T, directory string) string {
	t.Helper()
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	if _, err := zipper.Write([]byte("-- CHANGE REPLICATION SOURCE TO SOURCE_LOG_FILE='binlog.000001', SOURCE_LOG_POS=123;\nSELECT 1;\n")); err != nil {
		t.Fatalf("write compressed dump: %v", err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatalf("close compressed dump: %v", err)
	}
	dumpName := "database.sql.gz"
	dumpPath := filepath.Join(directory, dumpName)
	if err := os.WriteFile(dumpPath, compressed.Bytes(), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	dumpHash := sha256Hex(compressed.Bytes())
	writeSidecar(t, dumpPath+".sha256", dumpHash, dumpName)

	repositoryMigrations, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	manifestMigrations := make([]ManifestMigration, 0, len(repositoryMigrations))
	for _, migration := range repositoryMigrations {
		manifestMigrations = append(manifestMigrations, ManifestMigration{
			Version: migration.Version, Checksum: migration.Checksum,
		})
	}
	manifest := BackupManifest{
		SchemaVersion:   1,
		BackupID:        "backup-20260714T010203Z-01234567",
		CreatedAtUTC:    "2026-07-14T01:02:03Z",
		Database:        "new_api_pilot",
		DumpFile:        dumpName,
		DumpSHA256:      dumpHash,
		DumpSizeBytes:   int64(compressed.Len()),
		ImageDigest:     "sha256:" + strings.Repeat("a", 64),
		EncryptionKeyID: manifestTestKeyID,
		MySQLVersion:    "8.4.6",
		ServerUUID:      "00000000-0000-0000-0000-000000000001",
		Source: BackupSource{
			LogFile: "binlog.000001", LogPosition: 123,
		},
		Migrations:  manifestMigrations,
		ExportFiles: "excluded_regenerable",
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	writeSidecar(t, manifestPath+".sha256", sha256Hex(payload), "manifest.json")
	return manifestPath
}

func writeSidecar(t *testing.T, path, hash, name string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(hash+"  "+name+"\n"), 0o600); err != nil {
		t.Fatalf("write checksum sidecar: %v", err)
	}
}

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
