package docscheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"new-api-pilot/internal/acceptancecatalog"
)

const testFormalCommit = "0123456789abcdef0123456789abcdef01234567"

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

	current := newFinalEvidenceChecker(root)
	current.checkPlannedPath("manifest.yaml", "A01", "evidence_path", "artifacts/acceptance/A01/", true)
	if len(current.issues) != 0 {
		t.Fatalf("valid formal runner evidence produced issues: %#v", current.issues)
	}

	recordPath := filepath.Join(evidenceRoot, "run-1", "evidence.json")
	record := readFormalEvidenceRecord(t, recordPath)
	record.EvidenceClass = ""
	writeFormalEvidenceRecord(t, recordPath, record)
	current = newFinalEvidenceChecker(root)
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

	current := newFinalEvidenceChecker(root)
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

	current := newFinalEvidenceChecker(root)
	current.checkPlannedPath("manifest.yaml", "A03", "evidence_path", "artifacts/acceptance/A03/", true)
	if !containsIssue(current.issues, "invalid evidence duration") {
		t.Fatalf("inconsistent duration was accepted: %#v", current.issues)
	}

	record = readFormalEvidenceRecord(t, recordPath)
	record.DurationMilliseconds--
	record.StdoutLog = record.StderrLog
	writeFormalEvidenceRecord(t, recordPath, record)
	current = newFinalEvidenceChecker(root)
	current.checkPlannedPath("manifest.yaml", "A03", "evidence_path", "artifacts/acceptance/A03/", true)
	if !containsIssue(current.issues, "wrapper log names must be distinct") {
		t.Fatalf("aliased logs were accepted: %#v", current.issues)
	}
}

