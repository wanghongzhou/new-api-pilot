package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"new-api-pilot/constant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDeleteHasDependencies         = errors.New("record has deletion dependencies")
	ErrCustomerLifecycleContract     = errors.New("invalid customer lifecycle transition")
	ErrCustomerDisabled              = errors.New("customer is disabled")
	ErrCustomerEnableNotReady        = errors.New("customer enable rebuild is not complete")
	ErrCustomerOperationScopeChanged = errors.New("customer operation scope changed while acquiring locks")
)

type DeleteDependencyError struct {
	Resource string
	ID       int64
	Counts   map[string]int64
}

func (err *DeleteDependencyError) Error() string {
	return fmt.Sprintf("cannot delete %s %d: dependencies exist", err.Resource, err.ID)
}

func (err *DeleteDependencyError) Unwrap() error { return ErrDeleteHasDependencies }

type Customer struct {
	ID                       int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name                     string `gorm:"column:name"`
	Contact                  string `gorm:"column:contact"`
	Remark                   string `gorm:"column:remark"`
	Status                   string `gorm:"column:status"`
	StatisticsPausedAt       *int64 `gorm:"column:statistics_paused_at"`
	StatisticsBackfillStatus string `gorm:"column:statistics_backfill_status"`
	CreatedAt                int64  `gorm:"column:created_at"`
	UpdatedAt                int64  `gorm:"column:updated_at"`
}

func (Customer) TableName() string { return "customer" }

type CustomerFilter struct {
	Keyword      string
	Status       string
	Statuses     []string
	SortBy       string
	SortOrder    string
	TodayDateKey int
	Offset       int
	Limit        int
}

type CustomerDeletionDependencies struct {
	Accounts     int64
	HourlyStats  int64
	DailyStats   int64
	ActiveRuns   int64
	ActiveAlerts int64
	AlertHistory int64
}

func (dependencies CustomerDeletionDependencies) HasAny() bool {
	return dependencies.Accounts > 0 || dependencies.HourlyStats > 0 || dependencies.DailyStats > 0 ||
		dependencies.ActiveRuns > 0 || dependencies.ActiveAlerts > 0 || dependencies.AlertHistory > 0
}

func (dependencies CustomerDeletionDependencies) counts() map[string]int64 {
	return map[string]int64{
		"accounts":             dependencies.Accounts,
		"customer_stat_hourly": dependencies.HourlyStats,
		"customer_stat_daily":  dependencies.DailyStats,
		"active_collection":    dependencies.ActiveRuns,
		"active_alert":         dependencies.ActiveAlerts,
		"alert_history":        dependencies.AlertHistory,
	}
}

type CustomerRepository struct {
	db *gorm.DB
}

type CustomerOperationScope struct {
	Sites    []Site
	Customer Customer
	Accounts []Account
}

type customerAccountReference struct {
	ID         int64 `gorm:"column:id"`
	SiteID     int64 `gorm:"column:site_id"`
	CustomerID int64 `gorm:"column:customer_id"`
}

func NewCustomerRepository(db *gorm.DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

func (repository *CustomerRepository) WithTransaction(ctx context.Context, operation func(*CustomerRepository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(&CustomerRepository{db: tx})
	})
}

func (repository *CustomerRepository) Create(ctx context.Context, customer *Customer) error {
	return repository.db.WithContext(ctx).Create(customer).Error
}

// UpdateProfile cannot enter or leave disabled; those transitions use the explicit lifecycle methods.
func (repository *CustomerRepository) UpdateProfile(ctx context.Context, customer *Customer) error {
	if customer.Status == "disabled" {
		return ErrCustomerLifecycleContract
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := findCustomerForUpdate(tx, customer.ID)
		if err != nil {
			return err
		}
		if current.Status == "disabled" {
			return ErrCustomerLifecycleContract
		}
		return tx.Model(&Customer{}).Where("id = ?", customer.ID).Updates(map[string]any{
			"name":       customer.Name,
			"contact":    customer.Contact,
			"remark":     customer.Remark,
			"status":     customer.Status,
			"updated_at": customer.UpdatedAt,
		}).Error
	})
}

