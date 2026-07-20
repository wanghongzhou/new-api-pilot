package model

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/dto"
)

const financeStateNormal = "normal"
const financeStateMissing = "missing"

type SiteTopupOrder struct {
	ID, SiteID, RemoteID, RemoteUserID, Amount          int64
	Money, PaymentMethod, PaymentProvider, RemoteStatus string
	CreateTime, CompleteTime                            int64
	RemoteState                                         string
	MissingCount, ConfigVersion                         int
	FirstSeenAt                                         int64
	LastSeenAt                                          *int64
	CollectedAt, CreatedAt, UpdatedAt                   int64
}

func (SiteTopupOrder) TableName() string { return "site_topup_order" }

type SiteRedemption struct {
	ID, SiteID, RemoteID, RemoteUserID                        int64
	Name                                                      string
	RemoteStatus                                              int
	Quota, CreatedTime, RedeemedTime, UsedUserID, ExpiredTime int64
	RemoteState                                               string
	MissingCount, ConfigVersion                               int
	FirstSeenAt                                               int64
	LastSeenAt                                                *int64
	CollectedAt, CreatedAt, UpdatedAt                         int64
}

func (SiteRedemption) TableName() string { return "site_redemption" }

type SiteTopupCollectionState struct {
	SiteID                       int64
	LastSuccessAt, LastFailureAt *int64
	LastErrorCode                string
	ObservedTotal, ObservedMaxID int64
	ConfigVersion                int
	UpdatedAt                    int64
}

func (SiteTopupCollectionState) TableName() string { return "site_topup_collection_state" }

type SiteRedemptionCollectionState struct {
	SiteID                       int64
	LastSuccessAt, LastFailureAt *int64
	LastErrorCode                string
	ObservedTotal, ObservedMaxID int64
	ConfigVersion                int
	UpdatedAt                    int64
}

func (SiteRedemptionCollectionState) TableName() string { return "site_redemption_collection_state" }

func (r *SiteRepository) MarkFinanceCollectionFailure(ctx context.Context, site Site, observedAt int64, kind, code string) error {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || code == "" {
		return errors.New("invalid finance collection failure")
	}
	if kind != "topup" && kind != "redemption" {
		return errors.New("invalid finance collection kind")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, site.ID).Error; err != nil {
			return err
		}
		if current.ConfigVersion != site.ConfigVersion {
			return ErrSiteRunConfigChanged
		}
		if kind == "topup" {
			row := SiteTopupCollectionState{SiteID: site.ID, LastFailureAt: &observedAt, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
		}
		row := SiteRedemptionCollectionState{SiteID: site.ID, LastFailureAt: &observedAt, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&row).Error
	})
}

