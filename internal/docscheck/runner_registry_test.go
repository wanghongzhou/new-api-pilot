package docscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalRunnerStaticGateRequiresRegisteredFile(t *testing.T) {
	root := t.TempDir()
	current := &checker{root: root}
	current.checkCanonicalAcceptanceRunner("manifest.yaml", "A01")
	if !containsIssue(current.issues, "canonical runner is missing") {
		t.Fatalf("missing runner was accepted: %#v", current.issues)
	}

	runnerPath := filepath.Join(root, "scripts", "acceptance", "run-generic-case.ps1")
	if err := os.MkdirAll(filepath.Dir(runnerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runnerPath, "# canonical runner\n")
	current = &checker{root: root}
	current.checkCanonicalAcceptanceRunner("manifest.yaml", "A01")
	if len(current.issues) != 0 {
		t.Fatalf("registered runner produced issues: %#v", current.issues)
	}
}

func TestCanonicalRunnerStaticGateRejectsUnknownCase(t *testing.T) {
	current := &checker{root: t.TempDir()}
	current.checkCanonicalAcceptanceRunner("manifest.yaml", "A103")
	if !containsIssue(current.issues, "no canonical runner registry entry") {
		t.Fatalf("unknown case was accepted: %#v", current.issues)
	}
}

func TestControlledOperationsEvidenceUsesDedicatedValidator(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "artifacts", "acceptance", "A52")
	if err := os.MkdirAll(evidencePath, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A52", "evidence_path", "artifacts/acceptance/A52/", true)
	if !containsIssue(current.issues, "no valid controlled formal run") {
		t.Fatalf("A52 bypassed its dedicated evidence validator: %#v", current.issues)
	}
}
