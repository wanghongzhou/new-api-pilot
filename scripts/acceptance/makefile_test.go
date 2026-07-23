package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceBackendValidationUsesDockerTargets(t *testing.T) {
	payload, err := os.ReadFile("../../makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(payload), "\r\n", "\n")
	start := strings.Index(text, "\nacceptance:\n")
	if start < 0 {
		t.Fatal("makefile acceptance target is missing")
	}
	target := text[start:]
	for _, required := range []string{"$(MAKE) docs-check-final-docker", "$(MAKE) test-api-docker"} {
		if !strings.Contains(target, required) {
			t.Fatalf("acceptance target is missing %q", required)
		}
	}
	for _, forbidden := range []string{"$(MAKE) docs-check-final\n", "$(MAKE) test-api\n", "TEST_DATABASE_DSN is required"} {
		if strings.Contains(target, forbidden) {
			t.Fatalf("acceptance target still depends on host backend validation %q", forbidden)
		}
	}
}

func TestDockerBackendValidationUsesIsolatedProtectedDatabase(t *testing.T) {
	payload, err := os.ReadFile("../../makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(payload), "\r\n", "\n")
	start := strings.Index(text, "\ntest-api-docker:\n")
	if start < 0 {
		t.Fatal("test-api-docker target is missing")
	}
	body := text[start+1:]
	if next := strings.Index(body, "\n\n"); next >= 0 {
		body = body[:next]
	}
	for _, required := range []string{
		"docker build --target go-test-runner",
		"sh scripts/test-api-docker.sh",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("test-api-docker target is missing %q", required)
		}
	}

	scriptPath := filepath.Join("..", "test-api-docker.sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	scriptText := strings.ReplaceAll(string(script), "\r\n", "\n")
	for _, required := range []string{
		"new_api_pilot_test_$(date +%s)_$$",
		"TEST_API_DOCKER_GOPROXY:-off",
		"TEST_API_DOCKER_GOSUMDB:-off",
		"\"$test_image\" go list $go_packages",
		"-e \"GOPROXY=$test_go_proxy\"",
		"-e \"GOSUMDB=$test_go_sum_database\"",
		"current_database=\"${test_database_prefix}_${index}\"",
		"MSYS_NO_PATHCONV=1",
		"target=/root/.cache/go-build",
		"DROP DATABASE IF EXISTS",
		"CREATE DATABASE",
		"TEST_DATABASE_ADMIN_DSN",
		"trap cleanup EXIT HUP INT TERM",
	} {
		if !strings.Contains(scriptText, required) {
			t.Fatalf("test database runner is missing %q", required)
		}
	}

	command := exec.Command("sh", scriptPath)
	command.Env = append(os.Environ(),
		"TEST_API_DOCKER_VALIDATE_ONLY=1",
		"TEST_DATABASE_NAME=new_api_pilot",
	)
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "refusing unsafe test database name") {
		t.Fatalf("development database guard output=%q err=%v", output, err)
	}

	command = exec.Command("sh", scriptPath)
	command.Env = append(os.Environ(),
		"TEST_API_DOCKER_VALIDATE_ONLY=1",
		"TEST_DATABASE_NAME=new_api_pilot_test_regression",
	)
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "new_api_pilot_test_regression" {
		t.Fatalf("valid test database output=%q err=%v", output, err)
	}
}

func TestDockerDocsCheckTargetsReadTheRealWorkspace(t *testing.T) {
	payload, err := os.ReadFile("../../makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(payload), "\r\n", "\n")
	for _, targetName := range []string{"docs-check-docker", "docs-check-final-docker"} {
		start := strings.Index(text, "\n"+targetName+":\n")
		if start < 0 {
			t.Fatalf("%s target is missing", targetName)
		}
		body := text[start+1:]
		if next := strings.Index(body, "\n\n"); next >= 0 {
			body = body[:next]
		}
		for _, required := range []string{
			`-v "$(CURDIR):/workspace:ro"`,
			"-w /workspace",
			"go run ./cmd/docscheck -root .",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s target is missing %q", targetName, required)
			}
		}
		if strings.Contains(body, "docker run --rm new-api-pilot-go-test:latest go run") {
			t.Fatalf("%s still runs docs-check against the image-only filesystem", targetName)
		}
	}
	finalStart := strings.Index(text, "\ndocs-check-final-docker:\n")
	if finalStart < 0 || !strings.Contains(text[finalStart:], "go run ./cmd/docscheck -root . -final") {
		t.Fatal("docs-check-final-docker is missing final mode")
	}
}

func TestDockerBuildSourcesAreConfigurableAndDomesticByDefault(t *testing.T) {
	dockerfilePayload, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := strings.ReplaceAll(string(dockerfilePayload), "\r\n", "\n")
	for _, required := range []string{
		"ARG BUN_IMAGE=docker.m.daocloud.io/oven/bun:1.3.13-alpine",
		"ARG GO_IMAGE=docker.m.daocloud.io/library/golang:1.25-alpine",
		"ARG RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.22",
		"FROM ${BUN_IMAGE} AS web-builder",
		"FROM ${GO_IMAGE} AS go-deps",
		"FROM go-deps AS go-test-runner",
		"FROM go-test-runner AS go-builder",
		"FROM ${RUNTIME_IMAGE}",
		"ARG GO_MODULE_PROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct",
		"ARG GO_SUM_DATABASE=sum.golang.google.cn",
		"ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine",
		"--mount=type=cache,target=/root/.cache/go-mod",
		"GOMODCACHE=/root/.cache/go-mod go mod download",
		"cp -a /root/.cache/go-mod/. /go/pkg/mod/",
		"--mount=type=cache,target=/go/pkg/mod",
		"--mount=type=cache,target=/root/.cache/go-build",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("Dockerfile is missing configurable build source %q", required)
		}
	}
	composePayload, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := strings.ReplaceAll(string(composePayload), "\r\n", "\n")
	for _, required := range []string{
		"BUN_IMAGE: ${BUN_IMAGE:-docker.m.daocloud.io/oven/bun:1.3.13-alpine}",
		"GO_IMAGE: ${GO_IMAGE:-docker.m.daocloud.io/library/golang:1.25-alpine}",
		"RUNTIME_IMAGE: ${RUNTIME_IMAGE:-docker.m.daocloud.io/library/alpine:3.22}",
		"image: ${MYSQL_IMAGE:-docker.m.daocloud.io/library/mysql:8.4}",
		"image: ${REDIS_IMAGE:-docker.m.daocloud.io/library/redis:7-alpine}",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("docker-compose.yml is missing configurable image source %q", required)
		}
	}
}