func (repository *CustomerRepository) FindByID(ctx context.Context, id int64) (Customer, error) {
	var customer Customer
	err := repository.db.WithContext(ctx).First(&customer, id).Error
	return customer, err
}

func (repository *CustomerRepository) FindByIDForUpdate(ctx context.Context, id int64) (Customer, error) {
	var customer Customer
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, id).Error
	return customer, err
}

func (repository *CustomerRepository) List(ctx context.Context, filter CustomerFilter) ([]Customer, int64, error) {
	query := repository.db.WithContext(ctx).Model(&Customer{})
	if filter.Keyword != "" {
		keyword := "%" + escapeLike(filter.Keyword) + "%"
		query = query.Where("(customer.name LIKE ? ESCAPE '\\\\' OR customer.contact LIKE ? ESCAPE '\\\\')", keyword, keyword)
	}
	if filter.Status != "" {
		query = query.Where("customer.status = ?", filter.Status)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("customer.status IN ?", filter.Statuses)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortExpressions := map[string]string{
		"updated_at":    "customer.updated_at",
		"name":          "customer.name",
		"account_count": "(SELECT COUNT(*) FROM account account_count_row WHERE account_count_row.customer_id = customer.id)",
		"today_quota":   "(SELECT COALESCE(SUM(today_stat.quota), 0) FROM customer_stat_daily today_stat WHERE today_stat.customer_id = customer.id AND today_stat.date_key = CAST(DATE_FORMAT(DATE_ADD(UTC_TIMESTAMP(), INTERVAL 8 HOUR), '%Y%m%d') AS UNSIGNED))",
	}
	expression, exists := sortExpressions[filter.SortBy]
	if !exists {
		return nil, 0, fmt.Errorf("unsupported customer sort %q", filter.SortBy)
	}
	if filter.SortBy == "today_quota" && filter.TodayDateKey > 0 {
		expression = fmt.Sprintf("(SELECT COALESCE(SUM(today_stat.quota), 0) FROM customer_stat_daily today_stat WHERE today_stat.customer_id = customer.id AND today_stat.date_key = %d)", filter.TodayDateKey)
	}
	order := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		order = "ASC"
	}
	var customers []Customer
	err := query.Order(expression + " " + order).Order("customer.id DESC").
		Offset(filter.Offset).Limit(filter.Limit).Find(&customers).Error
	return customers, total, err
}

func (repository *CustomerRepository) Disable(ctx context.Context, id, pausedAt, updatedAt int64) error {
	return repository.WithTransaction(ctx, func(transaction *CustomerRepository) error {
		return transaction.DisableInTransaction(ctx, id, pausedAt, updatedAt)
	})
}

