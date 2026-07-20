package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/dto"
)

type SiteModelMeta struct {
	ID, SiteID, RemoteID                 int64
	ModelName, Description, Icon, Tags   string
	VendorID                             int64
	RemoteStatus, SyncOfficial, NameRule int
	RemoteCreatedTime, RemoteUpdatedTime int64
	SourceHash                           string
	ConfigVersion                        int
	CollectedAt, CreatedAt, UpdatedAt    int64
}

func (SiteModelMeta) TableName() string { return "site_model_meta" }

type SiteModelMetaCollectionState struct {
	SiteID                       int64
	LastSuccessAt, LastFailureAt *int64
	LastErrorCode                string
	ObservedCount                int64
	ConfigVersion                int
	UpdatedAt                    int64
}

func (SiteModelMetaCollectionState) TableName() string { return "site_model_meta_collection_state" }

type SiteChannelModelMapping struct {
	ID, SiteID, RemoteChannelID int64
	ModelName, RemoteGroup      string
	ConfigVersion               int
	CollectedAt                 int64
}

func (SiteChannelModelMapping) TableName() string { return "site_channel_model_mapping" }

func splitCatalogValues(v string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
func (r *SiteRepository) ReplaceChannelModelMappings(ctx context.Context, site Site, at int64, channels []SiteChannel) error {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 {
		return errors.New("invalid channel model mapping snapshot")
	}
	rows := []SiteChannelModelMapping{}
	seen := map[string]struct{}{}
	for _, c := range channels {
		groups := splitCatalogValues(c.RemoteGroup)
		if len(groups) == 0 {
			groups = []string{""}
		}
		for _, m := range splitCatalogValues(c.Models) {
			if !utf8.ValidString(m) || len(m) > 255 {
				return errors.New("invalid channel model")
			}
			for _, g := range groups {
				key := strings.Join([]string{strconv.FormatInt(c.RemoteChannelID, 10), m, g}, "\x00")
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				rows = append(rows, SiteChannelModelMapping{SiteID: site.ID, RemoteChannelID: c.RemoteChannelID, ModelName: m, RemoteGroup: g, ConfigVersion: site.ConfigVersion, CollectedAt: at})
			}
		}
	}
	if err := r.db.WithContext(ctx).Where("site_id=?", site.ID).Delete(&SiteChannelModelMapping{}).Error; err != nil {
		return err
	}
	if len(rows) > 0 {
		return r.db.WithContext(ctx).Create(&rows).Error
	}
	return nil
}
func (r *SiteRepository) SyncModelCatalog(ctx context.Context, site Site, at int64, snapshot dto.UpstreamModelMetaSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || snapshot.Total != int64(len(snapshot.Items)) || len(snapshot.Items) > 100000 || (len(snapshot.Items) > 0 && snapshot.MaxID != snapshot.Items[0].ID) || (len(snapshot.Items) == 0 && snapshot.MaxID != 0) {
		return 0, errors.New("invalid model catalog snapshot")
	}
	seen := map[int64]struct{}{}
	type catalogExistingRow struct {
		RemoteID          int64
		RemoteUpdatedTime int64
		SourceHash        string
	}
	var existingRows []catalogExistingRow
	if err := r.db.WithContext(ctx).Model(&SiteModelMeta{}).Select("remote_id", "remote_updated_time", "source_hash").Where("site_id = ?", site.ID).Find(&existingRows).Error; err != nil {
		return 0, err
	}
	existing := make(map[int64]catalogExistingRow, len(existingRows))
	for _, row := range existingRows {
		existing[row.RemoteID] = row
	}
	upserts := make([]SiteModelMeta, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ID <= 0 || item.ModelName == "" || len(item.ModelName) > 128 || item.VendorID < 0 || (item.Status != 0 && item.Status != 1) || (item.SyncOfficial != 0 && item.SyncOfficial != 1) || item.NameRule < 0 || item.NameRule > 3 || item.CreatedTime < 0 || item.UpdatedTime < 0 || !utf8.ValidString(item.Description) || len(item.Icon) > 512 || len(item.Tags) > 255 {
			return 0, errors.New("invalid model catalog item")
		}
		if _, ok := seen[item.ID]; ok {
			return 0, errors.New("duplicate model catalog item")
		}
		seen[item.ID] = struct{}{}
		raw, _ := json.Marshal(item)
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		previous, found := existing[item.ID]
		if found && (item.UpdatedTime < previous.RemoteUpdatedTime || (item.UpdatedTime == previous.RemoteUpdatedTime && hash == previous.SourceHash)) {
			continue
		}
		upserts = append(upserts, SiteModelMeta{SiteID: site.ID, RemoteID: item.ID, ModelName: item.ModelName, Description: item.Description, Icon: item.Icon, Tags: item.Tags, VendorID: item.VendorID, RemoteStatus: item.Status, SyncOfficial: item.SyncOfficial, NameRule: item.NameRule, RemoteCreatedTime: item.CreatedTime, RemoteUpdatedTime: item.UpdatedTime, SourceHash: hash, ConfigVersion: site.ConfigVersion, CollectedAt: at, CreatedAt: at, UpdatedAt: at})
	}
	if len(upserts) > 0 {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"model_name", "description", "icon", "tags", "vendor_id", "remote_status", "sync_official", "name_rule", "remote_created_time", "remote_updated_time", "source_hash", "config_version", "collected_at", "updated_at"})}).CreateInBatches(&upserts, 500).Error; err != nil {
			return 0, err
		}
	}
	var deletedRows int64
	if len(snapshot.Items) == 0 {
		deleted := r.db.WithContext(ctx).Where("site_id = ?", site.ID).Delete(&SiteModelMeta{})
		if deleted.Error != nil {
			return int64(len(upserts)), deleted.Error
		}
		deletedRows = deleted.RowsAffected
	} else {
		deleteIDs := make([]int64, 0)
		for remoteID := range existing {
			if _, kept := seen[remoteID]; !kept {
				deleteIDs = append(deleteIDs, remoteID)
			}
		}
		sort.Slice(deleteIDs, func(i, j int) bool { return deleteIDs[i] < deleteIDs[j] })
		for start := 0; start < len(deleteIDs); start += 500 {
			end := start + 500
			if end > len(deleteIDs) {
				end = len(deleteIDs)
			}
			deleted := r.db.WithContext(ctx).Where("site_id = ? AND remote_id IN ?", site.ID, deleteIDs[start:end]).Delete(&SiteModelMeta{})
			if deleted.Error != nil {
				return int64(len(upserts)) + deletedRows, deleted.Error
			}
			deletedRows += deleted.RowsAffected
		}
	}
	written := int64(len(upserts)) + deletedRows
	state := SiteModelMetaCollectionState{SiteID: site.ID, LastSuccessAt: &at, ObservedCount: snapshot.Total, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_success_at", "last_error_code", "observed_count", "config_version", "updated_at"})}).Create(&state).Error; err != nil {
		return written, err
	}
	return written, nil
}
func (r *SiteRepository) MarkModelCatalogFailure(ctx context.Context, site Site, at int64, code string) error {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || code == "" {
		return errors.New("invalid model catalog failure")
	}
	current, err := r.FindByIDForUpdate(ctx, site.ID)
	if err != nil {
		return err
	}
	if current.ConfigVersion != site.ConfigVersion {
		return ErrSiteRunConfigChanged
	}
	row := SiteModelMetaCollectionState{SiteID: site.ID, LastFailureAt: &at, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
}

type ModelCatalogReadRow struct {
	SiteModelMeta
	SiteName                       string
	CoveredChannels, CoveredGroups int64
}
type ModelCatalogRepository struct{ db *gorm.DB }

func NewModelCatalogRepository(db *gorm.DB) *ModelCatalogRepository {
	return &ModelCatalogRepository{db: db}
}
func (r *ModelCatalogRepository) base(ctx context.Context, q dto.ModelCatalogQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Table("site_model_meta m").Joins("JOIN site s ON s.id=m.site_id")
	if len(q.SiteIDs) > 0 {
		db = db.Where("m.site_id IN ?", q.SiteIDs)
	}
	if q.VendorID != nil {
		db = db.Where("m.vendor_id=?", *q.VendorID)
	}
	if len(q.Statuses) > 0 {
		db = db.Where("m.remote_status IN ?", q.Statuses)
	}
	if len(q.SyncOfficial) > 0 {
		db = db.Where("m.sync_official IN ?", q.SyncOfficial)
	}
	if q.Keyword != "" {
		db = db.Where("m.model_name LIKE ?", "%"+escapeLike(q.Keyword)+"%")
	}
	return db
}
func (r *ModelCatalogRepository) List(ctx context.Context, q dto.ModelCatalogQuery) ([]ModelCatalogReadRow, int64, error) {
	db := r.base(ctx, q)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModelCatalogReadRow
	err := db.Select("m.*,s.name site_name,(SELECT COUNT(DISTINCT x.remote_channel_id) FROM site_channel_model_mapping x WHERE m.name_rule=0 AND x.site_id=m.site_id AND x.model_name=m.model_name) covered_channels,(SELECT COUNT(DISTINCT x.remote_group) FROM site_channel_model_mapping x WHERE m.name_rule=0 AND x.site_id=m.site_id AND x.model_name=m.model_name) covered_groups").Order("m.site_id,m.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *ModelCatalogRepository) Status(ctx context.Context, siteIDs []int64) (string, error) {
	var expected int64
	db := r.db.WithContext(ctx).Model(&Site{}).Where("management_status='active'")
	if len(siteIDs) > 0 {
		db = db.Where("id IN ?", siteIDs)
	}
	if err := db.Count(&expected).Error; err != nil {
		return "", err
	}
	var complete int64
	db = r.db.WithContext(ctx).Model(&SiteModelMetaCollectionState{}).Where("last_success_at IS NOT NULL AND (last_failure_at IS NULL OR last_success_at>=last_failure_at)")
	if len(siteIDs) > 0 {
		db = db.Where("site_id IN ?", siteIDs)
	}
	if err := db.Count(&complete).Error; err != nil {
		return "", err
	}
	if expected == 0 {
		return "pending", nil
	}
	if complete == expected {
		return "complete", nil
	}
	if complete == 0 {
		return "unavailable", nil
	}
	return "partial", nil
}
func (r *ModelCatalogRepository) Missing(ctx context.Context, q dto.ModelCatalogQuery) ([]string, error) {
	db := r.db.WithContext(ctx).Table("site_channel_model_mapping x").Joins("LEFT JOIN site_model_meta m ON m.site_id=x.site_id AND m.model_name=x.model_name AND m.name_rule=0").Where("m.id IS NULL")
	if len(q.SiteIDs) > 0 {
		db = db.Where("x.site_id IN ?", q.SiteIDs)
	}
	var names []string
	err := db.Distinct().Order("x.model_name").Pluck("x.model_name", &names).Error
	return names, err
}

type ModelCatalogStatus struct {
	DataStatus string
	AsOf       *int64
}

func (r *ModelCatalogRepository) Statuses(ctx context.Context, siteIDs []int64) (map[int64]ModelCatalogStatus, string, *int64, error) {
	var sites []Site
	db := r.db.WithContext(ctx).Where("management_status='active'")
	if len(siteIDs) > 0 {
		db = db.Where("id IN ?", siteIDs)
	}
	if err := db.Find(&sites).Error; err != nil {
		return nil, "", nil, err
	}
	var states []SiteModelMetaCollectionState
	db = r.db.WithContext(ctx)
	if len(siteIDs) > 0 {
		db = db.Where("site_id IN ?", siteIDs)
	}
	if err := db.Find(&states).Error; err != nil {
		return nil, "", nil, err
	}
	by := map[int64]SiteModelMetaCollectionState{}
	for _, x := range states {
		by[x.SiteID] = x
	}
	out := map[int64]ModelCatalogStatus{}
	statusCounts := map[string]int{}
	var asOf *int64
	for _, site := range sites {
		x, ok := by[site.ID]
		st := ModelCatalogStatus{DataStatus: "pending"}
		if ok && x.LastSuccessAt != nil && (x.LastFailureAt == nil || *x.LastSuccessAt >= *x.LastFailureAt) {
			st.DataStatus = "complete"
			st.AsOf = x.LastSuccessAt
			if asOf == nil || *x.LastSuccessAt > *asOf {
				v := *x.LastSuccessAt
				asOf = &v
			}
		} else if ok && x.LastFailureAt != nil {
			st.DataStatus = "unavailable"
		}
		statusCounts[st.DataStatus]++
		out[site.ID] = st
	}
	overall := "pending"
	if len(sites) == 0 {
		return out, overall, asOf, nil
	}
	for _, candidate := range []string{"complete", "unavailable", "pending"} {
		if statusCounts[candidate] == len(sites) {
			return out, candidate, asOf, nil
		}
	}
	return out, "partial", asOf, nil
}

type MissingModelReadRow struct {
	SiteID, RemoteChannelID                       int64
	SiteName, ChannelName, ModelName, RemoteGroup string
	AsOf                                          *int64
}

func (r *ModelCatalogRepository) MissingRows(ctx context.Context, q dto.ModelCatalogQuery) ([]MissingModelReadRow, int64, error) {
	if q.VendorID != nil || len(q.Statuses) > 0 || len(q.SyncOfficial) > 0 {
		return []MissingModelReadRow{}, 0, nil
	}
	db := r.db.WithContext(ctx).Table("site_channel_model_mapping x").Joins("JOIN site s ON s.id=x.site_id").Joins("LEFT JOIN site_channel_inventory c ON c.site_id=x.site_id AND c.remote_channel_id=x.remote_channel_id").Joins("LEFT JOIN site_model_meta m ON m.site_id=x.site_id AND m.model_name=x.model_name AND m.name_rule=0").Joins("LEFT JOIN site_model_meta_collection_state st ON st.site_id=x.site_id").Where("m.id IS NULL")
	if len(q.SiteIDs) > 0 {
		db = db.Where("x.site_id IN ?", q.SiteIDs)
	}
	if q.Keyword != "" {
		db = db.Where("x.model_name LIKE ?", "%"+escapeLike(q.Keyword)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []MissingModelReadRow
	err := db.Select("x.site_id,s.name site_name,x.remote_channel_id,c.name channel_name,x.model_name,x.remote_group,st.last_success_at as_of").Order("x.site_id,x.model_name,x.remote_channel_id,x.remote_group").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}

type ModelCoverageRow struct {
	DimensionID, DimensionName                                             string
	SiteID                                                                 int64
	SiteName                                                               string
	CatalogModels, ExactCoveredModels, ExactMissingModels, ChannelMappings int64
	AsOf                                                                   *int64
}

func (r *ModelCatalogRepository) Coverage(ctx context.Context, q dto.ModelCatalogQuery, dim string) ([]ModelCoverageRow, error) {
	if dim == "site" {
		return r.siteCoverage(ctx, q)
	}
	expr := map[string]string{"site": "CAST(m.site_id AS CHAR)", "vendor": "CAST(m.vendor_id AS CHAR)", "status": "CAST(m.remote_status AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid model coverage dimension")
	}
	db := r.base(ctx, q).Joins("LEFT JOIN site_channel_model_mapping x ON x.site_id=m.site_id AND x.model_name=m.model_name AND m.name_rule=0")
	siteID, siteName := "0", "''"
	group := expr
	if dim == "site" {
		siteID = "m.site_id"
		siteName = "MAX(s.name)"
		group = "m.site_id"
	}
	sql := expr + " dimension_id," + expr + " dimension_name," + siteID + " site_id," + siteName + " site_name,COUNT(DISTINCT m.id) catalog_models,COUNT(DISTINCT CASE WHEN x.id IS NOT NULL THEN CONCAT(m.site_id,':',m.model_name) END) exact_covered_models,0 exact_missing_models,COUNT(DISTINCT x.id) channel_mappings,MAX(m.collected_at) as_of"
	var rows []ModelCoverageRow
	err := db.Select(sql).Group(group).Order("dimension_id").Scan(&rows).Error
	return rows, err
}

func (r *ModelCatalogRepository) siteCoverage(ctx context.Context, q dto.ModelCatalogQuery) ([]ModelCoverageRow, error) {
	var sites []Site
	siteDB := r.db.WithContext(ctx).Where("management_status='active'")
	if len(q.SiteIDs) > 0 {
		siteDB = siteDB.Where("id IN ?", q.SiteIDs)
	}
	if err := siteDB.Order("id").Find(&sites).Error; err != nil {
		return nil, err
	}
	rowsBySite := make(map[int64]ModelCoverageRow, len(sites))
	for _, site := range sites {
		rowsBySite[site.ID] = ModelCoverageRow{DimensionID: strconv.FormatInt(site.ID, 10), DimensionName: strconv.FormatInt(site.ID, 10), SiteID: site.ID, SiteName: site.Name}
	}
	var catalogRows []ModelCoverageRow
	catalogDB := r.base(ctx, q).Joins("LEFT JOIN site_channel_model_mapping x ON x.site_id=m.site_id AND x.model_name=m.model_name AND m.name_rule=0")
	if err := catalogDB.Select("CAST(m.site_id AS CHAR) dimension_id,CAST(m.site_id AS CHAR) dimension_name,m.site_id,MAX(s.name) site_name,COUNT(DISTINCT m.id) catalog_models,COUNT(DISTINCT CASE WHEN x.id IS NOT NULL THEN m.model_name END) exact_covered_models,COUNT(DISTINCT x.id) channel_mappings,MAX(m.collected_at) as_of").Group("m.site_id").Scan(&catalogRows).Error; err != nil {
		return nil, err
	}
	for _, row := range catalogRows {
		current, ok := rowsBySite[row.SiteID]
		if !ok {
			continue
		}
		current.CatalogModels, current.ExactCoveredModels, current.AsOf = row.CatalogModels, row.ExactCoveredModels, row.AsOf
		rowsBySite[row.SiteID] = current
	}

	type siteCount struct{ SiteID, Count int64 }
	var mappingCounts []siteCount
	mappingDB := r.db.WithContext(ctx).Table("site_channel_model_mapping x")
	filteredCatalog := q.VendorID != nil || len(q.Statuses) > 0 || len(q.SyncOfficial) > 0
	if filteredCatalog {
		mappingDB = mappingDB.Joins("JOIN site_model_meta m ON m.site_id=x.site_id AND m.model_name=x.model_name AND m.name_rule=0")
		if q.VendorID != nil {
			mappingDB = mappingDB.Where("m.vendor_id=?", *q.VendorID)
		}
		if len(q.Statuses) > 0 {
			mappingDB = mappingDB.Where("m.remote_status IN ?", q.Statuses)
		}
		if len(q.SyncOfficial) > 0 {
			mappingDB = mappingDB.Where("m.sync_official IN ?", q.SyncOfficial)
		}
	}
	if len(q.SiteIDs) > 0 {
		mappingDB = mappingDB.Where("x.site_id IN ?", q.SiteIDs)
	}
	if q.Keyword != "" {
		mappingDB = mappingDB.Where("x.model_name LIKE ?", "%"+escapeLike(q.Keyword)+"%")
	}
	if err := mappingDB.Select("x.site_id,COUNT(DISTINCT x.id) count").Group("x.site_id").Scan(&mappingCounts).Error; err != nil {
		return nil, err
	}
	for _, count := range mappingCounts {
		current, ok := rowsBySite[count.SiteID]
		if ok {
			current.ChannelMappings = count.Count
			rowsBySite[count.SiteID] = current
		}
	}
	if !filteredCatalog {
		var missingCounts []siteCount
		missingDB := r.db.WithContext(ctx).Table("site_channel_model_mapping x").Joins("LEFT JOIN site_model_meta m ON m.site_id=x.site_id AND m.model_name=x.model_name AND m.name_rule=0").Where("m.id IS NULL")
		if len(q.SiteIDs) > 0 {
			missingDB = missingDB.Where("x.site_id IN ?", q.SiteIDs)
		}
		if q.Keyword != "" {
			missingDB = missingDB.Where("x.model_name LIKE ?", "%"+escapeLike(q.Keyword)+"%")
		}
		if err := missingDB.Select("x.site_id,COUNT(*) count").Group("x.site_id").Scan(&missingCounts).Error; err != nil {
			return nil, err
		}
		for _, count := range missingCounts {
			current, ok := rowsBySite[count.SiteID]
			if ok {
				current.ExactMissingModels = count.Count
				rowsBySite[count.SiteID] = current
			}
		}
	}
	rows := make([]ModelCoverageRow, 0, len(sites))
	for _, site := range sites {
		rows = append(rows, rowsBySite[site.ID])
	}
	return rows, nil
}
