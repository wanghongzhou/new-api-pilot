package a45evidence

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
	AcceptanceID     = "A45"
	FormalClass      = "formal"
	DevelopmentClass = "development"
	targetTest       = "TestA45SecurityBoundaryAcceptance"
	testPackage      = "new-api-pilot/tests/integration"
	maxJSONSize      = 4 << 20
	maxLogSize       = 32 << 20
)

var (
	canonicalCommand = []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a45.ps1",
	}
	innerCommand = []string{
		"go", "test", "-json", "./tests/integration", "-run", "^TestA45SecurityBoundaryAcceptance$", "-count=1", "-timeout=4m",
	}
	requiredSubtests = []string{
		targetTest + "/origin_fail_closed",
		targetTest + "/trusted_proxy_boundary",
		targetTest + "/upstream_DNS_TLS_and_credential_boundary",
		targetTest + "/sensitive_response_and_logs",
	}
	// The inventory is a tenth inner file and hashes these nine fixed payloads.
	requiredArtifacts = []string{
		"a45-test.jsonl",
		"a45-test.stderr.log",
		"a45-test-summary.json",
		"a45-command.json",
		"a45-environment.json",
		"a45-fixture.json",
		"a45-report.json",
		"a45-cleanup.json",
		"a45-secret-scan.json",
	}
	secretScanTargets = []string{
		"a45-test.jsonl",
		"a45-test.stderr.log",
		"a45-test-summary.json",
		"a45-command.json",
		"a45-environment.json",
		"a45-fixture.json",
		"a45-report.json",
		"a45-cleanup.json",
	}
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern   = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	resourcePattern = regexp.MustCompile(
		`^new-api-pilot-a45-[0-9a-f]+-[0-9a-f]{12}-(?:network|gomod|gobuild)$`,
	)
	noTestsPattern = regexp.MustCompile(`(?i)no tests to run|\[no test files\]`)
	secretPattern  = regexp.MustCompile(
		`(?i)(?:a45-sensitive-value-never-log|a45-old-token-never-send|` +
			`(?:authorization|access[_-]?token|webhook(?:[_-]?url)?|password|session[_-]?secret|encryption[_-]?key)` +
			`\s*["']?\s*[:=]\s*["']?[^\s"']{4,}|` +
			`[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)`,
	)
)

const (
	fixtureF01Path   = "testdata/design/f01-auth.json"
	fixtureF01SHA256 = "d232dc1a6b83ba80f49995dadbd8afe11d7b73120f7474a2abcece7e1b46e1da"
	fixtureF02Path   = "testdata/design/f02-upstream/manifest.json"
	fixtureF02SHA256 = "f1a12b434ab24c01bf53d12bc65ccd86c90cd3e8f620c94f865e67f14b210f2f"
	fixtureManifest  = "testdata/design/manifest.sha256"
)

type testSummary struct {
	SchemaVersion     int      `json:"schema_version"`
	AcceptanceID      string   `json:"acceptance_id"`
	Status            string   `json:"status"`
	TargetTest        string   `json:"target_test"`
	Package           string   `json:"package"`
	PassEvents        int      `json:"pass_events"`
	SubtestPassEvents int      `json:"subtest_pass_events"`
	PackagePassEvents int      `json:"package_pass_events"`
	FailEvents        int      `json:"fail_events"`
	SkipEvents        int      `json:"skip_events"`
	NoTests           bool     `json:"no_tests"`
	JSONLines         int      `json:"json_lines"`
	RequiredSubtests  []string `json:"required_subtests"`
	JSONPath          string   `json:"json_path"`
	StderrPath        string   `json:"stderr_path"`
}

type commandReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	EvidenceClass string   `json:"evidence_class"`
	TargetTest    string   `json:"target_test"`
	WorkingDir    string   `json:"working_directory"`
	Command       []string `json:"command"`
	GoImage       string   `json:"go_image"`
}

type imageIdentity struct {
	Reference string `json:"reference"`
	ID        string `json:"id"`
	Digest    string `json:"digest"`
}

