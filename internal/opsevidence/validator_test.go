package opsevidence

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateCanonicalCommandRequiresExactA51FormalRunner(t *testing.T) {
	command := []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a51.ps1",
	}
	if err := ValidateCanonicalCommand("A51", command, FormalClass); err != nil {
		t.Fatalf("canonical A51 command was rejected: %v", err)
	}
	if err := ValidateCanonicalCommand("A51", command, SmokeClass); err == nil {
		t.Fatal("A51 smoke evidence was accepted")
	}
	changed := append([]string(nil), command...)
	changed[len(changed)-1] = "scripts/acceptance/other.ps1"
	if err := ValidateCanonicalCommand("A51", changed, FormalClass); err == nil {
		t.Fatal("non-canonical A51 command was accepted")
	}
}

func TestValidateA51WrapperLogsRejectsSecretsInBothStreams(t *testing.T) {
	base64Key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	payloads := map[string]string{
		"fixed plaintext":   "a51-site-token-alpha-never-log",
		"database dsn":      "DATABASE_DSN=root:@tcp(mysql-a51:3306)/pilot_a51?charset=utf8mb4",
		"secret assignment": "OLD_ENCRYPTION_KEY=not-redacted",
		"base64 key":        "leaked " + base64Key + " value",
		"full key id":       strings.Repeat("a", 64),
		"url credential":    "https://example.invalid/hook?token=leaked",
	}
	for label, payload := range payloads {
		for _, logName := range []string{"stdout.log", "stderr.log"} {
			t.Run(label+"/"+logName, func(t *testing.T) {
				runDirectory := writeValidA51RunDirectory(t)
				if err := os.WriteFile(filepath.Join(runDirectory, logName), []byte(payload), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := ValidateWrapperLogs(runDirectory, "A51"); err == nil {
					t.Fatalf("%s secret in %s was accepted", label, logName)
				}
			})
		}
	}

	runDirectory := writeValidA51RunDirectory(t)
	redacted := []byte("ENCRYPTION_KEY=[redacted]\nDATABASE_DSN=[redacted]\n")
	if err := os.WriteFile(filepath.Join(runDirectory, "stderr.log"), redacted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrapperLogs(runDirectory, "A51"); err != nil {
		t.Fatalf("redacted wrapper diagnostics were rejected: %v", err)
	}
}

func TestValidateA51RunDirectoryRejectsTamperingAndResiduals(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		runDirectory := writeValidA51RunDirectory(t)
		if err := ValidateRunDirectory(runDirectory, "A51", FormalClass); err != nil {
			t.Fatalf("valid A51 evidence was rejected: %v", err)
		}
	})

	t.Run("artifact hash tamper", func(t *testing.T) {
		runDirectory := writeValidA51RunDirectory(t)
		path := filepath.Join(runDirectory, "a51-seed.log")
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		payload[0] ^= 0xff
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		err = ValidateInnerArtifacts(runDirectory, "A51", FormalClass)
		if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
			t.Fatalf("artifact tamper error=%v, want SHA-256 mismatch", err)
		}
	})

	t.Run("cleanup residual", func(t *testing.T) {
		runDirectory := writeValidA51RunDirectory(t)
		cleanup := validA51CleanupReport()
		cleanup.Residuals.Containers = []string{"a51-residual"}
		writeJSONTestFile(t, filepath.Join(runDirectory, "a51-cleanup.json"), cleanup)
		err := ValidateInnerArtifacts(runDirectory, "A51", FormalClass)
		if err == nil || !strings.Contains(err.Error(), "cleanup report contract") {
			t.Fatalf("cleanup residual error=%v, want cleanup contract failure", err)
		}
	})

	t.Run("resources not created", func(t *testing.T) {
		runDirectory := writeValidA51RunDirectory(t)
		cleanup := validA51CleanupReport()
		cleanup.Lifecycle.Networks = "not_created"
		writeJSONTestFile(t, filepath.Join(runDirectory, "a51-cleanup.json"), cleanup)
		err := ValidateInnerArtifacts(runDirectory, "A51", FormalClass)
		if err == nil || !strings.Contains(err.Error(), "cleanup report contract") {
			t.Fatalf("not-created lifecycle error=%v, want cleanup contract failure", err)
		}
	})

	t.Run("wrapper command", func(t *testing.T) {
		runDirectory := writeValidA51RunDirectory(t)
		evidence := validA51WrapperEvidence()
		evidence.Command = append(evidence.Command, "-Unexpected")
		writeJSONTestFile(t, filepath.Join(runDirectory, "evidence.json"), evidence)
		err := ValidateRunDirectory(runDirectory, "A51", FormalClass)
		if err == nil || !strings.Contains(err.Error(), "canonical command") {
			t.Fatalf("wrapper command error=%v, want canonical command failure", err)
		}
	})
}

