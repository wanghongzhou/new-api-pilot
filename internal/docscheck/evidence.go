package docscheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"new-api-pilot/internal/a49evidence"
	"new-api-pilot/internal/acceptancecatalog"
	"new-api-pilot/internal/controlledopsevidence"

	"gopkg.in/yaml.v3"
)

const formalEvidenceClass = "formal"

type formalEvidenceRecord struct {
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

type genericEvidenceManifest struct {
	SchemaVersion      int                    `json:"schema_version"`
	AcceptanceID       string                 `json:"acceptance_id"`
	EvidenceClass      string                 `json:"evidence_class"`
	FixtureManifestSHA string                 `json:"fixture_manifest_sha256"`
	Files              []genericEvidenceEntry `json:"files"`
}

type genericEvidenceEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// checkFormalEvidenceRoot validates the generic wrapper record shared by every
// acceptance case. Specialized validators add their own artifact contracts
// after this baseline check. The normal docs check intentionally does not call
// this function so development work can retain planned or historical evidence.
func (current *checker) checkFormalEvidenceRoot(manifestPath, acceptanceID, evidenceRoot string) {
	if !current.repositoryStateReady {
		return
	}
	fixtureManifest := filepath.Join(current.root, "testdata", "design", "manifest.sha256")
	fixtureSHA, err := hashFile(fixtureManifest)
	if err != nil {
		current.add("evidence", manifestPath, "%s cannot hash current fixture manifest: %v", acceptanceID, err)
		return
	}
	rootInfo, err := os.Lstat(evidenceRoot)
	if err != nil {
		current.add("evidence", manifestPath, "%s inspect evidence directory: %v", acceptanceID, err)
		return
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		current.add("evidence", manifestPath, "%s evidence path must be a real directory", acceptanceID)
		return
	}

	entries, err := os.ReadDir(evidenceRoot)
	if err != nil {
		current.add("evidence", manifestPath, "%s read evidence directory: %v", acceptanceID, err)
		return
	}
	problems := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDirectory := filepath.Join(evidenceRoot, entry.Name())
		if err := validateFormalEvidenceRun(current.root, runDirectory, acceptanceID, fixtureSHA, current.expectedCommit); err == nil {
			if err := validateCloseoutSpecializedRun(runDirectory, acceptanceID); err == nil {
				return
			} else if len(problems) < 3 {
				problems = append(problems, entry.Name()+": "+err.Error())
			}
		} else if len(problems) < 3 {
			problems = append(problems, entry.Name()+": "+err.Error())
		}
	}
	if len(problems) == 0 {
		problems = append(problems, "no immutable run directory")
	}
	current.add("evidence", manifestPath, "%s has no valid current formal evidence: %s", acceptanceID, strings.Join(problems, "; "))
}

func validateCloseoutSpecializedRun(runDirectory, acceptanceID string) error {
	if acceptanceID == "A49" {
		return a49evidence.ValidateRunDirectory(runDirectory, a49evidence.FormalClass)
	}
	if controlledopsevidence.Supports(acceptanceID) {
		return controlledopsevidence.ValidateRunDirectory(runDirectory, acceptanceID, controlledopsevidence.FormalClass)
	}
	return nil
}

