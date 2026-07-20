package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"new-api-pilot/constant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AccountRemoteStateNormal           = "normal"
	AccountRemoteStateMissing          = "missing"
	AccountRemoteStateIdentityMismatch = "identity_mismatch"
	AccountManagedStatusActive         = "active"
	AccountManagedStatusArchived       = "archived"
)

var (
	ErrAccountIdentityMismatch      = errors.New("account remote identity does not match its fixed binding")
	ErrAccountObservationInvalid    = errors.New("invalid account remote observation")
	ErrAccountObservationCAS        = errors.New("account remote observation compare-and-swap failed")
	ErrAccountRestoreContract       = errors.New("invalid account restore transition")
	ErrAccountOperationScopeChanged = errors.New("account operation scope changed while acquiring locks")
	ErrRebuildRunNotReady           = errors.New("latest rebuild run is not successful and authoritative")
	ErrAccountListContract          = errors.New("invalid account list contract")
)

type Account struct {
	ID                       int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID                   int64  `gorm:"column:site_id"`
	CustomerID               int64  `gorm:"column:customer_id"`
	RemoteUserID             int64  `gorm:"column:remote_user_id"`
	RemoteCreatedAt          int64  `gorm:"column:remote_created_at"`
	Username                 string `gorm:"column:username"`
	DisplayName              string `gorm:"column:display_name"`
	RemoteGroup              string `gorm:"column:remote_group"`
	RemoteStatus             int    `gorm:"column:remote_status"`
	RemoteState              string `gorm:"column:remote_state"`
	RemoteMissingCount       int    `gorm:"column:remote_missing_count"`
	LastRemoteSeenAt         *int64 `gorm:"column:last_remote_seen_at"`
	Quota                    int64  `gorm:"column:quota"`
	UsedQuota                int64  `gorm:"column:used_quota"`
	RequestCount             int64  `gorm:"column:request_count"`
	ManagedStatus            string `gorm:"column:managed_status"`
	StatisticsPausedAt       *int64 `gorm:"column:statistics_paused_at"`
	StatisticsBackfillStatus string `gorm:"column:statistics_backfill_status"`
	LastSyncedAt             *int64 `gorm:"column:last_synced_at"`
	Remark                   string `gorm:"column:remark"`
	CreatedAt                int64  `gorm:"column:created_at"`
	UpdatedAt                int64  `gorm:"column:updated_at"`
}

func (Account) TableName() string { return "account" }

type AccountFilter struct {
	Keyword         string
	SiteID          *int64
	CustomerID      *int64
	RemoteStatus    *int
	RemoteStatuses  []int
	RemoteState     string
	RemoteStates    []string
	ManagedStatus   string
	ManagedStatuses []string
	SortBy          string
	SortOrder       string
	TodayDateKey    int
	Offset          int
	Limit           int
}

type AccountListMetadata struct {
	AccountID       int64   `gorm:"column:account_id"`
	SiteName        string  `gorm:"column:site_name"`
	CustomerName    string  `gorm:"column:customer_name"`
	QuotaPerUnit    *string `gorm:"column:quota_per_unit"`
	USDExchangeRate *string `gorm:"column:usd_exchange_rate"`
	LastRateAt      *int64  `gorm:"column:last_rate_at"`

	LatestRunID               *int64  `gorm:"column:latest_run_id"`
	LatestRunStatus           *string `gorm:"column:latest_run_status"`
	LatestRunStartTimestamp   *int64  `gorm:"column:latest_run_start_timestamp"`
	LatestRunEndTimestamp     *int64  `gorm:"column:latest_run_end_timestamp"`
	LatestRunTotalWindows     *int    `gorm:"column:latest_run_total_windows"`
	LatestRunCompletedWindows *int    `gorm:"column:latest_run_completed_windows"`
	LatestRunFailedWindows    *int    `gorm:"column:latest_run_failed_windows"`
}

type AccountDeletionDependencies struct {
	HourlyStats  int64
	DailyStats   int64
	ActiveRuns   int64
	ActiveAlerts int64
	AlertHistory int64
}

