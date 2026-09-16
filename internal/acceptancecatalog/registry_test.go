package acceptancecatalog

import (
	"fmt"
	"testing"
)

func TestRegistryCoversCompleteAcceptanceRange(t *testing.T) {
	runners := All()
	if len(runners) != 102 {
		t.Fatalf("registry size = %d, want 102", len(runners))
	}
	for number := 1; number <= 102; number++ {
		want := fmt.Sprintf("A%02d", number)
		runner, ok := Lookup(want)
		if !ok {
			t.Fatalf("runner %s is missing", want)
		}
		if _, err := RunnerPath(runner); err != nil {
			t.Fatalf("RunnerPath(%s): %v", want, err)
		}
	}
}

func TestValidateCanonicalCommandRejectsArbitrarySuccess(t *testing.T) {
	runner, ok := Lookup("A01")
	if !ok {
		t.Fatal("A01 runner is missing")
	}
	if err := ValidateCanonicalCommand("A01", runner.Command); err != nil {
		t.Fatalf("canonical command rejected: %v", err)
	}
	if err := ValidateCanonicalCommand("A01", []string{"powershell.exe", "-Command", "exit 0"}); err == nil {
		t.Fatal("arbitrary success command was accepted")
	}
	if err := ValidateCanonicalCommand("A103", runner.Command); err == nil {
		t.Fatal("unregistered case was accepted")
	}
}

func TestRegistrySeparatesExclusiveResources(t *testing.T) {
	for _, id := range []string{"A05", "A22", "A50", "A89", "A101"} {
		runner, _ := Lookup(id)
		if runner.Resource != ResourceExclusive {
			t.Fatalf("%s resource = %q, want exclusive", id, runner.Resource)
		}
	}
	for _, id := range []string{"A01", "A35", "A70", "A83", "A102"} {
		runner, _ := Lookup(id)
		if runner.Resource != ResourceGoParallel {
			t.Fatalf("%s resource = %q, want go-parallel", id, runner.Resource)
		}
	}
}

func TestEvidenceContractClassification(t *testing.T) {
	for _, id := range []string{"A01", "A83", "A85", "A102"} {
		if !UsesGenericEvidenceContract(id) {
			t.Fatalf("%s should use the generic evidence contract", id)
		}
	}
	for _, id := range []string{"A22", "A51", "A52", "A75"} {
		if UsesGenericEvidenceContract(id) {
			t.Fatalf("%s should use a closed evidence contract", id)
		}
	}
}
