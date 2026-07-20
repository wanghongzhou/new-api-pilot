package integration_test

import (
	"context"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestA98SubscriptionPlanCatalogMissingPrivacyAndExport(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type subscriptionFixture struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Plans         []struct {
			RemoteID                string `json:"remote_id"`
			Title                   string `json:"title"`
			Subtitle                string `json:"subtitle"`
			PriceAmount             string `json:"price_amount"`
			Currency                string `json:"currency"`
			DurationUnit            string `json:"duration_unit"`
			DurationValue           int    `json:"duration_value"`
			CustomSeconds           string `json:"custom_seconds"`
			Enabled                 bool   `json:"enabled"`
			SortOrder               int    `json:"sort_order"`
			TotalAmount             string `json:"total_amount"`
			QuotaResetPeriod        string `json:"quota_reset_period"`
			QuotaResetCustomSeconds string `json:"quota_reset_custom_seconds"`
		} `json:"plans"`
		Scenarios []string `json:"scenarios"`
	}
	fixture := loadDesignJSONFixture[subscriptionFixture](t, "f09-subscription-plans.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F09" || fixture.Clock.Timezone != "Asia/Shanghai" || len(fixture.Plans) != 1 {
		t.Fatalf("invalid F09 subscription-plan fixture: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.Scenarios, "body_hard_cap", "duplicate_id", "config_fence", "complete_missing", "reappear", "sensitive_absence")
	plan := fixture.Plans[0]
	db := openCoreAcceptanceTransaction(t)
	now := fixture.Clock.NowUnix
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	repo := model.NewSiteRepository(db)
	p1 := dto.UpstreamSubscriptionPlan{ID: fixtureInt64(t, "plans.remote_id", plan.RemoteID), Title: plan.Title, Subtitle: plan.Subtitle, PriceAmount: plan.PriceAmount, Currency: plan.Currency, DurationUnit: plan.DurationUnit, DurationValue: plan.DurationValue, CustomSeconds: fixtureInt64(t, "plans.custom_seconds", plan.CustomSeconds), Enabled: plan.Enabled, SortOrder: plan.SortOrder, TotalAmount: fixtureInt64(t, "plans.total_amount", plan.TotalAmount), QuotaResetPeriod: plan.QuotaResetPeriod, QuotaResetCustomSeconds: fixtureInt64(t, "plans.quota_reset_custom_seconds", plan.QuotaResetCustomSeconds), CreatedAt: 1, UpdatedAt: 2}
	p2 := dto.UpstreamSubscriptionPlan{ID: 2, Title: "Free", PriceAmount: "0", Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: false, TotalAmount: 0, QuotaResetPeriod: "never", CreatedAt: 1, UpdatedAt: 2}
	if written, err := repo.SyncSubscriptionPlans(context.Background(), site, now, dto.UpstreamSubscriptionPlanSnapshot{Items: []dto.UpstreamSubscriptionPlan{p1, p2}}); err != nil || written != 2 {
		t.Fatalf("initial written=%d err=%v", written, err)
	}
	if written, err := repo.SyncSubscriptionPlans(context.Background(), site, now+1, dto.UpstreamSubscriptionPlanSnapshot{Items: []dto.UpstreamSubscriptionPlan{p1}}); err != nil || written != 1 {
		t.Fatalf("missing transition written=%d err=%v", written, err)
	}
	svc, err := service.NewSubscriptionPlanService(db)
	if err != nil {
		t.Fatal(err)
	}
	q := dto.SubscriptionPlanQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}
	page, err := svc.List(context.Background(), q)
	normalizedPrice := strings.TrimRight(strings.TrimRight(plan.PriceAmount, "0"), ".")
	if err != nil || page.Total != 2 || page.Items[0].PriceAmount != normalizedPrice || page.Items[0].TotalAmount != plan.TotalAmount {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	stats, err := svc.Statistics(context.Background(), q)
	if err != nil || stats.Total != "2" || stats.Enabled != "1" || stats.Missing != "1" || len(stats.SiteBreakdown) != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	if written, err := repo.SyncSubscriptionPlans(context.Background(), site, now+2, dto.UpstreamSubscriptionPlanSnapshot{Items: []dto.UpstreamSubscriptionPlan{p1, p2}}); err != nil || written != 1 {
		t.Fatalf("reappear written=%d err=%v", written, err)
	}
	var state string
	_ = db.Model(&model.SiteSubscriptionPlan{}).Where("site_id=? AND remote_id=2", site.ID).Pluck("remote_state", &state).Error
	if state != "normal" {
		t.Fatalf("reappear state=%s", state)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "plans."+format)
		result, err := service.GenerateSubscriptionPlanExport(context.Background(), service.SubscriptionPlanExportOptions{Database: db, Query: q, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 2 {
			t.Fatalf("export=%+v err=%v", result, err)
		}
	}
	enabled := true
	enabledQuery := q
	enabledQuery.Enabled = &enabled
	filtered, err := svc.List(context.Background(), enabledQuery)
	if err != nil || filtered.Total != 1 || !filtered.Items[0].Enabled {
		t.Fatalf("enabled page=%#v err=%v", filtered, err)
	}
	filteredPath := filepath.Join(t.TempDir(), "enabled.csv")
	filteredExport, err := service.GenerateSubscriptionPlanExport(context.Background(), service.SubscriptionPlanExportOptions{Database: db, Query: enabledQuery, Format: dto.ExportFormatCSV, TemporaryPath: filteredPath, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
	if err != nil || filteredExport.RowCount != 1 {
		t.Fatalf("enabled export=%+v err=%v", filteredExport, err)
	}
	zeroSite := createCoreAuthorizedSite(t, db, newCoreCipher(t), now+10)
	disabledSite := createCoreAuthorizedSite(t, db, newCoreCipher(t), now+20)
	if _, err := repo.SyncSubscriptionPlans(context.Background(), disabledSite, now+21, dto.UpstreamSubscriptionPlanSnapshot{Items: []dto.UpstreamSubscriptionPlan{p1}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Site{}).Where("id=?", disabledSite.ID).Update("management_status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	globalStats, err := svc.Statistics(context.Background(), dto.SubscriptionPlanQuery{Page: 1, PageSize: 20})
	if err != nil || globalStats.Total != "2" || len(globalStats.SiteBreakdown) != 2 || globalStats.SiteBreakdown[1].SiteID != strconv.FormatInt(zeroSite.ID, 10) || globalStats.SiteBreakdown[1].Total != "0" || globalStats.SiteBreakdown[1].DataStatus != "pending" {
		t.Fatalf("global stats=%#v err=%v", globalStats, err)
	}
	for _, forbidden := range []string{"stripe_price_id", "creem_product_id", "waffo_pancake_product_id", "provider_payload"} {
		var count int64
		err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='site_subscription_plan' AND column_name=?", forbidden).Scan(&count).Error
		if err != nil || count != 0 {
			t.Fatalf("forbidden=%s count=%d err=%v", forbidden, count, err)
		}
	}
}
