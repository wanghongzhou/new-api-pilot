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
