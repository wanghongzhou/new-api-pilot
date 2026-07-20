package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestFinanceInventoryCompletenessCoversTheEntireFilteredResult(t *testing.T) {
	database := openUpstreamLogExportDatabase(t)
	now := int64(2100915000)
	site := model.Site{
		Name: "Finance Completeness", BaseURL: fmt.Sprintf("https://finance-completeness-%d.example", time.Now().UnixNano()),
		ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	topups := make([]model.SiteTopupOrder, 0, 101)
	redemptions := make([]model.SiteRedemption, 0, 101)
	for id := int64(1); id <= 101; id++ {
		state, updatedAt := "normal", now
		missingCount := 0
		var lastSeen *int64
		if id == 1 {
			state, updatedAt, missingCount = "missing", now+10, 1
		} else {
			seen := now
			lastSeen = &seen
		}
		topups = append(topups, model.SiteTopupOrder{
			SiteID: site.ID, RemoteID: id, Amount: 1, Money: "1", RemoteStatus: "success",
			RemoteState: state, MissingCount: missingCount, ConfigVersion: 1, FirstSeenAt: now,
			LastSeenAt: lastSeen, CollectedAt: now, CreatedAt: now, UpdatedAt: updatedAt,
		})
		redemptions = append(redemptions, model.SiteRedemption{
			SiteID: site.ID, RemoteID: id, Name: "batch", RemoteStatus: 1, Quota: 1,
			RemoteState: state, MissingCount: missingCount, ConfigVersion: 1, FirstSeenAt: now,
			LastSeenAt: lastSeen, CollectedAt: now, CreatedAt: now, UpdatedAt: updatedAt,
		})
	}
	if err := database.GORM.CreateInBatches(topups, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.CreateInBatches(redemptions, 100).Error; err != nil {
		t.Fatal(err)
	}
	svc, err := NewFinanceOperationsService(database.GORM, testsupport.NewFakeClock(time.Unix(now+20, 0)))
	if err != nil {
		t.Fatal(err)
	}
	query := dto.FinanceInventoryQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}}
	topupPage, err := svc.Topups(context.Background(), query)
	if err != nil || len(topupPage.Items) != 100 || topupPage.Items[0].RemoteState != "normal" || topupPage.DataStatus != "partial" || topupPage.AsOf == nil || *topupPage.AsOf != now+10 {
		t.Fatalf("topup completeness=%#v err=%v", topupPage, err)
	}
	redemptionPage, err := svc.Redemptions(context.Background(), query)
	if err != nil || len(redemptionPage.Items) != 100 || redemptionPage.Items[0].RemoteState != "normal" || redemptionPage.DataStatus != "partial" || redemptionPage.AsOf == nil || *redemptionPage.AsOf != now+10 {
		t.Fatalf("redemption completeness=%#v err=%v", redemptionPage, err)
	}
}