func (dependencies AccountDeletionDependencies) HasAny() bool {
	return dependencies.HourlyStats > 0 || dependencies.DailyStats > 0 || dependencies.ActiveRuns > 0 ||
		dependencies.ActiveAlerts > 0 || dependencies.AlertHistory > 0
}

func (dependencies AccountDeletionDependencies) counts() map[string]int64 {
	return map[string]int64{
		"account_stat_hourly": dependencies.HourlyStats,
		"account_stat_daily":  dependencies.DailyStats,
		"active_collection":   dependencies.ActiveRuns,
		"active_alert":        dependencies.ActiveAlerts,
		"alert_history":       dependencies.AlertHistory,
	}
}

type ManagedAccountBinding struct {
	RemoteUserID  int64  `gorm:"column:remote_user_id"`
	AccountID     int64  `gorm:"column:account_id"`
	CustomerID    int64  `gorm:"column:customer_id"`
	CustomerName  string `gorm:"column:customer_name"`
	ManagedStatus string `gorm:"column:managed_status"`
}

type AuthoritativeAccountRemoteSnapshot struct {
	RemoteCreatedAt int64
	Username        string
	DisplayName     string
	RemoteGroup     string
	RemoteStatus    int
	Quota           int64
	UsedQuota       int64
	RequestCount    int64
	ObservedAt      int64
	UpdatedAt       int64
}

type AccountRepository struct {
	db *gorm.DB
}

type AccountOperationScope struct {
	Site     Site
	Customer Customer
	Account  Account
}

type accountOperationReference struct {
	ID         int64 `gorm:"column:id"`
	SiteID     int64 `gorm:"column:site_id"`
	CustomerID int64 `gorm:"column:customer_id"`
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (repository *AccountRepository) WithTransaction(ctx context.Context, operation func(*AccountRepository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(&AccountRepository{db: tx})
	})
}

func (repository *AccountRepository) Create(ctx context.Context, account *Account) error {
	return repository.db.WithContext(ctx).Create(account).Error
}

func (repository *AccountRepository) FindByID(ctx context.Context, id int64) (Account, error) {
	var account Account
	err := repository.db.WithContext(ctx).First(&account, id).Error
	return account, err
}

func (repository *AccountRepository) FindByIDForUpdate(ctx context.Context, id int64) (Account, error) {
	var account Account
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error
	return account, err
}

func (repository *AccountRepository) FindBySiteAndRemoteUser(ctx context.Context, siteID, remoteUserID int64) (Account, error) {
	var account Account
	err := repository.db.WithContext(ctx).Where("site_id = ? AND remote_user_id = ?", siteID, remoteUserID).First(&account).Error
	return account, err
}

func (repository *AccountRepository) FindBySiteAndRemoteUserForUpdate(ctx context.Context, siteID, remoteUserID int64) (Account, error) {
	var account Account
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("site_id = ? AND remote_user_id = ?", siteID, remoteUserID).First(&account).Error
	return account, err
}

