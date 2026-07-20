package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type PricingCatalogService struct{ db *gorm.DB }

func NewPricingCatalogService(db *gorm.DB) (*PricingCatalogService, error) {
	if db == nil {
		return nil, errors.New("pricing catalog database required")
	}
	return &PricingCatalogService{db: db}, nil
}

func pricingStrings(raw string) []string {
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil {
		return []string{}
	}
	return values
}
func pricingOutputDecimal(raw string) string {
	number := json.Number(raw)
	value, ok := canonicalPricingDecimal(&number)
	if !ok || value == nil {
		return raw
	}
	return *value
}
func pricingOutputDecimalPointer(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := pricingOutputDecimal(*raw)
	return &value
}
func pricingItem(row model.PricingCatalogReadRow, status string) dto.PricingCatalogItem {
	return dto.PricingCatalogItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), VendorID: strconv.FormatInt(row.VendorID, 10), VendorKey: row.VendorKey, QuotaType: strconv.FormatInt(row.QuotaType, 10), SiteName: row.SiteName, ModelName: row.ModelName, Description: row.Description, Icon: row.Icon, Tags: row.Tags, OwnerBy: row.OwnerBy, ModelRatio: pricingOutputDecimal(row.ModelRatio), ModelPrice: pricingOutputDecimal(row.ModelPrice), CompletionRatio: pricingOutputDecimal(row.CompletionRatio), CacheRatio: pricingOutputDecimalPointer(row.CacheRatio), CreateCacheRatio: pricingOutputDecimalPointer(row.CreateCacheRatio), ImageRatio: pricingOutputDecimalPointer(row.ImageRatio), AudioRatio: pricingOutputDecimalPointer(row.AudioRatio), AudioCompletionRatio: pricingOutputDecimalPointer(row.AudioCompletionRatio), EnableGroups: pricingStrings(row.EnableGroupsJSON), SupportedEndpointTypes: pricingStrings(row.SupportedEndpointTypesJSON), PricingVersion: row.PricingVersion, RootVisible: row.RootVisible, RemoteState: row.RemoteState, MissingCount: row.MissingCount, CollectedAt: row.CollectedAt, DataStatus: status}
}
func pricingGroupItem(row model.PricingGroupReadRow, status string) dto.PricingGroupItem {
	return dto.PricingGroupItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), SiteName: row.SiteName, Name: row.GroupName, Ratio: pricingOutputDecimalPointer(row.RatioDecimal), Description: row.Description, RootVisible: row.RootVisible, RemoteState: row.RemoteState, MissingCount: row.MissingCount, CollectedAt: row.CollectedAt, DataStatus: status}
}
func pricingResourceStatus(complete, failure *int64) string {
	if complete != nil && (failure == nil || *complete >= *failure) {
		return "complete"
	}
	if failure != nil {
		return "unavailable"
	}
	return "pending"
}
func pricingStatus(row model.PricingCatalogMetricRow, kind string) string {
	if kind == "group" {
		return pricingResourceStatus(row.GroupLastCompleteAt, row.GroupLastFailureAt)
	}
	return pricingResourceStatus(row.PricingLastCompleteAt, row.PricingLastFailureAt)
}
func pricingCombinedStatus(row model.PricingCatalogMetricRow) string {
	pricing, group := pricingStatus(row, "pricing"), pricingStatus(row, "group")
	if pricing == group {
		return pricing
	}
	if pricing == "pending" && group == "pending" {
		return "pending"
	}
	return "partial"
}
func pricingOverall(rows []model.PricingCatalogMetricRow, kind string) string {
	if len(rows) == 0 {
		return "pending"
	}
	complete, unavailable := 0, 0
	for _, row := range rows {
		status := pricingCombinedStatus(row)
		if kind != "combined" {
			status = pricingStatus(row, kind)
		}
		switch status {
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

func (s *PricingCatalogService) List(ctx context.Context, q dto.PricingCatalogQuery) (dto.PricingCatalogPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.PricingCatalogPageResponse{}, ErrStatisticsInvalid
	}
	var out dto.PricingCatalogPageResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewPricingCatalogRepository(tx)
		rows, total, err := repo.List(ctx, q)
		if err != nil {
			return err
		}
		metrics, err := repo.SiteMetrics(ctx, q)
		if err != nil {
			return err
		}
		bySite := map[int64]string{}
		breakdown := make([]dto.PricingCatalogSiteBreakdown, 0, len(metrics))
		var asOf *int64
		for _, m := range metrics {
			bySite[m.SiteID] = pricingStatus(m, "pricing")
			breakdown = append(breakdown, dto.PricingCatalogSiteBreakdown{SiteID: strconv.FormatInt(m.SiteID, 10), SiteName: m.SiteName, Total: strconv.FormatInt(m.Total, 10), Missing: strconv.FormatInt(m.Missing, 10), DataStatus: pricingStatus(m, "pricing"), AsOf: m.PricingAsOf})
			if m.PricingAsOf != nil && (asOf == nil || *m.PricingAsOf > *asOf) {
				asOf = m.PricingAsOf
			}
		}
		items := make([]dto.PricingCatalogItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, pricingItem(row, bySite[row.SiteID]))
		}
		out = dto.PricingCatalogPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: pricingOverall(metrics, "pricing"), AsOf: asOf, SiteBreakdown: breakdown}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
func (s *PricingCatalogService) ListGroups(ctx context.Context, q dto.PricingCatalogQuery) (dto.PricingGroupPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.PricingGroupPageResponse{}, ErrStatisticsInvalid
	}
	var out dto.PricingGroupPageResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewPricingCatalogRepository(tx)
		rows, total, err := repo.ListGroups(ctx, q)
		if err != nil {
			return err
		}
		metrics, err := repo.SiteMetrics(ctx, q)
		if err != nil {
			return err
		}
		bySite := map[int64]string{}
		breakdown := make([]dto.PricingCatalogSiteBreakdown, 0, len(metrics))
		var asOf *int64
		for _, m := range metrics {
			bySite[m.SiteID] = pricingStatus(m, "group")
			breakdown = append(breakdown, dto.PricingCatalogSiteBreakdown{SiteID: strconv.FormatInt(m.SiteID, 10), SiteName: m.SiteName, Total: strconv.FormatInt(m.GroupTotal, 10), Missing: strconv.FormatInt(m.GroupMissing, 10), DataStatus: pricingStatus(m, "group"), AsOf: m.GroupAsOf})
			if m.GroupAsOf != nil && (asOf == nil || *m.GroupAsOf > *asOf) {
				asOf = m.GroupAsOf
			}
		}
		items := make([]dto.PricingGroupItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, pricingGroupItem(row, bySite[row.SiteID]))
		}
		out = dto.PricingGroupPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: pricingOverall(metrics, "group"), AsOf: asOf, SiteBreakdown: breakdown}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
