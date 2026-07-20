package integration_test

import (
	"testing"

	"new-api-pilot/constant"
)

func TestA101SiteTaskCatalogMatchesBackendTaskContract(t *testing.T) {
	type fixtureTask struct {
		TaskType     string `json:"task_type"`
		Category     string `json:"category"`
		TriggerClass string `json:"trigger_class"`
		PurposeKey   string `json:"purpose_key"`
	}
	type fixture struct {
		SchemaVersion int           `json:"schema_version"`
		FixtureID     string        `json:"fixture_id"`
		Tasks         []fixtureTask `json:"tasks"`
	}

	data := loadDesignJSONFixture[fixture](t, "f12-site-task-catalog.json")
	if data.SchemaVersion != 1 || data.FixtureID != "F12" {
		t.Fatalf("invalid F12 fixture identity: %#v", data)
	}
	want := map[string]struct {
		category string
		trigger  string
	}{
		constant.TaskTypeSiteProbe:        {"fast", "fast_interval"},
		constant.TaskTypeRealtimeStat:     {"fast", "fast_interval"},
		constant.TaskTypeResourceSnapshot: {"fast", "fast_interval"},
		constant.TaskTypePerformanceSync:  {"durable", "resource_interval"},
		constant.TaskTypeTopupSync:        {"durable", "resource_interval"},
		constant.TaskTypeRedemptionSync:   {"durable", "resource_interval"},
		constant.TaskTypeUpstreamTaskSync: {"durable", "resource_interval"},
		constant.TaskTypeModelMetaSync:    {"durable", "resource_interval"},
		constant.TaskTypePlanSync:         {"durable", "resource_interval"},
		constant.TaskTypePricingSync:      {"durable", "resource_interval"},
		constant.TaskTypeSystemTaskSync:   {"durable", "resource_interval"},
		constant.TaskTypeUserSync:         {"hourly", "hour_boundary"},
		constant.TaskTypeChannelSync:      {"hourly", "hour_boundary"},
		constant.TaskTypeLogSync:          {"hourly", "hour_boundary"},
		constant.TaskTypeUsageHour:        {"usage", "usage_delay"},
		constant.TaskTypeUsageBackfill:    {"usage", "event_backfill"},
		constant.TaskTypeUsageValidation:  {"usage", "validation_calendar"},
		constant.TaskTypeAccountRebuild:   {"rebuild", "event_rebuild"},
		constant.TaskTypeCustomerRebuild:  {"rebuild", "event_rebuild"},
	}
	if len(data.Tasks) != len(want) {
		t.Fatalf("F12 task count = %d, want %d", len(data.Tasks), len(want))
	}
	seen := make(map[string]bool, len(data.Tasks))
	for _, task := range data.Tasks {
		if seen[task.TaskType] {
			t.Fatalf("F12 repeats task type %q", task.TaskType)
		}
		seen[task.TaskType] = true
		contract, ok := want[task.TaskType]
		if !ok {
			t.Fatalf("F12 contains undocumented task type %q", task.TaskType)
		}
		if task.Category != contract.category || task.TriggerClass != contract.trigger {
			t.Fatalf("task %q contract = (%q,%q), want (%q,%q)", task.TaskType, task.Category, task.TriggerClass, contract.category, contract.trigger)
		}
		if !constant.ValidCollectionTaskType(task.TaskType) {
			t.Fatalf("task %q is not accepted by backend task validation", task.TaskType)
		}
		if _, ok := constant.CollectionTaskFamily(task.TaskType); !ok {
			t.Fatalf("task %q has no backend task family", task.TaskType)
		}
		if _, ok := constant.CollectionTaskTarget(task.TaskType); !ok {
			t.Fatalf("task %q has no backend task target", task.TaskType)
		}
	}
	for taskType := range want {
		if !seen[taskType] {
			t.Fatalf("backend task type %q is missing from F12", taskType)
		}
	}
}
