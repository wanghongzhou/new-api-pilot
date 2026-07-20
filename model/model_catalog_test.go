package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func cleanupModelCatalogTestRows(t *testing.T, database *Database, siteIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		_ = database.GORM.Where("site_id IN ?", siteIDs).Delete(&SiteChannelModelMapping{}).Error
		_ = database.GORM.Where("site_id IN ?", siteIDs).Delete(&SiteModelMeta{}).Error
		_ = database.GORM.Where("site_id IN ?", siteIDs).Delete(&SiteModelMetaCollectionState{}).Error
	})
}

func TestModelCatalogStatusesAggregatePendingUnavailableAndMixed(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_101_300_000)
	pending := createRunnableSite(t, database, fmt.Sprintf("model-status-pending-%d", time.Now().UnixNano()), now)
	unavailable := createRunnableSite(t, database, fmt.Sprintf("model-status-unavailable-%d", time.Now().UnixNano()), now+1)
	cleanupModelCatalogTestRows(t, database, pending.ID, unavailable.ID)

	failureAt := now + 2
	if err := database.GORM.Create(&SiteModelMetaCollectionState{
		SiteID: unavailable.ID, LastFailureAt: &failureAt, LastErrorCode: "UPSTREAM_UNAVAILABLE",
		ConfigVersion: unavailable.ConfigVersion, UpdatedAt: failureAt,
	}).Error; err != nil {
		t.Fatalf("create unavailable model catalog state: %v", err)
	}

	repository := NewModelCatalogRepository(database.GORM)
	_, status, _, err := repository.Statuses(context.Background(), []int64{pending.ID})
	if err != nil || status != "pending" {
		t.Fatalf("pending status=%q err=%v", status, err)
	}
	_, status, _, err = repository.Statuses(context.Background(), []int64{unavailable.ID})
	if err != nil || status != "unavailable" {
		t.Fatalf("unavailable status=%q err=%v", status, err)
	}
	_, status, _, err = repository.Statuses(context.Background(), []int64{pending.ID, unavailable.ID})
	if err != nil || status != "partial" {
		t.Fatalf("mixed status=%q err=%v", status, err)
	}
}

func TestSyncModelCatalogUsesBoundedQueriesAndCountsDeletes(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_101_301_000)
	site := createRunnableSite(t, database, fmt.Sprintf("model-batch-%d", time.Now().UnixNano()), now)
	cleanupModelCatalogTestRows(t, database, site.ID)

	const itemCount = 1_001
	items := make([]dto.UpstreamModelMeta, 0, itemCount)
	for id := int64(itemCount); id >= 1; id-- {
		items = append(items, dto.UpstreamModelMeta{
			ID: id, ModelName: fmt.Sprintf("model-%04d", id), VendorID: 7,
			Status: 1, SyncOfficial: 1, NameRule: 0, CreatedTime: 1, UpdatedTime: 2,
		})
	}
	snapshot := dto.UpstreamModelMetaSnapshot{Total: itemCount, MaxID: itemCount, Items: items}
	counted, counter := newTestSQLCountingDB(database.GORM)
	written, err := NewSiteRepository(counted).SyncModelCatalog(context.Background(), site, now, snapshot)
	if err != nil || written != itemCount {
		t.Fatalf("initial sync written=%d err=%v", written, err)
	}
	counts := counter.snapshot()
	if counts.Query != 1 || counts.Create > 4 {
		t.Fatalf("initial sync SQL counts=%+v, want one read and bounded batch writes", counts)
	}

	trimmed := dto.UpstreamModelMetaSnapshot{Total: 1, MaxID: items[0].ID, Items: items[:1]}
	written, err = NewSiteRepository(database.GORM).SyncModelCatalog(context.Background(), site, now+1, trimmed)
	if err != nil || written != itemCount-1 {
		t.Fatalf("trimmed sync written=%d err=%v", written, err)
	}
	var remaining int64
	if err := database.GORM.Model(&SiteModelMeta{}).Where("site_id = ?", site.ID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("remaining model catalog rows=%d err=%v", remaining, err)
	}
}
