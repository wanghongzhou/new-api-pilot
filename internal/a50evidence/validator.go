package a50evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	AcceptanceID                = "A50"
	FormalClass                 = "formal"
	DevelopmentClass            = "development"
	testSpecPath                = "web/e2e/statistics-states.spec.ts"
	approvedSpecSHA             = "38b71931dbce622dc82dbf9323836aa8ffcaf5aa475b8c87319864c3f750a40c"
	approvedPackageSHA          = "1a7799fb3dd87e6897536e4d33539c866a8d6a471add2799fd27cde4b8873683"
	approvedPlaywrightConfigSHA = "16c060617c14eefd3e70d3dc2bf15139ae7c8d389a3c43a1e022f6c06151f155"
	fixturePath                 = "testdata/design/f03-statistics.sql"
	fixtureSHA256               = "bcceaaf7d6b171014258b9d935fbb1e7cab4585b49403d760a6db373e5aabe94"
	fixtureManifest             = "testdata/design/manifest.sha256"
	maxJSONSize                 = 32 << 20
	maxArtifactSize             = 128 << 20
)

var (
	canonicalCommand = []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a50.ps1",
	}
	checkCommand            = []string{"bun", "run", "check"}
	playwrightCommandPrefix = []string{
		"bun", "x", "--no-install", "playwright", "test", "e2e/statistics-states.spec.ts",
		"--workers=2", "--retries=0", "--forbid-only",
		"--project=chromium-desktop", "--project=chromium-mobile",
		"--reporter=json,html", "--output",
	}
	requiredProjects = []string{"chromium-desktop", "chromium-mobile"}
	requiredRoutes   = []routeCase{
		{Key: "global", Path: "/statistics/global", Title: "global covers five states, refresh retention and reload"},
		{Key: "sites", Path: "/statistics/sites", Title: "sites covers five states, refresh retention and reload"},
		{Key: "customers", Path: "/statistics/customers", Title: "customers covers five states, refresh retention and reload"},
		{Key: "accounts", Path: "/statistics/accounts", Title: "accounts covers five states, refresh retention and reload"},
		{Key: "models", Path: "/statistics/models", Title: "models covers five states, refresh retention and reload"},
		{Key: "channels", Path: "/statistics/channels", Title: "channels covers five states, refresh retention and reload"},
		{Key: "site-deep-link", Path: "/sites/1/stats", Title: "site-deep-link covers five states, refresh retention and reload"},
		{Key: "customer-deep-link", Path: "/customers/7/stats", Title: "customer-deep-link covers five states, refresh retention and reload"},
		{Key: "account-deep-link", Path: "/accounts/9/stats", Title: "account-deep-link covers five states, refresh retention and reload"},
	}
	checkSteps = []string{"routes:generate", "typecheck", "lint", "format:check", "i18n:check", "build:app"}
	// The inventory is a sixteenth inner file and hashes these fifteen fixed payloads.
	requiredArtifacts = []string{
		"a50-playwright.json",
		"a50-playwright-report.html",
		"a50-playwright.stdout.log",
		"a50-playwright.stderr.log",
		"a50-check.stdout.log",
		"a50-check.stderr.log",
		"a50-server.stdout.log",
		"a50-server.stderr.log",
		"a50-check-summary.json",
		"a50-command.json",
		"a50-environment.json",
		"a50-fixture.json",
		"a50-report.json",
		"a50-cleanup.json",
		"a50-secret-scan.json",
	}
	secretScanTargets = []string{
		"a50-playwright.json",
		"a50-playwright-report.html",
		"a50-playwright.stdout.log",
		"a50-playwright.stderr.log",
		"a50-check.stdout.log",
		"a50-check.stderr.log",
		"a50-server.stdout.log",
		"a50-server.stderr.log",
		"a50-check-summary.json",
		"a50-command.json",
		"a50-environment.json",
		"a50-fixture.json",
		"a50-report.json",
		"a50-cleanup.json",
	}
	sha256Pattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern        = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	baseURLPattern       = regexp.MustCompile(`^http://127\.0\.0\.1:([0-9]{4,5})$`)
	tempDirectoryPattern = regexp.MustCompile(`^new-api-pilot-a50-[0-9a-f]+-[0-9a-f]{12}-(?:results|html)$`)
	secretPattern        = regexp.MustCompile(
		`(?i)(?:(?:authorization|access[_-]?token|webhook(?:[_-]?url)?|password|cookie|database[_-]?dsn|session[_-]?secret|encryption[_-]?key)` +
			`\s*["']?\s*[:=]\s*["']?[^\s"']{6,}|[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)`,
	)
	rawDSNPattern = regexp.MustCompile(
		`(?i)\b[A-Za-z0-9_.-]{1,64}:[^@\s]{0,256}@tcp\([A-Za-z0-9_.:-]{1,255}\)/[A-Za-z0-9_.-]{1,128}`,
	)
	urlCredentialPattern = regexp.MustCompile(
		`(?i)https?://[^\s"'<>]{0,1000}(?:access[_-]?token|token|secret|key|signature)=[^&\s"'<>]{1,512}`,
	)
	i18nOutputPattern           = regexp.MustCompile(`(?m)i18n check passed: 1 locale, ([0-9]+) keys`)
	htmlReporterSecretAllowlist = []string{
		`password:u,rawPassword:c,signed:f,encryptionStrength:r,checkPasswordOnly:o}){super({start(){Object.assign(this,{ready:new`,
		`password:I2(u,c),signed:f,strength:r-1,pending:new`,
		`password:A,strength:x,resolveReady:T,ready:D}=y;A?(await`,
		`password:u,rawPassword:c,encryptionStrength:f}){let`,
		`password:I2(u,c),strength:f-1,pending:new`,
		`password:y,strength:A,resolveReady:x,ready:T}=v;let`,
		`password=null;const`,
		`password:u,passwordVerification:c,checkPasswordOnly:f}){super({start(){Object.assign(this,{password:u,passwordVerification:c}),q2(this,u)},transform(r,o){const`,
		`password=null,v.at(-1)!=h.passwordVerification)throw`,
		`password:u,passwordVerification:c}){super({start(){Object.assign(this,{password:u,passwordVerification:c}),q2(this,u)},transform(f,r){const`,
		`password=null;const`,
		`password:H,rawPassword:j,zipCrypto:st,encryptionStrength:A&&A.strength,signed:ie(o,r,z8)&&!Y,passwordVerification:st&&(R?p>>>8&255:q>>>24&255),outputSize:E,signature:q,compressed:T!=0&&!Y,encrypted:o.encrypted&&!Y,useWebWorkers:ie(o,r,Y8),useCompressionStream:ie(o,r,L8),transferStreams:ie(o,r,G8),checkPasswordOnly:ht},config:D,streamOptions:{signal:$,size:M,onstart:L,onprogress:W,onend:et}};tt&&await`,
		`PASSWORD:lr,ERR_INVALID_SIGNATURE:ar,ERR_INVALID_UNCOMPRESSED_SIZE:sr,ERR_ITERATOR_COMPLETED_TOO_SOON:$2,ERR_LOCAL_FILE_HEADER_NOT_FOUND:Eh,ERR_OVERLAPPING_ENTRY:Sh,ERR_SPLIT_ZIP_FILE:Pf,ERR_UNSUPPORTED_COMPRESSION:_f,ERR_UNSUPPORTED_ENCRYPTION:bh,GenericReader:ah,GenericWriter:ih,Reader:sc,SplitDataReader:lh,SplitDataWriter:kf,TextWriter:a8,ZipReader:I8,configure:H2,initStream:Oi,readUint8Array:_t},Symbol.toStringTag,{value:`,
		`password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function`,
	}
)

