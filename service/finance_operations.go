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
	var coverage []model.FinanceCollectionCoverageRow
	var total int64
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		rows, total, err = repository.ListTopups(ctx, q)
		if err != nil {
			return err
		}
		completeness, err = repository.TopupMetrics(ctx, q, "summary")
		if err != nil {
			return err
		}
		coverage, err = repository.CollectionCoverage(ctx, q.SiteIDs, "topup")
		return err
	}); err != nil {
		return dto.FinanceInventoryPage[dto.TopupInventoryItem]{}, err
	}
	items := make([]dto.TopupInventoryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.TopupInventoryItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), RemoteID: strconv.FormatInt(r.RemoteID, 10), RemoteUserID: strconv.FormatInt(r.RemoteUserID, 10), SiteName: r.SiteName, Amount: strconv.FormatInt(r.Amount, 10), Money: r.Money, PaymentMethod: r.PaymentMethod, PaymentProvider: r.PaymentProvider, CreateTime: r.CreateTime, CompleteTime: r.CompleteTime, Status: r.RemoteStatus, RemoteState: r.RemoteState, MissingCount: r.MissingCount, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
	}
	if total > 0 && len(completeness) != 1 {
		return dto.FinanceInventoryPage[dto.TopupInventoryItem]{}, model.ErrStatisticsReadContract
	}
	status, asOf, complete := financeCoverage(coverage)
	return dto.FinanceInventoryPage[dto.TopupInventoryItem]{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: status, AsOf: asOf, Completeness: complete}, nil
}

