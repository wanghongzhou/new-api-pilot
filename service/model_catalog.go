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

type ModelCatalogService struct {
	db *gorm.DB
}

func NewModelCatalogService(db *gorm.DB) (*ModelCatalogService, error) {
	if db == nil {
		return nil, errors.New("model catalog database required")
	}
	return &ModelCatalogService{db: db}, nil
}
func (s *ModelCatalogService) readSnapshot(ctx context.Context, read func(*model.ModelCatalogRepository) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(model.NewModelCatalogRepository(tx))
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func (s *ModelCatalogService) List(ctx context.Context, q dto.ModelCatalogQuery) (dto.ModelCatalogPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.ModelCatalogPageResponse{}, ErrStatisticsInvalid
	}
	var rows []model.ModelCatalogReadRow
	var statuses map[int64]model.ModelCatalogStatus
	var total int64
	var overall string
	if err := s.readSnapshot(ctx, func(repo *model.ModelCatalogRepository) error {
		var err error
		rows, total, err = repo.List(ctx, q)
		if err != nil {
			return err
		}
		statuses, overall, _, err = repo.Statuses(ctx, q.SiteIDs)
		return err
	}); err != nil {
		return dto.ModelCatalogPageResponse{}, err
	}
	items := make([]dto.ModelCatalogItem, 0, len(rows))
	for _, r := range rows {
		st := statuses[r.SiteID]
		items = append(items, dto.ModelCatalogItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), RemoteID: strconv.FormatInt(r.RemoteID, 10), SiteName: r.SiteName, ModelName: r.ModelName, Description: r.Description, Icon: r.Icon, Tags: r.Tags, VendorID: strconv.FormatInt(r.VendorID, 10), Status: r.RemoteStatus, SyncOfficial: r.SyncOfficial, NameRule: r.NameRule, CreatedTime: r.RemoteCreatedTime, UpdatedTime: r.RemoteUpdatedTime, CoveredChannels: strconv.FormatInt(r.CoveredChannels, 10), CoveredGroups: strconv.FormatInt(r.CoveredGroups, 10), DataStatus: st.DataStatus})
	}
	return dto.ModelCatalogPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: overall}, nil
}
func (s *ModelCatalogService) Missing(ctx context.Context, q dto.ModelCatalogQuery) (dto.MissingModelPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.MissingModelPageResponse{}, ErrStatisticsInvalid
	}
	var rows []model.MissingModelReadRow
	var statuses map[int64]model.ModelCatalogStatus
	var total int64
	var overall string
	var asOf *int64
	if err := s.readSnapshot(ctx, func(repo *model.ModelCatalogRepository) error {
		var err error
		rows, total, err = repo.MissingRows(ctx, q)
		if err != nil {
			return err
		}
		statuses, overall, asOf, err = repo.Statuses(ctx, q.SiteIDs)
		return err
	}); err != nil {
		return dto.MissingModelPageResponse{}, err
	}
	items := make([]dto.MissingModelItem, 0, len(rows))
	for _, r := range rows {
		st := statuses[r.SiteID]
		items = append(items, dto.MissingModelItem{SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, RemoteChannelID: strconv.FormatInt(r.RemoteChannelID, 10), ChannelName: r.ChannelName, ModelName: r.ModelName, Group: r.RemoteGroup, DataStatus: st.DataStatus, AsOf: st.AsOf})
	}
	return dto.MissingModelPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: overall, AsOf: asOf}, nil
}
func breakdown(rows []model.ModelCoverageRow, statuses map[int64]model.ModelCatalogStatus, overall string) []dto.ModelCoverageBreakdown {
	out := make([]dto.ModelCoverageBreakdown, 0, len(rows))
	for _, r := range rows {
		status, asOf := overall, r.AsOf
		if r.SiteID > 0 {
			st := statuses[r.SiteID]
			status, asOf = st.DataStatus, st.AsOf
		}
		out = append(out, dto.ModelCoverageBreakdown{DimensionID: r.DimensionID, DimensionName: r.DimensionName, SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, CatalogModels: strconv.FormatInt(r.CatalogModels, 10), ExactCoveredModels: strconv.FormatInt(r.ExactCoveredModels, 10), ExactMissingModels: strconv.FormatInt(r.ExactMissingModels, 10), ChannelMappings: strconv.FormatInt(r.ChannelMappings, 10), DataStatus: status, AsOf: asOf})
	}
	return out
}
func (s *ModelCatalogService) Coverage(ctx context.Context, q dto.ModelCatalogQuery) (dto.ModelCoverageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.ModelCoverageResponse{}, ErrStatisticsInvalid
	}
	var statuses map[int64]model.ModelCatalogStatus
	var overall string
	var sites, vendors, states []model.ModelCoverageRow
	var missingTotal int64
	if err := s.readSnapshot(ctx, func(repo *model.ModelCatalogRepository) error {
		var err error
		statuses, overall, _, err = repo.Statuses(ctx, q.SiteIDs)
		if err != nil {
			return err
		}
		sites, err = repo.Coverage(ctx, q, "site")
		if err != nil {
			return err
		}
		vendors, err = repo.Coverage(ctx, q, "vendor")
		if err != nil {
			return err
		}
		states, err = repo.Coverage(ctx, q, "status")
		if err != nil {
			return err
		}
		_, missingTotal, err = repo.MissingRows(ctx, dto.ModelCatalogQuery{Page: 1, PageSize: 1, SiteIDs: q.SiteIDs, Keyword: q.Keyword, VendorID: q.VendorID, Statuses: q.Statuses, SyncOfficial: q.SyncOfficial})
		return err
	}); err != nil {
		return dto.ModelCoverageResponse{}, err
	}
	var catalog, covered, mappings int64
	for _, r := range sites {
		catalog += r.CatalogModels
		covered += r.ExactCoveredModels
		mappings += r.ChannelMappings
	}
	return dto.ModelCoverageResponse{CatalogModels: strconv.FormatInt(catalog, 10), ExactCoveredModels: strconv.FormatInt(covered, 10), ExactMissingModels: strconv.FormatInt(missingTotal, 10), ChannelMappings: strconv.FormatInt(mappings, 10), DataStatus: overall, SiteBreakdown: breakdown(sites, statuses, overall), VendorBreakdown: breakdown(vendors, statuses, overall), StatusBreakdown: breakdown(states, statuses, overall)}, nil
}
