package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/dto"
)

type SitePricingCatalog struct {
	ID, SiteID                                                                 int64
	ModelName, VendorKey, Description, Icon, Tags, OwnerBy                     string
	VendorID, QuotaType                                                        int64
	ModelRatio, ModelPrice, CompletionRatio                                    string
	CacheRatio, CreateCacheRatio, ImageRatio, AudioRatio, AudioCompletionRatio *string
	EnableGroupsJSON, SupportedEndpointTypesJSON, PricingVersion               string
	BillingMode, BillingExpr, PricingSource                                    string
	AbilityAvailable                                                           bool
	SourceHash, RemoteState                                                    string
	MissingCount, ConfigVersion                                                int
	FirstSeenAt                                                                int64
	LastSeenAt                                                                 *int64
	CollectedAt, CreatedAt, UpdatedAt                                          int64
}

func (SitePricingCatalog) TableName() string { return "site_pricing_catalog" }

type SitePricingGroup struct {
	ID, SiteID                                                                              int64
	GroupName, Description                                                                  string
	RatioDecimal, TopupRatioDecimal                                                         *string
	UserSelectable, DefaultUseAutoGroup                                                     bool
	AutoPriority                                                                            *int
	OutgoingOverridesJSON, IncomingOverridesJSON, VisibleToGroupsJSON, HiddenFromGroupsJSON string
	SourceHash, RemoteState                                                                 string
	MissingCount, ConfigVersion                                                             int
	FirstSeenAt                                                                             int64
	LastSeenAt                                                                              *int64
	CollectedAt, CreatedAt, UpdatedAt                                                       int64
}

func (SitePricingGroup) TableName() string { return "site_group_catalog" }

type SitePricingCollectionState struct {
	SiteID                              int64
	ResourceKind                        string
	DataStatus                          string
	AsOf, LastCompleteAt, LastFailureAt *int64
	LastErrorCode                       string
	ObservedCount                       int64
	ConfigVersion                       int
	UpdatedAt                           int64
}

func (SitePricingCollectionState) TableName() string { return "pricing_group_collection_state" }

func pricingHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortedJSON(values []string) string {
	copyValues := append([]string{}, values...)
	sort.Strings(copyValues)
	raw, _ := json.Marshal(copyValues)
	return string(raw)
}

func (r *SiteRepository) SyncPricingCatalog(ctx context.Context, site Site, at int64, snapshot dto.UpstreamPricingSnapshot) (int64, error) {
	groups, err := r.SyncPricingGroups(ctx, site, at, dto.UpstreamPricingGroupSnapshot{Groups: snapshot.Groups})
	if err != nil {
		return groups, err
	}
	pricing, err := r.SyncPricingItems(ctx, site, at, dto.UpstreamPricingOnlySnapshot{PricingVersion: snapshot.PricingVersion, Items: snapshot.Items, Groups: snapshot.Groups})
	return groups + pricing, err
}

func (r *SiteRepository) SyncPricingGroups(ctx context.Context, site Site, at int64, snapshot dto.UpstreamPricingGroupSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || len(snapshot.Groups) > 10000 {
		return 0, errors.New("invalid pricing group snapshot")
	}
	var written int64
	if err := r.syncPricingGroups(ctx, site, at, snapshot.Groups, &written); err != nil {
		return written, err
	}
	return written, r.markPricingResourceSuccess(ctx, site, at, "group", int64(len(snapshot.Groups)))
}
func (r *SiteRepository) SyncPricingItems(ctx context.Context, site Site, at int64, snapshot dto.UpstreamPricingOnlySnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || len(snapshot.Items) > 100000 {
		return 0, errors.New("invalid pricing snapshot")
	}
	var written int64
	if err := r.syncPricingItems(ctx, site, at, dto.UpstreamPricingSnapshot{PricingVersion: snapshot.PricingVersion, Items: snapshot.Items}, &written); err != nil {
		return written, err
	}
	return written, r.markPricingResourceSuccess(ctx, site, at, "pricing", int64(len(snapshot.Items)))
}
func (r *SiteRepository) markPricingResourceSuccess(ctx context.Context, site Site, at int64, kind string, count int64) error {
	state := SitePricingCollectionState{SiteID: site.ID, ResourceKind: kind, DataStatus: "complete", AsOf: &at, LastCompleteAt: &at, ObservedCount: count, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "resource_kind"}}, DoUpdates: clause.AssignmentColumns([]string{"data_status", "as_of", "last_complete_at", "last_error_code", "observed_count", "config_version", "updated_at"})}).Create(&state).Error
}

