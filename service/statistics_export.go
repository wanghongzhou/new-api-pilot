package service

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const StatisticsExportPageSize = model.StatisticsExportMaximumPageSize

type StatisticsExportIteratorOptions struct {
	Database *gorm.DB
	Clock    common.Clock
	Scope    string
	Query    dto.StatisticsQuery
	PageSize int
}

type statisticsExportRowReader interface {
	LoadExportRows(context.Context, model.StatisticsExportRowQuery) ([]model.StatisticsExportRow, error)
	LoadSites(context.Context, []int64) ([]model.StatisticsSite, error)
	LoadFallbackRates(context.Context) (model.StatisticsFallbackRates, error)
}

type StatisticsExportIterator struct {
	repository        statisticsExportRowReader
	clock             common.Clock
	scope             string
	query             dto.StatisticsQuery
	request           model.StatisticsReadRequest
	cursor            model.StatisticsExportRowCursor
	pageSize          int
	usageDelayMinutes int
	settingsLoaded    bool
	done              bool
	snapshotService   *StatisticsService
	snapshotItems     []dto.StatisticsBreakdownItem
	snapshotOffset    int
}

func NewStatisticsExportIterator(options StatisticsExportIteratorOptions) (*StatisticsExportIterator, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("statistics export iterator dependencies are required")
	}
	options.Query.Normalize()
	if options.Query.ValidateForExport(options.Scope) != nil {
		return nil, ErrStatisticsInvalid
	}
	request, err := statisticsReadRequest(options.Scope, options.Query)
	if err != nil {
		return nil, err
	}
	pageSize := options.PageSize
	if pageSize <= 0 {
		pageSize = StatisticsExportPageSize
	}
	if pageSize > StatisticsExportPageSize {
		return nil, errors.New("statistics export page size exceeds the fixed limit")
	}
	snapshotService, err := NewStatisticsService(StatisticsServiceOptions{Database: options.Database, Clock: options.Clock})
	if err != nil {
		return nil, err
	}
	return &StatisticsExportIterator{
		repository: model.NewStatisticsRepository(options.Database), clock: options.Clock,
		scope: options.Scope, query: options.Query, request: request, pageSize: pageSize, snapshotService: snapshotService,
	}, nil
}

func (iterator *StatisticsExportIterator) Next(ctx context.Context) (dto.StatisticsResponse, bool, error) {
	if iterator == nil || iterator.done {
		return dto.StatisticsResponse{}, true, nil
	}
	if iterator.scope == dto.StatisticsScopeGroup || iterator.scope == dto.StatisticsScopeToken || iterator.scope == dto.StatisticsScopeNode {
		return iterator.nextFlowDimensionSnapshot(ctx)
	}
	if !iterator.settingsLoaded {
		settings, err := iterator.repository.LoadFallbackRates(ctx)
		if err != nil {
			return dto.StatisticsResponse{}, false, err
		}
		iterator.usageDelayMinutes = settings.UsageDelayMinutes
		iterator.settingsLoaded = true
	}
	rows, err := iterator.repository.LoadExportRows(ctx, model.StatisticsExportRowQuery{
		Request: iterator.request, SortBy: iterator.query.SortBy, SortOrder: iterator.query.SortOrder,
		Now: iterator.clock.Now().Unix(), UsageDelayMinutes: iterator.usageDelayMinutes,
		Limit: iterator.pageSize, Cursor: iterator.cursor,
	})
	if err != nil {
		if errors.Is(err, model.ErrStatisticsExportCapacity) {
			return dto.StatisticsResponse{}, false, ErrExportFileTooLarge
		}
		return dto.StatisticsResponse{}, false, err
	}
	if len(rows) == 0 {
		iterator.done = true
		return dto.StatisticsResponse{}, true, nil
	}
	hasMore := len(rows) > iterator.pageSize
	if hasMore {
		rows = rows[:iterator.pageSize]
	}
	items := make([]dto.StatisticsBreakdownItem, 0, len(rows))
	for _, row := range rows {
		item, itemErr := statisticsExportBreakdownItem(iterator.scope, row)
		if itemErr != nil {
			return dto.StatisticsResponse{}, false, itemErr
		}
		items = append(items, item)
	}
	iterator.cursor = rows[len(rows)-1].Cursor()
	iterator.done = !hasMore
	return dto.StatisticsResponse{
		Scope: iterator.scope, Granularity: iterator.query.Granularity,
		Breakdown: common.NewPageData(1, len(items), int64(len(items)), items),
	}, false, nil
}

