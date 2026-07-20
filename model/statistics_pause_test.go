package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

func TestStatisticsPauseDateBoundaryUsesBeijingDay(t *testing.T) {
	pauseAt := time.Date(2026, 7, 14, 12, 34, 56, 0, usageAggregationLocation).Unix()
	dateKey, dateStart := statisticsPauseDateBoundary(pauseAt)
	if dateKey != 20260714 {
		t.Fatalf("date key = %d, want 20260714", dateKey)
	}
	wantStart := time.Date(2026, 7, 14, 0, 0, 0, 0, usageAggregationLocation).Unix()
	if dateStart != wantStart {
		t.Fatalf("date start = %d, want %d", dateStart, wantStart)
	}
}

func TestIdentityMismatchPauseRebuildsOnlyAccountAndCustomerStatistics(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	pauseAt := time.Date(2040, 1, 2, 12, 0, 0, 0, usageAggregationLocation).Unix()
	preHour, postHour := pauseAt-3600, pauseAt
	nextHour := time.Date(2040, 1, 3, 1, 0, 0, 0, usageAggregationLocation).Unix()
	now := pauseAt + 1800
	dateKey, _ := statisticsPauseDateBoundary(pauseAt)
	nextDateKey, _ := statisticsPauseDateBoundary(nextHour)

	site := createStatisticsPauseTestSite(t, database, "identity", now)
	customer := createStatisticsPauseTestCustomer(t, database, "identity", now)
	otherCustomer := createStatisticsPauseTestCustomer(t, database, "identity-other", now)
	target := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 101, now)
	sibling := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 102, now)
	other := createStatisticsPauseTestAccount(t, database, site.ID, otherCustomer.ID, 103, now)
	registerStatisticsPauseCleanup(t, database, []int64{site.ID}, []int64{customer.ID, otherCustomer.ID},
		[]int64{target.ID, sibling.ID, other.ID}, []int64{preHour, postHour, nextHour}, []int{dateKey, nextDateKey})

	createStatisticsPauseAccountHourly(t, database, target.ID, preHour, 10, now)
	createStatisticsPauseAccountHourly(t, database, target.ID, postHour, 20, now)
	createStatisticsPauseAccountHourly(t, database, target.ID, nextHour, 30, now)
	createStatisticsPauseAccountHourly(t, database, sibling.ID, preHour, 1, now)
	createStatisticsPauseAccountHourly(t, database, sibling.ID, postHour, 2, now)
	createStatisticsPauseAccountHourly(t, database, sibling.ID, nextHour, 3, now)
	createStatisticsPauseAccountHourly(t, database, other.ID, preHour, 7, now)
	createStatisticsPauseAccountHourly(t, database, other.ID, postHour, 8, now)
	createStatisticsPauseAccountHourly(t, database, other.ID, nextHour, 9, now)
	createStatisticsPauseAccountDaily(t, database, target.ID, dateKey, 30, UsageAggregationStatusPartial, true, now)
	createStatisticsPauseAccountDaily(t, database, target.ID, nextDateKey, 30, UsageAggregationStatusComplete, true, now)
	createStatisticsPauseAccountDaily(t, database, sibling.ID, dateKey, 3, UsageAggregationStatusPartial, true, now)
	createStatisticsPauseAccountDaily(t, database, sibling.ID, nextDateKey, 3, UsageAggregationStatusComplete, true, now)
	createStatisticsPauseAccountDaily(t, database, other.ID, dateKey, 15, UsageAggregationStatusComplete, true, now)
	createStatisticsPauseAccountDaily(t, database, other.ID, nextDateKey, 9, UsageAggregationStatusComplete, true, now)

	for _, row := range []CustomerStatHourly{
		statisticsPauseCustomerHourly(customer.ID, site.ID, preHour, 11, 2, UsageAggregationStatusComplete, now),
		statisticsPauseCustomerHourly(customer.ID, site.ID, postHour, 22, 2, UsageAggregationStatusPartial, now),
		statisticsPauseCustomerHourly(customer.ID, site.ID, nextHour, 33, 2, UsageAggregationStatusComplete, now),
		statisticsPauseCustomerHourly(otherCustomer.ID, site.ID, preHour, 7, 1, UsageAggregationStatusComplete, now),
		statisticsPauseCustomerHourly(otherCustomer.ID, site.ID, postHour, 8, 1, UsageAggregationStatusComplete, now),
		statisticsPauseCustomerHourly(otherCustomer.ID, site.ID, nextHour, 9, 1, UsageAggregationStatusComplete, now),
	} {
		row := row
		if err := database.GORM.Create(&row).Error; err != nil {
			t.Fatalf("create customer hourly: %v", err)
		}
	}
	for _, row := range []CustomerStatDaily{
		statisticsPauseCustomerDaily(customer.ID, site.ID, dateKey, 33, 2, UsageAggregationStatusPartial, true, now),
		statisticsPauseCustomerDaily(customer.ID, site.ID, nextDateKey, 33, 2, UsageAggregationStatusComplete, true, now),
		statisticsPauseCustomerDaily(otherCustomer.ID, site.ID, dateKey, 15, 1, UsageAggregationStatusComplete, true, now),
		statisticsPauseCustomerDaily(otherCustomer.ID, site.ID, nextDateKey, 9, 1, UsageAggregationStatusComplete, true, now),
	} {
		row := row
		if err := database.GORM.Create(&row).Error; err != nil {
			t.Fatalf("create customer daily: %v", err)
		}
	}
	siteStat := SiteStatHourly{SiteID: site.ID, HourTS: postHour, RequestCount: 999, Quota: 999, TokenUsed: 999,
		ActiveUsers: 3, DataStatus: UsageAggregationStatusComplete, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
	globalStat := GlobalStatHourly{HourTS: postHour, RequestCount: 888, Quota: 888, TokenUsed: 888,
		ActiveUsers: 3, DataStatus: UsageAggregationStatusComplete, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&siteStat).Error; err != nil {
		t.Fatalf("create site statistic: %v", err)
	}
	if err := database.GORM.Create(&globalStat).Error; err != nil {
		t.Fatalf("create global statistic: %v", err)
	}
	for _, fact := range []UsageFactHourly{
		{SiteID: site.ID, RemoteUserID: target.RemoteUserID, UsernameSnapshot: target.Username, ModelName: "pause-model", ChannelID: 1,
			HourTS: postHour, RequestCount: 20, Quota: 20, TokenUsed: 20, CollectedAt: now},
		{SiteID: site.ID, RemoteUserID: sibling.RemoteUserID, UsernameSnapshot: sibling.Username, ModelName: "pause-model", ChannelID: 1,
			HourTS: postHour, RequestCount: 2, Quota: 2, TokenUsed: 2, CollectedAt: now},
	} {
		fact := fact
		if err := database.GORM.Create(&fact).Error; err != nil {
			t.Fatalf("create retained usage fact: %v", err)
		}
	}
	window := CollectionWindow{SiteID: site.ID, HourTS: postHour, Status: CollectionWindowStatusComplete, FetchedRows: 2, UpdatedAt: now}
	if err := database.GORM.Create(&window).Error; err != nil {
		t.Fatalf("create complete collection window: %v", err)
	}

	committed, applied, err := NewAccountRepository(database.GORM).MarkIdentityMismatch(ctx, target.ID, now, pauseAt, now)
	if err != nil || !applied || committed.RemoteState != AccountRemoteStateIdentityMismatch || committed.StatisticsPausedAt == nil ||
		*committed.StatisticsPausedAt != pauseAt {
		t.Fatalf("mark identity mismatch = %#v applied=%t err=%v", committed, applied, err)
	}
	assertStatisticsPauseAccountHourly(t, database, target.ID, map[int64]int64{preHour: 10})
	assertStatisticsPauseAccountDaily(t, database, target.ID, map[int]statisticsPauseDailyWant{
		dateKey: {RequestCount: 10, DataStatus: UsageAggregationStatusPartial, IsFinal: true},
	})
	assertStatisticsPauseAccountHourly(t, database, sibling.ID, map[int64]int64{preHour: 1, postHour: 2, nextHour: 3})
	assertStatisticsPauseAccountDaily(t, database, sibling.ID, map[int]statisticsPauseDailyWant{
		dateKey:     {RequestCount: 3, DataStatus: UsageAggregationStatusPartial, IsFinal: true},
		nextDateKey: {RequestCount: 3, DataStatus: UsageAggregationStatusComplete, IsFinal: true},
	})
	assertStatisticsPauseCustomerHourly(t, database, customer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
		preHour:  {RequestCount: 11, ActiveUsers: 2},
		postHour: {RequestCount: 2, ActiveUsers: 1, DataStatus: UsageAggregationStatusPartial},
		nextHour: {RequestCount: 3, ActiveUsers: 1},
	})
	assertStatisticsPauseCustomerDaily(t, database, customer.ID, site.ID, map[int]statisticsPauseCustomerDailyWant{
		dateKey:     {RequestCount: 13, ActiveUsers: 2, DataStatus: UsageAggregationStatusPartial, IsFinal: true},
		nextDateKey: {RequestCount: 3, ActiveUsers: 1, DataStatus: UsageAggregationStatusComplete, IsFinal: true},
	})
	assertStatisticsPauseCustomerHourly(t, database, otherCustomer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
		preHour: {RequestCount: 7, ActiveUsers: 1}, postHour: {RequestCount: 8, ActiveUsers: 1}, nextHour: {RequestCount: 9, ActiveUsers: 1},
	})
	var unchangedSite SiteStatHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", site.ID, postHour).Take(&unchangedSite).Error; err != nil || unchangedSite.RequestCount != 999 {
		t.Fatalf("site statistic changed by account pause: %#v err=%v", unchangedSite, err)
	}
	var unchangedGlobal GlobalStatHourly
	if err := database.GORM.Where("hour_ts = ?", postHour).Take(&unchangedGlobal).Error; err != nil || unchangedGlobal.RequestCount != 888 {
		t.Fatalf("global statistic changed by account pause: %#v err=%v", unchangedGlobal, err)
	}
	var retainedFacts int64
	if err := database.GORM.Model(&UsageFactHourly{}).Where("site_id = ? AND hour_ts = ?", site.ID, postHour).Count(&retainedFacts).Error; err != nil || retainedFacts != 2 {
		t.Fatalf("retained facts count = %d err=%v", retainedFacts, err)
	}

	err = database.GORM.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, rebuildErr := rebuildUsageHourly(ctx, tx, site.ID, postHour, now+1,
			usageMetricAggregate{RequestCount: 22, Quota: 22, TokenUsed: 22, ActiveUsers: 2},
			usageCoverage{Expected: 1, Complete: 1}, usageAggregationRebuildOptions{})
		return rebuildErr
	})
	if err != nil {
		t.Fatalf("ordinary hourly rebuild: %v", err)
	}
	assertStatisticsPauseAccountHourly(t, database, target.ID, map[int64]int64{preHour: 10})
	assertStatisticsPauseCustomerHourly(t, database, customer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
		preHour: {RequestCount: 11, ActiveUsers: 2}, postHour: {RequestCount: 2, ActiveUsers: 1}, nextHour: {RequestCount: 3, ActiveUsers: 1},
	})
}

