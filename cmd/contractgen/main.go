package main

import (
	"flag"
	"fmt"
	"os"

	"new-api-pilot/internal/docscheck"
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := docscheck.WriteMessageRefOpenAPI(*root); err != nil {
		fmt.Fprintf(os.Stderr, "generate MessageRef OpenAPI contract: %v\n", err)
		os.Exit(1)
	}
	if err := docscheck.WriteFixtureChecksumManifest(*root); err != nil {
		fmt.Fprintf(os.Stderr, "generate fixture checksum manifest: %v\n", err)
		os.Exit(1)
	}
}
