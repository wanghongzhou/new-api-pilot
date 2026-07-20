package a50evidence

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyRequiresCanonicalA50Command(t *testing.T) {
	for _, test := range []struct {
		root  string
		class string
	}{
		{root: "artifacts/acceptance", class: FormalClass},
		{root: "artifacts/smoke", class: DevelopmentClass},
	} {
		class, err := Classify(AcceptanceID, test.root, canonicalCommand)
		if err != nil || class != test.class {
			t.Fatalf("A50 root=%q class=%q err=%v", test.root, class, err)
		}
	}
	if _, err := Classify(AcceptanceID, "artifacts/acceptance", []string{"powershell.exe", "-Command", "exit 0"}); err == nil {
		t.Fatal("arbitrary A50 exit-zero command was accepted")
	}
	if _, err := Classify(AcceptanceID, "artifacts/other", canonicalCommand); err == nil {
		t.Fatal("noncanonical A50 evidence root was accepted")
	}
}

func TestValidateA50RunAcceptsFormalAndDevelopment(t *testing.T) {
	for _, class := range []string{FormalClass, DevelopmentClass} {
		t.Run(class, func(t *testing.T) {
			run := writeValidA50Run(t, class)
			if err := ValidateRunDirectory(run, class); err != nil {
				t.Fatalf("validate A50 %s run: %v", class, err)
			}
		})
	}
}

func TestValidateA50RunRejectsFailCloseViolations(t *testing.T) {
	t.Run("missing matrix test", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		var report playwrightReport
		readA50JSON(t, filepath.Join(run, "a50-playwright.json"), &report)
		report.Suites[0].Specs = report.Suites[0].Specs[:len(report.Suites[0].Specs)-1]
		writeA50JSON(t, filepath.Join(run, "a50-playwright.json"), report)
		writeA50Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "spec") {
			t.Fatalf("A50 missing matrix error=%v", err)
		}
	})

	t.Run("retry and flaky", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		var report playwrightReport
		readA50JSON(t, filepath.Join(run, "a50-playwright.json"), &report)
		report.Suites[0].Specs[0].Tests[0].Status = "flaky"
		report.Suites[0].Specs[0].Tests[0].Results[0].Retry = 1
		report.Stats.Expected = 17
		report.Stats.Flaky = 1
		writeA50JSON(t, filepath.Join(run, "a50-playwright.json"), report)
		writeA50Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "config/stats") {
			t.Fatalf("A50 retry/flaky error=%v", err)
		}
	})

	t.Run("source skip guard", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		var report finalReport
		readA50JSON(t, filepath.Join(run, "a50-report.json"), &report)
		report.SourceGuards.NoSkip = false
		writeA50JSON(t, filepath.Join(run, "a50-report.json"), report)
		writeA50Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "final report") {
			t.Fatalf("A50 source skip error=%v", err)
		}
	})

	t.Run("html tamper", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		path := filepath.Join(run, "a50-playwright-report.html")
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.WriteString("\n")
		_ = file.Close()
		if err := ValidateRunDirectory(run, FormalClass); err == nil ||
			(!strings.Contains(err.Error(), "size mismatch") && !strings.Contains(err.Error(), "SHA-256 mismatch")) {
			t.Fatalf("A50 HTML tamper error=%v", err)
		}
	})

	t.Run("extra file", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		if err := os.WriteFile(filepath.Join(run, "unexpected.trace.zip"), []byte("extra"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "unexpected file") {
			t.Fatalf("A50 extra-file error=%v", err)
		}
	})

	t.Run("secret in server log", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		if err := os.WriteFile(filepath.Join(run, "a50-server.stderr.log"), []byte("Authorization=BearerLeakedValue\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeA50Inventory(t, run, FormalClass)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "forbidden secret") {
			t.Fatalf("A50 secret error=%v", err)
		}
	})

	t.Run("wrapper command", func(t *testing.T) {
		run := writeValidA50Run(t, FormalClass)
		var evidence wrapperEvidence
		readA50JSON(t, filepath.Join(run, "evidence.json"), &evidence)
		evidence.Command = append(evidence.Command, "-Unexpected")
		writeA50JSON(t, filepath.Join(run, "evidence.json"), evidence)
		if err := ValidateRunDirectory(run, FormalClass); err == nil || !strings.Contains(err.Error(), "wrapper evidence") {
			t.Fatalf("A50 wrapper command error=%v", err)
		}
	})
}