func (iterator *StatisticsExportIterator) nextFlowDimensionSnapshot(ctx context.Context) (dto.StatisticsResponse, bool, error) {
	if iterator.snapshotItems == nil {
		response, err := iterator.snapshotService.ExportSnapshot(ctx, iterator.scope, iterator.query)
		if err != nil {
			return dto.StatisticsResponse{}, false, err
		}
		iterator.snapshotItems = append([]dto.StatisticsBreakdownItem(nil), response.Breakdown.Items...)
	}
	if iterator.snapshotOffset >= len(iterator.snapshotItems) {
		iterator.done = true
		return dto.StatisticsResponse{}, true, nil
	}
	end := iterator.snapshotOffset + iterator.pageSize
	if end > len(iterator.snapshotItems) {
		end = len(iterator.snapshotItems)
	}
	items := append([]dto.StatisticsBreakdownItem(nil), iterator.snapshotItems[iterator.snapshotOffset:end]...)
	iterator.snapshotOffset = end
	iterator.done = end == len(iterator.snapshotItems)
	return dto.StatisticsResponse{Scope: iterator.scope, Granularity: iterator.query.Granularity, Breakdown: common.NewPageData(1, len(items), int64(len(items)), items)}, false, nil
}

func statisticsExportBreakdownItem(scope string, row model.StatisticsExportRow) (dto.StatisticsBreakdownItem, error) {
	var siteID, siteName *string
	if row.DimensionSiteID > 0 {
		id := strconv.FormatInt(row.DimensionSiteID, 10)
		name := row.BreakdownSiteName
		siteID, siteName = &id, &name
	}
	breakdown := make([]dto.SiteQuotaBreakdown, 0, 1)
	if row.BreakdownSiteID > 0 {
		breakdown = append(breakdown, dto.SiteQuotaBreakdown{
			SiteID: strconv.FormatInt(row.BreakdownSiteID, 10), SiteName: row.BreakdownSiteName,
			Quota: row.SiteQuota, DataStatus: row.DataStatus,
		})
	}
	base := dto.StatisticsBreakdownBase{
		DimensionID: row.DimensionID, DimensionName: row.DimensionName,
		SiteID: siteID, SiteName: siteName,
		BucketStart: row.BucketStart, BucketEnd: row.BucketEnd,
		RequestCount: row.RequestCount, Quota: row.Quota, TokenUsed: row.TokenUsed, ActiveUsers: row.ActiveUsers,
		DataStatus: row.DataStatus, IsFinal: row.IsFinal, AsOf: row.AsOf, SiteBreakdown: breakdown,
	}
	switch scope {
	case dto.StatisticsScopeGlobal:
		return dto.GlobalStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope}, nil
	case dto.StatisticsScopeSite:
		return dto.SiteStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope}, nil
	case dto.StatisticsScopeCustomer:
		return dto.CustomerStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope}, nil
	case dto.StatisticsScopeAccount:
		return dto.AccountStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope}, nil
	case dto.StatisticsScopeModel:
		return dto.ModelStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: scope, ModelName: row.DimensionValue,
		}, nil
	case dto.StatisticsScopeChannel:
		return dto.ChannelStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: scope,
			RemoteChannelID: strconv.FormatInt(row.EntityID, 10),
		}, nil
	case dto.StatisticsScopeGroup:
		return dto.GroupStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope, UseGroup: row.DimensionValue}, nil
	case dto.StatisticsScopeToken:
		return dto.TokenStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope, TokenID: strconv.FormatInt(row.EntityID, 10), TokenName: row.DimensionName}, nil
	case dto.StatisticsScopeNode:
		return dto.NodeStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: scope, NodeName: row.DimensionValue}, nil
	default:
		return nil, ErrStatisticsInvalid
	}
}

func (iterator *StatisticsExportIterator) LoadRateSnapshot(ctx context.Context) ([]dto.SiteQuotaBreakdown, error) {
	if iterator == nil || iterator.repository == nil {
		return nil, ErrStatisticsRead
	}
	sites, err := iterator.repository.LoadSites(ctx, iterator.query.SiteIDs)
	if err != nil {
		return nil, err
	}
	fallback, err := iterator.repository.LoadFallbackRates(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]dto.SiteQuotaBreakdown, 0, len(sites))
	for _, site := range sites {
		rate := statisticsEffectiveRate(site, fallback)
		result = append(result, dto.SiteQuotaBreakdown{
			SiteID: strconv.FormatInt(site.ID, 10), SiteName: site.Name,
			QuotaPerUnit: rate.QuotaPerUnit, USDExchangeRate: rate.USDExchangeRate,
			RateSource: rate.Source, RateUpdatedAt: rate.UpdatedAt,
			DataStatus: "complete",
		})
	}
	return result, nil
}