func TestApplySiteUserSnapshotIdentityMismatchUsesPauseCleanup(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	pauseAt := time.Date(2040, 2, 2, 10, 0, 0, 0, usageAggregationLocation).Unix()
	now := pauseAt + 1200
	dateKey, _ := statisticsPauseDateBoundary(pauseAt)
	site := createStatisticsPauseTestSite(t, database, "snapshot", now)
	customer := createStatisticsPauseTestCustomer(t, database, "snapshot", now)
	account := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 201, now)
	registerStatisticsPauseCleanup(t, database, []int64{site.ID}, []int64{customer.ID}, []int64{account.ID}, []int64{pauseAt}, []int{dateKey})
	createStatisticsPauseAccountHourly(t, database, account.ID, pauseAt, 20, now)
	createStatisticsPauseAccountDaily(t, database, account.ID, dateKey, 20, UsageAggregationStatusComplete, false, now)
	hourly := statisticsPauseCustomerHourly(customer.ID, site.ID, pauseAt, 20, 1, UsageAggregationStatusComplete, now)
	daily := statisticsPauseCustomerDaily(customer.ID, site.ID, dateKey, 20, 1, UsageAggregationStatusComplete, false, now)
	if err := database.GORM.Create(&hourly).Error; err != nil {
		t.Fatalf("create snapshot customer hourly: %v", err)
	}
	if err := database.GORM.Create(&daily).Error; err != nil {
		t.Fatalf("create snapshot customer daily: %v", err)
	}
	updated, err := NewCollectionTaskRepository(database.GORM).ApplySiteUserSnapshot(ctx, site, now, pauseAt, []SiteUserObservation{{
		RemoteUserID: account.RemoteUserID, RemoteCreatedAt: account.RemoteCreatedAt + 1,
		Username: "reused-id", RemoteStatus: 1,
	}})
	if err != nil || updated != 3 {
		t.Fatalf("apply identity mismatch snapshot updated=%d err=%v", updated, err)
	}
	loaded, err := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
	if err != nil || loaded.RemoteState != AccountRemoteStateIdentityMismatch || loaded.StatisticsPausedAt == nil || *loaded.StatisticsPausedAt != pauseAt {
		t.Fatalf("snapshot identity account = %#v err=%v", loaded, err)
	}
	assertStatisticsPauseAccountHourly(t, database, account.ID, map[int64]int64{})
	assertStatisticsPauseAccountDaily(t, database, account.ID, map[int]statisticsPauseDailyWant{})
	assertStatisticsPauseCustomerHourly(t, database, customer.ID, site.ID, map[int64]statisticsPauseCustomerWant{})
	assertStatisticsPauseCustomerDaily(t, database, customer.ID, site.ID, map[int]statisticsPauseCustomerDailyWant{})
}

