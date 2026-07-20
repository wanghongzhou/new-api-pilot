package a25evidence

import (
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

func TestClassifyRequiresCanonicalA25FormalCommand(t *testing.T) {
	class, err := Classify(AcceptanceID, "artifacts/acceptance", canonicalCommand)
	if err != nil || class != FormalClass {
		t.Fatalf("canonical A25 class=%q err=%v", class, err)
	}
	if _, err := Classify(AcceptanceID, "artifacts/acceptance", []string{"cmd.exe", "/c", "exit", "0"}); err == nil {
		t.Fatal("arbitrary A25 exit-zero command was accepted")
	}
	if _, err := Classify(AcceptanceID, "artifacts/smoke", canonicalCommand); err == nil {
		t.Fatal("noncanonical A25 evidence root was accepted")
	}
}

func TestValidateA25RunRejectsTamperSkipAndResiduals(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		run := writeValidA25Run(t)
		if err := ValidateRunDirectory(run, FormalClass); err != nil {
			t.Fatalf("validate A25 run: %v", err)
		}
	})

	t.Run("artifact tamper", func(t *testing.T) {
		run := writeValidA25Run(t)
		path := filepath.Join(run, "a25-report.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for index, value := range payload {
			if value == '\n' {
				payload[index] = ' '
				break
			}
		}
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateInnerArtifacts(run, FormalClass); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
			t.Fatalf("tampered artifact error=%v, want SHA-256 mismatch", err)
		}
	})

	t.Run("skip", func(t *testing.T) {
		run := writeValidA25Run(t)
		var summary testSummary
		readA25TestJSON(t, filepath.Join(run, "a25-test-summary.json"), &summary)
		summary.SkipEvents = 1
		writeA25TestJSON(t, filepath.Join(run, "a25-test-summary.json"), summary)
		writeA25Inventory(t, run)
		if err := ValidateInnerArtifacts(run, FormalClass); err == nil || !strings.Contains(err.Error(), "test summary") {
			t.Fatalf("skip evidence error=%v", err)
		}
	})

	t.Run("cleanup residual", func(t *testing.T) {
		run := writeValidA25Run(t)
		var cleanup cleanupReport
		readA25TestJSON(t, filepath.Join(run, "a25-cleanup.json"), &cleanup)
		cleanup.Residuals.Containers = []string{"a25-residual"}
		writeA25TestJSON(t, filepath.Join(run, "a25-cleanup.json"), cleanup)
		writeA25Inventory(t, run)
		if err := ValidateInnerArtifacts(run, FormalClass); err == nil || !strings.Contains(err.Error(), "cleanup") {
			t.Fatalf("residual evidence error=%v", err)
		}
	})

	t.Run("negative gate omitted", func(t *testing.T) {
		run := writeValidA25Run(t)
		var report finalReport
		readA25TestJSON(t, filepath.Join(run, "a25-report.json"), &report)
		report.VersionGate.LegacyMySQLRejected = false
		writeA25TestJSON(t, filepath.Join(run, "a25-report.json"), report)
		writeA25Inventory(t, run)
		if err := ValidateInnerArtifacts(run, FormalClass); err == nil || !strings.Contains(err.Error(), "version gate") {
			t.Fatalf("missing version gate error=%v", err)
		}
	})

	t.Run("stale migration inventory", func(t *testing.T) {
		run := writeValidA25Run(t)
		path := filepath.Join(run, "a25-report.json")
		var report finalReport
		readA25TestJSON(t, path, &report)
		report.RepositoryMigrations = report.RepositoryMigrations[:len(report.RepositoryMigrations)-1]
		writeA25TestJSON(t, path, report)
		writeA25Inventory(t, run)
		if err := ValidateInnerArtifacts(run, FormalClass); err == nil ||
			!strings.Contains(err.Error(), "migration inventory count is stale") {
			t.Fatalf("stale migration inventory error=%v", err)
		}
	})
}

