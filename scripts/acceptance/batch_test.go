package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"new-api-pilot/internal/acceptancecatalog"
)

func TestBuildBatchPlanSkipsPlannedAndGroupsResources(t *testing.T) {
	root := t.TempDir()
	writeBatchRunner(t, root, "scripts/acceptance/run-generic-case.ps1")
	writeBatchRunner(t, root, "scripts/acceptance/run-a22.ps1")
	manifest := batchManifest{}
	manifest.Baseline.ReleasePolicy = "required-no-skip"
	manifest.AcceptanceCases = append(manifest.AcceptanceCases,
		struct {
			AcceptanceID string `yaml:"acceptance_id"`
			EvidencePath string `yaml:"evidence_path"`
		}{"A01", "artifacts/acceptance/A01/"},
		struct {
			AcceptanceID string `yaml:"acceptance_id"`
			EvidencePath string `yaml:"evidence_path"`
		}{"A22", "artifacts/acceptance/A22/"},
		struct {
			AcceptanceID string `yaml:"acceptance_id"`
			EvidencePath string `yaml:"evidence_path"`
		}{"A49", "planned:artifacts/acceptance/A49/"},
	)
	plan, err := buildBatchPlan(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Parallel) != 1 || plan.Parallel[0].AcceptanceID != "A01" {
		t.Fatalf("parallel plan = %#v", plan.Parallel)
	}
	if len(plan.Exclusive) != 1 || plan.Exclusive[0].AcceptanceID != "A22" {
		t.Fatalf("exclusive plan = %#v", plan.Exclusive)
	}
}

func TestBuildBatchPlanFailsClosedForMissingRegistryEntry(t *testing.T) {
	manifest := batchManifest{}
	manifest.Baseline.ReleasePolicy = "required-no-skip"
	manifest.AcceptanceCases = append(manifest.AcceptanceCases, struct {
		AcceptanceID string `yaml:"acceptance_id"`
		EvidencePath string `yaml:"evidence_path"`
	}{"A103", "artifacts/acceptance/A103/"})
	if _, err := buildBatchPlan(t.TempDir(), manifest); err == nil || !strings.Contains(err.Error(), "no canonical runner") {
		t.Fatalf("missing registry entry error = %v", err)
	}
}

func TestRunParallelBatchHonorsLimitAndReportsAllFailures(t *testing.T) {
	runners := make([]acceptancecatalog.Runner, 8)
	for index := range runners {
		runners[index] = acceptancecatalog.Runner{AcceptanceID: fmt.Sprintf("A%02d", index+1)}
	}
	var mutex sync.Mutex
	running, maximum := 0, 0
	failures := runParallelBatch(runners, 3, func(runner acceptancecatalog.Runner) error {
		mutex.Lock()
		running++
		if running > maximum {
			maximum = running
		}
		mutex.Unlock()
		time.Sleep(5 * time.Millisecond)
		mutex.Lock()
		running--
		mutex.Unlock()
		if runner.AcceptanceID == "A02" || runner.AcceptanceID == "A07" {
			return fmt.Errorf("%s failed", runner.AcceptanceID)
		}
		return nil
	})
	if maximum > 3 || maximum < 2 {
		t.Fatalf("maximum concurrency = %d", maximum)
	}
	if len(failures) != 2 || !strings.Contains(strings.Join(failures, ","), "A02") || !strings.Contains(strings.Join(failures, ","), "A07") {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestValidateBatchRepositoryStateRejectsCommitAndWorktreeDrift(t *testing.T) {
	if err := validateBatchRepositoryState("abc", "abc", false); err != nil {
		t.Fatalf("stable repository rejected: %v", err)
	}
	if err := validateBatchRepositoryState("abc", "def", false); err == nil || !strings.Contains(err.Error(), "HEAD changed") {
		t.Fatalf("commit drift error = %v", err)
	}
	if err := validateBatchRepositoryState("abc", "abc", true); err == nil || !strings.Contains(err.Error(), "became dirty") {
		t.Fatalf("worktree drift error = %v", err)
	}
}

func writeBatchRunner(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# runner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
