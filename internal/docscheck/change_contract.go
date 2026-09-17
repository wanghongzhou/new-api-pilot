package docscheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func (current *checker) checkChangeContract() {
	paths, err := repositoryChangePaths(current.root)
	if err != nil {
		if _, statErr := os.Lstat(filepath.Join(current.root, ".git")); statErr == nil {
			current.add("change-contract", current.root, "inspect repository changes: %v", err)
		}
		return
	}
	implementation := false
	design := false
	tests := false
	for _, path := range paths {
		normalized := filepath.ToSlash(path)
		implementation = implementation || isImplementationChange(normalized)
		design = design || isAuthoritativeDesignChange(normalized)
		tests = tests || isTestContractChange(normalized)
	}
	if !implementation {
		return
	}
	if !design {
		current.add("change-contract", current.root, "implementation changes require an authoritative overview/detailed-design update under docs/")
	}
	if !tests {
		current.add("change-contract", current.root, "implementation changes require a deterministic test, E2E, or versioned testdata update")
	}
}

func repositoryChangePaths(root string) ([]string, error) {
	status, err := changeContractGitOutput(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var payload string
	if strings.TrimSpace(status) != "" {
		tracked, diffErr := changeContractGitOutput(root, "diff", "--name-only", "--no-renames", "HEAD", "--")
		if diffErr != nil {
			return nil, diffErr
		}
		untracked, untrackedErr := changeContractGitOutput(root, "ls-files", "--others", "--exclude-standard")
		if untrackedErr != nil {
			return nil, untrackedErr
		}
		payload = tracked + "\n" + untracked
	} else {
		if _, parentErr := changeContractGitOutput(root, "rev-parse", "--verify", "HEAD^"); parentErr != nil {
			return nil, nil
		}
		payload, err = changeContractGitOutput(root, "diff", "--name-only", "--no-renames", "HEAD^", "HEAD", "--")
		if err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	paths := []string{}
	for _, line := range strings.Split(payload, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func changeContractGitOutput(root string, args ...string) (string, error) {
	commandArgs := append([]string{"-c", "core.quotepath=false", "-C", root}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func isImplementationChange(path string) bool {
	if isTestContractChange(path) || strings.HasPrefix(path, "docs/") || strings.HasPrefix(path, "artifacts/") {
		return false
	}
	if strings.HasSuffix(path, ".gen.ts") || strings.HasSuffix(path, ".gen.go") {
		return false
	}
	for _, prefix := range []string{
		"cmd/", "common/", "constant/", "controller/", "dto/", "internal/", "middleware/",
		"migrations/", "model/", "router/", "service/", "worker/", "scripts/", "deploy/", "web/src/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	switch path {
	case "Dockerfile", "docker-compose.yml", "docker-compose.dev.yml", "go.mod", "go.sum", "Makefile", "web/package.json", "web/bun.lock", "web/rsbuild.config.ts", "web/playwright.config.ts":
		return true
	default:
		return false
	}
}

func isAuthoritativeDesignChange(path string) bool {
	if !strings.HasPrefix(path, "docs/") || !strings.HasSuffix(path, ".md") {
		return false
	}
	name := filepath.Base(filepath.FromSlash(path))
	return strings.Contains(name, "概要设计") || strings.Contains(name, "详细设计")
}

func isTestContractChange(path string) bool {
	return strings.HasPrefix(path, "testdata/") || strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx") ||
		strings.HasSuffix(path, ".spec.ts") || strings.HasSuffix(path, ".spec.tsx")
}
