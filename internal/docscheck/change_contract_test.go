package docscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChangeContractRequiresDesignAndTestsForImplementation(t *testing.T) {
	root := initializeChangeContractRepository(t)
	writeTestFile(t, filepath.Join(root, "service", "example.go"), "package service\n")

	current := &checker{root: root}
	current.checkChangeContract()
	if !containsIssue(current.issues, "authoritative overview/detailed-design") ||
		!containsIssue(current.issues, "deterministic test") {
		t.Fatalf("implementation-only change bypassed contract: %#v", current.issues)
	}

	writeTestFile(t, filepath.Join(root, "docs", "产品-详细设计.md"), "# updated\n")
	writeTestFile(t, filepath.Join(root, "service", "example_test.go"), "package service\n")
	current = &checker{root: root}
	current.checkChangeContract()
	if len(current.issues) != 0 {
		t.Fatalf("complete vertical slice produced issues: %#v", current.issues)
	}
}

func TestChangeContractUsesLastCommitWhenWorktreeIsClean(t *testing.T) {
	root := initializeChangeContractRepository(t)
	writeTestFile(t, filepath.Join(root, "controller", "example.go"), "package controller\n")
	runGitTest(t, root, "add", ".")
	runGitTest(t, root, "commit", "-m", "implementation without contract")

	current := &checker{root: root}
	current.checkChangeContract()
	if len(current.issues) != 2 {
		t.Fatalf("clean committed implementation bypassed contract: %#v", current.issues)
	}
}

func TestChangeContractIgnoresDocumentationOnlyChanges(t *testing.T) {
	root := initializeChangeContractRepository(t)
	writeTestFile(t, filepath.Join(root, "docs", "产品-详细设计.md"), "# docs only\n")
	current := &checker{root: root}
	current.checkChangeContract()
	if len(current.issues) != 0 {
		t.Fatalf("documentation-only change produced issues: %#v", current.issues)
	}
}

func initializeChangeContractRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"service", "controller", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runGitTest(t, root, "init")
	runGitTest(t, root, "config", "user.email", "docscheck@example.invalid")
	runGitTest(t, root, "config", "user.name", "docscheck")
	writeTestFile(t, filepath.Join(root, "README.md"), "baseline\n")
	runGitTest(t, root, "add", ".")
	runGitTest(t, root, "commit", "-m", "baseline")
	return root
}
