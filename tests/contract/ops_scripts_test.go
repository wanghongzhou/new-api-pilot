package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"new-api-pilot/model"
)

func TestBackupScriptContract(t *testing.T) {
	path, source := readScript(t, "backup.sh")
	for _, required := range []string{
		"--defaults-extra-file=\"$MYSQL_DEFAULTS_FILE\"",
		"--single-transaction",
		"--source-data=2",
		"GET_LOCK('" + model.MigrationLockName + "', 60)",
		"schema_migrations",
		"encryption_key_id",
		"${manifest_path}.sha256",
		"mv -- \"$staging_directory\" \"$final_directory\"",
		"trap on_exit EXIT",
		"report_backup_metrics failure",
		"report_backup_metrics success",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("%s is missing contract fragment %q", path, required)
		}
	}
	assertNoPasswordArgument(t, path, source)
	lockPosition := strings.LastIndex(source, "acquire_migration_lock")
	dumpPosition := strings.Index(source, "mysqldump --defaults-extra-file")
	migrationMetadataPosition := strings.Index(source, "SELECT version, checksum FROM schema_migration")
	releasePosition := strings.LastIndex(source, "release_migration_lock")
	if lockPosition < 0 || dumpPosition < lockPosition {
		t.Errorf("%s starts mysqldump before acquiring the migration advisory lock", path)
	}
	if migrationMetadataPosition < dumpPosition || releasePosition < migrationMetadataPosition {
		t.Errorf("%s releases the migration advisory lock before metadata capture completes", path)
	}
}

func TestRestoreScriptContract(t *testing.T) {
	path, source := readScript(t, "restore.sh")
	required := []string{
		"target database must be empty",
		"database-identity",
		"SELECT @@server_uuid, DATABASE()",
		"MYSQL_DEFAULTS_FILE and DATABASE_DSN must identify the same MySQL server",
		"verify_sidecar",
		"gzip -t",
		"verify-backup",
		"--mode=manifest-only",
		"verify-restore --mode=full --manifest=\"$manifest_path\"",
		".status == \"success\" and .summary.failed == 0",
		"mv -- \"$staging_release\" \"$release_directory\"",
	}
	for _, fragment := range required {
		position := strings.Index(source, fragment)
		if position < 0 {
			t.Errorf("%s is missing contract fragment %q", path, fragment)
		}
	}
	verifyPosition := strings.Index(source, "verify-restore --mode=full")
	preflightPosition := strings.Index(source, "verify-backup")
	identityPosition := strings.Index(source, `"$NEW_API_PILOT_BIN" database-identity`)
	importPosition := strings.Index(source, "gzip -cd \"$dump_path\"")
	publishPosition := strings.LastIndex(source, "mv -- \"$staging_release\" \"$release_directory\"")
	if preflightPosition < 0 || importPosition < preflightPosition {
		t.Errorf("%s imports MySQL before the complete manifest preflight", path)
	}
	if identityPosition < preflightPosition || identityPosition > importPosition {
		t.Errorf("%s does not compare the defaults-file and DSN server identity before import", path)
	}
	if verifyPosition < 0 || publishPosition < verifyPosition {
		t.Errorf("%s publishes its release gate before full verification", path)
	}
	for _, forbidden := range []string{"skip-ssl", "ssl-mode=DISABLED", "--ssl=0"} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("%s weakens MySQL TLS with %q; any TLS bypass belongs only in an isolated test fixture", path, forbidden)
		}
	}
	assertNoPasswordArgument(t, path, source)
}

func TestOpsScriptsHaveValidBashSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	root := repositoryRoot(t)
	command := exec.Command(
		bash, "-n",
		filepath.Join(root, "scripts", "backup.sh"),
		filepath.Join(root, "scripts", "backup_metrics.sh"),
		filepath.Join(root, "scripts", "restore.sh"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bash syntax check failed: %v\n%s", err, output)
	}
}

func TestBackupMetricsHelperPublishesAtomicSuccessAndFailureState(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	directory := t.TempDir()
	helper := filepath.Join(repositoryRoot(t), "scripts", "backup_metrics.sh")
	run := func(arguments ...string) {
		t.Helper()
		command := exec.Command(bash, append([]string{helper}, arguments...)...)
		command.Env = append(os.Environ(), "PROMETHEUS_TEXTFILE_DIR="+directory)
		if output, runErr := command.CombinedOutput(); runErr != nil {
			t.Fatalf("publish backup metrics: %v\n%s", runErr, output)
		}
	}

	run("success", "100", "105", "42")
	run("failure", "200", "207", "0")
	payload, err := os.ReadFile(filepath.Join(directory, "new_api_pilot_backup.prom"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, expected := range []string{
		"new_api_pilot_backup_last_success_timestamp_seconds 105",
		"new_api_pilot_backup_last_failure_timestamp_seconds 207",
		"new_api_pilot_backup_failures_total 1",
		"new_api_pilot_backup_last_duration_seconds 7",
		"new_api_pilot_backup_last_size_bytes 42",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("backup textfile is missing %q\n%s", expected, body)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "new_api_pilot_backup.prom" {
		t.Fatalf("backup metrics publication left temporary files: %+v", entries)
	}
}

func assertNoPasswordArgument(t *testing.T, path, source string) {
	t.Helper()
	for _, forbidden := range []string{"--password", "--password=", " -p$", " -p\""} {
		if strings.Contains(source, forbidden) {
			t.Errorf("%s contains forbidden password argument fragment %q", path, forbidden)
		}
	}
}

func readScript(t *testing.T, name string) (string, string) {
	t.Helper()
	path := filepath.Join(repositoryRoot(t), "scripts", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return path, string(payload)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve contract test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