func (s *FinanceOperationsService) Redemptions(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceInventoryPage[dto.RedemptionInventoryItem], error) {
	q.Normalize()
	if s == nil || s.clock == nil {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, ErrStatisticsInvalid
	}
	now := s.clock.Now().Unix()
	q.StatusAt = now
	if q.Validate() != nil || q.ValidateRedemptionStatuses() != nil {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, ErrStatisticsInvalid
	}
	var rows []model.RedemptionReadRow
	var completeness []model.FinanceMetricRow
	var coverage []model.FinanceCollectionCoverageRow
	var total int64
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		rows, total, err = repository.ListRedemptions(ctx, q)
		if err != nil {
			return err
		}
		completeness, err = repository.RedemptionMetrics(ctx, q, "summary", now)
		if err != nil {
			return err
		}
		coverage, err = repository.CollectionCoverage(ctx, q.SiteIDs, "redemption")
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
	if total > 0 && len(completeness) != 1 {
		return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{}, model.ErrStatisticsReadContract
	}
	status, asOf, complete := financeCoverage(coverage)
	return dto.FinanceInventoryPage[dto.RedemptionInventoryItem]{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: status, AsOf: asOf, Completeness: complete}, nil
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

func financeCoverageStatus(row model.FinanceCollectionCoverageRow) string {
	if row.LastSuccessAt != nil {
		if row.LastFailureAt != nil && *row.LastFailureAt > *row.LastSuccessAt {
			return "partial"
		}
		return "complete"
	}
	if row.LastFailureAt != nil {
		return "unavailable"
	}
	return "pending"
}

func financeCoverage(rows []model.FinanceCollectionCoverageRow) (string, *int64, dto.FinanceCompleteness) {
	counts := map[string]int{}
	var asOf *int64
	for _, row := range rows {
		counts[financeCoverageStatus(row)]++
		if row.AsOf != nil && (asOf == nil || *row.AsOf > *asOf) {
			value := *row.AsOf
			asOf = &value
		}
	}
	status := "pending"
	if len(rows) > 0 {
		status = "partial"
		for _, candidate := range []string{"complete", "unavailable", "pending"} {
			if counts[candidate] == len(rows) {
				status = candidate
				break
			}
		}
	}
	value := dto.FinanceCompleteness{DataStatus: status, CompleteSiteCount: counts["complete"], ExpectedSiteCount: len(rows), UnavailableCount: counts["unavailable"], PendingSiteCount: counts["pending"]}
	return status, asOf, value
}

func financeSiteBreakdown(rows []model.FinanceMetricRow, coverage []model.FinanceCollectionCoverageRow, topup bool) []dto.FinanceBreakdown {
	metrics := make(map[int64]model.FinanceMetricRow, len(rows))
	for _, row := range rows {
		metrics[row.SiteID] = row
	}
	out := make([]dto.FinanceBreakdown, 0, len(coverage))
	for _, site := range coverage {
		metric := dto.FinanceMetric{Count: "0", MissingCount: "0"}
		if topup {
			metric.Amount, metric.Money = "0", "0"
		} else {
			metric.Quota = "0"
		}
		if row, exists := metrics[site.SiteID]; exists {
			metric = financeMetric(row, topup)
		}
		out = append(out, dto.FinanceBreakdown{DimensionID: strconv.FormatInt(site.SiteID, 10), DimensionName: site.SiteName, SiteID: strconv.FormatInt(site.SiteID, 10), SiteName: site.SiteName, FinanceMetric: metric, DataStatus: financeCoverageStatus(site), AsOf: site.AsOf})
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
	var coverage []model.FinanceCollectionCoverageRow
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
		if sites, err = repository.TopupMetrics(ctx, q, "site"); err != nil {
			return err
		}
		coverage, err = repository.CollectionCoverage(ctx, q.SiteIDs, "topup")
		return err
	}); err != nil {
		return dto.FinanceStatisticsResponse{}, err
	}
	status, asOf, complete := financeCoverage(coverage)
	out := dto.FinanceStatisticsResponse{StatusBreakdown: financeBreakdown(statuses, true), ProviderBreakdown: financeBreakdown(providers, true), SiteBreakdown: financeSiteBreakdown(sites, coverage, true), DataStatus: status, AsOf: asOf, Completeness: complete}
	if len(summary) > 0 {
		out.Summary = financeMetric(summary[0], true)
		out.Summary.Amount = ""
		out.Summary.Money = ""
	} else {
		out.Summary = dto.FinanceMetric{Count: "0", MissingCount: "0"}
	}
	return out, nil
}
func (s *FinanceOperationsService) RedemptionStatistics(ctx context.Context, q dto.FinanceInventoryQuery) (dto.FinanceStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if s == nil || s.clock == nil {
		return dto.FinanceStatisticsResponse{}, ErrStatisticsInvalid
	}
	now := s.clock.Now().Unix()
	q.StatusAt = now
	if q.Validate() != nil || q.ValidateRedemptionStatuses() != nil {
		return dto.FinanceStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summary, statuses, sites []model.FinanceMetricRow
	var coverage []model.FinanceCollectionCoverageRow
	if err := s.readSnapshot(ctx, func(repository *model.FinanceRepository) error {
		var err error
		if summary, err = repository.RedemptionMetrics(ctx, q, "summary", now); err != nil {
			return err
		}
		if statuses, err = repository.RedemptionMetrics(ctx, q, "status", now); err != nil {
			return err
		}
		if sites, err = repository.RedemptionMetrics(ctx, q, "site", now); err != nil {
			return err
		}
		coverage, err = repository.CollectionCoverage(ctx, q.SiteIDs, "redemption")
		return err
	}); err != nil {
		return dto.FinanceStatisticsResponse{}, err
	}
	status, asOf, complete := financeCoverage(coverage)
	out := dto.FinanceStatisticsResponse{StatusBreakdown: financeBreakdown(statuses, false), SiteBreakdown: financeSiteBreakdown(sites, coverage, false), DataStatus: status, AsOf: asOf, Completeness: complete}
	if len(summary) > 0 {
		out.Summary = financeMetric(summary[0], false)
	} else {
		out.Summary = dto.FinanceMetric{Count: "0", MissingCount: "0", Quota: "0"}
	}
	return out, nil
}
