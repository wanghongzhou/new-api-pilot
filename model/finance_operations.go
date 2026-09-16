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

	"new-api-pilot/constant"
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
	SiteID                                          int64
	LastSuccessAt, LastFullSuccessAt, LastFailureAt *int64
	LastErrorCode                                   string
	ObservedTotal, ObservedMaxID                    int64
	ConfigVersion                                   int
	UpdatedAt                                       int64
}

func (SiteTopupCollectionState) TableName() string { return "site_topup_collection_state" }

type SiteRedemptionCollectionState struct {
	SiteID                                          int64
	LastSuccessAt, LastFullSuccessAt, LastFailureAt *int64
	LastErrorCode                                   string
	ObservedTotal, ObservedMaxID                    int64
	ConfigVersion                                   int
	UpdatedAt                                       int64
}

func (SiteRedemptionCollectionState) TableName() string { return "site_redemption_collection_state" }

type FinanceCollectionCheckpoint struct {
	ObservedTotal, ObservedMaxID int64
	LastFullSuccessAt            *int64
	ConfigVersion                int
}

func (r *SiteRepository) TopupCollectionCheckpoint(ctx context.Context, siteID int64) (FinanceCollectionCheckpoint, error) {
	if r == nil || r.db == nil || siteID <= 0 {
		return FinanceCollectionCheckpoint{}, errors.New("invalid topup collection checkpoint")
	}
	var state SiteTopupCollectionState
	err := r.db.WithContext(ctx).Where("site_id=?", siteID).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return FinanceCollectionCheckpoint{}, nil
	}
	if err != nil {
		return FinanceCollectionCheckpoint{}, err
	}
	return FinanceCollectionCheckpoint{ObservedTotal: state.ObservedTotal, ObservedMaxID: state.ObservedMaxID, LastFullSuccessAt: state.LastFullSuccessAt, ConfigVersion: state.ConfigVersion}, nil
}

func (r *SiteRepository) RedemptionCollectionCheckpoint(ctx context.Context, siteID int64) (FinanceCollectionCheckpoint, error) {
	if r == nil || r.db == nil || siteID <= 0 {
		return FinanceCollectionCheckpoint{}, errors.New("invalid redemption collection checkpoint")
	}
	var state SiteRedemptionCollectionState
	err := r.db.WithContext(ctx).Where("site_id=?", siteID).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return FinanceCollectionCheckpoint{}, nil
	}
	if err != nil {
		return FinanceCollectionCheckpoint{}, err
	}
	return FinanceCollectionCheckpoint{ObservedTotal: state.ObservedTotal, ObservedMaxID: state.ObservedMaxID, LastFullSuccessAt: state.LastFullSuccessAt, ConfigVersion: state.ConfigVersion}, nil
}

func (r *SiteRepository) OldestPendingTopupID(ctx context.Context, siteID int64) (int64, error) {
	if r == nil || r.db == nil || siteID <= 0 {
		return 0, errors.New("invalid pending topup lookup")
	}
	var id *int64
	err := r.db.WithContext(ctx).Model(&SiteTopupOrder{}).
		Select("MIN(remote_id)").Where("site_id=? AND remote_state=? AND remote_status=?", siteID, financeStateNormal, "pending").Scan(&id).Error
	if err != nil || id == nil {
		return 0, err
	}
	return *id, nil
}

