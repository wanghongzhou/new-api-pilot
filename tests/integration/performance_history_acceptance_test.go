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
	queryEnd := now - now%3600 + 3600
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
	query := dto.PerformanceHistoryQuery{Page: 1, PageSize: 100, StartTimestamp: queryEnd - 3600, EndTimestamp: queryEnd, SiteIDs: []int64{sites[0].ID, sites[1].ID}}
	average, err := svc.Statistics(context.Background(), query)
	if err != nil || average.AggregationStatus != "unavailable" || len(average.SiteBreakdown) != 2 || len(average.Trend) != 2 || average.Trend[0].SiteID == "" || average.Trend[0].ModelName == "" || average.ModelBreakdown == nil || average.GroupBreakdown == nil || average.Summary.SuccessRate != nil {
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
	if len(weighted.Trend) != 2 || len(weighted.ModelBreakdown) != 1 || weighted.ModelBreakdown[0].Dimension != "gpt-4o" || len(weighted.GroupBreakdown) != 1 || weighted.GroupBreakdown[0].Dimension != "default" {
		t.Fatalf("weighted breakdown contract=%#v", weighted)
	}
}

func TestPerformanceHistorySuccessfulEmptySnapshotIsComplete(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	db := openCoreAcceptanceTransaction(t)
	now := int64(2101000000)
	queryEnd := now - now%3600 + 3600
	cipher := newCoreCipher(t)
	site := createCoreAuthorizedSite(t, db, cipher, now)
	if written, err := model.NewSiteRepository(db).ApplyPerformanceHistorySnapshot(
		context.Background(), site, now, now-24*3600, now, dto.UpstreamPerformanceHistory{},
	); err != nil || written != 0 {
		t.Fatalf("apply empty performance snapshot=%d err=%v", written, err)
	}
	svc, err := service.NewPerformanceHistoryService(db)
	if err != nil {
		t.Fatal(err)
	}
	query := dto.PerformanceHistoryQuery{
		Page: 1, PageSize: 100, StartTimestamp: queryEnd - 24*3600, EndTimestamp: queryEnd, SiteIDs: []int64{site.ID},
	}
	page, err := svc.List(context.Background(), query)
	if err != nil || page.Total != 0 || page.DataStatus != "complete" || page.AsOf == nil || *page.AsOf != now {
		t.Fatalf("empty performance page=%#v err=%v", page, err)
	}
	statistics, err := svc.Statistics(context.Background(), query)
	if err != nil || statistics.DataStatus != "complete" || len(statistics.Trend) != 0 {
		t.Fatalf("empty performance statistics=%#v err=%v", statistics, err)
	}

	pendingSite := createCoreAuthorizedSite(t, db, cipher, now+1)
	query.SiteIDs = []int64{pendingSite.ID}
	pending, err := svc.List(context.Background(), query)
	if err != nil || pending.DataStatus != "pending" || pending.AsOf != nil {
		t.Fatalf("pending performance page=%#v err=%v", pending, err)
	}
}

func TestPerformanceHistoryBackfillCompletionIsDurable(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	db := openCoreAcceptanceTransaction(t)
	now := int64(2101001000)
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	repository := model.NewSiteRepository(db)
	required, err := repository.PerformanceBackfillRequired(context.Background(), site.ID)
	if err != nil || !required {
		t.Fatalf("initial performance backfill required=%v err=%v", required, err)
	}
	if _, err := repository.ApplyPerformanceHistorySnapshot(
		context.Background(), site, now, now-720*3600, now+1, dto.UpstreamPerformanceHistory{},
	); err != nil {
		t.Fatalf("complete performance backfill: %v", err)
	}
	required, err = repository.PerformanceBackfillRequired(context.Background(), site.ID)
	if err != nil || required {
		t.Fatalf("completed performance backfill required=%v err=%v", required, err)
	}
	var state model.SitePerformanceCollectionState
	if err := db.Where("site_id = ?", site.ID).Take(&state).Error; err != nil || state.BackfillCompletedAt == nil || *state.BackfillCompletedAt != now {
		t.Fatalf("performance backfill state=%#v err=%v", state, err)
	}
}
