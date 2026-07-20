package docscheck

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dataMaintenanceDesignPath  = "docs/多站点运营管理平台-详细设计-05C-平台API与Worker.md"
	dataMaintenanceFixturePath = "testdata/design/f13-data-maintenance.json"
	dataMaintenanceStart       = "<!-- DATA_MAINTENANCE_CATALOG_START -->"
	dataMaintenanceEnd         = "<!-- DATA_MAINTENANCE_CATALOG_END -->"
)

type dataMaintenanceContract struct {
	OperationID  string `json:"operation_id"`
	Category     string `json:"category"`
	TriggerClass string `json:"trigger_class"`
}

type dataMaintenanceFixture struct {
	SchemaVersion int                       `json:"schema_version"`
	FixtureID     string                    `json:"fixture_id"`
	Operations    []dataMaintenanceContract `json:"operations"`
	Clock         struct {
		DuePhaseCases []string `json:"due_phase_cases"`
	} `json:"clock"`
	ResourceDaily struct {
		FinalizeRebuilds                  []string `json:"finalize_rebuilds"`
		CoverageCases                     []string `json:"coverage_cases"`
		ScopeRevision                     string   `json:"scope_revision"`
		RevisionChangeAction              string   `json:"revision_change_action"`
		PartialFailureKeepsAllUnfinalized bool     `json:"partial_failure_keeps_all_daily_unfinalized"`
		PublishMode                       string   `json:"publish_mode"`
		RevisionVectors                   []struct {
			Name          string `json:"name"`
			CanonicalJSON string `json:"canonical_json"`
			SHA256        string `json:"sha256"`
		} `json:"revision_vectors"`
	} `json:"resource_daily"`
	GapRepair struct {
		BatchSize         int      `json:"batch_size"`
		CursorOrder       string   `json:"cursor_order"`
		CursorFields      []string `json:"cursor_fields"`
		NeverRewrites     []string `json:"never_rewrites"`
		ResourceKindOrder []string `json:"resource_kind_order"`
		Repairs           []string `json:"repairs"`
		SiteNodeSentinel  string   `json:"site_node_name_sentinel"`
		NodeNameOrder     string   `json:"node_name_order"`
	} `json:"gap_repair"`
	InstanceLifecycle struct {
		Table            string   `json:"table"`
		EvidenceStatuses []string `json:"evidence_statuses"`
		ExpectedUses     []string `json:"expected_uses"`
		SingleOpen       bool     `json:"single_open_interval"`
		SameMinute       string   `json:"same_minute_retire_reappear"`
		LegacyPolicy     string   `json:"legacy_migration_policy"`
		SiteDeletePolicy string   `json:"site_delete_policy"`
	} `json:"instance_lifecycle"`
	Runtime struct {
		StartupLimit   int      `json:"startup_authorization_limit"`
		RegularLimit   int      `json:"regular_authorization_limit"`
		ErrorsJoined   bool     `json:"operation_errors_are_joined"`
		NoStarvation   bool     `json:"failure_does_not_starve_other_due_operations"`
		LifecycleCases []string `json:"lifecycle_cases"`
	} `json:"runtime"`
	Retention struct {
		MetadataCleanupNeverDeletes []string `json:"metadata_cleanup_never_deletes"`
	} `json:"retention"`
}

func expectedDataMaintenanceContracts() []dataMaintenanceContract {
	return []dataMaintenanceContract{
		{"authorize_pricing_group_sync", "authorization_trigger", "authorize_post_commit"},
		{"resource_daily_finalize", "resource_maintenance", "beijing_daily"},
		{"resource_rollup_gap_repair", "resource_maintenance", "beijing_daily"},
		{"collection_run_error_redaction", "retention", "retention_scan"},
		{"metadata_diagnostic_run_cleanup", "retention", "retention_scan"},
	}
}

