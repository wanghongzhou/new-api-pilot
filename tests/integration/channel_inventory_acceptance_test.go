package integration_test

import (
	"context"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	"os"
	"strings"
	"testing"
)

func TestChannelInventorySnapshotStatisticsAndPrivacyAcceptance(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	db := openCoreAcceptanceTransaction(t)
	now := int64(2100700000)
	hour := coreFloorHour(now)
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	channels := []model.SiteChannel{{RemoteChannelID: 1, Name: "Primary", RemoteType: 1, RemoteStatus: 1, ResponseTimeMS: 100, Balance: "12.3456789012", Models: "gpt", RemoteGroup: "default", UsedQuota: 9007199254740993, Priority: 10, Weight: 20, AutoBan: 1, Tag: "prod"}, {RemoteChannelID: 2, Name: "Backup", RemoteType: 2, RemoteStatus: 2, ResponseTimeMS: 900, Balance: "0.1", Models: "claude", RemoteGroup: "vip"}}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return model.NewSiteRepository(tx).SyncChannels(context.Background(), site.ID, now, channels)
	}); err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewChannelInventoryService(db)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(context.Background(), dto.ChannelInventoryQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}})
	if err != nil || page.Total != "2" || page.Items[0].Balance != "12.3456789012" || page.Items[0].UsedQuota != "9007199254740993" {
		t.Fatalf("channel page=%#v err=%v", page, err)
	}
	stats, err := svc.Statistics(context.Background(), dto.ChannelInventoryStatisticsQuery{StartTimestamp: hour, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID}})
	if err != nil || stats.Summary.ChannelCount != "2" || stats.Summary.AvailableCount != "1" || len(stats.Trend) != 1 || len(stats.SiteBreakdown) != 1 {
		t.Fatalf("channel stats=%#v err=%v", stats, err)
	}
	filtered, err := svc.Statistics(context.Background(), dto.ChannelInventoryStatisticsQuery{StartTimestamp: hour, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID}, Types: []int{1}, Statuses: []int{1}, Groups: []string{"default"}, Tags: []string{"prod"}})
	if err != nil || len(filtered.Trend) != 1 || filtered.Trend[0].ChannelCount != "1" || filtered.Trend[0].AvailableCount != "1" || filtered.Trend[0].BalanceTotal != "12.3456789012" {
		t.Fatalf("filtered channel trend=%#v err=%v", filtered, err)
	}
	missingFilter, err := svc.Statistics(context.Background(), dto.ChannelInventoryStatisticsQuery{StartTimestamp: hour, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID}, Groups: []string{"absent"}})
	if err != nil || len(missingFilter.Trend) != 1 || missingFilter.Trend[0].DataStatus != "complete" ||
		missingFilter.Trend[0].ChannelCount != "0" || missingFilter.Trend[0].AvailableCount != "0" ||
		len(missingFilter.SiteBreakdown) != 1 || missingFilter.SiteBreakdown[0].ChannelCount != "0" {
		t.Fatalf("absent filtered channel trend=%#v err=%v", missingFilter, err)
	}
	legacy := model.SiteChannelInventoryHourly{SiteID: site.ID, RemoteType: -1, RemoteStatus: -1, HourTS: hour - 3600, ChannelCount: 2, AvailableCount: 1, UnavailableCount: 1, BalanceTotal: "12.4456789012", ResponseTimeAvgMS: "500", ResponseTimeMaxMS: 900, AvailabilityRate: "0.5", DataStatus: "complete", ConfigVersion: site.ConfigVersion, CollectedAt: now - 3600}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	legacyFiltered, err := svc.Statistics(context.Background(), dto.ChannelInventoryStatisticsQuery{StartTimestamp: hour - 3600, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID}, Types: []int{1}})
	if err != nil || legacyFiltered.DataStatus != "partial" || len(legacyFiltered.Trend) != 2 ||
		legacyFiltered.Trend[0].BucketStart != hour-3600 || legacyFiltered.Trend[0].DataStatus != "missing" ||
		legacyFiltered.Trend[1].BucketStart != hour || legacyFiltered.Trend[1].DataStatus != "complete" {
		t.Fatalf("legacy filtered channel trend=%#v err=%v", legacyFiltered, err)
	}
	var columns []struct {
		Name string `gorm:"column:COLUMN_NAME"`
	}
	if err := db.Raw(`SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='site_channel_inventory'`).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, c := range columns {
		joined += "|" + strings.ToLower(c.Name)
	}
	for _, forbidden := range []string{"key", "multi_key", "header_override", "param_override", "setting"} {
		if strings.Contains(joined, "|"+forbidden) {
			t.Fatalf("forbidden channel column %s in %s", forbidden, joined)
		}
	}
}
