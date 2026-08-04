package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"strconv"
)

type PerformanceHistoryService struct {
	repository *model.PerformanceHistoryRepository
}

var ErrPerformanceHistoryTooLarge = errors.New("performance history result set is too large")

func NewPerformanceHistoryService(db *gorm.DB) (*PerformanceHistoryService, error) {
	if db == nil {
		return nil, errors.New("performance history database is required")
	}
	return &PerformanceHistoryService{repository: model.NewPerformanceHistoryRepository(db)}, nil
}
func (s *PerformanceHistoryService) List(ctx context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryPage, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.PerformanceHistoryPage{}, ErrStatisticsInvalid
	}
	rows, total, err := s.repository.List(ctx, q)
	if err != nil {
		return dto.PerformanceHistoryPage{}, err
	}
	coverage, err := s.repository.CollectionCoverage(ctx, q.SiteIDs)
	if err != nil {
		return dto.PerformanceHistoryPage{}, err
	}
	items := performanceHistoryItems(rows)
	return dto.PerformanceHistoryPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: performanceCollectionStatus(coverage), AsOf: coverage.AsOf}, nil
}
func (s *PerformanceHistoryService) Statistics(ctx context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 100
	if s == nil || q.Validate() != nil {
		return dto.PerformanceHistoryStatisticsResponse{}, ErrStatisticsInvalid
	}
	rows, err := s.repository.All(ctx, q)
	if err != nil {
		if errors.Is(err, model.ErrPerformanceHistoryResultTooLarge) {
			return dto.PerformanceHistoryStatisticsResponse{}, ErrPerformanceHistoryTooLarge
		}
		return dto.PerformanceHistoryStatisticsResponse{}, err
	}
	coverage, err := s.repository.CollectionCoverage(ctx, q.SiteIDs)
	if err != nil {
		return dto.PerformanceHistoryStatisticsResponse{}, err
	}
	items := performanceHistoryItems(rows)
	out := dto.PerformanceHistoryStatisticsResponse{Trend: items, SiteBreakdown: items, AggregationStatus: "unavailable", DataStatus: performanceCollectionStatus(coverage), UnavailableReason: "upstream_standard_api_missing_counters"}
	if len(rows) == 0 {
		return out, nil
	}
	success, latency, ttft, tps, requests, ok := model.WeightedPerformance(rows)
	if ok {
		out.Summary = dto.PerformanceWeightedMetric{SuccessRate: &success, AvgLatencyMS: &latency, AvgTTFTMS: &ttft, AvgTPS: &tps, RequestCount: &requests}
		out.AggregationStatus = "complete"
		out.UnavailableReason = ""
	}
	return out, nil
}

func performanceCollectionStatus(coverage model.PerformanceCollectionCoverage) string {
	if coverage.SiteCount == 0 || coverage.SuccessfulSites == coverage.SiteCount {
		return "complete"
	}
	if coverage.UnavailableSites == coverage.SiteCount {
		return "unavailable"
	}
	if coverage.SuccessfulSites > 0 || coverage.UnavailableSites > 0 {
		return "partial"
	}
	return "pending"
}
func performanceHistoryItems(rows []model.PerformanceHistoryReadRow) []dto.PerformanceHistoryItem {
	out := make([]dto.PerformanceHistoryItem, 0, len(rows))
	for _, r := range rows {
		c := dto.PerformanceCounterSet{RequestCount: int64StringPointer(r.RequestCount), SuccessCount: int64StringPointer(r.SuccessCount), TotalLatencyMS: int64StringPointer(r.TotalLatencyMS), TTFTSumMS: int64StringPointer(r.TTFTSumMS), TTFTCount: int64StringPointer(r.TTFTCount), OutputTokens: int64StringPointer(r.OutputTokens), GenerationMS: int64StringPointer(r.GenerationMS)}
		out = append(out, dto.PerformanceHistoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, ModelName: r.ModelName, Group: r.RemoteGroup, BucketStart: r.BucketTS, SeriesSchema: r.SeriesSchema, MetricSource: r.MetricSource, AvgTTFTMS: r.AvgTTFTMS, AvgLatencyMS: r.AvgLatencyMS, SuccessRate: r.SuccessRate, AvgTPS: r.AvgTPS, PerformanceCounterSet: c, CollectedAt: r.CollectedAt})
	}
	return out
}
func int64StringPointer(value *int64) *string {
	if value == nil {
		return nil
	}
	result := strconv.FormatInt(*value, 10)
	return &result
}
