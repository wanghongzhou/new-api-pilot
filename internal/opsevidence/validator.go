package opsevidence

import (
	"crypto/sha256"
	"encoding/base64"
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
	FormalClass = "formal"
	SmokeClass  = "smoke"
	maxJSONSize = 8 * 1024 * 1024
	maxLogSize  = 8 * 1024 * 1024
)

var (
	a51SecretAssignmentPattern = regexp.MustCompile(`(?i)\b(?:OLD_ENCRYPTION_KEY|NEW_ENCRYPTION_KEY|ENCRYPTION_KEY|SESSION_SECRET)\s*=\s*([^\s"']+)`)
	a51DSNAssignmentPattern    = regexp.MustCompile(`(?i)\b(?:DATABASE_DSN|TEST_DATABASE_DSN)\s*=\s*([^\s"']+)`)
	a51RawDSNPattern           = regexp.MustCompile(`(?i)\b[^:\s]+:[^@\s]*@tcp\([^)\s]+\)/[^\s]+`)
	a51Base64KeyPattern        = regexp.MustCompile(`(?:^|[^A-Za-z0-9+/])([A-Za-z0-9+/]{43}=)(?:$|[^A-Za-z0-9+/=])`)
	a51FullKeyIDPattern        = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)
	a51URLCredentialPattern    = regexp.MustCompile(`(?i)https?://[^\s]*(?:access_token|token|secret|key|signature)=[^&\s]+`)
	a51URLUserInfoPattern      = regexp.MustCompile(`(?i)https?://[^/@\s]+@`)
)

var a51FixedPlaintextSecrets = []string{
	"a51-site-token-alpha-never-log",
	"a51-site-token-beta-never-log",
	"https://oapi.dingtalk.com/robot/send?access_token=a51-never-log",
	"a51-signing-secret-never-log",
}

var requiredArtifacts = map[string][]string{
	"A51": {
		"a51-preflight.log",
		"a51-seed-report.json",
		"a51-seed.log",
		"a51-dry-run.json",
		"a51-full.json",
		"a51-post-dry-run.json",
		"a51-verify.json",
		"a51-verify.log",
		"a51-integration-tests.jsonl",
		"a51-integration-tests.json",
		"a51-secret-scan.json",
		"a51-environment.json",
		"a51-mysql-contract.tsv",
		"a51-migration.log",
		"a51-report.json",
		"a51-report.log",
	},
}

type finalReport struct {
	SchemaVersion      int    `json:"schema_version"`
	AcceptanceID       string `json:"acceptance_id"`
	Status             string `json:"status"`
	Passed             bool   `json:"passed"`
	EvidenceClass      string `json:"evidence_class"`
	AcceptanceEligible bool   `json:"acceptance_eligible"`
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
	FixtureManifestPath  string   `json:"fixture_manifest_path"`
	FixtureManifestSHA   string   `json:"fixture_manifest_sha256"`
	StdoutLog            string   `json:"stdout_log"`
	StderrLog            string   `json:"stderr_log"`
	RequiredNoSkip       bool     `json:"required_no_skip"`
}

func Supports(acceptanceID string) bool {
	_, exists := requiredArtifacts[acceptanceID]
	return exists
}

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
		class = SmokeClass
	default:
		return "", fmt.Errorf("%s evidence root must be canonical artifacts/acceptance or artifacts/smoke", acceptanceID)
	}
	if err := ValidateCanonicalCommand(acceptanceID, command, class); err != nil {
		return "", err
	}
	return class, nil
}

func ValidateCanonicalCommand(acceptanceID string, command []string, class string) error {
	if acceptanceID != "A51" || class != FormalClass {
		return fmt.Errorf("%s does not support %s evidence", acceptanceID, class)
	}
	want := []string{
		"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/acceptance/run-a51.ps1",
	}
	if !equalStrings(command, want) {
		return fmt.Errorf("%s formal evidence requires canonical command %q", acceptanceID, strings.Join(want, " "))
	}
	return nil
}

