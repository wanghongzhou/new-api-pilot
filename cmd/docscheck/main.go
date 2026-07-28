package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"new-api-pilot/internal/docscheck"
)

func main() {
	root := flag.String("root", ".", "repository root to validate")
	final := flag.Bool("final", false, "reject every planned acceptance path")
	flag.Parse()

	options := docscheck.Options{RequireNoPlanned: *final}
	if *final {
		commit := strings.TrimSpace(os.Getenv("EXPECTED_GIT_COMMIT"))
		cleanValue := strings.TrimSpace(os.Getenv("EXPECTED_GIT_WORKTREE_CLEAN"))
		if commit != "" || cleanValue != "" {
			clean := cleanValue == "true"
			if cleanValue != "true" && cleanValue != "false" {
				fmt.Fprintln(os.Stderr, "docs-check failed: EXPECTED_GIT_WORKTREE_CLEAN must be true or false")
				os.Exit(2)
			}
			options.ExpectedGitCommit = commit
			options.ExpectedWorktreeClean = &clean
		}
	}
	issues := docscheck.CheckWithOptions(*root, options)
	if len(issues) == 0 {
		fmt.Println("docs-check passed")
		return
	}

	fmt.Fprintf(os.Stderr, "docs-check failed with %d issue(s):\n", len(issues))
	for _, issue := range issues {
		fmt.Fprintf(os.Stderr, "- %s\n", issue.String())
	}
	os.Exit(1)
}
