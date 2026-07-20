package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestA25WrapperClosesAndScansLogsBeforeInnerValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join")
	scanLogs := strings.Index(text, "a25evidence.ValidateWrapperLogs")
	innerValidation := strings.Index(text, "a25evidence.ValidateInnerArtifacts")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("A25 wrapper validation order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestA25WrapperRejectsArbitraryExitZeroCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"run", "-case", "A25", "--", "cmd.exe", "/c", "exit", "0"}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "canonical command") {
		t.Fatalf("A25 arbitrary command exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestA25RunnerUsesUniqueBoundedResourcesAndFinalInventory(t *testing.T) {
	payload, err := os.ReadFile("run-a25.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	cleanupWrite := strings.LastIndex(text, "Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a25-cleanup.json')")
	inventoryWrite := strings.LastIndex(text, "Write-A25ArtifactInventory -Directory $evidenceDirectory")
	if cleanupWrite < 0 || inventoryWrite < 0 || cleanupWrite >= inventoryWrite {
		t.Fatalf("A25 cleanup/inventory order is invalid: cleanup=%d inventory=%d", cleanupWrite, inventoryWrite)
	}
	for _, required := range []string{
		"mysql:8.4", "mysql:5.7", "mariadb:10.11", "--internal", "GOPROXY=off", "GOSUMDB=off",
		"$runToken-gomod", "$runToken-gobuild", "target=/workspace,readonly", "target=/evidence",
		"a25-test.jsonl", "a25-test.stderr.log", "a25-test-summary.json", "a25-command.json",
		"a25-environment.json", "a25-fixture.json", "a25-report.json", "a25-cleanup.json",
		"Get-OpsResidualSweep", "A25_DEVELOPMENT", "Remove-OpsDockerResource",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A25 runner is missing contract %q", required)
		}
	}
	for _, forbidden := range []string{"docker system prune", "docker container prune", "docker volume prune", "docker network prune", "new-api-pilot-go-mod-cache", "new-api-pilot-go-build-cache"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("A25 runner contains forbidden shared/destructive operation %q", forbidden)
		}
	}
}