type environmentReport struct {
	SchemaVersion int           `json:"schema_version"`
	AcceptanceID  string        `json:"acceptance_id"`
	EvidenceClass string        `json:"evidence_class"`
	Commit        string        `json:"commit"`
	WorktreeDirty bool          `json:"worktree_dirty"`
	Image         imageIdentity `json:"go_image"`
	Resources     struct {
		Network     string `json:"network"`
		ModuleCache string `json:"module_cache"`
		BuildCache  string `json:"build_cache"`
	} `json:"resources"`
	Network struct {
		Internal         bool     `json:"internal"`
		HostPorts        []string `json:"host_ports"`
		AttachedNetworks []string `json:"attached_networks"`
	} `json:"network"`
	RepositoryReadOnly        bool `json:"repository_read_only"`
	ContainerRootFSReadOnly   bool `json:"container_rootfs_read_only"`
	NoNewPrivileges           bool `json:"no_new_privileges"`
	AllCapabilitiesDropped    bool `json:"all_capabilities_dropped"`
	EvidenceMountedInTest     bool `json:"evidence_mounted_in_test"`
	OfflineTestNetwork        bool `json:"offline_test_network"`
	GoProxyOff                bool `json:"go_proxy_off"`
	GoSumDBOff                bool `json:"go_sumdb_off"`
	EnvironmentProxiesCleared bool `json:"environment_proxies_cleared"`
}

