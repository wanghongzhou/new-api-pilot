package a62evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyRequiresCanonicalA62FormalCommand(t *testing.T) {
	if class, err := Classify(AcceptanceID, "artifacts/acceptance", canonicalCommand); err != nil || class != FormalClass {
		t.Fatalf("canonical A62 class=%q err=%v", class, err)
	}
	if _, err := Classify(AcceptanceID, "artifacts/acceptance", []string{"powershell.exe", "-Command", "exit 0"}); err == nil {
		t.Fatal("arbitrary A62 exit-zero command was accepted")
	}
	if _, err := Classify(AcceptanceID, "artifacts/smoke", canonicalCommand); err == nil {
		t.Fatal("noncanonical A62 evidence root was accepted")
	}
	if class, err := Classify("A61", "artifacts/acceptance", []string{"anything"}); err != nil || class != "" {
		t.Fatalf("unrelated acceptance class=%q err=%v", class, err)
	}
}

func TestValidateRunDirectoryRejectsTamperSkipAndWrapperCommand(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		run := writeValidRunDirectory(t, false)
		if err := ValidateRunDirectory(run, FormalClass); err != nil {
			t.Fatalf("validate A62 run: %v", err)
		}
	})

	t.Run("artifact tamper", func(t *testing.T) {
		run := writeValidRunDirectory(t, false)
		path := filepath.Join(run, "a62-report.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		payload = []byte(strings.Replace(string(payload), strings.Repeat("b", 64), strings.Repeat("d", 64), 1))
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
			t.Fatalf("tampered A62 artifact error=%v, want SHA-256 mismatch", err)
		}
	})

	t.Run("skip event", func(t *testing.T) {
		run := writeValidRunDirectory(t, true)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "summary contract") {
			t.Fatalf("skip event error=%v, want summary contract failure", err)
		}
	})

	t.Run("no tests", func(t *testing.T) {
		run := writeValidRunDirectory(t, false)
		var summary testSummary
		readJSONTestFile(t, filepath.Join(run, "a62-test-summary.json"), &summary)
		summary.NoTests = true
		writeJSONTestFile(t, filepath.Join(run, "a62-test-summary.json"), summary)
		writeInventoryTestFile(t, run)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "summary contract") {
			t.Fatalf("no-tests error=%v, want summary contract failure", err)
		}
	})

	t.Run("wrapper command", func(t *testing.T) {
		run := writeValidRunDirectory(t, false)
		var evidence wrapperEvidence
		readJSONTestFile(t, filepath.Join(run, "evidence.json"), &evidence)
		evidence.Command = append(evidence.Command, "-Unexpected")
		writeJSONTestFile(t, filepath.Join(run, "evidence.json"), evidence)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "wrapper evidence") {
			t.Fatalf("wrapper command error=%v, want wrapper evidence failure", err)
		}
	})
}

