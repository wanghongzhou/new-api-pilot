package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestA93A94FinanceOperationsExactPrivacyAndAggregationBoundaries(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type financeFixture struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Topups        []struct {
			RemoteID        string `json:"remote_id"`
			RemoteUserID    string `json:"remote_user_id"`
			Amount          string `json:"amount"`
			Money           string `json:"money"`
			PaymentMethod   string `json:"payment_method"`
			PaymentProvider string `json:"payment_provider"`
			CreateTime      int64  `json:"create_time"`
			CompleteTime    int64  `json:"complete_time"`
			Status          string `json:"status"`
		} `json:"topups"`
		Redemptions []struct {
			RemoteID     string `json:"remote_id"`
			RemoteUserID string `json:"remote_user_id"`
			Name         string `json:"name"`
			Status       int    `json:"status"`
			Quota        string `json:"quota"`
			CreatedTime  int64  `json:"created_time"`
			RedeemedTime int64  `json:"redeemed_time"`
			UsedUserID   string `json:"used_user_id"`
			ExpiredTime  int64  `json:"expired_time"`
		} `json:"redemptions"`
		SnapshotScenarios []string `json:"snapshot_scenarios"`
		PrivacyContract   string   `json:"privacy_contract"`
	}
	fixture := loadDesignJSONFixture[financeFixture](t, "f06-finance-operations.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F06" || fixture.Clock.Timezone != "Asia/Shanghai" || len(fixture.Topups) != 1 || len(fixture.Redemptions) != 1 || fixture.PrivacyContract == "" {
		t.Fatalf("invalid F06 finance fixture: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.SnapshotScenarios, "complete", "total_drift", "maximum_id_drift", "duplicate_id", "over_100000", "missing", "reappear", "config_fence")
	topupFixture := fixture.Topups[0]
	redemptionFixture := fixture.Redemptions[0]
	db := openCoreAcceptanceTransaction(t)
	now := fixture.Clock.NowUnix
	cipher := newCoreCipher(t)
	sites := []model.Site{createCoreAuthorizedSite(t, db, cipher, now), createCoreAuthorizedSite(t, db, cipher, now+1)}
	for _, site := range sites {
		topupID := fixtureInt64(t, "topups.remote_id", topupFixture.RemoteID)
		redemptionID := fixtureInt64(t, "redemptions.remote_id", redemptionFixture.RemoteID)
		topups := dto.UpstreamTopupSnapshot{Total: 1, MaxID: topupID, Items: []dto.UpstreamTopup{{ID: topupID, UserID: fixtureInt64(t, "topups.remote_user_id", topupFixture.RemoteUserID), Amount: fixtureInt64(t, "topups.amount", topupFixture.Amount), Money: topupFixture.Money, PaymentMethod: topupFixture.PaymentMethod, PaymentProvider: topupFixture.PaymentProvider, CreateTime: topupFixture.CreateTime, CompleteTime: topupFixture.CompleteTime, Status: topupFixture.Status}}}
		redemptions := dto.UpstreamRedemptionSnapshot{Total: 1, MaxID: redemptionID, Items: []dto.UpstreamRedemption{{ID: redemptionID, UserID: fixtureInt64(t, "redemptions.remote_user_id", redemptionFixture.RemoteUserID), Name: redemptionFixture.Name, Status: redemptionFixture.Status, Quota: fixtureInt64(t, "redemptions.quota", redemptionFixture.Quota), CreatedTime: redemptionFixture.CreatedTime, RedeemedTime: redemptionFixture.RedeemedTime, UsedUserID: fixtureInt64(t, "redemptions.used_user_id", redemptionFixture.UsedUserID), ExpiredTime: redemptionFixture.ExpiredTime}}}
		if err := db.Transaction(func(tx *gorm.DB) error {
			repo := model.NewSiteRepository(tx)
			if _, err := repo.SyncTopups(context.Background(), site, now, topups); err != nil {
				return err
			}
			_, err := repo.SyncRedemptions(context.Background(), site, now, redemptions)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := service.NewFinanceOperationsService(db, testsupport.NewFakeClock(time.Unix(now, 0)))
	if err != nil {
		t.Fatal(err)
	}
	q := dto.FinanceInventoryQuery{Page: 1, PageSize: 20, SiteIDs: []int64{sites[0].ID, sites[1].ID}}
	topupPage, err := svc.Topups(context.Background(), q)
	if err != nil || topupPage.Total != 2 || topupPage.Items[0].Amount != topupFixture.Amount || topupPage.Items[0].Money != "10.1234567890" {
		t.Fatalf("topup page=%#v err=%v", topupPage, err)
	}
	topupStats, err := svc.TopupStatistics(context.Background(), q)
	if err != nil || topupStats.Summary.Count != "2" || topupStats.Summary.Amount != "" || topupStats.Summary.Money != "" || len(topupStats.ProviderBreakdown) != 2 || len(topupStats.SiteBreakdown) != 2 {
		t.Fatalf("topup stats=%#v err=%v", topupStats, err)
	}
	redemptionPage, err := svc.Redemptions(context.Background(), q)
	if err != nil || redemptionPage.Total != 2 || redemptionPage.Items[0].DerivedStatus != "expired" || redemptionPage.Items[0].Quota != redemptionFixture.Quota {
		t.Fatalf("redemption page=%#v err=%v", redemptionPage, err)
	}
	encoded, _ := json.Marshal(struct {
		Topups      any
		Redemptions any
	}{topupPage, redemptionPage})
	for _, forbidden := range []string{strings.Join([]string{"trade", "no"}, "_"), strings.Join([]string{"k", "ey"}, "")} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("forbidden field %q in response: %s", forbidden, encoded)
		}
	}
	for _, table := range []string{"site_topup_order", "site_redemption"} {
		var count int64
		for _, column := range []string{strings.Join([]string{"trade", "no"}, "_"), strings.Join([]string{"k", "ey"}, "")} {
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count).Error; err != nil || count != 0 {
				t.Fatalf("forbidden schema column %s.%s count=%d err=%v", table, column, count, err)
			}
		}
	}
}
