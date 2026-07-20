package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"new-api-pilot/internal/a22evidence"
	"new-api-pilot/internal/a25evidence"
	"new-api-pilot/internal/a45evidence"
	"new-api-pilot/internal/a49evidence"
	"new-api-pilot/internal/a50evidence"
	"new-api-pilot/internal/a62evidence"
	"new-api-pilot/internal/opsevidence"

	"gopkg.in/yaml.v3"
)

const acceptanceManifestPath = "docs/acceptance/manifest.yaml"

var acceptanceIDPattern = regexp.MustCompile(`^A(?:\d{2}|10[0-2])$`)

type acceptanceManifest struct {
	SchemaVersion   int                          `yaml:"schema_version"`
	Baseline        acceptanceBaseline           `yaml:"baseline"`
	Fixtures        map[string]fixtureDefinition `yaml:"fixtures"`
	AcceptanceCases []acceptanceCase             `yaml:"acceptance_cases"`
}

type acceptanceBaseline struct {
	Source                  string `yaml:"source"`
	AcceptanceRange         string `yaml:"acceptance_range"`
	ReleasePolicy           string `yaml:"release_policy"`
	ProductLocale           string `yaml:"product_locale"`
	FixtureChecksumManifest string `yaml:"fixture_checksum_manifest"`
}

type fixtureDefinition struct {
	Path    string `yaml:"path"`
	Purpose string `yaml:"purpose"`
}

type acceptanceCase struct {
	AcceptanceID       string   `yaml:"acceptance_id"`
	RequirementID      string   `yaml:"requirement_id"`
	Fixture            []string `yaml:"fixture"`
	Layer              string   `yaml:"layer"`
	TestOrRunbookPath  string   `yaml:"test_or_runbook_path"`
	TestOrRunbookPaths []string `yaml:"test_or_runbook_paths"`
	OwnerRole          string   `yaml:"owner_role"`
	EvidencePath       string   `yaml:"evidence_path"`
}

type multiLayerCoverage struct {
	BackendIntegration bool
	ContractOrUnit     bool
	DesktopMobileE2E   bool
	SafetyOrFixture    bool
}

