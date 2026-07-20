package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFinalEvidenceGateRequiresCurrentFormalRunnerEvidence(t *testing.T) {
	root := t.TempDir()
	fixtureManifest := filepath.Join(root, "testdata", "design", "manifest.sha256")
	if err := os.MkdirAll(filepath.Dir(fixtureManifest), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixtureManifest, "# fixture_manifest_version=1\n")
	fixtureSHA, err := hashFile(fixtureManifest)
	if err != nil {
		t.Fatal(err)
	}
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A01")
	writeFormalEvidenceRun(t, evidenceRoot, "A01", fixtureSHA, formalEvidenceClass)

	current := &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A01", "evidence_path", "artifacts/acceptance/A01/", true)
	if len(current.issues) != 0 {
		t.Fatalf("valid formal runner evidence produced issues: %#v", current.issues)
	}

	recordPath := filepath.Join(evidenceRoot, "run-1", "evidence.json")
	record := readFormalEvidenceRecord(t, recordPath)
	record.EvidenceClass = ""
	writeFormalEvidenceRecord(t, recordPath, record)
	current = &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A01", "evidence_path", "artifacts/acceptance/A01/", true)
	if len(current.issues) != 1 || current.issues[0].Check != "evidence" {
		t.Fatalf("non-formal evidence was accepted in final mode: %#v", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A01", "evidence_path", "artifacts/acceptance/A01/", true)
	if len(current.issues) != 0 {
		t.Fatalf("normal docs-check unexpectedly required formal evidence: %#v", current.issues)
	}
}

func TestFinalEvidenceGateRejectsStaleFixtureManifestAndEmptyLogs(t *testing.T) {
	root := t.TempDir()
	fixtureManifest := filepath.Join(root, "testdata", "design", "manifest.sha256")
	if err := os.MkdirAll(filepath.Dir(fixtureManifest), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixtureManifest, "current\n")
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A02")
	writeFormalEvidenceRun(t, evidenceRoot, "A02", "stale", formalEvidenceClass)
	for _, name := range []string{"stdout.log", "stderr.log"} {
		writeTestFile(t, filepath.Join(evidenceRoot, "run-1", name), "")
	}

	current := &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A02", "evidence_path", "artifacts/acceptance/A02/", true)
	if len(current.issues) != 1 || current.issues[0].Check != "evidence" {
		t.Fatalf("stale fixture/empty log evidence was accepted: %#v", current.issues)
	}
}

func TestFinalEvidenceGateRejectsInconsistentDurationAndLogAliases(t *testing.T) {
	root := t.TempDir()
	fixtureManifest := filepath.Join(root, "testdata", "design", "manifest.sha256")
	if err := os.MkdirAll(filepath.Dir(fixtureManifest), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixtureManifest, fixtureManifestHeader+"\n")
	fixtureSHA, err := hashFile(fixtureManifest)
	if err != nil {
		t.Fatal(err)
	}
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A03")
	writeFormalEvidenceRun(t, evidenceRoot, "A03", fixtureSHA, formalEvidenceClass)
	recordPath := filepath.Join(evidenceRoot, "run-1", "evidence.json")
	record := readFormalEvidenceRecord(t, recordPath)
	record.DurationMilliseconds++
	writeFormalEvidenceRecord(t, recordPath, record)

	current := &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A03", "evidence_path", "artifacts/acceptance/A03/", true)
	if !containsIssue(current.issues, "invalid evidence duration") {
		t.Fatalf("inconsistent duration was accepted: %#v", current.issues)
	}

	record = readFormalEvidenceRecord(t, recordPath)
	record.DurationMilliseconds--
	record.StdoutLog = record.StderrLog
	writeFormalEvidenceRecord(t, recordPath, record)
	current = &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A03", "evidence_path", "artifacts/acceptance/A03/", true)
	if !containsIssue(current.issues, "wrapper log names must be distinct") {
		t.Fatalf("aliased logs were accepted: %#v", current.issues)
	}
}

func TestFinalEvidenceGateRejectsSymlinkedEvidenceRoot(t *testing.T) {
	root := t.TempDir()
	fixtureManifest := filepath.Join(root, "testdata", "design", "manifest.sha256")
	if err := os.MkdirAll(filepath.Dir(fixtureManifest), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixtureManifest, fixtureManifestHeader+"\n")
	fixtureSHA, err := hashFile(fixtureManifest)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "evidence-target")
	writeFormalEvidenceRun(t, target, "A04", fixtureSHA, formalEvidenceClass)
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A04")
	if err := os.MkdirAll(filepath.Dir(evidenceRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, evidenceRoot); err != nil {
		t.Skipf("cannot create test symlink: %v", err)
	}

	current := &checker{root: root, options: Options{RequireNoPlanned: true}}
	current.checkPlannedPath("manifest.yaml", "A04", "evidence_path", "artifacts/acceptance/A04/", true)
	if !containsIssue(current.issues, "evidence path must be a real directory") {
		t.Fatalf("symlinked evidence root was accepted: %#v", current.issues)
	}
}

func writeFormalEvidenceRun(t *testing.T, root, acceptanceID, fixtureSHA, evidenceClass string) {
	t.Helper()
	run := filepath.Join(root, "run-1")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC)
	writeFormalEvidenceRecord(t, filepath.Join(run, "evidence.json"), formalEvidenceRecord{
		SchemaVersion: 1, AcceptanceID: acceptanceID, Status: "passed", EvidenceClass: evidenceClass,
		Command: []string{"go", "test", "./tests"}, WorkingDirectory: ".",
		StartedAt: now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
		DurationMilliseconds: 1000, ExitCode: 0, Commit: "unborn", FixtureManifestPath: "testdata/design/manifest.sha256",
		FixtureManifestSHA: fixtureSHA, StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	writeTestFile(t, filepath.Join(run, "stdout.log"), "test output\n")
	writeTestFile(t, filepath.Join(run, "stderr.log"), "")
}

func readFormalEvidenceRecord(t *testing.T, path string) formalEvidenceRecord {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record formalEvidenceRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func writeFormalEvidenceRecord(t *testing.T, path string, record formalEvidenceRecord) {
	t.Helper()
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}