func (r *SiteRepository) syncPricingItems(ctx context.Context, site Site, at int64, snapshot dto.UpstreamPricingSnapshot, written *int64) error {
	items := append([]dto.UpstreamPricingItem{}, snapshot.Items...)
	pricingKey := func(item dto.UpstreamPricingItem) string { return item.ModelName }
	sort.Slice(items, func(i, j int) bool { return pricingKey(items[i]) < pricingKey(items[j]) })
	for i, item := range items {
		if item.ModelName == "" || item.VendorName == "" || len(item.ModelName) > 255 || len(item.VendorName) > 128 || i > 0 && pricingKey(items[i-1]) == pricingKey(item) {
			return errors.New("invalid pricing item")
		}
	}
	var previousRows []SitePricingCatalog
	if err := r.db.WithContext(ctx).Where("site_id=?", site.ID).Find(&previousRows).Error; err != nil {
		return err
	}
	previous := make(map[string]SitePricingCatalog, len(previousRows))
	duplicateIDs := make([]int64, 0)
	for _, row := range previousRows {
		selected, exists := previous[row.ModelName]
		if !exists || row.RemoteState == "normal" && selected.RemoteState != "normal" || row.RemoteState == selected.RemoteState && row.UpdatedAt > selected.UpdatedAt {
			if exists {
				duplicateIDs = append(duplicateIDs, selected.ID)
			}
			previous[row.ModelName] = row
		} else {
			duplicateIDs = append(duplicateIDs, row.ID)
		}
	}
	if len(duplicateIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", duplicateIDs).Delete(&SitePricingCatalog{}).Error; err != nil {
			return err
		}
		*written += int64(len(duplicateIDs))
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[pricingKey(item)] = struct{}{}
	}
	missing := make([]int64, 0)
	for key, row := range previous {
		if _, ok := seen[key]; !ok && row.RemoteState != "missing" {
			missing = append(missing, row.ID)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	for start := 0; start < len(missing); start += 500 {
		end := start + 500
		if end > len(missing) {
			end = len(missing)
		}
		result := r.db.WithContext(ctx).Model(&SitePricingCatalog{}).Where("site_id=? AND id IN ? AND remote_state<>'missing'", site.ID, missing[start:end]).Updates(map[string]any{"remote_state": "missing", "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": at})
		if result.Error != nil {
			return result.Error
		}
		*written += result.RowsAffected
	}
	for _, item := range items {
		enableJSON, endpointJSON := sortedJSON(item.EnableGroups), sortedJSON(item.SupportedEndpointTypes)
		hash := pricingHash(struct {
			Item    dto.UpstreamPricingItem
			Version string
		}{item, snapshot.PricingVersion})
		seenAt := at
		row := SitePricingCatalog{SiteID: site.ID, ModelName: item.ModelName, VendorKey: item.VendorName, Description: item.Description, Icon: item.Icon, Tags: item.Tags, OwnerBy: item.OwnerBy, VendorID: item.VendorID, QuotaType: item.QuotaType, ModelRatio: item.ModelRatio, ModelPrice: item.ModelPrice, CompletionRatio: item.CompletionRatio, CacheRatio: item.CacheRatio, CreateCacheRatio: item.CreateCacheRatio, ImageRatio: item.ImageRatio, AudioRatio: item.AudioRatio, AudioCompletionRatio: item.AudioCompletionRatio, EnableGroupsJSON: enableJSON, SupportedEndpointTypesJSON: endpointJSON, PricingVersion: snapshot.PricingVersion, BillingMode: item.BillingMode, BillingExpr: item.BillingExpr, PricingSource: item.PricingSource, AbilityAvailable: item.AbilityAvailable, SourceHash: hash, RemoteState: "normal", ConfigVersion: site.ConfigVersion, FirstSeenAt: at, LastSeenAt: &seenAt, CollectedAt: at, CreatedAt: at, UpdatedAt: at}
		old, ok := previous[pricingKey(item)]
		if ok {
			row.ID, row.FirstSeenAt, row.CreatedAt = old.ID, old.FirstSeenAt, old.CreatedAt
			if err := r.db.WithContext(ctx).Model(&SitePricingCatalog{}).Where("id=?", old.ID).Select("*").Omit("id", "site_id", "model_name", "first_seen_at", "created_at").Updates(&row).Error; err != nil {
				return err
			}
			if old.SourceHash != hash || old.RemoteState != "normal" {
				*written += 1
			}
		} else {
			if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
				return err
			}
			*written += 1
		}
	}
	return nil
}