func (repository *AccountRepository) List(ctx context.Context, filter AccountFilter) ([]Account, int64, error) {
	query := repository.db.WithContext(ctx).Model(&Account{})
	if filter.Keyword != "" {
		keyword := "%" + escapeLike(filter.Keyword) + "%"
		query = query.Where("(account.username LIKE ? ESCAPE '\\\\' OR account.display_name LIKE ? ESCAPE '\\\\' OR CAST(account.remote_user_id AS CHAR) LIKE ? ESCAPE '\\\\')", keyword, keyword, keyword)
	}
	if filter.SiteID != nil {
		query = query.Where("account.site_id = ?", *filter.SiteID)
	}
	if filter.CustomerID != nil {
		query = query.Where("account.customer_id = ?", *filter.CustomerID)
	}
	if filter.RemoteStatus != nil {
		query = query.Where("account.remote_status = ?", *filter.RemoteStatus)
	}
	if len(filter.RemoteStatuses) > 0 {
		query = query.Where("account.remote_status IN ?", filter.RemoteStatuses)
	}
	if filter.RemoteState != "" {
		query = query.Where("account.remote_state = ?", filter.RemoteState)
	}
	if len(filter.RemoteStates) > 0 {
		query = query.Where("account.remote_state IN ?", filter.RemoteStates)
	}
	if filter.ManagedStatus != "" {
		query = query.Where("account.managed_status = ?", filter.ManagedStatus)
	}
	if len(filter.ManagedStatuses) > 0 {
		query = query.Where("account.managed_status IN ?", filter.ManagedStatuses)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortExpressions := map[string]string{
		"updated_at":  "account.updated_at",
		"username":    "account.username",
		"quota":       "account.quota",
		"today_quota": "(SELECT COALESCE(SUM(today_stat.quota), 0) FROM account_stat_daily today_stat WHERE today_stat.account_id = account.id AND today_stat.date_key = CAST(DATE_FORMAT(DATE_ADD(UTC_TIMESTAMP(), INTERVAL 8 HOUR), '%Y%m%d') AS UNSIGNED))",
	}
	expression, exists := sortExpressions[filter.SortBy]
	if !exists {
		return nil, 0, fmt.Errorf("unsupported account sort %q", filter.SortBy)
	}
	if filter.SortBy == "today_quota" && filter.TodayDateKey > 0 {
		expression = fmt.Sprintf("(SELECT COALESCE(SUM(today_stat.quota), 0) FROM account_stat_daily today_stat WHERE today_stat.account_id = account.id AND today_stat.date_key = %d)", filter.TodayDateKey)
	}
	order := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		order = "ASC"
	}
	var accounts []Account
	err := query.Order(expression + " " + order).Order("account.id DESC").
		Offset(filter.Offset).Limit(filter.Limit).Find(&accounts).Error
	return accounts, total, err
}

func (repository *AccountRepository) LoadListMetadata(
	ctx context.Context,
	accountIDs []int64,
) (map[int64]AccountListMetadata, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrAccountListContract
	}
	if len(accountIDs) == 0 {
		return map[int64]AccountListMetadata{}, nil
	}
	ids := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			return nil, ErrAccountListContract
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	latestRuns := repository.db.Table("collection_run AS candidate").
		Select("candidate.target_id, MAX(candidate.id) AS run_id").
		Where("candidate.target_type = ? AND candidate.target_id IN ? AND candidate.task_type IN ?",
			"account", ids, []string{constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild}).
		Group("candidate.target_id")
	var rows []AccountListMetadata
	err := repository.db.WithContext(ctx).Table("account AS account").
		Select(`account.id AS account_id, site.name AS site_name, customer.name AS customer_name,
  CAST(site.quota_per_unit AS CHAR) AS quota_per_unit,
  CAST(site.usd_exchange_rate AS CHAR) AS usd_exchange_rate, site.last_rate_at,
  run.id AS latest_run_id, run.status AS latest_run_status,
  run.start_timestamp AS latest_run_start_timestamp, run.end_timestamp AS latest_run_end_timestamp,
  run.total_windows AS latest_run_total_windows,
  run.completed_windows AS latest_run_completed_windows,
  run.failed_windows AS latest_run_failed_windows`).
		Joins("JOIN site ON site.id = account.site_id").
		Joins("JOIN customer ON customer.id = account.customer_id").
		Joins("LEFT JOIN (?) AS latest ON latest.target_id = account.id", latestRuns).
		Joins("LEFT JOIN collection_run AS run ON run.id = latest.run_id").
		Where("account.id IN ?", ids).
		Order("account.id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load account list metadata: %w", err)
	}
	result := make(map[int64]AccountListMetadata, len(rows))
	for _, row := range rows {
		if row.AccountID <= 0 {
			return nil, ErrAccountListContract
		}
		if _, duplicate := result[row.AccountID]; duplicate {
			return nil, ErrAccountListContract
		}
		result[row.AccountID] = row
	}
	if len(result) != len(ids) {
		return nil, ErrAccountListContract
	}
	return result, nil
}

func (repository *AccountRepository) UpdateRemark(ctx context.Context, id int64, remark string, updatedAt int64) error {
	return repository.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).
		Updates(map[string]any{"remark": remark, "updated_at": updatedAt}).Error
}

