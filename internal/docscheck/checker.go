package docscheck

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

type Issue struct {
	Check   string
	Path    string
	Message string
}

func (issue Issue) String() string {
	if issue.Path == "" {
		return fmt.Sprintf("[%s] %s", issue.Check, issue.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", issue.Check, filepath.ToSlash(issue.Path), issue.Message)
}

type checker struct {
	root                 string
	options              Options
	issues               []Issue
	expectedCommit       string
	worktreeClean        bool
	repositoryStateReady bool
}

type Options struct {
	RequireNoPlanned      bool
	ExpectedGitCommit     string
	ExpectedWorktreeClean *bool
}

func Check(root string) []Issue {
	return CheckWithOptions(root, Options{})
}

func CheckWithOptions(root string, options Options) []Issue {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return []Issue{{Check: "root", Message: fmt.Sprintf("resolve repository root: %v", err)}}
	}

	current := &checker{root: filepath.Clean(absoluteRoot), options: options}
	if options.RequireNoPlanned {
		current.initializeFinalRepositoryState()
	}
	trace := current.checkTraceability()
	current.checkAuthoritativeDesignContracts()
	manifest := current.checkAcceptanceManifest(trace)
	current.checkSiteTaskCatalog()
	current.checkDataMaintenanceCatalog()
	current.checkFixtureChecksums(manifest)
	catalog := current.checkMessageCatalog()
	current.checkMessageRefOpenAPI(catalog)
	current.checkFrontendMessageContract(catalog)
	current.checkLocales(catalog)
	current.checkMarkdownLinks()

	sort.Slice(current.issues, func(left int, right int) bool {
		if current.issues[left].Check != current.issues[right].Check {
			return current.issues[left].Check < current.issues[right].Check
		}
		if current.issues[left].Path != current.issues[right].Path {
			return current.issues[left].Path < current.issues[right].Path
		}
		return current.issues[left].Message < current.issues[right].Message
	})
	return current.issues
}

func (current *checker) initializeFinalRepositoryState() {
	commit := strings.TrimSpace(current.options.ExpectedGitCommit)
	clean := current.options.ExpectedWorktreeClean
	if commit != "" || clean != nil {
		if commit == "" || clean == nil {
			current.add("git", current.root, "final evidence validation requires both expected git commit and worktree state")
			return
		}
		if !gitCommitPattern.MatchString(commit) {
			current.add("git", current.root, "expected git commit is not a full hexadecimal object id")
			return
		}
		current.expectedCommit = commit
		current.worktreeClean = *clean
		current.repositoryStateReady = true
	} else {
		resolvedCommit, resolvedClean, err := repositoryGitState(current.root)
		if err != nil {
			current.add("git", current.root, "resolve current repository state: %v", err)
			return
		}
		current.expectedCommit = resolvedCommit
		current.worktreeClean = resolvedClean
		current.repositoryStateReady = true
	}
	if !current.worktreeClean {
		current.add("git", current.root, "current worktree is dirty; final evidence requires a clean candidate commit")
	}
}

func repositoryGitState(root string) (string, bool, error) {
	commitOutput, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return "", false, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	commit := strings.TrimSpace(string(commitOutput))
	if !gitCommitPattern.MatchString(commit) {
		return "", false, fmt.Errorf("git rev-parse returned an invalid object id")
	}
	statusOutput, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return "", false, fmt.Errorf("git status: %w", err)
	}
	return commit, len(strings.TrimSpace(string(statusOutput))) == 0, nil
}

func (current *checker) add(check string, path string, format string, args ...any) {
	if path != "" {
		if relative, err := filepath.Rel(current.root, path); err == nil {
			path = relative
		}
	}
	current.issues = append(current.issues, Issue{
		Check:   check,
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	})
}
