package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
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
func pricingStringMap(raw string) map[string]string {
	values := map[string]string{}
	if json.Unmarshal([]byte(raw), &values) != nil {
		return map[string]string{}
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
	return dto.PricingCatalogItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), VendorID: strconv.FormatInt(row.VendorID, 10), VendorName: row.VendorKey, QuotaType: strconv.FormatInt(row.QuotaType, 10), SiteName: row.SiteName, ModelName: row.ModelName, Description: row.Description, Icon: row.Icon, Tags: row.Tags, OwnerBy: row.OwnerBy, ModelRatio: pricingOutputDecimal(row.ModelRatio), ModelPrice: pricingOutputDecimal(row.ModelPrice), CompletionRatio: pricingOutputDecimal(row.CompletionRatio), CacheRatio: pricingOutputDecimalPointer(row.CacheRatio), CreateCacheRatio: pricingOutputDecimalPointer(row.CreateCacheRatio), ImageRatio: pricingOutputDecimalPointer(row.ImageRatio), AudioRatio: pricingOutputDecimalPointer(row.AudioRatio), AudioCompletionRatio: pricingOutputDecimalPointer(row.AudioCompletionRatio), BillingMode: row.BillingMode, BillingExpr: row.BillingExpr, PricingSource: row.PricingSource, AbilityAvailable: row.AbilityAvailable, EnableGroups: pricingStrings(row.EnableGroupsJSON), SupportedEndpointTypes: pricingStrings(row.SupportedEndpointTypesJSON), PricingVersion: row.PricingVersion, RemoteState: row.RemoteState, MissingCount: row.MissingCount, CollectedAt: row.CollectedAt, DataStatus: status}
}

type pricingGroupAssociation struct {
	active, missing       int64
	models, missingModels map[string]struct{}
}

func sortedPricingKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func pricingGroupItem(row model.PricingGroupReadRow, status string, association pricingGroupAssociation) dto.PricingGroupItem {
	return dto.PricingGroupItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), SiteName: row.SiteName, Name: row.GroupName, Ratio: pricingOutputDecimalPointer(row.RatioDecimal), TopupRatio: pricingOutputDecimalPointer(row.TopupRatioDecimal), Description: row.Description, UserSelectable: row.UserSelectable, DefaultUseAutoGroup: row.DefaultUseAutoGroup, AutoPriority: row.AutoPriority, OutgoingOverrides: pricingStringMap(row.OutgoingOverridesJSON), IncomingOverrides: pricingStringMap(row.IncomingOverridesJSON), VisibleToGroups: pricingStringMap(row.VisibleToGroupsJSON), HiddenFromGroups: pricingStrings(row.HiddenFromGroupsJSON), RemoteState: row.RemoteState, MissingCount: row.MissingCount, ActivePricingCount: strconv.FormatInt(association.active, 10), MissingPricingCount: strconv.FormatInt(association.missing, 10), ModelNames: sortedPricingKeys(association.models), MissingModelNames: sortedPricingKeys(association.missingModels), CollectedAt: row.CollectedAt, DataStatus: status}
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
		out = dto.PricingCatalogPageResponse{Items: items, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, DataStatus: pricingOverall(metrics, "pricing"), AsOf: asOf, SiteBreakdown: breakdown}
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
		associationRows, err := repo.GroupAssociations(ctx, rows)
		if err != nil {
			return err
		}
		associations := map[string]pricingGroupAssociation{}
		for _, associationRow := range associationRows {
			key := strconv.FormatInt(associationRow.SiteID, 10) + "\x00" + associationRow.GroupName
			association := associations[key]
			if association.models == nil {
				association.models = map[string]struct{}{}
				association.missingModels = map[string]struct{}{}
			}
			if associationRow.RemoteState == "missing" {
				association.missing++
				association.missingModels[associationRow.ModelName] = struct{}{}
			} else {
				association.active++
				association.models[associationRow.ModelName] = struct{}{}
			}
			associations[key] = association
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
			key := strconv.FormatInt(row.SiteID, 10) + "\x00" + row.GroupName
			association := associations[key]
			if association.models == nil {
				association = pricingGroupAssociation{models: map[string]struct{}{}, missingModels: map[string]struct{}{}}
			}
			items = append(items, pricingGroupItem(row, bySite[row.SiteID], association))
		}
		out = dto.PricingGroupPageResponse{Items: items, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, DataStatus: pricingOverall(metrics, "group"), AsOf: asOf, SiteBreakdown: breakdown}
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
		out = dto.PricingCatalogStatistics{DataStatus: pricingOverall(rows, "combined"), Sites: []dto.PricingCatalogSiteOverview{}}
		out.SiteCount = strconv.Itoa(len(rows))
		var pricingActive, pricingMissing, groupActive, groupMissing int64
		for _, row := range rows {
			pricingActive += row.Total - row.Missing
			pricingMissing += row.Missing
			groupActive += row.GroupTotal - row.GroupMissing
			groupMissing += row.GroupMissing
			out.Sites = append(out.Sites, dto.PricingCatalogSiteOverview{SiteID: strconv.FormatInt(row.SiteID, 10), SiteName: row.SiteName, PricingActive: strconv.FormatInt(row.Total-row.Missing, 10), PricingMissing: strconv.FormatInt(row.Missing, 10), GroupActive: strconv.FormatInt(row.GroupTotal-row.GroupMissing, 10), GroupMissing: strconv.FormatInt(row.GroupMissing, 10), PricingDataStatus: pricingStatus(row, "pricing"), GroupDataStatus: pricingStatus(row, "group"), PricingAsOf: row.PricingAsOf, GroupAsOf: row.GroupAsOf})
		}
		out.PricingActive = strconv.FormatInt(pricingActive, 10)
		out.PricingMissing = strconv.FormatInt(pricingMissing, 10)
		out.GroupActive = strconv.FormatInt(groupActive, 10)
		out.GroupMissing = strconv.FormatInt(groupMissing, 10)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
