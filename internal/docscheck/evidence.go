package docscheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
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

// checkFormalEvidenceRoot validates the generic wrapper record shared by every
// acceptance case. Specialized validators add their own artifact contracts
// after this baseline check. The normal docs check intentionally does not call
// this function so development work can retain planned or historical evidence.
func (current *checker) checkFormalEvidenceRoot(manifestPath, acceptanceID, evidenceRoot string) {
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
		if err := validateFormalEvidenceRun(runDirectory, acceptanceID, fixtureSHA); err == nil {
			return
		} else if len(problems) < 3 {
			problems = append(problems, entry.Name()+": "+err.Error())
		}
	}
	if len(problems) == 0 {
		problems = append(problems, "no immutable run directory")
	}
	current.add("evidence", manifestPath, "%s has no valid current formal evidence: %s", acceptanceID, strings.Join(problems, "; "))
}

func validateFormalEvidenceRun(runDirectory, acceptanceID, fixtureSHA string) error {
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
	if len(record.Command) == 0 || strings.TrimSpace(record.Command[0]) == "" {
		return fmt.Errorf("wrapper command is empty")
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
	if strings.TrimSpace(record.Commit) == "" {
		return fmt.Errorf("commit is empty")
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
	return nil
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
