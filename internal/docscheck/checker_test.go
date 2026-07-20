package docscheck

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandReferences(t *testing.T) {
	want := []string{"A01", "A02", "A03", "A07", "A10", "A11"}
	got := expandReferences("A01～A03、A07、A10-A11", "A")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandReferences = %#v, want %#v", got, want)
	}
}

func TestSplitMarkdownRowPreservesEscapedPipe(t *testing.T) {
	got := splitMarkdownRow("| CODE | value:`left\\|right` | - |")
	want := []string{"CODE", "value:`left|right`", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitMarkdownRow = %#v, want %#v", got, want)
	}
}

func TestExpandReferencesIncludesThreeDigitAcceptanceID(t *testing.T) {
	want := []string{"A99", "A100"}
	got := expandReferences("A99-A100", "A")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandReferences = %#v, want %#v", got, want)
	}
}

func TestAcceptanceIDPatternEndsAtA102(t *testing.T) {
	for _, value := range []string{"A01", "A99", "A100", "A101", "A102"} {
		if !acceptanceIDPattern.MatchString(value) {
			t.Fatalf("acceptanceIDPattern rejected %s", value)
		}
	}
	for _, value := range []string{"A1", "A000", "A103"} {
		if acceptanceIDPattern.MatchString(value) {
			t.Fatalf("acceptanceIDPattern accepted %s", value)
		}
	}
}

func TestParseParamList(t *testing.T) {
	got, err := parseParamList("site_id、start_timestamp、failure_kind:`data_mismatch|execution_failed`")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"site_id":         "IdString",
		"start_timestamp": "Timestamp",
		"failure_kind":    "data_mismatch|execution_failed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseParamList = %#v, want %#v", got, want)
	}
}

func TestMarkdownCheckFindsBrokenFileAndAnchor(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "README.md"), "[ok](docs/target.md#valid-heading)\n[missing](docs/missing.md)\n[bad anchor](docs/target.md#missing)\n")
	writeTestFile(t, filepath.Join(docs, "target.md"), "# Valid Heading\n")

	current := &checker{root: root}
	current.checkMarkdownLinks()
	if len(current.issues) != 2 {
		t.Fatalf("issues = %#v, want two broken-link issues", current.issues)
	}
}

func TestGithubStyleSlug(t *testing.T) {
	if got, want := githubStyleSlug("51. 核心验收矩阵"), "51-核心验收矩阵"; got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
}

func TestLocalesRequireOnlyZhCN(t *testing.T) {
	root := t.TempDir()
	localeDir := filepath.Join(root, "web", "src", "i18n", "locales")
	if err := os.MkdirAll(localeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "web", "package.json"), `{ "dependencies": {} }`)
	writeTestFile(t, filepath.Join(localeDir, "zh-CN.json"), `{ "INTERNAL_CONTRACT_ERROR": "内部契约错误" }`)
	writeTestFile(t, filepath.Join(root, "web", "src", "i18n", "config.ts"), `import zhCN from './locales/zh-CN.json'
export const supportedLanguages = ['zh-CN'] as const
export const resources = {
  'zh-CN': { translation: zhCN },
} as const
void i18n.init({ resources, lng: 'zh-CN', fallbackLng: 'zh-CN', supportedLngs: supportedLanguages })
`)
	catalog := &messageCatalog{Entries: []messageCatalogEntry{{Code: "INTERNAL_CONTRACT_ERROR"}}}
	current := &checker{root: root}
	current.checkLocales(catalog)
	if len(current.issues) != 0 {
		t.Fatalf("valid zh-CN-only config produced issues: %#v", current.issues)
	}

	writeTestFile(t, filepath.Join(localeDir, "en.json"), `{ "INTERNAL_CONTRACT_ERROR": "Internal contract error" }`)
	current = &checker{root: root}
	current.checkLocales(catalog)
	if len(current.issues) == 0 {
		t.Fatal("extra locale was not rejected")
	}
}

func TestA49UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A49")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A49", "evidence_path", "artifacts/acceptance/A49/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A49 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A49", "evidence_path", "planned:artifacts/acceptance/A49/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A49 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA51UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A51")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A51", "evidence_path", "artifacts/acceptance/A51/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A51 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A51", "evidence_path", "planned:artifacts/acceptance/A51/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A51 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA62UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A62")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A62", "evidence_path", "artifacts/acceptance/A62/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A62 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A62", "evidence_path", "planned:artifacts/acceptance/A62/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A62 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA25UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A25")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A25", "evidence_path", "artifacts/acceptance/A25/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A25 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A25", "evidence_path", "planned:artifacts/acceptance/A25/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A25 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA45UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A45")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A45", "evidence_path", "artifacts/acceptance/A45/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A45 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A45", "evidence_path", "planned:artifacts/acceptance/A45/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A45 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA50UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A50")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A50", "evidence_path", "artifacts/acceptance/A50/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A50 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A50", "evidence_path", "planned:artifacts/acceptance/A50/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A50 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func TestA22UnplannedEvidenceRequiresValidatedFormalRun(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "artifacts", "acceptance", "A22")
	if err := os.MkdirAll(evidence, 0o755); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A22", "evidence_path", "artifacts/acceptance/A22/", true)
	if len(current.issues) != 1 {
		t.Fatalf("empty unplanned A22 evidence directory produced issues=%#v, want one", current.issues)
	}

	current = &checker{root: root}
	current.checkPlannedPath("manifest.yaml", "A22", "evidence_path", "planned:artifacts/acceptance/A22/", true)
	if len(current.issues) != 0 {
		t.Fatalf("planned A22 evidence should not be validated as final evidence: %#v", current.issues)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