type routeCase struct {
	Key   string `json:"key"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

type checkSummary struct {
	SchemaVersion   int      `json:"schema_version"`
	AcceptanceID    string   `json:"acceptance_id"`
	Status          string   `json:"status"`
	Command         []string `json:"command"`
	ExitCode        int      `json:"exit_code"`
	Steps           []string `json:"steps"`
	RoutesGenerate  bool     `json:"routes_generate"`
	Typecheck       bool     `json:"typecheck"`
	Lint            bool     `json:"lint"`
	FormatCheck     bool     `json:"format_check"`
	I18nCheck       bool     `json:"i18n_check"`
	BuildApp        bool     `json:"build_app"`
	Locales         []string `json:"locales"`
	LocaleCount     int      `json:"locale_count"`
	TranslationKeys int      `json:"translation_keys"`
	StdoutPath      string   `json:"stdout_path"`
	StderrPath      string   `json:"stderr_path"`
}

type commandReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	EvidenceClass string   `json:"evidence_class"`
	WorkingDir    string   `json:"working_directory"`
	Check         []string `json:"check"`
	Server        []string `json:"server"`
	Playwright    []string `json:"playwright"`
}

type viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type environmentReport struct {
	SchemaVersion          int      `json:"schema_version"`
	AcceptanceID           string   `json:"acceptance_id"`
	EvidenceClass          string   `json:"evidence_class"`
	Commit                 string   `json:"commit"`
	WorktreeDirty          bool     `json:"worktree_dirty"`
	OS                     string   `json:"os"`
	Architecture           string   `json:"architecture"`
	BunVersion             string   `json:"bun_version"`
	PlaywrightVersion      string   `json:"playwright_version"`
	BaseURL                string   `json:"base_url"`
	Port                   int      `json:"port"`
	ServerPID              int      `json:"server_pid"`
	Workers                int      `json:"workers"`
	Retries                int      `json:"retries"`
	Projects               []string `json:"projects"`
	DesktopViewport        viewport `json:"desktop_viewport"`
	MobileViewport         viewport `json:"mobile_viewport"`
	SharedPort5173Used     bool     `json:"shared_port_5173_used"`
	TestOutputDirectory    string   `json:"test_output_directory"`
	HTMLOutputDirectory    string   `json:"html_output_directory"`
	SpecPath               string   `json:"spec_path"`
	SpecSHA256             string   `json:"spec_sha256"`
	LocalePath             string   `json:"locale_path"`
	LocaleSHA256           string   `json:"locale_sha256"`
	BunLockPath            string   `json:"bun_lock_path"`
	BunLockSHA256          string   `json:"bun_lock_sha256"`
	PackagePath            string   `json:"package_path"`
	PackageSHA256          string   `json:"package_sha256"`
	PlaywrightConfigPath   string   `json:"playwright_config_path"`
	PlaywrightConfigSHA256 string   `json:"playwright_config_sha256"`
}

type fixtureReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	FixtureID     string `json:"fixture_id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	ManifestPath  string `json:"manifest_path"`
	ManifestSHA   string `json:"manifest_sha256"`
}

