package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedDataMaintenanceContractsContainFiveOperations(t *testing.T) {
	contracts := expectedDataMaintenanceContracts()
	if len(contracts) != 5 {
		t.Fatalf("maintenance contracts = %d, want 5", len(contracts))
	}
}

func TestDataMaintenanceCatalogDetectsMissingExtraAndTriggerDrift(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*dataMaintenanceFixture, *string)
		want   string
	}{
		{
			name: "missing operation",
			mutate: func(fixture *dataMaintenanceFixture, _ *string) {
				fixture.Operations = fixture.Operations[:len(fixture.Operations)-1]
			},
			want: "fixture is missing operation_id metadata_diagnostic_run_cleanup",
		},
		{
			name: "extra operation",
			mutate: func(fixture *dataMaintenanceFixture, _ *string) {
				fixture.Operations = append(fixture.Operations, dataMaintenanceContract{"invented_cleanup", "retention", "retention_scan"})
			},
			want: "fixture has unknown operation_id invented_cleanup",
		},
		{
			name: "design trigger drift",
			mutate: func(_ *dataMaintenanceFixture, markdown *string) {
				*markdown = strings.Replace(*markdown, "| `resource_daily_finalize` | `resource_maintenance` | `beijing_daily` |", "| `resource_daily_finalize` | `resource_maintenance` | `retention_scan` |", 1)
			},
			want: "design operation_id resource_daily_finalize trigger_class = \"retention_scan\", want \"beijing_daily\"",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			fixture := dataMaintenanceFixture{SchemaVersion: 2, FixtureID: "F13", Operations: expectedDataMaintenanceContracts()}
			fixture.GapRepair.BatchSize = 2
			fixture.GapRepair.CursorOrder = "resource_kind,site_id,node_name,bucket_start"
			fixture.GapRepair.CursorFields = []string{"cursor_kind", "cursor_site_id", "cursor_node_name", "cursor_bucket_start"}
			fixture.Clock.DuePhaseCases = []string{"02:59", "03:00", "03:19", "03:20", "04:00"}
			fixture.ResourceDaily.FinalizeRebuilds = []string{"site_status_daily", "site_instance_status_daily"}
			fixture.ResourceDaily.CoverageCases = []string{"node_retire_gap_reappear", "zero_sample_expected_hour", "config_or_lifecycle_revision_changed"}
			fixture.ResourceDaily.ScopeRevision = "sha256_canonical_site_config_pause_known_lifecycle"
			fixture.ResourceDaily.RevisionChangeAction = "reset_composite_cursor_and_rescan"
			fixture.ResourceDaily.PartialFailureKeepsAllUnfinalized = true
			fixture.ResourceDaily.PublishMode = "two_phase_atomic_all_day"
			fixture.ResourceDaily.RevisionVectors = append(fixture.ResourceDaily.RevisionVectors,
				struct {
					Name          string `json:"name"`
					CanonicalJSON string `json:"canonical_json"`
					SHA256        string `json:"sha256"`
				}{Name: "global", CanonicalJSON: "[]", SHA256: "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e8b66fb31fbfa1d4f1e4b5b"},
				struct {
					Name          string `json:"name"`
					CanonicalJSON string `json:"canonical_json"`
					SHA256        string `json:"sha256"`
				}{Name: "site", CanonicalJSON: "[1]", SHA256: "080a9ed428559ef602668b4c00f114f1a11c3f6b02a435f0bdc154578e4d7f22"})
			fixture.GapRepair.ResourceKindOrder = []string{"instance_hourly", "site_hourly"}
			fixture.GapRepair.NodeNameOrder = "utf8mb4_bin"
			fixture.GapRepair.Repairs = []string{"hourly_missing", "hourly_unfixed_sample_expected_status"}
			fixture.GapRepair.NeverRewrites = []string{"correct_complete_hourly", "daily", "final", "last_calculated_at_of_unchanged_bucket"}
			fixture.InstanceLifecycle.Table = "site_instance_lifecycle"
			fixture.InstanceLifecycle.EvidenceStatuses = []string{"known", "legacy_unknown"}
			fixture.InstanceLifecycle.ExpectedUses = []string{"known"}
			fixture.InstanceLifecycle.SingleOpen = true
			fixture.InstanceLifecycle.SameMinute = "merge_open"
			fixture.InstanceLifecycle.LegacyPolicy = "diagnostic_only"
			fixture.InstanceLifecycle.SiteDeletePolicy = "owned_metadata"
			fixture.Runtime.StartupLimit, fixture.Runtime.RegularLimit = 20, 100
			fixture.Runtime.ErrorsJoined, fixture.Runtime.NoStarvation = true, true
			fixture.Runtime.LifecycleCases = []string{"parent_cancel_during_recovery", "quiesce_during_recovery", "stop_during_recovery", "no_double_close", "no_ready_rebound"}
			fixture.Retention.MetadataCleanupNeverDeletes = []string{"failed", "manual", "windowed", "parent_range_empty_but_child_window_exists", "local_rebuild", "other_task_type"}
			markdown := testDataMaintenanceMarkdown(fixture.Operations)
			scenario.mutate(&fixture, &markdown)
			writeDataMaintenanceTestFiles(t, root, fixture, markdown)
			current := &checker{root: root}
			current.checkDataMaintenanceCatalog()
			if !issuesContain(current.issues, scenario.want) {
				t.Fatalf("issues=%#v, want %q", current.issues, scenario.want)
			}
		})
	}
}

func testDataMaintenanceMarkdown(operations []dataMaintenanceContract) string {
	var builder strings.Builder
	builder.WriteString(dataMaintenanceStart + "\n")
	builder.WriteString("| operation_id | category | trigger_class | function | idempotency | failure | result |\n")
	builder.WriteString("|---|---|---|---|---|---|---|\n")
	for _, item := range operations {
		builder.WriteString("| `" + item.OperationID + "` | `" + item.Category + "` | `" + item.TriggerClass + "` | function | idempotency | failure | result |\n")
	}
	builder.WriteString(dataMaintenanceEnd + "\n")
	return builder.String()
}

func writeDataMaintenanceTestFiles(t *testing.T, root string, fixture dataMaintenanceFixture, markdown string) {
	t.Helper()
	payload, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(root, filepath.FromSlash(dataMaintenanceFixturePath))
	designPath := filepath.Join(root, filepath.FromSlash(dataMaintenanceDesignPath))
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
