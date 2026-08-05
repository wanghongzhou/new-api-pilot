package service

import (
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"testing"
)

func performanceInt64(value int64) *int64 { return &value }

func TestPerformanceCollectionStatus(t *testing.T) {
	tests := []struct {
		name     string
		coverage model.PerformanceCollectionCoverage
		want     string
	}{
		{name: "no eligible sites", want: "complete"},
		{name: "all successful", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, SuccessfulSites: 2}, want: "complete"},
		{name: "some successful", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, SuccessfulSites: 1}, want: "partial"},
		{name: "all unavailable", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, UnavailableSites: 2}, want: "unavailable"},
		{name: "some unavailable", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, UnavailableSites: 1}, want: "partial"},
		{name: "never collected", coverage: model.PerformanceCollectionCoverage{SiteCount: 2}, want: "pending"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := performanceCollectionStatus(test.coverage); got != test.want {
				t.Fatalf("performanceCollectionStatus(%#v)=%q want %q", test.coverage, got, test.want)
			}
		})
	}
}

func TestWeightedPerformanceBreakdownsUseCounterDenominators(t *testing.T) {
	rows := []model.PerformanceHistoryReadRow{
		{SitePerformanceMetricBucket: model.SitePerformanceMetricBucket{BucketTS: 3600, ModelName: "gpt", RemoteGroup: "default", MetricSource: model.PerformanceMetricSourceCounterReady, RequestCount: performanceInt64(1), SuccessCount: performanceInt64(1), TotalLatencyMS: performanceInt64(100), TTFTSumMS: performanceInt64(50), TTFTCount: performanceInt64(1), OutputTokens: performanceInt64(100), GenerationMS: performanceInt64(1000)}},
		{SitePerformanceMetricBucket: model.SitePerformanceMetricBucket{BucketTS: 3600, ModelName: "gpt", RemoteGroup: "default", MetricSource: model.PerformanceMetricSourceCounterReady, RequestCount: performanceInt64(9), SuccessCount: performanceInt64(0), TotalLatencyMS: performanceInt64(9000), TTFTSumMS: performanceInt64(4500), TTFTCount: performanceInt64(9), OutputTokens: performanceInt64(900), GenerationMS: performanceInt64(9000)}},
	}
	items := weightedPerformanceDimensionBreakdown(rows, func(row model.PerformanceHistoryReadRow) string { return row.ModelName })
	if len(items) != 1 || items[0].Dimension != "gpt" || items[0].AvgLatencyMS == nil || *items[0].AvgLatencyMS != "910.0000000000" || items[0].RequestCount == nil || *items[0].RequestCount != "10" {
		t.Fatalf("breakdown=%#v", items)
	}
	coverage := model.PerformanceCollectionCoverage{SiteCount: 2, SuccessfulSites: 1, UnavailableSites: 1}
	if got := performanceCompleteness(coverage); got != (dto.PerformanceCompleteness{DataStatus: "partial", SuccessfulSiteCount: 1, UnavailableSiteCount: 1, ExpectedSiteCount: 2}) {
		t.Fatalf("completeness=%#v", got)
	}
}

func TestPerformanceTrendPreservesRawSeriesIdentity(t *testing.T) {
	rows := []model.PerformanceHistoryReadRow{{
		SitePerformanceMetricBucket: model.SitePerformanceMetricBucket{
			ID: 1, SiteID: 2, BucketTS: 3600, ModelName: "gpt", RemoteGroup: "vip",
			MetricSource: model.PerformanceMetricSourceOfficialAverage,
			AvgLatencyMS: "100.0000000000", AvgTTFTMS: "50.0000000000",
			SuccessRate: "0.9000000000", AvgTPS: "1.0000000000",
		},
		SiteName: "east",
	}}
	items := performanceHistoryItems(rows)
	if len(items) != 1 || items[0].SiteID != "2" || items[0].SiteName != "east" || items[0].ModelName != "gpt" || items[0].Group != "vip" || items[0].BucketStart != 3600 {
		t.Fatalf("raw trend identity=%#v", items)
	}
}