type finalReport struct {
	SchemaVersion   int         `json:"schema_version"`
	AcceptanceID    string      `json:"acceptance_id"`
	Status          string      `json:"status"`
	SpecPath        string      `json:"spec_path"`
	SpecSHA256      string      `json:"spec_sha256"`
	Routes          []routeCase `json:"routes"`
	Projects        []string    `json:"projects"`
	ExpectedTests   int         `json:"expected_tests"`
	DesktopTests    int         `json:"desktop_tests"`
	MobileTests     int         `json:"mobile_tests"`
	Unexpected      int         `json:"unexpected"`
	Flaky           int         `json:"flaky"`
	Skipped         int         `json:"skipped"`
	RetriesObserved int         `json:"retries_observed"`
	SourceGuards    struct {
		NoSkip             bool `json:"no_skip"`
		NoOnly             bool `json:"no_only"`
		NoFixme            bool `json:"no_fixme"`
		NoLanguageSwitcher bool `json:"no_language_switcher"`
	} `json:"source_guards"`
	StateChecks struct {
		CompleteZeroVisible            bool `json:"complete_zero_visible"`
		PartialKnownDataPreserved      bool `json:"partial_known_data_preserved"`
		MissingReasonVisible           bool `json:"missing_reason_visible"`
		UnavailableReasonVisible       bool `json:"unavailable_reason_visible"`
		PausedDataAndReasonVisible     bool `json:"paused_data_and_reason_visible"`
		RefreshRetainsPreviousData     bool `json:"refresh_retains_previous_data"`
		URLReloadRestoresSearch        bool `json:"url_reload_restores_search"`
		ResponsiveNoHorizontalOverflow bool `json:"responsive_no_horizontal_overflow"`
	} `json:"state_checks"`
	I18n struct {
		CheckPassed       bool     `json:"check_passed"`
		Locales           []string `json:"locales"`
		LocaleCount       int      `json:"locale_count"`
		TranslationKeys   int      `json:"translation_keys"`
		ExtraLocaleAbsent bool     `json:"extra_locale_absent"`
	} `json:"i18n"`
	Artifacts struct {
		PlaywrightJSON bool `json:"playwright_json"`
		StandaloneHTML bool `json:"standalone_html"`
	} `json:"artifacts"`
	IndependentPort bool `json:"independent_port"`
}

type cleanupReport struct {
	SchemaVersion         int    `json:"schema_version"`
	AcceptanceID          string `json:"acceptance_id"`
	EvidenceClass         string `json:"evidence_class"`
	Passed                bool   `json:"passed"`
	ServerPID             int    `json:"server_pid"`
	Port                  int    `json:"port"`
	ServerStarted         bool   `json:"server_started"`
	ServerStopped         bool   `json:"server_stopped"`
	PIDTreeStopped        bool   `json:"pid_tree_stopped"`
	PortReleased          bool   `json:"port_released"`
	TestOutputCreated     bool   `json:"test_output_created"`
	TestOutputRemoved     bool   `json:"test_output_removed"`
	HTMLOutputCreated     bool   `json:"html_output_created"`
	HTMLOutputRemoved     bool   `json:"html_output_removed"`
	ServerLogsWritten     bool   `json:"server_logs_written"`
	SharedPort5173Touched bool   `json:"shared_port_5173_touched"`
	Residuals             struct {
		PIDs        []int    `json:"pids"`
		Ports       []int    `json:"ports"`
		Directories []string `json:"directories"`
	} `json:"residuals"`
}

type secretScanReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	EvidenceClass string   `json:"evidence_class"`
	Status        string   `json:"status"`
	Files         []string `json:"files"`
	Matches       int      `json:"matches"`
}

type artifactInventory struct {
	SchemaVersion int             `json:"schema_version"`
	AcceptanceID  string          `json:"acceptance_id"`
	EvidenceClass string          `json:"evidence_class"`
	Files         []artifactEntry `json:"files"`
}

type artifactEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type wrapperEvidence struct {
	SchemaVersion        int      `json:"schema_version"`
	AcceptanceID         string   `json:"acceptance_id"`
	Status               string   `json:"status"`
	EvidenceClass        string   `json:"evidence_class"`
	Command              []string `json:"command"`
	WorkingDirectory     string   `json:"working_directory"`
	StartedAt            string   `json:"started_at"`
	FinishedAt           string   `json:"finished_at"`
	DurationMilliseconds int64    `json:"duration_milliseconds"`
	ExitCode             int      `json:"exit_code"`
	Commit               string   `json:"commit"`
	WorktreeDirty        bool     `json:"worktree_dirty"`
	FixtureManifestPath  string   `json:"fixture_manifest_path"`
	FixtureManifestSHA   string   `json:"fixture_manifest_sha256"`
	StdoutLog            string   `json:"stdout_log"`
	StderrLog            string   `json:"stderr_log"`
	RequiredNoSkip       bool     `json:"required_no_skip"`
}