func validateFormalEvidenceRun(repositoryRoot, runDirectory, acceptanceID, fixtureSHA, expectedCommit string) error {
	info, err := os.Lstat(runDirectory)
	if err != nil {
		return fmt.Errorf("inspect run directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("run directory must be a real directory")
	}

	evidencePath, err := evidenceRunFile(runDirectory, "evidence.json")
	if err != nil {
		return err
	}
	evidenceInfo, err := os.Lstat(evidencePath)
	if err != nil {
		return fmt.Errorf("inspect evidence.json: %w", err)
	}
	if evidenceInfo.Mode()&os.ModeSymlink != 0 || !evidenceInfo.Mode().IsRegular() {
		return fmt.Errorf("evidence.json must be a regular file")
	}
	file, err := os.Open(evidencePath)
	if err != nil {
		return fmt.Errorf("open evidence.json: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var record formalEvidenceRecord
	if err := decoder.Decode(&record); err != nil {
		return fmt.Errorf("decode evidence.json: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if record.SchemaVersion != 1 || record.AcceptanceID != acceptanceID || record.Status != "passed" ||
		record.EvidenceClass != formalEvidenceClass || record.ExitCode != 0 || !record.RequiredNoSkip {
		return fmt.Errorf("wrapper result is not a passing formal required run")
	}
	if err := acceptancecatalog.ValidateCanonicalCommand(acceptanceID, record.Command); err != nil {
		return fmt.Errorf("wrapper command is not canonical: %w", err)
	}
	if !isRepositoryRelativePath(record.WorkingDirectory) {
		return fmt.Errorf("working directory is not a repository-relative path")
	}
	startedAt, err := time.Parse(time.RFC3339Nano, record.StartedAt)
	if err != nil {
		return fmt.Errorf("invalid started_at: %w", err)
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, record.FinishedAt)
	if err != nil {
		return fmt.Errorf("invalid finished_at: %w", err)
	}
	if finishedAt.Before(startedAt) || record.DurationMilliseconds < 0 ||
		finishedAt.Sub(startedAt).Milliseconds() != record.DurationMilliseconds {
		return fmt.Errorf("invalid evidence duration")
	}
	if record.WorktreeDirty {
		return fmt.Errorf("evidence was produced from a dirty worktree")
	}
	if err := validateEvidenceCommit(repositoryRoot, acceptanceID, record.Commit, expectedCommit); err != nil {
		return fmt.Errorf("evidence commit does not match the current candidate commit or a valid acceptance closeout parent: %w", err)
	}
	if record.FixtureManifestPath != "testdata/design/manifest.sha256" || record.FixtureManifestSHA != fixtureSHA {
		return fmt.Errorf("fixture manifest checksum does not match the current contract")
	}

	if record.StdoutLog == record.StderrLog || record.StdoutLog == "evidence.json" || record.StderrLog == "evidence.json" {
		return fmt.Errorf("wrapper log names must be distinct and cannot reuse evidence.json")
	}

	logSize := int64(0)
	for _, name := range []string{record.StdoutLog, record.StderrLog} {
		path, err := evidenceRunFile(runDirectory, name)
		if err != nil {
			return err
		}
		logInfo, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect wrapper log %q: %w", name, err)
		}
		if logInfo.Mode()&os.ModeSymlink != 0 || !logInfo.Mode().IsRegular() {
			return fmt.Errorf("wrapper log %q must be a regular file", name)
		}
		logSize += logInfo.Size()
	}
	if logSize == 0 {
		return fmt.Errorf("wrapper logs are empty")
	}
	if acceptancecatalog.UsesGenericEvidenceContract(acceptanceID) {
		if err := validateGenericEvidenceArtifacts(runDirectory, acceptanceID, fixtureSHA); err != nil {
			return err
		}
	}
	return nil
}

func validateGenericEvidenceArtifacts(runDirectory, acceptanceID, fixtureSHA string) error {
	manifestPath, err := evidenceRunFile(runDirectory, "run-manifest.json")
	if err != nil {
		return err
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open generic run manifest: %w", err)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest genericEvidenceManifest
	decodeErr := decoder.Decode(&manifest)
	eofErr := requireJSONEOF(decoder)
	closeErr := file.Close()
	if decodeErr != nil || eofErr != nil || closeErr != nil {
		return fmt.Errorf("decode generic run manifest: %v", errors.Join(decodeErr, eofErr, closeErr))
	}
	if manifest.SchemaVersion != 1 || manifest.AcceptanceID != acceptanceID || manifest.EvidenceClass != formalEvidenceClass || manifest.FixtureManifestSHA != fixtureSHA || len(manifest.Files) == 0 {
		return fmt.Errorf("generic run manifest contract is invalid")
	}
	wantedChecksums := make(map[string]string, len(manifest.Files)+1)
	for _, entry := range manifest.Files {
		if entry.Path == "run-manifest.json" || entry.Path == "checksums.sha256" || entry.SizeBytes < 0 || len(entry.SHA256) != 64 || entry.SHA256 != strings.ToLower(entry.SHA256) {
			return fmt.Errorf("generic run manifest entry is invalid")
		}
		path, pathErr := evidenceRunFile(runDirectory, entry.Path)
		if pathErr != nil {
			return pathErr
		}
		if _, duplicate := wantedChecksums[entry.Path]; duplicate {
			return fmt.Errorf("generic run manifest contains duplicate file %q", entry.Path)
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes {
			return fmt.Errorf("generic evidence file %q size or type mismatch", entry.Path)
		}
		digest, hashErr := hashFile(path)
		if hashErr != nil || digest != entry.SHA256 {
			return fmt.Errorf("generic evidence file %q checksum mismatch", entry.Path)
		}
		wantedChecksums[entry.Path] = entry.SHA256
	}
	manifestDigest, err := hashFile(manifestPath)
	if err != nil {
		return fmt.Errorf("hash generic run manifest: %w", err)
	}
	wantedChecksums["run-manifest.json"] = manifestDigest
	checksumPath, err := evidenceRunFile(runDirectory, "checksums.sha256")
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read generic checksums: %w", err)
	}
	seenChecksums := make(map[string]struct{}, len(wantedChecksums))
	for _, line := range strings.Split(strings.TrimSuffix(string(payload), "\n"), "\n") {
		fields := strings.Split(line, "  ")
		if len(fields) != 2 || wantedChecksums[fields[1]] != fields[0] {
			return fmt.Errorf("generic checksum entry is invalid")
		}
		if _, duplicate := seenChecksums[fields[1]]; duplicate {
			return fmt.Errorf("generic checksum entry is duplicated")
		}
		seenChecksums[fields[1]] = struct{}{}
	}
	if len(seenChecksums) != len(wantedChecksums) {
		return fmt.Errorf("generic checksum inventory is incomplete")
	}
	entries, err := os.ReadDir(runDirectory)
	if err != nil {
		return fmt.Errorf("read generic evidence directory: %w", err)
	}
	if len(entries) != len(manifest.Files)+2 {
		return fmt.Errorf("generic evidence contains an unexpected file")
	}
	return nil
}

func validateEvidenceCommit(repositoryRoot, acceptanceID, evidenceCommit, currentCommit string) error {
	if evidenceCommit == currentCommit {
		return nil
	}
	if !gitCommitPattern.MatchString(evidenceCommit) || !gitCommitPattern.MatchString(currentCommit) {
		return fmt.Errorf("commit object id is invalid")
	}
	parentLine, err := gitOutput(repositoryRoot, "rev-list", "--parents", "-n", "1", currentCommit)
	if err != nil {
		return fmt.Errorf("resolve closeout parent: %w", err)
	}
	parents := strings.Fields(parentLine)
	if len(parents) != 2 || parents[0] != currentCommit || parents[1] != evidenceCommit {
		return fmt.Errorf("evidence commit is not the sole direct parent")
	}
	changed, err := gitOutput(repositoryRoot, "diff", "--name-only", "--no-renames", evidenceCommit, currentCommit)
	if err != nil {
		return fmt.Errorf("inspect closeout files: %w", err)
	}
	if strings.TrimSpace(changed) != acceptanceManifestPath {
		return fmt.Errorf("closeout commit changed files other than %s", acceptanceManifestPath)
	}
	beforePayload, err := gitOutputBytes(repositoryRoot, "show", evidenceCommit+":"+acceptanceManifestPath)
	if err != nil {
		return fmt.Errorf("read parent acceptance manifest: %w", err)
	}
	afterPayload, err := gitOutputBytes(repositoryRoot, "show", currentCommit+":"+acceptanceManifestPath)
	if err != nil {
		return fmt.Errorf("read closeout acceptance manifest: %w", err)
	}
	before, err := decodeAcceptanceManifest(beforePayload)
	if err != nil {
		return fmt.Errorf("decode parent acceptance manifest: %w", err)
	}
	after, err := decodeAcceptanceManifest(afterPayload)
	if err != nil {
		return fmt.Errorf("decode closeout acceptance manifest: %w", err)
	}
	if before.SchemaVersion != after.SchemaVersion || !reflect.DeepEqual(before.Baseline, after.Baseline) || !reflect.DeepEqual(before.Fixtures, after.Fixtures) || len(before.AcceptanceCases) != len(after.AcceptanceCases) {
		return fmt.Errorf("closeout manifest changed non-evidence contracts")
	}
	transitioned := false
	for index := range before.AcceptanceCases {
		oldCase := before.AcceptanceCases[index]
		newCase := after.AcceptanceCases[index]
		oldEvidence := oldCase.EvidencePath
		newEvidence := newCase.EvidencePath
		oldCase.EvidencePath = ""
		newCase.EvidencePath = ""
		if !reflect.DeepEqual(oldCase, newCase) {
			return fmt.Errorf("closeout manifest changed acceptance contract %s", oldCase.AcceptanceID)
		}
		if oldEvidence == newEvidence {
			continue
		}
		if !strings.HasPrefix(oldEvidence, "planned:") || strings.TrimPrefix(oldEvidence, "planned:") != newEvidence || strings.HasPrefix(newEvidence, "planned:") {
			return fmt.Errorf("closeout manifest made a change other than removing planned from %s evidence_path", oldCase.AcceptanceID)
		}
		if oldCase.AcceptanceID == acceptanceID {
			transitioned = true
		}
	}
	if !transitioned {
		return fmt.Errorf("%s evidence_path was not finalized by the closeout commit", acceptanceID)
	}
	return nil
}

func decodeAcceptanceManifest(payload []byte) (acceptanceManifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	var manifest acceptanceManifest
	if err := decoder.Decode(&manifest); err != nil {
		return acceptanceManifest{}, err
	}
	return manifest, nil
}

func gitOutput(repositoryRoot string, arguments ...string) (string, error) {
	payload, err := gitOutputBytes(repositoryRoot, arguments...)
	return strings.TrimSpace(string(payload)), err
}

func gitOutputBytes(repositoryRoot string, arguments ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", repositoryRoot}, arguments...)...)
	payload, err := command.Output()
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func evidenceRunFile(runDirectory, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || name == "." ||
		strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return "", fmt.Errorf("unsafe evidence file path %q", name)
	}
	return filepath.Join(runDirectory, name), nil
}

func isRepositoryRelativePath(value string) bool {
	if value == "." {
		return true
	}
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return false
	}
	cleaned := path.Clean(value)
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return fmt.Errorf("multiple JSON values are not allowed")
}
