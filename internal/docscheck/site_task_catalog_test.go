package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedSiteTaskContractsContainNineteenConstants(t *testing.T) {
	contracts := expectedSiteTaskContracts()
	if len(contracts) != 19 {
		t.Fatalf("site task contracts = %d, want 19", len(contracts))
	}
	seen := make(map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		if _, duplicate := seen[contract.TaskType]; duplicate {
			t.Fatalf("duplicate task_type %s", contract.TaskType)
		}
		seen[contract.TaskType] = struct{}{}
	}
}

func TestSiteTaskCatalogDetectsMissingExtraAndTriggerDrift(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*siteTaskCatalogFixture, *string)
		want   string
	}{
		{
			name: "missing fixture task",
			mutate: func(fixture *siteTaskCatalogFixture, _ *string) {
				fixture.Tasks = fixture.Tasks[:len(fixture.Tasks)-1]
			},
			want: "fixture is missing task_type customer_rebuild",
		},
		{
			name: "extra fixture task",
			mutate: func(fixture *siteTaskCatalogFixture, _ *string) {
				fixture.Tasks = append(fixture.Tasks, siteTaskContract{TaskType: "invented_sync", Category: "durable", TriggerClass: "resource_interval", PurposeKey: "siteTasks.purpose.inventedSync"})
			},
			want: "fixture has unknown task_type invented_sync",
		},
		{
			name: "design trigger drift",
			mutate: func(_ *siteTaskCatalogFixture, markdown *string) {
				*markdown = strings.Replace(*markdown, "| `log_sync` | `hourly` | `hour_boundary` |", "| `log_sync` | `hourly` | `fast_interval` |", 1)
			},
			want: "design task_type log_sync trigger_class = \"fast_interval\", want \"hour_boundary\"",
		},
		{
			name: "fixture purpose drift",
			mutate: func(fixture *siteTaskCatalogFixture, _ *string) {
				fixture.Tasks[0].PurposeKey = "siteTasks.purpose.wrong"
			},
			want: "fixture task_type site_probe purpose_key = \"siteTasks.purpose.wrong\", want \"siteTasks.purpose.siteProbe\"",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			fixture := siteTaskCatalogFixture{SchemaVersion: 1, FixtureID: "F12", Tasks: expectedSiteTaskContracts()}
			markdown := testSiteTaskCatalogMarkdown(fixture.Tasks)
			scenario.mutate(&fixture, &markdown)
			writeSiteTaskCatalogTestFiles(t, root, fixture, markdown)
			current := &checker{root: root}
			current.checkSiteTaskCatalog()
			if !issuesContain(current.issues, scenario.want) {
				t.Fatalf("issues=%#v, want %q", current.issues, scenario.want)
			}
		})
	}
}

func testSiteTaskCatalogMarkdown(tasks []siteTaskContract) string {
	var builder strings.Builder
	builder.WriteString(siteTaskCatalogStart + "\n")
	builder.WriteString("| task_type | category | trigger_class | purpose_key | data | trigger | eligibility | offline/export | history |\n")
	builder.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, item := range tasks {
		builder.WriteString("| `" + item.TaskType + "` | `" + item.Category + "` | `" + item.TriggerClass + "` | `" + item.PurposeKey + "` | data | trigger | eligibility | boundary | history |\n")
	}
	builder.WriteString(siteTaskCatalogEnd + "\n")
	return builder.String()
}

func writeSiteTaskCatalogTestFiles(t *testing.T, root string, fixture siteTaskCatalogFixture, markdown string) {
	t.Helper()
	payload, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(root, filepath.FromSlash(siteTaskCatalogFixturePath))
	designPath := filepath.Join(root, filepath.FromSlash(siteTaskCatalogDesignPath))
	for _, path := range []string{fixturePath, designPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(fixturePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(designPath, []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
}