// ApplyAuthoritativeRemoteSnapshot is the only path that resets a missing
// counter. identity_mismatch is permanent in this release: a newer authoritative
// snapshot may refresh display fields, but it cannot remove isolation or pause.
func (repository *AccountRepository) ApplyAuthoritativeRemoteSnapshot(
	ctx context.Context,
	id int64,
	snapshot AuthoritativeAccountRemoteSnapshot,
) (Account, bool, error) {
	if id <= 0 || snapshot.RemoteCreatedAt <= 0 || snapshot.ObservedAt <= 0 || snapshot.UpdatedAt <= 0 {
		return Account{}, false, ErrAccountObservationInvalid
	}
	var account Account
	applied := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := findAccountForUpdate(tx, id)
		if err != nil {
			return err
		}
		account = locked
		if snapshot.RemoteCreatedAt != locked.RemoteCreatedAt && locked.RemoteState != AccountRemoteStateIdentityMismatch {
			return ErrAccountIdentityMismatch
		}
		if !isNewerAccountObservation(locked, snapshot.ObservedAt) {
			return nil
		}
		nextRemoteState := AccountRemoteStateNormal
		nextMissingCount := 0
		if locked.RemoteState == AccountRemoteStateIdentityMismatch {
			nextRemoteState = AccountRemoteStateIdentityMismatch
			nextMissingCount = locked.RemoteMissingCount
		}
		result := accountObservationCAS(tx, locked).Updates(map[string]any{
			"username":             snapshot.Username,
			"display_name":         snapshot.DisplayName,
			"remote_group":         snapshot.RemoteGroup,
			"remote_status":        snapshot.RemoteStatus,
			"remote_state":         nextRemoteState,
			"remote_missing_count": nextMissingCount,
			"last_remote_seen_at":  snapshot.ObservedAt,
			"quota":                snapshot.Quota,
			"used_quota":           snapshot.UsedQuota,
			"request_count":        snapshot.RequestCount,
			"last_synced_at":       snapshot.ObservedAt,
			"updated_at":           monotonicAccountUpdatedAt(locked, snapshot.UpdatedAt),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountObservationCAS
		}
		account, err = findAccount(tx, id)
		applied = err == nil
		return err
	})
	return account, applied, err
}

// MarkMissing records one complete-snapshot absence. The observation timestamp
// is the CAS version: duplicate or out-of-order observations are no-ops, and no
// missing observation can lower the counter or return the account to normal.
func (repository *AccountRepository) MarkMissing(ctx context.Context, id, observedAt, updatedAt int64) (Account, bool, error) {
	if id <= 0 || observedAt <= 0 || updatedAt <= 0 {
		return Account{}, false, ErrAccountObservationInvalid
	}
	var account Account
	applied := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := findAccountForUpdate(tx, id)
		if err != nil {
			return err
		}
		account = locked
		if !isNewerAccountObservation(locked, observedAt) {
			return nil
		}
		nextCount := locked.RemoteMissingCount
		nextState := locked.RemoteState
		if locked.RemoteState != AccountRemoteStateIdentityMismatch {
			if nextCount < math.MaxInt32 {
				nextCount++
			}
			if nextCount >= 2 {
				nextState = AccountRemoteStateMissing
			}
		}
		result := accountObservationCAS(tx, locked).Updates(map[string]any{
			"remote_state":         nextState,
			"remote_missing_count": nextCount,
			"last_synced_at":       observedAt,
			"updated_at":           monotonicAccountUpdatedAt(locked, updatedAt),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountObservationCAS
		}
		account, err = findAccount(tx, id)
		applied = err == nil
		return err
	})
	return account, applied, err
}