// DisableInTransaction locks sites, customer, and accounts in global order,
// then propagates the earliest pause boundary without changing account ownership.
func (repository *CustomerRepository) DisableInTransaction(ctx context.Context, id, pausedAt, updatedAt int64) error {
	if id <= 0 || pausedAt <= 0 || updatedAt <= 0 {
		return ErrCustomerLifecycleContract
	}
	scope, err := LockCustomerOperationScope(ctx, repository.db, id)
	if err != nil {
		return err
	}
	updatedAt = monotonicCustomerUpdatedAt(scope.Customer, updatedAt)
	accounts := make([]Account, 0, len(scope.Accounts))
	siteIDs := make([]int64, 0, len(scope.Accounts))
	seenSites := make(map[int64]struct{}, len(scope.Accounts))
	for _, account := range scope.Accounts {
		if _, exists := seenSites[account.SiteID]; !exists {
			seenSites[account.SiteID] = struct{}{}
			siteIDs = append(siteIDs, account.SiteID)
		}
		if account.ManagedStatus == AccountManagedStatusActive {
			accounts = append(accounts, account)
		}
	}
	sort.Slice(siteIDs, func(left, right int) bool { return siteIDs[left] < siteIDs[right] })
	effectiveCustomerPause := pausedAt
	if scope.Customer.StatisticsPausedAt != nil && *scope.Customer.StatisticsPausedAt < effectiveCustomerPause {
		effectiveCustomerPause = *scope.Customer.StatisticsPausedAt
	}
	if err := repository.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id).Updates(map[string]any{
		"status":                     "disabled",
		"statistics_paused_at":       effectiveCustomerPause,
		"statistics_backfill_status": "none",
		"updated_at":                 updatedAt,
	}).Error; err != nil {
		return err
	}
	statisticsAccounts := make([]statisticsPauseAccount, 0, len(accounts))
	for _, account := range accounts {
		effectiveAccountPause := effectiveCustomerPause
		if account.StatisticsPausedAt != nil && *account.StatisticsPausedAt < effectiveAccountPause {
			effectiveAccountPause = *account.StatisticsPausedAt
		}
		accountUpdatedAt := monotonicAccountUpdatedAt(account, updatedAt)
		if err := repository.db.WithContext(ctx).Model(&Account{}).Where("id = ?", account.ID).Updates(map[string]any{
			"statistics_paused_at": effectiveAccountPause,
			"updated_at":           accountUpdatedAt,
		}).Error; err != nil {
			return err
		}
		statisticsAccounts = append(statisticsAccounts, statisticsPauseAccount{
			ID: account.ID, SiteID: account.SiteID, CustomerID: account.CustomerID, PauseAt: effectiveAccountPause,
		})
	}
	return cleanupStatisticsForPause(ctx, repository.db, statisticsAccounts, []statisticsPauseCustomer{{
		ID: id, PauseAt: effectiveCustomerPause, SiteIDs: siteIDs,
	}}, updatedAt)
}

// BeginEnable records intent only. The customer remains disabled and all pause
// boundaries remain effective until CompleteEnable validates the rebuild runs.
func (repository *CustomerRepository) BeginEnable(ctx context.Context, id, updatedAt int64) error {
	return repository.WithTransaction(ctx, func(transaction *CustomerRepository) error {
		return transaction.BeginEnableInTransaction(ctx, id, updatedAt)
	})
}

func (repository *CustomerRepository) BeginEnableInTransaction(ctx context.Context, id, updatedAt int64) error {
	if id <= 0 || updatedAt <= 0 {
		return ErrCustomerLifecycleContract
	}
	scope, err := LockCustomerOperationScope(ctx, repository.db, id)
	if err != nil {
		return err
	}
	if scope.Customer.Status != "disabled" || scope.Customer.StatisticsPausedAt == nil {
		return ErrCustomerLifecycleContract
	}
	updatedAt = monotonicCustomerUpdatedAt(scope.Customer, updatedAt)
	return repository.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id).Updates(map[string]any{
		"statistics_backfill_status": "pending",
		"updated_at":                 updatedAt,
	}).Error
}

func (repository *CustomerRepository) CompleteEnable(ctx context.Context, id, updatedAt int64) error {
	return repository.WithTransaction(ctx, func(transaction *CustomerRepository) error {
		return transaction.CompleteEnableInTransaction(ctx, id, updatedAt)
	})
}

