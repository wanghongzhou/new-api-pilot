package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestA50WrapperClosesAndScansLogsBeforeInnerValidation(t *testing.T) {
	payload, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	closeLogs := strings.Index(text, "logCloseError := errors.Join")
	scanLogs := strings.Index(text, "a50evidence.ValidateWrapperLogs")
	innerValidation := strings.Index(text, "a50evidence.ValidateInnerArtifacts")
	if closeLogs < 0 || scanLogs < 0 || innerValidation < 0 || closeLogs >= scanLogs || scanLogs >= innerValidation {
		t.Fatalf("A50 wrapper validation order is invalid: close=%d scan=%d inner=%d", closeLogs, scanLogs, innerValidation)
	}
}

func TestA50WrapperRejectsArbitraryExitZeroCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"run", "-case", "A50", "--", "cmd.exe", "/c", "exit", "0"}, &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "canonical command") {
		t.Fatalf("A50 arbitrary command exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestA50RunnerUsesIndependentPortExactMatrixAndFinalInventory(t *testing.T) {
	payload, err := os.ReadFile("run-a50.ps1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	cleanupWrite := strings.LastIndex(text, "Write-OpsUtf8NoBom -Path (Join-Path $evidenceDirectory 'a50-cleanup.json')")
	secretScan := strings.LastIndex(text, "Write-A50SecretScan -Directory $evidenceDirectory")
	inventoryWrite := strings.LastIndex(text, "Write-A50ArtifactInventory -Directory $evidenceDirectory")
	if cleanupWrite < 0 || secretScan < 0 || inventoryWrite < 0 || cleanupWrite >= secretScan || secretScan >= inventoryWrite {
		t.Fatalf("A50 cleanup/secret/inventory order is invalid: cleanup=%d secret=%d inventory=%d", cleanupWrite, secretScan, inventoryWrite)
	}
	for _, required := range []string{
		"Get-A50FreePort", "$port -ne 5173", "127.0.0.1", "CreateNoWindow = $true",
		"ProcessWindowStyle]::Hidden", "PLAYWRIGHT_BASE_URL", "PLAYWRIGHT_JSON_OUTPUT_FILE",
		"PLAYWRIGHT_HTML_OUTPUT_DIR", "--workers=2", "--retries=0", "--forbid-only",
		"--project=chromium-desktop", "--project=chromium-mobile", "--reporter=json,html",
		"Test-A50PortOpen", "taskkill.exe", "Remove-A50TemporaryDirectory", "Copy-Item -LiteralPath $htmlIndex",
		"a50-playwright.json", "a50-playwright-report.html", "a50-playwright.stdout.log",
		"a50-playwright.stderr.log", "a50-check.stdout.log", "a50-check.stderr.log",
		"a50-server.stdout.log", "a50-server.stderr.log", "a50-check-summary.json",
		"a50-command.json", "a50-environment.json", "a50-fixture.json", "a50-report.json",
		"a50-cleanup.json", "a50-secret-scan.json", "a50-artifacts.json",
		"$htmlSecretAllowlist.Count -ne 14", "[regex]::Matches($payload, $pattern)",
		"[System.StringComparer]::Ordinal", "$rawDSNPattern", "$urlCredentialPattern",
		"if ($files.Count -ne 15)", "38b71931dbce622dc82dbf9323836aa8ffcaf5aa475b8c87319864c3f750a40c",
		"bcceaaf7d6b171014258b9d935fbb1e7cab4585b49403d760a6db373e5aabe94",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("A50 runner is missing evidence/isolation contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"reuseExistingServer", "--port 5173", "http://127.0.0.1:5173", "Start-Process",
		"web/test-results", "e2e/statistics-states.spec.ts --workers=1",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("A50 runner contains forbidden shared/noncanonical operation %q", forbidden)
		}
	}
}