type playwrightReport struct {
	Config struct {
		ForbidOnly bool `json:"forbidOnly"`
		Workers    int  `json:"workers"`
		Projects   []struct {
			OutputDir  string `json:"outputDir"`
			RepeatEach int    `json:"repeatEach"`
			Retries    int    `json:"retries"`
			ID         string `json:"id"`
			Name       string `json:"name"`
			TestDir    string `json:"testDir"`
		} `json:"projects"`
	} `json:"config"`
	Suites []playwrightSuite `json:"suites"`
	Errors []json.RawMessage `json:"errors"`
	Stats  struct {
		StartTime  string  `json:"startTime"`
		Duration   float64 `json:"duration"`
		Expected   int     `json:"expected"`
		Unexpected int     `json:"unexpected"`
		Flaky      int     `json:"flaky"`
		Skipped    int     `json:"skipped"`
	} `json:"stats"`
}

type playwrightSuite struct {
	Title  string            `json:"title"`
	File   string            `json:"file"`
	Specs  []playwrightSpec  `json:"specs"`
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSpec struct {
	Tags   []string         `json:"tags"`
	Title  string           `json:"title"`
	OK     bool             `json:"ok"`
	Tests  []playwrightTest `json:"tests"`
	ID     string           `json:"id"`
	File   string           `json:"file"`
	Line   int              `json:"line"`
	Column int              `json:"column"`
}

type playwrightTest struct {
	Annotations    []json.RawMessage      `json:"annotations"`
	ExpectedStatus string                 `json:"expectedStatus"`
	ProjectName    string                 `json:"projectName"`
	ProjectID      string                 `json:"projectId"`
	Results        []playwrightTestResult `json:"results"`
	Status         string                 `json:"status"`
}

type playwrightTestResult struct {
	Status      string            `json:"status"`
	Duration    float64           `json:"duration"`
	Error       json.RawMessage   `json:"error"`
	Errors      []json.RawMessage `json:"errors"`
	Stdout      []json.RawMessage `json:"stdout"`
	Stderr      []json.RawMessage `json:"stderr"`
	Retry       int               `json:"retry"`
	Attachments []json.RawMessage `json:"attachments"`
	Annotations []json.RawMessage `json:"annotations"`
}

type playwrightFacts struct {
	Expected        int
	Desktop         int
	Mobile          int
	Unexpected      int
	Flaky           int
	Skipped         int
	RetriesObserved int
}

func Supports(acceptanceID string) bool { return acceptanceID == AcceptanceID }

func Classify(acceptanceID, evidenceRoot string, command []string) (string, error) {
	if !Supports(acceptanceID) {
		return "", nil
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	class := ""
	switch cleaned {
	case "artifacts/acceptance":
		class = FormalClass
	case "artifacts/smoke":
		class = DevelopmentClass
	default:
		return "", errors.New("A50 evidence root must be canonical artifacts/acceptance or artifacts/smoke")
	}
	if err := ValidateCanonicalCommand(command, class); err != nil {
		return "", err
	}
	return class, nil
}

func ValidateCanonicalCommand(command []string, class string) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A50 evidence class %q", class)
	}
	if !equalStrings(command, canonicalCommand) {
		return fmt.Errorf("A50 evidence requires canonical command %q", strings.Join(canonicalCommand, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, class string) error {
	return validateInnerArtifacts(runDirectory, class, false)
}

func validateInnerArtifacts(runDirectory, class string, wrapperEvidencePresent bool) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A50 evidence class %q", class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	if err := validateExactFiles(runDirectory, wrapperEvidencePresent); err != nil {
		return err
	}

	var summary checkSummary
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-check-summary.json"), &summary); err != nil {
		return fmt.Errorf("validate A50 check summary: %w", err)
	}
	if err := validateCheckSummary(runDirectory, summary); err != nil {
		return err
	}

	facts, err := validatePlaywrightJSON(filepath.Join(runDirectory, "a50-playwright.json"))
	if err != nil {
		return err
	}
	if err := validateHTML(filepath.Join(runDirectory, "a50-playwright-report.html")); err != nil {
		return err
	}

	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-environment.json"), &environment); err != nil {
		return fmt.Errorf("validate A50 environment report: %w", err)
	}
	if err := validateEnvironment(environment, class); err != nil {
		return err
	}

	var command commandReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-command.json"), &command); err != nil {
		return fmt.Errorf("validate A50 command report: %w", err)
	}
	if err := validateCommand(command, environment, class); err != nil {
		return err
	}

	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-fixture.json"), &fixture); err != nil {
		return fmt.Errorf("validate A50 fixture report: %w", err)
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != AcceptanceID || fixture.FixtureID != "F03" ||
		fixture.Path != fixturePath || fixture.SHA256 != fixtureSHA256 || fixture.ManifestPath != fixtureManifest ||
		!sha256Pattern.MatchString(fixture.ManifestSHA) {
		return errors.New("A50 fixture report contract is invalid")
	}

	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-report.json"), &report); err != nil {
		return fmt.Errorf("validate A50 final report: %w", err)
	}
	if err := validateFinalReport(report, summary, facts); err != nil {
		return err
	}

	if err := validateCleanup(runDirectory, class, environment); err != nil {
		return err
	}
	if err := validateSecretScan(runDirectory, class); err != nil {
		return err
	}
	if err := validateInventory(runDirectory, class); err != nil {
		return err
	}
	for _, name := range append(append([]string{}, requiredArtifacts...), "a50-artifacts.json") {
		if err := validateNoSecret(filepath.Join(runDirectory, name)); err != nil {
			return err
		}
	}
	return nil
}

