package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"

	"gorm.io/gorm"
)

func TestDashboardServiceMySQLCompletePartialEmptyTopAndHealth(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	partialNow := time.Date(2032, 7, 1, 2, 30, 0, 0, location)
	partial := newDashboardTestService(t, fixture, partialNow, nil, nil, nil, nil)

	summary, err := partial.Summary(context.Background())
	if err != nil {
		t.Fatalf("partial dashboard summary: %v", err)
	}
	if summary.Today.DataStatus != model.UsageAggregationStatusPartial ||
		dashboardString(summary.Today.Quota) != strconv.FormatInt(statisticsLargeQuota+40, 10) ||
		summary.Today.AsOf == nil || summary.Today.IsFinal || len(summary.Today.SiteBreakdown) != 2 ||
		summary.Today.SiteBreakdown[0].RateSource != "site" || summary.Today.SiteBreakdown[1].RateSource != "fallback" ||
		dashboardString(summary.ActiveAccountsToday) != "2" || dashboardString(summary.RPM) != "9007199254740993" ||
		summary.RealtimeDataStatus != model.UsageAggregationStatusPartial || len(summary.StaleSiteIDs) != 1 {
		t.Fatalf("partial dashboard summary = %#v", summary)
	}

	trend, err := partial.Trend(context.Background(), dto.DashboardTrendQuery{Days: 30})
	if err != nil || len(trend) != 30 || trend[len(trend)-1].DataStatus != model.UsageAggregationStatusPartial ||
		len(trend[len(trend)-1].SiteBreakdown) != 2 {
		t.Fatalf("dashboard 30-day trend = %#v, %v", trend, err)
	}

	customers, err := partial.Top(context.Background(), dto.DashboardTopQuery{
		Type: dto.DashboardTopTypeCustomer, Metric: dto.DashboardTopMetricQuota, Limit: 2,
	})
	if err != nil || len(customers) != 2 || customers[0].DimensionName != "Managed One" ||
		dashboardString(customers[0].Value) != "40" || len(customers[0].SiteBreakdown) != 2 ||
		customers[1].Value == nil || customers[1].Reason == nil {
		t.Fatalf("dashboard customer top = %#v, %v", customers, err)
	}
	sitesTop, err := partial.Top(context.Background(), dto.DashboardTopQuery{
		Type: dto.DashboardTopTypeSite, Metric: dto.DashboardTopMetricQuota, Limit: 2,
	})
	if err != nil || len(sitesTop) != 2 || sitesTop[0].DimensionType != dto.DashboardTopTypeSite ||
		sitesTop[0].SiteID == nil || dashboardString(sitesTop[0].Value) != strconv.FormatInt(statisticsLargeQuota+10, 10) {
		t.Fatalf("dashboard site top = %#v, %v", sitesTop, err)
	}
	models, err := partial.Top(context.Background(), dto.DashboardTopQuery{
		Type: dto.DashboardTopTypeModel, Metric: dto.DashboardTopMetricRequestCount, Limit: 4,
	})
	if err != nil || len(models) == 0 || models[0].DimensionType != dto.DashboardTopTypeModel || models[0].SiteID == nil {
		t.Fatalf("dashboard model top = %#v, %v", models, err)
	}
	channels, err := partial.Top(context.Background(), dto.DashboardTopQuery{
		Type: dto.DashboardTopTypeChannel, Metric: dto.DashboardTopMetricRequestCount, Limit: 4,
	})
	if err != nil || len(channels) == 0 || channels[0].DimensionType != dto.DashboardTopTypeChannel ||
		channels[0].SiteID == nil {
		t.Fatalf("dashboard channel top = %#v, %v", channels, err)
	}

	completeNow := time.Date(2032, 7, 2, 2, 30, 0, 0, location)
	complete := newDashboardTestService(t, fixture, completeNow, nil, nil, nil, nil)
	completeSummary, err := complete.Summary(context.Background())
	if err != nil || completeSummary.Today.DataStatus != model.CollectionWindowStatusComplete ||
		completeSummary.Today.RequestCount == nil || completeSummary.Today.IsFinal {
		t.Fatalf("complete current-day dashboard summary = %#v, %v", completeSummary, err)
	}

	emptyNow := time.Date(2032, 6, 1, 2, 30, 0, 0, location)
	empty := newDashboardTestService(t, fixture, emptyNow, nil, nil, nil, nil)
	emptySummary, err := empty.Summary(context.Background())
	if err != nil || emptySummary.Today.RequestCount != nil || emptySummary.Today.Quota != nil ||
		emptySummary.Today.TokenUsed != nil || emptySummary.Today.ActiveUsers != nil || emptySummary.Today.AsOf != nil {
		t.Fatalf("empty dashboard summary = %#v, %v", emptySummary, err)
	}
	emptyTop, err := empty.Top(context.Background(), dto.DashboardTopQuery{
		Type: dto.DashboardTopTypeCustomer, Metric: dto.DashboardTopMetricQuota, Limit: 2,
	})
	if err != nil || len(emptyTop) != 2 || emptyTop[0].Value != nil ||
		emptyTop[0].DataStatus != model.CollectionWindowStatusComplete || emptyTop[0].Reason != nil {
		t.Fatalf("empty dashboard top = %#v, %v", emptyTop, err)
	}

	healthNow := time.Date(2032, 7, 3, 3, 0, 0, 0, location)
	healthService := newDashboardTestService(t, fixture, healthNow, nil, nil, nil, nil)
	health, err := healthService.Health(context.Background())
	if err != nil || health.YesterdayValidationStatus != model.CollectionWindowStatusComplete || !health.IsFinal ||
		health.AsOf == nil || len(health.Sites) != 2 || health.Sites[0].SiteName != "Statistics Beta" ||
		len(health.AuthExpiredSiteIDs) != 1 || len(health.StatisticsNotReadySiteIDs) != 1 ||
		health.FiringAlertCount != 1 || len(health.LatestAlerts) != 1 {
		t.Fatalf("dashboard health = %#v, %v", health, err)
	}
}