// CompleteEnableInTransaction locks sites, customer, accounts, and related
// rebuild runs in global order before atomically opening covered rows.
func (repository *CustomerRepository) CompleteEnableInTransaction(ctx context.Context, id, updatedAt int64) error {
	if id <= 0 || updatedAt <= 0 {
		return ErrCustomerLifecycleContract
	}
	scope, err := LockCustomerOperationScope(ctx, repository.db, id)
	if err != nil {
		return err
	}
	if scope.Customer.Status != "disabled" || scope.Customer.StatisticsPausedAt == nil {
		return ErrCustomerLifecycleContract
	}
	updatedAt = monotonicCustomerUpdatedAt(scope.Customer, updatedAt)
	accounts := make([]Account, 0, len(scope.Accounts))
	for _, account := range scope.Accounts {
		if account.ManagedStatus != AccountManagedStatusArchived && account.RemoteState != AccountRemoteStateIdentityMismatch {
			accounts = append(accounts, account)
		}
	}
	accountIDs := make([]int64, len(accounts))
	for index := range accounts {
		accountIDs[index] = accounts[index].ID
	}
	runs, err := lockCustomerEnableRuns(repository.db.WithContext(ctx), id, accountIDs)
	if err != nil {
		return err
	}
	var customerRun *CollectionRun
	accountRuns := make(map[int64]*CollectionRun, len(accounts))
	for index := range runs {
		run := &runs[index]
		switch {
		case run.TargetType == "customer" && run.TargetID == id && run.TaskType == constant.TaskTypeCustomerRebuild:
			if customerRun == nil || run.ID > customerRun.ID {
				customerRun = run
			}
		case run.TargetType == "account" && run.TaskType == constant.TaskTypeAccountRebuild:
			current := accountRuns[run.TargetID]
			if current == nil || run.ID > current.ID {
				accountRuns[run.TargetID] = run
			}
		}
	}
	if customerRun == nil || customerRun.Status != "success" ||
		!rebuildRunCoversRange(*customerRun, scope.Customer.StatisticsPausedAt, updatedAt) {
		return fmt.Errorf("%w: customer %d latest customer_rebuild is not authoritative success", ErrCustomerEnableNotReady, id)
	}
	for _, account := range accounts {
		authoritative := customerRun
		if accountRun := accountRuns[account.ID]; accountRun != nil && accountRun.ID > authoritative.ID {
			authoritative = accountRun
		}
		if authoritative.Status != "success" ||
			(account.StatisticsPausedAt != nil && !rebuildRunCoversRange(*authoritative, account.StatisticsPausedAt, updatedAt)) {
			return fmt.Errorf("%w: account %d latest run %d status %s", ErrCustomerEnableNotReady,
				account.ID, authoritative.ID, authoritative.Status)
		}
	}
	for _, account := range accounts {
		accountUpdatedAt := monotonicAccountUpdatedAt(account, updatedAt)
		if err := repository.db.WithContext(ctx).Model(&Account{}).Where("id = ?", account.ID).Updates(map[string]any{
			"statistics_paused_at":       nil,
			"statistics_backfill_status": "none",
			"updated_at":                 accountUpdatedAt,
		}).Error; err != nil {
			return err
		}
	}
	return repository.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id).Updates(map[string]any{
		"status":                     "using",
		"statistics_paused_at":       nil,
		"statistics_backfill_status": "none",
		"updated_at":                 updatedAt,
	}).Error
}

func monotonicCustomerUpdatedAt(customer Customer, updatedAt int64) int64 {
	if customer.UpdatedAt >= updatedAt {
		return customer.UpdatedAt + 1
	}
	return updatedAt
}

func lockCustomerEnableRuns(db *gorm.DB, customerID int64, accountIDs []int64) ([]CollectionRun, error) {
	query := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("target_type = ? AND target_id = ? AND task_type = ?",
			"customer", customerID, constant.TaskTypeCustomerRebuild)
	if len(accountIDs) > 0 {
		query = query.Or("target_type = ? AND target_id IN ? AND task_type = ?",
			"account", accountIDs, constant.TaskTypeAccountRebuild)
	}
	var runs []CollectionRun
	err := query.Order("id ASC").Find(&runs).Error
	return runs, err
}

func findCustomerForUpdate(db *gorm.DB, id int64) (Customer, error) {
	var customer Customer
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, id).Error
	return customer, err
}

