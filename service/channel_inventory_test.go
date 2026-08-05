package service

import (
	"context"
	"testing"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestChannelInventoryListUsesCollectionCompletenessForEmptyAndFilteredResults(t *testing.T) {
	database := openUpstreamLogExportDatabase(t)
	now := int64(2100700000)
	site := newTestSite(now, "https://channel-completeness.example")
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	hour := now - now%3600
	snapshot := model.SiteChannelInventoryHourly{SiteID: site.ID, RemoteType: -1, RemoteStatus: -1, DimensionsAvailable: true, HourTS: hour, BalanceTotal: "0", ResponseTimeAvgMS: "0", AvailabilityRate: "0", DataStatus: "complete", ConfigVersion: site.ConfigVersion, CollectedAt: now}
	if err := database.GORM.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{TaskType: constant.TaskTypeChannelSync, TriggerType: constant.CollectionTriggerSchedule, Priority: 0, RequestID: "req_channel_complete", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Model(&model.CollectionRun{}).Where("id=?", run.ID).Updates(map[string]any{"status": model.CollectionTaskStatusSuccess, "active_key": nil, "finished_at": now + 1, "updated_at": now + 1}).Error; err != nil {
		t.Fatal(err)
	}
	application, err := NewChannelInventoryService(database.GORM)
	if err != nil {
		t.Fatal(err)
	}
	page, err := application.List(context.Background(), dto.ChannelInventoryQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}, Keyword: "no-match"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.DataStatus != "complete" || page.AsOf == nil || *page.AsOf != now {
		t.Fatalf("empty completed snapshot page=%#v", page)
	}
}

func TestChannelCompletenessIgnoresStaleConfigVersion(t *testing.T) {
	row := model.SiteChannelInventoryCompletenessRow{InventoryCount: 1, LatestRunStatus: "failed"}
	if status := channelSiteStatus(row); status != "unavailable" {
		t.Fatalf("failed collection status=%q", status)
	}
	if status := channelOverallStatus([]model.SiteChannelInventoryCompletenessRow{{LatestRunStatus: "success"}, row}); status != "partial" {
		t.Fatalf("mixed collection status=%q", status)
	}
}