func writeValidRunDirectory(t *testing.T, withSkip bool) string {
	t.Helper()
	run := t.TempDir()
	tableFirst := retentionTableResult{
		Scanned: 5203, Deleted: 5201, SkippedUnfinalized: 2, SkippedMissingHourly: 1,
		SkippedDailyNotFinal: 1, PendingRows: true, Batches: 21,
	}
	first := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540, Instance: tableFirst, Site: tableFirst}
	tableSecond := retentionTableResult{Scanned: 2, Deleted: 2, Batches: 1, Complete: true}
	second := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540, Instance: tableSecond, Site: tableSecond, Complete: true}
	tableEmpty := retentionTableResult{Batches: 1, Complete: true}
	idempotent := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540, Instance: tableEmpty, Site: tableEmpty, Complete: true}
	starvationFirstTable := retentionTableResult{
		Scanned: 17, Deleted: 1, SkippedUnfinalized: 16, SkippedMissingHourly: 16,
		SkippedDailyNotFinal: 16, PendingRows: true, BlockedDiagnosticsTruncated: true, Batches: 1,
	}
	starvationFirst := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540,
		Instance: starvationFirstTable, Site: starvationFirstTable}
	starvationRestartTable := retentionTableResult{Scanned: 51, Deleted: 51, PendingRows: true, Batches: 3}
	starvationRestart := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540,
		Instance: starvationRestartTable, Site: starvationRestartTable}
	starvationFinalTable := retentionTableResult{Scanned: 1, Deleted: 1, Batches: 1, Complete: true}
	starvationFinal := retentionResult{RetentionDays: 90, Cutoff: 1_760_889_540,
		Instance: starvationFinalTable, Site: starvationFinalTable, Complete: true}
	report := finalReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed",
		FixturePath: "testdata/design/f05-ops-capacity.yaml", FixtureSHA256: strings.Repeat("a", 64),
		FixedNowUnix: 1_768_665_599, RetentionDays: 90, Cutoff: 1_760_889_540, BatchSize: 257,
		InitialRowsPerTable: 5205, FirstRun: first, SecondRun: second, IdempotentRun: idempotent,
		RowsAfterFirstRun: 4, RowsAfterFinalRun: 2, ProtectedAggregateSHA256: strings.Repeat("b", 64),
		BusinessFactsPreserved: true, ExactBoundaryPreserved: true, MissingHourlyBlocked: true,
		DailyNotFinalBlocked: true, InvalidRetentionRejected: true, HourlyDailyValuesUnchanged: true,
		StarvationProof: starvationProof{
			BlockedPrefixRows: 52, BatchSize: 17, MaximumBatches: 3,
			FirstRun: starvationFirst, RestartRun: starvationRestart, FinalRun: starvationFinal,
			EligibleDeletedBehindBlockedPrefix: true, RestartContinuationProved: true,
		},
	}
	writeJSONTestFile(t, filepath.Join(run, "a62-report.json"), report)

	events := []goTestEvent{
		{Action: "start", Package: "new-api-pilot/tests/integration"},
		{Action: "run", Package: "new-api-pilot/tests/integration", Test: "TestA62ResourceMinuteRetention"},
		{Action: "pass", Package: "new-api-pilot/tests/integration", Test: "TestA62ResourceMinuteRetention"},
		{Action: "pass", Package: "new-api-pilot/tests/integration"},
	}
	if withSkip {
		events = append(events, goTestEvent{Action: "skip", Package: "new-api-pilot/tests/integration", Test: "TestA62ResourceMinuteRetention"})
	}
	var stream strings.Builder
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		stream.Write(payload)
		stream.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(run, "a62-test.jsonl"), []byte(stream.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "a62-test.stderr.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	summary := testSummary{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", TargetTest: "TestA62ResourceMinuteRetention",
		Package: "new-api-pilot/tests/integration", PassEvents: 1, JSONLines: len(events),
		JSONPath: "a62-test.jsonl", StderrPath: "a62-test.stderr.log",
	}
	if withSkip {
		summary.SkipEvents = 1
	}
	writeJSONTestFile(t, filepath.Join(run, "a62-test-summary.json"), summary)
	writeJSONTestFile(t, filepath.Join(run, "a62-command.json"), commandReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass,
		TargetTest: "TestA62ResourceMinuteRetention", WorkingDirectory: "/workspace", Command: innerCommand,
		GoImage: "golang:1.25.1", MySQLImage: "mysql:8.4",
	})
	environment := environmentReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass,
		Commit: "unborn", WorktreeDirty: true, IsolatedGuard: true, Database: "pilot_a62",
	}
	environment.MySQL.Version = "8.4.10"
	environment.MySQL.TransactionIsolation = "READ-COMMITTED"
	environment.MySQL.CharacterSetServer = "utf8mb4"
	environment.MySQL.CollationServer = "utf8mb4_unicode_ci"
	environment.MySQL.TimeZone = "+08:00"
	environment.Network.Internal = true
	environment.Network.HostPorts = []string{}
	writeJSONTestFile(t, filepath.Join(run, "a62-environment.json"), environment)
	writeJSONTestFile(t, filepath.Join(run, "a62-fixture.json"), fixtureReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, FixtureID: "F05",
		Path: report.FixturePath, SHA256: report.FixtureSHA256, FixedNowUnix: report.FixedNowUnix, RetentionDays: 90,
	})
	cleanup := cleanupReport{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass, Passed: true, SweepsSucceeded: true}
	cleanup.Lifecycle.Containers = "created_and_removed"
	cleanup.Lifecycle.Networks = "created_and_removed"
	cleanup.Lifecycle.Volumes = "not_created"
	cleanup.Residuals.Containers = []string{}
	cleanup.Residuals.Networks = []string{}
	cleanup.Residuals.Volumes = []string{}
	cleanup.Residuals.Images = []string{}
	writeJSONTestFile(t, filepath.Join(run, "a62-cleanup.json"), cleanup)
	writeInventoryTestFile(t, run)
	if err := os.WriteFile(filepath.Join(run, "stdout.log"), []byte("A62 test passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "stderr.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	writeJSONTestFile(t, filepath.Join(run, "evidence.json"), wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: FormalClass,
		Command: canonicalCommand, WorkingDirectory: ".", StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: started.Add(time.Second).Format(time.RFC3339Nano), DurationMilliseconds: 1000,
		ExitCode: 0, Commit: "unborn", WorktreeDirty: true,
		FixtureManifestPath: "testdata/design/manifest.sha256", FixtureManifestSHA: strings.Repeat("c", 64),
		StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	return run
}

func writeInventoryTestFile(t *testing.T, run string) {
	t.Helper()
	inventory := artifactInventory{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: FormalClass}
	for _, name := range requiredArtifacts {
		path := filepath.Join(run, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		inventory.Files = append(inventory.Files, artifactEntry{Path: name, SizeBytes: info.Size(), SHA256: digest})
	}
	writeJSONTestFile(t, filepath.Join(run, "a62-artifacts.json"), inventory)
}

func writeJSONTestFile(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONTestFile(t *testing.T, path string, target any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatal(err)
	}
}