func (r *SiteRepository) SyncTopups(ctx context.Context, site Site, observedAt int64, snapshot dto.UpstreamTopupSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || snapshot.Total != int64(len(snapshot.Items)) || snapshot.Total > 100000 {
		return 0, errors.New("invalid topup snapshot")
	}
	items := append([]dto.UpstreamTopup{}, snapshot.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for i, item := range items {
		if item.ID <= 0 || item.UserID < 0 || item.Amount < 0 || item.CreateTime < 0 || item.CompleteTime < 0 || !validFinanceText(item.PaymentMethod, 50) || !validFinanceText(item.PaymentProvider, 50) || !validFinanceText(item.Status, 32) || !validDecimalString(item.Money) || i > 0 && items[i-1].ID == item.ID {
			return 0, errors.New("invalid topup observation")
		}
	}
	if snapshot.Total == 0 && snapshot.MaxID != 0 || snapshot.Total > 0 && snapshot.MaxID != items[len(items)-1].ID {
		return 0, errors.New("invalid topup snapshot fence")
	}
	if err := r.db.WithContext(ctx).Model(&SiteTopupOrder{}).Where("site_id=?", site.ID).Updates(map[string]any{"remote_state": financeStateMissing, "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": observedAt}).Error; err != nil {
		return 0, err
	}
	for _, item := range items {
		seen := observedAt
		row := SiteTopupOrder{SiteID: site.ID, RemoteID: item.ID, RemoteUserID: item.UserID, Amount: item.Amount, Money: item.Money, PaymentMethod: item.PaymentMethod, PaymentProvider: item.PaymentProvider, CreateTime: item.CreateTime, CompleteTime: item.CompleteTime, RemoteStatus: item.Status, RemoteState: financeStateNormal, ConfigVersion: site.ConfigVersion, FirstSeenAt: observedAt, LastSeenAt: &seen, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"remote_user_id", "amount", "money", "payment_method", "payment_provider", "create_time", "complete_time", "remote_status", "remote_state", "config_version", "last_seen_at", "collected_at", "updated_at"})}).Create(&row).Error; err != nil {
			return 0, err
		}
		if err := r.db.WithContext(ctx).Model(&SiteTopupOrder{}).Where("site_id=? AND remote_id=?", site.ID, item.ID).Update("missing_count", 0).Error; err != nil {
			return 0, err
		}
	}
	state := SiteTopupCollectionState{SiteID: site.ID, LastSuccessAt: &observedAt, ObservedTotal: snapshot.Total, ObservedMaxID: snapshot.MaxID, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_success_at", "last_error_code", "observed_total", "observed_max_id", "config_version", "updated_at"})}).Create(&state).Error; err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func (r *SiteRepository) SyncRedemptions(ctx context.Context, site Site, observedAt int64, snapshot dto.UpstreamRedemptionSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || snapshot.Total != int64(len(snapshot.Items)) || snapshot.Total > 100000 {
		return 0, errors.New("invalid redemption snapshot")
	}
	items := append([]dto.UpstreamRedemption{}, snapshot.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for i, item := range items {
		if item.ID <= 0 || item.UserID < 0 || item.Status < 0 || item.Quota < 0 || item.CreatedTime < 0 || item.RedeemedTime < 0 || item.UsedUserID < 0 || item.ExpiredTime < 0 || !validFinanceText(item.Name, 255) || i > 0 && items[i-1].ID == item.ID {
			return 0, errors.New("invalid redemption observation")
		}
	}
	if snapshot.Total == 0 && snapshot.MaxID != 0 || snapshot.Total > 0 && snapshot.MaxID != items[len(items)-1].ID {
		return 0, errors.New("invalid redemption snapshot fence")
	}
	if err := r.db.WithContext(ctx).Model(&SiteRedemption{}).Where("site_id=?", site.ID).Updates(map[string]any{"remote_state": financeStateMissing, "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": observedAt}).Error; err != nil {
		return 0, err
	}
	for _, item := range items {
		seen := observedAt
		row := SiteRedemption{SiteID: site.ID, RemoteID: item.ID, RemoteUserID: item.UserID, Name: item.Name, RemoteStatus: item.Status, Quota: item.Quota, CreatedTime: item.CreatedTime, RedeemedTime: item.RedeemedTime, UsedUserID: item.UsedUserID, ExpiredTime: item.ExpiredTime, RemoteState: financeStateNormal, ConfigVersion: site.ConfigVersion, FirstSeenAt: observedAt, LastSeenAt: &seen, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"remote_user_id", "name", "remote_status", "quota", "created_time", "redeemed_time", "used_user_id", "expired_time", "remote_state", "config_version", "last_seen_at", "collected_at", "updated_at"})}).Create(&row).Error; err != nil {
			return 0, err
		}
		if err := r.db.WithContext(ctx).Model(&SiteRedemption{}).Where("site_id=? AND remote_id=?", site.ID, item.ID).Update("missing_count", 0).Error; err != nil {
			return 0, err
		}
	}
	state := SiteRedemptionCollectionState{SiteID: site.ID, LastSuccessAt: &observedAt, ObservedTotal: snapshot.Total, ObservedMaxID: snapshot.MaxID, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_success_at", "last_error_code", "observed_total", "observed_max_id", "config_version", "updated_at"})}).Create(&state).Error; err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func validFinanceText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max
}
func validDecimalString(value string) bool {
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 28 || len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 10) {
		return false
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	r, ok := new(big.Rat).SetString(value)
	return ok && r.Sign() >= 0
}

type TopupReadRow struct {
	SiteTopupOrder
	SiteName string
}
type RedemptionReadRow struct {
	SiteRedemption
	SiteName string
}
type FinanceMetricRow struct {
	DimensionID, DimensionName string
	SiteID                     int64
	SiteName                   string
	Count, MissingCount        int64
	Amount                     int64
	Money                      string
	Quota                      int64
	AsOf                       *int64
}
type FinanceRepository struct{ db *gorm.DB }

func NewFinanceRepository(db *gorm.DB) *FinanceRepository { return &FinanceRepository{db: db} }