func (r *SiteRepository) syncPricingGroups(ctx context.Context, site Site, at int64, groups []dto.UpstreamPricingGroup, written *int64) error {
	groups = append([]dto.UpstreamPricingGroup{}, groups...)
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	for i, g := range groups {
		if g.Name == "" || len(g.Name) > 128 || i > 0 && groups[i-1].Name == g.Name {
			return errors.New("invalid pricing group")
		}
	}
	var oldRows []SitePricingGroup
	if err := r.db.WithContext(ctx).Where("site_id=?", site.ID).Find(&oldRows).Error; err != nil {
		return err
	}
	old := map[string]SitePricingGroup{}
	for _, row := range oldRows {
		old[row.GroupName] = row
	}
	seen := map[string]struct{}{}
	for _, g := range groups {
		seen[g.Name] = struct{}{}
	}
	missing := []string{}
	for name, row := range old {
		if _, ok := seen[name]; !ok && row.RemoteState != "missing" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result := r.db.WithContext(ctx).Model(&SitePricingGroup{}).Where("site_id=? AND group_name IN ? AND remote_state<>'missing'", site.ID, missing).Updates(map[string]any{"remote_state": "missing", "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": at})
		if result.Error != nil {
			return result.Error
		}
		*written += result.RowsAffected
	}
	rows := make([]SitePricingGroup, 0, len(groups))
	var changed int64
	for _, g := range groups {
		hash := pricingHash(g)
		seenAt := at
		outgoing, _ := json.Marshal(g.OutgoingOverrides)
		incoming, _ := json.Marshal(g.IncomingOverrides)
		visible, _ := json.Marshal(g.VisibleToGroups)
		hidden, _ := json.Marshal(g.HiddenFromGroups)
		rows = append(rows, SitePricingGroup{SiteID: site.ID, GroupName: g.Name, RatioDecimal: g.Ratio, TopupRatioDecimal: g.TopupRatio, Description: g.Description, UserSelectable: g.UserSelectable, DefaultUseAutoGroup: g.DefaultUseAutoGroup, AutoPriority: g.AutoPriority, OutgoingOverridesJSON: string(outgoing), IncomingOverridesJSON: string(incoming), VisibleToGroupsJSON: string(visible), HiddenFromGroupsJSON: string(hidden), SourceHash: hash, RemoteState: "normal", ConfigVersion: site.ConfigVersion, FirstSeenAt: at, LastSeenAt: &seenAt, CollectedAt: at, CreatedAt: at, UpdatedAt: at})
		row, ok := old[g.Name]
		if !ok || row.SourceHash != hash || row.RemoteState != "normal" {
			changed++
		}
	}
	if len(rows) > 0 {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "group_name"}}, DoUpdates: clause.AssignmentColumns([]string{"ratio_decimal", "topup_ratio_decimal", "description", "user_selectable", "default_use_auto_group", "auto_priority", "outgoing_overrides_json", "incoming_overrides_json", "visible_to_groups_json", "hidden_from_groups_json", "source_hash", "remote_state", "missing_count", "config_version", "last_seen_at", "collected_at", "updated_at"})}).CreateInBatches(&rows, 500).Error; err != nil {
			return err
		}
		*written += changed
	}
	return nil
}

