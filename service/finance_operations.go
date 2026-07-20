package service

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type FinanceOperationsService struct {
	database *gorm.DB
	clock    common.Clock
}

func NewFinanceOperationsService(db *gorm.DB, clock common.Clock) (*FinanceOperationsService, error) {
	if db == nil || clock == nil {
		return nil, errors.New("finance operations dependencies are required")
	}
	return &FinanceOperationsService{database: db, clock: clock}, nil
}

func (s *FinanceOperationsService) readSnapshot(ctx context.Context, read func(*model.FinanceRepository) error) error {
	if s == nil || s.database == nil || read == nil {
		return errors.New("finance operations snapshot dependencies are required")
	}
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(model.NewFinanceRepository(tx))
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (s *FinanceOperationsService) Topups(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceInventoryPage[dto.TopupInventoryItem], error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.FinanceInventoryPage[dto.TopupInventoryItem]{}, ErrStatisticsInvalid
	}
	var rows []model.TopupReadRow
	var completeness []model.FinanceMetricRow
	var total int64
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		rows, total, err = repository.ListTopups(ctx, q)
		if err != nil {
			return err
		}
		completeness, err = repository.TopupMetrics(ctx, q, "summary")
		return err
	}); err != nil {
		return dto.FinanceInventoryPage[dto.TopupInventoryItem]{}, err
	}
	items := make([]dto.TopupInventoryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.TopupInventoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), RemoteID: strconv.FormatInt(r.RemoteID, 10), RemoteUserID: strconv.FormatInt(r.RemoteUserID, 10), SiteName: r.SiteName, Amount: strconv.FormatInt(r.Amount, 10), Money: r.Money, PaymentMethod: r.PaymentMethod, PaymentProvider: r.PaymentProvider, CreateTime: r.CreateTime, CompleteTime: r.CompleteTime, Status: r.RemoteStatus, RemoteState: r.RemoteState, MissingCount: r.MissingCount, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
	}
	status := "complete"
	var asOf *int64
	if total == 0 {
		status = "pending"
	} else if len(completeness) != 1 {
		return dto.FinanceInventoryPage[dto.TopupInventoryItem]{}, model.ErrStatisticsReadContract
	} else {
		asOf = completeness[0].AsOf
		if completeness[0].MissingCount > 0 {
			status = "partial"
		}
	}
	return dto.FinanceInventoryPage[dto.TopupInventoryItem]{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: status, AsOf: asOf}, nil
}

func (s *FinanceOperationsService) Redemptions(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceInventoryPage[dto.RedemptionInventoryItem], error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, ErrStatisticsInvalid
	}
	now := s.clock.Now().Unix()
	var rows []model.RedemptionReadRow
	var completeness []model.FinanceMetricRow
	var total int64
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		rows, total, err = repository.ListRedemptions(ctx, q)
		if err != nil {
			return err
		}
		completeness, err = repository.RedemptionMetrics(ctx, q, "summary", now)
		return err
	}); err != nil {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, err
	}
	items := make([]dto.RedemptionInventoryItem, 0, len(rows))
	for _, r := range rows {
		derived := strconv.Itoa(r.RemoteStatus)
		if r.RemoteStatus == 1 && r.ExpiredTime != 0 && r.ExpiredTime < now {
			derived = "expired"
		}
		items = append(items, dto.RedemptionInventoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), RemoteID: strconv.FormatInt(r.RemoteID, 10), RemoteUserID: strconv.FormatInt(r.RemoteUserID, 10), SiteName: r.SiteName, Name: r.Name, Status: r.RemoteStatus, DerivedStatus: derived, Quota: strconv.FormatInt(r.Quota, 10), CreatedTime: r.CreatedTime, RedeemedTime: r.RedeemedTime, UsedUserID: strconv.FormatInt(r.UsedUserID, 10), ExpiredTime: r.ExpiredTime, RemoteState: r.RemoteState, MissingCount: r.MissingCount, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
	}
	status := "complete"
	var asOf *int64
	if total == 0 {
		status = "pending"
	} else if len(completeness) != 1 {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, model.ErrStatisticsReadContract
	} else {
		asOf = completeness[0].AsOf
		if completeness[0].MissingCount > 0 {
			status = "partial"
		}
	}
	return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: status, AsOf: asOf}, nil
}