func TestArchiveAndCustomerDisableSharePauseCleanupWithoutCrossCustomerDamage(t *testing.T) {
	t.Run("archive", func(t *testing.T) {
		database := openLockedSiteRunDatabase(t)
		ctx := context.Background()
		pauseAt := time.Date(2040, 3, 2, 9, 0, 0, 0, usageAggregationLocation).Unix()
		preHour, postHour := pauseAt-3600, pauseAt
		now := pauseAt + 600
		dateKey, _ := statisticsPauseDateBoundary(pauseAt)
		site := createStatisticsPauseTestSite(t, database, "archive", now)
		customer := createStatisticsPauseTestCustomer(t, database, "archive", now)
		account := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 301, now)
		registerStatisticsPauseCleanup(t, database, []int64{site.ID}, []int64{customer.ID}, []int64{account.ID}, []int64{preHour, postHour}, []int{dateKey})
		createStatisticsPauseAccountHourly(t, database, account.ID, preHour, 4, now)
		createStatisticsPauseAccountHourly(t, database, account.ID, postHour, 6, now)
		createStatisticsPauseAccountDaily(t, database, account.ID, dateKey, 10, UsageAggregationStatusPartial, false, now)
		for _, row := range []any{
			&CustomerStatHourly{CustomerID: customer.ID, SiteID: site.ID, HourTS: preHour, RequestCount: 4, Quota: 4, TokenUsed: 4, ActiveUsers: 1, DataStatus: UsageAggregationStatusComplete, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
			&CustomerStatHourly{CustomerID: customer.ID, SiteID: site.ID, HourTS: postHour, RequestCount: 6, Quota: 6, TokenUsed: 6, ActiveUsers: 1, DataStatus: UsageAggregationStatusComplete, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
			&CustomerStatDaily{CustomerID: customer.ID, SiteID: site.ID, DateKey: dateKey, RequestCount: 10, Quota: 10, TokenUsed: 10, ActiveUsers: 1, DataStatus: UsageAggregationStatusPartial, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		} {
			if err := database.GORM.Create(row).Error; err != nil {
				t.Fatalf("create archive customer statistic: %v", err)
			}
		}
		if err := NewAccountRepository(database.GORM).Archive(ctx, account.ID, pauseAt, now); err != nil {
			t.Fatalf("archive account: %v", err)
		}
		assertStatisticsPauseAccountHourly(t, database, account.ID, map[int64]int64{preHour: 4})
		assertStatisticsPauseAccountDaily(t, database, account.ID, map[int]statisticsPauseDailyWant{
			dateKey: {RequestCount: 4, DataStatus: UsageAggregationStatusPartial},
		})
		assertStatisticsPauseCustomerHourly(t, database, customer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
			preHour: {RequestCount: 4, ActiveUsers: 1},
		})
	})

	t.Run("customer disable", func(t *testing.T) {
		database := openLockedSiteRunDatabase(t)
		ctx := context.Background()
		pauseAt := time.Date(2040, 4, 2, 9, 0, 0, 0, usageAggregationLocation).Unix()
		preHour, postHour := pauseAt-3600, pauseAt
		now := pauseAt + 600
		dateKey, _ := statisticsPauseDateBoundary(pauseAt)
		site := createStatisticsPauseTestSite(t, database, "customer-disable", now)
		customer := createStatisticsPauseTestCustomer(t, database, "customer-disable", now)
		otherCustomer := createStatisticsPauseTestCustomer(t, database, "customer-disable-other", now)
		first := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 401, now)
		second := createStatisticsPauseTestAccount(t, database, site.ID, customer.ID, 402, now)
		other := createStatisticsPauseTestAccount(t, database, site.ID, otherCustomer.ID, 403, now)
		registerStatisticsPauseCleanup(t, database, []int64{site.ID}, []int64{customer.ID, otherCustomer.ID},
			[]int64{first.ID, second.ID, other.ID}, []int64{preHour, postHour}, []int{dateKey})
		for _, item := range []struct {
			accountID int64
			pre, post int64
		}{{first.ID, 4, 6}, {second.ID, 5, 7}, {other.ID, 8, 9}} {
			createStatisticsPauseAccountHourly(t, database, item.accountID, preHour, item.pre, now)
			createStatisticsPauseAccountHourly(t, database, item.accountID, postHour, item.post, now)
			createStatisticsPauseAccountDaily(t, database, item.accountID, dateKey, item.pre+item.post, UsageAggregationStatusComplete, false, now)
		}
		for _, row := range []CustomerStatHourly{
			statisticsPauseCustomerHourly(customer.ID, site.ID, preHour, 9, 2, UsageAggregationStatusComplete, now),
			statisticsPauseCustomerHourly(customer.ID, site.ID, postHour, 13, 2, UsageAggregationStatusComplete, now),
			statisticsPauseCustomerHourly(otherCustomer.ID, site.ID, preHour, 8, 1, UsageAggregationStatusComplete, now),
			statisticsPauseCustomerHourly(otherCustomer.ID, site.ID, postHour, 9, 1, UsageAggregationStatusComplete, now),
		} {
			row := row
			if err := database.GORM.Create(&row).Error; err != nil {
				t.Fatalf("create disable customer hourly: %v", err)
			}
		}
		for _, row := range []CustomerStatDaily{
			statisticsPauseCustomerDaily(customer.ID, site.ID, dateKey, 22, 2, UsageAggregationStatusPartial, false, now),
			statisticsPauseCustomerDaily(otherCustomer.ID, site.ID, dateKey, 17, 1, UsageAggregationStatusComplete, false, now),
		} {
			row := row
			if err := database.GORM.Create(&row).Error; err != nil {
				t.Fatalf("create disable customer daily: %v", err)
			}
		}
		if err := NewCustomerRepository(database.GORM).Disable(ctx, customer.ID, pauseAt, now); err != nil {
			t.Fatalf("disable customer: %v", err)
		}
		for _, accountID := range []int64{first.ID, second.ID} {
			assertStatisticsPauseAccountHourly(t, database, accountID, map[int64]int64{preHour: map[int64]int64{first.ID: 4, second.ID: 5}[accountID]})
		}
		assertStatisticsPauseCustomerHourly(t, database, customer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
			preHour: {RequestCount: 9, ActiveUsers: 2},
		})
		assertStatisticsPauseCustomerDaily(t, database, customer.ID, site.ID, map[int]statisticsPauseCustomerDailyWant{
			dateKey: {RequestCount: 9, ActiveUsers: 2, DataStatus: UsageAggregationStatusPartial},
		})
		assertStatisticsPauseAccountHourly(t, database, other.ID, map[int64]int64{preHour: 8, postHour: 9})
		assertStatisticsPauseCustomerHourly(t, database, otherCustomer.ID, site.ID, map[int64]statisticsPauseCustomerWant{
			preHour: {RequestCount: 8, ActiveUsers: 1}, postHour: {RequestCount: 9, ActiveUsers: 1},
		})
	})
}

