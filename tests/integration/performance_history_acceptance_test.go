package integration_test

import (
	"context"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	"os"
	"strings"
	"testing"
)

func perfPointer(v int64) *int64 { return &v }
func perfHistory(ts int64, counter bool, request, success, latency, ttftSum, ttftCount, output, generation int64) dto.UpstreamPerformanceHistory {
	bucket := dto.UpstreamPerformanceBucket{Timestamp: ts, AvgTTFTMS: "50", AvgLatencyMS: "100", SuccessRate: "0.9", AvgTPS: "20"}
	if counter {
		bucket.Counters = dto.UpstreamPerformanceCounters{RequestCount: perfPointer(request), SuccessCount: perfPointer(success), TotalLatencyMS: perfPointer(latency), TTFTSumMS: perfPointer(ttftSum), TTFTCount: perfPointer(ttftCount), OutputTokens: perfPointer(output), GenerationMS: perfPointer(generation)}
	}
	return dto.UpstreamPerformanceHistory{CounterReady: counter, Models: []dto.UpstreamPerformanceModelHistory{{ModelName: "gpt-4o", SeriesSchema: "ts,avg", Groups: []dto.UpstreamPerformanceGroupHistory{{Group: "default", Series: []dto.UpstreamPerformanceBucket{bucket}}}}}}
}
func TestA63PerformanceHistoryAverageBoundaryAndWeightedCounters(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	db := openCoreAcceptanceTransaction(t)
	now := int64(2101000000)
	bucket := now - 60
	cipher := newCoreCipher(t)
	sites := []model.Site{createCoreAuthorizedSite(t, db, cipher, now), createCoreAuthorizedSite(t, db, cipher, now+1)}
	for _, site := range sites {
		if _, err := model.NewSiteRepository(db).ApplyPerformanceHistorySnapshot(context.Background(), site, now, now-3600, now+1, perfHistory(bucket, false, 0, 0, 0, 0, 0, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := service.NewPerformanceHistoryService(db)
	if err != nil {
		t.Fatal(err)
	}
	query := dto.PerformanceHistoryQuery{Page: 1, PageSize: 100, StartTimestamp: now - 3600, EndTimestamp: now + 1, SiteIDs: []int64{sites[0].ID, sites[1].ID}}
	average, err := svc.Statistics(context.Background(), query)
	if err != nil || average.AggregationStatus != "unavailable" || len(average.SiteBreakdown) != 2 || average.Summary.SuccessRate != nil {
		t.Fatalf("average-only stats=%#v err=%v", average, err)
	}
	counterFixtures := []dto.UpstreamPerformanceHistory{perfHistory(bucket, true, 1, 1, 100, 50, 1, 100, 1000), perfHistory(bucket, true, 9, 0, 9000, 4500, 9, 900, 9000)}
	for i, site := range sites {
		if _, err := model.NewSiteRepository(db).ApplyPerformanceHistorySnapshot(context.Background(), site, now+2, now-3600, now+3, counterFixtures[i]); err != nil {
			t.Fatal(err)
		}
	}
	weighted, err := svc.Statistics(context.Background(), query)
	if err != nil || weighted.AggregationStatus != "complete" || weighted.Summary.SuccessRate == nil || *weighted.Summary.SuccessRate != "0.1000000000" || weighted.Summary.AvgLatencyMS == nil || *weighted.Summary.AvgLatencyMS != "910.0000000000" {
		t.Fatalf("weighted stats=%#v err=%v", weighted, err)
	}
}