func (current *checker) checkAcceptanceManifest(trace traceability) *acceptanceManifest {
	path := filepath.Join(current.root, filepath.FromSlash(acceptanceManifestPath))
	file, err := os.Open(path)
	if err != nil {
		current.add("manifest", path, "open: %v", err)
		return nil
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var manifest acceptanceManifest
	if err := decoder.Decode(&manifest); err != nil {
		current.add("manifest", path, "decode YAML: %v", err)
		return nil
	}
	if manifest.SchemaVersion != 1 {
		current.add("manifest", path, "schema_version = %d, want 1", manifest.SchemaVersion)
	}
	if manifest.Baseline.AcceptanceRange != "A01-A102" {
		current.add("manifest", path, "baseline.acceptance_range = %q, want A01-A102", manifest.Baseline.AcceptanceRange)
	}
	if manifest.Baseline.ReleasePolicy != "required-no-skip" {
		current.add("manifest", path, "baseline.release_policy = %q, want required-no-skip", manifest.Baseline.ReleasePolicy)
	}
	if manifest.Baseline.ProductLocale != "zh-CN" {
		current.add("manifest", path, "baseline.product_locale = %q, want zh-CN", manifest.Baseline.ProductLocale)
	}
	if manifest.Baseline.Source == "" {
		current.add("manifest", path, "baseline.source is empty")
	}
	if manifest.Baseline.FixtureChecksumManifest != "testdata/design/manifest.sha256" {
		current.add("manifest", path, "unexpected fixture checksum path %q", manifest.Baseline.FixtureChecksumManifest)
	}

	expectedFixturePaths := map[string]string{
		"F01": "testdata/design/f01-auth.json",
		"F02": "testdata/design/f02-upstream/",
		"F03": "testdata/design/f03-statistics.sql",
		"F04": "testdata/design/f04-state-machines.json",
		"F05": "testdata/design/f05-ops-capacity.yaml",
		"F06": "testdata/design/f06-finance-operations.json",
		"F07": "testdata/design/f07-upstream-tasks.json",
		"F08": "testdata/design/f08-model-catalog.json",
		"F09": "testdata/design/f09-subscription-plans.json",
		"F10": "testdata/design/f10-pricing-groups.json",
		"F11": "testdata/design/f11-system-tasks.json",
		"F12": "testdata/design/f12-site-task-catalog.json",
		"F13": "testdata/design/f13-data-maintenance.json",
	}
	for fixtureID, expectedPath := range expectedFixturePaths {
		definition, exists := manifest.Fixtures[fixtureID]
		if !exists {
			current.add("manifest", path, "missing fixture %s", fixtureID)
			continue
		}
		if definition.Path != expectedPath {
			current.add("manifest", path, "%s path = %q, want %q", fixtureID, definition.Path, expectedPath)
		}
		if strings.TrimSpace(definition.Purpose) == "" {
			current.add("manifest", path, "%s purpose is empty", fixtureID)
		}
		current.requireRepositoryPath("manifest", path, definition.Path, true)
	}
	for fixtureID := range manifest.Fixtures {
		if _, expected := expectedFixturePaths[fixtureID]; !expected {
			current.add("manifest", path, "unknown fixture %s", fixtureID)
		}
	}

	allowedLayers := stringSet("integration", "contract", "e2e", "static-analysis", "runbook")
	allowedOwners := stringSet("backend", "frontend", "sre", "security", "qa")
	caseIDs := make(map[string]struct{}, len(manifest.AcceptanceCases))
	for index, acceptance := range manifest.AcceptanceCases {
		location := fmt.Sprintf("acceptance_cases[%d]", index)
		if !acceptanceIDPattern.MatchString(acceptance.AcceptanceID) {
			current.add("manifest", path, "%s has invalid acceptance_id %q", location, acceptance.AcceptanceID)
		} else if _, duplicate := caseIDs[acceptance.AcceptanceID]; duplicate {
			current.add("manifest", path, "duplicate acceptance_id %s", acceptance.AcceptanceID)
		}
		caseIDs[acceptance.AcceptanceID] = struct{}{}

		requirementCases, validRequirement := trace.requirements[acceptance.RequirementID]
		if !validRequirement {
			current.add("manifest", path, "%s has invalid requirement_id %q", acceptance.AcceptanceID, acceptance.RequirementID)
		} else if _, linked := requirementCases[acceptance.AcceptanceID]; !linked {
			current.add("manifest", path, "%s primary requirement %s does not link that A case", acceptance.AcceptanceID, acceptance.RequirementID)
		}
		if len(acceptance.Fixture) == 0 {
			current.add("manifest", path, "%s has no fixture", acceptance.AcceptanceID)
		}
		seenFixtures := make(map[string]struct{}, len(acceptance.Fixture))
		for _, fixtureID := range acceptance.Fixture {
			if _, exists := manifest.Fixtures[fixtureID]; !exists {
				current.add("manifest", path, "%s references unknown fixture %s", acceptance.AcceptanceID, fixtureID)
			}
			if _, duplicate := seenFixtures[fixtureID]; duplicate {
				current.add("manifest", path, "%s repeats fixture %s", acceptance.AcceptanceID, fixtureID)
			}
			seenFixtures[fixtureID] = struct{}{}
		}
		if _, allowed := allowedLayers[acceptance.Layer]; !allowed {
			current.add("manifest", path, "%s has invalid layer %q", acceptance.AcceptanceID, acceptance.Layer)
		}
		if _, allowed := allowedOwners[acceptance.OwnerRole]; !allowed {
			current.add("manifest", path, "%s has invalid owner_role %q", acceptance.AcceptanceID, acceptance.OwnerRole)
		}
		testPaths := current.checkAcceptanceTestPaths(path, acceptance)
		if requiresMultiLayerAcceptance(acceptance.AcceptanceID) {
			current.checkMultiLayerAcceptance(path, acceptance.AcceptanceID, testPaths)
		}
		if acceptance.AcceptanceID == "A102" {
			current.checkIntegrationContractAcceptance(path, acceptance.AcceptanceID, testPaths)
		}
		current.checkPlannedPath(path, acceptance.AcceptanceID, "evidence_path", acceptance.EvidencePath, true)
		if acceptance.Layer == "runbook" {
			for _, testPath := range testPaths {
				if strings.HasPrefix(testPath, "planned:") {
					current.add("manifest", path, "%s runbook path must exist rather than remain planned", acceptance.AcceptanceID)
				}
			}
		}
	}
	checkIdentifierRange(current, path, "A", 1, 102, keys(caseIDs))
	for acceptanceID := range trace.acceptanceCases {
		if _, exists := caseIDs[acceptanceID]; !exists {
			current.add("manifest", path, "%s exists in design matrix but not manifest", acceptanceID)
		}
	}
	return &manifest
}

func (current *checker) checkAcceptanceTestPaths(manifestPath string, acceptance acceptanceCase) []string {
	hasLegacy := strings.TrimSpace(acceptance.TestOrRunbookPath) != ""
	hasArray := acceptance.TestOrRunbookPaths != nil
	if hasLegacy && hasArray {
		current.add("manifest", manifestPath, "%s test_or_runbook_path and test_or_runbook_paths are mutually exclusive", acceptance.AcceptanceID)
		return nil
	}
	if !hasLegacy && !hasArray {
		current.add("manifest", manifestPath, "%s has no test_or_runbook_path or test_or_runbook_paths", acceptance.AcceptanceID)
		return nil
	}
	paths := acceptance.TestOrRunbookPaths
	field := "test_or_runbook_paths"
	if hasLegacy {
		paths = []string{acceptance.TestOrRunbookPath}
		field = "test_or_runbook_path"
	}
	if len(paths) == 0 {
		current.add("manifest", manifestPath, "%s test_or_runbook_paths is empty", acceptance.AcceptanceID)
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	validPaths := make([]string, 0, len(paths))
	for index, testPath := range paths {
		trimmed := strings.TrimSpace(testPath)
		pathField := field
		if hasArray {
			pathField = fmt.Sprintf("test_or_runbook_paths[%d]", index)
		}
		if trimmed == "" {
			current.add("manifest", manifestPath, "%s %s is empty", acceptance.AcceptanceID, pathField)
			continue
		}
		if _, duplicate := seen[trimmed]; duplicate {
			current.add("manifest", manifestPath, "%s repeats %s path %s", acceptance.AcceptanceID, field, filepath.ToSlash(trimmed))
			continue
		}
		seen[trimmed] = struct{}{}
		validPaths = append(validPaths, trimmed)
		current.checkPlannedPath(manifestPath, acceptance.AcceptanceID, pathField, trimmed, false)
	}
	return validPaths
}

func requiresMultiLayerAcceptance(acceptanceID string) bool {
	if !acceptanceIDPattern.MatchString(acceptanceID) {
		return false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(acceptanceID, "A"))
	return err == nil && number >= 89 && number <= 101
}

func classifyMultiLayerPaths(paths []string) multiLayerCoverage {
	var coverage multiLayerCoverage
	for _, value := range paths {
		path := strings.ToLower(strings.TrimPrefix(filepath.ToSlash(value), "planned:"))
		if strings.HasPrefix(path, "tests/integration/") && strings.HasSuffix(path, "_test.go") {
			coverage.BackendIntegration = true
		}
		if (strings.HasPrefix(path, "tests/contract/") && strings.HasSuffix(path, "_test.go")) ||
			(!strings.HasPrefix(path, "tests/integration/") && strings.HasSuffix(path, "_test.go")) ||
			strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx") {
			coverage.ContractOrUnit = true
		}
		if strings.HasPrefix(path, "web/e2e/") && strings.HasSuffix(path, ".spec.ts") {
			coverage.DesktopMobileE2E = true
		}
		if strings.Contains(path, "privacy-boundary") || strings.Contains(path, "absence") ||
			strings.Contains(path, "fixture-consumption") || strings.Contains(path, "security_acceptance") ||
			strings.Contains(path, "restore_safety") {
			coverage.SafetyOrFixture = true
		}
	}
	return coverage
}

func (current *checker) checkMultiLayerAcceptance(manifestPath string, acceptanceID string, paths []string) {
	coverage := classifyMultiLayerPaths(paths)
	if !coverage.BackendIntegration {
		current.add("manifest", manifestPath, "%s test paths lack backend integration coverage", acceptanceID)
	}
	if !coverage.ContractOrUnit {
		current.add("manifest", manifestPath, "%s test paths lack contract/unit coverage", acceptanceID)
	}
	if !coverage.DesktopMobileE2E {
		current.add("manifest", manifestPath, "%s test paths lack desktop/mobile E2E coverage", acceptanceID)
	} else if !hasDesktopMobilePlaywrightProjects(current.root) {
		current.add("manifest", manifestPath, "%s E2E coverage requires chromium-desktop and chromium-mobile Playwright projects", acceptanceID)
	}
	if !coverage.SafetyOrFixture {
		current.add("manifest", manifestPath, "%s test paths lack safety absence/fixture consumption coverage", acceptanceID)
	}
}

func (current *checker) checkIntegrationContractAcceptance(manifestPath string, acceptanceID string, paths []string) {
	coverage := classifyMultiLayerPaths(paths)
	if !coverage.BackendIntegration {
		current.add("manifest", manifestPath, "%s test paths lack backend integration coverage", acceptanceID)
	}
	if !coverage.ContractOrUnit {
		current.add("manifest", manifestPath, "%s test paths lack contract/unit coverage", acceptanceID)
	}
}

func hasDesktopMobilePlaywrightProjects(root string) bool {
	payload, err := os.ReadFile(filepath.Join(root, "web", "playwright.config.ts"))
	if err != nil {
		return false
	}
	text := string(payload)
	return strings.Contains(text, "chromium-desktop") && strings.Contains(text, "chromium-mobile")
}

func (current *checker) checkPlannedPath(manifestPath string, acceptanceID string, field string, value string, wantDirectory bool) {
	planned := strings.HasPrefix(value, "planned:")
	relative := strings.TrimPrefix(value, "planned:")
	if strings.TrimSpace(relative) == "" {
		current.add("manifest", manifestPath, "%s %s is empty", acceptanceID, field)
		return
	}
	resolved, err := resolveRepositoryPath(current.root, relative)
	if err != nil {
		current.add("manifest", manifestPath, "%s %s: %v", acceptanceID, field, err)
		return
	}
	info, statErr := os.Stat(resolved)
	if planned {
		if current.options.RequireNoPlanned {
			current.add("manifest", manifestPath, "%s %s remains planned in final mode: %s", acceptanceID, field, filepath.ToSlash(relative))
		}
		return
	}
	if statErr != nil {
		current.add("manifest", manifestPath, "%s %s target does not exist: %s", acceptanceID, field, filepath.ToSlash(relative))
		return
	}
	if wantDirectory && !info.IsDir() {
		current.add("manifest", manifestPath, "%s %s must be a directory: %s", acceptanceID, field, filepath.ToSlash(relative))
	}
	if !wantDirectory && info.IsDir() {
		current.add("manifest", manifestPath, "%s %s must be a file: %s", acceptanceID, field, filepath.ToSlash(relative))
	}
	if current.options.RequireNoPlanned && field == "evidence_path" && wantDirectory && info.IsDir() {
		current.checkFormalEvidenceRoot(manifestPath, acceptanceID, resolved)
	}
	if acceptanceID == "A49" && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a49evidence.ValidateEvidenceRoot(resolved, a49evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "A49 evidence path has no valid formal run: %v", err)
		}
	}
	if a22evidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a22evidence.ValidateEvidenceRoot(resolved, a22evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
	if a25evidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a25evidence.ValidateEvidenceRoot(resolved, a25evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
	if a45evidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a45evidence.ValidateEvidenceRoot(resolved, a45evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
	if a50evidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a50evidence.ValidateEvidenceRoot(resolved, a50evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
	if opsevidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := opsevidence.ValidateEvidenceRoot(resolved, acceptanceID, opsevidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
	if a62evidence.Supports(acceptanceID) && field == "evidence_path" && wantDirectory && info.IsDir() {
		if err := a62evidence.ValidateEvidenceRoot(resolved, a62evidence.FormalClass); err != nil {
			current.add("manifest", manifestPath, "%s evidence path has no valid formal run: %v", acceptanceID, err)
		}
	}
}

func (current *checker) requireRepositoryPath(check string, sourcePath string, relative string, allowDirectory bool) {
	resolved, err := resolveRepositoryPath(current.root, relative)
	if err != nil {
		current.add(check, sourcePath, "%v", err)
		return
	}
	info, err := os.Stat(resolved)
	if err != nil {
		current.add(check, sourcePath, "required path is missing: %s", filepath.ToSlash(relative))
		return
	}
	if info.IsDir() && !allowDirectory {
		current.add(check, sourcePath, "expected file but found directory: %s", filepath.ToSlash(relative))
	}
}

func resolveRepositoryPath(root string, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("absolute repository path is not allowed: %s", relative)
	}
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository: %s", relative)
	}
	return resolved, nil
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	result := keys(values)
	sort.Strings(result)
	return result
}
