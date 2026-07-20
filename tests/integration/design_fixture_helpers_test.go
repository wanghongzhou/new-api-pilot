package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

type designClockFixture struct {
	NowUnix  int64  `json:"now_unix"`
	Timezone string `json:"timezone"`
}

func loadDesignJSONFixture[T any](t *testing.T, name string) T {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "design", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read design fixture %s: %v", path, err)
	}
	var fixture T
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("parse design fixture %s: %v", path, err)
	}
	return fixture
}

func requireDesignScenarios(t *testing.T, actual []string, expected ...string) {
	t.Helper()
	for _, scenario := range expected {
		if !slices.Contains(actual, scenario) {
			t.Fatalf("design fixture is missing scenario %q: %v", scenario, actual)
		}
	}
}

func fixtureInt64(t *testing.T, field, value string) int64 {
	t.Helper()
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("parse fixture %s=%q: %v", field, value, err)
	}
	return parsed
}