func (s *PricingCatalogService) Statistics(ctx context.Context, q dto.PricingCatalogQuery) (dto.PricingCatalogStatistics, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if q.Validate() != nil {
		return dto.PricingCatalogStatistics{}, ErrStatisticsInvalid
	}
	var out dto.PricingCatalogStatistics
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := model.NewPricingCatalogRepository(tx)
		rows, err := repo.SiteMetrics(ctx, q)
		if err != nil {
			return err
		}
		vendors, err := repo.VendorMetrics(ctx, q)
		if err != nil {
			return err
		}
		pricingGroups, err := repo.PricingGroupMetrics(ctx, q)
		if err != nil {
			return err
		}
		availability, err := repo.GroupAvailabilityMetrics(ctx, q)
		if err != nil {
			return err
		}
		out = dto.PricingCatalogStatistics{DataStatus: pricingOverall(rows, "combined"), SiteBreakdown: []dto.PricingCatalogSiteBreakdown{}, VendorBreakdown: []dto.PricingVendorBreakdown{}, GroupBreakdown: []dto.PricingModelGroupBreakdown{}, GroupCatalogBreakdown: []dto.GroupCatalogAvailabilityBreakdown{}}
		var total, missing, groups int64
		for _, row := range rows {
			total += row.Total
			missing += row.Missing
			groups += row.GroupTotal
			asOf := row.PricingAsOf
			if row.GroupAsOf != nil && (asOf == nil || *row.GroupAsOf > *asOf) {
				asOf = row.GroupAsOf
			}
			out.SiteBreakdown = append(out.SiteBreakdown, dto.PricingCatalogSiteBreakdown{SiteID: strconv.FormatInt(row.SiteID, 10), SiteName: row.SiteName, Total: strconv.FormatInt(row.Total, 10), Missing: strconv.FormatInt(row.Missing, 10), DataStatus: pricingCombinedStatus(row), AsOf: asOf})
		}
		out.Total = strconv.FormatInt(total, 10)
		out.Missing = strconv.FormatInt(missing, 10)
		out.GroupTotal = strconv.FormatInt(groups, 10)
		for _, row := range vendors {
			out.VendorBreakdown = append(out.VendorBreakdown, dto.PricingVendorBreakdown{VendorKey: row.VendorKey, VendorID: strconv.FormatInt(row.VendorID, 10), Total: strconv.FormatInt(row.Total, 10), Missing: strconv.FormatInt(row.Missing, 10)})
		}
		for _, row := range pricingGroups {
			out.GroupBreakdown = append(out.GroupBreakdown, dto.PricingModelGroupBreakdown{GroupName: row.GroupName, ModelCount: strconv.FormatInt(row.ModelCount, 10)})
		}
		for _, row := range availability {
			out.GroupCatalogBreakdown = append(out.GroupCatalogBreakdown, dto.GroupCatalogAvailabilityBreakdown{RootVisible: row.RootVisible, RatioAvailable: row.RatioAvailable, Count: strconv.FormatInt(row.Count, 10)})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
