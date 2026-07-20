package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestA45WrapperClosesAndScansLogsBeforeInnerValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join")
	scanLogs := strings.Index(text, "a45evidence.ValidateWrapperLogs")
	innerValidation := strings.Index(text, "a45evidence.ValidateInnerArtifacts")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("A45 wrapper validation order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestA45WrapperRejectsArbitraryExitZeroCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"run", "-case", "A45", "--", "cmd.exe", "/c", "exit", "0"}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "canonical command") {
		t.Fatalf("A45 arbitrary command exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestA45RunnerUsesIsolatedReadOnlyResourcesAndExactInventory(t *testing.T) {
	payload, err := os.ReadFile("run-a45.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	cleanupWrite := strings.LastIndex(text, "Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a45-cleanup.json')")
	secretScan := strings.LastIndex(text, "Write-A45SecretScan -Directory $evidenceDirectory")
	inventoryWrite := strings.LastIndex(text, "Write-A45ArtifactInventory -Directory $evidenceDirectory")
	if cleanupWrite < 0 || secretScan < 0 || inventoryWrite < 0 || cleanupWrite >= secretScan || secretScan >= inventoryWrite {
		t.Fatalf("A45 cleanup/secret/inventory order is invalid: cleanup=%d secret=%d inventory=%d", cleanupWrite, secretScan, inventoryWrite)
	}
	for _, required := range []string{
		"--internal", "target=/workspace,readonly", "--read-only", "--cap-drop", "ALL",
		"no-new-privileges:true", "GOPROXY=off", "GOSUMDB=off", "HTTP_PROXY=", "http_proxy=",
		"/tmp:rw,exec,nosuid,nodev,size=536870912",
		"new-api-pilot-a45-$runToken-network", "new-api-pilot-a45-$runToken-gomod", "new-api-pilot-a45-$runToken-gobuild",
		"a45-test.jsonl", "a45-test.stderr.log", "a45-test-summary.json", "a45-command.json",
		"a45-environment.json", "a45-fixture.json", "a45-report.json", "a45-cleanup.json",
		"a45-secret-scan.json", "a45-artifacts.json", "Get-OpsResidualSweep",
		"if ($files.Count -ne 9)", "evidence_mounted_in_test = $false", "host_ports = @()",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A45 runner is missing evidence/isolation contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"docker system prune", "docker container prune", "docker volume prune", "docker network prune",
		"new-api-pilot-go-mod-cache", "new-api-pilot-go-build-cache", "--publish", "-p 8080",
		"target=/evidence",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("A45 runner contains forbidden shared/destructive/network operation %q", forbidden)
		}
	}
}
