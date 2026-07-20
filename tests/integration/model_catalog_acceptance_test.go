package integration_test

import (
	"context"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	"os"
	"strings"
	"testing"
)

func TestA96ModelCatalogCoverageMissingAndPrivacy(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type modelCatalogFixture struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Models        []struct {
			RemoteID     string `json:"remote_id"`
			ModelName    string `json:"model_name"`
			VendorID     string `json:"vendor_id"`
			Status       int    `json:"status"`
			SyncOfficial int    `json:"sync_official"`
			NameRule     int    `json:"name_rule"`
		} `json:"models"`
		Channels []struct {
			RemoteChannelID string `json:"remote_channel_id"`
			Models          string `json:"models"`
			Groups          string `json:"groups"`
		} `json:"channels"`
		ExpectedExactMissing []string `json:"expected_exact_missing"`
		Scenarios            []string `json:"scenarios"`
	}
	fixture := loadDesignJSONFixture[modelCatalogFixture](t, "f08-model-catalog.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F08" || fixture.Clock.Timezone != "Asia/Shanghai" || len(fixture.Models) != 2 || len(fixture.Channels) != 1 || len(fixture.ExpectedExactMissing) != 1 {
		t.Fatalf("invalid F08 model-catalog fixture: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.Scenarios, "full_pagination_fence", "hard_cap", "config_fence", "complete_snapshot_edge_replace", "partial_snapshot_preserves_edges", "mapping_dedup", "exact_name_rule")
	db := openCoreAcceptanceTransaction(t)
	now := fixture.Clock.NowUnix
	site1 := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	site2 := createCoreAuthorizedSite(t, db, newCoreCipher(t), now+1)
	site3 := createCoreAuthorizedSite(t, db, newCoreCipher(t), now+2)
	repo := model.NewSiteRepository(db)
	channelFixture := fixture.Channels[0]
	channels1 := []model.SiteChannel{{RemoteChannelID: fixtureInt64(t, "channels.remote_channel_id", channelFixture.RemoteChannelID), Name: "primary", Models: channelFixture.Models + ",gpt-4o", RemoteGroup: channelFixture.Groups + ",default", Balance: "0"}}
	channels2 := []model.SiteChannel{{RemoteChannelID: 4, Name: "secondary", Models: "gpt-4o", RemoteGroup: "default", Balance: "0"}}
	channels3 := []model.SiteChannel{{RemoteChannelID: 5, Name: "catalog-empty", Models: "only-missing", RemoteGroup: "default", Balance: "0"}}
	if err := repo.SyncChannels(context.Background(), site1.ID, now, channels1); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncChannels(context.Background(), site2.ID, now+1, channels2); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncChannels(context.Background(), site3.ID, now+2, channels3); err != nil {
		t.Fatal(err)
	}
	modelExact := fixture.Models[0]
	modelRule := fixture.Models[1]
	snap1 := dto.UpstreamModelMetaSnapshot{Total: 2, MaxID: fixtureInt64(t, "models.remote_id", modelRule.RemoteID), Items: []dto.UpstreamModelMeta{{ID: fixtureInt64(t, "models.remote_id", modelRule.RemoteID), ModelName: modelRule.ModelName, Description: "prefix only", Icon: "https://icons.invalid/no-load.svg", VendorID: fixtureInt64(t, "models.vendor_id", modelRule.VendorID), Status: modelRule.Status, SyncOfficial: modelRule.SyncOfficial, NameRule: modelRule.NameRule, CreatedTime: 1, UpdatedTime: 2}, {ID: fixtureInt64(t, "models.remote_id", modelExact.RemoteID), ModelName: modelExact.ModelName, Description: "exact", VendorID: fixtureInt64(t, "models.vendor_id", modelExact.VendorID), Status: modelExact.Status, SyncOfficial: modelExact.SyncOfficial, NameRule: modelExact.NameRule, CreatedTime: 1, UpdatedTime: 2}}}
	snap2 := dto.UpstreamModelMetaSnapshot{Total: 1, MaxID: 21, Items: []dto.UpstreamModelMeta{{ID: 21, ModelName: "gpt-4o", Description: "same name other site", VendorID: 7, Status: 1, SyncOfficial: 1, NameRule: 0, CreatedTime: 1, UpdatedTime: 2}}}
	if _, err := repo.SyncModelCatalog(context.Background(), site1, now, snap1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SyncModelCatalog(context.Background(), site2, now+1, snap2); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SyncModelCatalog(context.Background(), site3, now+2, dto.UpstreamModelMetaSnapshot{}); err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewModelCatalogService(db)
	if err != nil {
		t.Fatal(err)
	}
	q := dto.ModelCatalogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site1.ID, site2.ID, site3.ID}}
	page, err := svc.List(context.Background(), q)
	if err != nil || page.Total != 3 || page.DataStatus != "complete" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	coverage, err := svc.Coverage(context.Background(), q)
	if err != nil || coverage.CatalogModels != "3" || coverage.ExactCoveredModels != "2" || coverage.ExactMissingModels != "3" || coverage.ChannelMappings != "6" || len(coverage.SiteBreakdown) != 3 {
		t.Fatalf("coverage=%#v err=%v", coverage, err)
	}
	missing, err := svc.Missing(context.Background(), q)
	if err != nil || missing.Total != 3 || missing.Items[0].ModelName != fixture.ExpectedExactMissing[0] {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
	filtered := q
	filtered.VendorID = func() *int64 { value := int64(7); return &value }()
	filteredCoverage, err := svc.Coverage(context.Background(), filtered)
	if err != nil || filteredCoverage.CatalogModels != "3" || filteredCoverage.ExactCoveredModels != "2" || filteredCoverage.ExactMissingModels != "0" || filteredCoverage.ChannelMappings != "3" || len(filteredCoverage.SiteBreakdown) != 3 {
		t.Fatalf("filtered coverage=%#v err=%v", filteredCoverage, err)
	}
	for _, item := range page.Items {
		if item.NameRule != 0 && item.CoveredChannels != "0" {
			t.Fatalf("non-exact model reported exact coverage: %#v", item)
		}
	}
	var mappingCount int64
	if err := db.Model(&model.SiteChannelModelMapping{}).Where("site_id=?", site1.ID).Count(&mappingCount).Error; err != nil || mappingCount != 4 {
		t.Fatalf("mapping count=%d err=%v", mappingCount, err)
	}
	if err := repo.SyncChannels(context.Background(), site1.ID, now+2, []model.SiteChannel{{RemoteChannelID: 3, Name: "a", Models: "changed", Balance: "0"}, {RemoteChannelID: 3, Name: "duplicate", Models: "changed", Balance: "0"}}); err == nil {
		t.Fatal("duplicate channel snapshot accepted")
	}
	var preserved int64
	_ = db.Model(&model.SiteChannelModelMapping{}).Where("site_id=?", site1.ID).Count(&preserved).Error
	if preserved != 4 {
		t.Fatalf("failed snapshot changed mappings=%d", preserved)
	}
	older := snap1
	older.Items = append([]dto.UpstreamModelMeta{}, snap1.Items...)
	older.Items[1].UpdatedTime = 1
	older.Items[1].Description = "regressed"
	newer := snap1
	newer.Items = append([]dto.UpstreamModelMeta{}, snap1.Items...)
	newer.Items[1].UpdatedTime = 3
	newer.Items[1].Description = "exact-updated"
	if written, err := repo.SyncModelCatalog(context.Background(), site1, now+2, newer); err != nil || written != 1 {
		t.Fatalf("updated snapshot written=%d err=%v", written, err)
	}
	if _, err := repo.SyncModelCatalog(context.Background(), site1, now+3, older); err != nil {
		t.Fatal(err)
	}
	var exact model.SiteModelMeta
	if err := db.Where("site_id=? AND remote_id=11", site1.ID).Take(&exact).Error; err != nil || exact.Description != "exact-updated" {
		t.Fatalf("old response regressed row=%+v err=%v", exact, err)
	}
	trimmed := dto.UpstreamModelMetaSnapshot{Total: 1, MaxID: 11, Items: []dto.UpstreamModelMeta{snap1.Items[1]}}
	if written, err := repo.SyncModelCatalog(context.Background(), site1, now+4, trimmed); err != nil || written != 1 {
		t.Fatalf("trimmed snapshot written=%d err=%v", written, err)
	}
	var remaining int64
	_ = db.Model(&model.SiteModelMeta{}).Where("site_id=?", site1.ID).Count(&remaining).Error
	if remaining != 1 {
		t.Fatalf("complete snapshot did not remove missing rows=%d", remaining)
	}
	stale := site1
	stale.ConfigVersion--
	err = repo.WithTransaction(context.Background(), func(r *model.SiteRepository) error {
		return r.MarkModelCatalogFailure(context.Background(), stale, now+5, "FAIL")
	})
	if err == nil {
		t.Fatal("stale config failure state committed")
	}
	for _, forbidden := range []string{"pricing", "billing_expr", "endpoints", "bound_channels"} {
		var count int64
		err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='site_model_meta' AND column_name=?", forbidden).Scan(&count).Error
		if err != nil || count != 0 {
			t.Fatalf("forbidden=%s count=%d err=%v", forbidden, count, err)
		}
	}
}