func (repository *AccountRepository) MarkIdentityMismatch(
	ctx context.Context,
	id, observedAt, pausedAt, updatedAt int64,
) (Account, bool, error) {
	if id <= 0 || observedAt <= 0 || pausedAt <= 0 || updatedAt <= 0 {
		return Account{}, false, ErrAccountObservationInvalid
	}
	var account Account
	applied := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := findAccountForUpdate(tx, id)
		if err != nil {
			return err
		}
		account = locked
		if !isNewerAccountObservation(locked, observedAt) {
			return nil
		}
		effectivePause := pausedAt
		if locked.StatisticsPausedAt != nil && *locked.StatisticsPausedAt < effectivePause {
			effectivePause = *locked.StatisticsPausedAt
		}
		result := accountObservationCAS(tx, locked).Updates(map[string]any{
			"remote_state":         AccountRemoteStateIdentityMismatch,
			"statistics_paused_at": effectivePause,
			"last_synced_at":       observedAt,
			"updated_at":           monotonicAccountUpdatedAt(locked, updatedAt),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountObservationCAS
		}
		if err := cleanupStatisticsForPause(ctx, tx, []statisticsPauseAccount{{
			ID: locked.ID, SiteID: locked.SiteID, CustomerID: locked.CustomerID, PauseAt: effectivePause,
		}}, nil, updatedAt); err != nil {
			return err
		}
		account, err = findAccount(tx, id)
		applied = err == nil
		return err
	})
	return account, applied, err
}

func accountObservationCAS(tx *gorm.DB, account Account) *gorm.DB {
	query := tx.Model(&Account{}).
		Where("id = ? AND remote_state = ? AND remote_missing_count = ?", account.ID, account.RemoteState, account.RemoteMissingCount)
	if account.LastSyncedAt == nil {
		return query.Where("last_synced_at IS NULL")
	}
	return query.Where("last_synced_at = ?", *account.LastSyncedAt)
}

func isNewerAccountObservation(account Account, observedAt int64) bool {
	return account.LastSyncedAt == nil || observedAt > *account.LastSyncedAt
}

func monotonicAccountUpdatedAt(account Account, updatedAt int64) int64 {
	if account.UpdatedAt >= updatedAt {
		return account.UpdatedAt + 1
	}
	return updatedAt
}

func findAccountForUpdate(db *gorm.DB, id int64) (Account, error) {
	var account Account
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error
	return account, err
}

func findAccount(db *gorm.DB, id int64) (Account, error) {
	var account Account
	err := db.First(&account, id).Error
	return account, err
}

// LockAccountOperationScope is the common transaction fence for account rebuild
// run writers and lifecycle completion. Callers must insert or inspect runs only
// after this function returns so READ-COMMITTED cannot admit a newer logical run.
func LockAccountOperationScope(ctx context.Context, db *gorm.DB, id int64) (AccountOperationScope, error) {
	if db == nil || id <= 0 {
		return AccountOperationScope{}, ErrAccountRestoreContract
	}
	var reference accountOperationReference
	if err := db.WithContext(ctx).Model(&Account{}).
		Select("id", "site_id", "customer_id").Where("id = ?", id).Take(&reference).Error; err != nil {
		return AccountOperationScope{}, err
	}

	sites, err := lockSitesForUpdate(db.WithContext(ctx), []int64{reference.SiteID})
	if err != nil {
		return AccountOperationScope{}, err
	}
	if len(sites) != 1 || sites[0].ID != reference.SiteID {
		return AccountOperationScope{}, gorm.ErrRecordNotFound
	}
	customer, err := findCustomerForUpdate(db.WithContext(ctx), reference.CustomerID)
	if err != nil {
		return AccountOperationScope{}, err
	}
	account, err := findAccountForUpdate(db.WithContext(ctx), id)
	if err != nil {
		return AccountOperationScope{}, err
	}
	if account.SiteID != reference.SiteID || account.CustomerID != reference.CustomerID {
		return AccountOperationScope{}, ErrAccountOperationScopeChanged
	}
	return AccountOperationScope{Site: sites[0], Customer: customer, Account: account}, nil
}

