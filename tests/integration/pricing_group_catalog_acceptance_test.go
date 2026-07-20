package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

func TestA99PricingAndGroupCatalog(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type fixtureType struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Groups        []struct {
			GroupName string `json:"group_name"`
		} `json:"groups"`
		Pricing []struct {
			ModelName          string   `json:"model_name"`
			InputPrice         string   `json:"input_price"`
			OutputPrice        string   `json:"output_price"`
			UsableGroups       []string `json:"usable_groups"`
			SupportedEndpoints []string `json:"supported_endpoints"`
		} `json:"pricing"`
		Scenarios []string `json:"scenarios"`
	}
	fixture := loadDesignJSONFixture[fixtureType](t, "f10-pricing-groups.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F10" || len(fixture.Groups) < 2 || len(fixture.Pricing) < 1 {
		t.Fatalf("invalid F10: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.Scenarios, "zero_usage_group_preserved", "exact_decimal_round_trip", "config_fence", "complete_missing", "reappear", "sensitive_absence")
	db := openCoreAcceptanceTransaction(t)
	now := fixture.Clock.NowUnix
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	repo := model.NewSiteRepository(db)
	item := dto.UpstreamPricingItem{ModelName: fixture.Pricing[0].ModelName, VendorKey: "openai", ModelRatio: fixture.Pricing[0].InputPrice, ModelPrice: fixture.Pricing[0].OutputPrice, CompletionRatio: "1", RootVisible: true, EnableGroups: []string{"default", "vip-zero-usage"}, SupportedEndpointTypes: []string{"chat_completions", "responses"}}
	one, discount := "1", "0.85"
	groups := []dto.UpstreamPricingGroup{{Name: "default", Ratio: &one, RootVisible: true}, {Name: "vip-zero-usage", Ratio: &discount, RootVisible: true}}
	if written, err := repo.SyncPricingCatalog(context.Background(), site, now, dto.UpstreamPricingSnapshot{PricingVersion: "pinned", Items: []dto.UpstreamPricingItem{item}, Groups: groups}); err != nil || written != 3 {
		t.Fatalf("sync written=%d err=%v", written, err)
	}
	svc, err := service.NewPricingCatalogService(db)
	if err != nil {
		t.Fatal(err)
	}
	q := dto.PricingCatalogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}
	page, err := svc.List(context.Background(), q)
	wantRatio := strings.TrimRight(strings.TrimRight(fixture.Pricing[0].InputPrice, "0"), ".")
	if err != nil || page.Total != 1 || page.Items[0].ModelRatio != wantRatio {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	groupPage, err := svc.ListGroups(context.Background(), q)
	if err != nil || groupPage.Total != 2 || groupPage.Items[1].Name != "vip-zero-usage" || !groupPage.Items[1].RootVisible {
		t.Fatalf("groups=%#v err=%v", groupPage, err)
	}
	if written, err := repo.SyncPricingCatalog(context.Background(), site, now+1, dto.UpstreamPricingSnapshot{PricingVersion: "pinned", Items: nil, Groups: groups[:1]}); err != nil || written != 2 {
		t.Fatalf("missing written=%d err=%v", written, err)
	}
	stats, err := svc.Statistics(context.Background(), q)
	if err != nil || stats.Total != "1" || stats.Missing != "1" {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	if _, err := repo.SyncPricingCatalog(context.Background(), site, now+2, dto.UpstreamPricingSnapshot{PricingVersion: "pinned", Items: []dto.UpstreamPricingItem{item}, Groups: groups}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"pricing", "group"} {
		path := filepath.Join(t.TempDir(), kind+".csv")
		result, err := service.GeneratePricingCatalogExport(context.Background(), service.PricingCatalogExportOptions{Database: db, Query: q, Kind: kind, Format: dto.ExportFormatCSV, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount < 1 {
			t.Fatalf("kind=%s result=%+v err=%v", kind, result, err)
		}
	}
	for _, forbidden := range []string{"billing_expr", "custom_path", "channel_key", "base_url"} {
		var count int64
		err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ('site_pricing_catalog','site_group_catalog') AND column_name=?", forbidden).Scan(&count).Error
		if err != nil || count != 0 {
			t.Fatalf("forbidden=%s count=%d err=%v", forbidden, count, err)
		}
	}
}
