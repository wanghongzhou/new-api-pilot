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

type UserInventoryService struct {
	db *gorm.DB
}

func NewUserInventoryService(db *gorm.DB) (*UserInventoryService, error) {
	if db == nil {
		return nil, errors.New("user inventory database is required")
	}
	return &UserInventoryService{db: db}, nil
}

func (service *UserInventoryService) List(ctx context.Context, query dto.UserInventoryQuery) (dto.UserInventoryPage, error) {
	query.Normalize()
	if service == nil || query.Validate() != nil {
		return dto.UserInventoryPage{}, ErrStatisticsInvalid
	}
	var rows []model.SiteUserInventoryReadRow
	var completeness []model.SiteUserInventoryCompletenessRow
	var total int64
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repository := model.NewSiteUserInventoryRepository(tx)
		var err error
		rows, total, err = repository.List(ctx, query)
		if err != nil {
			return err
		}
		completeness, err = repository.Completeness(ctx, query.SiteIDs)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return dto.UserInventoryPage{}, err
	}
	items := make([]dto.UserInventoryItem, 0, len(rows))
	for _, row := range rows {
		var accountID *string
		if row.AccountID != nil {
			value := strconv.FormatInt(*row.AccountID, 10)
			accountID = &value
		}
		items = append(items, dto.UserInventoryItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), SiteName: row.SiteName,
			RemoteUserID: strconv.FormatInt(row.RemoteUserID, 10), RemoteCreatedAt: row.RemoteCreatedAt, Username: row.Username, DisplayName: row.DisplayName,
			Role: row.RemoteRole, Status: row.RemoteStatus, Group: row.RemoteGroup, Quota: strconv.FormatInt(row.Quota, 10),
			UsedQuota: strconv.FormatInt(row.UsedQuota, 10), Balance: strconv.FormatInt(row.Balance, 10), RequestCount: strconv.FormatInt(row.RequestCount, 10),
			LastLoginAt: row.LastLoginAt, RemoteState: row.RemoteState, MissingCount: row.MissingCount, FirstSeenAt: row.FirstSeenAt,
			LastSeenAt: row.LastSeenAt, AccountID: accountID})
	}
	status := inventoryOverallStatus(completeness)
	return dto.UserInventoryPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, DataStatus: status}, nil
}

func (service *UserInventoryService) Statistics(ctx context.Context, query dto.UserInventoryStatisticsQuery) (dto.UserInventoryStatisticsResponse, error) {
	query.Normalize()
	if service == nil || query.Validate() != nil {
		return dto.UserInventoryStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summaryRows, roleRows, statusRows, groupRows, siteRows, trendRows []model.SiteUserInventoryMetricRow
	var completeness []model.SiteUserInventoryCompletenessRow
	var coverage []model.SiteUserInventoryCoverageRow
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repository := model.NewSiteUserInventoryRepository(tx)
		var err error
		if summaryRows, err = repository.CurrentMetrics(ctx, query, "summary"); err != nil {
			return err
		}
		if roleRows, err = repository.CurrentMetrics(ctx, query, "role"); err != nil {
			return err
		}
		if statusRows, err = repository.CurrentMetrics(ctx, query, "status"); err != nil {
			return err
		}
		if groupRows, err = repository.CurrentMetrics(ctx, query, "group"); err != nil {
			return err
		}
		if siteRows, err = repository.CurrentMetrics(ctx, query, "site"); err != nil {
			return err
		}
		if trendRows, err = repository.Trend(ctx, query); err != nil {
			return err
		}
		if completeness, err = repository.Completeness(ctx, query.SiteIDs); err != nil {
			return err
		}
		coverage, err = repository.TrendCoverage(ctx, query)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return dto.UserInventoryStatisticsResponse{}, err
	}
	response := dto.UserInventoryStatisticsResponse{
		RoleBreakdown: inventoryBreakdown(roleRows), StatusBreakdown: inventoryBreakdown(statusRows),
		GroupBreakdown: inventoryBreakdown(groupRows), SiteBreakdown: inventorySiteBreakdown(siteRows, completeness),
		DataStatus: inventoryOverallStatus(completeness),
	}
	response.Trend = inventoryCompleteTrend(query, trendRows, coverage, len(completeness))
	if len(summaryRows) > 0 {
		response.Summary = inventoryMetric(summaryRows[0])
	} else {
		response.Summary = emptyInventoryMetric()
	}
	return response, nil
}

func inventoryMetric(row model.SiteUserInventoryMetricRow) dto.UserInventoryMetric {
	balance := row.Quota - row.UsedQuota
	return dto.UserInventoryMetric{UserCount: strconv.FormatInt(row.UserCount, 10), NewUserCount: strconv.FormatInt(row.NewUserCount, 10),
		ActiveUserCount: strconv.FormatInt(row.ActiveUserCount, 10), Quota: strconv.FormatInt(row.Quota, 10), UsedQuota: strconv.FormatInt(row.UsedQuota, 10),
		Balance: strconv.FormatInt(balance, 10), RequestCount: strconv.FormatInt(row.RequestCount, 10)}
}

