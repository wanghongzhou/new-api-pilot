package docscheck

import (
	"fmt"
	"path/filepath"
	"sort"
)

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
	root    string
	options Options
	issues  []Issue
}

type Options struct {
	RequireNoPlanned bool
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
	trace := current.checkTraceability()
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