func (r *SiteRepository) MarkPricingCatalogFailure(ctx context.Context, site Site, at int64, code string) error {
	return r.MarkPricingResourceFailure(ctx, site, at, "pricing", code)
}
func (r *SiteRepository) MarkPricingResourceFailure(ctx context.Context, site Site, at int64, kind, code string) error {
	if kind != "pricing" && kind != "group" {
		return errors.New("invalid pricing resource kind")
	}
	current, err := r.FindByIDForUpdate(ctx, site.ID)
	if err != nil {
		return err
	}
	if current.ConfigVersion != site.ConfigVersion {
		return ErrSiteRunConfigChanged
	}
	row := SitePricingCollectionState{SiteID: site.ID, ResourceKind: kind, DataStatus: "unavailable", AsOf: &at, LastFailureAt: &at, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "resource_kind"}}, DoUpdates: clause.AssignmentColumns([]string{"data_status", "as_of", "last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
}

type PricingCatalogReadRow struct {
	SitePricingCatalog
	SiteName string
}
type PricingGroupReadRow struct {
	SitePricingGroup
	SiteName string
}
type PricingCatalogMetricRow struct {
	SiteID                                                   int64
	SiteName                                                 string
	Total, Missing, GroupTotal, GroupMissing                 int64
	PricingAsOf, PricingLastCompleteAt, PricingLastFailureAt *int64
	GroupAsOf, GroupLastCompleteAt, GroupLastFailureAt       *int64
}
type PricingGroupAssociationRow struct {
	SiteID      int64
	GroupName   string
	ModelName   string
	RemoteState string
}
type PricingCatalogRepository struct{ db *gorm.DB }

func NewPricingCatalogRepository(db *gorm.DB) *PricingCatalogRepository {
	return &PricingCatalogRepository{db: db}
}
func (r *PricingCatalogRepository) base(ctx context.Context, q dto.PricingCatalogQuery, table string) *gorm.DB {
	db := r.db.WithContext(ctx).Table(table + " p").Joins("JOIN site s ON s.id=p.site_id AND s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("p.site_id IN ?", q.SiteIDs)
	}
	if len(q.States) > 0 {
		db = db.Where("p.remote_state IN ?", q.States)
	}
	return db
}
func (r *PricingCatalogRepository) List(ctx context.Context, q dto.PricingCatalogQuery) ([]PricingCatalogReadRow, int64, error) {
	db := r.base(ctx, q, "site_pricing_catalog")
	if q.Keyword != "" {
		db = db.Where("p.model_name LIKE ?", "%"+escapeLike(strings.TrimSpace(q.Keyword))+"%")
	}
	if q.Group != "" {
		db = db.Where("JSON_CONTAINS(p.enable_groups_json, JSON_QUOTE(?))", q.Group)
	}
	if q.BillingMode != "" {
		if q.BillingMode == "token" {
			db = db.Where("p.pricing_source IN ('token_explicit','token_default')")
		} else {
			db = db.Where("p.pricing_source=?", q.BillingMode)
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []PricingCatalogReadRow
	err := db.Select("p.*,s.name site_name").Order("p.site_id,p.model_name").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *PricingCatalogRepository) ListGroups(ctx context.Context, q dto.PricingCatalogQuery) ([]PricingGroupReadRow, int64, error) {
	db := r.base(ctx, q, "site_group_catalog")
	if q.Keyword != "" {
		db = db.Where("p.group_name LIKE ?", "%"+escapeLike(strings.TrimSpace(q.Keyword))+"%")
	}
	if q.Group != "" {
		db = db.Where("p.group_name=?", q.Group)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []PricingGroupReadRow
	err := db.Select("p.*,s.name site_name").Order("p.site_id,p.group_name").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *PricingCatalogRepository) SiteMetrics(ctx context.Context, q dto.PricingCatalogQuery) ([]PricingCatalogMetricRow, error) {
	pricingJoin := "LEFT JOIN site_pricing_catalog p ON p.site_id=s.id"
	pricingArgs := make([]any, 0, 3)
	if len(q.States) > 0 {
		pricingJoin += " AND p.remote_state IN ?"
		pricingArgs = append(pricingArgs, q.States)
	}
	if q.Keyword != "" {
		pricingJoin += " AND p.model_name LIKE ?"
		pricingArgs = append(pricingArgs, "%"+escapeLike(strings.TrimSpace(q.Keyword))+"%")
	}
	if q.Group != "" {
		pricingJoin += " AND JSON_CONTAINS(p.enable_groups_json, JSON_QUOTE(?))"
		pricingArgs = append(pricingArgs, q.Group)
	}
	if q.BillingMode != "" {
		if q.BillingMode == "token" {
			pricingJoin += " AND p.pricing_source IN ('token_explicit','token_default')"
		} else {
			pricingJoin += " AND p.pricing_source=?"
			pricingArgs = append(pricingArgs, q.BillingMode)
		}
	}
	db := r.db.WithContext(ctx).Table("site s").Joins(pricingJoin, pricingArgs...).Joins("LEFT JOIN site_group_catalog g ON g.site_id=s.id").Joins("LEFT JOIN pricing_group_collection_state ps ON ps.site_id=s.id AND ps.resource_kind='pricing'").Joins("LEFT JOIN pricing_group_collection_state gs ON gs.site_id=s.id AND gs.resource_kind='group'").Where("s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("s.id IN ?", q.SiteIDs)
	}
	var rows []PricingCatalogMetricRow
	err := db.Select("s.id site_id,MAX(s.name) site_name,COUNT(DISTINCT p.id) total,COUNT(DISTINCT CASE WHEN p.remote_state='missing' THEN p.id END) missing,COUNT(DISTINCT g.id) group_total,COUNT(DISTINCT CASE WHEN g.remote_state='missing' THEN g.id END) group_missing,MAX(ps.as_of) pricing_as_of,MAX(ps.last_complete_at) pricing_last_complete_at,MAX(ps.last_failure_at) pricing_last_failure_at,MAX(gs.as_of) group_as_of,MAX(gs.last_complete_at) group_last_complete_at,MAX(gs.last_failure_at) group_last_failure_at").Group("s.id").Order("s.id").Scan(&rows).Error
	return rows, err
}
func (r *PricingCatalogRepository) GroupAssociations(ctx context.Context, groups []PricingGroupReadRow) ([]PricingGroupAssociationRow, error) {
	if len(groups) == 0 {
		return []PricingGroupAssociationRow{}, nil
	}
	conditions := make([]string, 0, len(groups))
	args := make([]any, 0, len(groups)*2)
	for _, group := range groups {
		conditions = append(conditions, "(p.site_id=? AND j.group_name=?)")
		args = append(args, group.SiteID, group.GroupName)
	}
	var rows []PricingGroupAssociationRow
	err := r.db.WithContext(ctx).Table("site_pricing_catalog p").
		Joins("JOIN JSON_TABLE(p.enable_groups_json, '$[*]' COLUMNS(group_name VARCHAR(128) PATH '$')) AS j ON TRUE").
		Where(strings.Join(conditions, " OR "), args...).
		Select("p.site_id,j.group_name,p.model_name,p.remote_state").
		Order("p.site_id,j.group_name,p.model_name").Scan(&rows).Error
	return rows, err
}
