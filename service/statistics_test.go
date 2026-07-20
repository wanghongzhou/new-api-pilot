package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const statisticsLargeQuota int64 = 9_007_199_254_740_993

type statisticsServiceFixture struct {
	service   *StatisticsService
	database  *gorm.DB
	start     int64
	monthEnd  int64
	sites     []model.Site
	customers []model.Customer
	accounts  []model.Account
}

func TestStatisticsServiceSixScopesNullZeroRatesAndDistinctActiveUsers(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	query := dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.start + 2*3600,
		Granularity: dto.StatisticsGranularityHour, Page: 1, PageSize: 100,
		SortBy: "bucket_start", SortOrder: "asc",
	}
	global, err := fixture.service.Global(context.Background(), query)
	if err != nil {
		t.Fatalf("query global statistics: %v", err)
	}
	if len(global.Trend) != 2 || stringValue(global.Trend[0].RequestCount) != "6" ||
		stringValue(global.Trend[0].Quota) != strconv.FormatInt(statisticsLargeQuota+40, 10) ||
		stringValue(global.Trend[0].ActiveUsers) != "3" || global.Trend[0].DataStatus != "complete" {
		t.Fatalf("global complete point = %#v", global.Trend)
	}
	partial := global.Trend[1]
	if stringValue(partial.RequestCount) != "0" || partial.DataStatus != "partial" || partial.Reason == nil ||
		partial.Reason.Code != constant.MessageDataPartialSites || partial.CompleteSiteCount != 1 || partial.ExpectedSiteCount != 2 {
		t.Fatalf("global partial zero point = %#v", partial)
	}
	if len(partial.SiteBreakdown) != 2 || stringValue(partial.SiteBreakdown[0].Quota) != "0" ||
		partial.SiteBreakdown[1].Quota != nil || partial.SiteBreakdown[1].DataStatus != "missing" {
		t.Fatalf("partial site breakdown = %#v", partial.SiteBreakdown)
	}
	if global.Completeness.CompleteUnitCount != 3 || global.Completeness.ExpectedUnitCount != 4 ||
		len(global.Completeness.MissingRanges) != 1 || global.Completeness.MissingSiteIDs[0] != strconv.FormatInt(fixture.sites[1].ID, 10) {
		t.Fatalf("global completeness = %#v", global.Completeness)
	}
	if len(global.Breakdown.Items) != 2 {
		t.Fatalf("global breakdown = %#v", global.Breakdown)
	}
	if _, ok := global.Breakdown.Items[0].(dto.GlobalStatisticsBreakdown); !ok {
		t.Fatalf("global breakdown type = %T", global.Breakdown.Items[0])
	}

	customerQuery := query
	customerQuery.CustomerIDs = []int64{fixture.customers[0].ID}
	customer, err := fixture.service.Customers(context.Background(), customerQuery)
	if err != nil || stringValue(customer.Trend[0].RequestCount) != "4" || stringValue(customer.Trend[0].ActiveUsers) != "2" {
		t.Fatalf("customer managed-only statistics = %#v, %v", customer, err)
	}
	if len(customer.Trend[0].SiteBreakdown) != 2 || stringValue(customer.Trend[0].SiteBreakdown[0].Quota) != "10" ||
		stringValue(customer.Trend[0].SiteBreakdown[1].Quota) != "30" {
		t.Fatalf("customer per-bucket site breakdown = %#v", customer.Trend[0].SiteBreakdown)
	}

	accountQuery := query
	accountQuery.AccountIDs = []int64{fixture.accounts[0].ID}
	account, err := fixture.service.Accounts(context.Background(), accountQuery)
	if err != nil || stringValue(account.Trend[0].RequestCount) != "1" ||
		stringValue(account.Trend[0].ActiveUsers) != "1" || stringValue(account.Trend[1].ActiveUsers) != "0" ||
		stringValue(account.Summary.ActiveUsers) != "1" {
		t.Fatalf("account statistics = %#v, %v", account, err)
	}
	missingAccountQuery := query
	missingAccountQuery.AccountIDs = []int64{fixture.accounts[1].ID}
	missingAccount, err := fixture.service.Accounts(context.Background(), missingAccountQuery)
	if err != nil || missingAccount.Trend[1].DataStatus != model.CollectionWindowStatusMissing ||
		missingAccount.Trend[1].ActiveUsers != nil {
		t.Fatalf("incomplete account active users = %#v, %v", missingAccount.Trend[1], err)
	}

	modelQuery := query
	modelQuery.ModelNames = []string{"Model-A", "model-a"}
	models, err := fixture.service.Models(context.Background(), modelQuery)
	if err != nil || stringValue(models.Trend[0].RequestCount) != "6" || models.Breakdown.Total != 8 {
		t.Fatalf("case-sensitive model statistics = %#v, %v", models, err)
	}

	channelQuery := query
	channelQuery.ChannelKeys = []string{fmt.Sprintf("%d:1", fixture.sites[0].ID)}
	channels, err := fixture.service.Channels(context.Background(), channelQuery)
	if err != nil || stringValue(channels.Trend[0].RequestCount) != "1" || len(channels.Trend[0].SiteBreakdown) != 1 {
		t.Fatalf("exact channel-key statistics = %#v, %v", channels, err)
	}
	unknownChannelQuery := query
	unknownChannelQuery.ChannelKeys = []string{fmt.Sprintf("%d:0", fixture.sites[0].ID)}
	unknownChannel, err := fixture.service.Channels(context.Background(), unknownChannelQuery)
	if err != nil || len(unknownChannel.Breakdown.Items) == 0 {
		t.Fatalf("unknown channel statistics = %#v, %v", unknownChannel.Breakdown, err)
	}
	unknownItem, ok := unknownChannel.Breakdown.Items[0].(dto.ChannelStatisticsBreakdown)
	if !ok || unknownItem.RemoteChannelID != "0" || unknownItem.RemoteMissing {
		t.Fatalf("unknown channel statistics = %#v, %v", unknownChannel.Breakdown, err)
	}

	monthQuery := dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.monthEnd,
		Granularity: dto.StatisticsGranularityMonth, Page: 1, PageSize: 100,
		SortBy: "quota", SortOrder: "desc",
	}
	month, err := fixture.service.Global(context.Background(), monthQuery)
	if err != nil || len(month.Trend) != 1 || stringValue(month.Trend[0].RequestCount) != "12" ||
		stringValue(month.Trend[0].ActiveUsers) != "3" {
		t.Fatalf("monthly distinct active statistics = %#v, %v", month, err)
	}
	if month.Trend[0].SiteBreakdown[0].RateSource != "site" ||
		month.Trend[0].SiteBreakdown[1].RateSource != "fallback" {
		t.Fatalf("monthly rate sources = %#v", month.Trend[0].SiteBreakdown)
	}

	newRate := "2.5000000000"
	if err := fixture.database.Model(&model.Site{}).Where("id = ?", fixture.sites[0].ID).
		Update("usd_exchange_rate", newRate).Error; err != nil {
		t.Fatalf("change current historical display rate: %v", err)
	}
	updated, err := fixture.service.Global(context.Background(), monthQuery)
	if err != nil || stringValue(updated.Trend[0].SiteBreakdown[0].USDExchangeRate) != newRate {
		t.Fatalf("updated current rate statistics = %#v, %v", updated, err)
	}

	missingCustomerQuery := query
	missingCustomerQuery.CustomerIDs = []int64{math.MaxInt64}
	missingCustomer, err := fixture.service.Customers(context.Background(), missingCustomerQuery)
	if err != nil || missingCustomer.Breakdown.Total != 0 || len(missingCustomer.SiteBreakdown) != 0 ||
		len(missingCustomer.Trend) != 2 || missingCustomer.Trend[0].RequestCount != nil ||
		missingCustomer.Summary.RequestCount != nil {
		t.Fatalf("missing customer statistics leaked data = %#v, %v", missingCustomer, err)
	}
}

