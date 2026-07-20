package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/dto"
	"sort"
	"strings"
	"unicode/utf8"
)

type SiteSubscriptionPlan struct {
	ID, SiteID, RemoteID                                      int64
	Title, Subtitle, PriceAmount, Currency, DurationUnit      string
	DurationValue                                             int
	CustomSeconds                                             int64
	Enabled                                                   bool
	SortOrder                                                 int
	TotalAmount                                               int64
	QuotaResetPeriod                                          string
	QuotaResetCustomSeconds, RemoteCreatedAt, RemoteUpdatedAt int64
	SourceHash, RemoteState                                   string
	MissingCount, ConfigVersion                               int
	FirstSeenAt                                               int64
	LastSeenAt                                                *int64
	CollectedAt, CreatedAt, UpdatedAt                         int64
}

func (SiteSubscriptionPlan) TableName() string { return "site_subscription_plan" }

type SiteSubscriptionPlanCollectionState struct {
	SiteID                       int64
	LastSuccessAt, LastFailureAt *int64
	LastErrorCode                string
	ObservedCount                int64
	ConfigVersion                int
	UpdatedAt                    int64
}

func (SiteSubscriptionPlanCollectionState) TableName() string {
	return "site_subscription_plan_collection_state"
}
func (r *SiteRepository) SyncSubscriptionPlans(ctx context.Context, site Site, at int64, snapshot dto.UpstreamSubscriptionPlanSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || at <= 0 || len(snapshot.Items) > 100000 {
		return 0, errors.New("invalid subscription plan snapshot")
	}
	items := append([]dto.UpstreamSubscriptionPlan{}, snapshot.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for i, p := range items {
		if p.ID <= 0 || p.Title == "" || len(p.Title) > 128 || len(p.Subtitle) > 255 || p.DurationValue <= 0 || p.CustomSeconds < 0 || p.TotalAmount < 0 || p.QuotaResetCustomSeconds < 0 || p.CreatedAt < 0 || p.UpdatedAt < 0 || !utf8.ValidString(p.Title) || i > 0 && items[i-1].ID == p.ID {
			return 0, errors.New("invalid subscription plan")
		}
	}
	var existing []SiteSubscriptionPlan
	if err := r.db.WithContext(ctx).Where("site_id=?", site.ID).Find(&existing).Error; err != nil {
		return 0, err
	}
	previous := make(map[int64]SiteSubscriptionPlan, len(existing))
	for _, row := range existing {
		previous[row.RemoteID] = row
	}
	var written int64
	observedIDs := make(map[int64]struct{}, len(items))
	for _, item := range items {
		observedIDs[item.ID] = struct{}{}
	}
	missingIDs := make([]int64, 0)
	for remoteID, row := range previous {
		if _, observed := observedIDs[remoteID]; !observed && row.RemoteState != "missing" {
			missingIDs = append(missingIDs, remoteID)
		}
	}
	sort.Slice(missingIDs, func(i, j int) bool { return missingIDs[i] < missingIDs[j] })
	for start := 0; start < len(missingIDs); start += 500 {
		end := start + 500
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		result := r.db.WithContext(ctx).Model(&SiteSubscriptionPlan{}).
			Where("site_id = ? AND remote_id IN ? AND remote_state <> 'missing'", site.ID, missingIDs[start:end]).
			Updates(map[string]any{"remote_state": "missing", "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": at})
		if result.Error != nil {
			return written, result.Error
		}
		written += result.RowsAffected
	}
	upserts := make([]SiteSubscriptionPlan, 0, len(items))
	var upsertWritten int64
	for _, p := range items {
		raw, _ := json.Marshal(p)
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		seen := at
		upserts = append(upserts, SiteSubscriptionPlan{SiteID: site.ID, RemoteID: p.ID, Title: p.Title, Subtitle: p.Subtitle, PriceAmount: p.PriceAmount, Currency: p.Currency, DurationUnit: p.DurationUnit, DurationValue: p.DurationValue, CustomSeconds: p.CustomSeconds, Enabled: p.Enabled, SortOrder: p.SortOrder, TotalAmount: p.TotalAmount, QuotaResetPeriod: p.QuotaResetPeriod, QuotaResetCustomSeconds: p.QuotaResetCustomSeconds, RemoteCreatedAt: p.CreatedAt, RemoteUpdatedAt: p.UpdatedAt, SourceHash: hash, RemoteState: "normal", MissingCount: 0, ConfigVersion: site.ConfigVersion, FirstSeenAt: at, LastSeenAt: &seen, CollectedAt: at, CreatedAt: at, UpdatedAt: at})
		old, found := previous[p.ID]
		if !found || old.SourceHash != hash || old.RemoteState != "normal" {
			upsertWritten++
		}
	}
	if len(upserts) > 0 {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"title", "subtitle", "price_amount", "currency", "duration_unit", "duration_value", "custom_seconds", "enabled", "sort_order", "total_amount", "quota_reset_period", "quota_reset_custom_seconds", "remote_created_at", "remote_updated_at", "source_hash", "remote_state", "missing_count", "config_version", "last_seen_at", "collected_at", "updated_at"})}).CreateInBatches(&upserts, 500).Error; err != nil {
			return written, err
		}
		written += upsertWritten
	}
	state := SiteSubscriptionPlanCollectionState{SiteID: site.ID, LastSuccessAt: &at, ObservedCount: int64(len(items)), ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_success_at", "last_error_code", "observed_count", "config_version", "updated_at"})}).Create(&state).Error
	return written, err
}
func (r *SiteRepository) MarkSubscriptionPlanFailure(ctx context.Context, site Site, at int64, code string) error {
	current, err := r.FindByIDForUpdate(ctx, site.ID)
	if err != nil {
		return err
	}
	if current.ConfigVersion != site.ConfigVersion {
		return ErrSiteRunConfigChanged
	}
	row := SiteSubscriptionPlanCollectionState{SiteID: site.ID, LastFailureAt: &at, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: at}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
}

