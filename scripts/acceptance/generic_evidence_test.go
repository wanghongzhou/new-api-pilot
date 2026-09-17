package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"new-api-pilot/internal/acceptancecatalog"
)

func TestAcceptanceIDPatternCoversManifestRange(t *testing.T) {
	for _, value := range []string{"A01", "A99", "A100", "A101", "A102"} {
		if !acceptanceIDPattern.MatchString(value) {
			t.Fatalf("acceptanceIDPattern rejected %s", value)
		}
	}
	for _, value := range []string{"A00", "A1", "A103", "A999"} {
		if acceptanceIDPattern.MatchString(value) {
			t.Fatalf("acceptanceIDPattern accepted %s", value)
		}
	}
}

func TestCanonicalGenericEvidenceRejectsArbitraryAndEncodedCommands(t *testing.T) {
	genericCount := 0
	for _, runner := range acceptancecatalog.All() {
		path, err := acceptancecatalog.RunnerPath(runner)
		if err != nil {
			t.Fatal(err)
		}
		if path != "scripts/acceptance/run-generic-case.ps1" {
			continue
		}
		genericCount++
		for _, command := range [][]string{
			{"powershell.exe", "-NoProfile", "-EncodedCommand", "ZQB4AGkAdAAgADAA"},
			{"powershell.exe", "-Command", "exit 0"},
			{"docker.exe", "run", "--rm", "anything"},
		} {
			if err := acceptancecatalog.ValidateCanonicalCommand(runner.AcceptanceID, command); err == nil {
				t.Fatalf("%s accepted arbitrary command %q", runner.AcceptanceID, command)
			}
		}
	}
	if genericCount != 91 {
		t.Fatalf("canonical generic registry contains %d cases, want 91", genericCount)
	}
}

func TestCanonicalGenericRunnerCoversEveryRegisteredCaseAndManifestPath(t *testing.T) {
	payload, err := os.ReadFile("run-generic-case.ps1")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(payload), "\r\n", "\n")
	executableCases := map[string]struct{}{"A47": {}, "A83": {}}
	for _, section := range []struct{ start, end string }{
		{"$goCases = @{", "$e2eCases = @{"},
		{"$e2eCases = @{", "$bunCases = @{"},
		{"$bunCases = @{", "function Write-CaseJson"},
	} {
		start := strings.Index(script, section.start)
		end := strings.Index(script, section.end)
		if start < 0 || end <= start {
			t.Fatalf("cannot locate runner registry section %q..%q", section.start, section.end)
		}
		for _, match := range regexp.MustCompile(`(?m)^\s+(A\d+)\s*=`).FindAllStringSubmatch(script[start:end], -1) {
			executableCases[match[1]] = struct{}{}
		}
	}
	for _, runner := range acceptancecatalog.All() {
		path, err := acceptancecatalog.RunnerPath(runner)
		if err != nil {
			t.Fatal(err)
		}
		_, executable := executableCases[runner.AcceptanceID]
		if path == "scripts/acceptance/run-generic-case.ps1" && !executable {
			t.Fatalf("runner has no canonical mapping for %s", runner.AcceptanceID)
		}
	}
	for _, required := range []string{
		"A83') { Invoke-DocsNegativeAcceptance }",
		"./tests/integration ./internal/docscheck",
		"e2e/logs.spec.ts", "src/features/logs/api.test.ts",
		"e2e/user-inventory.spec.ts", "src/features/user-inventory/api.test.ts",
		"e2e/channel-inventory.spec.ts", "src/features/channel-inventory/api.test.ts",
		"e2e/performance-history.spec.ts", "src/features/performance-history/api.test.ts",
		"e2e/financial-operations.spec.ts", "src/features/financial-operations/export-request.test.ts",
		"e2e/upstream-tasks.spec.ts", "src/features/upstream-tasks/api.test.ts",
		"e2e/model-catalog.spec.ts", "src/features/model-catalog/icon-boundary.test.ts",
		"e2e/rankings.spec.ts", "src/features/rankings/api.test.ts",
		"e2e/subscription-plans.spec.ts", "src/features/subscription-plans/privacy-boundary.test.ts",
		"e2e/pricing-groups.spec.ts", "src/features/pricing-groups/privacy-boundary.test.ts",
		"e2e/system-tasks.spec.ts", "src/features/system-tasks/privacy-boundary.test.ts",
		"e2e/site-task-catalog.spec.ts", "src/features/sites/site-task-catalog-fixture-consumption.test.ts",
		"src/lib/acceptance-fixture-consumption.test.ts",
		"database_class = 'isolated_new_api_pilot_test'",
		"'--project=chromium-desktop', '--project=chromium-mobile'",
		"PLAYWRIGHT_INTERNAL_PORT', '4173'",
		"'-p', '1'", "'-count=1'",
		"TestA08A12A13A28CollectionHourReplacementAndVisibility",
		"TestA19A63ResourceSnapshotAcceptance",
		"TestA76CRUDRouteAndDTOAcceptance",
		"TestA86DisabledRecheckStaysPausedWithoutBackfill",
		"TestA102AuthorizationPricingIntentIdempotencyFailureAndFence",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("canonical generic runner is missing %q", required)
		}
	}
	if strings.Contains(script, "PLAYWRIGHT_BASE_URL") {
		t.Fatal("canonical generic runner depends on an unchecked external Playwright server")
	}
}

func TestClassifyGenericEvidence(t *testing.T) {
	tests := []struct {
		root  string
		want  string
		valid bool
	}{
		{root: "artifacts/acceptance", want: "formal", valid: true},
		{root: "./artifacts/smoke", want: "development", valid: true},
		{root: "artifacts/custom", valid: false},
	}
	for _, test := range tests {
		t.Run(test.root, func(t *testing.T) {
			got, err := classifyGenericEvidence(test.root)
			if (err == nil) != test.valid || got != test.want {
				t.Fatalf("classifyGenericEvidence(%q) = %q, %v; want %q valid=%t", test.root, got, err, test.want, test.valid)
			}
		})
	}
}