func TestAccountStatisticsActiveUsersRequiresCompleteExpectedCoverage(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	query := func(accountID, start, end int64) dto.StatisticsResponse {
		t.Helper()
		response, err := fixture.service.Accounts(context.Background(), dto.StatisticsQuery{
			StartTimestamp: start, EndTimestamp: end, Granularity: dto.StatisticsGranularityDay,
			AccountIDs: []int64{accountID}, Page: 1, PageSize: 20,
			SortBy: "bucket_start", SortOrder: "asc",
		})
		if err != nil {
			t.Fatalf("query account %d statistics: %v", accountID, err)
		}
		if len(response.Trend) != 1 || len(response.Breakdown.Items) != 1 {
			t.Fatalf("account %d statistics shape = %#v", accountID, response)
		}
		return response
	}

	partial := query(fixture.accounts[1].ID, fixture.start, fixture.start+24*3600)
	partialItem, ok := partial.Breakdown.Items[0].(dto.AccountStatisticsBreakdown)
	if !ok || partial.Trend[0].DataStatus != model.UsageAggregationStatusPartial ||
		partial.Trend[0].ActiveUsers != nil || partial.Summary.ActiveUsers != nil ||
		partialItem.ActiveUsers != nil || stringValue(partial.Trend[0].RequestCount) != "3" ||
		stringValue(partial.Summary.Quota) != "30" || stringValue(partialItem.TokenUsed) != "300" {
		t.Fatalf("partial account statistics = %#v", partial)
	}

	completeStart := fixture.start + 24*3600
	completeEnd := completeStart + 24*3600
	active := query(fixture.accounts[0].ID, completeStart, completeEnd)
	activeItem := active.Breakdown.Items[0].(dto.AccountStatisticsBreakdown)
	if active.Trend[0].DataStatus != model.CollectionWindowStatusComplete ||
		stringValue(active.Trend[0].ActiveUsers) != "1" || stringValue(active.Summary.ActiveUsers) != "1" ||
		stringValue(activeItem.ActiveUsers) != "1" {
		t.Fatalf("complete active account statistics = %#v", active)
	}

	inactive := query(fixture.accounts[2].ID, completeStart, completeEnd)
	inactiveItem := inactive.Breakdown.Items[0].(dto.AccountStatisticsBreakdown)
	if inactive.Trend[0].DataStatus != model.CollectionWindowStatusComplete ||
		stringValue(inactive.Trend[0].ActiveUsers) != "0" || stringValue(inactive.Summary.ActiveUsers) != "0" ||
		stringValue(inactiveItem.ActiveUsers) != "0" {
		t.Fatalf("complete inactive account statistics = %#v", inactive)
	}
}