func (r *SiteRepository) EnabledRedemptionIDs(ctx context.Context, siteID int64) ([]int64, error) {
	if r == nil || r.db == nil || siteID <= 0 {
		return nil, errors.New("invalid enabled redemption lookup")
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&SiteRedemption{}).
		Where("site_id=? AND remote_state=? AND remote_status=?", siteID, financeStateNormal, 1).
		Order("remote_id DESC").Limit(100001).Pluck("remote_id", &ids).Error
	if err != nil {
		return nil, err
	}
	if len(ids) > 100000 {
		return nil, errors.New("enabled redemption set is too large")
	}
	return ids, nil
}

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
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || snapshot.Total > 100000 ||
		(!snapshot.Incremental && snapshot.Total != int64(len(snapshot.Items))) ||
		(snapshot.Incremental && (snapshot.Total <= 0 || len(snapshot.Items) == 0 || int64(len(snapshot.Items)) > snapshot.Total)) {
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
	// A snapshot is authoritative, but avoid rewriting every row and then issuing a
	// second UPDATE for each item. Mark only the rows absent from this snapshot in
	// one statement; present rows are refreshed in bounded batches below.
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var missingRows int64
	if !snapshot.Incremental {
		missing := r.db.WithContext(ctx).Model(&SiteTopupOrder{}).Where("site_id=?", site.ID)
		if len(ids) > 0 {
			missing = missing.Where("remote_id NOT IN ?", ids)
		}
		missingResult := missing.Updates(map[string]any{"remote_state": financeStateMissing, "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": observedAt})
		if missingResult.Error != nil {
			return 0, missingResult.Error
		}
		missingRows = missingResult.RowsAffected
	}
	var existing []SiteTopupOrder
	existingQuery := r.db.WithContext(ctx).Where("site_id=?", site.ID)
	if snapshot.Incremental {
		existingQuery = existingQuery.Where("remote_id IN ?", ids)
	}
	if err := existingQuery.Find(&existing).Error; err != nil {
		return 0, err
	}
	byID := make(map[int64]SiteTopupOrder, len(existing))
	for _, row := range existing {
		byID[row.RemoteID] = row
	}
	changed := make([]SiteTopupOrder, 0, len(items))
	for _, item := range items {
		seen := observedAt
		row := SiteTopupOrder{SiteID: site.ID, RemoteID: item.ID, RemoteUserID: item.UserID, Amount: item.Amount, Money: financeMoneyCanonical(item.Money), PaymentMethod: item.PaymentMethod, PaymentProvider: item.PaymentProvider, CreateTime: item.CreateTime, CompleteTime: item.CompleteTime, RemoteStatus: item.Status, RemoteState: financeStateNormal, ConfigVersion: site.ConfigVersion, FirstSeenAt: observedAt, LastSeenAt: &seen, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
		old, ok := byID[item.ID]
		if !ok || old.RemoteUserID != row.RemoteUserID || old.Amount != row.Amount || old.Money != row.Money || old.PaymentMethod != row.PaymentMethod || old.PaymentProvider != row.PaymentProvider || old.CreateTime != row.CreateTime || old.CompleteTime != row.CompleteTime || old.RemoteStatus != row.RemoteStatus || old.RemoteState != financeStateNormal || old.MissingCount != 0 || old.ConfigVersion != site.ConfigVersion {
			changed = append(changed, row)
		}
	}
	if len(changed) > 0 {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"remote_user_id", "amount", "money", "payment_method", "payment_provider", "create_time", "complete_time", "remote_status", "remote_state", "missing_count", "last_seen_at", "collected_at", "config_version", "updated_at"})}).CreateInBatches(&changed, 500).Error; err != nil {
			return 0, err
		}
	}
	state := SiteTopupCollectionState{SiteID: site.ID, LastSuccessAt: &observedAt, ObservedTotal: snapshot.Total, ObservedMaxID: snapshot.MaxID, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
	stateColumns := []string{"last_success_at", "last_error_code", "observed_total", "observed_max_id", "config_version", "updated_at"}
	if !snapshot.Incremental {
		state.LastFullSuccessAt = &observedAt
		stateColumns = append(stateColumns, "last_full_success_at")
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns(stateColumns)}).Create(&state).Error; err != nil {
		return 0, err
	}
	return missingRows + int64(len(changed)), nil
}

