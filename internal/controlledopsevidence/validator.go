package controlledopsevidence

import (
	"archive/zip"
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

const FormalClass = "formal"

var (
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	secretPattern = regexp.MustCompile(`(?i)(?:DATABASE_DSN\s*=|ENCRYPTION_KEY\s*=|SESSION_SECRET\s*=|\b[^:\s]+:[^@\s]*@tcp\([^)\s]+\)/[^\s]+|https?://[^/@\s]+@)`)
	caseContracts = map[string]caseContract{
		"A52": {
			runner: "scripts/acceptance/run-a52.ps1", scope: "controlled_production_read_only",
			fixtures:   []string{"F02", "F05"},
			zipFiles:   []string{"approvals.json", "readonly-verification.json", "site-inventory.json"},
			assertions: []string{"export_enabled", "first_root_confirmed", "independent_review", "owner_confirmed", "quota_retention_confirmed", "readonly_contracts_verified", "status_identity_verified", "uniform_version"},
		},
		"A74": {
			runner: "scripts/acceptance/run-a74.ps1", scope: "controlled_pilot_owned_isolated",
			fixtures:   []string{"F05"},
			zipFiles:   []string{"approvals.json", "cleanup.json", "deployment.json", "monitoring.json", "rollback.json"},
			assertions: []string{"approvals_complete", "backup_restored", "cleanup_verified", "failure_injected", "health_ready_verified", "immutable_images", "migration_verified", "monitoring_observed", "old_image_verified", "pilot_owned_scope", "smoke_verified"},
		},
		"A75": {
			runner: "scripts/acceptance/run-a75.ps1", scope: "controlled_pilot_owned_isolated",
			fixtures:   []string{"F05"},
			zipFiles:   []string{"approvals.json", "backup.json", "cleanup.json", "restore.json", "rpo-rto.json", "verify-restore.json"},
			assertions: []string{"approvals_complete", "binlog_verified", "cleanup_verified", "full_backup_verified", "keys_verified", "no_production_switch", "pilot_owned_scope", "restore_verified", "rpo_met", "rto_met", "target_identity_verified", "verify_restore_full"},
		},
	}
)

type caseContract struct {
	runner, scope string
	fixtures      []string
	zipFiles      []string
	assertions    []string
}

type finalReport struct {
	SchemaVersion      int             `json:"schema_version"`
	AcceptanceID       string          `json:"acceptance_id"`
	Status             string          `json:"status"`
	Passed             bool            `json:"passed"`
	EvidenceClass      string          `json:"evidence_class"`
	AcceptanceEligible bool            `json:"acceptance_eligible"`
	Scope              string          `json:"scope"`
	TargetIdentity     string          `json:"target_identity"`
	ImmutableReference string          `json:"immutable_reference"`
	StartedAt          string          `json:"started_at"`
	FinishedAt         string          `json:"finished_at"`
	MaterialSHA256     string          `json:"material_sha256"`
	MaterialSizeBytes  int64           `json:"material_size_bytes"`
	Assertions         map[string]bool `json:"assertions"`
}

type commandReport struct {
	SchemaVersion int      `json:"schema_version"`
	AcceptanceID  string   `json:"acceptance_id"`
	EvidenceClass string   `json:"evidence_class"`
	Command       []string `json:"command"`
}

type fixtureReport struct {
	SchemaVersion  int      `json:"schema_version"`
	AcceptanceID   string   `json:"acceptance_id"`
	ManifestPath   string   `json:"manifest_path"`
	ManifestSHA256 string   `json:"manifest_sha256"`
	FixtureIDs     []string `json:"fixture_ids"`
}

type materialDocument struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	ArtifactType  string `json:"artifact_type"`
	Passed        bool   `json:"passed"`
	Sanitized     bool   `json:"sanitized"`
	Operator      string `json:"operator,omitempty"`
	Reviewer      string `json:"reviewer,omitempty"`
	Approver      string `json:"approver,omitempty"`
	Approved      bool   `json:"approved,omitempty"`
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

func Supports(id string) bool { _, ok := caseContracts[id]; return ok }

func CanonicalCommand(id string) []string {
	contract, ok := caseContracts[id]
	if !ok {
		return nil
	}
	return []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", contract.runner}
}

func Classify(id, evidenceRoot string, command []string) (string, error) {
	if !Supports(id) {
		return "", nil
	}
	if strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./") != "artifacts/acceptance" {
		return "", fmt.Errorf("%s evidence root must be canonical artifacts/acceptance", id)
	}
	if err := ValidateCanonicalCommand(id, command, FormalClass); err != nil {
		return "", err
	}
	return FormalClass, nil
}

