package a49evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	FormalClass = "formal"
	SmokeClass  = "smoke"
	maxJSONSize = 8 * 1024 * 1024
)

var requiredArtifacts = []string{
	"a49-seed-report.json",
	"a49-load-results.jsonl",
	"a49-load-metadata.json",
	"a49-app.log",
	"a49-environment.json",
	"a49-docker-stats.tsv",
	"a49-mysql-status.tsv",
	"a49-query-plans.txt",
	"a49-report.json",
	"a49-negative-guard.log",
	"a49-migration.log",
	"a49-loader.log",
	"a49-load.log",
	"a49-report.log",
	"a49-image-build.log",
}

type finalReport struct {
	SchemaVersion      int    `json:"schema_version"`
	AcceptanceID       string `json:"acceptance_id"`
	Status             string `json:"status"`
	Passed             bool   `json:"passed"`
	Mode               string `json:"mode"`
	EvidenceClass      string `json:"evidence_class"`
	AcceptanceEligible bool   `json:"acceptance_eligible"`
}

type artifactInventory struct {
	SchemaVersion int             `json:"schema_version"`
	AcceptanceID  string          `json:"acceptance_id"`
	EvidenceClass string          `json:"evidence_class"`
	Mode          string          `json:"mode"`
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
	Mode            string `json:"mode"`
	Passed          bool   `json:"passed"`
	SweepsSucceeded bool   `json:"sweeps_succeeded"`
	Residuals       struct {
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

// ValidateCanonicalCommand binds A49 evidence to the single reviewed runner entry point.
func ValidateCanonicalCommand(command []string, class string) error {
	if _, _, err := classContract(class); err != nil {
		return err
	}
	want := []string{
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		"scripts/acceptance/run-a49.ps1",
	}
	if class == SmokeClass {
		want = append(want, "-Smoke")
	}
	if !equalStrings(command, want) {
		return fmt.Errorf("A49 %s evidence requires canonical command %q", class, strings.Join(want, " "))
	}
	return nil
}

// ValidateInnerArtifacts validates artifacts produced by run-a49.ps1 before the wrapper can pass.
func ValidateInnerArtifacts(runDirectory, class string) error {
	mode, eligible, err := classContract(class)
	if err != nil {
		return err
	}
	if err := requireDirectory(runDirectory); err != nil {
		return err
	}

	var report finalReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a49-report.json"), &report); err != nil {
		return fmt.Errorf("validate A49 final report: %w", err)
	}
	wantStatus := "passed"
	if class == SmokeClass {
		wantStatus = "smoke_passed_not_acceptance_evidence"
	}
	if report.SchemaVersion != 1 || report.AcceptanceID != "A49" || report.Status != wantStatus || !report.Passed ||
		report.Mode != mode || report.EvidenceClass != class || report.AcceptanceEligible != eligible {
		return errors.New("A49 final report contract is invalid")
	}

	if err := validateInventory(runDirectory, class, mode); err != nil {
		return err
	}
	if err := validateCleanup(runDirectory, class, mode); err != nil {
		return err
	}
	return nil
}

// ValidateRunDirectory validates both the script artifacts and the acceptance wrapper.
func ValidateRunDirectory(runDirectory, class string) error {
	if err := ValidateInnerArtifacts(runDirectory, class); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSONFile(filepath.Join(runDirectory, "evidence.json"), &evidence); err != nil {
		return fmt.Errorf("validate A49 wrapper evidence: %w", err)
	}
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != "A49" || evidence.Status != "passed" ||
		evidence.EvidenceClass != class || evidence.ExitCode != 0 || !evidence.RequiredNoSkip ||
		evidence.WorkingDirectory != "." || evidence.StdoutLog != "stdout.log" || evidence.StderrLog != "stderr.log" ||
		evidence.FixtureManifestPath != "testdata/design/manifest.sha256" || !validSHA256(evidence.FixtureManifestSHA) {
		return errors.New("A49 wrapper evidence contract is invalid")
	}
	if err := ValidateCanonicalCommand(evidence.Command, class); err != nil {
		return err
	}
	started, startErr := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	if startErr != nil || finishErr != nil || finished.Before(started) ||
		finished.Sub(started).Milliseconds() != evidence.DurationMilliseconds {
		return errors.New("A49 wrapper timing contract is invalid")
	}
	for _, logName := range []string{evidence.StdoutLog, evidence.StderrLog} {
		if err := requireRegularFile(filepath.Join(runDirectory, logName), false); err != nil {
			return fmt.Errorf("validate A49 wrapper log %s: %w", logName, err)
		}
	}
	return nil
}

// ValidateEvidenceRoot requires at least one complete run directory of the requested class.
func ValidateEvidenceRoot(evidenceRoot, class string) error {
	if _, _, err := classContract(class); err != nil {
		return err
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
		if err := ValidateRunDirectory(runDirectory, class); err == nil {
			return nil
		} else if len(reasons) < 3 {
			reasons = append(reasons, fmt.Sprintf("%s: %v", entry.Name(), err))
		}
	}
	if len(reasons) == 0 {
		return fmt.Errorf("no A49 %s run directories found", class)
	}
	return fmt.Errorf("no valid A49 %s run directory found (%s)", class, strings.Join(reasons, "; "))
}

func validateInventory(runDirectory, class, mode string) error {
	var inventory artifactInventory
	if err := decodeJSONFile(filepath.Join(runDirectory, "a49-artifacts.json"), &inventory); err != nil {
		return fmt.Errorf("validate A49 artifact inventory: %w", err)
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != "A49" || inventory.EvidenceClass != class ||
		inventory.Mode != mode || len(inventory.Files) != len(requiredArtifacts) {
		return errors.New("A49 artifact inventory contract is invalid")
	}
	wanted := make(map[string]struct{}, len(requiredArtifacts))
	for _, path := range requiredArtifacts {
		wanted[path] = struct{}{}
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for _, entry := range inventory.Files {
		if _, ok := wanted[entry.Path]; !ok {
			return fmt.Errorf("A49 artifact inventory contains unexpected path %q", entry.Path)
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("A49 artifact inventory repeats path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if entry.SizeBytes <= 0 || !validSHA256(entry.SHA256) {
			return fmt.Errorf("A49 artifact metadata is invalid for %q", entry.Path)
		}
		path := filepath.Join(runDirectory, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("A49 artifact %q is missing or not regular", entry.Path)
		}
		if info.Size() != entry.SizeBytes {
			return fmt.Errorf("A49 artifact size mismatch for %q", entry.Path)
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != entry.SHA256 {
			return fmt.Errorf("A49 artifact SHA-256 mismatch for %q", entry.Path)
		}
	}
	return nil
}

func validateCleanup(runDirectory, class, mode string) error {
	var cleanup cleanupReport
	if err := decodeJSONFile(filepath.Join(runDirectory, "a49-cleanup.json"), &cleanup); err != nil {
		return fmt.Errorf("validate A49 cleanup report: %w", err)
	}
	if cleanup.SchemaVersion != 1 || cleanup.AcceptanceID != "A49" || cleanup.EvidenceClass != class ||
		cleanup.Mode != mode || !cleanup.Passed || !cleanup.SweepsSucceeded ||
		cleanup.Residuals.Containers == nil || cleanup.Residuals.Networks == nil ||
		cleanup.Residuals.Volumes == nil || cleanup.Residuals.Images == nil ||
		len(cleanup.Residuals.Containers) != 0 || len(cleanup.Residuals.Networks) != 0 ||
		len(cleanup.Residuals.Volumes) != 0 || len(cleanup.Residuals.Images) != 0 {
		return errors.New("A49 cleanup report contract is invalid")
	}
	return nil
}

func classContract(class string) (string, bool, error) {
	switch class {
	case FormalClass:
		return "full", true, nil
	case SmokeClass:
		return "smoke", false, nil
	default:
		return "", false, fmt.Errorf("unknown A49 evidence class %q", class)
	}
}

func decodeJSONFile(path string, result any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("JSON evidence must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxJSONSize {
		return fmt.Errorf("JSON evidence size %d is outside the allowed range", info.Size())
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
		return errors.New("A49 run path must be a real directory")
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