type fixtureEntry struct {
	FixtureID string `json:"fixture_id"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
}

type fixtureReport struct {
	SchemaVersion int            `json:"schema_version"`
	AcceptanceID  string         `json:"acceptance_id"`
	Fixtures      []fixtureEntry `json:"fixtures"`
	ManifestPath  string         `json:"manifest_path"`
	ManifestSHA   string         `json:"manifest_sha256"`
}

type finalReport struct {
	SchemaVersion    int      `json:"schema_version"`
	AcceptanceID     string   `json:"acceptance_id"`
	Status           string   `json:"status"`
	TargetTest       string   `json:"target_test"`
	RequiredSubtests []string `json:"required_subtests"`
	Scenarios        struct {
		OriginFailClosed                 bool `json:"origin_fail_closed"`
		TrustedProxyBoundary             bool `json:"trusted_proxy_boundary"`
		UpstreamDNSTLSCredentialBoundary bool `json:"upstream_dns_tls_credential_boundary"`
		SensitiveResponseAndLogs         bool `json:"sensitive_response_and_logs"`
	} `json:"scenarios"`
	SecurityChecks struct {
		ForgedForwardedForRejected       bool `json:"forged_forwarded_for_rejected"`
		InvalidOriginRejected            bool `json:"invalid_origin_rejected"`
		DNSRebindingRejected             bool `json:"dns_rebinding_rejected"`
		NonAllowlistedAddressRejected    bool `json:"non_allowlisted_address_rejected"`
		TLSDowngradeRedirectRejected     bool `json:"tls_downgrade_redirect_rejected"`
		OldTokenOriginGuarded            bool `json:"old_token_origin_guarded"`
		EnvironmentProxyUnused           bool `json:"environment_proxy_unused"`
		SensitiveResponseAndLogsRedacted bool `json:"sensitive_response_and_logs_redacted"`
	} `json:"security_checks"`
	IsolationChecks struct {
		UniqueInternalNetwork     bool `json:"unique_internal_network"`
		NoHostPorts               bool `json:"no_host_ports"`
		RepositoryReadOnly        bool `json:"repository_read_only"`
		ContainerRootFSReadOnly   bool `json:"container_rootfs_read_only"`
		EnvironmentProxiesCleared bool `json:"environment_proxies_cleared"`
		GoProxyOff                bool `json:"go_proxy_off"`
	} `json:"isolation_checks"`
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
	class := ""
	switch cleaned {
	case "artifacts/acceptance":
		class = FormalClass
	case "artifacts/smoke":
		class = DevelopmentClass
	default:
		return "", errors.New("A45 evidence root must be canonical artifacts/acceptance or artifacts/smoke")
	}
	if err := ValidateCanonicalCommand(command, class); err != nil {
		return "", err
	}
	return class, nil
}

func ValidateCanonicalCommand(command []string, class string) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A45 evidence class %q", class)
	}
	if !equalStrings(command, canonicalCommand) {
		return fmt.Errorf("A45 evidence requires canonical command %q", strings.Join(canonicalCommand, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, class string) error {
	return validateInnerArtifacts(runDirectory, class, false)
}

func validateInnerArtifacts(runDirectory, class string, wrapperEvidencePresent bool) error {
	if class != FormalClass && class != DevelopmentClass {
		return fmt.Errorf("unsupported A45 evidence class %q", class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	if err := validateExactFiles(runDirectory, wrapperEvidencePresent); err != nil {
		return err
	}

	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-report.json"), &report); err != nil {
		return fmt.Errorf("validate A45 report: %w", err)
	}
	if err := validateFinalReport(report); err != nil {
		return err
	}

	var summary testSummary
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-test-summary.json"), &summary); err != nil {
		return fmt.Errorf("validate A45 test summary: %w", err)
	}
	if err := validateTestStream(filepath.Join(runDirectory, "a45-test.jsonl"), summary); err != nil {
		return err
	}

	var command commandReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-command.json"), &command); err != nil {
		return fmt.Errorf("validate A45 command report: %w", err)
	}
	if command.SchemaVersion != 1 || command.AcceptanceID != AcceptanceID || command.EvidenceClass != class ||
		command.TargetTest != targetTest || command.WorkingDir != "/workspace" || command.GoImage != "golang:1.25.1" ||
		!equalStrings(command.Command, innerCommand) {
		return errors.New("A45 command report contract is invalid")
	}

	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-environment.json"), &environment); err != nil {
		return fmt.Errorf("validate A45 environment report: %w", err)
	}
	if err := validateEnvironment(environment, class); err != nil {
		return err
	}

	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-fixture.json"), &fixture); err != nil {
		return fmt.Errorf("validate A45 fixture report: %w", err)
	}
	if err := validateFixture(fixture); err != nil {
		return err
	}

	if err := validateCleanup(runDirectory, class); err != nil {
		return err
	}
	if err := validateSecretScan(runDirectory, class); err != nil {
		return err
	}
	if err := validateInventory(runDirectory, class); err != nil {
		return err
	}
	for _, name := range append(append([]string{}, requiredArtifacts...), "a45-artifacts.json") {
		if err := validateNoSecret(filepath.Join(runDirectory, name)); err != nil {
			return err
		}
	}
	return nil
}

func ValidateWrapperLogs(runDirectory string) error {
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := validateNoSecret(filepath.Join(runDirectory, name)); err != nil {
			return fmt.Errorf("validate A45 wrapper log %s: %w", name, err)
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
		return fmt.Errorf("validate A45 wrapper evidence: %w", err)
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
		return errors.New("A45 wrapper evidence contract is invalid")
	}
	var environment environmentReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-environment.json"), &environment); err != nil {
		return err
	}
	var fixture fixtureReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-fixture.json"), &fixture); err != nil {
		return err
	}
	if environment.Commit != evidence.Commit || environment.WorktreeDirty != evidence.WorktreeDirty ||
		fixture.ManifestPath != evidence.FixtureManifestPath || fixture.ManifestSHA != evidence.FixtureManifestSHA {
		return errors.New("A45 wrapper metadata is not bound to environment and fixture evidence")
	}
	return validateNoSecret(filepath.Join(runDirectory, "evidence.json"))
}

func ValidateEvidenceRoot(root, class string) error {
	if class != FormalClass {
		return errors.New("A45 manifest evidence must be formal")
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
		return errors.New("no A45 evidence run directories exist")
	}
	return fmt.Errorf("no valid A45 formal run: %s", strings.Join(failures, "; "))
}

func validateFinalReport(report finalReport) error {
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.Status != "passed" ||
		report.TargetTest != targetTest || !equalStrings(report.RequiredSubtests, requiredSubtests) ||
		!report.Scenarios.OriginFailClosed || !report.Scenarios.TrustedProxyBoundary ||
		!report.Scenarios.UpstreamDNSTLSCredentialBoundary || !report.Scenarios.SensitiveResponseAndLogs ||
		!report.SecurityChecks.ForgedForwardedForRejected || !report.SecurityChecks.InvalidOriginRejected ||
		!report.SecurityChecks.DNSRebindingRejected || !report.SecurityChecks.NonAllowlistedAddressRejected ||
		!report.SecurityChecks.TLSDowngradeRedirectRejected || !report.SecurityChecks.OldTokenOriginGuarded ||
		!report.SecurityChecks.EnvironmentProxyUnused || !report.SecurityChecks.SensitiveResponseAndLogsRedacted ||
		!report.IsolationChecks.UniqueInternalNetwork || !report.IsolationChecks.NoHostPorts ||
		!report.IsolationChecks.RepositoryReadOnly || !report.IsolationChecks.ContainerRootFSReadOnly ||
		!report.IsolationChecks.EnvironmentProxiesCleared || !report.IsolationChecks.GoProxyOff {
		return errors.New("A45 final report contract is invalid")
	}
	return nil
}

func validateEnvironment(environment environmentReport, class string) error {
	if environment.SchemaVersion != 1 || environment.AcceptanceID != AcceptanceID || environment.EvidenceClass != class ||
		(environment.Commit != "unborn" && !commitPattern.MatchString(environment.Commit)) ||
		environment.Image.Reference != "golang:1.25.1" || !strings.HasPrefix(environment.Image.ID, "sha256:") ||
		!sha256Pattern.MatchString(strings.TrimPrefix(environment.Image.ID, "sha256:")) ||
		!strings.Contains(environment.Image.Digest, "@sha256:") ||
		!sha256Pattern.MatchString(environment.Image.Digest[strings.LastIndex(environment.Image.Digest, "@sha256:")+8:]) ||
		!environment.Network.Internal || environment.Network.HostPorts == nil || len(environment.Network.HostPorts) != 0 ||
		len(environment.Network.AttachedNetworks) != 1 ||
		!environment.RepositoryReadOnly || !environment.ContainerRootFSReadOnly || !environment.NoNewPrivileges ||
		!environment.AllCapabilitiesDropped || environment.EvidenceMountedInTest || !environment.OfflineTestNetwork ||
		!environment.GoProxyOff || !environment.GoSumDBOff || !environment.EnvironmentProxiesCleared {
		return errors.New("A45 environment isolation contract is invalid")
	}
	resources := []string{environment.Resources.Network, environment.Resources.ModuleCache, environment.Resources.BuildCache}
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if !resourcePattern.MatchString(resource) {
			return fmt.Errorf("A45 resource name %q is invalid", resource)
		}
		if _, duplicate := seen[resource]; duplicate {
			return errors.New("A45 resource names are not unique")
		}
		seen[resource] = struct{}{}
	}
	if environment.Network.AttachedNetworks[0] != environment.Resources.Network {
		return errors.New("A45 test container is not attached only to the recorded internal network")
	}
	return nil
}

func validateFixture(fixture fixtureReport) error {
	want := []fixtureEntry{
		{FixtureID: "F01", Path: fixtureF01Path, SHA256: fixtureF01SHA256},
		{FixtureID: "F02", Path: fixtureF02Path, SHA256: fixtureF02SHA256},
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != AcceptanceID || fixture.ManifestPath != fixtureManifest ||
		!sha256Pattern.MatchString(fixture.ManifestSHA) || len(fixture.Fixtures) != len(want) {
		return errors.New("A45 fixture report contract is invalid")
	}
	for index := range want {
		if fixture.Fixtures[index] != want[index] {
			return fmt.Errorf("A45 fixture report differs at index %d", index)
		}
	}
	return nil
}

func validateTestStream(path string, summary testSummary) error {
	if summary.SchemaVersion != 1 || summary.AcceptanceID != AcceptanceID || summary.Status != "passed" ||
		summary.TargetTest != targetTest || summary.Package != testPackage || summary.PassEvents != 1 ||
		summary.SubtestPassEvents != len(requiredSubtests) || summary.PackagePassEvents != 1 ||
		summary.FailEvents != 0 || summary.SkipEvents != 0 || summary.NoTests || summary.JSONLines <= 0 ||
		!equalStrings(summary.RequiredSubtests, requiredSubtests) || summary.JSONPath != "a45-test.jsonl" ||
		summary.StderrPath != "a45-test.stderr.log" {
		return errors.New("A45 test summary contract is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maxLogSize+1))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	lines, targetPasses, packagePasses, failures, skips := 0, 0, 0, 0, 0
	noTests := false
	subtestPasses := make(map[string]int, len(requiredSubtests))
	wantedSubtests := make(map[string]struct{}, len(requiredSubtests))
	for _, subtest := range requiredSubtests {
		wantedSubtests[subtest] = struct{}{}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines++
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("A45 test stream line %d is invalid JSON: %w", lines, err)
		}
		if event.Package != testPackage {
			return fmt.Errorf("A45 test stream line %d has unexpected package %q", lines, event.Package)
		}
		switch event.Action {
		case "skip":
			skips++
		case "fail":
			failures++
		case "pass":
			switch {
			case event.Test == targetTest:
				targetPasses++
			case event.Test == "":
				packagePasses++
			case strings.HasPrefix(event.Test, targetTest+"/"):
				if _, wanted := wantedSubtests[event.Test]; !wanted {
					return fmt.Errorf("A45 test stream contains unexpected subtest %q", event.Test)
				}
				subtestPasses[event.Test]++
			}
		}
		if noTestsPattern.MatchString(event.Output) {
			noTests = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, subtest := range requiredSubtests {
		if subtestPasses[subtest] != 1 {
			return fmt.Errorf("A45 subtest %q emitted %d pass events", subtest, subtestPasses[subtest])
		}
	}
	if lines != summary.JSONLines || targetPasses != summary.PassEvents || packagePasses != summary.PackagePassEvents ||
		len(requiredSubtests) != summary.SubtestPassEvents || failures != summary.FailEvents || skips != summary.SkipEvents ||
		noTests != summary.NoTests {
		return errors.New("A45 raw test stream does not match its summary")
	}
	return nil
}

func validateCleanup(runDirectory, class string) error {
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate A45 cleanup report: %w", err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != AcceptanceID || cleanup.EvidenceClass != class ||
		!cleanup.Passed || !cleanup.SweepsSucceeded || cleanup.Lifecycle.Containers != "created_and_removed" ||
		cleanup.Lifecycle.Networks != "created_and_removed" || cleanup.Lifecycle.Volumes != "created_and_removed" ||
		cleanup.Residuals.Containers == nil || cleanup.Residuals.Networks == nil || cleanup.Residuals.Volumes == nil ||
		cleanup.Residuals.Images == nil || len(cleanup.Residuals.Containers) != 0 || len(cleanup.Residuals.Networks) != 0 ||
		len(cleanup.Residuals.Volumes) != 0 || len(cleanup.Residuals.Images) != 0 {
		return errors.New("A45 cleanup report contract is invalid")
	}
	return nil
}

func validateSecretScan(runDirectory, class string) error {
	var report secretScanReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-secret-scan.json"), &report); err != nil {
		return fmt.Errorf("validate A45 secret scan report: %w", err)
	}
	if report.SchemaVersion != 1 || report.AcceptanceID != AcceptanceID || report.EvidenceClass != class ||
		report.Status != "passed" || report.Matches != 0 || !equalStrings(report.Files, secretScanTargets) {
		return errors.New("A45 secret scan report contract is invalid")
	}
	return nil
}

func validateInventory(runDirectory, class string) error {
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, "a45-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate A45 artifact inventory: %w", err)
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != AcceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A45 artifact inventory contract is invalid")
	}
	wanted := make(map[string]struct{}, len(requiredArtifacts))
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for _, entry := range inventory.Files {
		if _, exists := wanted[entry.Path]; !exists || filepath.IsAbs(entry.Path) || filepath.Base(entry.Path) != entry.Path {
			return fmt.Errorf("A45 inventory contains unexpected path %q", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("A45 inventory repeats %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		path := filepath.Join(runDirectory, entry.Path)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes || entry.SizeBytes < 0 {
			return fmt.Errorf("A45 inventory size mismatch for %q", entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 || !sha256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("A45 inventory SHA-256 mismatch for %q", entry.Path)
		}
	}
	if len(seen) != len(wanted) {
		return errors.New("A45 artifact inventory is incomplete")
	}
	return nil
}

func validateExactFiles(runDirectory string, wrapperEvidencePresent bool) error {
	wanted := make(map[string]struct{}, len(requiredArtifacts)+4)
	for _, name := range requiredArtifacts {
		wanted[name] = struct{}{}
	}
	for _, name := range []string{"a45-artifacts.json", "stdout.log", "stderr.log"} {
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
			return fmt.Errorf("A45 evidence directory contains non-regular entry %q", entry.Name())
		}
		if _, allowed := wanted[entry.Name()]; !allowed {
			return fmt.Errorf("A45 evidence directory contains unexpected file %q", entry.Name())
		}
		seen[entry.Name()] = struct{}{}
	}
	for name := range wanted {
		if _, exists := seen[name]; !exists {
			return fmt.Errorf("A45 evidence directory is missing %q", name)
		}
	}
	return nil
}

func validateNoSecret(path string) error {
	payload, err := readBounded(path, maxLogSize)
	if err != nil {
		return err
	}
	if secretPattern.Match(payload) {
		return fmt.Errorf("A45 evidence file %s contains a forbidden secret pattern", filepath.Base(path))
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
		return errors.New("A45 evidence path is not a directory")
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
