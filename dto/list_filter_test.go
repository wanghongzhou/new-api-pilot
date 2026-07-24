package dto

import (
	"reflect"
	"testing"
)

func TestAlertAndExportListQueriesNormalizeStableUniqueEnums(t *testing.T) {
	alerts := AlertListQuery{
		Page: 1, PageSize: 20,
		Statuses:    []string{" FIRING ", "pending", "firing"},
		Levels:      []string{"WARNING", "critical", "warning"},
		TargetTypes: []string{"site", "ACCOUNT", "site"},
	}
	alerts.Normalize()
	if alerts.Validate() != nil ||
		!reflect.DeepEqual(alerts.Statuses, []string{"firing", "pending"}) ||
		!reflect.DeepEqual(alerts.Levels, []string{"warning", "critical"}) ||
		!reflect.DeepEqual(alerts.TargetTypes, []string{"site", "account"}) {
		t.Fatalf("normalized alert filters = %#v errors=%#v", alerts, alerts.Validate())
	}

	exports := ExportListQuery{
		Page: 1, PageSize: 20,
		Statuses: []string{" SUCCESS ", "failed", "success"},
	}
	exports.Normalize()
	if exports.Validate() != nil || !reflect.DeepEqual(exports.Statuses, []string{"success", "failed"}) {
		t.Fatalf("normalized export filters = %#v errors=%#v", exports, exports.Validate())
	}
}

func TestAlertAndExportListQueriesRejectInvalidEnums(t *testing.T) {
	alerts := AlertListQuery{Page: 1, PageSize: 20, Statuses: []string{"firing", "unknown"}}
	alerts.Normalize()
	if fields := alerts.Validate(); fields == nil || fields["status"] == "" {
		t.Fatalf("invalid alert status fields = %#v", fields)
	}
	exports := ExportListQuery{Page: 1, PageSize: 20, Statuses: []string{"pending", "unknown"}}
	exports.Normalize()
	if fields := exports.Validate(); fields == nil || fields["status"] == "" {
		t.Fatalf("invalid export status fields = %#v", fields)
	}
}

func TestAlertListQueryAcceptsOnlyDocumentedBusinessSorts(t *testing.T) {
	for _, sortBy := range []string{"rule_key", "status", "level", "site_name", "first_fired_at", "last_fired_at", "resolved_at"} {
		query := AlertListQuery{Page: 1, PageSize: 20, SortBy: sortBy, SortOrder: "asc"}
		if fields := query.Validate(); fields != nil {
			t.Errorf("documented sort %s rejected: %#v", sortBy, fields)
		}
	}
	for _, sortBy := range []string{"current_value", "target_name", "updated_at"} {
		query := AlertListQuery{Page: 1, PageSize: 20, SortBy: sortBy, SortOrder: "asc"}
		if fields := query.Validate(); fields == nil || fields["sort_by"] == "" {
			t.Errorf("invalid sort %s accepted: %#v", sortBy, fields)
		}
	}
}

func TestAlertRuleListQueryNormalizesAndValidatesContract(t *testing.T) {
	enabled, inherited := true, false
	query := AlertRuleListQuery{
		ScopeType: " SITE ", ScopeID: 42, Page: 1, PageSize: 20,
		Categories: []string{" INSTANCE ", "channel", "instance"},
		Levels:     []string{"WARNING", "critical", "warning"},
		Enabled:    &enabled, Inherited: &inherited, SortBy: " METRIC ", SortOrder: " DESC ",
	}
	query.Normalize()
	if fields := query.Validate(); fields != nil ||
		!reflect.DeepEqual(query.Categories, []string{"instance", "channel"}) ||
		!reflect.DeepEqual(query.Levels, []string{"warning", "critical"}) ||
		query.SortBy != "metric" || query.SortOrder != "desc" {
		t.Fatalf("normalized alert rule query = %#v errors=%#v", query, fields)
	}

	query.SortBy = "for_times"
	if fields := query.Validate(); fields == nil || fields["sort_by"] == "" {
		t.Fatalf("invalid alert rule sort fields = %#v", fields)
	}
}