func ValidateInnerArtifacts(runDirectory, acceptanceID, class string) error {
	if !Supports(acceptanceID) || class != FormalClass {
		return fmt.Errorf("unsupported operations evidence %s/%s", acceptanceID, class)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	prefix := strings.ToLower(acceptanceID)
	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, prefix+"-report.json"), &report); err != nil {
		return fmt.Errorf("validate %s final report: %w", acceptanceID, err)
	}
	if report.SchemaVersion != 1 || report.AcceptanceID != acceptanceID || report.Status != "passed" || !report.Passed ||
		report.EvidenceClass != class || !report.AcceptanceEligible {
		return fmt.Errorf("%s final report contract is invalid", acceptanceID)
	}
	if err := validateInventory(runDirectory, acceptanceID, class); err != nil {
		return err
	}
	if err := validateCleanup(runDirectory, acceptanceID, class); err != nil {
		return err
	}
	return nil
}

func ValidateWrapperLogs(runDirectory, acceptanceID string) error {
	if acceptanceID != "A51" {
		return fmt.Errorf("unsupported operations wrapper log validation %s", acceptanceID)
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}
	for _, name := range []string{"stdout.log", "stderr.log"} {
		path := filepath.Join(runDirectory, name)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("validate %s wrapper log %s: %w", acceptanceID, name, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > maxLogSize {
			return fmt.Errorf("validate %s wrapper log %s: log must be a bounded regular file", acceptanceID, name)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("validate %s wrapper log %s: %w", acceptanceID, name, err)
		}
		if violation := classifyA51WrapperLog(string(payload)); violation != "" {
			return fmt.Errorf("%s wrapper log %s contains forbidden %s", acceptanceID, name, violation)
		}
	}
	return nil
}

func classifyA51WrapperLog(payload string) string {
	for _, plaintext := range a51FixedPlaintextSecrets {
		if strings.Contains(payload, plaintext) {
			return "plaintext secret"
		}
	}
	for _, match := range a51SecretAssignmentPattern.FindAllStringSubmatch(payload, -1) {
		if len(match) == 2 && !strings.EqualFold(match[1], "[redacted]") {
			return "secret environment value"
		}
	}
	for _, match := range a51DSNAssignmentPattern.FindAllStringSubmatch(payload, -1) {
		if len(match) == 2 && !strings.EqualFold(match[1], "[redacted]") {
			return "database DSN"
		}
	}
	if a51RawDSNPattern.MatchString(payload) {
		return "database DSN"
	}
	if a51URLCredentialPattern.MatchString(payload) || a51URLUserInfoPattern.MatchString(payload) {
		return "URL credential"
	}
	for _, match := range a51Base64KeyPattern.FindAllStringSubmatch(payload, -1) {
		if len(match) != 2 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(match[1])
		if err == nil && len(decoded) == 32 {
			return "32-byte Base64 key"
		}
	}
	if a51FullKeyIDPattern.MatchString(payload) {
		return "full key fingerprint"
	}
	return ""
}

func ValidateRunDirectory(runDirectory, acceptanceID, class string) error {
	if err := ValidateWrapperLogs(runDirectory, acceptanceID); err != nil {
		return err
	}
	if err := ValidateInnerArtifacts(runDirectory, acceptanceID, class); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return fmt.Errorf("validate %s wrapper evidence: %w", acceptanceID, err)
	}
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != acceptanceID || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || evidence.ExitCode != 0 || !evidence.RequiredNoSkip || evidence.WorkingDirectory != "." ||
		evidence.StdoutLog != "stdout.log" || evidence.StderrLog != "stderr.log" ||
		evidence.FixtureManifestPath != "testdata/design/manifest.sha256" || !validSHA256(evidence.FixtureManifestSHA) {
		return fmt.Errorf("%s wrapper evidence contract is invalid", acceptanceID)
	}
	if err := ValidateCanonicalCommand(acceptanceID, evidence.Command, class); err != nil {
		return err
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if startErr != nil || finishErr != nil || finished.Before(started) ||
		finished.Sub(started).Milliseconds() != evidence.DurationMilliseconds {
		return fmt.Errorf("%s wrapper timing contract is invalid", acceptanceID)
	}
	for _, logName := range []string{evidence.StdoutLog, evidence.StderrLog} {
		if err := requireRegularFile(filepath.Join(runDirectory, logName), false); err != nil {
			return fmt.Errorf("validate %s wrapper log %s: %w", acceptanceID, logName, err)
		}
	}
	return nil
}

func ValidateEvidenceRoot(evidenceRoot, acceptanceID, class string) error {
	if !Supports(acceptanceID) {
		return fmt.Errorf("unsupported operations acceptance %s", acceptanceID)
	}
	entries, err := os.ReadDir(evidenceRoot)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	reasons := make([]string, 0, 3)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		runDirectory := filepath.Join(evidenceRoot, entry.Name())
		if err := ValidateRunDirectory(runDirectory, acceptanceID, class); err == nil {
			return nil
		} else if len(reasons) < 3 {
			reasons = append(reasons, fmt.Sprintf("%s: %v", entry.Name(), err))
		}
	}
	if len(reasons) == 0 {
		return fmt.Errorf("no %s %s run directories found", acceptanceID, class)
	}
	return fmt.Errorf("no valid %s %s run directory found (%s)", acceptanceID, class, strings.Join(reasons, "; "))
}

