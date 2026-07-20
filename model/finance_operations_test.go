package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/dto"
)

func TestFinanceSnapshotsExactMissingAndAtomic(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100900000)
	site := createRunnableSite(t, db, fmt.Sprintf("finance-%d", time.Now().UnixNano()), now)
	topups := dto.UpstreamTopupSnapshot{Total: 2, MaxID: 2, Items: []dto.UpstreamTopup{{ID: 2, UserID: 7, Amount: 9007199254740993, Money: "123456789012345678.123456789", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now - 10, CompleteTime: now - 1, Status: "success"}, {ID: 1, UserID: 8, Amount: 1, Money: "0.1", PaymentMethod: "balance", PaymentProvider: "balance", CreateTime: now - 20, Status: "pending"}}}
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncTopups(context.Background(), site, now, topups)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var rows []SiteTopupOrder
	if err := db.GORM.Where("site_id=?", site.ID).Order("remote_id DESC").Find(&rows).Error; err != nil || len(rows) != 2 || rows[0].Money != "123456789012345678.1234567890" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	topups.Total = 1
	topups.MaxID = 2
	topups.Items = topups.Items[:1]
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncTopups(context.Background(), site, now+1, topups)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var missing SiteTopupOrder
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&missing).Error; err != nil || missing.RemoteState != financeStateMissing || missing.MissingCount != 1 {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	bad := topups
	bad.Total = 2
	bad.Items = append(bad.Items, bad.Items[0])
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncTopups(context.Background(), site, now+2, bad)
		return err
	}); err == nil {
		t.Fatal("duplicate snapshot accepted")
	}
	var preserved SiteTopupOrder
	_ = db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&preserved).Error
	if preserved.UpdatedAt != missing.UpdatedAt {
		t.Fatalf("failed snapshot partially committed: %+v", preserved)
	}
	wrongFence := topups
	wrongFence.MaxID = 99
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncTopups(context.Background(), site, now+3, wrongFence)
		return err
	}); err == nil {
		t.Fatal("snapshot with mismatched maximum id was accepted")
	}
	full := dto.UpstreamTopupSnapshot{Total: 2, MaxID: 2, Items: []dto.UpstreamTopup{
		{ID: 2, UserID: 7, Amount: 9007199254740993, Money: "1234567890123456789012345678.123456789", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now - 10, CompleteTime: now - 1, Status: "refunded"},
		{ID: 1, UserID: 8, Amount: 1, Money: "0.1", PaymentMethod: "balance", PaymentProvider: "balance", CreateTime: now - 20, Status: "pending"},
	}}
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncTopups(context.Background(), site, now+4, full)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var reappeared SiteTopupOrder
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&reappeared).Error; err != nil || reappeared.RemoteState != financeStateNormal || reappeared.MissingCount != 0 || reappeared.FirstSeenAt != now {
		t.Fatalf("reappeared=%+v err=%v", reappeared, err)
	}
	var changed SiteTopupOrder
	if err := db.GORM.Where("site_id=? AND remote_id=2", site.ID).Take(&changed).Error; err != nil || changed.RemoteStatus != "refunded" || changed.Money != "1234567890123456789012345678.1234567890" {
		t.Fatalf("changed topup=%+v err=%v", changed, err)
	}
	stale := site
	stale.ConfigVersion++
	if err := NewSiteRepository(db.GORM).MarkFinanceCollectionFailure(context.Background(), stale, now+5, "topup", "TEST_FAILURE"); !errors.Is(err, ErrSiteRunConfigChanged) {
		t.Fatalf("stale finance failure fence error=%v", err)
	}
}

func TestFinanceStatisticsNeverExposeCrossSiteTopupTotalsAndDeriveExpired(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100901000)
	site1 := createRunnableSite(t, db, fmt.Sprintf("finance-a-%d", time.Now().UnixNano()), now)
	site2 := createRunnableSite(t, db, fmt.Sprintf("finance-b-%d", time.Now().UnixNano()), now)
	for _, site := range []Site{site1, site2} {
		snapshot := dto.UpstreamTopupSnapshot{Total: 1, MaxID: 1, Items: []dto.UpstreamTopup{{ID: 1, UserID: 1, Amount: 100, Money: "10", PaymentProvider: "stripe", PaymentMethod: "stripe", CreateTime: now, Status: "success"}}}
		redemption := dto.UpstreamRedemptionSnapshot{Total: 1, MaxID: 1, Items: []dto.UpstreamRedemption{{ID: 1, Status: 1, Name: "batch", Quota: 100, CreatedTime: now - 10, ExpiredTime: now - 1}}}
		if err := db.GORM.Transaction(func(tx *gorm.DB) error {
			repo := NewSiteRepository(tx)
			if _, err := repo.SyncTopups(context.Background(), site, now, snapshot); err != nil {
				return err
			}
			_, err := repo.SyncRedemptions(context.Background(), site, now, redemption)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewFinanceRepository(db.GORM)
	q := dto.FinanceInventoryQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site1.ID, site2.ID}}
	summary, err := repo.TopupMetrics(context.Background(), q, "summary")
	if err != nil || len(summary) != 1 || summary[0].Count != 2 || summary[0].Amount != 0 || summary[0].Money != "0" {
		t.Fatalf("topup summary=%+v err=%v", summary, err)
	}
	providers, err := repo.TopupMetrics(context.Background(), q, "provider")
	if err != nil || len(providers) != 2 || providers[0].SiteID == 0 || providers[0].Amount != 100 {
		t.Fatalf("provider rows=%+v err=%v", providers, err)
	}
	statuses, err := repo.RedemptionMetrics(context.Background(), q, "status", now)
	if err != nil || len(statuses) == 0 || statuses[0].DimensionID != "expired" {
		t.Fatalf("redemption statuses=%+v err=%v", statuses, err)
	}
}