func writeValidA51RunDirectory(t *testing.T) string {
	t.Helper()
	runDirectory := t.TempDir()
	for _, path := range requiredArtifacts["A51"] {
		payload := []byte("artifact:" + path)
		if path == "a51-report.json" {
			payload = mustJSONTest(t, finalReport{
				SchemaVersion:      1,
				AcceptanceID:       "A51",
				Status:             "passed",
				Passed:             true,
				EvidenceClass:      FormalClass,
				AcceptanceEligible: true,
			})
		}
		if err := os.WriteFile(filepath.Join(runDirectory, path), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	inventory := artifactInventory{
		SchemaVersion: 1,
		AcceptanceID:  "A51",
		EvidenceClass: FormalClass,
		Files:         make([]artifactEntry, 0, len(requiredArtifacts["A51"])),
	}
	for _, path := range requiredArtifacts["A51"] {
		fullPath := filepath.Join(runDirectory, path)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(fullPath)
		if err != nil {
			t.Fatal(err)
		}
		inventory.Files = append(inventory.Files, artifactEntry{Path: path, SizeBytes: info.Size(), SHA256: digest})
	}
	writeJSONTestFile(t, filepath.Join(runDirectory, "a51-artifacts.json"), inventory)
	writeJSONTestFile(t, filepath.Join(runDirectory, "a51-cleanup.json"), validA51CleanupReport())
	writeJSONTestFile(t, filepath.Join(runDirectory, "evidence.json"), validA51WrapperEvidence())
	for _, path := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(runDirectory, path), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return runDirectory
}

func validA51CleanupReport() cleanupReport {
	report := cleanupReport{
		SchemaVersion:   1,
		AcceptanceID:    "A51",
		EvidenceClass:   FormalClass,
		Passed:          true,
		SweepsSucceeded: true,
		Residuals: struct {
			Containers []string `json:"containers"`
			Networks   []string `json:"networks"`
			Volumes    []string `json:"volumes"`
			Images     []string `json:"images"`
		}{
			Containers: []string{},
			Networks:   []string{},
			Volumes:    []string{},
			Images:     []string{},
		},
	}
	report.Lifecycle.Containers = "created_and_removed"
	report.Lifecycle.Networks = "created_and_removed"
	report.Lifecycle.Volumes = "created_and_removed"
	return report
}

func validA51WrapperEvidence() wrapperEvidence {
	started := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	finished := started.Add(1500 * time.Millisecond)
	return wrapperEvidence{
		SchemaVersion:        1,
		AcceptanceID:         "A51",
		Status:               "passed",
		EvidenceClass:        FormalClass,
		Command:              []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a51.ps1"},
		WorkingDirectory:     ".",
		StartedAt:            started.Format(time.RFC3339Nano),
		FinishedAt:           finished.Format(time.RFC3339Nano),
		DurationMilliseconds: 1500,
		ExitCode:             0,
		FixtureManifestPath:  "testdata/design/manifest.sha256",
		FixtureManifestSHA:   strings.Repeat("a", 64),
		StdoutLog:            "stdout.log",
		StderrLog:            "stderr.log",
		RequiredNoSkip:       true,
	}
}

func writeJSONTestFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.WriteFile(path, mustJSONTest(t, value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSONTest(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