func (repository *AccountRepository) Archive(ctx context.Context, id, pausedAt, updatedAt int64) error {
	if id <= 0 || pausedAt <= 0 || updatedAt <= 0 {
		return ErrAccountObservationInvalid
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account, err := findAccountForUpdate(tx, id)
		if err != nil {
			return err
		}
		effectivePause := pausedAt
		if account.StatisticsPausedAt != nil && *account.StatisticsPausedAt < effectivePause {
			effectivePause = *account.StatisticsPausedAt
		}
		updatedAt = monotonicAccountUpdatedAt(account, updatedAt)
		if err := tx.Model(&Account{}).Where("id = ?", id).Updates(map[string]any{
			"managed_status":             AccountManagedStatusArchived,
			"statistics_paused_at":       effectivePause,
			"statistics_backfill_status": "none",
			"updated_at":                 updatedAt,
		}).Error; err != nil {
			return err
		}
		return cleanupStatisticsForPause(ctx, tx, []statisticsPauseAccount{{
			ID: account.ID, SiteID: account.SiteID, CustomerID: account.CustomerID, PauseAt: effectivePause,
		}}, nil, updatedAt)
	})
}

// BeginRestore creates the durable pending boundary. It deliberately leaves the
// account archived and paused until CompleteRestore validates the authoritative run.
func (repository *AccountRepository) BeginRestore(ctx context.Context, id, updatedAt int64) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return NewAccountRepository(tx).BeginRestoreInTransaction(ctx, id, updatedAt)
	})
}

// BeginRestoreInTransaction holds the site/customer/account scope through the
// service transaction that creates the account_rebuild run.
func (repository *AccountRepository) BeginRestoreInTransaction(ctx context.Context, id, updatedAt int64) error {
	if id <= 0 || updatedAt <= 0 {
		return ErrAccountRestoreContract
	}
	scope, err := LockAccountOperationScope(ctx, repository.db, id)
	if err != nil {
		return err
	}
	if scope.Customer.Status == "disabled" {
		return ErrCustomerDisabled
	}
	if scope.Account.RemoteState == AccountRemoteStateIdentityMismatch {
		return ErrAccountIdentityMismatch
	}
	if scope.Account.ManagedStatus != AccountManagedStatusArchived || scope.Account.StatisticsPausedAt == nil {
		return ErrAccountRestoreContract
	}
	updatedAt = monotonicAccountUpdatedAt(scope.Account, updatedAt)
	return repository.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Updates(map[string]any{
		"statistics_backfill_status": "pending",
		"updated_at":                 updatedAt,
	}).Error
}

func (repository *AccountRepository) CompleteRestore(ctx context.Context, id, updatedAt int64) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return NewAccountRepository(tx).CompleteRestoreInTransaction(ctx, id, updatedAt)
	})
}

// CompleteRestoreInTransaction atomically opens an archived account only when
// its latest exact account_rebuild run succeeded and covers the pause boundary.
func (repository *AccountRepository) CompleteRestoreInTransaction(ctx context.Context, id, updatedAt int64) error {
	if id <= 0 || updatedAt <= 0 {
		return ErrAccountRestoreContract
	}
	scope, err := LockAccountOperationScope(ctx, repository.db, id)
	if err != nil {
		return err
	}
	if scope.Customer.Status == "disabled" {
		return ErrCustomerDisabled
	}
	if scope.Account.RemoteState == AccountRemoteStateIdentityMismatch {
		return ErrAccountIdentityMismatch
	}
	if scope.Account.ManagedStatus != AccountManagedStatusArchived || scope.Account.StatisticsPausedAt == nil {
		return ErrAccountRestoreContract
	}
	updatedAt = monotonicAccountUpdatedAt(scope.Account, updatedAt)
	run, err := latestRebuildRunForUpdate(
		repository.db.WithContext(ctx), "account", id, constant.TaskTypeAccountRebuild,
	)
	if err != nil {
		return err
	}
	if run.Status != "success" || !rebuildRunCoversRange(run, scope.Account.StatisticsPausedAt, updatedAt) {
		return fmt.Errorf("%w: account %d run %d status %s", ErrRebuildRunNotReady, id, run.ID, run.Status)
	}
	return repository.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Updates(map[string]any{
		"managed_status":             AccountManagedStatusActive,
		"statistics_paused_at":       nil,
		"statistics_backfill_status": "none",
		"updated_at":                 updatedAt,
	}).Error
}

