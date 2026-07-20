package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceTestPathsSupportsLegacySinglePath(t *testing.T) {
	root := t.TempDir()
	path := "tests/contract/example_test.go"
	writeManifestTestFile(t, root, path)
	current := &checker{root: root}
	paths := current.checkAcceptanceTestPaths("manifest.yaml", acceptanceCase{
		AcceptanceID:      "A88",
		TestOrRunbookPath: path,
	})
	if len(current.issues) != 0 || len(paths) != 1 || paths[0] != path {
		t.Fatalf("legacy path result paths=%#v issues=%#v", paths, current.issues)
	}
}

func TestAcceptanceTestPathsRejectsInvalidArrays(t *testing.T) {
	root := t.TempDir()
	path := "tests/contract/example_test.go"
	writeManifestTestFile(t, root, path)
	for _, test := range []struct {
		name string
		item acceptanceCase
		want string
	}{
		{
			name: "mutually exclusive",
			item: acceptanceCase{AcceptanceID: "A89", TestOrRunbookPath: path, TestOrRunbookPaths: []string{path}},
			want: "mutually exclusive",
		},
		{
			name: "empty array",
			item: acceptanceCase{AcceptanceID: "A89", TestOrRunbookPaths: []string{}},
			want: "is empty",
		},
		{
			name: "duplicate",
			item: acceptanceCase{AcceptanceID: "A89", TestOrRunbookPaths: []string{path, path}},
			want: "repeats",
		},
		{
			name: "missing path",
			item: acceptanceCase{AcceptanceID: "A89", TestOrRunbookPaths: []string{"tests/contract/missing_test.go"}},
			want: "target does not exist",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := &checker{root: root}
			current.checkAcceptanceTestPaths("manifest.yaml", test.item)
			if !issuesContain(current.issues, test.want) {
				t.Fatalf("issues=%#v, want substring %q", current.issues, test.want)
			}
		})
	}
}

func TestA89ToA100RequireEveryTestLayer(t *testing.T) {
	complete := []string{
		"tests/integration/example_acceptance_test.go",
		"web/src/features/example/api.test.ts",
		"web/e2e/example.spec.ts",
		"web/src/lib/acceptance-fixture-consumption.test.ts",
	}
	coverage := classifyMultiLayerPaths(complete)
	if !coverage.BackendIntegration || !coverage.ContractOrUnit || !coverage.DesktopMobileE2E || !coverage.SafetyOrFixture {
		t.Fatalf("complete paths classified as %#v", coverage)
	}

	checks := []struct {
		name    string
		paths   []string
		missing func(multiLayerCoverage) bool
	}{
		{"backend integration", complete[1:], func(value multiLayerCoverage) bool { return !value.BackendIntegration }},
		{"contract unit", []string{complete[0], complete[2], "testdata/security-absence.scan"}, func(value multiLayerCoverage) bool { return !value.ContractOrUnit }},
		{"desktop mobile e2e", []string{complete[0], complete[1], complete[3]}, func(value multiLayerCoverage) bool { return !value.DesktopMobileE2E }},
		{"safety fixture", complete[:3], func(value multiLayerCoverage) bool { return !value.SafetyOrFixture }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !check.missing(classifyMultiLayerPaths(check.paths)) {
				t.Fatalf("paths %#v did not omit required coverage", check.paths)
			}
		})
	}
}

func TestMultiLayerAcceptanceRangeStartsAtA89AndIncludesA100(t *testing.T) {
	for _, id := range []string{"A89", "A99", "A100"} {
		if !requiresMultiLayerAcceptance(id) {
			t.Fatalf("%s should require multi-layer coverage", id)
		}
	}
	for _, id := range []string{"A01", "A88", "A101", "invalid"} {
		if requiresMultiLayerAcceptance(id) {
			t.Fatalf("%s should not require multi-layer coverage", id)
		}
	}
}

func TestMultiLayerAcceptanceRequiresDesktopAndMobileProjects(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "web", "playwright.config.ts")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("chromium-desktop only"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkMultiLayerAcceptance("manifest.yaml", "A100", []string{
		"tests/integration/example_acceptance_test.go",
		"web/src/features/example/api.test.ts",
		"web/e2e/example.spec.ts",
		"web/src/lib/acceptance-fixture-consumption.test.ts",
	})
	if !issuesContain(current.issues, "chromium-desktop and chromium-mobile") {
		t.Fatalf("issues=%#v, want missing mobile project", current.issues)
	}
}

func writeManifestTestFile(t *testing.T, root string, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func issuesContain(issues []Issue, substring string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.String(), substring) {
			return true
		}
	}
	return false
}
