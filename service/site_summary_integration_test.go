package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSiteListAndDetailUseLatestResourceSummaryAndDefaultMissingMetricsToZero(t *testing.T) {
	tx := openSiteTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{
		authenticated: authorizedTestSiteClient(now), public: authorizedTestSiteClient(now),
	})
	repository := model.NewSiteRepository(tx)
	site := newTestSite(now, "https://site-summary.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	query := dto.SiteListQuery{Page: 1, PageSize: 20, SortBy: "priority", SortOrder: "asc"}
	page, err := sites.List(context.Background(), query)
	if err != nil {
		t.Fatalf("list sites without resource sample: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("list items = %#v", page.Items)
	}
	assertZeroSiteSummary(t, page.Items[0])

	cpu, memory, disk := 12.5, 34.5, 56.5
	minute := now - now%60
	if err := tx.Create(&model.SiteStatusMinutely{
		SiteID: site.ID, MinuteTS: minute, InstanceCount: 2, OnlineInstanceCount: 1,
		CPUMaxPercent: &cpu, MemoryMaxPercent: &memory, DiskMaxUsedPercent: &disk,
		HealthStatus: constant.SiteHealthOK, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create resource sample: %v", err)
	}

	page, err = sites.List(context.Background(), query)
	if err != nil {
		t.Fatalf("list sites with resource sample: %v", err)
	}
	item := page.Items[0]
	if item.Resource.InstanceCount == nil || *item.Resource.InstanceCount != 2 ||
		item.Resource.OnlineInstanceCount == nil || *item.Resource.OnlineInstanceCount != 1 ||
		item.Resource.CPUMaxPercent == nil || *item.Resource.CPUMaxPercent != cpu ||
		item.Resource.MemoryMaxPercent == nil || *item.Resource.MemoryMaxPercent != memory ||
		item.Resource.DiskMaxUsedPercent == nil || *item.Resource.DiskMaxUsedPercent != disk ||
		item.Resource.UpdatedAt == nil || *item.Resource.UpdatedAt != minute ||
		item.Resource.DataStatus != "complete" {
		t.Fatalf("list resource summary = %#v", item.Resource)
	}

	detail, err := sites.Get(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get site detail: %v", err)
	}
	if detail.Resource.CPUMaxPercent == nil || *detail.Resource.CPUMaxPercent != cpu ||
		detail.Resource.DataStatus != "complete" {
		t.Fatalf("detail resource summary = %#v", detail.Resource)
	}
}

func TestSiteListOverviewAggregatesTodayUsageAndDeduplicatesActiveUsers(t *testing.T) {
	tx := openSiteTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{
		authenticated: authorizedTestSiteClient(now), public: authorizedTestSiteClient(now),
	})
	repository := model.NewSiteRepository(tx)
	site := newTestSite(now, "https://site-usage-overview.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create site: %v", err)
	}

	todayStart, _ := siteTodayUsageRange(clock.Now())
	yesterdayHour := todayStart - 3600
	firstHour := now - 2*3600
	secondHour := now - 3600
	for _, hour := range []int64{yesterdayHour, firstHour, secondHour} {
		if err := tx.Create(&model.CollectionWindow{
			SiteID: site.ID, HourTS: hour, Status: model.CollectionWindowStatusComplete,
			FetchedRows: 1, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("create complete window: %v", err)
		}
	}
	stats := []model.SiteStatHourly{
		{SiteID: site.ID, HourTS: yesterdayHour, RequestCount: 99, Quota: 990, TokenUsed: 9900, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: yesterdayHour + 3600, CreatedAt: now, UpdatedAt: now},
		{SiteID: site.ID, HourTS: firstHour, RequestCount: 3, Quota: 30, TokenUsed: 300, ActiveUsers: 1, DataStatus: "complete", LastCalculatedAt: firstHour + 3600, CreatedAt: now, UpdatedAt: now},
		{SiteID: site.ID, HourTS: secondHour, RequestCount: 7, Quota: 70, TokenUsed: 700, ActiveUsers: 2, DataStatus: "complete", LastCalculatedAt: secondHour + 3600, CreatedAt: now, UpdatedAt: now},
	}
	if err := tx.Create(&stats).Error; err != nil {
		t.Fatalf("create site hourly stats: %v", err)
	}
	facts := []model.UsageFactHourly{
		{SiteID: site.ID, RemoteUserID: 303, ModelName: "model-c", ChannelID: 1, HourTS: yesterdayHour, RequestCount: 99, Quota: 990, TokenUsed: 9900, CollectedAt: now},
		{SiteID: site.ID, RemoteUserID: 101, ModelName: "model-a", ChannelID: 1, HourTS: firstHour, RequestCount: 3, Quota: 30, TokenUsed: 300, CollectedAt: now},
		{SiteID: site.ID, RemoteUserID: 101, ModelName: "model-a", ChannelID: 1, HourTS: secondHour, RequestCount: 4, Quota: 40, TokenUsed: 400, CollectedAt: now},
		{SiteID: site.ID, RemoteUserID: 202, ModelName: "model-b", ChannelID: 1, HourTS: secondHour, RequestCount: 3, Quota: 30, TokenUsed: 300, CollectedAt: now},
	}
	if err := tx.Create(&facts).Error; err != nil {
		t.Fatalf("create usage facts: %v", err)
	}

	page, err := sites.List(context.Background(), dto.SiteListQuery{Page: 1, PageSize: 20, SortBy: "priority", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("list sites: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("list items = %#v", page.Items)
	}
	usage := page.Items[0].Today
	if usage.RequestCount == nil || *usage.RequestCount != "10" ||
		usage.Quota == nil || *usage.Quota != "100" ||
		usage.TokenUsed == nil || *usage.TokenUsed != "1000" ||
		usage.ActiveUsers == nil || *usage.ActiveUsers != "2" ||
		usage.AsOf == nil || *usage.AsOf != secondHour+3600 || usage.DataStatus != "complete" {
		t.Fatalf("today usage overview = %#v", usage)
	}
	avgRPM, err := strconv.ParseFloat(*usage.AvgRPM, 64)
	if err != nil || avgRPM < 0.083 || avgRPM > 0.084 {
		t.Fatalf("average RPM = %q", *usage.AvgRPM)
	}
	avgTPM, err := strconv.ParseFloat(*usage.AvgTPM, 64)
	if err != nil || avgTPM < 8.33 || avgTPM > 8.34 {
		t.Fatalf("average TPM = %q", *usage.AvgTPM)
	}
}

func assertZeroSiteSummary(t *testing.T, item dto.SiteListItem) {
	t.Helper()
	if item.Realtime.RPM == nil || *item.Realtime.RPM != "0" ||
		item.Realtime.TPM == nil || *item.Realtime.TPM != "0" ||
		item.Today.RequestCount == nil || *item.Today.RequestCount != "0" ||
		item.Today.Quota == nil || *item.Today.Quota != "0" ||
		item.Today.AvgRPM == nil || *item.Today.AvgRPM != "0" ||
		item.Today.AvgTPM == nil || *item.Today.AvgTPM != "0" ||
		item.Today.ActiveUsers == nil || *item.Today.ActiveUsers != "0" ||
		item.Resource.InstanceCount == nil || *item.Resource.InstanceCount != 0 ||
		item.Resource.OnlineInstanceCount == nil || *item.Resource.OnlineInstanceCount != 0 ||
		item.Resource.CPUMaxPercent == nil || *item.Resource.CPUMaxPercent != 0 ||
		item.Resource.MemoryMaxPercent == nil || *item.Resource.MemoryMaxPercent != 0 ||
		item.Resource.DiskMaxUsedPercent == nil || *item.Resource.DiskMaxUsedPercent != 0 ||
		item.Resource.DataStatus != "missing" {
		t.Fatalf("zero-value site summary = %#v", item)
	}
}
