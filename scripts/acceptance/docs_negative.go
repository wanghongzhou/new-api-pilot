package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"new-api-pilot/internal/docscheck"
)

type docsNegativeScenario struct {
	Name          string   `json:"name"`
	ExpectedCheck string   `json:"expected_check"`
	Passed        bool     `json:"passed"`
	Issues        []string `json:"issues"`
	mutate        func(string) error
}

type docsNegativeReport struct {
	SchemaVersion  int                    `json:"schema_version"`
	AcceptanceID   string                 `json:"acceptance_id"`
	Status         string                 `json:"status"`
	PositiveBefore bool                   `json:"positive_before"`
	Scenarios      []docsNegativeScenario `json:"scenarios"`
	PositiveAfter  bool                   `json:"positive_after"`
}

func runDocsNegative(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("docs-negative", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(stderr, "resolve repository root: %v\n", err)
		return 2
	}
	report := docsNegativeReport{
		SchemaVersion:  evidenceSchemaVersion,
		AcceptanceID:   "A83",
		PositiveBefore: len(docscheck.Check(absoluteRoot)) == 0,
		Scenarios: []docsNegativeScenario{
			{Name: "missing-decision-mapping", ExpectedCheck: "traceability", mutate: removeDecisionMapping},
			{Name: "missing-acceptance-case", ExpectedCheck: "manifest", mutate: removeAcceptanceCase},
			{Name: "missing-required-multilayer-path", ExpectedCheck: "manifest", mutate: removeRequiredMultiLayerPath},
			{Name: "extra-locale", ExpectedCheck: "locales", mutate: addExtraLocale},
			{Name: "broken-markdown-link", ExpectedCheck: "markdown", mutate: addBrokenMarkdownLink},
			{Name: "fixture-checksum-mismatch", ExpectedCheck: "fixtures", mutate: alterFixtureWithoutChecksum},
		},
	}
	for index := range report.Scenarios {
		scenario := &report.Scenarios[index]
		temporaryRoot, temporaryErr := os.MkdirTemp("", "new-api-pilot-a83-")
		if temporaryErr != nil {
			scenario.Issues = []string{temporaryErr.Error()}
			continue
		}
		func() {
			defer os.RemoveAll(temporaryRoot)
			if copyErr := copyRepositoryForDocsCheck(absoluteRoot, temporaryRoot); copyErr != nil {
				scenario.Issues = []string{copyErr.Error()}
				return
			}
			if mutationErr := scenario.mutate(temporaryRoot); mutationErr != nil {
				scenario.Issues = []string{mutationErr.Error()}
				return
			}
			issues := docscheck.Check(temporaryRoot)
			for _, issue := range issues {
				scenario.Issues = append(scenario.Issues, issue.String())
				if issue.Check == scenario.ExpectedCheck {
					scenario.Passed = true
				}
			}
		}()
	}
	report.PositiveAfter = len(docscheck.Check(absoluteRoot)) == 0
	report.Status = "passed"
	if !report.PositiveBefore || !report.PositiveAfter {
		report.Status = "failed"
	}
	for _, scenario := range report.Scenarios {
		if !scenario.Passed {
			report.Status = "failed"
		}
	}
	for index := range report.Scenarios {
		report.Scenarios[index].mutate = nil
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode negative report: %v\n", err)
		return 2
	}
	payload = append(payload, '\n')
	if _, err := stdout.Write(payload); err != nil {
		fmt.Fprintf(stderr, "write negative report: %v\n", err)
		return 2
	}
	if evidenceDirectory := os.Getenv("ACCEPTANCE_EVIDENCE_DIR"); evidenceDirectory != "" {
		if err := writeJSONAtomic(filepath.Join(evidenceDirectory, "negative-report.json"), report); err != nil {
			fmt.Fprintf(stderr, "persist negative report: %v\n", err)
			return 2
		}
	}
	if report.Status != "passed" {
		return 1
	}
	return 0
}

func copyRepositoryForDocsCheck(sourceRoot string, destinationRoot string) error {
	skippedDirectories := map[string]struct{}{
		".git": {}, ".codegraph": {}, "data": {},
		filepath.Join("web", "node_modules"): {}, filepath.Join("web", "dist"): {},
		filepath.Join("web", ".tanstack"): {}, filepath.Join("web", "playwright-report"): {},
		filepath.Join("web", "test-results"): {},
	}
	return filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if _, skipped := skippedDirectories[relative]; skipped && entry.IsDir() {
			return filepath.SkipDir
		}
		target := filepath.Join(destinationRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository copy rejects symlink %s", filepath.ToSlash(relative))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, payload, info.Mode().Perm())
	})
}

func replaceExactlyOnce(root string, relative string, old string, replacement string) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Count(string(payload), old) != 1 {
		return fmt.Errorf("mutation target in %s is not unique", relative)
	}
	return os.WriteFile(path, []byte(strings.Replace(string(payload), old, replacement, 1)), 0o640)
}

func removeDecisionMapping(root string) error {
	const row = "| D01 | 本期不做财务对账、欠费、账单和结算 | 1.2、概要设计 2.3 | R05 | N/A-absence-scan |"
	const broken = "| D01 | 本期不做财务对账、欠费、账单和结算 | 1.2、概要设计 2.3 |  | N/A-absence-scan |"
	return replaceExactlyOnce(root, "docs/多站点运营管理平台-详细设计-07-运维与验收.md", row, broken)
}

func removeAcceptanceCase(root string) error {
	path := filepath.Join(root, "docs", "acceptance", "manifest.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	marker := "\n  - acceptance_id: A92\n"
	index := strings.Index(string(payload), marker)
	if index < 0 {
		return errors.New("A92 manifest block was not found")
	}
	return os.WriteFile(path, payload[:index+1], 0o640)
}

func removeRequiredMultiLayerPath(root string) error {
	return replaceExactlyOnce(
		root,
		"docs/acceptance/manifest.yaml",
		"      - \"web/e2e/system-tasks.spec.ts\"\n",
		"",
	)
}

func addExtraLocale(root string) error {
	source := filepath.Join(root, "web", "src", "i18n", "locales", "zh-CN.json")
	destination := filepath.Join(root, "web", "src", "i18n", "locales", "en-US.json")
	payload, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, payload, 0o640)
}

func addBrokenMarkdownLink(root string) error {
	path := filepath.Join(root, "docs", "acceptance", "README.md")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString("\n[broken A83 link](./missing-a83-target.md)\n")
	return err
}

func alterFixtureWithoutChecksum(root string) error {
	path := filepath.Join(root, "testdata", "design", "f01-auth.json")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString("\n")
	return err
}