func applyFinanceListFilters(db *gorm.DB, q dto.FinanceInventoryQuery, alias, timeColumn string) *gorm.DB {
	if len(q.SiteIDs) > 0 {
		db = db.Where(alias+".site_id IN ?", q.SiteIDs)
	}
	if q.RemoteID != nil {
		db = db.Where(alias+".remote_id=?", *q.RemoteID)
	}
	if q.RemoteUserID != nil {
		db = db.Where(alias+".remote_user_id=?", *q.RemoteUserID)
	}
	if len(q.Statuses) > 0 {
		db = db.Where(alias+".remote_status IN ?", q.Statuses)
	}
	if len(q.States) > 0 {
		db = db.Where(alias+".remote_state IN ?", q.States)
	}
	if q.StartTimestamp > 0 {
		db = db.Where(alias+"."+timeColumn+">=?", q.StartTimestamp)
	}
	if q.EndTimestamp > 0 {
		db = db.Where(alias+"."+timeColumn+"<?", q.EndTimestamp)
	}
	return db
}
func (r *FinanceRepository) ListTopups(ctx context.Context, q dto.FinanceInventoryQuery) ([]TopupReadRow, int64, error) {
	db := applyFinanceListFilters(r.db.WithContext(ctx).Table("site_topup_order t").Joins("JOIN site s ON s.id=t.site_id"), q, "t", "create_time")
	if len(q.Providers) > 0 {
		db = db.Where("t.payment_provider IN ?", q.Providers)
	}
	if len(q.Methods) > 0 {
		db = db.Where("t.payment_method IN ?", q.Methods)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TopupReadRow
	err := db.Select("t.*,s.name site_name").Order("t.site_id,t.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *FinanceRepository) ListRedemptions(ctx context.Context, q dto.FinanceInventoryQuery) ([]RedemptionReadRow, int64, error) {
	db := applyFinanceListFilters(r.db.WithContext(ctx).Table("site_redemption r").Joins("JOIN site s ON s.id=r.site_id"), q, "r", "created_time")
	if q.Keyword != "" {
		db = db.Where("r.name LIKE ? ESCAPE '\\\\'", "%"+escapeLike(q.Keyword)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []RedemptionReadRow
	err := db.Select("r.*,s.name site_name").Order("r.site_id,r.remote_id DESC").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}

func (r *FinanceRepository) TopupMetrics(ctx context.Context, q dto.FinanceInventoryQuery, dim string) ([]FinanceMetricRow, error) {
	expr := map[string]string{"summary": "'summary'", "status": "t.remote_status", "provider": "t.payment_provider", "site": "CAST(t.site_id AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid topup dimension")
	}
	db := applyFinanceListFilters(r.db.WithContext(ctx).Table("site_topup_order t").Joins("JOIN site s ON s.id=t.site_id"), q, "t", "create_time")
	if len(q.Providers) > 0 {
		db = db.Where("t.payment_provider IN ?", q.Providers)
	}
	if len(q.Methods) > 0 {
		db = db.Where("t.payment_method IN ?", q.Methods)
	}
	siteID, siteName := "0", "''"
	group := expr
	if dim == "provider" {
		siteID = "t.site_id"
		siteName = "MAX(s.name)"
		group = "t.site_id,t.payment_provider"
	} else if dim == "site" {
		siteID = "t.site_id"
		siteName = "MAX(s.name)"
		group = "t.site_id"
	}
	totals := "0 amount,'0' money"
	if dim == "provider" || dim == "site" {
		totals = "COALESCE(SUM(CASE WHEN t.remote_state='normal' THEN t.amount ELSE 0 END),0) amount,COALESCE(SUM(CASE WHEN t.remote_state='normal' THEN t.money ELSE 0 END),0) money"
	}
	db = db.Select(expr + " dimension_id," + expr + " dimension_name," + siteID + " site_id," + siteName + " site_name,SUM(t.remote_state='normal') count,SUM(t.remote_state='missing') missing_count," + totals + ",0 quota,MAX(t.updated_at) as_of")
	if dim != "summary" {
		db = db.Group(group)
	}
	var rows []FinanceMetricRow
	err := db.Order("site_id,dimension_id").Scan(&rows).Error
	return rows, err
}
func (r *FinanceRepository) RedemptionMetrics(ctx context.Context, q dto.FinanceInventoryQuery, dim string, now int64) ([]FinanceMetricRow, error) {
	statusExpression := fmt.Sprintf("CASE WHEN r.remote_status=1 AND r.expired_time<>0 AND r.expired_time<%d THEN 'expired' ELSE CAST(r.remote_status AS CHAR) END", now)
	expr := map[string]string{"summary": "'summary'", "status": statusExpression, "site": "CAST(r.site_id AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid redemption dimension")
	}
	db := applyFinanceListFilters(r.db.WithContext(ctx).Table("site_redemption r").Joins("JOIN site s ON s.id=r.site_id"), q, "r", "created_time")
	siteID, siteName := "0", "''"
	group := expr
	if dim == "site" {
		siteID = "r.site_id"
		siteName = "MAX(s.name)"
		group = "r.site_id"
	}
	sql := expr + " dimension_id," + expr + " dimension_name," + siteID + " site_id," + siteName + " site_name,SUM(r.remote_state='normal') count,SUM(r.remote_state='missing') missing_count,0 amount,'0' money,COALESCE(SUM(CASE WHEN r.remote_state='normal' THEN r.quota ELSE 0 END),0) quota,MAX(r.updated_at) as_of"
	db = db.Select(sql)
	if dim != "summary" {
		db = db.Group(group)
	}
	var rows []FinanceMetricRow
	err := db.Order("site_id,dimension_id").Scan(&rows).Error
	return rows, err
}
