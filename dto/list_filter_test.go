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