func latestRebuildRunForUpdate(db *gorm.DB, targetType string, targetID int64, taskType string) (CollectionRun, error) {
	var runs []CollectionRun
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("target_type = ? AND target_id = ? AND task_type = ?", targetType, targetID, taskType).
		Order("id DESC").Limit(1).Find(&runs).Error; err != nil {
		return CollectionRun{}, err
	}
	if len(runs) == 0 {
		return CollectionRun{}, fmt.Errorf("%w: no %s run for %s %d", ErrRebuildRunNotReady, taskType, targetType, targetID)
	}
	return runs[0], nil
}

func rebuildRunCoversRange(run CollectionRun, pausedAt *int64, completedAt int64) bool {
	if pausedAt == nil || run.StartTimestamp == nil || run.EndTimestamp == nil {
		return false
	}
	requiredEnd := completedAt - completedAt%3600
	if requiredEnd <= *pausedAt {
		requiredEnd = *pausedAt + 1
	}
	return *run.StartTimestamp <= *pausedAt && *run.EndTimestamp >= requiredEnd
}

func (repository *AccountRepository) ListManagedBindings(ctx context.Context, siteID int64, remoteUserIDs []int64) ([]ManagedAccountBinding, error) {
	if len(remoteUserIDs) == 0 {
		return []ManagedAccountBinding{}, nil
	}
	var bindings []ManagedAccountBinding
	err := repository.db.WithContext(ctx).Table("account").
		Select("account.remote_user_id, account.id AS account_id, account.customer_id, customer.name AS customer_name, account.managed_status").
		Joins("JOIN customer ON customer.id = account.customer_id").
		Where("account.site_id = ? AND account.remote_user_id IN ?", siteID, remoteUserIDs).
		Order("account.remote_user_id ASC").Scan(&bindings).Error
	return bindings, err
}

func (repository *AccountRepository) FindManagedBindings(ctx context.Context, siteID int64, remoteUserIDs []int64) (map[int64]ManagedAccountBinding, error) {
	rows, err := repository.ListManagedBindings(ctx, siteID, remoteUserIDs)
	if err != nil {
		return nil, err
	}
	bindings := make(map[int64]ManagedAccountBinding, len(rows))
	for _, row := range rows {
		bindings[row.RemoteUserID] = row
	}
	return bindings, nil
}

func (repository *AccountRepository) DeletionDependencies(ctx context.Context, id int64) (AccountDeletionDependencies, error) {
	account, err := repository.FindByID(ctx, id)
	if err != nil {
		return AccountDeletionDependencies{}, err
	}
	return accountDeletionDependencies(repository.db.WithContext(ctx), id, account.CustomerID)
}

func accountDeletionDependencies(db *gorm.DB, id, customerID int64) (AccountDeletionDependencies, error) {
	var dependencies AccountDeletionDependencies
	checks := []struct {
		table  string
		target *int64
	}{
		{table: "account_stat_hourly", target: &dependencies.HourlyStats},
		{table: "account_stat_daily", target: &dependencies.DailyStats},
	}
	for _, check := range checks {
		if err := db.Table(check.table).Where("account_id = ?", id).Count(check.target).Error; err != nil {
			return AccountDeletionDependencies{}, err
		}
	}
	if err := db.Model(&CollectionRun{}).
		Where("status IN ? AND ((target_type = ? AND target_id = ?) OR (target_type = ? AND target_id = ?))",
			[]string{"pending", "running"}, "account", id, "customer", customerID).
		Count(&dependencies.ActiveRuns).Error; err != nil {
		return AccountDeletionDependencies{}, err
	}
	targetKey := strconv.FormatInt(id, 10)
	if err := db.Raw(`SELECT COUNT(*) FROM alert_event e
WHERE (e.target_type = 'account' AND e.target_key = ?
       OR e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND EXISTS (
         SELECT 1 FROM collection_run r
         WHERE r.target_type = 'account' AND r.target_id = ?
           AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR)))
  AND e.status <> 'resolved'`, targetKey, id).Scan(&dependencies.ActiveAlerts).Error; err != nil {
		return AccountDeletionDependencies{}, err
	}
	if err := db.Raw(`SELECT COUNT(*) FROM alert_event e
WHERE (e.target_type = 'account' AND e.target_key = ?
       OR e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND EXISTS (
         SELECT 1 FROM collection_run r
         WHERE r.target_type = 'account' AND r.target_id = ?
           AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR)))
  AND e.status = 'resolved'
  AND (e.first_fired_at IS NOT NULL OR e.last_fired_at IS NOT NULL OR
       EXISTS (SELECT 1 FROM alert_delivery d WHERE d.alert_event_id = e.id))`, targetKey, id).
		Scan(&dependencies.AlertHistory).Error; err != nil {
		return AccountDeletionDependencies{}, err
	}
	return dependencies, nil
}

