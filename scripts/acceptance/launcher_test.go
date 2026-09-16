package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsLauncherBuildsCurrentHarnessInDocker(t *testing.T) {
	payload, err := os.ReadFile("run.ps1")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(payload), "\r\n", "\n")
	for _, required := range []string{
		"new-api-pilot-go-test:latest",
		"type=bind,source=$repositoryRoot,target=/workspace,readonly",
		"type=bind,source=$outputDirectory,target=/out",
		"CGO_ENABLED=0",
		"GOOS=windows",
		"GOARCH=amd64",
		"/out/acceptance.exe",
		"./scripts/acceptance",
		"$acceptanceArguments = @($args)",
		"& $runnerPath @acceptanceArguments",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("Windows acceptance launcher is missing %q", required)
		}
	}
	for _, forbidden := range []string{"go run", "go install", "winget", "choco"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("Windows acceptance launcher depends on host tooling %q", forbidden)
		}
	}
}
