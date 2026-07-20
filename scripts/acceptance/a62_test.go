package main

import (
	"os"
	"strings"
	"testing"
)

func TestA62WrapperClosesLogsBeforeOuterValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join")
	scanLogs := strings.Index(text, "a62evidence.ValidateWrapperLogs")
	innerValidation := strings.Index(text, "a62evidence.ValidateInnerArtifacts")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("A62 wrapper validation order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestA62RunnerFinalizesCleanupBeforeFixedInventory(t *testing.T) {
	payload, err := os.ReadFile("run-a62.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	cleanupWrite := strings.LastIndex(text, "Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a62-cleanup.json')")
	inventoryWrite := strings.LastIndex(text, "Write-A62ArtifactInventory -Directory $evidenceDirectory")
	if cleanupWrite < 0 || inventoryWrite < 0 || cleanupWrite >= inventoryWrite {
		t.Fatalf("A62 cleanup/inventory order is invalid: cleanup=%d inventory=%d", cleanupWrite, inventoryWrite)
	}
	for _, required := range []string{
		"a62-test.jsonl", "a62-test.stderr.log", "a62-test-summary.json", "a62-command.json",
		"a62-environment.json", "a62-fixture.json", "a62-report.json", "a62-cleanup.json",
		"Get-OpsResidualSweep", "Write-OpsUtf8NoBom", "A62_DEVELOPMENT",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A62 runner is missing evidence contract %q", required)
		}
	}
}