func TestDashboardServiceUsesFixedReaderAndSQLCounts(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2032, 7, 1, 2, 30, 0, 0, location)
	counter := &statisticsQueryCounter{Interface: fixture.database.Logger}
	clock := testsupport.NewFakeClock(now)
	statistics, err := NewStatisticsService(StatisticsServiceOptions{
		Database: fixture.database.Session(&gorm.Session{Logger: counter}), Clock: clock,
	})
	if err != nil {
		t.Fatalf("create counted dashboard statistics: %v", err)
	}
	realtime := dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}
	sites := dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)}
	alerts := dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)}
	dashboard, err := NewDashboardService(DashboardServiceOptions{
		Statistics: statistics, SiteHealth: &sites, Alerts: &alerts, Realtime: &realtime, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create counted dashboard: %v", err)
	}

	assertQueries := func(name string, maximum int64, operation func() error) {
		t.Helper()
		before := counter.statements.Load()
		if err := operation(); err != nil {
			t.Fatalf("%s dashboard operation: %v", name, err)
		}
		if count := counter.statements.Load() - before; count > maximum {
			t.Fatalf("%s dashboard SQL statements = %d, want <= %d", name, count, maximum)
		}
	}
	assertQueries("summary", 5, func() error {
		_, err := dashboard.Summary(context.Background())
		return err
	})
	assertQueries("trend", 5, func() error {
		_, err := dashboard.Trend(context.Background(), dto.DashboardTrendQuery{Days: 30})
		return err
	})
	assertQueries("top", 7, func() error {
		_, err := dashboard.Top(context.Background(), dto.DashboardTopQuery{
			Type: dto.DashboardTopTypeCustomer, Metric: dto.DashboardTopMetricRequestCount, Limit: 5,
		})
		return err
	})
	assertQueries("health", 5, func() error {
		_, err := dashboard.Health(context.Background())
		return err
	})
	if realtime.calls != 1 || sites.calls != 1 || alerts.calls != 1 {
		t.Fatalf("dashboard external reader calls = realtime:%d sites:%d alerts:%d", realtime.calls, sites.calls, alerts.calls)
	}
}