// LockCustomerOperationScope is the common transaction fence for customer
// rebuild writers and lifecycle operations. It freezes the complete account
// membership before callers insert or inspect collection_run rows.
func LockCustomerOperationScope(ctx context.Context, db *gorm.DB, id int64) (CustomerOperationScope, error) {
	if db == nil || id <= 0 {
		return CustomerOperationScope{}, ErrCustomerLifecycleContract
	}
	references, err := loadCustomerAccountReferences(db.WithContext(ctx), id)
	if err != nil {
		return CustomerOperationScope{}, err
	}
	siteIDs := uniqueSortedCustomerSiteIDs(references)
	sites, err := lockSitesForUpdate(db.WithContext(ctx), siteIDs)
	if err != nil {
		return CustomerOperationScope{}, err
	}
	if len(sites) != len(siteIDs) {
		return CustomerOperationScope{}, gorm.ErrRecordNotFound
	}
	customer, err := findCustomerForUpdate(db.WithContext(ctx), id)
	if err != nil {
		return CustomerOperationScope{}, err
	}
	accountIDs := make([]int64, len(references))
	for index := range references {
		accountIDs[index] = references[index].ID
	}
	accounts := make([]Account, 0, len(accountIDs))
	if len(accountIDs) > 0 {
		if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", accountIDs).Order("id ASC").Find(&accounts).Error; err != nil {
			return CustomerOperationScope{}, err
		}
	}
	lockedReferences, err := loadCustomerAccountReferences(db.WithContext(ctx), id)
	if err != nil {
		return CustomerOperationScope{}, err
	}
	if !sameCustomerAccountReferences(references, lockedReferences) || len(accounts) != len(references) {
		return CustomerOperationScope{}, ErrCustomerOperationScopeChanged
	}
	for index := range accounts {
		if accounts[index].ID != references[index].ID || accounts[index].SiteID != references[index].SiteID ||
			accounts[index].CustomerID != references[index].CustomerID {
			return CustomerOperationScope{}, ErrCustomerOperationScopeChanged
		}
	}
	return CustomerOperationScope{Sites: sites, Customer: customer, Accounts: accounts}, nil
}

func loadCustomerAccountReferences(db *gorm.DB, customerID int64) ([]customerAccountReference, error) {
	var references []customerAccountReference
	err := db.Model(&Account{}).Select("id", "site_id", "customer_id").
		Where("customer_id = ?", customerID).Order("id ASC").Find(&references).Error
	return references, err
}