func ValidateWrapperLogs(runDirectory string) error {
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := validateNoSecret(filepath.Join(runDirectory, name)); err != nil {
			return fmt.Errorf("validate A50 wrapper log %s: %w", name, err)
		}
	}
	return nil
}

func ValidateRunDirectory(runDirectory, class string) error {
	if err := validateInnerArtifacts(runDirectory, class, true); err != nil {
		return err
	}
	if err := ValidateWrapperLogs(runDirectory); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return fmt.Errorf("validate A50 wrapper evidence: %w", err)
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != AcceptanceID || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || ValidateCanonicalCommand(evidence.Command, class) != nil ||
		evidence.WorkingDirectory != "." || evidence.ExitCode != 0 || !evidence.RequiredNoSkip ||
		(evidence.Commit != "unborn" && !commitPattern.MatchString(evidence.Commit)) ||
		evidence.FixtureManifestPath != fixtureManifest || !sha256Pattern.MatchString(evidence.FixtureManifestSHA) ||
		evidence.StdoutLog != "stdout.log" || evidence.StderrLog != "stderr.log" ||
		startErr != nil || finishErr != nil || finished.Before(started) || evidence.DurationMilliseconds < 0 {
		return errors.New("A50 wrapper evidence contract is invalid")
	}
	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-environment.json"), &environment); err != nil {
		return err
	}
	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-fixture.json"), &fixture); err != nil {
		return err
	}
	if environment.Commit != evidence.Commit || environment.WorktreeDirty != evidence.WorktreeDirty ||
		fixture.ManifestPath != evidence.FixtureManifestPath || fixture.ManifestSHA != evidence.FixtureManifestSHA {
		return errors.New("A50 wrapper metadata is not bound to environment and fixture evidence")
	}
	return validateNoSecret(filepath.Join(runDirectory, "evidence.json"))
}