type SubscriptionPlanReadRow struct {
	SiteSubscriptionPlan
	SiteName string
}
type SubscriptionPlanMetricRow struct {
	SiteID                            int64
	SiteName                          string
	Total, Enabled, Disabled, Missing int64
	AsOf                              *int64
	LastSuccessAt, LastFailureAt      *int64
}
type SubscriptionPlanRepository struct{ db *gorm.DB }

func NewSubscriptionPlanRepository(db *gorm.DB) *SubscriptionPlanRepository {
	return &SubscriptionPlanRepository{db: db}
}
func (r *SubscriptionPlanRepository) base(ctx context.Context, q dto.SubscriptionPlanQuery) *gorm.DB {
	db := r.db.WithContext(ctx).Table("site_subscription_plan p").Joins("JOIN site s ON s.id=p.site_id AND s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("p.site_id IN ?", q.SiteIDs)
	}
	if len(q.States) > 0 {
		db = db.Where("p.remote_state IN ?", q.States)
	}
	if q.Enabled != nil {
		db = db.Where("p.enabled=?", *q.Enabled)
	}
	if q.Keyword != "" {
		db = db.Where("p.title LIKE ?", "%"+escapeLike(strings.TrimSpace(q.Keyword))+"%")
	}
	return db
}
func (r *SubscriptionPlanRepository) ActiveSiteStates(ctx context.Context, q dto.SubscriptionPlanQuery) ([]SubscriptionPlanMetricRow, error) {
	db := r.db.WithContext(ctx).Table("site s").Joins("LEFT JOIN site_subscription_plan_collection_state cs ON cs.site_id=s.id").Where("s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("s.id IN ?", q.SiteIDs)
	}
	var rows []SubscriptionPlanMetricRow
	err := db.Select("s.id site_id,s.name site_name,cs.last_success_at,cs.last_failure_at,cs.updated_at as_of").Order("s.id").Scan(&rows).Error
	return rows, err
}
func (r *SubscriptionPlanRepository) List(ctx context.Context, q dto.SubscriptionPlanQuery) ([]SubscriptionPlanReadRow, int64, error) {
	db := r.base(ctx, q)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SubscriptionPlanReadRow
	err := db.Select("p.*,s.name site_name").Order("p.site_id,p.sort_order DESC,p.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *SubscriptionPlanRepository) Metrics(ctx context.Context, q dto.SubscriptionPlanQuery, site bool) ([]SubscriptionPlanMetricRow, error) {
	db := r.base(ctx, q)
	selectSQL := "0 site_id,'' site_name"
	if site {
		selectSQL = "p.site_id,MAX(s.name) site_name"
	}
	var rows []SubscriptionPlanMetricRow
	query := db.Select(selectSQL + ",COUNT(*) total,SUM(p.remote_state='normal' AND p.enabled) enabled,SUM(p.remote_state='normal' AND NOT p.enabled) disabled,SUM(p.remote_state='missing') missing,MAX(p.updated_at) as_of")
	if site {
		query = query.Group("p.site_id")
	}
	err := query.Scan(&rows).Error
	return rows, err
}

func (r *SubscriptionPlanRepository) SiteMetrics(ctx context.Context, q dto.SubscriptionPlanQuery) ([]SubscriptionPlanMetricRow, error) {
	join := "LEFT JOIN site_subscription_plan p ON p.site_id=s.id"
	args := make([]any, 0, 3)
	if len(q.States) > 0 {
		join += " AND p.remote_state IN ?"
		args = append(args, q.States)
	}
	if q.Enabled != nil {
		join += " AND p.enabled=?"
		args = append(args, *q.Enabled)
	}
	if q.Keyword != "" {
		join += " AND p.title LIKE ?"
		args = append(args, "%"+escapeLike(strings.TrimSpace(q.Keyword))+"%")
	}
	db := r.db.WithContext(ctx).Table("site s").Joins(join, args...).Joins("LEFT JOIN site_subscription_plan_collection_state cs ON cs.site_id=s.id").Where("s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("s.id IN ?", q.SiteIDs)
	}
	var rows []SubscriptionPlanMetricRow
	err := db.Select("s.id site_id,MAX(s.name) site_name,COUNT(p.id) total,COALESCE(SUM(p.remote_state='normal' AND p.enabled),0) enabled,COALESCE(SUM(p.remote_state='normal' AND NOT p.enabled),0) disabled,COALESCE(SUM(p.remote_state='missing'),0) missing,MAX(p.updated_at) as_of,MAX(cs.last_success_at) last_success_at,MAX(cs.last_failure_at) last_failure_at").Group("s.id").Order("s.id").Scan(&rows).Error
	return rows, err
}
