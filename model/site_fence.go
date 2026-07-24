package model

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/common"
	"new-api-pilot/constant"
)

type SiteMonitoringPause struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID        int64  `gorm:"column:site_id"`
	StartMinuteTS int64  `gorm:"column:start_minute_ts"`
	EndMinuteTS   *int64 `gorm:"column:end_minute_ts"`
	Reason        string `gorm:"column:reason"`
	CreatedAt     int64  `gorm:"column:created_at"`
}

func (SiteMonitoringPause) TableName() string { return "site_monitoring_pause" }

type CollectionRun struct {
	ID                   int64          `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID               *int64         `gorm:"column:site_id"`
	SiteConfigVersion    int            `gorm:"column:site_config_version"`
	TaskType             string         `gorm:"column:task_type"`
	TargetType           string         `gorm:"column:target_type"`
	TargetID             int64          `gorm:"column:target_id"`
	TriggerType          string         `gorm:"column:trigger_type"`
	StartTimestamp       *int64         `gorm:"column:start_timestamp"`
	EndTimestamp         *int64         `gorm:"column:end_timestamp"`
	Scope                []byte         `gorm:"column:scope;type:json"`
	ActiveKey            *string        `gorm:"column:active_key"`
	Status               string         `gorm:"column:status"`
	FetchedRows          int64          `gorm:"column:fetched_rows"`
	WrittenRows          int64          `gorm:"column:written_rows"`
	RetryCount           int            `gorm:"column:retry_count"`
	Priority             int            `gorm:"column:priority"`
	NextAttemptAt        int64          `gorm:"column:next_attempt_at"`
	HeartbeatAt          *int64         `gorm:"column:heartbeat_at"`
	WindowsInitializedAt *int64         `gorm:"column:windows_initialized_at"`
	TotalWindows         int            `gorm:"column:total_windows"`
	CompletedWindows     int            `gorm:"column:completed_windows"`
	FailedWindows        int            `gorm:"column:failed_windows"`
	UnavailableWindows   int            `gorm:"column:unavailable_windows;->"`
	CreatedRequestID     string         `gorm:"column:created_request_id"`
	LastRequestID        string         `gorm:"column:last_request_id"`
	ErrorCode            string         `gorm:"column:error_code"`
	ErrorParams          []byte         `gorm:"column:error_params;type:json"`
	ErrorMessage         sql.NullString `gorm:"column:error_message"`
	StartedAt            *int64         `gorm:"column:started_at"`
	FinishedAt           *int64         `gorm:"column:finished_at"`
	CreatedAt            int64          `gorm:"column:created_at"`
	UpdatedAt            int64          `gorm:"column:updated_at"`
}

func (CollectionRun) TableName() string { return "collection_run" }

type CollectionRunWindow struct {
	ID           int64   `gorm:"column:id;primaryKey;autoIncrement"`
	RunID        int64   `gorm:"column:run_id"`
	SiteID       int64   `gorm:"column:site_id"`
	HourTS       int64   `gorm:"column:hour_ts"`
	Status       string  `gorm:"column:status"`
	AttemptCount int     `gorm:"column:attempt_count"`
	NextRetryAt  *int64  `gorm:"column:next_retry_at"`
	FetchedRows  int64   `gorm:"column:fetched_rows"`
	WrittenRows  int64   `gorm:"column:written_rows"`
	ErrorCode    string  `gorm:"column:error_code"`
	ErrorParams  []byte  `gorm:"column:error_params;type:json"`
	ErrorMessage *string `gorm:"column:error_message"`
	StartedAt    *int64  `gorm:"column:started_at"`
	FinishedAt   *int64  `gorm:"column:finished_at"`
	UpdatedAt    int64   `gorm:"column:updated_at"`
}

func (CollectionRunWindow) TableName() string { return "collection_run_window" }

type CollectionRunFilter struct {
	SiteID    *int64
	TaskType  string
	Status    string
	SortBy    string
	SortOrder string
	Offset    int
	Limit     int
}

type CollectionRunWindowFilter struct {
	RunID  int64
	Status string
	Offset int
	Limit  int
}

type CollectionRunWindowSnapshot struct {
	CollectionRunWindow
	FactStatus string `gorm:"column:fact_status"`
	VerifiedAt *int64 `gorm:"column:verified_at"`
}

func (repository *SiteRepository) BumpSiteFence(ctx context.Context, site *Site, now int64) error {
	if site == nil || site.ID <= 0 || site.ConfigVersion <= 0 || now <= 0 {
		return fmt.Errorf("invalid site fence target")
	}
	plan, err := repository.buildSiteFencePlan(ctx, site.ID)
	if err != nil {
		return err
	}
	lockedSite, err := repository.FindByIDForUpdate(ctx, site.ID)
	if err != nil {
		return err
	}
	if lockedSite.ConfigVersion != site.ConfigVersion {
		return ErrSiteRunConfigChanged
	}
	newVersion := site.ConfigVersion + 1
	if err := repository.lockSiteFenceCustomers(ctx, plan.CustomerIDs); err != nil {
		return err
	}
	if len(plan.AccountRefs) > 0 {
		lockedAccounts, err := repository.siteFenceAccountRefs(ctx, plan.accountIDs(), true)
		if err != nil {
			return err
		}
		if !equalSiteFenceAccountRefs(plan.AccountRefs, lockedAccounts) {
			return ErrSiteRunConfigChanged
		}
	}
	verifiedPlan, err := repository.buildSiteFencePlan(ctx, site.ID)
	if err != nil {
		return err
	}
	if !equalSiteFencePlans(plan, verifiedPlan) {
		return ErrSiteRunConfigChanged
	}
	runIDs := plan.runIDs()
	runs, err := repository.lockSiteFenceRuns(ctx, runIDs)
	if err != nil {
		return err
	}
	if !equalCollectionRunIDs(plan.Runs, runs) {
		return ErrSiteRunConfigChanged
	}
	windowsByRun := make(map[int64][]CollectionRunWindow, len(runs))
	if len(runIDs) > 0 {
		var windows []CollectionRunWindow
		if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("run_id IN ?", runIDs).Order("run_id ASC, hour_ts ASC, id ASC").Find(&windows).Error; err != nil {
			return err
		}
		for _, window := range windows {
			windowsByRun[window.RunID] = append(windowsByRun[window.RunID], window)
		}
	}
	for index := range runs {
		run := &runs[index]
		params, marshalErr := common.Marshal(map[string]any{
			"site_id":                 strconv.FormatInt(site.ID, 10),
			"expected_config_version": run.SiteConfigVersion,
			"actual_config_version":   newVersion,
		})
		if marshalErr != nil {
			return fmt.Errorf("encode site fence error params: %w", marshalErr)
		}
		if err := repository.FailLockedCollectionRun(ctx, run, windowsByRun[run.ID], CollectionRunFailure{
			Code: constant.CodeSiteConfigChanged, Params: params,
		}, now); err != nil {
			return err
		}
	}
	if err := repository.recalculateSiteFenceBackfillStatuses(ctx, plan, now); err != nil {
		return err
	}
	site.ConfigVersion = newVersion
	site.UpdatedAt = now
	return repository.db.WithContext(ctx).Model(&Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"config_version": newVersion, "updated_at": now}).Error
}

type siteFenceAccountRef struct {
	ID         int64 `gorm:"column:id"`
	SiteID     int64 `gorm:"column:site_id"`
	CustomerID int64 `gorm:"column:customer_id"`
}

type siteFencePlan struct {
	Runs        []CollectionRun
	AccountRefs []siteFenceAccountRef
	CustomerIDs []int64
}

func (plan siteFencePlan) runIDs() []int64 {
	ids := make([]int64, len(plan.Runs))
	for index := range plan.Runs {
		ids[index] = plan.Runs[index].ID
	}
	return ids
}

func (plan siteFencePlan) accountIDs() []int64 {
	ids := make([]int64, len(plan.AccountRefs))
	for index := range plan.AccountRefs {
		ids[index] = plan.AccountRefs[index].ID
	}
	return ids
}

func (repository *SiteRepository) buildSiteFencePlan(ctx context.Context, siteID int64) (siteFencePlan, error) {
	if siteID <= 0 {
		return siteFencePlan{}, ErrCollectionRunContract
	}
	var parentIDs []int64
	if err := repository.db.WithContext(ctx).Raw(`
SELECT candidate.id FROM (
  SELECT r.id
  FROM collection_run r
  WHERE r.status IN ('pending','running')
    AND (r.site_id = ? OR (r.target_type = 'site' AND r.target_id = ?)
         OR EXISTS (SELECT 1 FROM collection_run_window rw WHERE rw.run_id = r.id AND rw.site_id = ?))
  UNION
  SELECT r.id
  FROM collection_run r
  JOIN account a ON r.target_type = 'account' AND r.target_id = a.id
  WHERE r.status IN ('pending','running') AND r.task_type = ? AND a.site_id = ?
  UNION
  SELECT r.id
  FROM collection_run r
  WHERE r.status IN ('pending','running') AND r.task_type = ? AND r.target_type = 'customer'
    AND EXISTS (SELECT 1 FROM account a WHERE a.customer_id = r.target_id AND a.site_id = ?)
) candidate
ORDER BY candidate.id ASC`, siteID, siteID, siteID, constant.TaskTypeAccountRebuild, siteID,
		constant.TaskTypeCustomerRebuild, siteID).Scan(&parentIDs).Error; err != nil {
		return siteFencePlan{}, err
	}
	plan := siteFencePlan{Runs: []CollectionRun{}, AccountRefs: []siteFenceAccountRef{}, CustomerIDs: []int64{}}
	if len(parentIDs) == 0 {
		return plan, nil
	}
	if err := repository.db.WithContext(ctx).Where("id IN ?", parentIDs).Order("id ASC").Find(&plan.Runs).Error; err != nil {
		return siteFencePlan{}, err
	}
	if len(plan.Runs) != len(parentIDs) {
		return siteFencePlan{}, ErrSiteRunConfigChanged
	}
	accountTargetIDs := make([]int64, 0)
	customerRunIDs := make([]int64, 0)
	for _, run := range plan.Runs {
		switch run.TargetType {
		case "site":
			if run.TargetID != siteID {
				return siteFencePlan{}, ErrCollectionRunContract
			}
		case "account":
			if run.TaskType != constant.TaskTypeAccountRebuild {
				return siteFencePlan{}, ErrCollectionRunContract
			}
			accountTargetIDs = appendUniqueSortedID(accountTargetIDs, run.TargetID)
		case "customer":
			if run.TaskType != constant.TaskTypeCustomerRebuild {
				return siteFencePlan{}, ErrCollectionRunContract
			}
			plan.CustomerIDs = appendUniqueSortedID(plan.CustomerIDs, run.TargetID)
			customerRunIDs = appendUniqueSortedID(customerRunIDs, run.TargetID)
		default:
			return siteFencePlan{}, ErrCollectionRunContract
		}
	}
	if len(accountTargetIDs) == 0 && len(customerRunIDs) == 0 {
		return plan, nil
	}
	query := repository.db.WithContext(ctx).Table("account").Select("id, site_id, customer_id")
	switch {
	case len(accountTargetIDs) > 0 && len(customerRunIDs) > 0:
		query = query.Where("id IN ? OR (customer_id IN ? AND site_id = ?)", accountTargetIDs, customerRunIDs, siteID)
	case len(accountTargetIDs) > 0:
		query = query.Where("id IN ?", accountTargetIDs)
	default:
		query = query.Where("customer_id IN ? AND site_id = ?", customerRunIDs, siteID)
	}
	if err := query.Order("id ASC").Find(&plan.AccountRefs).Error; err != nil {
		return siteFencePlan{}, err
	}
	foundTargets := make(map[int64]struct{}, len(accountTargetIDs))
	for _, account := range plan.AccountRefs {
		if account.SiteID != siteID {
			return siteFencePlan{}, ErrCollectionRunContract
		}
		plan.CustomerIDs = appendUniqueSortedID(plan.CustomerIDs, account.CustomerID)
		foundTargets[account.ID] = struct{}{}
	}
	for _, accountID := range accountTargetIDs {
		if _, found := foundTargets[accountID]; !found {
			return siteFencePlan{}, ErrCollectionRunContract
		}
	}
	return plan, nil
}

func (repository *SiteRepository) siteFenceAccountRefs(
	ctx context.Context,
	accountIDs []int64,
	lock bool,
) ([]siteFenceAccountRef, error) {
	if len(accountIDs) == 0 {
		return []siteFenceAccountRef{}, nil
	}
	query := repository.db.WithContext(ctx).Table("account").Select("id, site_id, customer_id").Where("id IN ?", accountIDs)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var accounts []siteFenceAccountRef
	if err := query.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (repository *SiteRepository) lockSiteFenceCustomers(ctx context.Context, customerIDs []int64) error {
	if len(customerIDs) == 0 {
		return nil
	}
	var locked []struct {
		ID int64 `gorm:"column:id"`
	}
	if err := repository.db.WithContext(ctx).Table("customer").Select("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", customerIDs).
		Order("id ASC").Find(&locked).Error; err != nil {
		return err
	}
	if len(locked) != len(customerIDs) {
		return ErrCollectionRunContract
	}
	for index, customer := range locked {
		if customer.ID != customerIDs[index] {
			return ErrCollectionRunContract
		}
	}
	return nil
}

func (repository *SiteRepository) lockSiteFenceRuns(ctx context.Context, runIDs []int64) ([]CollectionRun, error) {
	if len(runIDs) == 0 {
		return []CollectionRun{}, nil
	}
	var runs []CollectionRun
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ? AND status IN ('pending','running')", runIDs).
		Order("id ASC").Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

func appendUniqueSortedID(ids []int64, id int64) []int64 {
	if id <= 0 {
		return ids
	}
	position := 0
	for position < len(ids) && ids[position] < id {
		position++
	}
	if position < len(ids) && ids[position] == id {
		return ids
	}
	ids = append(ids, 0)
	copy(ids[position+1:], ids[position:])
	ids[position] = id
	return ids
}

func equalSiteFencePlans(first, second siteFencePlan) bool {
	if !equalCollectionRunIDs(first.Runs, second.Runs) || !equalSiteFenceAccountRefs(first.AccountRefs, second.AccountRefs) ||
		len(first.CustomerIDs) != len(second.CustomerIDs) {
		return false
	}
	for index := range first.CustomerIDs {
		if first.CustomerIDs[index] != second.CustomerIDs[index] {
			return false
		}
	}
	return true
}

func equalSiteFenceAccountRefs(first, second []siteFenceAccountRef) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func equalCollectionRunIDs(first, second []CollectionRun) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].ID != second[index].ID {
			return false
		}
	}
	return true
}

type siteFenceStatusRow struct {
	ID       int64 `gorm:"column:id"`
	Priority int   `gorm:"column:priority"`
}

func (repository *SiteRepository) recalculateSiteFenceBackfillStatuses(ctx context.Context, plan siteFencePlan, now int64) error {
	if len(plan.AccountRefs) > 0 {
		var accountRows []siteFenceStatusRow
		if err := repository.db.WithContext(ctx).Raw(`SELECT a.id,
COALESCE((SELECT CASE r.status WHEN 'running' THEN 3 WHEN 'pending' THEN 2 WHEN 'failed' THEN 1 WHEN 'success' THEN 0 ELSE -1 END
          FROM collection_run r
          WHERE r.target_type = 'account' AND r.target_id = a.id AND r.task_type = ?
          ORDER BY r.id DESC LIMIT 1), 0) AS priority
FROM account a WHERE a.id IN ? ORDER BY a.id ASC`, constant.TaskTypeAccountRebuild, plan.accountIDs()).Scan(&accountRows).Error; err != nil {
			return err
		}
		if len(accountRows) != len(plan.AccountRefs) {
			return ErrSiteRunConfigChanged
		}
		if err := repository.batchUpdateSiteFenceStatuses(ctx, "account", accountRows, now); err != nil {
			return err
		}
	}
	if len(plan.CustomerIDs) == 0 {
		return nil
	}
	var customerRows []siteFenceStatusRow
	if err := repository.db.WithContext(ctx).Raw(`SELECT c.id, GREATEST(
  COALESCE((SELECT CASE r.status WHEN 'running' THEN 3 WHEN 'pending' THEN 2 WHEN 'failed' THEN 1 WHEN 'success' THEN 0 ELSE -1 END
            FROM collection_run r
            WHERE r.target_type = 'customer' AND r.target_id = c.id AND r.task_type = ?
            ORDER BY r.id DESC LIMIT 1), 0),
  COALESCE((SELECT MAX(CASE a.statistics_backfill_status
                       WHEN 'running' THEN 3 WHEN 'pending' THEN 2 WHEN 'failed' THEN 1 WHEN 'none' THEN 0 ELSE -1 END)
            FROM account a WHERE a.customer_id = c.id), 0)
) AS priority
FROM customer c WHERE c.id IN ? ORDER BY c.id ASC`, constant.TaskTypeCustomerRebuild, plan.CustomerIDs).Scan(&customerRows).Error; err != nil {
		return err
	}
	if len(customerRows) != len(plan.CustomerIDs) {
		return ErrSiteRunConfigChanged
	}
	return repository.batchUpdateSiteFenceStatuses(ctx, "customer", customerRows, now)
}

func (repository *SiteRepository) batchUpdateSiteFenceStatuses(ctx context.Context, table string, rows []siteFenceStatusRow, now int64) error {
	groups := map[int][]int64{0: {}, 1: {}, 2: {}, 3: {}}
	for _, row := range rows {
		if _, valid := groups[row.Priority]; !valid {
			return ErrCollectionRunContract
		}
		groups[row.Priority] = append(groups[row.Priority], row.ID)
	}
	statuses := []string{"none", "failed", "pending", "running"}
	for priority, status := range statuses {
		if len(groups[priority]) == 0 {
			continue
		}
		if err := repository.db.WithContext(ctx).Table(table).Where("id IN ?", groups[priority]).
			Updates(map[string]any{"statistics_backfill_status": status, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repository *SiteRepository) CreateOrGetRun(ctx context.Context, run *CollectionRun) (CollectionRun, bool, error) {
	if run == nil || ValidateCollectionRunForCreate(*run) != nil {
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	err := repository.db.WithContext(ctx).Create(run).Error
	if err == nil {
		return *run, false, nil
	}
	if !IsDuplicateKey(err) || run.ActiveKey == nil {
		return CollectionRun{}, false, err
	}
	var existing CollectionRun
	if findErr := repository.db.WithContext(ctx).Where("active_key = ?", *run.ActiveKey).First(&existing).Error; findErr != nil {
		return CollectionRun{}, false, err
	}
	if !sameActiveCollectionRun(existing, *run) {
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	return existing, true, nil
}

func sameActiveCollectionRun(existing, requested CollectionRun) bool {
	if existing.ActiveKey == nil || requested.ActiveKey == nil || *existing.ActiveKey != *requested.ActiveKey ||
		(existing.Status != "pending" && existing.Status != "running") ||
		existing.TaskType != requested.TaskType || existing.TargetType != requested.TargetType ||
		existing.TargetID != requested.TargetID || existing.SiteConfigVersion != requested.SiteConfigVersion ||
		!equalInt64Pointers(existing.SiteID, requested.SiteID) ||
		!equalInt64Pointers(existing.StartTimestamp, requested.StartTimestamp) ||
		!equalInt64Pointers(existing.EndTimestamp, requested.EndTimestamp) {
		return false
	}
	expectedKey, err := CollectionRunActiveKey(
		existing.TaskType, existing.TargetType, existing.TargetID, existing.StartTimestamp, existing.EndTimestamp,
	)
	return err == nil && expectedKey == *existing.ActiveKey
}

func equalInt64Pointers(first, second *int64) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

func (repository *SiteRepository) FindCollectionRunByID(ctx context.Context, id int64) (CollectionRun, error) {
	var run CollectionRun
	err := collectionRunQueryWithUnavailable(repository.db.WithContext(ctx).Model(&CollectionRun{})).
		Where("collection_run.id = ?", id).First(&run).Error
	return run, err
}

func (repository *SiteRepository) ListCollectionRuns(ctx context.Context, filter CollectionRunFilter) ([]CollectionRun, int64, error) {
	query := repository.db.WithContext(ctx).Model(&CollectionRun{})
	if filter.SiteID != nil {
		query = query.Where("site_id = ?", *filter.SiteID)
	}
	if filter.TaskType != "" {
		query = query.Where("task_type = ?", filter.TaskType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sortColumns := map[string]string{
		"created_at": "created_at",
		"started_at": "started_at",
		"priority":   "priority",
		"status":     "status",
	}
	column, exists := sortColumns[filter.SortBy]
	if !exists {
		return nil, 0, fmt.Errorf("unsupported collection run sort %q", filter.SortBy)
	}
	order := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		order = "ASC"
	}
	var runs []CollectionRun
	err := collectionRunQueryWithUnavailable(query).Order(column + " " + order).Order("id DESC").
		Offset(filter.Offset).Limit(filter.Limit).Find(&runs).Error
	return runs, total, err
}

func (repository *SiteRepository) ListCollectionRunWindows(ctx context.Context, filter CollectionRunWindowFilter) ([]CollectionRunWindowSnapshot, int64, error) {
	countQuery := repository.db.WithContext(ctx).Model(&CollectionRunWindow{}).Where("run_id = ?", filter.RunID)
	if filter.Status != "" {
		countQuery = countQuery.Where("status = ?", filter.Status)
	}
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := repository.db.WithContext(ctx).Table("collection_run_window AS rw").
		Select("rw.*, COALESCE(cw.status, 'missing') AS fact_status, cw.verified_at").
		Joins("LEFT JOIN collection_window AS cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts").
		Where("rw.run_id = ?", filter.RunID)
	if filter.Status != "" {
		query = query.Where("rw.status = ?", filter.Status)
	}
	var windows []CollectionRunWindowSnapshot
	err := query.Order("rw.hour_ts ASC, rw.id ASC").Offset(filter.Offset).Limit(filter.Limit).Scan(&windows).Error
	return windows, total, err
}

func (repository *SiteRepository) LatestBackfillRun(ctx context.Context, siteID int64) (CollectionRun, error) {
	var run CollectionRun
	err := collectionRunQueryWithUnavailable(repository.db.WithContext(ctx).Model(&CollectionRun{})).
		Where("site_id = ? AND task_type = ?", siteID, constant.TaskTypeUsageBackfill).
		Order("created_at DESC, id DESC").First(&run).Error
	return run, err
}

func (repository *SiteRepository) LatestBackfillRuns(ctx context.Context, siteIDs []int64) (map[int64]CollectionRun, error) {
	result := make(map[int64]CollectionRun, len(siteIDs))
	if len(siteIDs) == 0 {
		return result, nil
	}
	var runs []CollectionRun
	err := collectionRunQueryWithUnavailable(repository.db.WithContext(ctx).Model(&CollectionRun{})).
		Where("collection_run.site_id IN ? AND collection_run.task_type = ?", siteIDs, constant.TaskTypeUsageBackfill).
		Where(`NOT EXISTS (
  SELECT 1 FROM collection_run AS newer
  WHERE newer.site_id = collection_run.site_id AND newer.task_type = collection_run.task_type
    AND (newer.created_at > collection_run.created_at OR
      (newer.created_at = collection_run.created_at AND newer.id > collection_run.id))
)`).Find(&runs).Error
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.SiteID != nil {
			result[*run.SiteID] = run
		}
	}
	return result, nil
}

func (repository *SiteRepository) LatestActiveRecoveryRun(ctx context.Context, siteID int64) (CollectionRun, error) {
	var run CollectionRun
	err := collectionRunQueryWithUnavailable(repository.db.WithContext(ctx).Model(&CollectionRun{})).
		Where("site_id = ? AND task_type = ? AND trigger_type = ? AND status IN ('pending','running')",
			siteID, constant.TaskTypeUsageBackfill, constant.CollectionTriggerRecovery).
		Order("created_at DESC, id DESC").First(&run).Error
	return run, err
}

func (repository *SiteRepository) LatestCompleteHour(ctx context.Context, siteID int64) (*int64, error) {
	var value sql.NullInt64
	err := repository.db.WithContext(ctx).Raw(
		"SELECT last_complete_hour FROM collection_cursor WHERE site_id = ? AND cursor_key = 'usage'", siteID,
	).Scan(&value).Error
	if err != nil || !value.Valid {
		return nil, err
	}
	result := value.Int64
	return &result, nil
}

func (repository *SiteRepository) OpenMonitoringPause(ctx context.Context, siteID, startMinute, now int64) error {
	pause := SiteMonitoringPause{
		SiteID: siteID, StartMinuteTS: startMinute, Reason: "management_disabled", CreatedAt: now,
	}
	return repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "start_minute_ts"}},
		DoUpdates: clause.Assignments(map[string]any{
			"end_minute_ts": nil,
			"reason":        "management_disabled",
			"created_at":    now,
		}),
	}).Create(&pause).Error
}

func (repository *SiteRepository) CloseMonitoringPause(ctx context.Context, siteID, endMinute int64) error {
	return repository.db.WithContext(ctx).Model(&SiteMonitoringPause{}).
		Where("site_id = ? AND end_minute_ts IS NULL", siteID).
		Update("end_minute_ts", endMinute).Error
}

func (repository *SiteRepository) HasDeleteDependencies(ctx context.Context, siteID int64) (bool, error) {
	blockers, err := repository.SiteDeleteBlockers(ctx, siteID)
	return len(blockers) > 0, err
}

type SiteDeleteDependencyType string

const (
	SiteDeleteDependencyAccount            SiteDeleteDependencyType = "account"
	SiteDeleteDependencyUsageFact          SiteDeleteDependencyType = "usage_fact"
	SiteDeleteDependencyBusinessStatistics SiteDeleteDependencyType = "business_statistics"
	SiteDeleteDependencyResourceHistory    SiteDeleteDependencyType = "resource_history"
	SiteDeleteDependencyActiveCollection   SiteDeleteDependencyType = "active_collection"
	SiteDeleteDependencyActiveAlert        SiteDeleteDependencyType = "active_alert"
	SiteDeleteDependencyAlertHistory       SiteDeleteDependencyType = "alert_history"
	SiteDeleteDependencyDataMaintenance    SiteDeleteDependencyType = "data_maintenance"
)

func (repository *SiteRepository) SiteDeleteBlockers(ctx context.Context, siteID int64) ([]SiteDeleteDependencyType, error) {
	if siteID <= 0 {
		return nil, fmt.Errorf("invalid site delete target")
	}
	checks := []struct {
		name  SiteDeleteDependencyType
		query string
		args  []any
	}{
		{name: SiteDeleteDependencyAccount, query: `SELECT EXISTS(SELECT 1 FROM account WHERE site_id = ? LIMIT 1)`, args: []any{siteID}},
		{name: SiteDeleteDependencyUsageFact, query: `SELECT EXISTS(
SELECT 1 FROM usage_fact_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM usage_fact_daily WHERE site_id = ? LIMIT 1)`, args: []any{siteID, siteID}},
		{name: SiteDeleteDependencyBusinessStatistics, query: `SELECT EXISTS(
SELECT 1 FROM customer_stat_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM customer_stat_daily WHERE site_id = ?
UNION ALL SELECT 1 FROM site_stat_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM site_stat_daily WHERE site_id = ?
UNION ALL SELECT 1 FROM model_stat_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM model_stat_daily WHERE site_id = ?
UNION ALL SELECT 1 FROM channel_stat_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM channel_stat_daily WHERE site_id = ?
UNION ALL SELECT 1 FROM account_stat_hourly s JOIN account a ON a.id = s.account_id WHERE a.site_id = ?
UNION ALL SELECT 1 FROM account_stat_daily s JOIN account a ON a.id = s.account_id WHERE a.site_id = ? LIMIT 1)`,
			args: []any{siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID}},
		{name: SiteDeleteDependencyResourceHistory, query: `SELECT EXISTS(
SELECT 1 FROM site_instance_status_minutely WHERE site_id = ?
UNION ALL SELECT 1 FROM site_instance_status_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM site_instance_status_daily WHERE site_id = ?
UNION ALL SELECT 1 FROM site_status_minutely WHERE site_id = ?
UNION ALL SELECT 1 FROM site_status_hourly WHERE site_id = ?
UNION ALL SELECT 1 FROM site_status_daily WHERE site_id = ? LIMIT 1)`,
			args: []any{siteID, siteID, siteID, siteID, siteID, siteID}},
		{name: SiteDeleteDependencyActiveCollection, query: `SELECT EXISTS(
SELECT 1 FROM collection_run
 WHERE status IN ('pending','running') AND (site_id = ? OR (target_type = 'site' AND target_id = ?))
UNION ALL
SELECT 1 FROM collection_run_window rw JOIN collection_run r ON r.id = rw.run_id
 WHERE rw.site_id = ? AND r.status IN ('pending','running') LIMIT 1)`, args: []any{siteID, siteID, siteID}},
		{name: SiteDeleteDependencyActiveAlert, query: `SELECT EXISTS(
SELECT 1 FROM alert_event WHERE site_id = ? AND status <> 'resolved' LIMIT 1)`, args: []any{siteID}},
		{name: SiteDeleteDependencyAlertHistory, query: `SELECT EXISTS(
SELECT 1 FROM alert_event e
WHERE e.site_id = ? AND e.status = 'resolved'
  AND (e.first_fired_at IS NOT NULL OR e.last_fired_at IS NOT NULL OR
       EXISTS (SELECT 1 FROM alert_delivery d WHERE d.alert_event_id = e.id))
LIMIT 1)`, args: []any{siteID}},
		{name: SiteDeleteDependencyDataMaintenance, query: `SELECT EXISTS(
SELECT 1 FROM data_maintenance_state WHERE site_id = ? AND status IN ('pending','running') LIMIT 1)`, args: []any{siteID}},
	}
	blockers := make([]SiteDeleteDependencyType, 0, len(checks))
	for _, check := range checks {
		var exists bool
		if err := repository.db.WithContext(ctx).Raw(check.query, check.args...).Row().Scan(&exists); err != nil {
			return nil, fmt.Errorf("check site delete blocker %s: %w", check.name, err)
		}
		if exists {
			blockers = append(blockers, check.name)
		}
	}
	return blockers, nil
}

func (repository *SiteRepository) DeleteOwnedMetadata(ctx context.Context, siteID int64) error {
	if siteID <= 0 {
		return fmt.Errorf("invalid site metadata delete target")
	}
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "empty resolved alert events", query: `DELETE e FROM alert_event e
LEFT JOIN alert_delivery d ON d.alert_event_id = e.id
WHERE e.site_id = ? AND e.status = 'resolved'
  AND e.first_fired_at IS NULL AND e.last_fired_at IS NULL AND d.id IS NULL`, args: []any{siteID}},
		{name: "site alert overrides", query: `DELETE r FROM alert_rule r
LEFT JOIN alert_event e ON e.rule_id = r.id
WHERE r.scope_type = 'site' AND r.scope_id = ? AND e.id IS NULL`, args: []any{siteID}},
		{name: "terminal run windows", query: `DELETE rw FROM collection_run_window rw
JOIN collection_run r ON r.id = rw.run_id
WHERE rw.site_id = ? AND r.status IN ('success','failed')`, args: []any{siteID}},
		{name: "collection windows", query: "DELETE FROM collection_window WHERE site_id = ?", args: []any{siteID}},
		{name: "collection cursors", query: "DELETE FROM collection_cursor WHERE site_id = ?", args: []any{siteID}},
		{name: "terminal collection runs", query: `DELETE FROM collection_run
WHERE status IN ('success','failed') AND (site_id = ? OR (target_type = 'site' AND target_id = ?))`, args: []any{siteID, siteID}},
		{name: "site aggregation locks", query: "DELETE FROM aggregation_bucket_lock WHERE lock_key LIKE ?", args: []any{"stats:site:" + strconv.FormatInt(siteID, 10) + ":%"}},
		{name: "terminal data maintenance", query: "DELETE FROM data_maintenance_state WHERE site_id = ? AND status IN ('complete','failed')", args: []any{siteID}},
		{name: "instance lifecycle metadata", query: "DELETE FROM site_instance_lifecycle WHERE site_id = ?", args: []any{siteID}},
		{name: "current instances", query: "DELETE FROM site_instance WHERE site_id = ?", args: []any{siteID}},
		{name: "monitoring pauses", query: "DELETE FROM site_monitoring_pause WHERE site_id = ?", args: []any{siteID}},
		{name: "channels", query: "DELETE FROM site_channel WHERE site_id = ?", args: []any{siteID}},
		{name: "capabilities", query: "DELETE FROM site_capability WHERE site_id = ?", args: []any{siteID}},
	}
	for _, statement := range statements {
		if err := repository.db.WithContext(ctx).Exec(statement.query, statement.args...).Error; err != nil {
			return fmt.Errorf("delete site %s: %w", statement.name, err)
		}
	}
	return nil
}

func (repository *SiteRepository) UpdateCurrentStatus(ctx context.Context, site *Site) error {
	return repository.db.WithContext(ctx).Save(site).Error
}

func collectionRunQueryWithUnavailable(query *gorm.DB) *gorm.DB {
	return query.Select(`collection_run.*,
  (SELECT COUNT(*) FROM collection_run_window unavailable
   WHERE unavailable.run_id = collection_run.id AND unavailable.status = 'unavailable') AS unavailable_windows`)
}
