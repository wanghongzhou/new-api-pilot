package main

import "testing"

func TestClassifyGenericEvidence(t *testing.T) {
	tests := []struct {
		root  string
		want  string
		valid bool
	}{
		{root: "artifacts/acceptance", want: "formal", valid: true},
		{root: "./artifacts/smoke", want: "development", valid: true},
		{root: "artifacts/custom", valid: false},
	}
	for _, test := range tests {
		t.Run(test.root, func(t *testing.T) {
			got, err := classifyGenericEvidence(test.root)
			if (err == nil) != test.valid || got != test.want {
				t.Fatalf("classifyGenericEvidence(%q) = %q, %v; want %q valid=%t", test.root, got, err, test.want, test.valid)
			}
		})
	}
}
