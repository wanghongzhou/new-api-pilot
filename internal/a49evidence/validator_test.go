package a49evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRunDirectory(t *testing.T) {
	t.Run("formal", func(t *testing.T) {
		runDirectory := newTestRunDirectory(t, FormalClass)
		if err := ValidateRunDirectory(runDirectory, FormalClass); err != nil {
			t.Fatalf("valid formal run rejected: %v", err)
		}
		if err := ValidateEvidenceRoot(filepath.Dir(runDirectory), FormalClass); err != nil {
			t.Fatalf("valid formal evidence root rejected: %v", err)
		}
	})
	t.Run("smoke", func(t *testing.T) {
		runDirectory := newTestRunDirectory(t, SmokeClass)
		if err := ValidateRunDirectory(runDirectory, SmokeClass); err != nil {
			t.Fatalf("valid smoke run rejected: %v", err)
		}
		if err := ValidateRunDirectory(runDirectory, FormalClass); err == nil {
			t.Fatal("smoke run was accepted as formal evidence")
		}
	})
	t.Run("artifact tamper", func(t *testing.T) {
		runDirectory := newTestRunDirectory(t, FormalClass)
		if err := os.WriteFile(filepath.Join(runDirectory, "a49-app.log"), []byte("tampered\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(runDirectory, FormalClass); err == nil {
			t.Fatal("artifact tamper was accepted")
		}
	})
	t.Run("cleanup residual", func(t *testing.T) {
		runDirectory := newTestRunDirectory(t, FormalClass)
		var cleanup cleanupReport
		if err := decodeJSONFile(filepath.Join(runDirectory, "a49-cleanup.json"), &cleanup); err != nil {
			t.Fatal(err)
		}
		cleanup.Residuals.Containers = []string{"left-behind-container"}
		writeTestJSON(t, filepath.Join(runDirectory, "a49-cleanup.json"), cleanup)
		if err := ValidateRunDirectory(runDirectory, FormalClass); err == nil {
			t.Fatal("cleanup residual was accepted")
		}
	})
	t.Run("noncanonical wrapper", func(t *testing.T) {
		runDirectory := newTestRunDirectory(t, FormalClass)
		var evidence wrapperEvidence
		if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
			t.Fatal(err)
		}
		evidence.Command = []string{"powershell.exe", "-Command", "exit 0"}
		writeTestJSON(t, filepath.Join(runDirectory, "evidence.json"), evidence)
		if err := ValidateRunDirectory(runDirectory, FormalClass); err == nil {
			t.Fatal("noncanonical wrapper command was accepted")
		}
	})
}

func TestValidateCanonicalCommand(t *testing.T) {
	formal := []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a49.ps1"}
	if err := ValidateCanonicalCommand(formal, FormalClass); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCanonicalCommand(append(append([]string(nil), formal...), "-Smoke"), SmokeClass); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCanonicalCommand([]string{"cmd.exe", "/c", "exit", "0"}, FormalClass); err == nil {
		t.Fatal("arbitrary exit-zero command was accepted")
	}
}

func newTestRunDirectory(t *testing.T, class string) string {
	t.Helper()
	root := t.TempDir()
	runDirectory := filepath.Join(root, "20260714T000000.000000000Z-1")
	if err := os.Mkdir(runDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	mode, eligible, err := classContract(class)
	if err != nil {
		t.Fatal(err)
	}
	status := "passed"
	if class == SmokeClass {
		status = "smoke_passed_not_acceptance_evidence"
	}
	for _, name := range requiredArtifacts {
		payload := []byte("test artifact " + name + "\n")
		if name == "a49-report.json" {
			payload, err = json.Marshal(finalReport{
				SchemaVersion: 1, AcceptanceID: "A49", Status: status, Passed: true,
				Mode: mode, EvidenceClass: class, AcceptanceEligible: eligible,
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(runDirectory, name), payload, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	inventory := artifactInventory{
		SchemaVersion: 1, AcceptanceID: "A49", EvidenceClass: class, Mode: mode,
		Files: make([]artifactEntry, 0, len(requiredArtifacts)),
	}
	for _, name := range requiredArtifacts {
		path := filepath.Join(runDirectory, name)
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
	writeTestJSON(t, filepath.Join(runDirectory, "a49-artifacts.json"), inventory)
	cleanup := cleanupReport{
		SchemaVersion: 1, AcceptanceID: "A49", EvidenceClass: class, Mode: mode,
		Passed: true, SweepsSucceeded: true,
	}
	cleanup.Residuals.Containers = []string{}
	cleanup.Residuals.Networks = []string{}
	cleanup.Residuals.Volumes = []string{}
	cleanup.Residuals.Images = []string{}
	writeTestJSON(t, filepath.Join(runDirectory, "a49-cleanup.json"), cleanup)
	if err := os.WriteFile(filepath.Join(runDirectory, "stdout.log"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDirectory, "stderr.log"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	command := []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a49.ps1"}
	if class == SmokeClass {
		command = append(command, "-Smoke")
	}
	evidence := wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: "A49", Status: "passed", EvidenceClass: class,
		Command: command, WorkingDirectory: ".", StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: started.Add(time.Second).Format(time.RFC3339Nano), DurationMilliseconds: 1000,
		ExitCode: 0, FixtureManifestPath: "testdata/design/manifest.sha256",
		FixtureManifestSHA: strings.Repeat("a", 64), StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	}
	writeTestJSON(t, filepath.Join(runDirectory, "evidence.json"), evidence)
	return runDirectory
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o640); err != nil {
		t.Fatal(err)
	}
}
