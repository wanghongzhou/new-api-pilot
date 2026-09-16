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

func TestFinanceUnchangedSnapshotsDoNotRewriteFacts(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100900500)
	site := createRunnableSite(t, db, fmt.Sprintf("finance-unchanged-%d", time.Now().UnixNano()), now)
	topups := dto.UpstreamTopupSnapshot{Total: 1, MaxID: 1, Items: []dto.UpstreamTopup{{
		ID: 1, UserID: 7, Amount: 10, Money: "1", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now - 10, Status: "success",
	}}}
	redemptions := dto.UpstreamRedemptionSnapshot{Total: 1, MaxID: 1, Items: []dto.UpstreamRedemption{{
		ID: 1, UserID: 7, Name: "stable", Status: 1, Quota: 10, CreatedTime: now - 10,
	}}}
	repository := NewSiteRepository(db.GORM)
	if _, err := repository.SyncTopups(context.Background(), site, now, topups); err != nil {
		t.Fatalf("seed topups: %v", err)
	}
	if _, err := repository.SyncRedemptions(context.Background(), site, now, redemptions); err != nil {
		t.Fatalf("seed redemptions: %v", err)
	}
	var originalTopup SiteTopupOrder
	var originalRedemption SiteRedemption
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&originalTopup).Error; err != nil {
		t.Fatalf("read original topup: %v", err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&originalRedemption).Error; err != nil {
		t.Fatalf("read original redemption: %v", err)
	}
	if written, err := repository.SyncTopups(context.Background(), site, now+60, topups); err != nil || written != 0 {
		t.Fatalf("unchanged topup written=%d err=%v", written, err)
	}
	if written, err := repository.SyncRedemptions(context.Background(), site, now+60, redemptions); err != nil || written != 0 {
		t.Fatalf("unchanged redemption written=%d err=%v", written, err)
	}
	var currentTopup SiteTopupOrder
	var currentRedemption SiteRedemption
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&currentTopup).Error; err != nil || currentTopup.UpdatedAt != originalTopup.UpdatedAt || currentTopup.CollectedAt != originalTopup.CollectedAt {
		t.Fatalf("unchanged topup current=%+v original=%+v err=%v", currentTopup, originalTopup, err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&currentRedemption).Error; err != nil || currentRedemption.UpdatedAt != originalRedemption.UpdatedAt || currentRedemption.CollectedAt != originalRedemption.CollectedAt {
		t.Fatalf("unchanged redemption current=%+v original=%+v err=%v", currentRedemption, originalRedemption, err)
	}
}

func TestFinanceIncrementalSnapshotsOnlyTouchReturnedIDsAndFullSnapshotsCalibrateMissing(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100900750)
	site := createRunnableSite(t, db, fmt.Sprintf("finance-incremental-%d", time.Now().UnixNano()), now)
	repository := NewSiteRepository(db.GORM)
	fullTopups := dto.UpstreamTopupSnapshot{Total: 2, MaxID: 2, Items: []dto.UpstreamTopup{
		{ID: 2, UserID: 2, Amount: 20, Money: "2", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now - 20, Status: "success"},
		{ID: 1, UserID: 1, Amount: 10, Money: "1", PaymentMethod: "balance", PaymentProvider: "balance", CreateTime: now - 30, Status: "success"},
	}}
	fullRedemptions := dto.UpstreamRedemptionSnapshot{Total: 2, MaxID: 2, Items: []dto.UpstreamRedemption{
		{ID: 2, UserID: 2, Name: "two", Status: 1, Quota: 20, CreatedTime: now - 20},
		{ID: 1, UserID: 1, Name: "one", Status: 1, Quota: 10, CreatedTime: now - 30},
	}}
	if _, err := repository.SyncTopups(context.Background(), site, now, fullTopups); err != nil {
		t.Fatalf("seed topups: %v", err)
	}
	if _, err := repository.SyncRedemptions(context.Background(), site, now, fullRedemptions); err != nil {
		t.Fatalf("seed redemptions: %v", err)
	}
	var oldTopup SiteTopupOrder
	var oldRedemption SiteRedemption
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&oldTopup).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&oldRedemption).Error; err != nil {
		t.Fatal(err)
	}
	incrementalTopups := dto.UpstreamTopupSnapshot{Total: 3, MaxID: 3, Incremental: true, Items: []dto.UpstreamTopup{
		{ID: 3, UserID: 3, Amount: 30, Money: "3", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now + 1, Status: "success"},
		fullTopups.Items[0],
	}}
	incrementalRedemptions := dto.UpstreamRedemptionSnapshot{Total: 3, MaxID: 3, Incremental: true, Items: []dto.UpstreamRedemption{
		{ID: 3, UserID: 3, Name: "three", Status: 1, Quota: 30, CreatedTime: now + 1},
		fullRedemptions.Items[0],
	}}
	if written, err := repository.SyncTopups(context.Background(), site, now+60, incrementalTopups); err != nil || written != 1 {
		t.Fatalf("incremental topups written=%d err=%v", written, err)
	}
	if written, err := repository.SyncRedemptions(context.Background(), site, now+60, incrementalRedemptions); err != nil || written != 1 {
		t.Fatalf("incremental redemptions written=%d err=%v", written, err)
	}
	var untouchedTopup SiteTopupOrder
	var untouchedRedemption SiteRedemption
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&untouchedTopup).Error; err != nil || untouchedTopup.RemoteState != financeStateNormal || untouchedTopup.UpdatedAt != oldTopup.UpdatedAt {
		t.Fatalf("incremental sync touched omitted topup: %+v err=%v", untouchedTopup, err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&untouchedRedemption).Error; err != nil || untouchedRedemption.RemoteState != financeStateNormal || untouchedRedemption.UpdatedAt != oldRedemption.UpdatedAt {
		t.Fatalf("incremental sync touched omitted redemption: %+v err=%v", untouchedRedemption, err)
	}
	if written, err := repository.SyncTopups(context.Background(), site, now+120, incrementalTopups); err != nil || written != 0 {
		t.Fatalf("unchanged incremental topups written=%d err=%v", written, err)
	}
	if written, err := repository.SyncRedemptions(context.Background(), site, now+120, incrementalRedemptions); err != nil || written != 0 {
		t.Fatalf("unchanged incremental redemptions written=%d err=%v", written, err)
	}
	topupCheckpoint, err := repository.TopupCollectionCheckpoint(context.Background(), site.ID)
	if err != nil || topupCheckpoint.LastFullSuccessAt == nil || *topupCheckpoint.LastFullSuccessAt != now || topupCheckpoint.ObservedTotal != 3 || topupCheckpoint.ObservedMaxID != 3 {
		t.Fatalf("topup checkpoint after incremental=%+v err=%v", topupCheckpoint, err)
	}
	redemptionCheckpoint, err := repository.RedemptionCollectionCheckpoint(context.Background(), site.ID)
	if err != nil || redemptionCheckpoint.LastFullSuccessAt == nil || *redemptionCheckpoint.LastFullSuccessAt != now || redemptionCheckpoint.ObservedTotal != 3 || redemptionCheckpoint.ObservedMaxID != 3 {
		t.Fatalf("redemption checkpoint after incremental=%+v err=%v", redemptionCheckpoint, err)
	}
	calibrationTopups := dto.UpstreamTopupSnapshot{Total: 2, MaxID: 3, Items: incrementalTopups.Items[:1]}
	calibrationTopups.Items = append(calibrationTopups.Items, fullTopups.Items[0])
	calibrationRedemptions := dto.UpstreamRedemptionSnapshot{Total: 2, MaxID: 3, Items: incrementalRedemptions.Items[:1]}
	calibrationRedemptions.Items = append(calibrationRedemptions.Items, fullRedemptions.Items[0])
	if _, err := repository.SyncTopups(context.Background(), site, now+86400, calibrationTopups); err != nil {
		t.Fatalf("calibrate topups: %v", err)
	}
	if _, err := repository.SyncRedemptions(context.Background(), site, now+86400, calibrationRedemptions); err != nil {
		t.Fatalf("calibrate redemptions: %v", err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&untouchedTopup).Error; err != nil || untouchedTopup.RemoteState != financeStateMissing {
		t.Fatalf("full calibration did not mark topup missing: %+v err=%v", untouchedTopup, err)
	}
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&untouchedRedemption).Error; err != nil || untouchedRedemption.RemoteState != financeStateMissing {
		t.Fatalf("full calibration did not mark redemption missing: %+v err=%v", untouchedRedemption, err)
	}
}

