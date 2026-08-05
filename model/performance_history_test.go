package model

import (
	"context"
	"fmt"
	"new-api-pilot/dto"
	"testing"
	"time"
)

func p64(v int64) *int64 { return &v }
func TestWeightedPerformanceUsesCounterNumeratorsAndDenominators(t *testing.T) {
	rows := []PerformanceHistoryReadRow{{SitePerformanceMetricBucket: SitePerformanceMetricBucket{MetricSource: PerformanceMetricSourceCounterReady, RequestCount: p64(1), SuccessCount: p64(1), TotalLatencyMS: p64(100), TTFTSumMS: p64(50), TTFTCount: p64(1), OutputTokens: p64(100), GenerationMS: p64(1000)}}, {SitePerformanceMetricBucket: SitePerformanceMetricBucket{MetricSource: PerformanceMetricSourceCounterReady, RequestCount: p64(9), SuccessCount: p64(0), TotalLatencyMS: p64(9000), TTFTSumMS: p64(4500), TTFTCount: p64(9), OutputTokens: p64(900), GenerationMS: p64(9000)}}}
	success, latency, ttft, tps, requests, ok := WeightedPerformance(rows)
	if !ok || requests != "10" || success != "0.1000000000" || latency != "910.0000000000" || ttft != "455.0000000000" || tps != "100.0000000000" {
		t.Fatalf("weighted=%s/%s/%s/%s requests=%s ok=%t", success, latency, ttft, tps, requests, ok)
	}
	rows[1].MetricSource = PerformanceMetricSourceOfficialAverage
	if _, _, _, _, _, ok = WeightedPerformance(rows); ok {
		t.Fatal("average-only rows produced a weighted aggregate")
	}
}
func TestPerformanceHistorySnapshotAverageOnlyAndConfigFence(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2100800000)
	site := createRunnableSite(t, db, fmt.Sprintf("performance-%d", time.Now().UnixNano()), now)
	history := dto.UpstreamPerformanceHistory{Models: []dto.UpstreamPerformanceModelHistory{{ModelName: "gpt-4o", SeriesSchema: "ts,avg", Groups: []dto.UpstreamPerformanceGroupHistory{{Group: "default", Series: []dto.UpstreamPerformanceBucket{{Timestamp: now - 60, AvgTTFTMS: "10.5", AvgLatencyMS: "20.25", SuccessRate: "0.9", AvgTPS: "30.125"}}}}}}}
	written, err := NewSiteRepository(db.GORM).ApplyPerformanceHistorySnapshot(context.Background(), site, now, now-3600, now+1, history)
	if err != nil || written != 1 {
		t.Fatalf("apply average history=%d err=%v", written, err)
	}
	var row SitePerformanceMetricBucket
	if err := db.GORM.Where("site_id=?", site.ID).Take(&row).Error; err != nil || row.MetricSource != PerformanceMetricSourceOfficialAverage || row.RequestCount != nil || row.AvgLatencyMS != "20.2500000000" {
		t.Fatalf("average row=%+v err=%v", row, err)
	}
	var state SitePerformanceCollectionState
	if err := db.GORM.First(&state, site.ID).Error; err != nil || state.CapabilityStatus != "average_only" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	stale := site
	stale.ConfigVersion++
	if _, err := NewSiteRepository(db.GORM).ApplyPerformanceHistorySnapshot(context.Background(), stale, now+1, now-3600, now+1, history); err == nil {
		t.Fatal("stale config snapshot committed")
	}
	var count int64
	_ = db.GORM.Model(&SitePerformanceMetricBucket{}).Where("site_id=?", site.ID).Count(&count).Error
	if count != 1 {
		t.Fatalf("stale snapshot changed rows=%d", count)
	}
	start := (now / 3600) * 3600
	old := SitePerformanceMetricBucket{SiteID: site.ID, ModelName: "old-model", RemoteGroup: "default", BucketTS: start + 3600, SeriesSchema: "ts,avg", MetricSource: PerformanceMetricSourceOfficialAverage, AvgTTFTMS: "1", AvgLatencyMS: "1", SuccessRate: "1", AvgTPS: "1", ConfigVersion: site.ConfigVersion - 1, CollectedAt: now - 1, CreatedAt: now - 1, UpdatedAt: now - 1}
	future := SitePerformanceMetricBucket{SiteID: site.ID, ModelName: "future-model", RemoteGroup: "default", BucketTS: start + 7200, SeriesSchema: "ts,avg", MetricSource: PerformanceMetricSourceOfficialAverage, AvgTTFTMS: "1", AvgLatencyMS: "1", SuccessRate: "1", AvgTPS: "1", ConfigVersion: site.ConfigVersion, CollectedAt: now + 100, CreatedAt: now + 100, UpdatedAt: now + 100}
	if err := db.GORM.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Create(&future).Error; err != nil {
		t.Fatal(err)
	}
	query := dto.PerformanceHistoryQuery{Page: 1, PageSize: 20, StartTimestamp: start - 3600, EndTimestamp: start + 10800, SiteIDs: []int64{site.ID}, SnapshotAt: now}
	items, total, err := NewPerformanceHistoryRepository(db.GORM).List(context.Background(), query)
	if err != nil || total != 1 || len(items) != 1 || items[0].ModelName != "gpt-4o" {
		t.Fatalf("current snapshot rows=%#v total=%d err=%v", items, total, err)
	}
}
