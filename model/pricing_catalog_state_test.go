package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func TestPricingCollectionStateIsFencedByCurrentSiteConfig(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_101_500_000)
	site := createRunnableSite(t, database, fmt.Sprintf("pricing-state-%d", time.Now().UnixNano()), now)
	t.Cleanup(func() {
		_ = database.GORM.Where("site_id=?", site.ID).Delete(&SitePricingCollectionState{}).Error
	})
	for _, kind := range []string{"pricing", "group"} {
		asOf := now
		state := SitePricingCollectionState{SiteID: site.ID, ResourceKind: kind, DataStatus: "complete", AsOf: &asOf, LastCompleteAt: &asOf, ConfigVersion: site.ConfigVersion, UpdatedAt: now}
		if err := database.GORM.Create(&state).Error; err != nil {
			t.Fatal(err)
		}
	}
	repository := NewPricingCatalogRepository(database.GORM)
	query := dto.PricingCatalogQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}
	rows, err := repository.SiteMetrics(context.Background(), query)
	if err != nil || len(rows) != 1 || rows[0].PricingLastCompleteAt == nil || rows[0].GroupLastCompleteAt == nil {
		t.Fatalf("current config pricing state=%#v err=%v", rows, err)
	}
	if err := database.GORM.Model(&Site{}).Where("id=?", site.ID).Update("config_version", site.ConfigVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	rows, err = repository.SiteMetrics(context.Background(), query)
	if err != nil || len(rows) != 1 || rows[0].PricingLastCompleteAt != nil || rows[0].GroupLastCompleteAt != nil {
		t.Fatalf("stale config pricing state leaked=%#v err=%v", rows, err)
	}
}

func TestPricingCatalogUnchangedFactsAreNotRewritten(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_101_500_500)
	site := createRunnableSite(t, database, fmt.Sprintf("pricing-zero-write-%d", time.Now().UnixNano()), now)
	repository := NewSiteRepository(database.GORM)
	ratio := "1"
	snapshot := dto.UpstreamPricingSnapshot{
		PricingVersion: "v1",
		Items: []dto.UpstreamPricingItem{{
			ModelName: "gpt-test", VendorName: "openai", ModelRatio: "1", ModelPrice: "0",
			CompletionRatio: "1", BillingMode: "token", PricingSource: "configured", AbilityAvailable: true,
		}},
		Groups: []dto.UpstreamPricingGroup{{Name: "default", Ratio: &ratio, TopupRatio: &ratio, UserSelectable: true}},
	}
	if written, err := repository.SyncPricingCatalog(context.Background(), site, now, snapshot); err != nil || written != 2 {
		t.Fatalf("initial pricing sync written=%d err=%v", written, err)
	}
	var originalItem SitePricingCatalog
	var originalGroup SitePricingGroup
	if err := database.GORM.Where("site_id=? AND model_name=?", site.ID, "gpt-test").Take(&originalItem).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Where("site_id=? AND group_name=?", site.ID, "default").Take(&originalGroup).Error; err != nil {
		t.Fatal(err)
	}
	if written, err := repository.SyncPricingCatalog(context.Background(), site, now+1, snapshot); err != nil || written != 0 {
		t.Fatalf("unchanged pricing sync written=%d err=%v", written, err)
	}
	var unchangedItem SitePricingCatalog
	var unchangedGroup SitePricingGroup
	if err := database.GORM.Where("site_id=? AND model_name=?", site.ID, "gpt-test").Take(&unchangedItem).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Where("site_id=? AND group_name=?", site.ID, "default").Take(&unchangedGroup).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedItem.UpdatedAt != originalItem.UpdatedAt || unchangedItem.CollectedAt != originalItem.CollectedAt ||
		unchangedGroup.UpdatedAt != originalGroup.UpdatedAt || unchangedGroup.CollectedAt != originalGroup.CollectedAt {
		t.Fatalf("unchanged pricing facts were rewritten: item=%+v group=%+v", unchangedItem, unchangedGroup)
	}
}