func TestFinalEvidenceGateRejectsArbitrarySuccessfulCommand(t *testing.T) {
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
	record.Command = []string{"powershell.exe", "-NoProfile", "-Command", "exit 0"}
	writeFormalEvidenceRecord(t, recordPath, record)

	current := newFinalEvidenceChecker(root)
	current.checkFormalEvidenceRoot("manifest.yaml", "A03", evidenceRoot)
	if !containsIssue(current.issues, "wrapper command is not canonical") {
		t.Fatalf("arbitrary successful command was accepted: %#v", current.issues)
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

	current := newFinalEvidenceChecker(root)
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
	runner, ok := acceptancecatalog.Lookup(acceptanceID)
	if !ok {
		t.Fatalf("missing canonical runner for %s", acceptanceID)
	}
	writeFormalEvidenceRecord(t, filepath.Join(run, "evidence.json"), formalEvidenceRecord{
		SchemaVersion: 1, AcceptanceID: acceptanceID, Status: "passed", EvidenceClass: evidenceClass,
		Command: runner.Command, WorkingDirectory: ".",
		StartedAt: now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
		DurationMilliseconds: 1000, ExitCode: 0, Commit: testFormalCommit, FixtureManifestPath: "testdata/design/manifest.sha256",
		FixtureManifestSHA: fixtureSHA, StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	writeTestFile(t, filepath.Join(run, "stdout.log"), "test output\n")
	writeTestFile(t, filepath.Join(run, "stderr.log"), "")
	if acceptancecatalog.UsesGenericEvidenceContract(acceptanceID) {
		files := make([]genericEvidenceEntry, 0, 3)
		for _, name := range []string{"evidence.json", "stderr.log", "stdout.log"} {
			path := filepath.Join(run, name)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := hashFile(path)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, genericEvidenceEntry{Path: name, SizeBytes: info.Size(), SHA256: digest})
		}
		manifest := genericEvidenceManifest{SchemaVersion: 1, AcceptanceID: acceptanceID, EvidenceClass: evidenceClass, FixtureManifestSHA: fixtureSHA, Files: files}
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(run, "run-manifest.json"), string(payload)+"\n")
		manifestDigest, err := hashFile(filepath.Join(run, "run-manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var checksums strings.Builder
		for _, entry := range files {
			checksums.WriteString(entry.SHA256 + "  " + entry.Path + "\n")
		}
		checksums.WriteString(manifestDigest + "  run-manifest.json\n")
		writeTestFile(t, filepath.Join(run, "checksums.sha256"), checksums.String())
	}
}

func TestFinalEvidenceGateRejectsTamperedOrExtraGenericArtifacts(t *testing.T) {
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
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A01")
	writeFormalEvidenceRun(t, evidenceRoot, "A01", fixtureSHA, formalEvidenceClass)
	run := filepath.Join(evidenceRoot, "run-1")
	writeTestFile(t, filepath.Join(run, "stdout.log"), "evil output\n")
	current := newFinalEvidenceChecker(root)
	current.checkFormalEvidenceRoot("manifest.yaml", "A01", evidenceRoot)
	if !containsIssue(current.issues, "checksum mismatch") {
		t.Fatalf("tampered generic artifact was accepted: %#v", current.issues)
	}

	writeFormalEvidenceRun(t, evidenceRoot, "A01", fixtureSHA, formalEvidenceClass)
	writeTestFile(t, filepath.Join(run, "unexpected.txt"), "not inventoried\n")
	current = newFinalEvidenceChecker(root)
	current.checkFormalEvidenceRoot("manifest.yaml", "A01", evidenceRoot)
	if !containsIssue(current.issues, "unexpected file") {
		t.Fatalf("extra generic artifact was accepted: %#v", current.issues)
	}
}

func TestFinalEvidenceGateRejectsDirtyAndDifferentCommitEvidence(t *testing.T) {
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
	evidenceRoot := filepath.Join(root, "artifacts", "acceptance", "A05")
	writeFormalEvidenceRun(t, evidenceRoot, "A05", fixtureSHA, formalEvidenceClass)
	recordPath := filepath.Join(evidenceRoot, "run-1", "evidence.json")
	record := readFormalEvidenceRecord(t, recordPath)
	record.WorktreeDirty = true
	writeFormalEvidenceRecord(t, recordPath, record)

	current := newFinalEvidenceChecker(root)
	current.checkFormalEvidenceRoot("manifest.yaml", "A05", evidenceRoot)
	if !containsIssue(current.issues, "evidence was produced from a dirty worktree") {
		t.Fatalf("dirty evidence was accepted: %#v", current.issues)
	}

	record.WorktreeDirty = false
	record.Commit = "abcdef0123456789abcdef0123456789abcdef01"
	writeFormalEvidenceRecord(t, recordPath, record)
	current = newFinalEvidenceChecker(root)
	current.checkFormalEvidenceRoot("manifest.yaml", "A05", evidenceRoot)
	if !containsIssue(current.issues, "evidence commit does not match the current candidate commit") {
		t.Fatalf("evidence for another commit was accepted: %#v", current.issues)
	}
}

func TestEvidenceCommitAllowsOnlySingleManifestCloseoutCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable in this test image")
	}
	t.Run("valid", func(t *testing.T) {
		root, parent, current := createCloseoutRepository(t, "artifacts/acceptance/A49/", false)
		if err := validateEvidenceCommit(root, "A49", parent, current); err != nil {
			t.Fatalf("valid closeout rejected: %v", err)
		}
	})
	t.Run("unrelated-file", func(t *testing.T) {
		root, parent, current := createCloseoutRepository(t, "artifacts/acceptance/A49/", true)
		if err := validateEvidenceCommit(root, "A49", parent, current); err == nil || !strings.Contains(err.Error(), "changed files other than") {
			t.Fatalf("closeout with unrelated file was accepted: %v", err)
		}
	})
	t.Run("rewritten-path", func(t *testing.T) {
		root, parent, current := createCloseoutRepository(t, "artifacts/acceptance/other/", false)
		if err := validateEvidenceCommit(root, "A49", parent, current); err == nil || !strings.Contains(err.Error(), "other than removing planned") {
			t.Fatalf("rewritten evidence path was accepted: %v", err)
		}
	})
	t.Run("not-target-case", func(t *testing.T) {
		root, parent, current := createCloseoutRepository(t, "artifacts/acceptance/A49/", false)
		if err := validateEvidenceCommit(root, "A52", parent, current); err == nil || !strings.Contains(err.Error(), "was not finalized") {
			t.Fatalf("evidence for a case not finalized was accepted: %v", err)
		}
	})
	t.Run("not-direct-parent", func(t *testing.T) {
		root, parent, _ := createCloseoutRepository(t, "artifacts/acceptance/A49/", false)
		writeTestFile(t, filepath.Join(root, "later.txt"), "later\n")
		runGitTest(t, root, "add", "later.txt")
		runGitTest(t, root, "commit", "-m", "later")
		current := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
		if err := validateEvidenceCommit(root, "A49", parent, current); err == nil || !strings.Contains(err.Error(), "sole direct parent") {
			t.Fatalf("non-direct ancestor evidence was accepted: %v", err)
		}
	})
}

func TestCloseoutRequiresGenericAndSpecializedContractsInSameRun(t *testing.T) {
	for _, acceptanceID := range []string{"A49", "A52", "A74", "A75"} {
		t.Run(acceptanceID, func(t *testing.T) {
			if err := validateCloseoutSpecializedRun(t.TempDir(), acceptanceID); err == nil {
				t.Fatal("generic-only run bypassed the specialized evidence contract")
			}
		})
	}
}

func createCloseoutRepository(t *testing.T, finalizedPath string, addUnrelated bool) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init")
	runGitTest(t, root, "config", "user.email", "acceptance@example.invalid")
	runGitTest(t, root, "config", "user.name", "Acceptance Test")
	manifestPath := filepath.Join(root, filepath.FromSlash(acceptanceManifestPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, manifestPath, closeoutManifest("planned:artifacts/acceptance/A49/"))
	runGitTest(t, root, "add", acceptanceManifestPath)
	runGitTest(t, root, "commit", "-m", "candidate")
	parent := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	writeTestFile(t, manifestPath, closeoutManifest(finalizedPath))
	if addUnrelated {
		writeTestFile(t, filepath.Join(root, "unrelated.txt"), "changed\n")
		runGitTest(t, root, "add", "unrelated.txt")
	}
	runGitTest(t, root, "add", acceptanceManifestPath)
	runGitTest(t, root, "commit", "-m", "acceptance closeout")
	current := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	return root, parent, current
}

func closeoutManifest(evidencePath string) string {
	return "schema_version: 1\n" +
		"baseline:\n" +
		"  source: docs/design.md\n" +
		"  acceptance_range: A01-A102\n" +
		"  release_policy: required-no-skip\n" +
		"  product_locale: zh-CN\n" +
		"  fixture_checksum_manifest: testdata/design/manifest.sha256\n" +
		"fixtures: {}\n" +
		"acceptance_cases:\n" +
		"  - acceptance_id: A49\n" +
		"    requirement_id: R05\n" +
		"    fixture: [F05]\n" +
		"    layer: operations\n" +
		"    test_or_runbook_path: docs/acceptance/runbooks/capacity-performance.md\n" +
		"    owner_role: platform-operator\n" +
		"    evidence_path: " + evidencePath + "\n"
}

func runGitTest(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	payload, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, payload)
	}
	return string(payload)
}

func TestFinalEvidenceGateRejectsDirtyCurrentWorktree(t *testing.T) {
	clean := false
	current := &checker{root: t.TempDir(), options: Options{
		RequireNoPlanned: true, ExpectedGitCommit: testFormalCommit, ExpectedWorktreeClean: &clean,
	}}
	current.initializeFinalRepositoryState()
	if !containsIssue(current.issues, "current worktree is dirty") {
		t.Fatalf("dirty current worktree was accepted: %#v", current.issues)
	}
}

func newFinalEvidenceChecker(root string) *checker {
	return &checker{
		root: root, options: Options{RequireNoPlanned: true}, expectedCommit: testFormalCommit,
		worktreeClean: true, repositoryStateReady: true,
	}
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