func TestDashboardServiceReaderFailureIsIsolatedToItsEndpoint(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2032, 7, 1, 2, 30, 0, 0, location)
	clock := testsupport.NewFakeClock(now)
	statistics, err := NewStatisticsService(StatisticsServiceOptions{Database: fixture.database, Clock: clock})
	if err != nil {
		t.Fatalf("create dashboard isolation statistics: %v", err)
	}
	injected := errors.New("injected dashboard reader failure")

	tests := []struct {
		name       string
		failed     string
		statistics *dashboardStatisticsReader
		sites      *dashboardSiteHealthReader
		alerts     *dashboardAlertReader
		realtime   *dashboardRealtimeReader
	}{
		{name: "summary realtime", failed: "summary", statistics: &dashboardStatisticsReader{delegate: statistics},
			sites:    &dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)},
			alerts:   &dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)},
			realtime: &dashboardRealtimeReader{err: injected}},
		{name: "trend statistics", failed: "trend", statistics: &dashboardStatisticsReader{delegate: statistics, longGlobalErr: injected},
			sites:    &dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)},
			alerts:   &dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)},
			realtime: &dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}},
		{name: "top statistics", failed: "top", statistics: &dashboardStatisticsReader{delegate: statistics, customerErr: injected},
			sites:    &dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)},
			alerts:   &dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)},
			realtime: &dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}},
		{name: "health sites", failed: "health", statistics: &dashboardStatisticsReader{delegate: statistics},
			sites:    &dashboardSiteHealthReader{err: injected},
			alerts:   &dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)},
			realtime: &dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}},
		{name: "health alerts", failed: "health", statistics: &dashboardStatisticsReader{delegate: statistics},
			sites:    &dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)},
			alerts:   &dashboardAlertReader{err: injected},
			realtime: &dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dashboard, err := NewDashboardService(DashboardServiceOptions{
				Statistics: test.statistics, SiteHealth: test.sites, Alerts: test.alerts,
				Realtime: test.realtime, Clock: clock,
			})
			if err != nil {
				t.Fatalf("create isolated dashboard: %v", err)
			}
			results := map[string]error{}
			_, results["summary"] = dashboard.Summary(context.Background())
			_, results["trend"] = dashboard.Trend(context.Background(), dto.DashboardTrendQuery{Days: 30})
			_, results["top"] = dashboard.Top(context.Background(), dto.DashboardTopQuery{
				Type: dto.DashboardTopTypeCustomer, Metric: dto.DashboardTopMetricQuota, Limit: 5,
			})
			_, results["health"] = dashboard.Health(context.Background())
			for endpoint, result := range results {
				if endpoint == test.failed {
					if !errors.Is(result, ErrDashboardRead) {
						t.Fatalf("%s error = %v, want dashboard read failure", endpoint, result)
					}
				} else if result != nil {
					t.Fatalf("%s was blocked by %s failure: %v", endpoint, test.failed, result)
				}
			}
		})
	}
}

func TestDashboardTopUsesFixedFourScopeQueries(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2032, 7, 1, 2, 30, 0, 0, location)
	clock := testsupport.NewFakeClock(now)
	start, end := dashboardTodayRange(now)
	for _, test := range []struct {
		topType string
		metric  string
	}{
		{dto.DashboardTopTypeSite, dto.DashboardTopMetricQuota},
		{dto.DashboardTopTypeCustomer, dto.DashboardTopMetricRequestCount},
		{dto.DashboardTopTypeModel, dto.DashboardTopMetricQuota},
		{dto.DashboardTopTypeChannel, dto.DashboardTopMetricRequestCount},
	} {
		t.Run(test.topType, func(t *testing.T) {
			reader := &dashboardTopRoutingReader{}
			dashboard, err := NewDashboardService(DashboardServiceOptions{
				Statistics: reader,
				SiteHealth: &dashboardSiteHealthReader{},
				Alerts:     &dashboardAlertReader{},
				Realtime:   &dashboardRealtimeReader{},
				Clock:      clock,
			})
			if err != nil {
				t.Fatalf("create dashboard: %v", err)
			}
			items, err := dashboard.Top(context.Background(), dto.DashboardTopQuery{
				Type: test.topType, Metric: test.metric, Limit: 7,
			})
			if err != nil || len(items) != 0 || reader.called != test.topType {
				t.Fatalf("top route items=%#v called=%q err=%v", items, reader.called, err)
			}
			query := reader.query
			if query.StartTimestamp != start || query.EndTimestamp != end ||
				query.Granularity != dto.StatisticsGranularityDay || query.Page != 1 || query.PageSize != 7 ||
				query.SortBy != test.metric || query.SortOrder != "desc" {
				t.Fatalf("fixed top query = %#v", query)
			}
		})
	}
}