type statisticsPauseDailyWant struct {
	RequestCount int64
	DataStatus   string
	IsFinal      bool
}

type statisticsPauseCustomerWant struct {
	RequestCount int64
	ActiveUsers  int64
	DataStatus   string
}

type statisticsPauseCustomerDailyWant struct {
	RequestCount int64
	ActiveUsers  int64
	DataStatus   string
	IsFinal      bool
}

func createStatisticsPauseTestSite(t *testing.T, database *Database, suffix string, now int64) Site {
	t.Helper()
	unique := fmt.Sprintf("%s-%d", suffix, time.Now().UnixNano())
	site := Site{
		Name: "Run run-pause-" + unique, BaseURL: "https://pause-" + unique + ".example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create pause site: %v", err)
	}
	return site
}

func createStatisticsPauseTestCustomer(t *testing.T, database *Database, suffix string, now int64) Customer {
	t.Helper()
	customer := Customer{
		Name:   "Run Customer pause " + suffix + fmt.Sprintf("-%d", time.Now().UnixNano()),
		Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create pause customer: %v", err)
	}
	return customer
}

func createStatisticsPauseTestAccount(t *testing.T, database *Database, siteID, customerID, remoteUserID, now int64) Account {
	t.Helper()
	account := Account{
		SiteID: siteID, CustomerID: customerID, RemoteUserID: remoteUserID, RemoteCreatedAt: now - 30*24*3600,
		Username: fmt.Sprintf("pause-user-%d", remoteUserID), RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create pause account: %v", err)
	}
	return account
}

