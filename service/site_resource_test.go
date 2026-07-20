package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSiteResourceBuilderRepresentsZeroPausedMissingAndPendingMinutes(t *testing.T) {
	start := time.Date(2025, time.July, 13, 12, 0, 0, 0, siteResourceLocation).Unix()
	now := start + 3*60 + 30
	zero := 0
	asOf := start
	builder := siteResourceBuilder{
		siteID: 7,
		query: dto.ResourceQuery{StartTimestamp: start, EndTimestamp: start + 4*60,
			Granularity: dto.ResourceGranularityMinute},
		now: now, lifecycleStart: start,
		rows: []model.SiteResourceRow{{
			BucketStart: start, InstanceCount: &zero, OnlineInstanceCount: &zero,
			SampleCount: 1, ExpectedSampleCount: 1, HealthStatus: constant.SiteHealthOK,
			DataStatus: "complete", SourceAsOf: &asOf,
		}},
		pauses: []model.SiteMonitoringPause{{SiteID: 7, StartMinuteTS: start + 60, EndMinuteTS: int64PointerForResource(start + 120)}},
	}
	trend, err := builder.buildTrend()
	if err != nil {
		t.Fatalf("buildTrend() error = %v", err)
	}
	if len(trend) != 4 {
		t.Fatalf("trend length = %d, want 4", len(trend))
	}
	wantStatuses := []string{"complete", "paused", "missing", "pending"}
	for index, want := range wantStatuses {
		if trend[index].DataStatus != want {
			t.Fatalf("trend[%d].data_status = %q, want %q", index, trend[index].DataStatus, want)
		}
		if want == "complete" && trend[index].Reason != nil || want != "complete" && trend[index].Reason == nil {
			t.Fatalf("trend[%d].reason = %#v for %s", index, trend[index].Reason, want)
		}
	}
	if trend[0].InstanceCount == nil || *trend[0].InstanceCount != 0 || trend[0].CPUMaxPercent != nil {
		t.Fatalf("successful zero-instance point = %#v", trend[0])
	}
	if trend[2].CPUMaxPercent != nil || trend[2].InstanceCount != nil {
		t.Fatalf("missing point exposed fake zero metrics = %#v", trend[2])
	}
	summary := builder.buildSummary(trend)
	if summary == nil || summary.SampleCount != 1 || summary.ExpectedSampleCount != 3 || summary.DataStatus != "partial" || summary.Reason == nil {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSiteResourceBuilderAggregatesOnlyKnownPartialMetrics(t *testing.T) {
	start := time.Date(2025, time.July, 13, 8, 0, 0, 0, siteResourceLocation).Unix()
	cpu10, cpu20, cpu30, memory40, disk50, diskLast51 := 10.0, 20.0, 30.0, 40.0, 50.0, 51.0
	asOf1, asOf2 := start+3600, start+2*3600
	builder := siteResourceBuilder{
		siteID: 9,
		query: dto.ResourceQuery{StartTimestamp: start, EndTimestamp: start + 2*3600,
			Granularity: dto.ResourceGranularityHour},
		now: start + 3*3600, lifecycleStart: start,
		rows: []model.SiteResourceRow{
			{
				BucketStart: start, CPUMaxPercent: &cpu20, CPUAvgPercent: &cpu10,
				MemoryMaxPercent: &memory40, MemoryAvgPercent: &cpu20, DiskMaxUsedPercent: &disk50,
				SampleCount: 60, ExpectedSampleCount: 60, HealthStatus: constant.SiteHealthOK,
				DataStatus: "complete", SourceAsOf: &asOf1,
			},
			{
				BucketStart: start + 3600, CPUMaxPercent: &cpu30, CPUAvgPercent: &cpu30,
				MemoryMaxPercent: &memory40, MemoryAvgPercent: &memory40,
				DiskMaxUsedPercent: &diskLast51, DiskLastUsedPercent: &diskLast51,
				SampleCount: 30, ExpectedSampleCount: 60, HealthStatus: constant.SiteHealthWarning,
				DataStatus: "partial", SourceAsOf: &asOf2,
			},
		},
	}
	trend, err := builder.buildTrend()
	if err != nil {
		t.Fatalf("buildTrend() error = %v", err)
	}
	summary := builder.buildSummary(trend)
	if summary == nil || summary.DataStatus != "partial" || summary.SampleCount != 90 || summary.ExpectedSampleCount != 120 {
		t.Fatalf("summary coverage = %#v", summary)
	}
	wantCPUAvg := (cpu10*60 + cpu30*30) / 90
	if summary.CPUAvgPercent == nil || math.Abs(*summary.CPUAvgPercent-wantCPUAvg) > 1e-9 ||
		summary.CPUMaxPercent == nil || *summary.CPUMaxPercent != cpu30 {
		t.Fatalf("summary CPU = %#v, want avg %f max %f", summary, wantCPUAvg, cpu30)
	}
	if summary.DiskLastUsedPercent == nil || *summary.DiskLastUsedPercent != diskLast51 ||
		summary.AsOf == nil || *summary.AsOf != asOf2 || summary.IsFinal {
		t.Fatalf("summary freshness/finality = %#v", summary)
	}
}

func TestSiteResourceBuilderUsesPersistedDailyFinality(t *testing.T) {
	start := time.Date(2025, time.July, 12, 0, 0, 0, 0, siteResourceLocation).Unix()
	asOf := start + 24*60*60 + 60
	builder := siteResourceBuilder{
		siteID: 11,
		query: dto.ResourceQuery{StartTimestamp: start, EndTimestamp: start + 24*60*60,
			Granularity: dto.ResourceGranularityDay},
		now: start + 2*24*60*60, lifecycleStart: start,
		rows: []model.SiteResourceRow{{
			DateKey: siteResourceDateKey(start), SampleCount: 1440, ExpectedSampleCount: 1440,
			HealthStatus: constant.SiteHealthOK, DataStatus: "complete", SourceAsOf: &asOf, IsFinal: true,
		}},
	}
	trend, err := builder.buildTrend()
	if err != nil {
		t.Fatalf("buildTrend() error = %v", err)
	}
	if len(trend) != 1 || !trend[0].IsFinal {
		t.Fatalf("daily trend = %#v", trend)
	}
}

func TestValidMinuteResourceRangeRequiresClosedBoundedRetention(t *testing.T) {
	now := int64(1_752_400_830)
	closedEnd := floorMinute(now)
	valid := dto.ResourceQuery{
		StartTimestamp: closedEnd - 24*60*60, EndTimestamp: closedEnd,
		Granularity: dto.ResourceGranularityMinute,
	}
	if !validMinuteResourceRange(valid, now, 1) {
		t.Fatal("valid closed retention range was rejected")
	}
	tests := []dto.ResourceQuery{
		{StartTimestamp: valid.StartTimestamp, EndTimestamp: closedEnd + 60, Granularity: dto.ResourceGranularityMinute},
		{StartTimestamp: valid.StartTimestamp - 60, EndTimestamp: closedEnd, Granularity: dto.ResourceGranularityMinute},
		{StartTimestamp: valid.StartTimestamp - 60, EndTimestamp: closedEnd - 60, Granularity: dto.ResourceGranularityMinute},
	}
	for _, query := range tests {
		if validMinuteResourceRange(query, now, 1) {
			t.Fatalf("invalid minute range was accepted: %#v", query)
		}
	}
}

func TestValidClosedResourceRangeRejectsOpenAndExcessiveBucketsBeforeAllocation(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2025, time.July, 14, 10, 30, 30, 0, location).Unix()
	hourEnd := time.Date(2025, time.July, 14, 10, 0, 0, 0, location).Unix()
	dayEnd := time.Date(2025, time.July, 14, 0, 0, 0, 0, location).Unix()
	tests := []struct {
		query dto.ResourceQuery
		valid bool
	}{
		{query: dto.ResourceQuery{StartTimestamp: hourEnd - 3600, EndTimestamp: hourEnd, Granularity: dto.ResourceGranularityHour}, valid: true},
		{query: dto.ResourceQuery{StartTimestamp: hourEnd, EndTimestamp: hourEnd + 3600, Granularity: dto.ResourceGranularityHour}},
		{query: dto.ResourceQuery{StartTimestamp: dayEnd - 24*60*60, EndTimestamp: dayEnd, Granularity: dto.ResourceGranularityDay}, valid: true},
		{query: dto.ResourceQuery{StartTimestamp: dayEnd, EndTimestamp: dayEnd + 24*60*60, Granularity: dto.ResourceGranularityDay}},
		{query: dto.ResourceQuery{
			StartTimestamp: hourEnd - int64(dto.ResourceMaximumBuckets+1)*3600,
			EndTimestamp:   hourEnd, Granularity: dto.ResourceGranularityHour,
		}},
	}
	for _, test := range tests {
		if got := validClosedResourceRange(test.query, now); got != test.valid {
			t.Fatalf("validClosedResourceRange(%#v) = %v, want %v", test.query, got, test.valid)
		}
	}
}

func TestResourceStatusRejectsOpenOrExcessiveRangeBeforeRepositoryAccess(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2025, time.July, 14, 10, 30, 30, 0, location)
	service := &SiteService{clock: testsupport.NewFakeClock(now)}
	hourStart := time.Date(2025, time.July, 14, 10, 0, 0, 0, location).Unix()
	for _, query := range []dto.ResourceQuery{
		{StartTimestamp: hourStart, EndTimestamp: hourStart + 3600, Granularity: dto.ResourceGranularityHour},
		{
			StartTimestamp: floorMinute(now.Unix()) - int64(dto.ResourceMaximumBuckets+1)*60,
			EndTimestamp:   floorMinute(now.Unix()), Granularity: dto.ResourceGranularityMinute,
		},
	} {
		if _, err := service.ResourceStatus(context.Background(), 1, query); !errors.Is(err, ErrSiteResourceRange) {
			t.Fatalf("ResourceStatus(%#v) error = %v, want ErrSiteResourceRange", query, err)
		}
	}
	excessive := dto.ResourceQuery{
		StartTimestamp: hourStart - int64(dto.ResourceMaximumBuckets+1)*3600,
		EndTimestamp:   hourStart, Granularity: dto.ResourceGranularityHour,
	}
	if buckets := resourceBuckets(excessive); len(buckets) != 0 {
		t.Fatalf("resourceBuckets allocated %d excessive buckets", len(buckets))
	}
}

func int64PointerForResource(value int64) *int64 { return &value }
