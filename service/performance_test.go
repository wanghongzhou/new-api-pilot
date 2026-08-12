package service

import (
	"testing"
	"time"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestSitePerformanceSummaryPreservesUpstreamModelMetrics(t *testing.T) {
	summary := sitePerformanceSummary(24, 1_784_255_643, dto.UpstreamPerformanceSummary{Models: []dto.UpstreamPerformanceModel{
		{ModelName: "small", SuccessRate: 50, AvgLatencyMS: 100, AvgTPS: 20},
		{ModelName: "large", SuccessRate: 100, AvgLatencyMS: 200, AvgTPS: 40},
	}})
	if summary.Hours != 24 || summary.SampledAt == nil || *summary.SampledAt != 1_784_255_643 || summary.DataStatus != sitePerformanceDataReady {
		t.Fatalf("unexpected summary metadata: %#v", summary)
	}
	if len(summary.Models) != 2 || summary.Models[0].SuccessRate != 50 || summary.Models[1].AvgTPS != 40 {
		t.Fatalf("unexpected model summaries: %#v", summary.Models)
	}
}

func TestSitePerformanceCacheRespectsVersionAndExpiry(t *testing.T) {
	cache := newSitePerformanceCache()
	summary := unavailableSitePerformanceSummary(24)
	cache.Store(1, 2, summary, 100)
	if _, ok := cache.Get(1, 2, 100); !ok {
		t.Fatal("stored cache entry was not available")
	}
	if _, ok := cache.Get(1, 3, 100); ok {
		t.Fatal("entry from an older configuration was used")
	}
	if _, ok := cache.Get(1, 2, 100+int64(sitePerformanceCacheTTL/time.Second)); ok {
		t.Fatal("expired entry was used")
	}
}

func TestSitePerformanceCacheKeepsLatestSuccessDuringRefresh(t *testing.T) {
	cache := newSitePerformanceCache()
	sampledAt := int64(100)
	summary := dto.SitePerformanceSummary{
		Hours: 24, SampledAt: &sampledAt, DataStatus: sitePerformanceDataReady,
		Models: []dto.SitePerformanceModel{{ModelName: "gpt-test", SuccessRate: 100}},
	}
	cache.Store(1, 2, summary, sampledAt)
	expiredAt := sampledAt + int64(sitePerformanceCacheTTL/time.Second)
	if _, ok := cache.Get(1, 2, expiredAt); ok {
		t.Fatal("expired cache entry was reported as fresh")
	}
	got, ok := cache.Latest(1, 2)
	if !ok || got.SampledAt == nil || *got.SampledAt != sampledAt {
		t.Fatalf("latest successful sample was not retained during refresh: %#v, ok=%v", got, ok)
	}
	if _, ok := cache.Latest(1, 3); ok {
		t.Fatal("latest sample from an older configuration was exposed")
	}
}

func TestSitePerformanceCacheFailureDoesNotOverwriteNewerSuccess(t *testing.T) {
	cache := newSitePerformanceCache()
	if !cache.StartRefresh(1) {
		t.Fatal("background refresh did not start")
	}
	sampledAt := int64(200)
	summary := dto.SitePerformanceSummary{
		Hours: 24, SampledAt: &sampledAt, DataStatus: sitePerformanceDataReady,
		Models: []dto.SitePerformanceModel{{ModelName: "gpt-test", SuccessRate: 100}},
	}
	cache.Store(1, 2, summary, sampledAt)
	cache.StoreFailure(1, 2, sampledAt+1)

	got, ok := cache.Get(1, 2, sampledAt+1)
	if !ok || got.DataStatus != sitePerformanceDataReady || len(got.Models) != 1 {
		t.Fatalf("late background failure overwrote newer success: %#v, ok=%v", got, ok)
	}
}

func TestSitePerformanceCacheStoresCurrentRefreshFailure(t *testing.T) {
	cache := newSitePerformanceCache()
	if !cache.StartRefresh(1) {
		t.Fatal("background refresh did not start")
	}
	cache.StoreFailure(1, 2, 200)

	got, ok := cache.Get(1, 2, 201)
	if !ok || got.DataStatus != "unavailable" || got.Models == nil {
		t.Fatalf("current background failure was not cached: %#v, ok=%v", got, ok)
	}
}

func TestSiteListItemPreservesPerformanceSummary(t *testing.T) {
	sampledAt := int64(200)
	performance := dto.SitePerformanceSummary{
		Hours: 24, SampledAt: &sampledAt, DataStatus: sitePerformanceDataReady,
		Models: []dto.SitePerformanceModel{{ModelName: "gpt-test", SuccessRate: 100, AvgLatencyMS: 250, AvgTPS: 40}},
	}
	item := siteListItemFromModel(model.Site{ID: 1}, 200, model.SiteStatusMinutely{}, model.SiteUsageOverview{}, performance, 0)

	if item.Performance.DataStatus != sitePerformanceDataReady || len(item.Performance.Models) != 1 ||
		item.Performance.Models[0].AvgTPS != 40 {
		t.Fatalf("site list item discarded performance summary: %#v", item.Performance)
	}
}
