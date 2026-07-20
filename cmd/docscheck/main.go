package main

import (
	"flag"
	"fmt"
	"os"

	"new-api-pilot/internal/docscheck"
)

func main() {
	root := flag.String("root", ".", "repository root to validate")
	final := flag.Bool("final", false, "reject every planned acceptance path")
	flag.Parse()

	issues := docscheck.CheckWithOptions(*root, docscheck.Options{RequireNoPlanned: *final})
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
