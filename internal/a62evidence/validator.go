package a62evidence

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	AcceptanceID = "A62"
	FormalClass  = "formal"
	maxJSONSize  = 8 * 1024 * 1024
	maxLogSize   = 16 * 1024 * 1024
)

var (
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	secretPattern     = regexp.MustCompile(`(?i)(?:TEST_DATABASE_DSN\s*=|\b[^:\s]+:[^@\s]*@tcp\([^)\s]+\)/[^\s]+|https?://[^/@\s]+@)`)
	requiredArtifacts = []string{
		"a62-test.jsonl",
		"a62-test.stderr.log",
		"a62-test-summary.json",
		"a62-command.json",
		"a62-environment.json",
		"a62-fixture.json",
		"a62-report.json",
		"a62-cleanup.json",
	}
	canonicalCommand = []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a62.ps1",
	}
	innerCommand = []string{
		"go", "test", "-json", "./tests/integration", "-run", "^TestA62ResourceMinuteRetention$", "-count=1", "-timeout=6m",
	}
)

type retentionTableResult struct {
	Scanned                     int   `json:"scanned"`
	Deleted                     int64 `json:"deleted"`
	SkippedUnfinalized          int   `json:"skipped_unfinalized"`
	SkippedMissingHourly        int   `json:"skipped_missing_hourly"`
	SkippedDailyNotFinal        int   `json:"skipped_daily_not_final"`
	PendingRows                 bool  `json:"pending_rows"`
	BlockedDiagnosticsTruncated bool  `json:"blocked_diagnostics_truncated"`
	Batches                     int   `json:"batches"`
	Complete                    bool  `json:"complete"`
}

type retentionResult struct {
	RetentionDays int                  `json:"retention_days"`
	Cutoff        int64                `json:"cutoff"`
	Instance      retentionTableResult `json:"instance"`
	Site          retentionTableResult `json:"site"`
	Complete      bool                 `json:"complete"`
}

type starvationProof struct {
	BlockedPrefixRows                  int             `json:"blocked_prefix_rows"`
	BatchSize                          int             `json:"batch_size"`
	MaximumBatches                     int             `json:"maximum_batches"`
	FirstRun                           retentionResult `json:"first_run"`
	RestartRun                         retentionResult `json:"restart_run"`
	FinalRun                           retentionResult `json:"final_run"`
	EligibleDeletedBehindBlockedPrefix bool            `json:"eligible_deleted_behind_blocked_prefix"`
	RestartContinuationProved          bool            `json:"restart_continuation_proved"`
}

type finalReport struct {
	SchemaVersion              int             `json:"schema_version"`
	AcceptanceID               string          `json:"acceptance_id"`
	Status                     string          `json:"status"`
	FixturePath                string          `json:"fixture_path"`
	FixtureSHA256              string          `json:"fixture_sha256"`
	FixedNowUnix               int64           `json:"fixed_now_unix"`
	RetentionDays              int             `json:"retention_days"`
	Cutoff                     int64           `json:"cutoff"`
	BatchSize                  int             `json:"batch_size"`
	InitialRowsPerTable        int64           `json:"initial_rows_per_table"`
	FirstRun                   retentionResult `json:"first_run"`
	SecondRun                  retentionResult `json:"second_run"`
	IdempotentRun              retentionResult `json:"idempotent_run"`
	RowsAfterFirstRun          int64           `json:"rows_after_first_run"`
	RowsAfterFinalRun          int64           `json:"rows_after_final_run"`
	ProtectedAggregateSHA256   string          `json:"protected_aggregate_sha256"`
	BusinessFactsPreserved     bool            `json:"business_facts_preserved"`
	ExactBoundaryPreserved     bool            `json:"exact_boundary_preserved"`
	MissingHourlyBlocked       bool            `json:"missing_hourly_blocked"`
	DailyNotFinalBlocked       bool            `json:"daily_not_final_blocked"`
	InvalidRetentionRejected   bool            `json:"invalid_retention_rejected"`
	HourlyDailyValuesUnchanged bool            `json:"hourly_daily_values_unchanged"`
	StarvationProof            starvationProof `json:"starvation_proof"`
}

type testSummary struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	TargetTest    string `json:"target_test"`
	Package       string `json:"package"`
	PassEvents    int    `json:"pass_events"`
	FailEvents    int    `json:"fail_events"`
	SkipEvents    int    `json:"skip_events"`
	NoTests       bool   `json:"no_tests"`
	JSONLines     int    `json:"json_lines"`
	JSONPath      string `json:"json_path"`
	StderrPath    string `json:"stderr_path"`
}