func TestFinanceMutableLookupsAndExactRedemptionMissingProof(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100900900)
	site := createRunnableSite(t, db, fmt.Sprintf("finance-mutable-%d", time.Now().UnixNano()), now)
	repository := NewSiteRepository(db.GORM)
	topups := dto.UpstreamTopupSnapshot{Total: 3, MaxID: 3, Items: []dto.UpstreamTopup{
		{ID: 3, UserID: 1, Amount: 1, Money: "1", Status: "success"},
		{ID: 2, UserID: 1, Amount: 1, Money: "1", Status: "pending"},
		{ID: 1, UserID: 1, Amount: 1, Money: "1", Status: "pending"},
	}}
	redemptions := dto.UpstreamRedemptionSnapshot{Total: 3, MaxID: 3, Items: []dto.UpstreamRedemption{
		{ID: 3, Status: 3, Name: "used", Quota: 1},
		{ID: 2, Status: 1, Name: "enabled-two", Quota: 1},
		{ID: 1, Status: 1, Name: "enabled-one", Quota: 1},
	}}
	if _, err := repository.SyncTopups(context.Background(), site, now, topups); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncRedemptions(context.Background(), site, now, redemptions); err != nil {
		t.Fatal(err)
	}
	oldest, err := repository.OldestPendingTopupID(context.Background(), site.ID)
	if err != nil || oldest != 1 {
		t.Fatalf("oldest pending=%d err=%v", oldest, err)
	}
	enabled, err := repository.EnabledRedemptionIDs(context.Background(), site.ID)
	if err != nil || len(enabled) != 2 || enabled[0] != 2 || enabled[1] != 1 {
		t.Fatalf("enabled redemption ids=%v err=%v", enabled, err)
	}
	incremental := dto.UpstreamRedemptionSnapshot{Total: 3, MaxID: 3, Incremental: true,
		Items: []dto.UpstreamRedemption{{ID: 3, Status: 3, Name: "used", Quota: 1}, {ID: 2, Status: 3, Name: "used-two", Quota: 1}}}
	written, err := repository.SyncRedemptions(context.Background(), site, now+60, incremental)
	if err != nil || written != 1 {
		t.Fatalf("incremental exact changes written=%d err=%v", written, err)
	}
	var changed SiteRedemption
	if err := db.GORM.Where("site_id=? AND remote_id=2", site.ID).Take(&changed).Error; err != nil || changed.RemoteStatus != 3 {
		t.Fatalf("changed redemption=%+v err=%v", changed, err)
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
	q.Statuses = []string{"expired"}
	q.StatusAt = now
	rows, total, err := repo.ListRedemptions(context.Background(), q)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("expired filter rows=%+v total=%d err=%v", rows, total, err)
	}
}
