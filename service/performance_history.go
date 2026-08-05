package service

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"sort"
	"strconv"
)

type PerformanceHistoryService struct {
	database *gorm.DB
}

var ErrPerformanceHistoryTooLarge = errors.New("performance history result set is too large")

func NewPerformanceHistoryService(db *gorm.DB) (*PerformanceHistoryService, error) {
	if db == nil {
		return nil, errors.New("performance history database is required")
	}
	return &PerformanceHistoryService{database: db}, nil
}

func (s *PerformanceHistoryService) readSnapshot(ctx context.Context, read func(*model.PerformanceHistoryRepository) error) error {
	if s == nil || s.database == nil || read == nil {
		return errors.New("performance history snapshot dependencies are required")
	}
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(model.NewPerformanceHistoryRepository(tx))
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func (s *PerformanceHistoryService) List(ctx context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryPage, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.PerformanceHistoryPage{}, ErrStatisticsInvalid
	}
	var rows []model.PerformanceHistoryReadRow
	var coverage model.PerformanceCollectionCoverage
	var total int64
	if err := s.readSnapshot(ctx, func(repository *model.PerformanceHistoryRepository) error {
		var err error
		if rows, total, err = repository.List(ctx, q); err != nil {
			return err
		}
		coverage, err = repository.CollectionCoverage(ctx, q.SiteIDs)
		return err
	}); err != nil {
		return dto.PerformanceHistoryPage{}, err
	}
	items := performanceHistoryItems(rows)
	complete := performanceCompleteness(coverage)
	return dto.PerformanceHistoryPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: complete.DataStatus, AsOf: coverage.AsOf, Completeness: complete}, nil
}
func (s *PerformanceHistoryService) Statistics(ctx context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 100
	if s == nil || q.Validate() != nil {
		return dto.PerformanceHistoryStatisticsResponse{}, ErrStatisticsInvalid
	}
	var rows []model.PerformanceHistoryReadRow
	var coverage model.PerformanceCollectionCoverage
	if err := s.readSnapshot(ctx, func(repository *model.PerformanceHistoryRepository) error {
		var readErr error
		if rows, readErr = repository.All(ctx, q); readErr != nil {
			return readErr
		}
		coverage, readErr = repository.CollectionCoverage(ctx, q.SiteIDs)
		return readErr
	}); err != nil {
		if errors.Is(err, model.ErrPerformanceHistoryResultTooLarge) {
			return dto.PerformanceHistoryStatisticsResponse{}, ErrPerformanceHistoryTooLarge
		}
		return dto.PerformanceHistoryStatisticsResponse{}, err
	}
	items := performanceHistoryItems(rows)
	complete := performanceCompleteness(coverage)
	out := dto.PerformanceHistoryStatisticsResponse{
		Trend:             items,
		ModelBreakdown:    []dto.PerformanceDimensionBreakdown{},
		GroupBreakdown:    []dto.PerformanceDimensionBreakdown{},
		SiteBreakdown:     items,
		AggregationStatus: "unavailable",
		DataStatus:        complete.DataStatus,
		AsOf:              coverage.AsOf,
		Completeness:      complete,
		UnavailableReason: "upstream_standard_api_missing_counters",
	}
	if len(rows) == 0 {
		return out, nil
	}
	success, latency, ttft, tps, requests, ok := model.WeightedPerformance(rows)
	if ok {
		out.Summary = dto.PerformanceWeightedMetric{SuccessRate: &success, AvgLatencyMS: &latency, AvgTTFTMS: &ttft, AvgTPS: &tps, RequestCount: &requests}
		out.ModelBreakdown = weightedPerformanceDimensionBreakdown(rows, func(row model.PerformanceHistoryReadRow) string { return row.ModelName })
		out.GroupBreakdown = weightedPerformanceDimensionBreakdown(rows, func(row model.PerformanceHistoryReadRow) string { return row.RemoteGroup })
		out.AggregationStatus = "complete"
		out.UnavailableReason = ""
	}
	return out, nil
}

func performanceCompleteness(coverage model.PerformanceCollectionCoverage) dto.PerformanceCompleteness {
	return dto.PerformanceCompleteness{DataStatus: performanceCollectionStatus(coverage), SuccessfulSiteCount: coverage.SuccessfulSites, UnavailableSiteCount: coverage.UnavailableSites, ExpectedSiteCount: coverage.SiteCount}
}

func weightedPerformanceDimensionBreakdown(rows []model.PerformanceHistoryReadRow, key func(model.PerformanceHistoryReadRow) string) []dto.PerformanceDimensionBreakdown {
	grouped := make(map[string][]model.PerformanceHistoryReadRow)
	for _, row := range rows {
		value := key(row)
		grouped[value] = append(grouped[value], row)
	}
	keys := make([]string, 0, len(grouped))
	for value := range grouped {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	out := make([]dto.PerformanceDimensionBreakdown, 0, len(keys))
	for _, value := range keys {
		group := grouped[value]
		success, latency, ttft, tps, requests, ok := model.WeightedPerformance(group)
		if !ok {
			continue
		}
		requestCount := requests
		out = append(out, dto.PerformanceDimensionBreakdown{
			Dimension: value,
			PerformanceWeightedMetric: dto.PerformanceWeightedMetric{
				SuccessRate:  &success,
				AvgLatencyMS: &latency,
				AvgTTFTMS:    &ttft,
				AvgTPS:       &tps,
				RequestCount: &requestCount,
			},
		})
	}
	return out
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
