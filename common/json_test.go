package common

import (
	"errors"
	"strings"
	"testing"
)

type strictJSONFixture struct {
	Name string `json:"name"`
}

func TestDecodeJSONRejectsUnknownDuplicateAndTrailingFields(t *testing.T) {
	tests := []string{
		`{"name":"first","unknown":true}`,
		`{"name":"first","name":"second"}`,
		`{"name":"first"} {"name":"second"}`,
	}
	for _, input := range tests {
		var target strictJSONFixture
		if err := DecodeJSON(strings.NewReader(input), &target, 1024); err == nil {
			t.Fatalf("DecodeJSON() accepted %s", input)
		}
	}
}

func TestDecodeJSONEnforcesSize(t *testing.T) {
	var target strictJSONFixture
	err := DecodeJSON(strings.NewReader(`{"name":"long"}`), &target, 4)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
}

func TestDecodeJSONAcceptsOneStrictObject(t *testing.T) {
	var target strictJSONFixture
	if err := DecodeJSON(strings.NewReader(`{"name":"pilot"}`), &target, 1024); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
	if target.Name != "pilot" {
		t.Fatalf("decoded name = %q", target.Name)
	}
}
