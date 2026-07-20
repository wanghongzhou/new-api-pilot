package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func cleanupSubscriptionPlanTestRows(t *testing.T, database *Database, siteID int64) {
	t.Helper()
	t.Cleanup(func() {
		_ = database.GORM.Where("site_id = ?", siteID).Delete(&SiteSubscriptionPlan{}).Error
		_ = database.GORM.Where("site_id = ?", siteID).Delete(&SiteSubscriptionPlanCollectionState{}).Error
	})
}

func subscriptionPlanTestItem(id int64) dto.UpstreamSubscriptionPlan {
	return dto.UpstreamSubscriptionPlan{
		ID: id, Title: fmt.Sprintf("Plan %d", id), Subtitle: "safe", PriceAmount: "19.99",
		Currency: "USD", DurationUnit: "month", DurationValue: 1, Enabled: true,
		TotalAmount: 9_007_199_254_740_993, QuotaResetPeriod: "monthly", CreatedAt: 1, UpdatedAt: 2,
	}
}

func TestSyncSubscriptionPlansUsesBoundedBatchSQLAtLargeCardinality(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_101_400_000)
	site := createRunnableSite(t, database, fmt.Sprintf("plan-batch-%d", time.Now().UnixNano()), now)
	cleanupSubscriptionPlanTestRows(t, database, site.ID)

	const itemCount = 1_001
	items := make([]dto.UpstreamSubscriptionPlan, 0, itemCount)
	for id := int64(1); id <= itemCount; id++ {
		items = append(items, subscriptionPlanTestItem(id))
	}
	counted, counter := newTestSQLCountingDB(database.GORM)
	written, err := NewSiteRepository(counted).SyncSubscriptionPlans(context.Background(), site, now, dto.UpstreamSubscriptionPlanSnapshot{Items: items})
	if err != nil || written != itemCount {
		t.Fatalf("initial plan sync written=%d err=%v", written, err)
	}
	counts := counter.snapshot()
	if counts.Query != 1 || counts.Create > 4 || counts.Update != 0 {
		t.Fatalf("initial plan sync SQL counts=%+v, want one read and bounded batch writes", counts)
	}

	secondCounted, secondCounter := newTestSQLCountingDB(database.GORM)
	written, err = NewSiteRepository(secondCounted).SyncSubscriptionPlans(context.Background(), site, now+1, dto.UpstreamSubscriptionPlanSnapshot{Items: items[:1]})
	if err != nil || written != itemCount-1 {
		t.Fatalf("missing transition written=%d err=%v", written, err)
	}
	secondCounts := secondCounter.snapshot()
	if secondCounts.Query != 1 || secondCounts.Update > 2 || secondCounts.Create > 2 {
		t.Fatalf("trimmed plan sync SQL counts=%+v, want bounded missing/update batches", secondCounts)
	}
	var missing int64
	if err := database.GORM.Model(&SiteSubscriptionPlan{}).Where("site_id = ? AND remote_state = 'missing'", site.ID).Count(&missing).Error; err != nil || missing != itemCount-1 {
		t.Fatalf("missing plans=%d err=%v", missing, err)
	}
}
