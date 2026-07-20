package service

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type ChannelInventoryService struct {
	repository *model.SiteChannelInventoryRepository
}

const (
	ChannelMetricAvailableCount    = "channel.available_count"
	ChannelMetricUnavailableCount  = "channel.unavailable_count"
	ChannelMetricAvailabilityRate  = "channel.availability_rate"
	ChannelMetricBalanceTotal      = "channel.balance_total"
	ChannelMetricResponseTimeAvgMS = "channel.response_time_avg_ms"
	ChannelMetricResponseTimeMaxMS = "channel.response_time_max_ms"
)

type ChannelAlertRuleContract struct{ Key, Metric, Operator, Unit string }

var ChannelAlertRuleContracts = []ChannelAlertRuleContract{
	{Key: "channel_balance_low", Metric: ChannelMetricBalanceTotal, Operator: "<=", Unit: "decimal"},
	{Key: "channel_response_time_high", Metric: ChannelMetricResponseTimeAvgMS, Operator: ">=", Unit: "ms"},
	{Key: "channel_availability_low", Metric: ChannelMetricAvailabilityRate, Operator: "<=", Unit: "ratio"},
}

func NewChannelInventoryService(db *gorm.DB) (*ChannelInventoryService, error) {
	if db == nil {
		return nil, errors.New("channel inventory database is required")
	}
	return &ChannelInventoryService{repository: model.NewSiteChannelInventoryRepository(db)}, nil
}
func (s *ChannelInventoryService) List(ctx context.Context, q dto.ChannelInventoryQuery) (dto.ChannelInventoryPage, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.ChannelInventoryPage{}, ErrStatisticsInvalid
	}
	rows, total, err := s.repository.List(ctx, q)
	if err != nil {
		return dto.ChannelInventoryPage{}, err
	}
	items := make([]dto.ChannelInventoryItem, 0, len(rows))
	var asOf *int64
	for _, r := range rows {
		items = append(items, dto.ChannelInventoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, RemoteChannelID: strconv.FormatInt(r.RemoteChannelID, 10), Name: r.Name, Type: r.RemoteType, Status: r.RemoteStatus, TestTime: r.TestTime, ResponseTimeMS: strconv.FormatInt(r.ResponseTimeMS, 10), Balance: r.Balance, BalanceUpdatedAt: r.BalanceUpdatedAt, Models: r.Models, Group: r.RemoteGroup, UsedQuota: strconv.FormatInt(r.UsedQuota, 10), Priority: strconv.FormatInt(r.Priority, 10), Weight: strconv.FormatInt(r.Weight, 10), AutoBan: r.AutoBan, Tag: r.Tag, RemoteState: r.RemoteState, MissingCount: r.MissingCount, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
		v := r.UpdatedAt
		if asOf == nil || v > *asOf {
			asOf = &v
		}
	}
	status := "complete"
	if total == 0 {
		status = "pending"
	}
	return dto.ChannelInventoryPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: status, AsOf: asOf}, nil
}
func (s *ChannelInventoryService) Statistics(ctx context.Context, q dto.ChannelInventoryStatisticsQuery) (dto.ChannelInventoryStatisticsResponse, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.ChannelInventoryStatisticsResponse{}, ErrStatisticsInvalid
	}
	summary, err := s.repository.Current(ctx, q, "summary")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	types, err := s.repository.Current(ctx, q, "type")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	statuses, err := s.repository.Current(ctx, q, "status")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	groups, err := s.repository.Current(ctx, q, "group")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	tags, err := s.repository.Current(ctx, q, "tag")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	sites, err := s.repository.Current(ctx, q, "site")
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	trend, err := s.repository.Trend(ctx, q)
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	out := dto.ChannelInventoryStatisticsResponse{Trend: channelTrend(trend), TypeBreakdown: channelBreakdown(types), StatusBreakdown: channelBreakdown(statuses), GroupBreakdown: channelBreakdown(groups), TagBreakdown: channelBreakdown(tags), SiteBreakdown: channelBreakdown(sites), DataStatus: "complete"}
	if len(summary) > 0 {
		out.Summary = channelMetric(summary[0])
	} else {
		out.Summary = emptyChannelMetric()
		out.DataStatus = "pending"
	}
	return out, nil
}
func channelMetric(r model.SiteChannelInventoryMetricRow) dto.ChannelInventoryMetric {
	return dto.ChannelInventoryMetric{ChannelCount: strconv.FormatInt(r.ChannelCount, 10), AvailableCount: strconv.FormatInt(r.AvailableCount, 10), UnavailableCount: strconv.FormatInt(r.UnavailableCount, 10), MissingCount: strconv.FormatInt(r.MissingCount, 10), BalanceTotal: r.BalanceTotal, UsedQuota: strconv.FormatInt(r.UsedQuota, 10), ResponseTimeAvgMS: r.ResponseTimeAvgMS, ResponseTimeMaxMS: strconv.FormatInt(r.ResponseTimeMaxMS, 10), AvailabilityRate: r.AvailabilityRate}
}
func emptyChannelMetric() dto.ChannelInventoryMetric {
	return dto.ChannelInventoryMetric{ChannelCount: "0", AvailableCount: "0", UnavailableCount: "0", MissingCount: "0", BalanceTotal: "0", UsedQuota: "0", ResponseTimeAvgMS: "0", ResponseTimeMaxMS: "0", AvailabilityRate: "0"}
}
func channelBreakdown(rows []model.SiteChannelInventoryMetricRow) []dto.ChannelInventoryBreakdown {
	out := make([]dto.ChannelInventoryBreakdown, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ChannelInventoryBreakdown{DimensionID: r.DimensionID, DimensionName: r.DimensionName, SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, ChannelInventoryMetric: channelMetric(r), DataStatus: "complete", AsOf: r.AsOf})
	}
	return out
}
func channelTrend(rows []model.SiteChannelInventoryMetricRow) []dto.ChannelInventoryTrendPoint {
	out := make([]dto.ChannelInventoryTrendPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ChannelInventoryTrendPoint{BucketStart: r.BucketStart, BucketEnd: r.BucketStart + 3600, ChannelInventoryMetric: channelMetric(r), DataStatus: "complete"})
	}
	return out
}
