package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"new-api-pilot/dto"

	"gorm.io/gorm"
)

func TestDashboardStatisticsProjectionsUseOnlyPreaggregatedMetrics(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	recorder := &statisticsQueryCounter{Interface: fixture.database.Logger}
	statistics, err := NewStatisticsService(StatisticsServiceOptions{
		Database: fixture.database.Session(&gorm.Session{Logger: recorder}),
		Clock:    fixture.service.clock,
	})
	if err != nil {
		t.Fatalf("create recorded dashboard statistics service: %v", err)
	}
	query := dto.StatisticsQuery{
		StartTimestamp: fixture.start, EndTimestamp: fixture.start + 24*60*60,
		Granularity: dto.StatisticsGranularityDay, Page: 1, PageSize: 100,
		SortBy: "request_count", SortOrder: "desc",
	}

	globalQuery := query
	globalQuery.PageSize = 1
	globalQuery.SortBy = "bucket_start"
	global, err := statistics.DashboardGlobal(context.Background(), globalQuery)
	if err != nil {
		t.Fatalf("query dashboard global projection: %v", err)
	}
	if global.Scope != dto.StatisticsScopeGlobal || len(global.Trend) != 1 ||
		stringValue(global.Trend[0].RequestCount) != "6" || stringValue(global.Trend[0].ActiveUsers) != "3" {
		t.Fatalf("dashboard global projection = %#v", global)
	}

	sites, err := statistics.DashboardSites(context.Background(), query)
	if err != nil {
		t.Fatalf("query dashboard site projection: %v", err)
	}
	if sites.Scope != dto.StatisticsScopeSite || len(sites.Breakdown.Items) != 2 {
		t.Fatalf("dashboard site projection = %#v", sites)
	}
	assertDashboardSiteProjection(t, sites, fixture.sites[0].ID, "3", "2")
	assertDashboardSiteProjection(t, sites, fixture.sites[1].ID, "3", "1")

	customers, err := statistics.DashboardCustomers(context.Background(), query)
	if err != nil {
		t.Fatalf("query dashboard customer projection: %v", err)
	}
	if customers.Scope != dto.StatisticsScopeCustomer {
		t.Fatalf("dashboard customer scope = %q", customers.Scope)
	}
	assertDashboardCustomerProjection(t, customers, fixture.customers[0].ID, "4", "2")

	models, err := statistics.DashboardModels(context.Background(), query)
	if err != nil {
		t.Fatalf("query dashboard model projection: %v", err)
	}
	if models.Scope != dto.StatisticsScopeModel {
		t.Fatalf("dashboard model scope = %q", models.Scope)
	}
	assertDashboardModelProjection(t, models, fixture.sites[0].ID, "model-a", "2", "1")

	channels, err := statistics.DashboardChannels(context.Background(), query)
	if err != nil {
		t.Fatalf("query dashboard channel projection: %v", err)
	}
	if channels.Scope != dto.StatisticsScopeChannel {
		t.Fatalf("dashboard channel scope = %q", channels.Scope)
	}
	assertDashboardChannelProjection(t, channels, fixture.sites[0].ID, "0", "2", "1")

	recorder.mu.Lock()
	queries := append([]string(nil), recorder.queries...)
	recorder.mu.Unlock()
	if len(queries) == 0 {
		t.Fatal("dashboard projection SQL was not recorded")
	}
	for _, statement := range queries {
		lower := strings.ToLower(statement)
		if strings.Contains(lower, "usage_fact_hourly") || strings.Contains(lower, "usage_fact_daily") {
			t.Fatalf("dashboard metrics-only projection accessed a usage fact table: %s", statement)
		}
	}
}

func assertDashboardSiteProjection(
	t *testing.T,
	response dto.StatisticsResponse,
	siteID int64,
	requestCount, activeUsers string,
) {
	t.Helper()
	wantID := strconv.FormatInt(siteID, 10)
	for _, raw := range response.Breakdown.Items {
		item, ok := raw.(dto.SiteStatisticsBreakdown)
		if !ok || item.DimensionID != wantID {
			continue
		}
		if stringValue(item.RequestCount) != requestCount || stringValue(item.ActiveUsers) != activeUsers {
			t.Fatalf("dashboard site %s projection = %#v", wantID, item)
		}
		return
	}
	t.Fatalf("dashboard site %s projection missing: %#v", wantID, response.Breakdown)
}

func assertDashboardCustomerProjection(
	t *testing.T,
	response dto.StatisticsResponse,
	customerID int64,
	requestCount, activeUsers string,
) {
	t.Helper()
	wantID := strconv.FormatInt(customerID, 10)
	for _, raw := range response.Breakdown.Items {
		item, ok := raw.(dto.CustomerStatisticsBreakdown)
		if !ok || item.DimensionID != wantID {
			continue
		}
		if stringValue(item.RequestCount) != requestCount || stringValue(item.ActiveUsers) != activeUsers {
			t.Fatalf("dashboard customer %s projection = %#v", wantID, item)
		}
		return
	}
	t.Fatalf("dashboard customer %s projection missing: %#v", wantID, response.Breakdown)
}

func assertDashboardModelProjection(
	t *testing.T,
	response dto.StatisticsResponse,
	siteID int64,
	modelName, requestCount, activeUsers string,
) {
	t.Helper()
	wantSiteID := strconv.FormatInt(siteID, 10)
	for _, raw := range response.Breakdown.Items {
		item, ok := raw.(dto.ModelStatisticsBreakdown)
		if !ok || item.SiteID == nil || *item.SiteID != wantSiteID || item.ModelName != modelName {
			continue
		}
		if stringValue(item.RequestCount) != requestCount || stringValue(item.ActiveUsers) != activeUsers {
			t.Fatalf("dashboard model %s/%s projection = %#v", wantSiteID, modelName, item)
		}
		return
	}
	t.Fatalf("dashboard model %s/%s projection missing: %#v", wantSiteID, modelName, response.Breakdown)
}

func assertDashboardChannelProjection(
	t *testing.T,
	response dto.StatisticsResponse,
	siteID int64,
	channelID, requestCount, activeUsers string,
) {
	t.Helper()
	wantSiteID := strconv.FormatInt(siteID, 10)
	for _, raw := range response.Breakdown.Items {
		item, ok := raw.(dto.ChannelStatisticsBreakdown)
		if !ok || item.SiteID == nil || *item.SiteID != wantSiteID || item.RemoteChannelID != channelID {
			continue
		}
		if stringValue(item.RequestCount) != requestCount || stringValue(item.ActiveUsers) != activeUsers {
			t.Fatalf("dashboard channel %s/%s projection = %#v", wantSiteID, channelID, item)
		}
		return
	}
	t.Fatalf("dashboard channel %s/%s projection missing: %#v", wantSiteID, channelID, response.Breakdown)
}