func TestStatisticsOptionsAndSiteConvenience(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	options := dto.StatisticsOptionQuery{SiteIDs: []int64{fixture.sites[0].ID}, Page: 1, PageSize: 20}
	models, err := fixture.service.ModelOptions(context.Background(), options)
	if err != nil || models.Total != 2 || len(models.Items) != 2 || models.Items[0].SiteID != strconv.FormatInt(fixture.sites[0].ID, 10) ||
		models.Items[0].Key == models.Items[1].Key || models.Items[0].ModelName == models.Items[1].ModelName {
		t.Fatalf("model options = %#v, %v", models, err)
	}
	channels, err := fixture.service.ChannelOptions(context.Background(), options)
	if err != nil || channels.Total != 2 || len(channels.Items) != 2 || channels.Items[0].RemoteChannelID != "0" ||
		channels.Items[0].Name != "未知通道" || channels.Items[0].RemoteMissing {
		t.Fatalf("channel options = %#v, %v", channels, err)
	}
	filtered := options
	filtered.Keyword = "Primary"
	channels, err = fixture.service.ChannelOptions(context.Background(), filtered)
	if err != nil || channels.Total != 1 || len(channels.Items) != 1 || channels.Items[0].RemoteChannelID != "1" {
		t.Fatalf("filtered channel options = %#v, %v", channels, err)
	}

	query := dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.start + 3600,
		Granularity: dto.StatisticsGranularityHour, Page: 1, PageSize: 20,
		SortBy: "bucket_start", SortOrder: "asc",
	}
	statistics, err := fixture.service.SiteStatistics(context.Background(), fixture.sites[0].ID, query)
	if err != nil || statistics.Scope != dto.StatisticsScopeSite || statistics.Breakdown.Total != 1 {
		t.Fatalf("site convenience statistics = %#v, %v", statistics, err)
	}
	if _, err := fixture.service.SiteStatistics(context.Background(), math.MaxInt64, query); !errors.Is(err, ErrStatisticsNotFound) {
		t.Fatalf("missing site statistics error = %v", err)
	}
	for _, siteID := range []int64{0, -1} {
		invalid := dto.StatisticsOptionQuery{SiteIDs: []int64{siteID}, Page: 1, PageSize: 20}
		if _, err := fixture.service.ModelOptions(context.Background(), invalid); !errors.Is(err, ErrStatisticsInvalid) {
			t.Fatalf("model options site ID %d error = %v", siteID, err)
		}
		if _, err := fixture.service.ChannelOptions(context.Background(), invalid); !errors.Is(err, ErrStatisticsInvalid) {
			t.Fatalf("channel options site ID %d error = %v", siteID, err)
		}
	}
}

