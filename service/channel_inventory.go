package service

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type ChannelInventoryService struct {
	db *gorm.DB
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
	return &ChannelInventoryService{db: db}, nil
}
func (s *ChannelInventoryService) List(ctx context.Context, q dto.ChannelInventoryQuery) (dto.ChannelInventoryPage, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.ChannelInventoryPage{}, ErrStatisticsInvalid
	}
	var rows []model.SiteChannelInventoryReadRow
	var completeness []model.SiteChannelInventoryCompletenessRow
	var total int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewSiteChannelInventoryRepository(tx)
		var err error
		rows, total, err = repo.List(ctx, q)
		if err != nil {
			return err
		}
		completeness, err = repo.Completeness(ctx, q.SiteIDs)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return dto.ChannelInventoryPage{}, err
	}
	items := make([]dto.ChannelInventoryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.ChannelInventoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, RemoteChannelID: strconv.FormatInt(r.RemoteChannelID, 10), Name: r.Name, Type: r.RemoteType, Status: r.RemoteStatus, TestTime: r.TestTime, ResponseTimeMS: strconv.FormatInt(r.ResponseTimeMS, 10), Balance: r.Balance, BalanceUpdatedAt: r.BalanceUpdatedAt, Models: r.Models, Group: r.RemoteGroup, UsedQuota: strconv.FormatInt(r.UsedQuota, 10), Priority: strconv.FormatInt(r.Priority, 10), Weight: strconv.FormatInt(r.Weight, 10), AutoBan: r.AutoBan, Tag: r.Tag, RemoteState: r.RemoteState, MissingCount: r.MissingCount, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
	}
	var asOf *int64
	for _, row := range completeness {
		if row.AsOf != nil && (asOf == nil || *row.AsOf > *asOf) {
			value := *row.AsOf
			asOf = &value
		}
	}
	return dto.ChannelInventoryPage{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: channelOverallStatus(completeness), AsOf: asOf}, nil
}
func (s *ChannelInventoryService) Statistics(ctx context.Context, q dto.ChannelInventoryStatisticsQuery) (dto.ChannelInventoryStatisticsResponse, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.ChannelInventoryStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summary, types, statuses, groups, tags, sites, trend []model.SiteChannelInventoryMetricRow
	var completeness []model.SiteChannelInventoryCompletenessRow
	var coverage []model.SiteChannelInventoryCoverageRow
	var legacyTrend bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewSiteChannelInventoryRepository(tx)
		var err error
		if summary, err = repo.Current(ctx, q, "summary"); err != nil {
			return err
		}
		if types, err = repo.Current(ctx, q, "type"); err != nil {
			return err
		}
		if statuses, err = repo.Current(ctx, q, "status"); err != nil {
			return err
		}
		if groups, err = repo.Current(ctx, q, "group"); err != nil {
			return err
		}
		if tags, err = repo.Current(ctx, q, "tag"); err != nil {
			return err
		}
		if sites, err = repo.Current(ctx, q, "site"); err != nil {
			return err
		}
		if trend, err = repo.Trend(ctx, q); err != nil {
			return err
		}
		if legacyTrend, err = repo.HasLegacyTrend(ctx, q); err != nil {
			return err
		}
		if completeness, err = repo.Completeness(ctx, q.SiteIDs); err != nil {
			return err
		}
		coverage, err = repo.TrendCoverage(ctx, q)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return dto.ChannelInventoryStatisticsResponse{}, err
	}
	out := dto.ChannelInventoryStatisticsResponse{Trend: channelCompleteTrend(q, trend, coverage, len(completeness)), TypeBreakdown: channelBreakdown(types), StatusBreakdown: channelBreakdown(statuses), GroupBreakdown: channelBreakdown(groups), TagBreakdown: channelBreakdown(tags), SiteBreakdown: channelSiteBreakdown(sites, completeness), DataStatus: channelOverallStatus(completeness)}
	if len(summary) > 0 {
		out.Summary = channelMetric(summary[0])
	} else {
		out.Summary = emptyChannelMetric()
	}
	if legacyTrend && out.DataStatus == "complete" {
		out.DataStatus = "partial"
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

func channelSiteStatus(row model.SiteChannelInventoryCompletenessRow) string {
	switch row.LatestRunStatus {
	case "failed":
		return "unavailable"
	case "pending", "running":
		if row.AsOf != nil {
			return "partial"
		}
		return "pending"
	case "success":
		return "complete"
	default:
		if row.AsOf != nil || row.InventoryCount > 0 {
			return "complete"
		}
		return "pending"
	}
}

func channelOverallStatus(rows []model.SiteChannelInventoryCompletenessRow) string {
	if len(rows) == 0 {
		return "pending"
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[channelSiteStatus(row)]++
	}
	for _, status := range []string{"complete", "unavailable", "pending"} {
		if counts[status] == len(rows) {
			return status
		}
	}
	return "partial"
}

func channelSiteBreakdown(rows []model.SiteChannelInventoryMetricRow, completeness []model.SiteChannelInventoryCompletenessRow) []dto.ChannelInventoryBreakdown {
	metrics := make(map[int64]model.SiteChannelInventoryMetricRow, len(rows))
	for _, row := range rows {
		metrics[row.SiteID] = row
	}
	out := make([]dto.ChannelInventoryBreakdown, 0, len(completeness))
	for _, site := range completeness {
		metric := emptyChannelMetric()
		if row, ok := metrics[site.SiteID]; ok {
			metric = channelMetric(row)
		}
		out = append(out, dto.ChannelInventoryBreakdown{DimensionID: strconv.FormatInt(site.SiteID, 10), DimensionName: site.SiteName, SiteID: strconv.FormatInt(site.SiteID, 10), SiteName: site.SiteName, ChannelInventoryMetric: metric, DataStatus: channelSiteStatus(site), AsOf: site.AsOf})
	}
	return out
}

func channelCompleteTrend(q dto.ChannelInventoryStatisticsQuery, rows []model.SiteChannelInventoryMetricRow, coverage []model.SiteChannelInventoryCoverageRow, expectedSites int) []dto.ChannelInventoryTrendPoint {
	metrics := make(map[int64]model.SiteChannelInventoryMetricRow, len(rows))
	for _, row := range rows {
		metrics[row.BucketStart] = row
	}
	covered := make(map[int64]int64, len(coverage))
	for _, row := range coverage {
		covered[row.BucketStart] = row.CompleteSiteCount
	}
	out := make([]dto.ChannelInventoryTrendPoint, 0, (q.EndTimestamp-q.StartTimestamp)/3600)
	for bucket := q.StartTimestamp; bucket < q.EndTimestamp; bucket += 3600 {
		metric := emptyChannelMetric()
		if row, ok := metrics[bucket]; ok {
			metric = channelMetric(row)
		}
		status := "missing"
		if expectedSites == 0 {
			status = "pending"
		} else if covered[bucket] == int64(expectedSites) {
			status = "complete"
		} else if covered[bucket] > 0 {
			status = "partial"
		}
		out = append(out, dto.ChannelInventoryTrendPoint{BucketStart: bucket, BucketEnd: bucket + 3600, ChannelInventoryMetric: metric, DataStatus: status})
	}
	return out
}