func (repository *AccountRepository) CanDelete(ctx context.Context, id int64) (bool, AccountDeletionDependencies, error) {
	dependencies, err := repository.DeletionDependencies(ctx, id)
	return err == nil && !dependencies.HasAny(), dependencies, err
}

func (repository *AccountRepository) Delete(ctx context.Context, account *Account) error {
	return repository.DeleteByID(ctx, account.ID)
}

func (repository *AccountRepository) DeleteByID(ctx context.Context, id int64) error {
	accountRef, err := repository.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var siteID int64
		if err := tx.Model(&Site{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").Where("id = ?", accountRef.SiteID).Scan(&siteID).Error; err != nil {
			return err
		}
		if siteID != accountRef.SiteID {
			return gorm.ErrRecordNotFound
		}
		var customerID int64
		if err := tx.Model(&Customer{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").Where("id = ?", accountRef.CustomerID).Scan(&customerID).Error; err != nil {
			return err
		}
		if customerID != accountRef.CustomerID {
			return gorm.ErrRecordNotFound
		}
		account, err := findAccountForUpdate(tx, id)
		if err != nil {
			return err
		}
		if account.SiteID != accountRef.SiteID || account.CustomerID != accountRef.CustomerID {
			return ErrAccountObservationCAS
		}
		dependencies, err := accountDeletionDependencies(tx, id, account.CustomerID)
		if err != nil {
			return err
		}
		if dependencies.HasAny() {
			return &DeleteDependencyError{Resource: "account", ID: id, Counts: dependencies.counts()}
		}
		if err := deleteAccountOwnedMetadata(tx, id); err != nil {
			return err
		}
		return tx.Delete(&account).Error
	})
}

func deleteAccountOwnedMetadata(db *gorm.DB, id int64) error {
	targetKey := strconv.FormatInt(id, 10)
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "terminal run windows", query: `DELETE rw FROM collection_run_window rw
JOIN collection_run r ON r.id = rw.run_id
WHERE r.target_type = 'account' AND r.target_id = ? AND r.status IN ('success','failed')`, args: []any{id}},
		{name: "empty resolved collection alerts", query: `DELETE e FROM alert_event e
LEFT JOIN alert_delivery d ON d.alert_event_id = e.id
WHERE e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND e.status = 'resolved'
  AND e.first_fired_at IS NULL AND e.last_fired_at IS NULL AND d.id IS NULL
  AND EXISTS (SELECT 1 FROM collection_run r
              WHERE r.target_type = 'account' AND r.target_id = ?
                AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR))`, args: []any{id}},
		{name: "terminal collection runs", query: `DELETE FROM collection_run
WHERE target_type = 'account' AND target_id = ? AND status IN ('success','failed')`, args: []any{id}},
		{name: "empty resolved alert events", query: `DELETE e FROM alert_event e
LEFT JOIN alert_delivery d ON d.alert_event_id = e.id
WHERE e.target_type = 'account' AND e.target_key = ? AND e.status = 'resolved'
  AND e.first_fired_at IS NULL AND e.last_fired_at IS NULL AND d.id IS NULL`, args: []any{targetKey}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			return fmt.Errorf("delete account %s: %w", statement.name, err)
		}
	}
	return nil
}