func createStatisticsPauseAccountHourly(t *testing.T, database *Database, accountID, hourTS, value, now int64) {
	t.Helper()
	row := AccountStatHourly{AccountID: accountID, HourTS: hourTS, RequestCount: value, Quota: value, TokenUsed: value,
		DataStatus: UsageAggregationStatusComplete, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&row).Error; err != nil {
		t.Fatalf("create account hourly: %v", err)
	}
}

func createStatisticsPauseAccountDaily(
	t *testing.T, database *Database, accountID int64, dateKey int, value int64, status string, final bool, now int64,
) {
	t.Helper()
	row := AccountStatDaily{AccountID: accountID, DateKey: dateKey, RequestCount: value, Quota: value, TokenUsed: value,
		DataStatus: status, IsFinal: final, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&row).Error; err != nil {
		t.Fatalf("create account daily: %v", err)
	}
}

func statisticsPauseCustomerHourly(
	customerID, siteID, hourTS, value, activeUsers int64, status string, now int64,
) CustomerStatHourly {
	return CustomerStatHourly{CustomerID: customerID, SiteID: siteID, HourTS: hourTS,
		RequestCount: value, Quota: value, TokenUsed: value, ActiveUsers: activeUsers,
		DataStatus: status, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
}

func statisticsPauseCustomerDaily(
	customerID, siteID int64, dateKey int, value, activeUsers int64, status string, final bool, now int64,
) CustomerStatDaily {
	return CustomerStatDaily{CustomerID: customerID, SiteID: siteID, DateKey: dateKey,
		RequestCount: value, Quota: value, TokenUsed: value, ActiveUsers: activeUsers,
		DataStatus: status, IsFinal: final, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now}
}

func assertStatisticsPauseAccountHourly(t *testing.T, database *Database, accountID int64, want map[int64]int64) {
	t.Helper()
	var rows []AccountStatHourly
	if err := database.GORM.Where("account_id = ?", accountID).Order("hour_ts ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read account hourly: %v", err)
	}
	if len(rows) != len(want) {
		t.Fatalf("account %d hourly rows = %#v, want %#v", accountID, rows, want)
	}
	for _, row := range rows {
		if value, exists := want[row.HourTS]; !exists || row.RequestCount != value || row.Quota != value || row.TokenUsed != value {
			t.Fatalf("account %d hourly row = %#v, want %#v", accountID, row, want)
		}
	}
}

