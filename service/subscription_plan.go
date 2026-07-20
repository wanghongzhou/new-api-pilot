package service

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"strconv"
)

type SubscriptionPlanService struct {
	db   *gorm.DB
	repo *model.SubscriptionPlanRepository
}

func NewSubscriptionPlanService(db *gorm.DB) (*SubscriptionPlanService, error) {
	if db == nil {
		return nil, errors.New("subscription plan database required")
	}
	return &SubscriptionPlanService{db: db, repo: model.NewSubscriptionPlanRepository(db)}, nil
}
func planSiteStatus(row model.SubscriptionPlanMetricRow) string {
	if row.LastSuccessAt != nil && (row.LastFailureAt == nil || *row.LastSuccessAt >= *row.LastFailureAt) {
		return "complete"
	}
	if row.LastFailureAt != nil {
		return "unavailable"
	}
	return "pending"
}
func planOverallStatus(rows []model.SubscriptionPlanMetricRow) string {
	if len(rows) == 0 {
		return "pending"
	}
	complete, unavailable := 0, 0
	for _, row := range rows {
		switch planSiteStatus(row) {
		case "complete":
			complete++
		case "unavailable":
			unavailable++
		}
	}
	if complete == len(rows) {
		return "complete"
	}
	if unavailable == len(rows) {
		return "unavailable"
	}
	if complete == 0 && unavailable == 0 {
		return "pending"
	}
	return "partial"
}
func planItem(r model.SubscriptionPlanReadRow, status string) dto.SubscriptionPlanItem {
	price, _ := canonicalNonNegativeMoneyDecimal(r.PriceAmount)
	return dto.SubscriptionPlanItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), RemoteID: strconv.FormatInt(r.RemoteID, 10), SiteName: r.SiteName, Title: r.Title, Subtitle: r.Subtitle, PriceAmount: price, Currency: r.Currency, DurationUnit: r.DurationUnit, DurationValue: r.DurationValue, CustomSeconds: strconv.FormatInt(r.CustomSeconds, 10), Enabled: r.Enabled, SortOrder: r.SortOrder, TotalAmount: strconv.FormatInt(r.TotalAmount, 10), QuotaResetPeriod: r.QuotaResetPeriod, QuotaResetCustomSeconds: strconv.FormatInt(r.QuotaResetCustomSeconds, 10), CreatedAt: r.RemoteCreatedAt, UpdatedAt: r.RemoteUpdatedAt, RemoteState: r.RemoteState, MissingCount: r.MissingCount, DataStatus: status}
}
func (s *SubscriptionPlanService) List(ctx context.Context, q dto.SubscriptionPlanQuery) (dto.SubscriptionPlanPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.SubscriptionPlanPageResponse{}, ErrStatisticsInvalid
	}
	var out dto.SubscriptionPlanPageResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewSubscriptionPlanRepository(tx)
		rows, total, err := repo.List(ctx, q)
		if err != nil {
			return err
		}
		states, err := repo.ActiveSiteStates(ctx, q)
		if err != nil {
			return err
		}
		statusBySite := make(map[int64]string, len(states))
		for _, row := range states {
			statusBySite[row.SiteID] = planSiteStatus(row)
		}
		items := make([]dto.SubscriptionPlanItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, planItem(row, statusBySite[row.SiteID]))
		}
		out = dto.SubscriptionPlanPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: planOverallStatus(states)}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
func (s *SubscriptionPlanService) Statistics(ctx context.Context, q dto.SubscriptionPlanQuery) (dto.SubscriptionPlanStatistics, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if q.Validate() != nil {
		return dto.SubscriptionPlanStatistics{}, ErrStatisticsInvalid
	}
	var out dto.SubscriptionPlanStatistics
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sites, err := model.NewSubscriptionPlanRepository(tx).SiteMetrics(ctx, q)
		if err != nil {
			return err
		}
		out = dto.SubscriptionPlanStatistics{DataStatus: planOverallStatus(sites), SiteBreakdown: []dto.SubscriptionPlanBreakdown{}}
		var total, enabled, disabled, missing int64
		for _, r := range sites {
			total += r.Total
			enabled += r.Enabled
			disabled += r.Disabled
			missing += r.Missing
			out.SiteBreakdown = append(out.SiteBreakdown, dto.SubscriptionPlanBreakdown{SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, Total: strconv.FormatInt(r.Total, 10), Enabled: strconv.FormatInt(r.Enabled, 10), Disabled: strconv.FormatInt(r.Disabled, 10), Missing: strconv.FormatInt(r.Missing, 10), DataStatus: planSiteStatus(r), AsOf: r.AsOf})
		}
		out.Total = strconv.FormatInt(total, 10)
		out.Enabled = strconv.FormatInt(enabled, 10)
		out.Disabled = strconv.FormatInt(disabled, 10)
		out.Missing = strconv.FormatInt(missing, 10)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