func ValidateCanonicalCommand(id string, command []string, class string) error {
	want := CanonicalCommand(id)
	if class != FormalClass || want == nil || !equalStrings(command, want) {
		return fmt.Errorf("%s formal evidence requires canonical command %q", id, strings.Join(want, " "))
	}
	return nil
}

func ValidateInnerArtifacts(dir, id, class string) error {
	contract, ok := caseContracts[id]
	if !ok || class != FormalClass {
		return fmt.Errorf("unsupported controlled operations evidence %s/%s", id, class)
	}
	if err := requireDirectory(dir); err != nil {
		return err
	}
	prefix := strings.ToLower(id)
	var report finalReport
	if err := decodeJSON(filepath.Join(dir, prefix+"-report.json"), &report); err != nil {
		return fmt.Errorf("validate %s report: %w", id, err)
	}
	if err := validateReport(report, id, contract); err != nil {
		return err
	}
	var command commandReport
	if err := decodeJSON(filepath.Join(dir, prefix+"-command.json"), &command); err != nil {
		return err
	}
	if command.SchemaVersion != 1 || command.AcceptanceID != id || command.EvidenceClass != class || !equalStrings(command.Command, CanonicalCommand(id)) {
		return fmt.Errorf("%s command report contract is invalid", id)
	}
	var fixture fixtureReport
	if err := decodeJSON(filepath.Join(dir, prefix+"-fixture.json"), &fixture); err != nil {
		return err
	}
	if fixture.SchemaVersion != 1 || fixture.AcceptanceID != id || fixture.ManifestPath != "testdata/design/manifest.sha256" || !sha256Pattern.MatchString(fixture.ManifestSHA256) || !equalStrings(fixture.FixtureIDs, contract.fixtures) {
		return fmt.Errorf("%s fixture report contract is invalid", id)
	}
	materialPath := filepath.Join(dir, prefix+"-controlled-material.zip")
	info, err := os.Stat(materialPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != report.MaterialSizeBytes {
		return fmt.Errorf("%s controlled material size is invalid", id)
	}
	digest, err := hashFile(materialPath)
	if err != nil || digest != report.MaterialSHA256 {
		return fmt.Errorf("%s controlled material checksum is invalid", id)
	}
	if err := validateZip(materialPath, id, contract); err != nil {
		return err
	}
	return validateInventory(dir, id, contract)
}

func validateReport(report finalReport, id string, contract caseContract) error {
	if report.SchemaVersion != 1 || report.AcceptanceID != id || report.Status != "passed" || !report.Passed || report.EvidenceClass != FormalClass || !report.AcceptanceEligible || report.Scope != contract.scope || strings.TrimSpace(report.TargetIdentity) == "" || !sha256Pattern.MatchString(report.ImmutableReference) || !sha256Pattern.MatchString(report.MaterialSHA256) || report.MaterialSizeBytes <= 0 {
		return fmt.Errorf("%s final report contract is invalid", id)
	}
	start, e1 := time.Parse(time.RFC3339Nano, report.StartedAt)
	finish, e2 := time.Parse(time.RFC3339Nano, report.FinishedAt)
	if e1 != nil || e2 != nil || finish.Before(start) {
		return fmt.Errorf("%s report timeline is invalid", id)
	}
	if len(report.Assertions) != len(contract.assertions) {
		return fmt.Errorf("%s assertion set is invalid", id)
	}
	for _, key := range contract.assertions {
		if !report.Assertions[key] {
			return fmt.Errorf("%s assertion %s did not pass", id, key)
		}
	}
	return nil
}

func validateZip(path, id string, contract caseContract) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open %s controlled material: %w", id, err)
	}
	defer archive.Close()
	names := make([]string, 0, len(archive.File))
	docs := map[string]materialDocument{}
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != file.Name || file.UncompressedSize64 > 8*1024*1024 {
			return fmt.Errorf("%s controlled material contains unsafe entry", id)
		}
		names = append(names, file.Name)
		reader, err := file.Open()
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(io.LimitReader(reader, 8*1024*1024+1))
		decoder.DisallowUnknownFields()
		var doc materialDocument
		err = decoder.Decode(&doc)
		_ = reader.Close()
		if err != nil {
			return fmt.Errorf("decode %s/%s: %w", id, file.Name, err)
		}
		docs[file.Name] = doc
	}
	sort.Strings(names)
	if !equalStrings(names, contract.zipFiles) {
		return fmt.Errorf("%s controlled material file set is invalid", id)
	}
	for name, doc := range docs {
		if doc.SchemaVersion != 1 || doc.AcceptanceID != id || doc.ArtifactType != strings.TrimSuffix(name, ".json") || !doc.Passed || !doc.Sanitized {
			return fmt.Errorf("%s controlled material %s contract is invalid", id, name)
		}
	}
	approval := docs["approvals.json"]
	if !approval.Approved || strings.TrimSpace(approval.Operator) == "" || strings.TrimSpace(approval.Reviewer) == "" || strings.TrimSpace(approval.Approver) == "" || approval.Operator == approval.Reviewer || approval.Operator == approval.Approver || approval.Reviewer == approval.Approver {
		return fmt.Errorf("%s approvals are not independent and complete", id)
	}
	return nil
}