func ValidateEvidenceRoot(root, class string) error {
	if class != FormalClass {
		return errors.New("A50 manifest evidence must be formal")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	failures := make([]string, 0, len(names))
	for _, name := range names {
		if err := ValidateRunDirectory(filepath.Join(root, name), class); err == nil {
			return nil
		} else {
			failures = append(failures, name+": "+err.Error())
		}
	}
	if len(failures) == 0 {
		return errors.New("no A50 evidence run directories exist")
	}
	return fmt.Errorf("no valid A50 formal run: %s", strings.Join(failures, "; "))
}

func validateCheckSummary(runDirectory string, summary checkSummary) error {
	if summary.SchemaVersion != 1 || summary.AcceptanceID != AcceptanceID || summary.Status != "passed" ||
		!equalStrings(summary.Command, checkCommand) || summary.ExitCode != 0 || !equalStrings(summary.Steps, checkSteps) ||
		!summary.RoutesGenerate || !summary.Typecheck || !summary.Lint || !summary.FormatCheck ||
		!summary.I18nCheck || !summary.BuildApp || !equalStrings(summary.Locales, []string{"zh-CN"}) ||
		summary.LocaleCount != 1 || summary.TranslationKeys < 1000 || summary.StdoutPath != "a50-check.stdout.log" ||
		summary.StderrPath != "a50-check.stderr.log" {
		return errors.New("A50 bun check summary contract is invalid")
	}
	payload, err := readBounded(filepath.Join(runDirectory, summary.StdoutPath), maxArtifactSize)
	if err != nil {
		return err
	}
	matches := i18nOutputPattern.FindSubmatch(payload)
	if len(matches) != 2 {
		return errors.New("A50 bun check stdout lacks the single zh-CN i18n result")
	}
	keys, err := strconv.Atoi(string(matches[1]))
	if err != nil || keys != summary.TranslationKeys {
		return errors.New("A50 i18n key count differs between stdout and summary")
	}
	return nil
}

func validateCommand(command commandReport, environment environmentReport, class string) error {
	if command.SchemaVersion != 1 || command.AcceptanceID != AcceptanceID || command.EvidenceClass != class ||
		command.WorkingDir != "web" || !equalStrings(command.Check, checkCommand) {
		return errors.New("A50 command report header is invalid")
	}
	wantServer := []string{"bun", "run", "dev", "--", "--host", "127.0.0.1", "--port", strconv.Itoa(environment.Port)}
	if !equalStrings(command.Server, wantServer) {
		return errors.New("A50 independent server command is invalid")
	}
	wantPlaywright := append(append([]string{}, playwrightCommandPrefix...), environment.TestOutputDirectory)
	if !equalStrings(command.Playwright, wantPlaywright) {
		return errors.New("A50 Playwright command is invalid")
	}
	return nil
}

func validateEnvironment(environment environmentReport, class string) error {
	match := baseURLPattern.FindStringSubmatch(environment.BaseURL)
	port := 0
	if len(match) == 2 {
		port, _ = strconv.Atoi(match[1])
	}
	if environment.SchemaVersion != 1 || environment.AcceptanceID != AcceptanceID || environment.EvidenceClass != class ||
		(environment.Commit != "unborn" && !commitPattern.MatchString(environment.Commit)) ||
		environment.OS == "" || environment.Architecture == "" || environment.BunVersion != "1.3.13" ||
		environment.PlaywrightVersion != "1.61.1" || port != environment.Port || port < 1024 || port > 65535 || port == 5173 ||
		environment.ServerPID <= 0 || environment.Workers != 2 || environment.Retries != 0 ||
		!equalStrings(environment.Projects, requiredProjects) || environment.DesktopViewport != (viewport{Width: 1440, Height: 900}) ||
		environment.MobileViewport != (viewport{Width: 390, Height: 844}) || environment.SharedPort5173Used ||
		environment.SpecPath != testSpecPath || environment.SpecSHA256 != approvedSpecSHA ||
		environment.LocalePath != "web/src/i18n/locales/zh-CN.json" || !sha256Pattern.MatchString(environment.LocaleSHA256) ||
		environment.BunLockPath != "web/bun.lock" || !sha256Pattern.MatchString(environment.BunLockSHA256) {
		return errors.New("A50 environment contract is invalid")
	}
	if environment.PackagePath != "web/package.json" || environment.PackageSHA256 != approvedPackageSHA ||
		environment.PlaywrightConfigPath != "web/playwright.config.ts" ||
		environment.PlaywrightConfigSHA256 != approvedPlaywrightConfigSHA {
		return errors.New("A50 environment contract is invalid")
	}
	for _, directory := range []string{environment.TestOutputDirectory, environment.HTMLOutputDirectory} {
		if !isAbsoluteEvidencePath(directory) || !tempDirectoryPattern.MatchString(evidencePathBase(directory)) {
			return fmt.Errorf("A50 temporary directory %q is invalid", directory)
		}
	}
	if environment.TestOutputDirectory == environment.HTMLOutputDirectory {
		return errors.New("A50 temporary output directories are not independent")
	}
	return nil
}

func isAbsoluteEvidencePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	return regexp.MustCompile(`^[A-Za-z]:[\\/]`).MatchString(path) || strings.HasPrefix(path, `\\`)
}

func evidencePathBase(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")
	return normalized[strings.LastIndex(normalized, "/")+1:]
}

func validateFinalReport(report finalReport, summary checkSummary, facts playwrightFacts) error {
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.Status != "passed" ||
		report.SpecPath != testSpecPath || report.SpecSHA256 != approvedSpecSHA || !equalRoutes(report.Routes, requiredRoutes) ||
		!equalStrings(report.Projects, requiredProjects) || report.ExpectedTests != 18 || report.DesktopTests != 9 ||
		report.MobileTests != 9 || report.Unexpected != 0 || report.Flaky != 0 || report.Skipped != 0 ||
		report.RetriesObserved != 0 || facts.Expected != report.ExpectedTests || facts.Desktop != report.DesktopTests ||
		facts.Mobile != report.MobileTests || facts.Unexpected != report.Unexpected || facts.Flaky != report.Flaky ||
		facts.Skipped != report.Skipped || facts.RetriesObserved != report.RetriesObserved ||
		!report.SourceGuards.NoSkip || !report.SourceGuards.NoOnly || !report.SourceGuards.NoFixme ||
		!report.SourceGuards.NoLanguageSwitcher || !report.StateChecks.CompleteZeroVisible ||
		!report.StateChecks.PartialKnownDataPreserved || !report.StateChecks.MissingReasonVisible ||
		!report.StateChecks.UnavailableReasonVisible || !report.StateChecks.PausedDataAndReasonVisible ||
		!report.StateChecks.RefreshRetainsPreviousData || !report.StateChecks.URLReloadRestoresSearch ||
		!report.StateChecks.ResponsiveNoHorizontalOverflow || !report.I18n.CheckPassed ||
		!equalStrings(report.I18n.Locales, []string{"zh-CN"}) || report.I18n.LocaleCount != 1 ||
		report.I18n.TranslationKeys != summary.TranslationKeys || !report.I18n.ExtraLocaleAbsent ||
		!report.Artifacts.PlaywrightJSON || !report.Artifacts.StandaloneHTML || !report.IndependentPort {
		return errors.New("A50 final report contract is invalid")
	}
	return nil
}