func emptyInventoryMetric() dto.UserInventoryMetric {
	return dto.UserInventoryMetric{UserCount: "0", NewUserCount: "0", ActiveUserCount: "0", Quota: "0", UsedQuota: "0", Balance: "0", RequestCount: "0"}
}

func inventoryBreakdown(rows []model.SiteUserInventoryMetricRow) []dto.UserInventoryBreakdown {
	result := make([]dto.UserInventoryBreakdown, 0, len(rows))
	for _, row := range rows {
		result = append(result, dto.UserInventoryBreakdown{DimensionID: row.DimensionID, DimensionName: row.DimensionName, UserInventoryMetric: inventoryMetric(row)})
	}
	return result
}

func inventorySiteBreakdown(rows []model.SiteUserInventoryMetricRow, completeness []model.SiteUserInventoryCompletenessRow) []dto.UserInventorySiteBreakdown {
	metrics := make(map[int64]model.SiteUserInventoryMetricRow, len(rows))
	for _, row := range rows {
		metrics[row.SiteID] = row
	}
	result := make([]dto.UserInventorySiteBreakdown, 0, len(completeness))
	for _, site := range completeness {
		metric, exists := metrics[site.SiteID]
		value := emptyInventoryMetric()
		if exists {
			value = inventoryMetric(metric)
		}
		result = append(result, dto.UserInventorySiteBreakdown{SiteID: strconv.FormatInt(site.SiteID, 10), SiteName: site.SiteName,
			UserInventoryMetric: value, DataStatus: inventorySiteStatus(site), AsOf: site.AsOf})
	}
	return result
}

func inventoryTrend(rows []model.SiteUserInventoryMetricRow) []dto.UserInventoryTrendPoint {
	result := make([]dto.UserInventoryTrendPoint, 0, len(rows))
	for _, row := range rows {
		result = append(result, dto.UserInventoryTrendPoint{BucketStart: row.BucketStart, BucketEnd: row.BucketStart + 3600,
			UserInventoryMetric: inventoryMetric(row), DataStatus: "complete"})
	}
	return result
}

func inventorySiteStatus(row model.SiteUserInventoryCompletenessRow) string {
	switch row.LatestRunStatus {
	case "failed":
		return "unavailable"
	case "pending", "running":
		if row.InventoryCount > 0 {
			return "partial"
		}
		return "pending"
	case "success":
		return "complete"
	default:
		if row.InventoryCount > 0 {
			return "complete"
		}
		return "pending"
	}
}

func inventoryOverallStatus(rows []model.SiteUserInventoryCompletenessRow) string {
	if len(rows) == 0 {
		return "pending"
	}
	complete, unavailable, pending := 0, 0, 0
	for _, row := range rows {
		switch inventorySiteStatus(row) {
		case "complete":
			complete++
		case "unavailable":
			unavailable++
		default:
			pending++
		}
	}
	if complete == len(rows) {
		return "complete"
	}
	if complete > 0 {
		return "partial"
	}
	if unavailable > 0 {
		return "unavailable"
	}
	if pending > 0 {
		return "pending"
	}
	return "missing"
}

func inventoryCompleteTrend(
	query dto.UserInventoryStatisticsQuery,
	rows []model.SiteUserInventoryMetricRow,
	coverage []model.SiteUserInventoryCoverageRow,
	expectedSites int,
) []dto.UserInventoryTrendPoint {
	metrics := make(map[int64]model.SiteUserInventoryMetricRow, len(rows))
	for _, row := range rows {
		metrics[row.BucketStart] = row
	}
	covered := make(map[int64]int64, len(coverage))
	for _, row := range coverage {
		covered[row.BucketStart] = row.CompleteSiteCount
	}
	result := make([]dto.UserInventoryTrendPoint, 0, (query.EndTimestamp-query.StartTimestamp)/3600)
	for bucket := query.StartTimestamp; bucket < query.EndTimestamp; bucket += 3600 {
		metric := emptyInventoryMetric()
		if row, exists := metrics[bucket]; exists {
			metric = inventoryMetric(row)
		}
		status := "missing"
		count := covered[bucket]
		if expectedSites == 0 {
			status = "pending"
		} else if count == int64(expectedSites) {
			status = "complete"
		} else if count > 0 {
			status = "partial"
		}
		result = append(result, dto.UserInventoryTrendPoint{BucketStart: bucket, BucketEnd: bucket + 3600,
			UserInventoryMetric: metric, DataStatus: status})
	}
	return result
}
