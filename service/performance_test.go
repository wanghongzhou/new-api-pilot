package service

import (
	"testing"
	"time"

	"new-api-pilot/dto"
)

func TestSitePerformanceSummaryUsesRequestWeightedMetrics(t *testing.T) {
	summary := sitePerformanceSummary(24, 1_784_255_643, dto.UpstreamPerformanceSummary{Models: []dto.UpstreamPerformanceModel{
		{ModelName: "small", RequestCount: 10, SuccessRate: 50, AvgLatencyMS: 100, AvgTPS: 20},
		{ModelName: "large", RequestCount: 30, SuccessRate: 100, AvgLatencyMS: 200, AvgTPS: 40},
	}})
	if summary.Hours != 24 || summary.SampledAt == nil || *summary.SampledAt != 1_784_255_643 || summary.DataStatus != sitePerformanceDataReady {
		t.Fatalf("unexpected summary metadata: %#v", summary)
	}
	if summary.RequestCount != "40" || summary.SuccessRate != 87.5 || summary.AvgLatencyMS != 175 || summary.AvgTPS != 35 {
		t.Fatalf("unexpected weighted summary: %#v", summary)
	}
	if len(summary.Models) != 2 || summary.Models[0].RequestCount != "10" {
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