func validatePlaywrightJSON(path string) (playwrightFacts, error) {
	var report playwrightReport
	payload, err := readBounded(path, maxJSONSize)
	if err != nil {
		return playwrightFacts{}, err
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		return playwrightFacts{}, fmt.Errorf("decode A50 Playwright JSON: %w", err)
	}
	if !report.Config.ForbidOnly || report.Config.Workers != 2 || len(report.Config.Projects) != len(requiredProjects) ||
		len(report.Errors) != 0 || report.Stats.Expected != 18 || report.Stats.Unexpected != 0 ||
		report.Stats.Flaky != 0 || report.Stats.Skipped != 0 || !validPositiveDuration(report.Stats.Duration) {
		return playwrightFacts{}, errors.New("A50 Playwright config/stats contract is invalid")
	}
	projectNames := make([]string, 0, len(report.Config.Projects))
	for _, project := range report.Config.Projects {
		if project.Name == "" || project.ID == "" || project.Retries != 0 || project.RepeatEach != 1 ||
			!strings.HasSuffix(filepath.ToSlash(project.TestDir), "/web/e2e") {
			return playwrightFacts{}, errors.New("A50 Playwright project contract is invalid")
		}
		projectNames = append(projectNames, project.Name)
	}
	if !equalStrings(projectNames, requiredProjects) {
		return playwrightFacts{}, errors.New("A50 Playwright projects are not the exact desktop/mobile pair")
	}

	var specs []playwrightSpec
	collectPlaywrightSpecs(report.Suites, &specs)
	if len(specs) != len(requiredRoutes)*len(requiredProjects) {
		return playwrightFacts{}, fmt.Errorf("A50 Playwright report has %d specs, want %d", len(specs), len(requiredRoutes)*len(requiredProjects))
	}
	wantedTitles := make(map[string]struct{}, len(requiredRoutes))
	for _, route := range requiredRoutes {
		wantedTitles[route.Title] = struct{}{}
	}
	seenCombinations := make(map[string]struct{}, len(specs))
	titleCounts := make(map[string]int, len(requiredRoutes))
	facts := playwrightFacts{Expected: report.Stats.Expected, Unexpected: report.Stats.Unexpected, Flaky: report.Stats.Flaky, Skipped: report.Stats.Skipped}
	for _, spec := range specs {
		if _, wanted := wantedTitles[spec.Title]; !wanted || !spec.OK || spec.ID == "" || spec.Line <= 0 || spec.Column <= 0 ||
			evidencePathBase(spec.File) != "statistics-states.spec.ts" || len(spec.Tests) != 1 || len(spec.Tags) != 0 {
			return playwrightFacts{}, fmt.Errorf("A50 Playwright spec %q is invalid", spec.Title)
		}
		titleCounts[spec.Title]++
		for _, test := range spec.Tests {
			if test.ExpectedStatus != "passed" || test.Status != "expected" || len(test.Annotations) != 0 ||
				len(test.Results) != 1 || (test.ProjectName != requiredProjects[0] && test.ProjectName != requiredProjects[1]) ||
				test.ProjectID == "" {
				return playwrightFacts{}, fmt.Errorf("A50 Playwright test for %q/%q is invalid", spec.Title, test.ProjectName)
			}
			combination := spec.Title + "\x00" + test.ProjectName
			if _, duplicate := seenCombinations[combination]; duplicate {
				return playwrightFacts{}, fmt.Errorf("A50 Playwright route/project combination is duplicated for %q/%q", spec.Title, test.ProjectName)
			}
			seenCombinations[combination] = struct{}{}
			result := test.Results[0]
			if result.Status != "passed" || !validPositiveDuration(result.Duration) || result.Retry != 0 || !jsonNull(result.Error) ||
				len(result.Errors) != 0 || len(result.Stdout) != 0 || len(result.Stderr) != 0 ||
				len(result.Attachments) != 0 || len(result.Annotations) != 0 {
				return playwrightFacts{}, fmt.Errorf("A50 Playwright result for %q/%q is invalid", spec.Title, test.ProjectName)
			}
			if test.ProjectName == requiredProjects[0] {
				facts.Desktop++
			} else {
				facts.Mobile++
			}
			facts.RetriesObserved += result.Retry
		}
	}
	for title := range wantedTitles {
		if titleCounts[title] != len(requiredProjects) {
			return playwrightFacts{}, fmt.Errorf("A50 Playwright route %q does not have both projects", title)
		}
	}
	if len(seenCombinations) != 18 || facts.Desktop != 9 || facts.Mobile != 9 || facts.RetriesObserved != 0 {
		return playwrightFacts{}, errors.New("A50 Playwright route/project matrix is incomplete")
	}
	return facts, nil
}

func collectPlaywrightSpecs(suites []playwrightSuite, target *[]playwrightSpec) {
	for _, suite := range suites {
		*target = append(*target, suite.Specs...)
		collectPlaywrightSpecs(suite.Suites, target)
	}
}

func validateHTML(path string) error {
	payload, err := readBounded(path, maxArtifactSize)
	if err != nil {
		return fmt.Errorf("validate A50 HTML report: %w", err)
	}
	if len(payload) < 100_000 || !strings.Contains(strings.ToLower(string(payload[:min(len(payload), 1_000_000)])), "playwright test report") {
		return errors.New("A50 standalone Playwright HTML report is invalid")
	}
	return nil
}