func validateInventory(runDirectory, acceptanceID, class string) error {
	prefix := strings.ToLower(acceptanceID)
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, prefix+"-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate %s artifact inventory: %w", acceptanceID, err)
	}
	required := requiredArtifacts[acceptanceID]
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != acceptanceID || inventory.EvidenceClass != class ||
		len(inventory.Files) != len(required) {
		return fmt.Errorf("%s artifact inventory contract is invalid", acceptanceID)
	}
	wanted := make(map[string]struct{}, len(required))
	for _, path := range required {
		wanted[path] = struct{}{}
	}
	seen := make(map[string]struct{}, len(required))
	for _, entry := range inventory.Files {
		if _, ok := wanted[entry.Path]; !ok {
			return fmt.Errorf("%s inventory contains unexpected path %q", acceptanceID, entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("%s inventory repeats path %q", acceptanceID, entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if entry.SizeBytes <= 0 || !validSHA256(entry.SHA256) {
			return fmt.Errorf("%s artifact metadata is invalid for %q", acceptanceID, entry.Path)
		}
		path := filepath.Join(runDirectory, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != entry.SizeBytes {
			return fmt.Errorf("%s artifact size/type mismatch for %q", acceptanceID, entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 {
			return fmt.Errorf("%s artifact SHA-256 mismatch for %q", acceptanceID, entry.Path)
		}
	}
	return nil
}

func validateCleanup(runDirectory, acceptanceID, class string) error {
	prefix := strings.ToLower(acceptanceID)
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, prefix+"-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate %s cleanup report: %w", acceptanceID, err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != acceptanceID || cleanup.EvidenceClass != class ||
		!cleanup.Passed || !cleanup.SweepsSucceeded || cleanup.Residuals.Containers == nil || cleanup.Residuals.Networks == nil ||
		cleanup.Residuals.Volumes == nil || cleanup.Residuals.Images == nil || len(cleanup.Residuals.Containers) != 0 ||
		len(cleanup.Residuals.Networks) != 0 || len(cleanup.Residuals.Volumes) != 0 || len(cleanup.Residuals.Images) != 0 ||
		cleanup.Lifecycle.Containers != "created_and_removed" || cleanup.Lifecycle.Networks != "created_and_removed" ||
		cleanup.Lifecycle.Volumes != "created_and_removed" {
		return fmt.Errorf("%s cleanup report contract is invalid", acceptanceID)
	}
	return nil
}

func decodeJSONFile(path string, result any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxJSONSize {
		return errors.New("JSON evidence must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(result); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON evidence contains multiple values")
		}
		return err
	}
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("run path must be a real directory")
	}
	return nil
}

func requireRegularFile(path string, nonempty bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || (nonempty && info.Size() <= 0) {
		return errors.New("path must be a regular file")
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

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
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
