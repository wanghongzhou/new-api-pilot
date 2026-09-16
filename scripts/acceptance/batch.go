package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"new-api-pilot/internal/acceptancecatalog"

	"gopkg.in/yaml.v3"
)

type batchManifest struct {
	Baseline struct {
		ReleasePolicy string `yaml:"release_policy"`
	} `yaml:"baseline"`
	AcceptanceCases []struct {
		AcceptanceID string `yaml:"acceptance_id"`
		EvidencePath string `yaml:"evidence_path"`
	} `yaml:"acceptance_cases"`
}

type batchPlan struct {
	Parallel  []acceptancecatalog.Runner
	Exclusive []acceptancecatalog.Runner
}

func runBatch(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("batch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	parallelism := flags.Int("parallel", 4, "maximum concurrent ordinary Go acceptance runs")
	dryRun := flags.Bool("dry-run", false, "validate and print the execution plan without running it")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *parallelism < 1 || *parallelism > 16 {
		fmt.Fprintln(stderr, "batch accepts no positional arguments and requires -parallel between 1 and 16")
		return 2
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(stderr, "resolve repository root: %v\n", err)
		return 2
	}
	commit, dirty := gitState(absoluteRoot)
	if commit == "unborn" || dirty {
		fmt.Fprintln(stderr, "batch acceptance requires a clean repository HEAD")
		return 2
	}
	manifestPath := filepath.Join(absoluteRoot, "docs", "acceptance", "manifest.yaml")
	file, err := os.Open(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "open acceptance manifest: %v\n", err)
		return 2
	}
	manifest, err := decodeBatchManifest(file)
	_ = file.Close()
	if err != nil {
		fmt.Fprintf(stderr, "decode acceptance manifest: %v\n", err)
		return 2
	}
	plan, err := buildBatchPlan(absoluteRoot, manifest)
	if err != nil {
		fmt.Fprintf(stderr, "build acceptance plan: %v\n", err)
		return 2
	}
	printBatchPlan(stdout, commit, plan, *parallelism)
	if *dryRun {
		return 0
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve acceptance executable: %v\n", err)
		return 2
	}
	runOne := func(runner acceptancecatalog.Runner) error {
		currentCommit, currentDirty := gitState(absoluteRoot)
		if err := validateBatchRepositoryState(commit, currentCommit, currentDirty); err != nil {
			return fmt.Errorf("%s preflight: %w", runner.AcceptanceID, err)
		}
		childArguments := []string{"run", "-case", runner.AcceptanceID, "-root", absoluteRoot, "--"}
		childArguments = append(childArguments, runner.Command...)
		command := exec.Command(executable, childArguments...)
		command.Dir = absoluteRoot
		command.Stdout = stdout
		command.Stderr = stderr
		runErr := command.Run()
		currentCommit, currentDirty = gitState(absoluteRoot)
		if err := validateBatchRepositoryState(commit, currentCommit, currentDirty); err != nil {
			return fmt.Errorf("%s postflight: %w", runner.AcceptanceID, err)
		}
		if runErr != nil {
			return fmt.Errorf("%s: %w", runner.AcceptanceID, runErr)
		}
		return nil
	}
	if failures := runParallelBatch(plan.Parallel, *parallelism, runOne); len(failures) > 0 {
		fmt.Fprintf(stderr, "parallel acceptance group failed: %s\n", strings.Join(failures, "; "))
		return 1
	}
	for _, runner := range plan.Exclusive {
		if err := runOne(runner); err != nil {
			fmt.Fprintf(stderr, "exclusive acceptance group failed: %v\n", err)
			return 1
		}
	}
	return 0
}

func validateBatchRepositoryState(expectedCommit, currentCommit string, dirty bool) error {
	if expectedCommit == "" || expectedCommit == "unborn" || currentCommit != expectedCommit {
		return fmt.Errorf("repository HEAD changed during acceptance batch")
	}
	if dirty {
		return fmt.Errorf("repository worktree became dirty during acceptance batch")
	}
	return nil
}

func decodeBatchManifest(reader io.Reader) (batchManifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(false)
	var manifest batchManifest
	if err := decoder.Decode(&manifest); err != nil {
		return batchManifest{}, err
	}
	if manifest.Baseline.ReleasePolicy != "required-no-skip" {
		return batchManifest{}, fmt.Errorf("release_policy must be required-no-skip")
	}
	return manifest, nil
}

func buildBatchPlan(root string, manifest batchManifest) (batchPlan, error) {
	plan := batchPlan{}
	seen := make(map[string]struct{}, len(manifest.AcceptanceCases))
	for _, acceptance := range manifest.AcceptanceCases {
		if _, duplicate := seen[acceptance.AcceptanceID]; duplicate {
			return batchPlan{}, fmt.Errorf("duplicate acceptance case %s", acceptance.AcceptanceID)
		}
		seen[acceptance.AcceptanceID] = struct{}{}
		if strings.HasPrefix(acceptance.EvidencePath, "planned:") {
			continue
		}
		runner, ok := acceptancecatalog.Lookup(acceptance.AcceptanceID)
		if !ok {
			return batchPlan{}, fmt.Errorf("required case %s has no canonical runner", acceptance.AcceptanceID)
		}
		runnerPath, err := acceptancecatalog.RunnerPath(runner)
		if err != nil {
			return batchPlan{}, err
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(runnerPath)))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return batchPlan{}, fmt.Errorf("%s canonical runner is missing or not a regular file: %s", acceptance.AcceptanceID, runnerPath)
		}
		switch runner.Resource {
		case acceptancecatalog.ResourceGoParallel:
			plan.Parallel = append(plan.Parallel, runner)
		case acceptancecatalog.ResourceExclusive:
			plan.Exclusive = append(plan.Exclusive, runner)
		default:
			return batchPlan{}, fmt.Errorf("%s has unknown resource class %q", acceptance.AcceptanceID, runner.Resource)
		}
	}
	order := func(values []acceptancecatalog.Runner) {
		sort.Slice(values, func(i, j int) bool { return values[i].AcceptanceID < values[j].AcceptanceID })
	}
	order(plan.Parallel)
	order(plan.Exclusive)
	return plan, nil
}

func printBatchPlan(writer io.Writer, commit string, plan batchPlan, parallelism int) {
	fmt.Fprintf(writer, "acceptance batch commit=%s parallel=%d ordinary=%d exclusive=%d\n", commit, parallelism, len(plan.Parallel), len(plan.Exclusive))
	for _, runner := range append(append([]acceptancecatalog.Runner{}, plan.Parallel...), plan.Exclusive...) {
		fmt.Fprintf(writer, "%s %s %s\n", runner.AcceptanceID, runner.Resource, strings.Join(runner.Command, " "))
	}
}

func runParallelBatch(runners []acceptancecatalog.Runner, parallelism int, run func(acceptancecatalog.Runner) error) []string {
	jobs := make(chan acceptancecatalog.Runner)
	failures := make(chan string, len(runners))
	var workers sync.WaitGroup
	for index := 0; index < parallelism; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for runner := range jobs {
				if err := run(runner); err != nil {
					failures <- err.Error()
				}
			}
		}()
	}
	for _, runner := range runners {
		jobs <- runner
	}
	close(jobs)
	workers.Wait()
	close(failures)
	result := make([]string, 0, len(failures))
	for failure := range failures {
		result = append(result, failure)
	}
	sort.Strings(result)
	return result
}
