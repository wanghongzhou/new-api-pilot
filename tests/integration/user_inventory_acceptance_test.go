package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

func TestA81CompleteUserInventorySnapshotStatisticsAndPrivacyAcceptance(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		if os.Getenv("ACCEPTANCE_ID") == "A81" {
			t.Fatal("A81 requires TEST_DATABASE_DSN")
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	database := openCoreAcceptanceTransaction(t)
	now := int64(2_100_400_000)
	hour := coreFloorHour(now)
	site := createCoreAuthorizedSite(t, database, newCoreCipher(t), now)
	repository := model.NewCollectionTaskRepository(database)
	observations := []model.SiteUserObservation{
		{RemoteUserID: 1, RemoteCreatedAt: hour - 7200, Username: "alice", DisplayName: "Alice", RemoteRole: 10, RemoteStatus: 1, RemoteGroup: "vip", Quota: 1000, UsedQuota: 400, RequestCount: 7, LastLoginAt: hour + 60},
		{RemoteUserID: 2, RemoteCreatedAt: hour + 120, Username: "bob", DisplayName: "Bob", RemoteRole: 1, RemoteStatus: 1, RemoteGroup: "default", Quota: 2000, UsedQuota: 500, RequestCount: 9},
	}
	if _, err := repository.ApplySiteUserSnapshot(context.Background(), site, now, hour, observations); err != nil {
		t.Fatalf("apply complete A81 snapshot: %v", err)
	}
	inventory, err := service.NewUserInventoryService(database)
	if err != nil {
		t.Fatal(err)
	}
	page, err := inventory.List(context.Background(), dto.UserInventoryQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}})
	if err != nil || page.Total != 2 || page.DataStatus != "complete" || page.Items[0].Quota == "" || page.Items[0].RemoteUserID == "" {
		t.Fatalf("A81 inventory page=%#v err=%v", page, err)
	}
	statistics, err := inventory.Statistics(context.Background(), dto.UserInventoryStatisticsQuery{StartTimestamp: hour, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID}})
	if err != nil || statistics.Summary.UserCount != "2" || statistics.Summary.NewUserCount != "1" || statistics.Summary.ActiveUserCount != "1" ||
		len(statistics.Trend) != 1 || statistics.Trend[0].DataStatus != "complete" || len(statistics.SiteBreakdown) != 1 ||
		statistics.SiteBreakdown[0].DataStatus != "complete" {
		t.Fatalf("A81 statistics=%#v err=%v", statistics, err)
	}
	pendingSite := createCoreAuthorizedSite(t, database, newCoreCipher(t), now+1)
	partialPage, err := inventory.List(context.Background(), dto.UserInventoryQuery{
		Page: 1, PageSize: 20, SiteIDs: []int64{site.ID, pendingSite.ID},
	})
	if err != nil || partialPage.DataStatus != "partial" || partialPage.Total != 2 {
		t.Fatalf("A81 partial inventory page=%#v err=%v", partialPage, err)
	}
	partialStatistics, err := inventory.Statistics(context.Background(), dto.UserInventoryStatisticsQuery{
		StartTimestamp: hour, EndTimestamp: hour + 3600, SiteIDs: []int64{site.ID, pendingSite.ID},
	})
	if err != nil || partialStatistics.DataStatus != "partial" || len(partialStatistics.SiteBreakdown) != 2 ||
		partialStatistics.SiteBreakdown[1].DataStatus != "pending" || partialStatistics.Trend[0].DataStatus != "partial" {
		t.Fatalf("A81 partial statistics=%#v err=%v", partialStatistics, err)
	}
	var columns []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
	}
	if err := database.Raw(`SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'site_user_inventory'`).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	joined := "|"
	for _, column := range columns {
		joined += strings.ToLower(column.ColumnName) + "|"
	}
	for _, forbidden := range []string{"email", "password", "access_token", "oauth", "wechat", "github", "setting", "stripe"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("A81 inventory schema contains forbidden sensitive field %q: %s", forbidden, joined)
		}
	}
}
