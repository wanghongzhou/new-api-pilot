package docscheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"new-api-pilot/constant"
)

const (
	siteTaskCatalogDesignPath  = "docs/多站点运营管理平台-详细设计-02-数据采集与统计.md"
	siteTaskCatalogFixturePath = "testdata/design/f12-site-task-catalog.json"
	siteTaskCatalogStart       = "<!-- SITE_TASK_CATALOG_START -->"
	siteTaskCatalogEnd         = "<!-- SITE_TASK_CATALOG_END -->"
)

type siteTaskContract struct {
	TaskType     string `json:"task_type"`
	Category     string `json:"category"`
	TriggerClass string `json:"trigger_class"`
	PurposeKey   string `json:"purpose_key"`
}

type siteTaskCatalogFixture struct {
	SchemaVersion int                `json:"schema_version"`
	FixtureID     string             `json:"fixture_id"`
	Tasks         []siteTaskContract `json:"tasks"`
}

func expectedSiteTaskContracts() []siteTaskContract {
	return []siteTaskContract{
		{constant.TaskTypeSiteProbe, "fast", "fast_interval", "siteTasks.purpose.siteProbe"},
		{constant.TaskTypeRealtimeStat, "fast", "fast_interval", "siteTasks.purpose.realtimeStat"},
		{constant.TaskTypeResourceSnapshot, "fast", "fast_interval", "siteTasks.purpose.resourceSnapshot"},
		{constant.TaskTypePerformanceSync, "durable", "resource_interval", "siteTasks.purpose.performanceSync"},
		{constant.TaskTypeTopupSync, "durable", "resource_interval", "siteTasks.purpose.topupSync"},
		{constant.TaskTypeRedemptionSync, "durable", "resource_interval", "siteTasks.purpose.redemptionSync"},
		{constant.TaskTypeUpstreamTaskSync, "durable", "resource_interval", "siteTasks.purpose.upstreamTaskSync"},
		{constant.TaskTypeModelMetaSync, "durable", "resource_interval", "siteTasks.purpose.modelMetaSync"},
		{constant.TaskTypePlanSync, "durable", "resource_interval", "siteTasks.purpose.planSync"},
		{constant.TaskTypePricingSync, "durable", "resource_interval", "siteTasks.purpose.pricingGroupSync"},
		{constant.TaskTypeSystemTaskSync, "durable", "resource_interval", "siteTasks.purpose.systemTaskSync"},
		{constant.TaskTypeUserSync, "hourly", "hour_boundary", "siteTasks.purpose.userSync"},
		{constant.TaskTypeChannelSync, "hourly", "hour_boundary", "siteTasks.purpose.channelSync"},
		{constant.TaskTypeLogSync, "hourly", "hour_boundary", "siteTasks.purpose.logSync"},
		{constant.TaskTypeUsageHour, "usage", "usage_delay", "siteTasks.purpose.usageHour"},
		{constant.TaskTypeUsageBackfill, "usage", "event_backfill", "siteTasks.purpose.usageBackfill"},
		{constant.TaskTypeUsageValidation, "usage", "validation_calendar", "siteTasks.purpose.usageValidation"},
		{constant.TaskTypeAccountRebuild, "rebuild", "event_rebuild", "siteTasks.purpose.accountRebuild"},
		{constant.TaskTypeCustomerRebuild, "rebuild", "event_rebuild", "siteTasks.purpose.customerRebuild"},
	}
}

func (current *checker) checkSiteTaskCatalog() {
	expected := expectedSiteTaskContracts()
	fixturePath := filepath.Join(current.root, filepath.FromSlash(siteTaskCatalogFixturePath))
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		current.add("site-task-catalog", fixturePath, "open: %v", err)
		return
	}
	var fixture siteTaskCatalogFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		current.add("site-task-catalog", fixturePath, "decode JSON: %v", err)
		return
	}
	if fixture.SchemaVersion != 1 {
		current.add("site-task-catalog", fixturePath, "schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if fixture.FixtureID != "F12" {
		current.add("site-task-catalog", fixturePath, "fixture_id = %q, want F12", fixture.FixtureID)
	}
	current.compareSiteTaskContracts(fixturePath, "fixture", expected, fixture.Tasks)

	designPath := filepath.Join(current.root, filepath.FromSlash(siteTaskCatalogDesignPath))
	designPayload, err := os.ReadFile(designPath)
	if err != nil {
		current.add("site-task-catalog", designPath, "open: %v", err)
		return
	}
	designTasks, err := parseSiteTaskCatalogMarkdown(string(designPayload))
	if err != nil {
		current.add("site-task-catalog", designPath, "%v", err)
		return
	}
	current.compareSiteTaskContracts(designPath, "design", expected, designTasks)
	current.compareSiteTaskContracts(designPath, "design versus fixture", fixture.Tasks, designTasks)
}

func (current *checker) compareSiteTaskContracts(path, source string, expected, actual []siteTaskContract) {
	want := make(map[string]siteTaskContract, len(expected))
	for _, item := range expected {
		want[item.TaskType] = item
	}
	got := make(map[string]siteTaskContract, len(actual))
	for _, item := range actual {
		if strings.TrimSpace(item.TaskType) != item.TaskType || item.TaskType == "" {
			current.add("site-task-catalog", path, "%s has invalid task_type %q", source, item.TaskType)
			continue
		}
		if _, duplicate := got[item.TaskType]; duplicate {
			current.add("site-task-catalog", path, "%s repeats task_type %s", source, item.TaskType)
			continue
		}
		got[item.TaskType] = item
	}
	for taskType, contract := range want {
		item, exists := got[taskType]
		if !exists {
			current.add("site-task-catalog", path, "%s is missing task_type %s", source, taskType)
			continue
		}
		if item.Category != contract.Category {
			current.add("site-task-catalog", path, "%s task_type %s category = %q, want %q", source, taskType, item.Category, contract.Category)
		}
		if item.TriggerClass != contract.TriggerClass {
			current.add("site-task-catalog", path, "%s task_type %s trigger_class = %q, want %q", source, taskType, item.TriggerClass, contract.TriggerClass)
		}
		if item.PurposeKey != contract.PurposeKey {
			current.add("site-task-catalog", path, "%s task_type %s purpose_key = %q, want %q", source, taskType, item.PurposeKey, contract.PurposeKey)
		}
	}
	for taskType := range got {
		if _, exists := want[taskType]; !exists {
			current.add("site-task-catalog", path, "%s has unknown task_type %s", source, taskType)
		}
	}
}

func parseSiteTaskCatalogMarkdown(payload string) ([]siteTaskContract, error) {
	start := strings.Index(payload, siteTaskCatalogStart)
	end := strings.Index(payload, siteTaskCatalogEnd)
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("site task catalog markers are missing or out of order")
	}
	rows := make([]siteTaskContract, 0, 19)
	for _, raw := range strings.Split(payload[start+len(siteTaskCatalogStart):end], "\n") {
		columns := splitMarkdownRow(raw)
		if len(columns) == 0 || columns[0] == "task_type" || strings.HasPrefix(columns[0], "---") {
			continue
		}
		if len(columns) != 9 {
			return nil, fmt.Errorf("site task catalog row has %d columns, want 9: %s", len(columns), strings.TrimSpace(raw))
		}
		rows = append(rows, siteTaskContract{
			TaskType:     strings.Trim(columns[0], "` "),
			Category:     strings.Trim(columns[1], "` "),
			TriggerClass: strings.Trim(columns[2], "` "),
			PurposeKey:   strings.Trim(columns[3], "` "),
		})
	}
	return rows, nil
}