func TestValidateNoSecretUsesExactHTMLReporterAllowlist(t *testing.T) {
	directory := t.TempDir()
	htmlPath := filepath.Join(directory, "a50-playwright-report.html")
	allowedPayload := strings.Join(htmlReporterSecretAllowlist, "\n")
	if err := os.WriteFile(htmlPath, []byte(allowedPayload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNoSecret(htmlPath); err != nil {
		t.Fatalf("exact Playwright reporter password contexts were rejected: %v", err)
	}
	for index := range htmlReporterSecretAllowlist {
		modified := append([]string(nil), htmlReporterSecretAllowlist...)
		modified[index] += "x"
		if err := os.WriteFile(htmlPath, []byte(strings.Join(modified, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := validateNoSecret(htmlPath); err == nil || !strings.Contains(err.Error(), "forbidden secret") {
			t.Fatalf("modified HTML allowlist context %d error=%v", index, err)
		}
	}

	secretFixtures := map[string]string{
		"password assignment": `password="LeakedPassword"`,
		"password JSON":       `{"password":"AnotherLeakedPassword"}`,
		"authorization":       `Authorization=BearerLeakedValue`,
		"cookie":              `Set-Cookie=session=LeakedCookieValue`,
		"access token":        `access_token=LeakedAccessToken`,
		"database DSN":        `DATABASE_DSN=root:LeakedPassword@tcp(mysql:3306)/pilot`,
		"raw database DSN":    `root:LeakedPassword@tcp(mysql:3306)/pilot`,
		"URL query token":     `https://example.invalid/hook?token=LeakedToken`,
		"URL user info":       `https://user:LeakedPassword@example.invalid/path`,
	}
	logPath := filepath.Join(directory, "a50-server.stderr.log")
	for label, fixture := range secretFixtures {
		t.Run(label, func(t *testing.T) {
			for _, target := range []struct {
				path    string
				payload string
			}{
				{path: htmlPath, payload: allowedPayload + "\n" + fixture},
				{path: logPath, payload: fixture},
			} {
				if err := os.WriteFile(target.path, []byte(target.payload), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := validateNoSecret(target.path); err == nil || !strings.Contains(err.Error(), "forbidden secret") {
					t.Fatalf("secret fixture in %s error=%v", filepath.Base(target.path), err)
				}
			}
		})
	}
}

func TestValidPositiveDurationRejectsNonFiniteAndNonPositiveValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "fractional", value: 201544.223, want: true},
		{name: "positive", value: 1, want: true},
		{name: "zero", value: 0, want: false},
		{name: "negative", value: -1, want: false},
		{name: "nan", value: math.NaN(), want: false},
		{name: "positive infinity", value: math.Inf(1), want: false},
		{name: "negative infinity", value: math.Inf(-1), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validPositiveDuration(test.value); got != test.want {
				t.Fatalf("validPositiveDuration(%v)=%t want %t", test.value, got, test.want)
			}
		})
	}
}

func writeValidA50Run(t *testing.T, class string) string {
	t.Helper()
	run := t.TempDir()
	testOutput := `C:\Temp\new-api-pilot-a50-1-aaaaaaaaaaaa-results`
	htmlOutput := `C:\Temp\new-api-pilot-a50-1-aaaaaaaaaaaa-html`
	port := 49200

	pw := playwrightReport{}
	pw.Config.ForbidOnly = true
	pw.Config.Workers = 2
	for _, projectName := range requiredProjects {
		pw.Config.Projects = append(pw.Config.Projects, struct {
			OutputDir  string `json:"outputDir"`
			RepeatEach int    `json:"repeatEach"`
			Retries    int    `json:"retries"`
			ID         string `json:"id"`
			Name       string `json:"name"`
			TestDir    string `json:"testDir"`
		}{OutputDir: testOutput, RepeatEach: 1, ID: projectName, Name: projectName, TestDir: "/workspace/web/e2e"})
	}
	suite := playwrightSuite{Title: "statistics-states.spec.ts", File: "e2e/statistics-states.spec.ts"}
	for index, route := range requiredRoutes {
		for _, project := range requiredProjects {
			spec := playwrightSpec{Title: route.Title, OK: true, ID: "spec-" + route.Key + "-" + project, File: "statistics-states.spec.ts", Line: 637 + index, Column: 5, Tags: []string{}}
			result := playwrightTestResult{
				Status: "passed", Duration: 1000, Error: json.RawMessage("null"), Errors: []json.RawMessage{},
				Stdout: []json.RawMessage{}, Stderr: []json.RawMessage{}, Attachments: []json.RawMessage{}, Annotations: []json.RawMessage{},
			}
			spec.Tests = append(spec.Tests, playwrightTest{
				Annotations: []json.RawMessage{}, ExpectedStatus: "passed", ProjectName: project,
				ProjectID: project, Results: []playwrightTestResult{result}, Status: "expected",
			})
			suite.Specs = append(suite.Specs, spec)
		}
	}
	pw.Suites = []playwrightSuite{suite}
	pw.Errors = []json.RawMessage{}
	pw.Stats.StartTime = "2026-07-15T00:00:00.000Z"
	pw.Stats.Duration = 1000
	pw.Stats.Expected = 18
	writeA50JSON(t, filepath.Join(run, "a50-playwright.json"), pw)
	html := "<!doctype html><html><head><title>Playwright Test Report</title></head><body>" +
		strings.Join(htmlReporterSecretAllowlist, "\n") + "\n" + strings.Repeat("x", 110_000) + "</body></html>"
	if err := os.WriteFile(filepath.Join(run, "a50-playwright-report.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	writeA50TextFiles(t, run, map[string]string{
		"a50-playwright.stdout.log": "18 passed\n",
		"a50-playwright.stderr.log": "",
		"a50-check.stdout.log":      "i18n check passed: 1 locale, 1297 keys\n",
		"a50-check.stderr.log":      "",
		"a50-server.stdout.log":     "Local: http://127.0.0.1:49200\n",
		"a50-server.stderr.log":     "",
	})
	writeA50JSON(t, filepath.Join(run, "a50-check-summary.json"), checkSummary{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", Command: checkCommand,
		Steps: checkSteps, RoutesGenerate: true, Typecheck: true, Lint: true, FormatCheck: true,
		I18nCheck: true, BuildApp: true, Locales: []string{"zh-CN"}, LocaleCount: 1, TranslationKeys: 1297,
		StdoutPath: "a50-check.stdout.log", StderrPath: "a50-check.stderr.log",
	})
	playwrightCommand := append(append([]string{}, playwrightCommandPrefix...), testOutput)
	writeA50JSON(t, filepath.Join(run, "a50-command.json"), commandReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, WorkingDir: "web",
		Check: checkCommand, Server: []string{"bun", "run", "dev", "--", "--host", "127.0.0.1", "--port", "49200"},
		Playwright: playwrightCommand,
	})
	environment := environmentReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, Commit: "unborn", WorktreeDirty: true,
		OS: "windows", Architecture: "amd64", BunVersion: "1.3.13", PlaywrightVersion: "1.61.1",
		BaseURL: "http://127.0.0.1:49200", Port: port, ServerPID: 12345, Workers: 2,
		Projects: requiredProjects, DesktopViewport: viewport{Width: 1440, Height: 900}, MobileViewport: viewport{Width: 390, Height: 844},
		TestOutputDirectory: testOutput, HTMLOutputDirectory: htmlOutput,
		SpecPath: testSpecPath, SpecSHA256: approvedSpecSHA,
		LocalePath: "web/src/i18n/locales/zh-CN.json", LocaleSHA256: strings.Repeat("a", 64),
		BunLockPath: "web/bun.lock", BunLockSHA256: strings.Repeat("b", 64),
		PackagePath: "web/package.json", PackageSHA256: approvedPackageSHA,
		PlaywrightConfigPath: "web/playwright.config.ts", PlaywrightConfigSHA256: approvedPlaywrightConfigSHA,
	}
	writeA50JSON(t, filepath.Join(run, "a50-environment.json"), environment)
	writeA50JSON(t, filepath.Join(run, "a50-fixture.json"), fixtureReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, FixtureID: "F03", Path: fixturePath, SHA256: fixtureSHA256,
		ManifestPath: fixtureManifest, ManifestSHA: strings.Repeat("c", 64),
	})
	report := finalReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", SpecPath: testSpecPath,
		SpecSHA256: approvedSpecSHA, Routes: requiredRoutes, Projects: requiredProjects,
		ExpectedTests: 18, DesktopTests: 9, MobileTests: 9,
	}
	report.SourceGuards.NoSkip = true
	report.SourceGuards.NoOnly = true
	report.SourceGuards.NoFixme = true
	report.SourceGuards.NoLanguageSwitcher = true
	report.StateChecks.CompleteZeroVisible = true
	report.StateChecks.PartialKnownDataPreserved = true
	report.StateChecks.MissingReasonVisible = true
	report.StateChecks.UnavailableReasonVisible = true
	report.StateChecks.PausedDataAndReasonVisible = true
	report.StateChecks.RefreshRetainsPreviousData = true
	report.StateChecks.URLReloadRestoresSearch = true
	report.StateChecks.ResponsiveNoHorizontalOverflow = true
	report.I18n.CheckPassed = true
	report.I18n.Locales = []string{"zh-CN"}
	report.I18n.LocaleCount = 1
	report.I18n.TranslationKeys = 1297
	report.I18n.ExtraLocaleAbsent = true
	report.Artifacts.PlaywrightJSON = true
	report.Artifacts.StandaloneHTML = true
	report.IndependentPort = true
	writeA50JSON(t, filepath.Join(run, "a50-report.json"), report)
	cleanup := cleanupReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, Passed: true,
		ServerPID: 12345, Port: port, ServerStarted: true, ServerStopped: true, PIDTreeStopped: true,
		PortReleased: true, TestOutputRemoved: true, HTMLOutputRemoved: true, ServerLogsWritten: true,
	}
	cleanup.TestOutputCreated = true
	cleanup.HTMLOutputCreated = true
	cleanup.Residuals.PIDs = []int{}
	cleanup.Residuals.Ports = []int{}
	cleanup.Residuals.Directories = []string{}
	writeA50JSON(t, filepath.Join(run, "a50-cleanup.json"), cleanup)
	writeA50JSON(t, filepath.Join(run, "a50-secret-scan.json"), secretScanReport{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class, Status: "passed",
		Files: secretScanTargets,
	})
	writeA50Inventory(t, run, class)
	writeA50TextFiles(t, run, map[string]string{"stdout.log": "A50 passed\n", "stderr.log": ""})
	started := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	writeA50JSON(t, filepath.Join(run, "evidence.json"), wrapperEvidence{
		SchemaVersion: 1, AcceptanceID: AcceptanceID, Status: "passed", EvidenceClass: class,
		Command: canonicalCommand, WorkingDirectory: ".", StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: started.Add(time.Second).Format(time.RFC3339Nano), DurationMilliseconds: 1000,
		ExitCode: 0, Commit: "unborn", WorktreeDirty: true, FixtureManifestPath: fixtureManifest,
		FixtureManifestSHA: strings.Repeat("c", 64), StdoutLog: "stdout.log", StderrLog: "stderr.log", RequiredNoSkip: true,
	})
	return run
}

func writeA50Inventory(t *testing.T, run, class string) {
	t.Helper()
	inventory := artifactInventory{SchemaVersion: 1, AcceptanceID: AcceptanceID, EvidenceClass: class}
	for _, name := range requiredArtifacts {
		path := filepath.Join(run, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		inventory.Files = append(inventory.Files, artifactEntry{Path: name, SizeBytes: info.Size(), SHA256: digest})
	}
	writeA50JSON(t, filepath.Join(run, "a50-artifacts.json"), inventory)
}

func writeA50TextFiles(t *testing.T, run string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(run, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeA50JSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readA50JSON(t *testing.T, path string, target any) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatal(err)
	}
}