func writeValidA25Run(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	repository, err := model.LoadMigrationVersions(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	report := finalReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed",
		FixturePath: "testdata/design/f05-ops-capacity.yaml", FixtureSHA256: strings.Repeat("b", 64),
		FixedNowUnix: 1768665599, AuthoritativeSchemaSHA256: hash,
	}
	for _, migration := range repository {
		report.RepositoryMigrations = append(report.RepositoryMigrations, migrationEvidence{
			Version: migration.Version, Checksum: migration.Checksum,
		})
	}
	report.VersionGate.CurrentVersion = "8.4.6"
	report.VersionGate.LegacyMySQLVersion = "5.7.44"
	report.VersionGate.MariaDBVersion = "10.11.14-MariaDB"
	report.VersionGate.CurrentAccepted = true
	report.VersionGate.LegacyMySQLRejected = true
	report.VersionGate.MariaDBRejected = true
	report.VersionGate.LegacyLockAbsentBefore = true
	report.VersionGate.LegacyLockAbsentAfter = true
	report.VersionGate.MariaDBLockAbsentBefore = true
	report.VersionGate.MariaDBLockAbsentAfter = true
	report.EmptyDatabase.MigrationCount = len(repository)
	report.EmptyDatabase.SchemaSHA256 = hash
	report.EmptyDatabase.AppliedAtStable = true
	report.EmptyDatabase.IdempotentSchemaStable = true
	report.Upgrade.PrefixMigrationCount = 1
	report.Upgrade.HistoricalRows = 5
	report.Upgrade.HistoricalSHA256 = strings.Repeat("c", 64)
	report.Upgrade.HistoricalPreserved = true
	report.Upgrade.ForeignKeysPreserved = true
	report.Upgrade.BackfillScopeMigrated = true
	report.Upgrade.SchemaSHA256 = hash
	report.Upgrade.MatchesAuthoritative = true
	report.Tamper.DatabaseChecksumRejected = true
	report.Tamper.RepositorySourceRejected = true
	report.Tamper.UnknownVersionRejected = true
	report.Tamper.NoSchemaMutation = true
	report.DMLFailure.InitialFailureObserved = true
	report.DMLFailure.CheckpointReady = true
	report.DMLFailure.ResumeCompleted = true
	report.DMLFailure.IdempotentRowCount = 1
	report.DDLRecovery.DirtyWithoutDDLReplayed = true
	report.DDLRecovery.DirtyWithDDLRecognized = true
	report.DDLRecovery.ReplaySchemaSHA256 = hash
	report.DDLRecovery.CommittedSchemaSHA256 = hash
	writeA25TestJSON(t, filepath.Join(run, "a25-report.json"), report)

	events := []goTestEvent{
		{Action: "run", Package: "new-api-pilot/tests/integration", Test: "TestA25MigrationAcceptance"},
		{Action: "pass", Package: "new-api-pilot/tests/integration", Test: "TestA25MigrationAcceptance"},
		{Action: "pass", Package: "new-api-pilot/tests/integration"},
	}
	var stream strings.Builder
	encoder := json.NewEncoder(&stream)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(run, "a25-test.jsonl"), []byte(stream.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "a25-test.stderr.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writeA25TestJSON(t, filepath.Join(run, "a25-test-summary.json"), testSummary{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", TargetTest: "TestA25MigrationAcceptance",
		Package: "new-api-pilot/tests/integration", PassEvents: 1, JSONLines: len(events),
		JSONPath: "a25-test.jsonl", StderrPath: "a25-test.stderr.log",
	})
	writeA25TestJSON(t, filepath.Join(run, "a25-command.json"), commandReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass,
		TargetTest: "TestA25MigrationAcceptance", WorkingDir: "/workspace", Command: innerCommand,
		GoImage: "golang:1.25.1", MySQLImage: "mysql:8.4", LegacyImage: "mysql:5.7", MariaDBImage: "mariadb:10.11",
	})

	environment := environmentReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass, Commit: "unborn",
		WorktreeDirty: true, IsolatedGuard: true, RepositoryReadOnly: true, EvidenceWritable: true,
		OfflineTestNetwork: true,
	}
	environment.Images.Go = a25TestImage("golang:1.25.1", "golang")
	environment.Images.Current = a25TestImage("mysql:8.4", "mysql")
	environment.Images.Legacy = a25TestImage("mysql:5.7", "mysql")
	environment.Images.MariaDB = a25TestImage("mariadb:10.11", "mariadb")
	environment.Servers.Current = currentServer{
		Version: "8.4.6", TransactionIsolation: "READ-COMMITTED", CharacterSetServer: "utf8mb4",
		CollationServer: "utf8mb4_unicode_ci", TimeZone: "+08:00",
	}
	environment.Servers.Legacy.Version = "5.7.44"
	environment.Servers.MariaDB.Version = "10.11.14-MariaDB"
	environment.Network.Internal = true
	environment.Network.HostPorts = []string{}
	environment.Databases.Current = "pilot_a25"
	environment.Databases.Legacy = "pilot_a25_legacy"
	environment.Databases.MariaDB = "pilot_a25_mariadb"
	environment.Resources.Network = "new-api-pilot-a25-abc-0123456789ab-network"
	environment.Resources.ModuleCache = "new-api-pilot-a25-abc-0123456789ab-gomod"
	environment.Resources.BuildCache = "new-api-pilot-a25-abc-0123456789ab-gobuild"
	writeA25TestJSON(t, filepath.Join(run, "a25-environment.json"), environment)
	writeA25TestJSON(t, filepath.Join(run, "a25-fixture.json"), fixtureReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, FixtureID: "F05",
		Path: report.FixturePath, SHA256: report.FixtureSHA256, FixedNowUnix: report.FixedNowUnix,
		MigrationCount: len(repository),
	})
	cleanup := cleanupReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass,
		Passed: true, SweepsSucceeded: true,
	}
	cleanup.Lifecycle.Containers = "created_and_removed"
	cleanup.Lifecycle.Networks = "created_and_removed"
	cleanup.Lifecycle.Volumes = "created_and_removed"
	cleanup.Residuals.Containers = []string{}
	cleanup.Residuals.Networks = []string{}
	cleanup.Residuals.Volumes = []string{}
	cleanup.Residuals.Images = []string{}
	writeA25TestJSON(t, filepath.Join(run, "a25-cleanup.json"), cleanup)
	if err := os.WriteFile(filepath.Join(run, "stdout.log"), []byte("A25 passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "stderr.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writeA25Inventory(t, run)
	writeA25TestJSON(t, filepath.Join(run, "evidence.json"), wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: FormalClass,
		Command: canonicalCommand, WorkingDirectory: ".", StartedAt: "2026-07-15T01:02:03Z",
		FinishedAt: "2026-07-15T01:03:03Z", DurationMilliseconds: 60000, ExitCode: 0,
		Commit: "unborn", WorktreeDirty: true, FixtureManifestPath: "testdata/design/manifest.sha256",
		FixtureManifestSHA: strings.Repeat("d", 64), StdoutLog: "stdout.log", StderrLog: "stderr.log",
		RequiredNoSkip: true,
	})
	return run
}

func a25TestImage(reference, repository string) imageIdentity {
	digest := strings.Repeat("e", 64)
	return imageIdentity{Reference: reference, ID: "sha256:" + digest, Digest: repository + "@sha256:" + digest}
}

func writeA25Inventory(t *testing.T, run string) {
	t.Helper()
	inventory := artifactInventory{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass}
	for _, name := range requiredArtifacts {
		path := filepath.Join(run, name)
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		inventory.Files = append(inventory.Files, artifactEntry{
			Path: name, SizeBytes: info.Size(), SHA256: hex.EncodeToString(digest[:]),
		})
	}
	writeA25TestJSON(t, filepath.Join(run, "a25-artifacts.json"), inventory)
}

func writeA25TestJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readA25TestJSON(t *testing.T, path string, target any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatal(err)
	}
}
