package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"new-api-pilot/internal/a22evidence"
	"new-api-pilot/internal/a25evidence"
	"new-api-pilot/internal/a45evidence"
	"new-api-pilot/internal/a49evidence"
	"new-api-pilot/internal/a50evidence"
	"new-api-pilot/internal/a62evidence"
	"new-api-pilot/internal/opsevidence"
)

const evidenceSchemaVersion = 1

var acceptanceIDPattern = regexp.MustCompile(`^A(?:0[1-9]|[1-9]\d|10[0-2])$`)

type evidenceRecord struct {
	SchemaVersion        int      `json:"schema_version"`
	AcceptanceID         string   `json:"acceptance_id"`
	Status               string   `json:"status"`
	EvidenceClass        string   `json:"evidence_class,omitempty"`
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

type genericRunManifest struct {
	SchemaVersion      int                   `json:"schema_version"`
	AcceptanceID       string                `json:"acceptance_id"`
	EvidenceClass      string                `json:"evidence_class"`
	FixtureManifestSHA string                `json:"fixture_manifest_sha256"`
	Files              []genericRunFileEntry `json:"files"`
}

type genericRunFileEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout io.Writer, stderr io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprintln(stderr, "usage: acceptance <schema-version|run|docs-negative|a49-seed|a49-load|a49-report|a51-preflight|a51-seed|a51-verify|a51-report> [arguments]")
		return 2
	}
	switch arguments[0] {
	case "schema-version":
		return runSchemaVersion(arguments[1:], stdout, stderr)
	case "run":
		return runCase(arguments[1:], stdout, stderr)
	case "docs-negative":
		return runDocsNegative(arguments[1:], stdout, stderr)
	case "a49-seed":
		return runA49Seed(arguments[1:], stdout, stderr)
	case "a49-load":
		return runA49Load(arguments[1:], stdout, stderr)
	case "a49-report":
		return runA49Report(arguments[1:], stdout, stderr)
	case "a51-preflight":
		return runA51Preflight(arguments[1:], stdout, stderr)
	case "a51-seed":
		return runA51Seed(arguments[1:], stdout, stderr)
	case "a51-verify":
		return runA51Verify(arguments[1:], stdout, stderr)
	case "a51-report":
		return runA51Report(arguments[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown acceptance command %q\n", arguments[0])
		return 2
	}
}

func runSchemaVersion(arguments []string, stdout io.Writer, stderr io.Writer) int {
	if len(arguments) != 0 {
		fmt.Fprintln(stderr, "schema-version does not accept arguments")
		return 2
	}
	payload := struct {
		SchemaVersion        int      `json:"schema_version"`
		A22EnvironmentImages []string `json:"a22_environment_images"`
	}{
		SchemaVersion:        evidenceSchemaVersion,
		A22EnvironmentImages: []string{"go", "mysql", "redis", "tools"},
	}
	if err := json.NewEncoder(stdout).Encode(payload); err != nil {
		fmt.Fprintf(stderr, "encode schema version: %v\n", err)
		return 1
	}
	return 0
}

func runCase(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	acceptanceID := flags.String("case", "", "acceptance ID, for example A83")
	root := flags.String("root", ".", "repository root")
	workingDirectory := flags.String("cwd", ".", "command working directory relative to root")
	evidenceRoot := flags.String("evidence-root", "artifacts/acceptance", "evidence directory relative to root")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	commandArguments := flags.Args()
	if !acceptanceIDPattern.MatchString(*acceptanceID) || len(commandArguments) == 0 {
		fmt.Fprintln(stderr, "run requires -case Axx and a command after --")
		return 2
	}
	a22EvidenceClass, err := a22evidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "A22 evidence guard: %v\n", err)
		return 2
	}
	a25EvidenceClass, err := a25evidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "A25 evidence guard: %v\n", err)
		return 2
	}
	a45EvidenceClass, err := a45evidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "A45 evidence guard: %v\n", err)
		return 2
	}
	a49EvidenceClass, err := classifyA49Evidence(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "A49 evidence guard: %v\n", err)
		return 2
	}
	a50EvidenceClass, err := a50evidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "A50 evidence guard: %v\n", err)
		return 2
	}
	opsEvidenceClass, err := opsevidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "%s evidence guard: %v\n", *acceptanceID, err)
		return 2
	}
	a62EvidenceClass, err := a62evidence.Classify(*acceptanceID, *evidenceRoot, commandArguments)
	if err != nil {
		fmt.Fprintf(stderr, "%s evidence guard: %v\n", *acceptanceID, err)
		return 2
	}
	evidenceClass := a22EvidenceClass
	if evidenceClass == "" {
		evidenceClass = a25EvidenceClass
	}
	if evidenceClass == "" {
		evidenceClass = a45EvidenceClass
	}
	if evidenceClass == "" {
		evidenceClass = a49EvidenceClass
	}
	if evidenceClass == "" {
		evidenceClass = a50EvidenceClass
	}
	if evidenceClass == "" {
		evidenceClass = opsEvidenceClass
	}
	if evidenceClass == "" {
		evidenceClass = a62EvidenceClass
	}
	if evidenceClass == "" {
		genericEvidenceClass, classifyErr := classifyGenericEvidence(*evidenceRoot)
		if classifyErr != nil {
			fmt.Fprintf(stderr, "generic evidence guard: %v\n", classifyErr)
			return 2
		}
		evidenceClass = genericEvidenceClass
	}

	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(stderr, "resolve repository root: %v\n", err)
		return 2
	}
	commandDirectory, err := repositoryPath(absoluteRoot, *workingDirectory)
	if err != nil {
		fmt.Fprintf(stderr, "resolve command directory: %v\n", err)
		return 2
	}
	evidenceDirectory, err := repositoryPath(absoluteRoot, *evidenceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "resolve evidence directory: %v\n", err)
		return 2
	}
	startedAt := time.Now().UTC()
	runID := startedAt.Format("20060102T150405.000000000Z") + fmt.Sprintf("-%d", os.Getpid())
	runDirectory := filepath.Join(evidenceDirectory, *acceptanceID, runID)
	if err := os.MkdirAll(runDirectory, 0o750); err != nil {
		fmt.Fprintf(stderr, "create evidence directory: %v\n", err)
		return 2
	}
	stdoutPath := filepath.Join(runDirectory, "stdout.log")
	stderrPath := filepath.Join(runDirectory, "stderr.log")
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		fmt.Fprintf(stderr, "create stdout log: %v\n", err)
		return 2
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		_ = stdoutFile.Close()
		fmt.Fprintf(stderr, "create stderr log: %v\n", err)
		return 2
	}
	defer stderrFile.Close()

	command := exec.Command(commandArguments[0], commandArguments[1:]...)
	command.Dir = commandDirectory
	runnerExecutable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve acceptance runner executable: %v\n", err)
		return 2
	}
	command.Env = append(os.Environ(),
		"ACCEPTANCE_ID="+*acceptanceID,
		"ACCEPTANCE_EVIDENCE_DIR="+runDirectory,
		"ACCEPTANCE_RUNNER_EXE="+runnerExecutable,
	)
	if evidenceClass != "" {
		command.Env = append(command.Env, "ACCEPTANCE_EVIDENCE_CLASS="+evidenceClass)
	}
	command.Stdout = io.MultiWriter(stdout, stdoutFile)
	command.Stderr = io.MultiWriter(stderr, stderrFile)
	runError := command.Run()
	logCloseError := errors.Join(stdoutFile.Sync(), stderrFile.Sync(), stdoutFile.Close(), stderrFile.Close())
	finishedAt := time.Now().UTC()
	exitCode := commandExitCode(runError)
	if logCloseError != nil {
		message := fmt.Sprintf("close acceptance wrapper logs: %v\n", logCloseError)
		fmt.Fprint(stderr, message)
		_ = appendAcceptanceLog(stderrPath, message)
		exitCode = 1
	}
	if opsEvidenceClass != "" {
		if validationError := opsevidence.ValidateWrapperLogs(runDirectory, *acceptanceID); validationError != nil {
			message := fmt.Sprintf("%s wrapper log validation failed: %v\n", *acceptanceID, validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if a62EvidenceClass != "" {
		if validationError := a62evidence.ValidateWrapperLogs(runDirectory); validationError != nil {
			message := fmt.Sprintf("A62 wrapper log validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if a25EvidenceClass != "" {
		if validationError := a25evidence.ValidateWrapperLogs(runDirectory); validationError != nil {
			message := fmt.Sprintf("A25 wrapper log validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if a45EvidenceClass != "" {
		if validationError := a45evidence.ValidateWrapperLogs(runDirectory); validationError != nil {
			message := fmt.Sprintf("A45 wrapper log validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if a50EvidenceClass != "" {
		if validationError := a50evidence.ValidateWrapperLogs(runDirectory); validationError != nil {
			message := fmt.Sprintf("A50 wrapper log validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if a22EvidenceClass != "" {
		if validationError := a22evidence.ValidateWrapperLogs(runDirectory); validationError != nil {
			message := fmt.Sprintf("A22 wrapper log validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a22EvidenceClass != "" {
		if validationError := a22evidence.ValidateInnerArtifacts(runDirectory, a22EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A22 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a25EvidenceClass != "" {
		if validationError := a25evidence.ValidateInnerArtifacts(runDirectory, a25EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A25 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a45EvidenceClass != "" {
		if validationError := a45evidence.ValidateInnerArtifacts(runDirectory, a45EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A45 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a49EvidenceClass != "" {
		if validationError := a49evidence.ValidateInnerArtifacts(runDirectory, a49EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A49 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a50EvidenceClass != "" {
		if validationError := a50evidence.ValidateInnerArtifacts(runDirectory, a50EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A50 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && opsEvidenceClass != "" {
		if validationError := opsevidence.ValidateInnerArtifacts(runDirectory, *acceptanceID, opsEvidenceClass); validationError != nil {
			message := fmt.Sprintf("%s inner evidence validation failed: %v\n", *acceptanceID, validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	if exitCode == 0 && a62EvidenceClass != "" {
		if validationError := a62evidence.ValidateInnerArtifacts(runDirectory, a62EvidenceClass); validationError != nil {
			message := fmt.Sprintf("A62 inner evidence validation failed: %v\n", validationError)
			fmt.Fprint(stderr, message)
			_ = appendAcceptanceLog(stderrPath, message)
			exitCode = 1
		}
	}
	status := "passed"
	if exitCode != 0 {
		status = "failed"
	}
	relativeWorkingDirectory, _ := filepath.Rel(absoluteRoot, commandDirectory)
	commit, dirty := gitState(absoluteRoot)
	fixturePath := filepath.Join("testdata", "design", "manifest.sha256")
	fixtureSHA, _ := fileSHA256(filepath.Join(absoluteRoot, fixturePath))
	record := evidenceRecord{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: *acceptanceID, Status: status,
		EvidenceClass: evidenceClass,
		Command:       commandArguments, WorkingDirectory: filepath.ToSlash(relativeWorkingDirectory),
		StartedAt: startedAt.Format(time.RFC3339Nano), FinishedAt: finishedAt.Format(time.RFC3339Nano),
		DurationMilliseconds: finishedAt.Sub(startedAt).Milliseconds(), ExitCode: exitCode,
		Commit: commit, WorktreeDirty: dirty, FixtureManifestPath: filepath.ToSlash(fixturePath),
		FixtureManifestSHA: fixtureSHA, StdoutLog: "stdout.log", StderrLog: "stderr.log",
		RequiredNoSkip: true,
	}
	if err := writeJSONAtomic(filepath.Join(runDirectory, "evidence.json"), record); err != nil {
		fmt.Fprintf(stderr, "write evidence metadata: %v\n", err)
		return 2
	}
	if a22EvidenceClass == "" && a25EvidenceClass == "" && a45EvidenceClass == "" &&
		a49EvidenceClass == "" && a50EvidenceClass == "" && opsEvidenceClass == "" && a62EvidenceClass == "" {
		if err := finalizeGenericEvidence(runDirectory, *acceptanceID, evidenceClass, fixtureSHA); err != nil {
			fmt.Fprintf(stderr, "finalize generic evidence: %v\n", err)
			return 2
		}
	}
	fmt.Fprintf(stdout, "acceptance evidence: %s\n", filepath.ToSlash(runDirectory))
	return exitCode
}

func finalizeGenericEvidence(runDirectory, acceptanceID, evidenceClass, fixtureSHA string) error {
	entries, err := collectGenericRunFiles(runDirectory, map[string]struct{}{
		"run-manifest.json": {},
		"checksums.sha256":  {},
	})
	if err != nil {
		return err
	}
	manifest := genericRunManifest{
		SchemaVersion: 1, AcceptanceID: acceptanceID, EvidenceClass: evidenceClass,
		FixtureManifestSHA: fixtureSHA, Files: entries,
	}
	if err := writeJSONAtomic(filepath.Join(runDirectory, "run-manifest.json"), manifest); err != nil {
		return fmt.Errorf("write run manifest: %w", err)
	}
	checksumEntries, err := collectGenericRunFiles(runDirectory, map[string]struct{}{"checksums.sha256": {}})
	if err != nil {
		return err
	}
	var payload strings.Builder
	for _, entry := range checksumEntries {
		fmt.Fprintf(&payload, "%s  %s\n", entry.SHA256, entry.Path)
	}
	checksumPath := filepath.Join(runDirectory, "checksums.sha256")
	return os.WriteFile(checksumPath, []byte(payload.String()), 0o640)
}

func collectGenericRunFiles(runDirectory string, excluded map[string]struct{}) ([]genericRunFileEntry, error) {
	directoryEntries, err := os.ReadDir(runDirectory)
	if err != nil {
		return nil, fmt.Errorf("read run directory: %w", err)
	}
	files := make([]genericRunFileEntry, 0, len(directoryEntries))
	for _, entry := range directoryEntries {
		if _, skip := excluded[entry.Name()]; skip {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("generic evidence contains unsupported non-file %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("inspect generic evidence file %q: %w", entry.Name(), err)
		}
		digest, err := fileSHA256(filepath.Join(runDirectory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("hash generic evidence file %q: %w", entry.Name(), err)
		}
		files = append(files, genericRunFileEntry{Path: entry.Name(), SizeBytes: info.Size(), SHA256: digest})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func appendAcceptanceLog(path, message string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, message)
	return err
}

func classifyA49Evidence(acceptanceID, evidenceRoot string, command []string) (string, error) {
	if acceptanceID != a49AcceptanceID {
		return "", nil
	}
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	class := ""
	switch cleaned {
	case "artifacts/acceptance":
		class = a49evidence.FormalClass
	case "artifacts/smoke":
		class = a49evidence.SmokeClass
	default:
		return "", errors.New("A49 evidence root must be canonical artifacts/acceptance or artifacts/smoke")
	}
	if err := a49evidence.ValidateCanonicalCommand(command, class); err != nil {
		return "", err
	}
	return class, nil
}

func classifyGenericEvidence(evidenceRoot string) (string, error) {
	cleaned := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(evidenceRoot)), "./")
	switch cleaned {
	case "artifacts/acceptance":
		return "formal", nil
	case "artifacts/smoke":
		return "development", nil
	default:
		return "", errors.New("evidence root must be canonical artifacts/acceptance or artifacts/smoke")
	}
}

func repositoryPath(root string, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("path must be relative to the repository root")
	}
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the repository root")
	}
	return resolved, nil
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 127
}

func gitState(root string) (string, bool) {
	commit := "unborn"
	if output, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(output))
	}
	dirty := true
	if output, err := exec.Command("git", "-C", root, "status", "--porcelain").Output(); err == nil {
		dirty = len(strings.TrimSpace(string(output))) > 0
	}
	return commit, dirty
}

func fileSHA256(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func writeJSONAtomic(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o640); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
