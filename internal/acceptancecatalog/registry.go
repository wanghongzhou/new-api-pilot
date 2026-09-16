package acceptancecatalog

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type ResourceClass string

const (
	ResourceGoParallel ResourceClass = "go-parallel"
	ResourceExclusive  ResourceClass = "exclusive"
)

type Runner struct {
	AcceptanceID string
	Command      []string
	Resource     ResourceClass
}

var powershellPrefix = []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File"}

var specializedRunnerPaths = map[string]string{
	"A22": "scripts/acceptance/run-a22.ps1",
	"A25": "scripts/acceptance/run-a25.ps1",
	"A45": "scripts/acceptance/run-a45.ps1",
	"A49": "scripts/acceptance/run-a49.ps1",
	"A50": "scripts/acceptance/run-a50.ps1",
	"A51": "scripts/acceptance/run-a51.ps1",
	"A52": "scripts/acceptance/run-a52.ps1",
	"A62": "scripts/acceptance/run-a62.ps1",
	"A74": "scripts/acceptance/run-a74.ps1",
	"A75": "scripts/acceptance/run-a75.ps1",
	"A85": "scripts/acceptance/run-a85.ps1",
}

var closedEvidenceCases = stringSet("A22", "A25", "A45", "A49", "A50", "A51", "A52", "A62", "A74", "A75")

// exclusiveCases either own a dedicated Docker environment, drive a browser,
// or combine backend and browser acceptance. They must never overlap another
// acceptance run on the shared workstation.
var exclusiveCases = stringSet(
	"A05", "A22", "A25", "A45", "A47", "A49", "A50", "A51", "A52", "A55",
	"A62", "A66", "A67", "A71", "A72", "A73", "A74", "A75", "A77", "A85",
	"A88", "A89", "A90", "A91", "A92", "A93", "A94", "A95", "A96", "A97",
	"A98", "A99", "A100", "A101",
)

var registry = buildRegistry()

func buildRegistry() map[string]Runner {
	result := make(map[string]Runner, 102)
	for number := 1; number <= 102; number++ {
		id := fmt.Sprintf("A%02d", number)
		path := "scripts/acceptance/run-generic-case.ps1"
		if specialized, ok := specializedRunnerPaths[id]; ok {
			path = specialized
		}
		resource := ResourceGoParallel
		if _, exclusive := exclusiveCases[id]; exclusive {
			resource = ResourceExclusive
		}
		command := append(append([]string{}, powershellPrefix...), path)
		result[id] = Runner{AcceptanceID: id, Command: command, Resource: resource}
	}
	return result
}

func Lookup(acceptanceID string) (Runner, bool) {
	runner, ok := registry[acceptanceID]
	if !ok {
		return Runner{}, false
	}
	runner.Command = append([]string(nil), runner.Command...)
	return runner, true
}

func All() []Runner {
	result := make([]Runner, 0, len(registry))
	for _, runner := range registry {
		runner.Command = append([]string(nil), runner.Command...)
		result = append(result, runner)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.TrimPrefix(result[i].AcceptanceID, "A"))
		right, _ := strconv.Atoi(strings.TrimPrefix(result[j].AcceptanceID, "A"))
		return left < right
	})
	return result
}

func ValidateCanonicalCommand(acceptanceID string, command []string) error {
	runner, ok := Lookup(acceptanceID)
	if !ok {
		return fmt.Errorf("acceptance case %s has no canonical runner", acceptanceID)
	}
	if !reflect.DeepEqual(command, runner.Command) {
		return fmt.Errorf("command is not the registered canonical runner for %s", acceptanceID)
	}
	return nil
}

func UsesGenericEvidenceContract(acceptanceID string) bool {
	if _, ok := registry[acceptanceID]; !ok {
		return false
	}
	_, closed := closedEvidenceCases[acceptanceID]
	return !closed
}

func RunnerPath(runner Runner) (string, error) {
	if len(runner.Command) != len(powershellPrefix)+1 ||
		!reflect.DeepEqual(runner.Command[:len(powershellPrefix)], powershellPrefix) {
		return "", fmt.Errorf("%s canonical command is not a PowerShell file runner", runner.AcceptanceID)
	}
	value := runner.Command[len(runner.Command)-1]
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return "", fmt.Errorf("%s canonical runner path is not repository-relative", runner.AcceptanceID)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned != value || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%s canonical runner path is not normalized", runner.AcceptanceID)
	}
	return value, nil
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