type dashboardTopRoutingReader struct {
	called string
	query  dto.StatisticsQuery
}

func (reader *dashboardTopRoutingReader) response(scope string, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	reader.called, reader.query = scope, query
	return dto.StatisticsResponse{
		Scope: scope, Granularity: dto.StatisticsGranularityDay,
		Range: dto.StatisticsRange{StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp},
	}, nil
}

func (reader *dashboardTopRoutingReader) Global(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return dto.StatisticsResponse{}, errors.New("unexpected global query")
}

func (reader *dashboardTopRoutingReader) Sites(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.response(dto.StatisticsScopeSite, query)
}

func (reader *dashboardTopRoutingReader) Customers(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.response(dto.StatisticsScopeCustomer, query)
}

func (reader *dashboardTopRoutingReader) Models(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.response(dto.StatisticsScopeModel, query)
}

func (reader *dashboardTopRoutingReader) Channels(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.response(dto.StatisticsScopeChannel, query)
}

type dashboardStatisticsReader struct {
	delegate      DashboardStatisticsReader
	longGlobalErr error
	customerErr   error
}

func (reader *dashboardStatisticsReader) Global(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	if reader.longGlobalErr != nil && query.EndTimestamp-query.StartTimestamp > 2*24*3600 {
		return dto.StatisticsResponse{}, reader.longGlobalErr
	}
	return reader.delegate.Global(ctx, query)
}

func (reader *dashboardStatisticsReader) Sites(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.delegate.Sites(ctx, query)
}

func (reader *dashboardStatisticsReader) Customers(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	if reader.customerErr != nil {
		return dto.StatisticsResponse{}, reader.customerErr
	}
	return reader.delegate.Customers(ctx, query)
}

func (reader *dashboardStatisticsReader) Models(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.delegate.Models(ctx, query)
}

func (reader *dashboardStatisticsReader) Channels(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return reader.delegate.Channels(ctx, query)
}

type dashboardSiteHealthReader struct {
	sites []DashboardSiteHealthSnapshot
	err   error
	calls int
}

func (reader *dashboardSiteHealthReader) ReadDashboardSiteHealth(context.Context) ([]DashboardSiteHealthSnapshot, error) {
	reader.calls++
	return append([]DashboardSiteHealthSnapshot(nil), reader.sites...), reader.err
}

type dashboardAlertReader struct {
	snapshot DashboardAlertSnapshot
	err      error
	calls    int
}

func (reader *dashboardAlertReader) ReadDashboardAlerts(context.Context, int) (DashboardAlertSnapshot, error) {
	reader.calls++
	return reader.snapshot, reader.err
}

type dashboardRealtimeReader struct {
	snapshot DashboardRealtimeSnapshot
	err      error
	calls    int
}

func (reader *dashboardRealtimeReader) ReadDashboardRealtime(context.Context) (DashboardRealtimeSnapshot, error) {
	reader.calls++
	return reader.snapshot, reader.err
}