func TestStatisticsActiveRunWindowStatusMatrix(t *testing.T) {
	database := openSiteTestTransaction(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2033, 1, 1, 0, 0, 0, 0, location).Unix()
	now := start + 8*3600 + 5*60
	statisticsStart := start
	site := model.Site{
		Name: "Statistics Active Window Matrix", BaseURL: "https://statistics-active-window.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true,
		StatisticsStartAt: &statisticsStart, CreatedAt: start, UpdatedAt: start,
	}
	if err := database.Create(&site).Error; err != nil {
		t.Fatalf("create active-window statistics site: %v", err)
	}
	verifiedAt := start + 24*3600
	windows := []model.CollectionWindow{
		{SiteID: site.ID, HourTS: start, Status: model.CollectionWindowStatusComplete, VerifiedAt: &verifiedAt, UpdatedAt: now},
		{SiteID: site.ID, HourTS: start + 3600, Status: model.CollectionWindowStatusUnavailable, UpdatedAt: now},
		{SiteID: site.ID, HourTS: start + 2*3600, Status: model.CollectionWindowStatusMissing, UpdatedAt: now},
		{SiteID: site.ID, HourTS: start + 3*3600, Status: model.CollectionWindowStatusPending, UpdatedAt: now},
	}
	if err := database.Create(&windows).Error; err != nil {
		t.Fatalf("create active-window fact states: %v", err)
	}
	createStatisticsActiveRun(t, database, site, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusPending,
		[]int64{start, start + 3600, start + 2*3600}, model.CollectionTaskStatusPending, now, "pending-backfill")
	createStatisticsActiveRun(t, database, site, constant.TaskTypeUsageHour, model.CollectionTaskStatusRunning,
		[]int64{start + 3*3600}, model.CollectionTaskStatusRunning, now, "running-hour")
	createStatisticsActiveRun(t, database, site, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusRunning,
		[]int64{start + 4*3600}, model.CollectionTaskStatusRunning, now, "running-backfill")
	createStatisticsActiveRun(t, database, site, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusPending,
		[]int64{start + 5*3600}, model.CollectionTaskStatusSuccess, now, "terminal-window")
	createStatisticsActiveRun(t, database, site, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusPending,
		[]int64{start + 6*3600}, model.CollectionTaskStatusPending, now, "before-deadline-backfill")

	statistics, err := NewStatisticsService(StatisticsServiceOptions{
		Database: database, Clock: testsupport.NewFakeClock(time.Unix(now, 0)),
	})
	if err != nil {
		t.Fatalf("create active-window statistics service: %v", err)
	}
	query := dto.StatisticsQuery{
		StartTimestamp: start, EndTimestamp: start + 8*3600, Granularity: dto.StatisticsGranularityHour,
		SiteIDs: []int64{site.ID}, Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	}
	response, err := statistics.Sites(context.Background(), query)
	want := []string{
		model.CollectionWindowStatusComplete,
		model.CollectionWindowStatusUnavailable,
		constant.SiteStatisticsBackfilling,
		model.CollectionWindowStatusPending,
		constant.SiteStatisticsBackfilling,
		model.CollectionWindowStatusMissing,
		constant.SiteStatisticsBackfilling,
		model.CollectionWindowStatusPending,
	}
	if err != nil || len(response.Trend) != len(want) {
		t.Fatalf("active-window matrix response = %#v, %v", response, err)
	}
	for index, status := range want {
		if response.Trend[index].DataStatus != status {
			t.Fatalf("active-window hour %d status = %s, want %s", index, response.Trend[index].DataStatus, status)
		}
	}
	assertStatisticsCorruptActiveWindowsIgnored(t, database, site, statistics, start+5*3600, now)

	disabledAt := start
	pausedSite := model.Site{
		Name: "Statistics Active Window Paused", BaseURL: "https://statistics-active-window-paused.example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementDisabled, OnlineStatus: constant.SiteOnlineOffline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsPaused,
		HealthStatus: constant.SiteHealthUnavailable, DataExportEnabled: true,
		StatisticsStartAt: &statisticsStart, DisabledAt: &disabledAt, CreatedAt: start, UpdatedAt: start,
	}
	if err := database.Create(&pausedSite).Error; err != nil {
		t.Fatalf("create paused active-window site: %v", err)
	}
	if err := database.Create(&model.CollectionWindow{
		SiteID: pausedSite.ID, HourTS: start, Status: model.CollectionWindowStatusMissing, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create paused active-window fact state: %v", err)
	}
	createStatisticsActiveRun(t, database, pausedSite, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusRunning,
		[]int64{start}, model.CollectionTaskStatusRunning, now, "paused-backfill")
	query.EndTimestamp = start + 3600
	query.SiteIDs = []int64{pausedSite.ID}
	response, err = statistics.Sites(context.Background(), query)
	if err != nil || len(response.Trend) != 1 || response.Trend[0].DataStatus != constant.SiteStatisticsPaused {
		t.Fatalf("paused active-window priority = %#v, %v", response.Trend, err)
	}
}

func assertStatisticsCorruptActiveWindowsIgnored(
	t *testing.T,
	database *gorm.DB,
	site model.Site,
	statistics *StatisticsService,
	hour, now int64,
) {
	t.Helper()
	const requestID = "req_statistics_corruption_probe"
	rollbackProbe := errors.New("rollback statistics corruption probe")
	err := database.Transaction(func(corrupt *gorm.DB) error {
		otherSiteStart := hour
		otherSite := model.Site{
			Name: "Statistics Corruption Other Site", BaseURL: "https://statistics-corruption-other.example",
			ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, StatisticsStartAt: &otherSiteStart,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := corrupt.Create(&otherSite).Error; err != nil {
			return fmt.Errorf("create corruption probe site: %w", err)
		}

		insideStart := hour
		insideEnd := hour + 3600
		outsideStart := hour + 3600
		outsideEnd := hour + 7200
		initializedAt := now
		probes := []struct {
			name       string
			parentSite int64
			targetType string
			targetID   int64
			start      *int64
			end        *int64
		}{
			{name: "cross-site-window", parentSite: otherSite.ID, targetType: "site", targetID: otherSite.ID, start: &insideStart, end: &insideEnd},
			{name: "wrong-target-type", parentSite: site.ID, targetType: "account", targetID: site.ID, start: &insideStart, end: &insideEnd},
			{name: "wrong-target-id", parentSite: site.ID, targetType: "site", targetID: otherSite.ID, start: &insideStart, end: &insideEnd},
			{name: "outside-parent-range", parentSite: site.ID, targetType: "site", targetID: site.ID, start: &outsideStart, end: &outsideEnd},
		}
		for _, probe := range probes {
			activeKey := "statistics-corruption-probe:" + probe.name
			run := model.CollectionRun{
				SiteID: &probe.parentSite, SiteConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
				TargetType: probe.targetType, TargetID: probe.targetID, TriggerType: constant.CollectionTriggerManual,
				StartTimestamp: probe.start, EndTimestamp: probe.end, Scope: []byte(`{}`), ActiveKey: &activeKey,
				Status: model.CollectionTaskStatusRunning, NextAttemptAt: now, WindowsInitializedAt: &initializedAt,
				TotalWindows: 1, CreatedRequestID: requestID, LastRequestID: requestID, CreatedAt: now, UpdatedAt: now,
			}
			if err := corrupt.Create(&run).Error; err != nil {
				return fmt.Errorf("create %s corruption run: %w", probe.name, err)
			}
			if err := corrupt.Create(&model.CollectionRunWindow{
				RunID: run.ID, SiteID: site.ID, HourTS: hour,
				Status: model.CollectionTaskStatusRunning, UpdatedAt: now,
			}).Error; err != nil {
				return fmt.Errorf("create %s corruption window: %w", probe.name, err)
			}
		}

		windows, err := model.NewStatisticsRepository(corrupt).LoadWindows(
			context.Background(), []int64{site.ID}, hour, hour+3600,
		)
		if err != nil {
			return err
		}
		if len(windows) != 0 {
			return fmt.Errorf("corrupt active windows leaked into coverage: %#v", windows)
		}
		corruptService, err := NewStatisticsService(StatisticsServiceOptions{
			Database: corrupt, Clock: testsupport.NewFakeClock(time.Unix(now, 0)),
		})
		if err != nil {
			return err
		}
		response, err := corruptService.Sites(context.Background(), dto.StatisticsQuery{
			StartTimestamp: hour, EndTimestamp: hour + 3600, Granularity: dto.StatisticsGranularityHour,
			SiteIDs: []int64{site.ID}, Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
		})
		if err != nil {
			return err
		}
		if len(response.Trend) != 1 || response.Trend[0].DataStatus != model.CollectionWindowStatusMissing {
			return fmt.Errorf("corrupt active windows changed coverage: %#v", response.Trend)
		}
		return rollbackProbe
	})
	if !errors.Is(err, rollbackProbe) {
		t.Fatalf("statistics corruption probe transaction: %v", err)
	}

	var remaining int64
	if err := database.Model(&model.CollectionRun{}).Where("created_request_id = ?", requestID).Count(&remaining).Error; err != nil {
		t.Fatalf("count rolled-back corruption runs: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("rolled-back corruption runs remaining = %d", remaining)
	}
	windows, err := model.NewStatisticsRepository(database).LoadWindows(
		context.Background(), []int64{site.ID}, hour, hour+3600,
	)
	if err != nil || len(windows) != 0 {
		t.Fatalf("statistics fixture after corruption rollback = %#v, %v", windows, err)
	}
	response, err := statistics.Sites(context.Background(), dto.StatisticsQuery{
		StartTimestamp: hour, EndTimestamp: hour + 3600, Granularity: dto.StatisticsGranularityHour,
		SiteIDs: []int64{site.ID}, Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	})
	if err != nil || len(response.Trend) != 1 || response.Trend[0].DataStatus != model.CollectionWindowStatusMissing {
		t.Fatalf("statistics fixture coverage after corruption rollback = %#v, %v", response.Trend, err)
	}
}

func createStatisticsActiveRun(
	t *testing.T,
	database *gorm.DB,
	site model.Site,
	taskType, runStatus string,
	hours []int64,
	windowStatus string,
	now int64,
	keySuffix string,
) {
	t.Helper()
	start := hours[0]
	end := hours[len(hours)-1] + 3600
	initializedAt := now
	activeKey := "statistics-active-window:" + strconv.FormatInt(site.ID, 10) + ":" + keySuffix
	run := model.CollectionRun{
		SiteID: &site.ID, SiteConfigVersion: site.ConfigVersion, TaskType: taskType,
		TargetType: "site", TargetID: site.ID, TriggerType: constant.CollectionTriggerManual,
		StartTimestamp: &start, EndTimestamp: &end, Scope: []byte(`{}`), ActiveKey: &activeKey,
		Status: runStatus, NextAttemptAt: now, WindowsInitializedAt: &initializedAt, TotalWindows: len(hours),
		CreatedRequestID: "req_statistics_matrix", LastRequestID: "req_statistics_matrix", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&run).Error; err != nil {
		t.Fatalf("create %s active run: %v", keySuffix, err)
	}
	rows := make([]model.CollectionRunWindow, 0, len(hours))
	for _, hour := range hours {
		rows = append(rows, model.CollectionRunWindow{
			RunID: run.ID, SiteID: site.ID, HourTS: hour, Status: windowStatus, UpdatedAt: now,
		})
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatalf("create %s active run windows: %v", keySuffix, err)
	}
}

func TestStatisticsCoverageZeroExpectedAndPausedUnits(t *testing.T) {
	empty := newStatisticsCoverage()
	if empty.status() != model.CollectionWindowStatusComplete || empty.completenessRate() != 1 {
		t.Fatalf("zero expected coverage = status %s rate %v", empty.status(), empty.completenessRate())
	}

	mixed := newStatisticsCoverage()
	mixed.add(1, model.CollectionWindowStatusComplete, 3600, true, nil)
	mixed.add(1, constant.SiteStatisticsPaused, 7200, false, nil)
	if mixed.Expected != 2 || mixed.Complete != 1 || mixed.status() != model.UsageAggregationStatusPartial ||
		mixed.completenessRate() != 0.5 {
		t.Fatalf("mixed paused coverage = %#v", mixed)
	}

	paused := newStatisticsCoverage()
	paused.add(1, constant.SiteStatisticsPaused, 3600, false, nil)
	if paused.Expected != 1 || paused.Complete != 0 || paused.status() != constant.SiteStatisticsPaused ||
		paused.completenessRate() != 0 {
		t.Fatalf("paused coverage = %#v", paused)
	}
}

func TestStatisticsPausedAccountAndCustomerHoursRemainExpected(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	pauseAt := fixture.start + 3600
	if err := fixture.database.Model(&model.Account{}).Where("id = ?", fixture.accounts[0].ID).
		Update("statistics_paused_at", pauseAt).Error; err != nil {
		t.Fatalf("pause statistics account: %v", err)
	}
	query := dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.start + 2*3600,
		Granularity: dto.StatisticsGranularityHour, AccountIDs: []int64{fixture.accounts[0].ID},
		Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	}
	account, err := fixture.service.Accounts(context.Background(), query)
	if err != nil || account.Completeness.ExpectedUnitCount != 2 || account.Completeness.CompleteUnitCount != 1 ||
		account.Completeness.DataStatus != model.UsageAggregationStatusPartial || account.Completeness.CompletenessRate != 0.5 ||
		account.Trend[1].DataStatus != constant.SiteStatisticsPaused || account.Trend[1].RequestCount != nil {
		t.Fatalf("paused account statistics = %#v, %v", account, err)
	}

	if err := fixture.database.Model(&model.Customer{}).Where("id = ?", fixture.customers[0].ID).
		Update("statistics_paused_at", pauseAt).Error; err != nil {
		t.Fatalf("pause statistics customer: %v", err)
	}
	query.AccountIDs = nil
	query.CustomerIDs = []int64{fixture.customers[0].ID}
	customer, err := fixture.service.Customers(context.Background(), query)
	if err != nil || customer.Completeness.ExpectedUnitCount != 4 || customer.Completeness.CompleteUnitCount != 2 ||
		customer.Completeness.DataStatus != model.UsageAggregationStatusPartial || customer.Completeness.CompletenessRate != 0.5 ||
		customer.Trend[1].DataStatus != constant.SiteStatisticsPaused {
		t.Fatalf("paused customer statistics = %#v, %v", customer, err)
	}
}

func TestStatisticsServiceUsesFixedQueryCountForLongTrend(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	counter := &statisticsQueryCounter{Interface: fixture.database.Logger}
	service, err := NewStatisticsService(StatisticsServiceOptions{
		Database: fixture.database.Session(&gorm.Session{Logger: counter}),
		Clock:    fixture.service.clock,
	})
	if err != nil {
		t.Fatalf("create counted statistics service: %v", err)
	}
	_, err = service.Global(context.Background(), dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.monthEnd,
		Granularity: dto.StatisticsGranularityDay, Page: 1, PageSize: 20,
		SortBy: "bucket_start", SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("query counted statistics: %v", err)
	}
	if got := counter.statements.Load(); got > 6 {
		t.Fatalf("statistics SQL statements = %d, want at most 6 fixed batch reads", got)
	}
}

func TestStatisticsRangeSiteBreakdownRejectsMetricOverflow(t *testing.T) {
	builder := &statisticsResponseBuilder{
		data: statisticsReadData{sites: []model.StatisticsSite{{ID: 1, Name: "overflow"}}},
		siteMetrics: map[statisticsSiteBucketKey]statisticsMetric{
			{SiteID: 1, Bucket: 1}: {Quota: math.MaxInt64},
			{SiteID: 1, Bucket: 2}: {Quota: 1},
		},
		siteRangeCoverage: map[int64]*statisticsCoverage{1: newStatisticsCoverage()},
	}
	if _, err := builder.buildRangeSiteBreakdown(); !errors.Is(err, model.ErrStatisticsReadContract) {
		t.Fatalf("range site breakdown overflow error = %v", err)
	}
}

func TestStatisticsMetricSortAlwaysPlacesUnknownRowsLast(t *testing.T) {
	known := statisticsBreakdownRow{Known: true, Metric: statisticsMetric{Quota: 0}}
	unknown := statisticsBreakdownRow{Known: false, Metric: statisticsMetric{Quota: math.MaxInt64}}
	for _, order := range []string{"asc", "desc"} {
		builder := &statisticsResponseBuilder{query: dto.StatisticsQuery{SortBy: "quota", SortOrder: order}}
		if !builder.breakdownLess(known, unknown) || builder.breakdownLess(unknown, known) {
			t.Fatalf("%s metric sorting did not keep unknown row last", order)
		}
	}
}

type statisticsQueryCounter struct {
	logger.Interface
	statements atomic.Int64
}

func (counter *statisticsQueryCounter) Trace(
	ctx context.Context,
	begin time.Time,
	query func() (string, int64),
	err error,
) {
	counter.statements.Add(1)
	counter.Interface.Trace(ctx, begin, query, err)
}

func newStatisticsServiceFixture(t *testing.T) statisticsServiceFixture {
	t.Helper()
	tx := openSiteTestTransaction(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix()
	statisticsEnd := time.Date(2032, 7, 3, 0, 0, 0, 0, location).Unix()
	monthEnd := time.Date(2032, 8, 1, 0, 0, 0, 0, location).Unix()
	now := time.Date(2032, 8, 2, 3, 0, 0, 0, location)
	quotaPerUnit := "500000.0000000000"
	exchangeRate := "7.1000000000"
	rateAt := now.Unix() - 60
	sites := []model.Site{
		{
			Name: "Statistics Alpha", BaseURL: "https://statistics-alpha.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, DataExportEnabled: true,
			QuotaPerUnit: &quotaPerUnit, USDExchangeRate: &exchangeRate, LastRateAt: &rateAt,
			StatisticsStartAt: &start, StatisticsEndAt: &statisticsEnd, CreatedAt: start, UpdatedAt: start,
		},
		{
			Name: "Statistics Beta", BaseURL: "https://statistics-beta.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsPartial,
			HealthStatus: constant.SiteHealthWarning, DataExportEnabled: true,
			StatisticsStartAt: &start, StatisticsEndAt: &statisticsEnd, CreatedAt: start, UpdatedAt: start,
		},
	}
	if err := tx.Create(&sites).Error; err != nil {
		t.Fatalf("create statistics sites: %v", err)
	}
	if err := tx.Exec(`UPDATE platform_setting SET setting_value = CASE setting_key
  WHEN 'rate.fallback_quota_per_unit' THEN '400000.0000000000'
  WHEN 'rate.fallback_usd_exchange_rate' THEN '7.0000000000'
  ELSE setting_value END
WHERE setting_key IN ('rate.fallback_quota_per_unit', 'rate.fallback_usd_exchange_rate')`).Error; err != nil {
		t.Fatalf("set statistics fallback rates: %v", err)
	}
	customers := []model.Customer{
		{Name: "Managed One", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none", CreatedAt: start, UpdatedAt: start},
		{Name: "Managed Zero", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none", CreatedAt: start, UpdatedAt: start},
	}
	if err := tx.Create(&customers).Error; err != nil {
		t.Fatalf("create statistics customers: %v", err)
	}
	accounts := []model.Account{
		statisticsTestAccount(sites[0].ID, customers[0].ID, 1, "alpha-user", start),
		statisticsTestAccount(sites[1].ID, customers[0].ID, 2, "beta-user", start),
		statisticsTestAccount(sites[0].ID, customers[1].ID, 3, "alpha-zero", start),
		statisticsTestAccount(sites[1].ID, customers[1].ID, 4, "beta-zero", start),
	}
	if err := tx.Create(&accounts).Error; err != nil {
		t.Fatalf("create statistics accounts: %v", err)
	}
	channels := []model.SiteChannel{
		{SiteID: sites[0].ID, RemoteChannelID: 1, Name: "Alpha Primary", LastSyncedAt: start, CreatedAt: start, UpdatedAt: start},
		{SiteID: sites[1].ID, RemoteChannelID: 1, Name: "Beta Primary", LastSyncedAt: start, CreatedAt: start, UpdatedAt: start},
	}
	if err := tx.Create(&channels).Error; err != nil {
		t.Fatalf("create statistics channels: %v", err)
	}

	windows := make([]model.CollectionWindow, 0, 96)
	for _, site := range sites {
		for hour := start; hour < statisticsEnd; hour += 3600 {
			local := time.Unix(hour, 0).In(location)
			verified := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 5, 0, 0, location).Unix()
			window := model.CollectionWindow{
				SiteID: site.ID, HourTS: hour, Status: model.CollectionWindowStatusComplete,
				VerifiedAt: &verified, UpdatedAt: verified,
			}
			if site.ID == sites[1].ID && hour == start+3600 {
				params, _ := common.Marshal(map[string]any{
					"site_id": strconv.FormatInt(site.ID, 10), "start_timestamp": hour, "end_timestamp": hour + 3600,
				})
				window.Status = model.CollectionWindowStatusMissing
				window.VerifiedAt = nil
				window.LastErrorCode = string(constant.MessageDataWindowMissing)
				window.LastErrorParams = params
			}
			windows = append(windows, window)
		}
	}
	if err := tx.CreateInBatches(windows, 200).Error; err != nil {
		t.Fatalf("create statistics windows: %v", err)
	}

	for day := 0; day < 2; day++ {
		hour := start + int64(day*24*3600)
		dateKey := 20320701 + day
		insertStatisticsFacts(t, tx, sites, hour, dateKey, now.Unix())
		insertStatisticsSummaries(t, tx, sites, customers, accounts, hour, dateKey, day == 0, now.Unix())
	}
	statistics, err := NewStatisticsService(StatisticsServiceOptions{
		Database: tx, Clock: testsupport.NewFakeClock(now),
	})
	if err != nil {
		t.Fatalf("create statistics service: %v", err)
	}
	return statisticsServiceFixture{
		service: statistics, database: tx, start: start, monthEnd: monthEnd,
		sites: sites, customers: customers, accounts: accounts,
	}
}

func statisticsTestAccount(siteID, customerID, remoteUserID int64, username string, start int64) model.Account {
	return model.Account{
		SiteID: siteID, CustomerID: customerID, RemoteUserID: remoteUserID, RemoteCreatedAt: start - 3600,
		Username: username, RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: start, UpdatedAt: start,
	}
}

func insertStatisticsFacts(t *testing.T, tx *gorm.DB, sites []model.Site, hour int64, dateKey int, now int64) {
	t.Helper()
	facts := []model.UsageFactHourly{
		{SiteID: sites[0].ID, RemoteUserID: 1, ModelName: "Model-A", ChannelID: 1, HourTS: hour, RequestCount: 1, Quota: 10, TokenUsed: 100, CollectedAt: now},
		{SiteID: sites[0].ID, RemoteUserID: 99, ModelName: "model-a", ChannelID: 0, HourTS: hour, RequestCount: 2, Quota: statisticsLargeQuota, TokenUsed: 200, CollectedAt: now},
		{SiteID: sites[1].ID, RemoteUserID: 2, ModelName: "Model-A", ChannelID: 1, HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, CollectedAt: now},
	}
	if err := tx.Create(&facts).Error; err != nil {
		t.Fatalf("create hourly statistics facts: %v", err)
	}
	daily := make([]model.UsageFactDaily, 0, len(facts))
	for _, fact := range facts {
		daily = append(daily, model.UsageFactDaily{
			SiteID: fact.SiteID, RemoteUserID: fact.RemoteUserID, ModelName: fact.ModelName,
			ChannelID: fact.ChannelID, DateKey: dateKey, RequestCount: fact.RequestCount,
			Quota: fact.Quota, TokenUsed: fact.TokenUsed, IsFinal: true,
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Create(&daily).Error; err != nil {
		t.Fatalf("create daily statistics facts: %v", err)
	}
}

func insertStatisticsSummaries(
	t *testing.T,
	tx *gorm.DB,
	sites []model.Site,
	customers []model.Customer,
	accounts []model.Account,
	hour int64,
	dateKey int,
	firstDay bool,
	now int64,
) {
	t.Helper()
	siteStatus := []string{"complete", "complete"}
	isFinal := []bool{true, true}
	if firstDay {
		siteStatus[1] = "partial"
		isFinal[1] = false
	}
	if err := tx.Create([]model.SiteStatHourly{
		{SiteID: sites[0].ID, HourTS: hour, RequestCount: 3, Quota: statisticsLargeQuota + 10, TokenUsed: 300, ActiveUsers: 2, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("create hourly site summaries: %v", err)
	}
	if err := tx.Create([]model.SiteStatDaily{
		{SiteID: sites[0].ID, DateKey: dateKey, RequestCount: 3, Quota: statisticsLargeQuota + 10, TokenUsed: 300, ActiveUsers: 2, DataStatus: siteStatus[0], IsFinal: isFinal[0], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, DateKey: dateKey, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: siteStatus[1], IsFinal: isFinal[1], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("create daily site summaries: %v", err)
	}
	if err := tx.Create([]model.CustomerStatHourly{
		{CustomerID: customers[0].ID, SiteID: sites[0].ID, HourTS: hour, RequestCount: 1, Quota: 10, TokenUsed: 100, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{CustomerID: customers[0].ID, SiteID: sites[1].ID, HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("create hourly customer summaries: %v", err)
	}
	dailyCustomers := []model.CustomerStatDaily{
		{CustomerID: customers[0].ID, SiteID: sites[0].ID, DateKey: dateKey, RequestCount: 1, Quota: 10, TokenUsed: 100, ActiveUsers: 1, DataStatus: siteStatus[0], IsFinal: isFinal[0], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{CustomerID: customers[0].ID, SiteID: sites[1].ID, DateKey: dateKey, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: siteStatus[1], IsFinal: isFinal[1], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if firstDay {
		dailyCustomers = append(dailyCustomers, model.CustomerStatDaily{
			CustomerID: customers[1].ID, SiteID: sites[1].ID, DateKey: dateKey,
			DataStatus: "partial", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Create(&dailyCustomers).Error; err != nil {
		t.Fatalf("create daily customer summaries: %v", err)
	}
	if err := tx.Create([]model.AccountStatHourly{
		{AccountID: accounts[0].ID, HourTS: hour, RequestCount: 1, Quota: 10, TokenUsed: 100, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{AccountID: accounts[1].ID, HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("create hourly account summaries: %v", err)
	}
	dailyAccounts := []model.AccountStatDaily{
		{AccountID: accounts[0].ID, DateKey: dateKey, RequestCount: 1, Quota: 10, TokenUsed: 100, DataStatus: siteStatus[0], IsFinal: isFinal[0], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{AccountID: accounts[1].ID, DateKey: dateKey, RequestCount: 3, Quota: 30, TokenUsed: 300, DataStatus: siteStatus[1], IsFinal: isFinal[1], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if firstDay {
		dailyAccounts = append(dailyAccounts, model.AccountStatDaily{
			AccountID: accounts[3].ID, DateKey: dateKey, DataStatus: "partial",
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Create(&dailyAccounts).Error; err != nil {
		t.Fatalf("create daily account summaries: %v", err)
	}
	insertStatisticsModelChannelSummaries(t, tx, sites, hour, dateKey, siteStatus, isFinal, now)
}

func insertStatisticsModelChannelSummaries(
	t *testing.T,
	tx *gorm.DB,
	sites []model.Site,
	hour int64,
	dateKey int,
	status []string,
	final []bool,
	now int64,
) {
	t.Helper()
	modelsHourly := []model.ModelStatHourly{
		{SiteID: sites[0].ID, ModelName: "Model-A", HourTS: hour, RequestCount: 1, Quota: 10, TokenUsed: 100, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[0].ID, ModelName: "model-a", HourTS: hour, RequestCount: 2, Quota: statisticsLargeQuota, TokenUsed: 200, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, ModelName: "Model-A", HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	modelsDaily := make([]model.ModelStatDaily, 0, len(modelsHourly))
	for _, item := range modelsHourly {
		index := 0
		if item.SiteID == sites[1].ID {
			index = 1
		}
		modelsDaily = append(modelsDaily, model.ModelStatDaily{
			SiteID: item.SiteID, ModelName: item.ModelName, DateKey: dateKey,
			RequestCount: item.RequestCount, Quota: item.Quota, TokenUsed: item.TokenUsed, ActiveUsers: item.ActiveUsers,
			DataStatus: status[index], IsFinal: final[index], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Create(&modelsHourly).Error; err != nil {
		t.Fatalf("create hourly model summaries: %v", err)
	}
	if err := tx.Create(&modelsDaily).Error; err != nil {
		t.Fatalf("create daily model summaries: %v", err)
	}
	channelsHourly := []model.ChannelStatHourly{
		{SiteID: sites[0].ID, ChannelID: 1, HourTS: hour, RequestCount: 1, Quota: 10, TokenUsed: 100, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[0].ID, ChannelID: 0, HourTS: hour, RequestCount: 2, Quota: statisticsLargeQuota, TokenUsed: 200, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, ChannelID: 1, HourTS: hour, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	channelsDaily := make([]model.ChannelStatDaily, 0, len(channelsHourly))
	for _, item := range channelsHourly {
		index := 0
		if item.SiteID == sites[1].ID {
			index = 1
		}
		channelsDaily = append(channelsDaily, model.ChannelStatDaily{
			SiteID: item.SiteID, ChannelID: item.ChannelID, DateKey: dateKey,
			RequestCount: item.RequestCount, Quota: item.Quota, TokenUsed: item.TokenUsed, ActiveUsers: item.ActiveUsers,
			DataStatus: status[index], IsFinal: final[index], LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Create(&channelsHourly).Error; err != nil {
		t.Fatalf("create hourly channel summaries: %v", err)
	}
	if err := tx.Create(&channelsDaily).Error; err != nil {
		t.Fatalf("create daily channel summaries: %v", err)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
