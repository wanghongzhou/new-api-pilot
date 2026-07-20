package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestA22WrapperClosesAndScansLogsBeforeInnerValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join")
	scanLogs := strings.Index(text, "a22evidence.ValidateWrapperLogs")
	innerValidation := strings.Index(text, "a22evidence.ValidateInnerArtifacts")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("A22 wrapper validation order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestA22WrapperRejectsArbitraryExitZeroCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"run", "-case", "A22", "--", "cmd.exe", "/c", "exit", "0"}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "canonical command") {
		t.Fatalf("A22 arbitrary command exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestA22RunnerUsesIsolatedBoundedResourcesAndFinalInventory(t *testing.T) {
	payload, err := os.ReadFile("run-a22.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	cleanupWrite := strings.LastIndex(text, "Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a22-cleanup.json')")
	inventoryWrite := strings.LastIndex(text, "Write-A22ArtifactInventory -Directory $evidenceDirectory")
	if cleanupWrite < 0 || inventoryWrite < 0 || cleanupWrite >= inventoryWrite {
		t.Fatalf("A22 cleanup/inventory order is invalid: cleanup=%d inventory=%d", cleanupWrite, inventoryWrite)
	}
	for _, required := range []string{
		"mysql:8.4", "golang:1.25.1", "--internal", "target=/workspace,readonly", "target=/evidence",
		"a22-negative-manifest.json", "a22-negative-target-mismatch.json", "a22-verify-restore.json",
		"a22-app-smoke.json", "a22-rpo-rto.json", "a22-secret-scan.json", "a22-cleanup.json",
		"Get-OpsResidualSweep", "A22_DEVELOPMENT", "production_release_authorized = $false",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A22 runner is missing evidence contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"docker system prune", "docker container prune", "docker volume prune", "docker network prune",
		"production_release_authorized = $true", "SESSION_COOKIE_SECURE=true",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("A22 runner contains forbidden operation or release authorization %q", forbidden)
		}
	}
}

func TestA22ApplicationSmokeRequiresCookieUserHeaderAndTargetIdentity(t *testing.T) {
	payload, err := os.ReadFile("a22-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, required := range []string{
		"--cookie-jar", "--cookie", ".data.id", "New-Api-User: $self_user_id",
		"A22_SOURCE_UUID_FINGERPRINT", "A22_TARGET_UUID_FINGERPRINT", "observed_fingerprint",
		"connected_to_target=true", "production_release_authorized:false",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A22 application smoke is missing contract %q", required)
		}
	}
}