func newDashboardTestService(
	t *testing.T,
	fixture statisticsServiceFixture,
	now time.Time,
	statistics DashboardStatisticsReader,
	sites *dashboardSiteHealthReader,
	alerts *dashboardAlertReader,
	realtime *dashboardRealtimeReader,
) *DashboardService {
	t.Helper()
	clock := testsupport.NewFakeClock(now)
	if statistics == nil {
		var err error
		statistics, err = NewStatisticsService(StatisticsServiceOptions{Database: fixture.database, Clock: clock})
		if err != nil {
			t.Fatalf("create dashboard statistics: %v", err)
		}
	}
	if sites == nil {
		sites = &dashboardSiteHealthReader{sites: dashboardSiteHealthFixture(fixture, now)}
	}
	if alerts == nil {
		alerts = &dashboardAlertReader{snapshot: dashboardAlertFixture(fixture, now)}
	}
	if realtime == nil {
		realtime = &dashboardRealtimeReader{snapshot: dashboardRealtimeFixture(fixture, now)}
	}
	dashboard, err := NewDashboardService(DashboardServiceOptions{
		Statistics: statistics, SiteHealth: sites, Alerts: alerts, Realtime: realtime, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create dashboard service: %v", err)
	}
	return dashboard
}

func dashboardRealtimeFixture(fixture statisticsServiceFixture, now time.Time) DashboardRealtimeSnapshot {
	active := "2"
	var activeAccounts *string
	if now.Unix() >= fixture.start {
		activeAccounts = &active
	}
	rpm := "9007199254740993"
	tpm := "9007199254740994"
	instances := 3
	onlineInstances := 2
	asOf := now.Unix() - 30
	reason := dto.MustMessageRef(constant.MessageDataPartialSites, map[string]any{
		"complete_site_count": 1, "expected_site_count": 2,
	}, "")
	return DashboardRealtimeSnapshot{
		ActiveAccountsToday: activeAccounts,
		SiteCount:           2, OnlineSiteCount: 1, OfflineSiteCount: 1,
		CustomerCount: 2, ManagedAccountCount: 4,
		InstanceCount: &instances, OnlineInstanceCount: &onlineInstances,
		ResourceCompleteSiteCount: 1, ResourceExpectedSiteCount: 2,
		ResourceStaleSiteIDs: []int64{fixture.sites[1].ID}, ResourceDataStatus: model.UsageAggregationStatusPartial,
		ResourceAsOf: &asOf, ResourceReason: &reason,
		RPM: &rpm, TPM: &tpm, RealtimeCompleteSiteCount: 1, RealtimeExpectedSiteCount: 2,
		StaleSiteIDs: []int64{fixture.sites[1].ID}, RealtimeDataStatus: model.UsageAggregationStatusPartial,
		RealtimeAsOf: &asOf, RealtimeReason: &reason,
	}
}

func dashboardSiteHealthFixture(fixture statisticsServiceFixture, now time.Time) []DashboardSiteHealthSnapshot {
	return []DashboardSiteHealthSnapshot{
		{SiteID: fixture.sites[0].ID, SiteName: fixture.sites[0].Name,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, UpdatedAt: now.Unix()},
		{SiteID: fixture.sites[1].ID, SiteName: fixture.sites[1].Name,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOffline,
			AuthStatus: constant.SiteAuthExpired, StatisticsStatus: constant.SiteStatisticsPartial,
			HealthStatus: constant.SiteHealthWarning, UpdatedAt: now.Unix()},
	}
}

func dashboardAlertFixture(fixture statisticsServiceFixture, now time.Time) DashboardAlertSnapshot {
	siteID := strconv.FormatInt(fixture.sites[1].ID, 10)
	message := dto.MustMessageRef(constant.MessageAlertAuthExpired, map[string]any{
		"site_id": siteID, "site_name": fixture.sites[1].Name,
	}, "")
	return DashboardAlertSnapshot{
		Summary: dto.AlertSummary{
			FiringCount: 1, CriticalCount: 1, WarningCount: 0, ResolvedTodayCount: 0, UpdatedAt: now.Unix(),
		},
		Latest: []dto.AlertEventItem{{
			ID: "1", RuleID: "1", RuleKey: "site.auth_expired", SiteID: &siteID,
			SiteName: fixture.sites[1].Name, TargetType: "site", TargetKey: siteID,
			TargetName: fixture.sites[1].Name, Level: dto.AlertLevelCritical, Status: dto.AlertStatusFiring,
			Message: message, FirstObservedAt: now.Unix() - 60,
		}},
	}
}

func dashboardString(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