func financeMetric(r model.FinanceMetricRow, topup bool) dto.FinanceMetric {
	m := dto.FinanceMetric{Count: strconv.FormatInt(r.Count, 10), MissingCount: strconv.FormatInt(r.MissingCount, 10)}
	if topup {
		m.Amount = strconv.FormatInt(r.Amount, 10)
		m.Money = r.Money
	} else {
		m.Quota = strconv.FormatInt(r.Quota, 10)
	}
	return m
}
func financeBreakdown(rows []model.FinanceMetricRow, topup bool) []dto.FinanceBreakdown {
	out := make([]dto.FinanceBreakdown, 0, len(rows))
	for _, r := range rows {
		status := "complete"
		if r.MissingCount > 0 {
			status = "partial"
		}
		out = append(out, dto.FinanceBreakdown{DimensionID: r.DimensionID, DimensionName: r.DimensionName, SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, FinanceMetric: financeMetric(r, topup), DataStatus: status, AsOf: r.AsOf})
	}
	return out
}
func (s *FinanceOperationsService) TopupStatistics(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if s == nil || q.Validate() != nil {
		return dto.FinanceStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summary, statuses, providers, sites []model.FinanceMetricRow
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		if summary, err = repository.TopupMetrics(ctx, q, "summary"); err != nil {
			return err
		}
		if statuses, err = repository.TopupMetrics(ctx, q, "status"); err != nil {
			return err
		}
		if providers, err = repository.TopupMetrics(ctx, q, "provider"); err != nil {
			return err
		}
		sites, err = repository.TopupMetrics(ctx, q, "site")
		return err
	}); err != nil {
		return dto.FinanceStatisticsResponse{}, err
	}
	out := dto.FinanceStatisticsResponse{StatusBreakdown: financeBreakdown(statuses, true), ProviderBreakdown: financeBreakdown(providers, true), SiteBreakdown: financeBreakdown(sites, true), DataStatus: "complete"}
	if len(summary) > 0 {
		out.Summary = financeMetric(summary[0], true)
		out.Summary.Amount = ""
		out.Summary.Money = ""
		if summary[0].MissingCount > 0 {
			out.DataStatus = "partial"
		}
	} else {
		out.Summary = dto.FinanceMetric{Count: "0", MissingCount: "0"}
		out.DataStatus = "pending"
	}
	return out, nil
}
func (s *FinanceOperationsService) RedemptionStatistics(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if s == nil || q.Validate() != nil {
		return dto.FinanceStatisticsResponse{}, ErrStatisticsInvalid
	}
	now := s.clock.Now().Unix()
	var summary, statuses, sites []model.FinanceMetricRow
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		if summary, err = repository.RedemptionMetrics(ctx, q, "summary", now); err != nil {
			return err
		}
		if statuses, err = repository.RedemptionMetrics(ctx, q, "status", now); err != nil {
			return err
		}
		sites, err = repository.RedemptionMetrics(ctx, q, "site", now)
		return err
	}); err != nil {
		return dto.FinanceStatisticsResponse{}, err
	}
	out := dto.FinanceStatisticsResponse{StatusBreakdown: financeBreakdown(statuses, false), SiteBreakdown: financeBreakdown(sites, false), DataStatus: "complete"}
	if len(summary) > 0 {
		out.Summary = financeMetric(summary[0], false)
		if summary[0].MissingCount > 0 {
			out.DataStatus = "partial"
		}
	} else {
		out.Summary = dto.FinanceMetric{Count: "0", MissingCount: "0", Quota: "0"}
		out.DataStatus = "pending"
	}
	return out, nil
}