func validateCleanup(runDirectory, class string, environment environmentReport) error {
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate A50 cleanup report: %w", err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != AcceptanceID || cleanup.EvidenceClass != class ||
		!cleanup.Passed || cleanup.ServerPID != environment.ServerPID || cleanup.Port != environment.Port ||
		!cleanup.ServerStarted || !cleanup.ServerStopped || !cleanup.PIDTreeStopped || !cleanup.PortReleased ||
		!cleanup.TestOutputCreated || !cleanup.TestOutputRemoved || !cleanup.HTMLOutputCreated ||
		!cleanup.HTMLOutputRemoved || !cleanup.ServerLogsWritten ||
		cleanup.SharedPort5173Touched || cleanup.Residuals.PIDs == nil || cleanup.Residuals.Ports == nil ||
		cleanup.Residuals.Directories == nil || len(cleanup.Residuals.PIDs) != 0 || len(cleanup.Residuals.Ports) != 0 ||
		len(cleanup.Residuals.Directories) != 0 {
		return errors.New("A50 cleanup report contract is invalid")
	}
	return nil
}

func validateSecretScan(runDirectory, class string) error {
	var report secretScanReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-secret-scan.json"), &report); err != nil {
		return fmt.Errorf("validate A50 secret scan report: %w", err)
	}
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.EvidenceClass != class ||
		report.Status != "passed" || report.Matches != 0 || !equalStrings(report.Files, secretScanTargets) {
		return errors.New("A50 secret scan report contract is invalid")
	}
	return nil
}

func validateInventory(runDirectory, class string) error {
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, "a50-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate A50 artifact inventory: %w", err)
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != AcceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A50 artifact inventory contract is invalid")
	}
	wanted := make(map[string]struct{}, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for _, entry := range inventory.Files {
		if _, exists := wanted[entry.Path]; !exists || filepath.IsAbs(entry.Path) || filepath.Base(entry.Path) != entry.Path {
			return fmt.Errorf("A50 inventory contains unexpected path %q", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("A50 inventory repeats %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		path := filepath.Join(runDirectory, entry.Path)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes || entry.SizeBytes < 0 {
			return fmt.Errorf("A50 inventory size mismatch for %q", entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 || !sha256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("A50 inventory SHA-256 mismatch for %q", entry.Path)
		}
	}
	if len(seen) != len(wanted) {
		return errors.New("A50 artifact inventory is incomplete")
	}
	return nil
}

func validateExactFiles(runDirectory string, wrapperEvidencePresent bool) error {
	wanted := make(map[string]struct{}, len(requiredArtifacts)+4)
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	for _, name := range []string{"a50-artifacts.json", "stdout.log", "stderr.log"} {
		wanted[name] = struct{}{}
	}
	if wrapperEvidencePresent {
		wanted["evidence.json"] = struct{}{}
	}
	entries, err := os.ReadDir(runDirectory)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("A50 evidence directory contains non-regular entry %q", entry.Name())
		}
		if _, allowed := wanted[entry.Name()]; !allowed {
			return fmt.Errorf("A50 evidence directory contains unexpected file %q", entry.Name())
		}
		seen[entry.Name()] = struct{}{}
	}
	for name := range wanted {
		if _, exists := seen[name]; !exists {
			return fmt.Errorf("A50 evidence directory is missing %q", name)
		}
	}
	return nil
}

func validateNoSecret(path string) error {
	payload, err := readBounded(path, maxArtifactSize)
	if err != nil {
		return err
	}
	forbidden := containsForbiddenSecret(payload)
	if filepath.Base(path) == "a50-playwright-report.html" {
		forbidden = !matchesHTMLReporterSecretAllowlist(payload)
	}
	if forbidden {
		return fmt.Errorf("A50 evidence file %s contains a forbidden secret pattern", filepath.Base(path))
	}
	return nil
}

func containsForbiddenSecret(payload []byte) bool {
	return secretPattern.Match(payload) || rawDSNPattern.Match(payload) || urlCredentialPattern.Match(payload)
}

func matchesHTMLReporterSecretAllowlist(payload []byte) bool {
	if rawDSNPattern.Match(payload) || urlCredentialPattern.Match(payload) {
		return false
	}
	expected := make(map[string]int, len(htmlReporterSecretAllowlist))
	for _, allowed := range htmlReporterSecretAllowlist {
		expected[allowed]++
	}
	seen := make(map[string]int, len(expected))
	matches := secretPattern.FindAll(payload, -1)
	if len(matches) != len(htmlReporterSecretAllowlist) {
		return false
	}
	for _, match := range matches {
		value := string(match)
		allowedCount, allowed := expected[value]
		if !allowed || seen[value] >= allowedCount {
			return false
		}
		seen[value]++
	}
	for value, count := range expected {
		if seen[value] != count {
			return false
		}
	}
	return true
}

func validPositiveDuration(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func jsonNull(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed == "" || trimmed == "null"
}

func decodeJSONFile(path string, target any) error {
	payload, err := readBounded(path, maxJSONSize)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON file contains trailing data")
	}
	return nil
}

func readBounded(path string, maximum int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file size %d exceeds contract", info.Size())
	}
	return os.ReadFile(path)
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("A50 evidence path is not a directory")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalRoutes(left, right []routeCase) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