func uniqueSortedCustomerSiteIDs(references []customerAccountReference) []int64 {
	unique := make(map[int64]struct{}, len(references))
	for _, reference := range references {
		unique[reference.SiteID] = struct{}{}
	}
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func lockSitesForUpdate(db *gorm.DB, ids []int64) ([]Site, error) {
	if len(ids) == 0 {
		return []Site{}, nil
	}
	var sites []Site
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id ASC").Find(&sites).Error
	return sites, err
}

func sameCustomerAccountReferences(left, right []customerAccountReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (repository *CustomerRepository) DeletionDependencies(ctx context.Context, id int64) (CustomerDeletionDependencies, error) {
	return customerDeletionDependencies(repository.db.WithContext(ctx), id)
}

func customerDeletionDependencies(db *gorm.DB, id int64) (CustomerDeletionDependencies, error) {
	var dependencies CustomerDeletionDependencies
	checks := []struct {
		table  string
		column string
		target *int64
	}{
		{table: "account", column: "customer_id", target: &dependencies.Accounts},
		{table: "customer_stat_hourly", column: "customer_id", target: &dependencies.HourlyStats},
		{table: "customer_stat_daily", column: "customer_id", target: &dependencies.DailyStats},
	}
	for _, check := range checks {
		if err := db.Table(check.table).Where(check.column+" = ?", id).Count(check.target).Error; err != nil {
			return CustomerDeletionDependencies{}, err
		}
	}
	if err := db.Model(&CollectionRun{}).
		Where("status IN ? AND target_type = ? AND target_id = ?", []string{"pending", "running"}, "customer", id).
		Count(&dependencies.ActiveRuns).Error; err != nil {
		return CustomerDeletionDependencies{}, err
	}
	targetKey := strconv.FormatInt(id, 10)
	if err := db.Raw(`SELECT COUNT(*) FROM alert_event e
WHERE (e.target_type = 'customer' AND e.target_key = ?
       OR e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND EXISTS (
         SELECT 1 FROM collection_run r
         WHERE r.target_type = 'customer' AND r.target_id = ?
           AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR)))
  AND e.status <> 'resolved'`, targetKey, id).Scan(&dependencies.ActiveAlerts).Error; err != nil {
		return CustomerDeletionDependencies{}, err
	}
	if err := db.Raw(`SELECT COUNT(*) FROM alert_event e
WHERE (e.target_type = 'customer' AND e.target_key = ?
       OR e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND EXISTS (
         SELECT 1 FROM collection_run r
         WHERE r.target_type = 'customer' AND r.target_id = ?
           AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR)))
  AND e.status = 'resolved'
  AND (e.first_fired_at IS NOT NULL OR e.last_fired_at IS NOT NULL OR
       EXISTS (SELECT 1 FROM alert_delivery d WHERE d.alert_event_id = e.id))`, targetKey, id).
		Scan(&dependencies.AlertHistory).Error; err != nil {
		return CustomerDeletionDependencies{}, err
	}
	return dependencies, nil
}

func (repository *CustomerRepository) CanDelete(ctx context.Context, id int64) (bool, CustomerDeletionDependencies, error) {
	dependencies, err := repository.DeletionDependencies(ctx, id)
	return err == nil && !dependencies.HasAny(), dependencies, err
}

func (repository *CustomerRepository) Delete(ctx context.Context, customer *Customer) error {
	return repository.DeleteByID(ctx, customer.ID)
}

func (repository *CustomerRepository) DeleteByID(ctx context.Context, id int64) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customer Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, id).Error; err != nil {
			return err
		}
		dependencies, err := customerDeletionDependencies(tx, id)
		if err != nil {
			return err
		}
		if dependencies.HasAny() {
			return &DeleteDependencyError{Resource: "customer", ID: id, Counts: dependencies.counts()}
		}
		if err := deleteCustomerOwnedMetadata(tx, id); err != nil {
			return err
		}
		return tx.Delete(&customer).Error
	})
}

func deleteCustomerOwnedMetadata(db *gorm.DB, id int64) error {
	targetKey := strconv.FormatInt(id, 10)
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "terminal run windows", query: `DELETE rw FROM collection_run_window rw
JOIN collection_run r ON r.id = rw.run_id
WHERE r.target_type = 'customer' AND r.target_id = ? AND r.status IN ('success','failed')`, args: []any{id}},
		{name: "empty resolved collection alerts", query: `DELETE e FROM alert_event e
LEFT JOIN alert_delivery d ON d.alert_event_id = e.id
WHERE e.target_type = 'collection' AND e.rule_key = 'backfill_failed' AND e.status = 'resolved'
  AND e.first_fired_at IS NULL AND e.last_fired_at IS NULL AND d.id IS NULL
  AND EXISTS (SELECT 1 FROM collection_run r
              WHERE r.target_type = 'customer' AND r.target_id = ?
                AND SUBSTRING_INDEX(e.target_key, '/', -1) = CAST(r.id AS CHAR))`, args: []any{id}},
		{name: "terminal collection runs", query: `DELETE FROM collection_run
WHERE target_type = 'customer' AND target_id = ? AND status IN ('success','failed')`, args: []any{id}},
		{name: "empty resolved alert events", query: `DELETE e FROM alert_event e
LEFT JOIN alert_delivery d ON d.alert_event_id = e.id
WHERE e.target_type = 'customer' AND e.target_key = ? AND e.status = 'resolved'
  AND e.first_fired_at IS NULL AND e.last_fired_at IS NULL AND d.id IS NULL`, args: []any{targetKey}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			return fmt.Errorf("delete customer %s: %w", statement.name, err)
		}
	}
	return nil
}