type commandReport struct {
	SchemaVersion    int      `json:"schema_version"`
	AcceptanceID     string   `json:"acceptance_id"`
	EvidenceClass    string   `json:"evidence_class"`
	TargetTest       string   `json:"target_test"`
	WorkingDirectory string   `json:"working_directory"`
	Command          []string `json:"command"`
	GoImage          string   `json:"go_image"`
	MySQLImage       string   `json:"mysql_image"`
}

type environmentReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	EvidenceClass string `json:"evidence_class"`
	Commit        string `json:"commit"`
	WorktreeDirty bool   `json:"worktree_dirty"`
	IsolatedGuard bool   `json:"isolated_guard"`
	MySQL         struct {
		Version              string `json:"version"`
		TransactionIsolation string `json:"transaction_isolation"`
		CharacterSetServer   string `json:"character_set_server"`
		CollationServer      string `json:"collation_server"`
		TimeZone             string `json:"time_zone"`
	} `json:"mysql"`
	Network struct {
		Internal  bool     `json:"internal"`
		HostPorts []string `json:"host_ports"`
	} `json:"network"`
	Database string `json:"database"`
}

type fixtureReport struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	FixtureID     string `json:"fixture_id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	FixedNowUnix  int64  `json:"fixed_now_unix"`
	RetentionDays int    `json:"retention_days"`
}

type cleanupReport struct {
	SchemaVersion   int    `json:"schema_version"`
	AcceptanceID    string `json:"acceptance_id"`
	EvidenceClass   string `json:"evidence_class"`
	Passed          bool   `json:"passed"`
	SweepsSucceeded bool   `json:"sweeps_succeeded"`
	Lifecycle       struct {
		Containers string `json:"containers"`
		Networks   string `json:"networks"`
		Volumes    string `json:"volumes"`
	} `json:"lifecycle"`
	Residuals struct {
		Containers []string `json:"containers"`
		Networks   []string `json:"networks"`
		Volumes    []string `json:"volumes"`
		Images     []string `json:"images"`
	} `json:"residuals"`
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

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func Supports(acceptanceID string) bool { return acceptanceID == AcceptanceID }

func Classify(acceptanceID, evidenceRoot string, command []string) (string, error) {
	if !Supports(acceptanceID) {
		return "", nil
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	if cleaned != "artifacts/acceptance" {
		return "", errors.New("A62 evidence root must be canonical artifacts/acceptance")
	}
	if err := ValidateCanonicalCommand(command, FormalClass); err != nil {
		return "", err
	}
	return FormalClass, nil
}

func ValidateCanonicalCommand(command []string, class string) error {
	if class != FormalClass || !equalStrings(command, canonicalCommand) {
		return fmt.Errorf("A62 formal evidence requires canonical command %q", strings.Join(canonicalCommand, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, class string) error {
	if class != FormalClass {
		return fmt.Errorf("unsupported A62 evidence class %q", class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-report.json"), &report); err != nil {
		return fmt.Errorf("validate A62 report: %w", err)
	}
	if err := validateFinalReport(report); err != nil {
		return err
	}
	var summary testSummary
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-test-summary.json"), &summary); err != nil {
		return fmt.Errorf("validate A62 test summary: %w", err)
	}
	if err := validateTestStream(filepath.Join(runDirectory, "a62-test.jsonl"), summary); err != nil {
		return err
	}
	if err := validateLogFile(filepath.Join(runDirectory, "a62-test.stderr.log")); err != nil {
		return err
	}
	var command commandReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-command.json"), &command); err != nil {
		return fmt.Errorf("validate A62 command report: %w", err)
	}
	if command.SchemaVersion != 1 || command.AcceptanceID != AcceptanceID || command.EvidenceClass != class ||
		command.TargetTest != "TestA62ResourceMinuteRetention" || command.WorkingDirectory != "/workspace" ||
		command.GoImage != "golang:1.25.1" || command.MySQLImage != "mysql:8.4" || !equalStrings(command.Command, innerCommand) {
		return errors.New("A62 command report contract is invalid")
	}
	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-environment.json"), &environment); err != nil {
		return fmt.Errorf("validate A62 environment report: %w", err)
	}
	if environment.SchemaVersion != 1 || environment.AcceptanceID != AcceptanceID || environment.EvidenceClass != class ||
		(environment.Commit != "unborn" && !regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(environment.Commit)) ||
		!environment.IsolatedGuard || !strings.HasPrefix(environment.MySQL.Version, "8.") ||
		environment.MySQL.TransactionIsolation != "READ-COMMITTED" || environment.MySQL.CharacterSetServer != "utf8mb4" ||
		environment.MySQL.CollationServer != "utf8mb4_unicode_ci" || environment.MySQL.TimeZone != "+08:00" ||
		!environment.Network.Internal || environment.Network.HostPorts == nil || len(environment.Network.HostPorts) != 0 ||
		environment.Database != "pilot_a62" {
		return errors.New("A62 environment report contract is invalid")
	}
	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-fixture.json"), &fixture); err != nil {
		return fmt.Errorf("validate A62 fixture report: %w", err)
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != AcceptanceID || fixture.FixtureID != "F05" ||
		fixture.Path != report.FixturePath || fixture.SHA256 != report.FixtureSHA256 || fixture.FixedNowUnix != report.FixedNowUnix ||
		fixture.RetentionDays != report.RetentionDays || !sha256Pattern.MatchString(fixture.SHA256) {
		return errors.New("A62 fixture report contract is invalid")
	}
	if err := validateCleanup(runDirectory, class); err != nil {
		return err
	}
	if err := validateInventory(runDirectory, class); err != nil {
		return err
	}
	return nil
}

func ValidateWrapperLogs(runDirectory string) error {
	for _, name := range []string{"stdout.log", "stderr.log"} {
		path := filepath.Join(runDirectory, name)
		payload, err := readBounded(path, maxLogSize)
		if err != nil {
			return fmt.Errorf("validate A62 wrapper log %s: %w", name, err)
		}
		if secretPattern.Match(payload) {
			return fmt.Errorf("A62 wrapper log %s contains a credential or DSN", name)
		}
	}
	return nil
}

func ValidateRunDirectory(runDirectory, class string) error {
	if err := ValidateInnerArtifacts(runDirectory, class); err != nil {
		return err
	}
	if err := ValidateWrapperLogs(runDirectory); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return fmt.Errorf("validate A62 wrapper evidence: %w", err)
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != AcceptanceID || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || ValidateCanonicalCommand(evidence.Command, class) != nil ||
		evidence.WorkingDirectory != "." || evidence.ExitCode != 0 || !evidence.RequiredNoSkip ||
		(evidence.Commit != "unborn" && !regexp.MustCompile(`^[0-9a-f]{40,64}$`).MatchString(evidence.Commit)) ||
		evidence.FixtureManifestPath != "testdata/design/manifest.sha256" || !sha256Pattern.MatchString(evidence.FixtureManifestSHA) ||
		evidence.StdoutLog != "stdout.log" || evidence.StderrLog != "stderr.log" || startErr != nil || finishErr != nil ||
		finished.Before(started) || evidence.DurationMilliseconds < 0 {
		return errors.New("A62 wrapper evidence contract is invalid")
	}
	return nil
}

func ValidateEvidenceRoot(root, class string) error {
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
	var failures []string
	for _, name := range names {
		if err := ValidateRunDirectory(filepath.Join(root, name), class); err == nil {
			return nil
		} else {
			failures = append(failures, name+": "+err.Error())
		}
	}
	if len(failures) == 0 {
		return errors.New("no A62 evidence run directories exist")
	}
	return fmt.Errorf("no valid A62 formal run: %s", strings.Join(failures, "; "))
}

func validateFinalReport(report finalReport) error {
	starvation := report.StarvationProof
	wantRestartDeleted := int64(starvation.BatchSize * starvation.MaximumBatches)
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.Status != "passed" ||
		report.FixturePath != "testdata/design/f05-ops-capacity.yaml" || !sha256Pattern.MatchString(report.FixtureSHA256) ||
		report.FixedNowUnix <= 0 || report.RetentionDays != 90 || report.Cutoff <= 0 || report.Cutoff%60 != 0 ||
		report.BatchSize != 257 || report.InitialRowsPerTable <= 5000 || report.FirstRun.Complete ||
		report.FirstRun.Instance.Deleted <= 5000 || report.FirstRun.Site.Deleted <= 5000 ||
		!report.FirstRun.Instance.PendingRows || !report.FirstRun.Site.PendingRows ||
		report.FirstRun.Instance.SkippedMissingHourly < 1 || report.FirstRun.Site.SkippedMissingHourly < 1 ||
		report.FirstRun.Instance.SkippedDailyNotFinal < 1 || report.FirstRun.Site.SkippedDailyNotFinal < 1 ||
		!report.SecondRun.Complete || !report.IdempotentRun.Complete || report.IdempotentRun.Instance.Deleted != 0 ||
		report.IdempotentRun.Site.Deleted != 0 || report.RowsAfterFirstRun != 4 || report.RowsAfterFinalRun != 2 ||
		!sha256Pattern.MatchString(report.ProtectedAggregateSHA256) || !report.BusinessFactsPreserved ||
		!report.ExactBoundaryPreserved || !report.MissingHourlyBlocked || !report.DailyNotFinalBlocked ||
		!report.InvalidRetentionRejected || !report.HourlyDailyValuesUnchanged ||
		starvation.BlockedPrefixRows <= starvation.BatchSize*starvation.MaximumBatches || starvation.BatchSize <= 0 ||
		starvation.MaximumBatches <= 0 || starvation.FirstRun.Complete || starvation.FirstRun.Instance.Deleted != 1 ||
		starvation.FirstRun.Site.Deleted != 1 || !starvation.FirstRun.Instance.PendingRows || !starvation.FirstRun.Site.PendingRows ||
		!starvation.FirstRun.Instance.BlockedDiagnosticsTruncated || !starvation.FirstRun.Site.BlockedDiagnosticsTruncated ||
		starvation.RestartRun.Complete || starvation.RestartRun.Instance.Deleted != wantRestartDeleted ||
		starvation.RestartRun.Site.Deleted != wantRestartDeleted || !starvation.RestartRun.Instance.PendingRows ||
		!starvation.RestartRun.Site.PendingRows || !starvation.FinalRun.Complete || starvation.FinalRun.Instance.Deleted != 1 ||
		starvation.FinalRun.Site.Deleted != 1 || starvation.FinalRun.Instance.PendingRows || starvation.FinalRun.Site.PendingRows ||
		!starvation.EligibleDeletedBehindBlockedPrefix || !starvation.RestartContinuationProved {
		return errors.New("A62 final report contract is invalid")
	}
	return nil
}

func validateTestStream(path string, summary testSummary) error {
	if summary.SchemaVersion != 1 || summary.AcceptanceID != AcceptanceID || summary.Status != "passed" ||
		summary.TargetTest != "TestA62ResourceMinuteRetention" || summary.Package != "new-api-pilot/tests/integration" ||
		summary.PassEvents != 1 || summary.FailEvents != 0 || summary.SkipEvents != 0 || summary.NoTests ||
		summary.JSONLines <= 0 || summary.JSONPath != "a62-test.jsonl" || summary.StderrPath != "a62-test.stderr.log" {
		return errors.New("A62 test summary contract is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := io.LimitReader(file, maxLogSize+1)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	lines, passes, failures, skips := 0, 0, 0, 0
	noTests := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("A62 test stream line %d is invalid JSON: %w", lines, err)
		}
		if event.Action == "skip" {
			skips++
		}
		if event.Action == "fail" {
			failures++
		}
		if event.Test == summary.TargetTest && event.Action == "pass" {
			passes++
		}
		if regexp.MustCompile(`(?i)no tests to run|\[no test files\]`).MatchString(event.Output) {
			noTests = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if lines != summary.JSONLines || passes != summary.PassEvents || failures != summary.FailEvents ||
		skips != summary.SkipEvents || noTests != summary.NoTests {
		return errors.New("A62 raw test stream does not match its summary")
	}
	return nil
}

func validateCleanup(runDirectory, class string) error {
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate A62 cleanup report: %w", err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != AcceptanceID || cleanup.EvidenceClass != class ||
		!cleanup.Passed || !cleanup.SweepsSucceeded || cleanup.Lifecycle.Containers != "created_and_removed" ||
		cleanup.Lifecycle.Networks != "created_and_removed" || cleanup.Lifecycle.Volumes != "not_created" ||
		cleanup.Residuals.Containers == nil || cleanup.Residuals.Networks == nil || cleanup.Residuals.Volumes == nil ||
		cleanup.Residuals.Images == nil || len(cleanup.Residuals.Containers) != 0 || len(cleanup.Residuals.Networks) != 0 ||
		len(cleanup.Residuals.Volumes) != 0 || len(cleanup.Residuals.Images) != 0 {
		return errors.New("A62 cleanup report contract is invalid")
	}
	return nil
}

func validateInventory(runDirectory, class string) error {
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, "a62-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate A62 artifact inventory: %w", err)
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != AcceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A62 artifact inventory contract is invalid")
	}
	wanted := make(map[string]struct{}, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for _, entry := range inventory.Files {
		if _, exists := wanted[entry.Path]; !exists || filepath.IsAbs(entry.Path) || filepath.Base(entry.Path) != entry.Path {
			return fmt.Errorf("A62 inventory contains unexpected path %q", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("A62 inventory repeats %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		path := filepath.Join(runDirectory, entry.Path)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes || entry.SizeBytes < 0 {
			return fmt.Errorf("A62 inventory size mismatch for %q", entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 || !sha256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("A62 inventory SHA-256 mismatch for %q", entry.Path)
		}
	}
	return nil
}

func validateLogFile(path string) error {
	payload, err := readBounded(path, maxLogSize)
	if err != nil {
		return err
	}
	if secretPattern.Match(payload) {
		return errors.New("A62 test stderr contains a credential or DSN")
	}
	return nil
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
		return errors.New("A62 evidence path is not a directory")
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