func assertStatisticsPauseAccountDaily(t *testing.T, database *Database, accountID int64, want map[int]statisticsPauseDailyWant) {
	t.Helper()
	var rows []AccountStatDaily
	if err := database.GORM.Where("account_id = ?", accountID).Order("date_key ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read account daily: %v", err)
	}
	if len(rows) != len(want) {
		t.Fatalf("account %d daily rows = %#v, want %#v", accountID, rows, want)
	}
	for _, row := range rows {
		expected, exists := want[row.DateKey]
		if !exists || row.RequestCount != expected.RequestCount || row.Quota != expected.RequestCount || row.TokenUsed != expected.RequestCount ||
			row.DataStatus != expected.DataStatus || row.IsFinal != expected.IsFinal {
			t.Fatalf("account %d daily row = %#v, want %#v", accountID, row, expected)
		}
	}
}

func assertStatisticsPauseCustomerHourly(
	t *testing.T, database *Database, customerID, siteID int64, want map[int64]statisticsPauseCustomerWant,
) {
	t.Helper()
	var rows []CustomerStatHourly
	if err := database.GORM.Where("customer_id = ? AND site_id = ?", customerID, siteID).Order("hour_ts ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read customer hourly: %v", err)
	}
	if len(rows) != len(want) {
		t.Fatalf("customer %d hourly rows = %#v, want %#v", customerID, rows, want)
	}
	for _, row := range rows {
		expected, exists := want[row.HourTS]
		status := expected.DataStatus
		if status == "" {
			status = UsageAggregationStatusComplete
		}
		if !exists || row.RequestCount != expected.RequestCount || row.Quota != expected.RequestCount || row.TokenUsed != expected.RequestCount ||
			row.ActiveUsers != expected.ActiveUsers || row.DataStatus != status {
			t.Fatalf("customer %d hourly row = %#v, want %#v", customerID, row, expected)
		}
	}
}