func (r *SiteRepository) SyncRedemptions(ctx context.Context, site Site, observedAt int64, snapshot dto.UpstreamRedemptionSnapshot) (int64, error) {
	if r == nil || r.db == nil || site.ID <= 0 || observedAt <= 0 || snapshot.Total > 100000 ||
		(!snapshot.Incremental && snapshot.Total != int64(len(snapshot.Items))) ||
		(snapshot.Incremental && (snapshot.Total <= 0 || len(snapshot.Items) == 0 || int64(len(snapshot.Items)) > snapshot.Total)) {
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
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var missingRows int64
	if !snapshot.Incremental {
		missing := r.db.WithContext(ctx).Model(&SiteRedemption{}).Where("site_id=?", site.ID)
		if len(ids) > 0 {
			missing = missing.Where("remote_id NOT IN ?", ids)
		}
		missingResult := missing.Updates(map[string]any{"remote_state": financeStateMissing, "missing_count": gorm.Expr("missing_count+1"), "last_seen_at": nil, "updated_at": observedAt})
		if missingResult.Error != nil {
			return 0, missingResult.Error
		}
		missingRows = missingResult.RowsAffected
	}
	var existing []SiteRedemption
	existingQuery := r.db.WithContext(ctx).Where("site_id=?", site.ID)
	if snapshot.Incremental {
		existingQuery = existingQuery.Where("remote_id IN ?", ids)
	}
	if err := existingQuery.Find(&existing).Error; err != nil {
		return 0, err
	}
	byID := make(map[int64]SiteRedemption, len(existing))
	for _, row := range existing {
		byID[row.RemoteID] = row
	}
	changed := make([]SiteRedemption, 0, len(items))
	for _, item := range items {
		seen := observedAt
		row := SiteRedemption{SiteID: site.ID, RemoteID: item.ID, RemoteUserID: item.UserID, Name: item.Name, RemoteStatus: item.Status, Quota: item.Quota, CreatedTime: item.CreatedTime, RedeemedTime: item.RedeemedTime, UsedUserID: item.UsedUserID, ExpiredTime: item.ExpiredTime, RemoteState: financeStateNormal, ConfigVersion: site.ConfigVersion, FirstSeenAt: observedAt, LastSeenAt: &seen, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
		old, ok := byID[item.ID]
		if !ok || old.RemoteUserID != row.RemoteUserID || old.Name != row.Name || old.RemoteStatus != row.RemoteStatus || old.Quota != row.Quota || old.CreatedTime != row.CreatedTime || old.RedeemedTime != row.RedeemedTime || old.UsedUserID != row.UsedUserID || old.ExpiredTime != row.ExpiredTime || old.RemoteState != financeStateNormal || old.MissingCount != 0 || old.ConfigVersion != site.ConfigVersion {
			changed = append(changed, row)
		}
	}
	if len(changed) > 0 {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_id"}}, DoUpdates: clause.AssignmentColumns([]string{"remote_user_id", "name", "remote_status", "quota", "created_time", "redeemed_time", "used_user_id", "expired_time", "remote_state", "missing_count", "last_seen_at", "collected_at", "config_version", "updated_at"})}).CreateInBatches(&changed, 500).Error; err != nil {
			return 0, err
		}
	}
	state := SiteRedemptionCollectionState{SiteID: site.ID, LastSuccessAt: &observedAt, ObservedTotal: snapshot.Total, ObservedMaxID: snapshot.MaxID, ConfigVersion: site.ConfigVersion, UpdatedAt: observedAt}
	stateColumns := []string{"last_success_at", "last_error_code", "observed_total", "observed_max_id", "config_version", "updated_at"}
	if !snapshot.Incremental {
		state.LastFullSuccessAt = &observedAt
		stateColumns = append(stateColumns, "last_full_success_at")
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns(stateColumns)}).Create(&state).Error; err != nil {
		return 0, err
	}
	return missingRows + int64(len(changed)), nil
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

// financeMoneyCanonical matches DECIMAL(38,10) storage so an unchanged
// upstream value such as "0.1" does not look changed after a round trip.
func financeMoneyCanonical(value string) string {
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return value
	}
	return r.FloatString(10)
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
type FinanceCollectionCoverageRow struct {
	SiteID            int64  `gorm:"column:site_id"`
	SiteName          string `gorm:"column:site_name"`
	LastSuccessAt     *int64 `gorm:"column:last_success_at"`
	LastFullSuccessAt *int64 `gorm:"column:last_full_success_at"`
	LastFailureAt     *int64 `gorm:"column:last_failure_at"`
	AsOf              *int64 `gorm:"column:as_of"`
}
type FinanceRepository struct{ db *gorm.DB }

func NewFinanceRepository(db *gorm.DB) *FinanceRepository { return &FinanceRepository{db: db} }

func applyFinanceListFilters(db *gorm.DB, q dto.FinanceInventoryQuery, alias, timeColumn string) *gorm.DB {
	db = db.Where(alias+".config_version=s.config_version").
		Where("s.management_status=? AND s.auth_status=?", constant.SiteManagementActive, constant.SiteAuthAuthorized)
	if len(q.SiteIDs) > 0 {
		db = db.Where(alias+".site_id IN ?", q.SiteIDs)
	}
	if q.RemoteID != nil {
		db = db.Where(alias+".remote_id=?", *q.RemoteID)
	}
	if q.RemoteUserID != nil {
		db = db.Where(alias+".remote_user_id=?", *q.RemoteUserID)
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
	if q.SnapshotAt > 0 {
		db = db.Where(alias+".collected_at<=?", q.SnapshotAt)
	}
	return db
}
func (r *FinanceRepository) ListTopups(ctx context.Context, q dto.FinanceInventoryQuery) ([]TopupReadRow, int64, error) {
	db := applyFinanceListFilters(r.db.WithContext(ctx).Table("site_topup_order t").Joins("JOIN site s ON s.id=t.site_id"), q, "t", "create_time")
	if len(q.Statuses) > 0 {
		db = db.Where("t.remote_status IN ?", q.Statuses)
	}
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
	db = applyRedemptionStatusFilter(db, q, "r")
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
	if len(q.Statuses) > 0 {
		db = db.Where("t.remote_status IN ?", q.Statuses)
	}
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
	db = applyRedemptionStatusFilter(db, q, "r")
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

func applyRedemptionStatusFilter(db *gorm.DB, q dto.FinanceInventoryQuery, alias string) *gorm.DB {
	if len(q.Statuses) == 0 {
		return db
	}
	now := q.StatusAt
	clauses := make([]string, 0, len(q.Statuses))
	args := make([]any, 0, len(q.Statuses)+1)
	for _, status := range q.Statuses {
		if status == "expired" {
			clauses = append(clauses, "("+alias+".remote_status=1 AND "+alias+".expired_time<>0 AND "+alias+".expired_time<?)")
			args = append(args, now)
			continue
		}
		clauses = append(clauses, alias+".remote_status=?")
		args = append(args, status)
	}
	return db.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

func (r *FinanceRepository) CollectionCoverage(ctx context.Context, siteIDs []int64, kind string) ([]FinanceCollectionCoverageRow, error) {
	stateTable := map[string]string{"topup": "site_topup_collection_state", "redemption": "site_redemption_collection_state"}[kind]
	if r == nil || r.db == nil || stateTable == "" {
		return nil, ErrStatisticsReadContract
	}
	query := r.db.WithContext(ctx).Table("site s").
		Joins("LEFT JOIN "+stateTable+" c ON c.site_id=s.id AND c.config_version=s.config_version").
		Where("s.management_status=? AND s.auth_status=?", constant.SiteManagementActive, constant.SiteAuthAuthorized)
	if len(siteIDs) > 0 {
		query = query.Where("s.id IN ?", siteIDs)
	}
	var rows []FinanceCollectionCoverageRow
	err := query.Select("s.id site_id,s.name site_name,c.last_success_at,c.last_full_success_at,c.last_failure_at,c.last_full_success_at as_of").Order("s.id").Scan(&rows).Error
	return rows, err
}