func validateInventory(dir, id string, contract caseContract) error {
	prefix := strings.ToLower(id)
	expected := []string{prefix + "-command.json", prefix + "-controlled-material.zip", prefix + "-fixture.json", prefix + "-report.json"}
	var inventory artifactInventory
	if err := decodeJSON(filepath.Join(dir, prefix+"-artifacts.json"), &inventory); err != nil {
		return err
	}
	if inventory.SchemaVersion != 1 || inventory.AcceptanceID != id || inventory.EvidenceClass != FormalClass || len(inventory.Files) != len(expected) {
		return fmt.Errorf("%s artifact inventory contract is invalid", id)
	}
	seen := map[string]bool{}
	for _, entry := range inventory.Files {
		if seen[entry.Path] || !contains(expected, entry.Path) || entry.SizeBytes <= 0 || !sha256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("%s artifact inventory entry is invalid", id)
		}
		info, err := os.Stat(filepath.Join(dir, entry.Path))
		if err != nil || info.Size() != entry.SizeBytes {
			return fmt.Errorf("%s artifact size mismatch", id)
		}
		digest, err := hashFile(filepath.Join(dir, entry.Path))
		if err != nil || digest != entry.SHA256 {
			return fmt.Errorf("%s artifact checksum mismatch", id)
		}
		seen[entry.Path] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"evidence.json": true, "stdout.log": true, "stderr.log": true, prefix + "-artifacts.json": true}
	for _, name := range expected {
		allowed[name] = true
	}
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			return fmt.Errorf("%s evidence contains unexpected artifact %q", id, entry.Name())
		}
	}
	return nil
}

func ValidateWrapperLogs(dir, id string) error {
	for _, name := range []string{"stdout.log", "stderr.log"} {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if len(payload) > 8*1024*1024 || secretPattern.Match(payload) {
			return fmt.Errorf("%s wrapper log %s is unsafe", id, name)
		}
	}
	return nil
}

func ValidateRunDirectory(dir, id, class string) error {
	if err := ValidateWrapperLogs(dir, id); err != nil {
		return err
	}
	if err := ValidateInnerArtifacts(dir, id, class); err != nil {
		return err
	}
	var evidence wrapperEvidence
	if err := decodeJSON(filepath.Join(dir, "evidence.json"), &evidence); err != nil {
		return err
	}
	if evidence.SchemaVersion != 1 || evidence.AcceptanceID != id || evidence.Status != "passed" || evidence.EvidenceClass != class || evidence.ExitCode != 0 || evidence.WorktreeDirty || !evidence.RequiredNoSkip || !equalStrings(evidence.Command, CanonicalCommand(id)) || evidence.FixtureManifestPath != "testdata/design/manifest.sha256" || !sha256Pattern.MatchString(evidence.FixtureManifestSHA) {
		return fmt.Errorf("%s wrapper evidence contract is invalid", id)
	}
	return nil
}

func ValidateEvidenceRoot(root, id, class string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var problems []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := ValidateRunDirectory(filepath.Join(root, e.Name()), id, class); err == nil {
			return nil
		} else if len(problems) < 3 {
			problems = append(problems, e.Name()+": "+err.Error())
		}
	}
	return fmt.Errorf("no valid %s formal run: %s", id, strings.Join(problems, "; "))
}

func decodeJSON(path string, target any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 8*1024*1024+1))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values are not allowed")
	}
	return nil
}
func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("evidence directory is unavailable")
	}
	return nil
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