func assertStatisticsPauseCustomerDaily(
	t *testing.T, database *Database, customerID, siteID int64, want map[int]statisticsPauseCustomerDailyWant,
) {
	t.Helper()
	var rows []CustomerStatDaily
	if err := database.GORM.Where("customer_id = ? AND site_id = ?", customerID, siteID).Order("date_key ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read customer daily: %v", err)
	}
	if len(rows) != len(want) {
		t.Fatalf("customer %d daily rows = %#v, want %#v", customerID, rows, want)
	}
	for _, row := range rows {
		expected, exists := want[row.DateKey]
		status := expected.DataStatus
		if status == "" {
			status = UsageAggregationStatusComplete
		}
		if !exists || row.RequestCount != expected.RequestCount || row.Quota != expected.RequestCount || row.TokenUsed != expected.RequestCount ||
			row.ActiveUsers != expected.ActiveUsers || row.DataStatus != status || row.IsFinal != expected.IsFinal {
			t.Fatalf("customer %d daily row = %#v, want %#v", customerID, row, expected)
		}
	}
}

func registerStatisticsPauseCleanup(
	t *testing.T,
	database *Database,
	siteIDs, customerIDs, accountIDs, hours []int64,
	dateKeys []int,
) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if len(hours) > 0 {
			_ = database.GORM.WithContext(ctx).Where("hour_ts IN ?", hours).Delete(&GlobalStatHourly{}).Error
		}
		if len(dateKeys) > 0 {
			_ = database.GORM.WithContext(ctx).Where("date_key IN ?", dateKeys).Delete(&GlobalStatDaily{}).Error
		}
		if len(siteIDs) > 0 {
			for _, value := range []any{&ModelStatHourly{}, &ModelStatDaily{}, &ChannelStatHourly{}, &ChannelStatDaily{},
				&SiteStatHourly{}, &SiteStatDaily{}, &CustomerStatHourly{}, &CustomerStatDaily{}, &UsageFactHourly{}, &UsageFactDaily{},
				&CollectionWindow{}, &SiteUserInventoryHourly{}, &SiteUserInventory{}} {
				_ = database.GORM.WithContext(ctx).Where("site_id IN ?", siteIDs).Delete(value).Error
			}
		}
		if len(accountIDs) > 0 {
			_ = database.GORM.WithContext(ctx).Where("account_id IN ?", accountIDs).Delete(&AccountStatHourly{}).Error
			_ = database.GORM.WithContext(ctx).Where("account_id IN ?", accountIDs).Delete(&AccountStatDaily{}).Error
			_ = database.GORM.WithContext(ctx).Where("id IN ?", accountIDs).Delete(&Account{}).Error
		}
		if len(customerIDs) > 0 {
			_ = database.GORM.WithContext(ctx).Where("id IN ?", customerIDs).Delete(&Customer{}).Error
		}
		if len(siteIDs) > 0 {
			_ = database.GORM.WithContext(ctx).Where("site_id IN ?", siteIDs).Delete(&SiteCapability{}).Error
			_ = database.GORM.WithContext(ctx).Where("id IN ?", siteIDs).Delete(&Site{}).Error
		}
	})
}