func (current *checker) checkDataMaintenanceCatalog() {
	expected := expectedDataMaintenanceContracts()
	fixturePath := filepath.Join(current.root, filepath.FromSlash(dataMaintenanceFixturePath))
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		current.add("data-maintenance-catalog", fixturePath, "open: %v", err)
		return
	}
	var fixture dataMaintenanceFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		current.add("data-maintenance-catalog", fixturePath, "decode JSON: %v", err)
		return
	}
	if fixture.SchemaVersion != 2 {
		current.add("data-maintenance-catalog", fixturePath, "schema_version = %d, want 2", fixture.SchemaVersion)
	}
	if fixture.FixtureID != "F13" {
		current.add("data-maintenance-catalog", fixturePath, "fixture_id = %q, want F13", fixture.FixtureID)
	}
	current.compareDataMaintenanceContracts(fixturePath, "fixture", expected, fixture.Operations)
	if !equalStrings(fixture.Clock.DuePhaseCases, []string{"02:59", "03:00", "03:19", "03:20", "04:00"}) {
		current.add("data-maintenance-catalog", fixturePath, "maintenance due phase contract drifted")
	}
	if !equalStrings(fixture.ResourceDaily.FinalizeRebuilds, []string{"site_status_daily", "site_instance_status_daily"}) || fixture.ResourceDaily.ScopeRevision != "sha256_canonical_site_config_pause_known_lifecycle" || fixture.ResourceDaily.RevisionChangeAction != "reset_composite_cursor_and_rescan" || !fixture.ResourceDaily.PartialFailureKeepsAllUnfinalized || fixture.ResourceDaily.PublishMode != "two_phase_atomic_all_day" || !containsAll(fixture.ResourceDaily.CoverageCases, []string{"node_retire_gap_reappear", "zero_sample_expected_hour", "config_or_lifecycle_revision_changed"}) {
		current.add("data-maintenance-catalog", fixturePath, "resource daily lifecycle/revision contract drifted")
	}
	if len(fixture.ResourceDaily.RevisionVectors) != 2 {
		current.add("data-maintenance-catalog", fixturePath, "scope revision vectors = %d, want 2", len(fixture.ResourceDaily.RevisionVectors))
	} else {
		for _, vector := range fixture.ResourceDaily.RevisionVectors {
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(vector.CanonicalJSON)))
			if vector.Name == "" || vector.SHA256 != digest {
				current.add("data-maintenance-catalog", fixturePath, "scope revision vector %q checksum drifted", vector.Name)
			}
		}
	}
	if fixture.GapRepair.BatchSize != 2 || fixture.GapRepair.CursorOrder != "resource_kind,site_id,node_name,bucket_start" ||
		!equalStrings(fixture.GapRepair.CursorFields, []string{"cursor_kind", "cursor_site_id", "cursor_node_name", "cursor_bucket_start"}) {
		current.add("data-maintenance-catalog", fixturePath, "gap repair batch/cursor contract drifted")
	}
	if !equalStrings(fixture.GapRepair.ResourceKindOrder, []string{"instance_hourly", "site_hourly"}) || fixture.GapRepair.SiteNodeSentinel != "" || fixture.GapRepair.NodeNameOrder != "utf8mb4_bin" || !equalStrings(fixture.GapRepair.Repairs, []string{"hourly_missing", "hourly_unfixed_sample_expected_status"}) || !containsAll(fixture.GapRepair.NeverRewrites, []string{"correct_complete_hourly", "daily", "final", "last_calculated_at_of_unchanged_bucket"}) {
		current.add("data-maintenance-catalog", fixturePath, "gap repair kind/no-rewrite contract drifted")
	}
	if fixture.InstanceLifecycle.Table != "site_instance_lifecycle" ||
		!equalStrings(fixture.InstanceLifecycle.EvidenceStatuses, []string{"known", "legacy_unknown"}) ||
		!equalStrings(fixture.InstanceLifecycle.ExpectedUses, []string{"known"}) || !fixture.InstanceLifecycle.SingleOpen ||
		fixture.InstanceLifecycle.SameMinute != "merge_open" || fixture.InstanceLifecycle.LegacyPolicy == "" || fixture.InstanceLifecycle.SiteDeletePolicy != "owned_metadata" {
		current.add("data-maintenance-catalog", fixturePath, "instance lifecycle evidence contract drifted")
	}
	if fixture.Runtime.StartupLimit != 20 || fixture.Runtime.RegularLimit != 100 || !fixture.Runtime.ErrorsJoined || !fixture.Runtime.NoStarvation {
		current.add("data-maintenance-catalog", fixturePath, "maintenance runtime recovery contract drifted")
	}
	if !containsAll(fixture.Runtime.LifecycleCases, []string{"parent_cancel_during_recovery", "quiesce_during_recovery", "stop_during_recovery", "no_double_close", "no_ready_rebound"}) {
		current.add("data-maintenance-catalog", fixturePath, "maintenance runtime lifecycle contract drifted")
	}
	if !containsAll(fixture.Retention.MetadataCleanupNeverDeletes, []string{"failed", "manual", "windowed", "parent_range_empty_but_child_window_exists", "local_rebuild", "other_task_type"}) {
		current.add("data-maintenance-catalog", fixturePath, "metadata cleanup exclusion contract drifted")
	}

	designPath := filepath.Join(current.root, filepath.FromSlash(dataMaintenanceDesignPath))
	designPayload, err := os.ReadFile(designPath)
	if err != nil {
		current.add("data-maintenance-catalog", designPath, "open: %v", err)
		return
	}
	designOperations, err := parseDataMaintenanceMarkdown(string(designPayload))
	if err != nil {
		current.add("data-maintenance-catalog", designPath, "%v", err)
		return
	}
	current.compareDataMaintenanceContracts(designPath, "design", expected, designOperations)
	current.compareDataMaintenanceContracts(designPath, "design versus fixture", fixture.Operations, designOperations)
}

func equalStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func containsAll(actual, expected []string) bool {
	values := make(map[string]struct{}, len(actual))
	for _, item := range actual {
		values[item] = struct{}{}
	}
	for _, item := range expected {
		if _, exists := values[item]; !exists {
			return false
		}
	}
	return true
}

func (current *checker) compareDataMaintenanceContracts(path, source string, expected, actual []dataMaintenanceContract) {
	want := make(map[string]dataMaintenanceContract, len(expected))
	for _, item := range expected {
		want[item.OperationID] = item
	}
	got := make(map[string]dataMaintenanceContract, len(actual))
	for _, item := range actual {
		if strings.TrimSpace(item.OperationID) != item.OperationID || item.OperationID == "" {
			current.add("data-maintenance-catalog", path, "%s has invalid operation_id %q", source, item.OperationID)
			continue
		}
		if _, duplicate := got[item.OperationID]; duplicate {
			current.add("data-maintenance-catalog", path, "%s repeats operation_id %s", source, item.OperationID)
			continue
		}
		got[item.OperationID] = item
	}
	for operationID, contract := range want {
		item, exists := got[operationID]
		if !exists {
			current.add("data-maintenance-catalog", path, "%s is missing operation_id %s", source, operationID)
			continue
		}
		if item.Category != contract.Category {
			current.add("data-maintenance-catalog", path, "%s operation_id %s category = %q, want %q", source, operationID, item.Category, contract.Category)
		}
		if item.TriggerClass != contract.TriggerClass {
			current.add("data-maintenance-catalog", path, "%s operation_id %s trigger_class = %q, want %q", source, operationID, item.TriggerClass, contract.TriggerClass)
		}
	}
	for operationID := range got {
		if _, exists := want[operationID]; !exists {
			current.add("data-maintenance-catalog", path, "%s has unknown operation_id %s", source, operationID)
		}
	}
}

func parseDataMaintenanceMarkdown(payload string) ([]dataMaintenanceContract, error) {
	start := strings.Index(payload, dataMaintenanceStart)
	end := strings.Index(payload, dataMaintenanceEnd)
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("data maintenance catalog markers are missing or out of order")
	}
	rows := make([]dataMaintenanceContract, 0, 5)
	for _, raw := range strings.Split(payload[start+len(dataMaintenanceStart):end], "\n") {
		columns := splitMarkdownRow(raw)
		if len(columns) == 0 || columns[0] == "operation_id" || strings.HasPrefix(columns[0], "---") {
			continue
		}
		if len(columns) != 7 {
			return nil, fmt.Errorf("data maintenance catalog row has %d columns, want 7: %s", len(columns), strings.TrimSpace(raw))
		}
		rows = append(rows, dataMaintenanceContract{
			OperationID:  strings.Trim(columns[0], "` "),
			Category:     strings.Trim(columns[1], "` "),
			TriggerClass: strings.Trim(columns[2], "` "),
		})
	}
	return rows, nil
}
