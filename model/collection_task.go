package model

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
)

var (
	ErrCollectionTaskUnavailable = errors.New("no collection task is currently claimable")
	ErrCollectionTaskClaimLost   = errors.New("collection task claim is no longer active")
)

const (
	CollectionTaskStatusPending     = "pending"
	CollectionTaskStatusRunning     = "running"
	CollectionTaskStatusSuccess     = "success"
	CollectionTaskStatusFailed      = "failed"
	CollectionTaskStatusUnavailable = "unavailable"

	CollectionTaskLeaseLostCode       = "WORKER_LEASE_LOST"
	CollectionTaskExecutionFailedCode = "WORKER_EXECUTION_FAILED"
)

type CollectionTaskRepository struct {
	db *gorm.DB
}

func NewCollectionTaskRepository(db *gorm.DB) *CollectionTaskRepository {
	return &CollectionTaskRepository{db: db}
}

type SiteTaskEnqueueRequest struct {
	SiteID                int64
	ExpectedConfigVersion int
	TaskType              string
	TriggerType           string
	StartTimestamp        *int64
	EndTimestamp          *int64
	Scope                 []byte
	Priority              int
	RequestID             string
	Now                   int64
	Mode                  SiteWindowRunCreateMode
}

// ScheduledSiteTaskDiagnosticRequest records the exceptional state transition
// observed by an in-memory scheduled fast task. It is intentionally terminal:
// the task has already run and must never be picked up by the durable executor.
type ScheduledSiteTaskDiagnosticRequest struct {
	SiteID    int64
	TaskType  string
	RequestID string
	ErrorCode string
	Now       int64
}

func (repository *CollectionTaskRepository) EnqueueSiteTask(
	ctx context.Context,
	request SiteTaskEnqueueRequest,
) (CollectionRun, bool, error) {
	if repository == nil || repository.db == nil || request.SiteID <= 0 || request.ExpectedConfigVersion <= 0 ||
		request.Now <= 0 || !constant.ValidCollectionTaskType(request.TaskType) {
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	if existing, found, err := repository.findLogicalSiteTask(ctx, request); err != nil {
		return CollectionRun{}, false, err
	} else if found {
		return existing, true, nil
	}
	if constant.CollectionTaskWindowed(request.TaskType) {
		if request.StartTimestamp == nil || request.EndTimestamp == nil {
			return CollectionRun{}, false, ErrCollectionRunContract
		}
		mode := request.Mode
		if mode == "" {
			mode = SiteWindowRunSchedule
		}
		result, err := NewSiteRepository(repository.db).CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
			SiteID: request.SiteID, ExpectedConfigVersion: request.ExpectedConfigVersion,
			TaskType: request.TaskType, TriggerType: request.TriggerType,
			StartTimestamp: *request.StartTimestamp, EndTimestamp: *request.EndTimestamp,
			Scope: request.Scope, Priority: request.Priority, RequestID: request.RequestID,
			Now: request.Now, Mode: mode,
		})
		if err != nil {
			return CollectionRun{}, false, err
		}
		if len(result.Runs) != 1 {
			return CollectionRun{}, false, ErrCollectionRunContract
		}
		return result.Runs[0].Run, result.Runs[0].Deduplicated, nil
	}
	var run CollectionRun
	deduplicated := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, request.SiteID).Error; err != nil {
			return err
		}
		if site.ConfigVersion != request.ExpectedConfigVersion || !siteAllowsTask(site, request.TaskType) {
			return ErrSiteRunConfigChanged
		}
		created, err := NewSiteCollectionRun(site, SiteRunSpec{
			TaskType: request.TaskType, TriggerType: request.TriggerType, Scope: request.Scope,
			Priority: request.Priority, RequestID: request.RequestID, Now: request.Now,
		})
		if err != nil {
			return err
		}
		run, deduplicated, err = NewSiteRepository(tx).CreateOrGetRun(ctx, &created)
		return err
	})
	return run, deduplicated, err
}

// EnqueueScheduledSiteWindowTask preserves every uncovered range produced by
// schedule mode. A weekly validation can intentionally overlap a still-active
// daily validation; CreateSiteWindowRun then returns multiple non-overlapping
// gaps, which cannot be represented by EnqueueSiteTask's single-run result.
func (repository *CollectionTaskRepository) EnqueueScheduledSiteWindowTask(
	ctx context.Context,
	request SiteTaskEnqueueRequest,
) ([]CollectionRun, error) {
	if repository == nil || repository.db == nil || request.SiteID <= 0 || request.ExpectedConfigVersion <= 0 ||
		request.Now <= 0 || request.Mode != SiteWindowRunSchedule || request.TriggerType != constant.CollectionTriggerSchedule ||
		!constant.CollectionTaskWindowed(request.TaskType) || request.StartTimestamp == nil || request.EndTimestamp == nil {
		return nil, ErrCollectionRunContract
	}
	result, err := NewSiteRepository(repository.db).CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: request.SiteID, ExpectedConfigVersion: request.ExpectedConfigVersion,
		TaskType: request.TaskType, TriggerType: request.TriggerType,
		StartTimestamp: *request.StartTimestamp, EndTimestamp: *request.EndTimestamp,
		Scope: request.Scope, Priority: request.Priority, RequestID: request.RequestID,
		Now: request.Now, Mode: SiteWindowRunSchedule,
	})
	if err != nil {
		return nil, err
	}
	runs := make([]CollectionRun, len(result.Runs))
	for index := range result.Runs {
		runs[index] = result.Runs[index].Run
	}
	return runs, nil
}

func (repository *CollectionTaskRepository) findLogicalSiteTask(
	ctx context.Context,
	request SiteTaskEnqueueRequest,
) (CollectionRun, bool, error) {
	query := repository.db.WithContext(ctx).Where(
		"site_id = ? AND site_config_version = ? AND task_type = ? AND target_type = 'site' AND target_id = ? AND trigger_type = ? AND created_request_id = ?",
		request.SiteID, request.ExpectedConfigVersion, request.TaskType, request.SiteID, request.TriggerType, request.RequestID,
	)
	if request.StartTimestamp == nil {
		query = query.Where("start_timestamp IS NULL AND end_timestamp IS NULL")
	} else if request.EndTimestamp != nil {
		query = query.Where("start_timestamp = ? AND end_timestamp = ?", *request.StartTimestamp, *request.EndTimestamp)
	} else {
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	var run CollectionRun
	err := query.Order("id DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CollectionRun{}, false, nil
	}
	if err != nil {
		return CollectionRun{}, false, err
	}
	return run, true, nil
}

func (repository *CollectionTaskRepository) ListSitesForScheduling(ctx context.Context) ([]Site, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("collection task repository is required")
	}
	var sites []Site
	err := repository.db.WithContext(ctx).Order("id ASC").Find(&sites).Error
	return sites, err
}

// FindSiteForScheduling loads a current site snapshot for scheduler-owned
// in-memory work. Unlike a runnable task snapshot, this remains available
// after a fast task has transitioned the site offline or authorization expired.
func (repository *CollectionTaskRepository) FindSiteForScheduling(ctx context.Context, siteID int64) (Site, error) {
	if repository == nil || repository.db == nil || siteID <= 0 {
		return Site{}, ErrCollectionRunContract
	}
	return NewSiteRepository(repository.db).FindByID(ctx, siteID)
}

func (repository *CollectionTaskRepository) RecordScheduledSiteTaskDiagnostic(
	ctx context.Context,
	request ScheduledSiteTaskDiagnosticRequest,
) (CollectionRun, bool, error) {
	if repository == nil || repository.db == nil || request.SiteID <= 0 || request.Now <= 0 ||
		request.ErrorCode == "" || !validCollectionRequestID(request.RequestID) {
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	switch request.TaskType {
	case constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot:
	default:
		return CollectionRun{}, false, ErrCollectionRunContract
	}
	var result CollectionRun
	deduplicated := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, request.SiteID).Error; err != nil {
			return err
		}
		var existing CollectionRun
		err := tx.Where(
			"site_id = ? AND task_type = ? AND trigger_type = ? AND created_request_id = ?",
			request.SiteID, request.TaskType, constant.CollectionTriggerSchedule, request.RequestID,
		).Order("id DESC").First(&existing).Error
		if err == nil {
			result = existing
			deduplicated = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		run, err := NewSiteCollectionRun(site, SiteRunSpec{
			TaskType: request.TaskType, TriggerType: constant.CollectionTriggerSchedule,
			RequestID: request.RequestID, Now: request.Now,
		})
		if err != nil {
			return err
		}
		finished := request.Now
		run.Status = CollectionTaskStatusFailed
		run.ActiveKey = nil
		run.ErrorCode = request.ErrorCode
		run.FinishedAt = &finished
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		result = run
		return nil
	})
	return result, deduplicated, err
}

type CollectionTaskClaimOptions struct {
	TaskTypes []string
	Now       int64
	RequestID string
	MaxWindow int
	ScanLimit int
}

type CollectionTaskClaim struct {
	Run       CollectionRun
	Windows   []CollectionRunWindow
	RequestID string
}

func (repository *CollectionTaskRepository) ClaimNext(
	ctx context.Context,
	options CollectionTaskClaimOptions,
) (CollectionTaskClaim, error) {
	if repository == nil || repository.db == nil || options.Now <= 0 || !validCollectionRequestID(options.RequestID) ||
		len(options.TaskTypes) == 0 {
		return CollectionTaskClaim{}, ErrCollectionRunContract
	}
	for _, taskType := range options.TaskTypes {
		if !constant.ValidCollectionTaskType(taskType) {
			return CollectionTaskClaim{}, ErrCollectionRunContract
		}
	}
	if options.MaxWindow <= 0 || options.MaxWindow > 24 {
		options.MaxWindow = 24
	}
	if options.ScanLimit <= 0 || options.ScanLimit > 256 {
		options.ScanLimit = 64
	}
	var cursor *collectionTaskClaimCursor
	for {
		var candidates []CollectionRun
		query := repository.db.WithContext(ctx).
			Where("status = 'pending' AND windows_initialized_at IS NOT NULL AND next_attempt_at <= ? AND task_type IN ?", options.Now, options.TaskTypes).
			Order(collectionTaskClaimOrder).Limit(options.ScanLimit)
		if cursor != nil {
			query = query.Where(collectionTaskClaimAfterCursor,
				cursor.Priority, cursor.InitialStart, cursor.End, cursor.CreatedWithoutStart,
				cursor.NextAttemptAt, cursor.CreatedAt, cursor.ID,
			)
		}
		if err := query.Find(&candidates).Error; err != nil {
			return CollectionTaskClaim{}, err
		}
		for _, candidate := range candidates {
			claim, err := repository.claimCandidate(ctx, candidate, options)
			if err == nil {
				return claim, nil
			}
			if !errors.Is(err, ErrCollectionTaskUnavailable) && !errors.Is(err, ErrCollectionTaskClaimLost) {
				return CollectionTaskClaim{}, err
			}
		}
		if len(candidates) < options.ScanLimit {
			break
		}
		nextCursor := newCollectionTaskClaimCursor(candidates[len(candidates)-1])
		cursor = &nextCursor
	}
	return CollectionTaskClaim{}, ErrCollectionTaskUnavailable
}

// Normalize every sort component to a non-null ascending key so pagination can
// continue from an immutable page cursor even while earlier rows leave pending.
const collectionTaskClaimOrder = `0 - priority ASC,
CASE WHEN priority = 50 AND start_timestamp IS NOT NULL THEN start_timestamp ELSE -1 END ASC,
CASE WHEN priority <> 50 AND end_timestamp IS NOT NULL THEN 0 - end_timestamp ELSE 9223372036854775807 END ASC,
CASE WHEN start_timestamp IS NULL THEN created_at ELSE -1 END ASC,
next_attempt_at ASC, created_at ASC, id ASC`

const collectionTaskClaimAfterCursor = `(0 - priority,
CASE WHEN priority = 50 AND start_timestamp IS NOT NULL THEN start_timestamp ELSE -1 END,
CASE WHEN priority <> 50 AND end_timestamp IS NOT NULL THEN 0 - end_timestamp ELSE 9223372036854775807 END,
CASE WHEN start_timestamp IS NULL THEN created_at ELSE -1 END,
next_attempt_at, created_at, id) > (?, ?, ?, ?, ?, ?, ?)`

type collectionTaskClaimCursor struct {
	Priority            int64
	InitialStart        int64
	End                 int64
	CreatedWithoutStart int64
	NextAttemptAt       int64
	CreatedAt           int64
	ID                  int64
}

func newCollectionTaskClaimCursor(run CollectionRun) collectionTaskClaimCursor {
	cursor := collectionTaskClaimCursor{
		Priority: -int64(run.Priority), InitialStart: -1, End: int64(^uint64(0) >> 1),
		CreatedWithoutStart: -1, NextAttemptAt: run.NextAttemptAt, CreatedAt: run.CreatedAt, ID: run.ID,
	}
	if run.Priority == constant.CollectionPriorityInitialBackfill && run.StartTimestamp != nil {
		cursor.InitialStart = *run.StartTimestamp
	}
	if run.Priority != constant.CollectionPriorityInitialBackfill && run.EndTimestamp != nil {
		cursor.End = -*run.EndTimestamp
	}
	if run.StartTimestamp == nil {
		cursor.CreatedWithoutStart = run.CreatedAt
	}
	return cursor
}

func (repository *CollectionTaskRepository) claimCandidate(
	ctx context.Context,
	candidate CollectionRun,
	options CollectionTaskClaimOptions,
) (CollectionTaskClaim, error) {
	var claim CollectionTaskClaim
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := repository.resolveTaskScope(ctx, tx, candidate)
		if err != nil {
			return err
		}
		if err := repository.lockTaskScope(ctx, tx, candidate, scope, true); err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("id = ?", candidate.ID).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCollectionTaskUnavailable
			}
			return err
		}
		if run.Status != CollectionTaskStatusPending || run.WindowsInitializedAt == nil || run.NextAttemptAt > options.Now ||
			run.TaskType != candidate.TaskType || run.TargetType != candidate.TargetType || run.TargetID != candidate.TargetID ||
			run.LastRequestID == options.RequestID {
			return ErrCollectionTaskClaimLost
		}
		if err := repository.verifyTaskScopeLocked(ctx, tx, run, scope); err != nil {
			return err
		}
		busy, err := repository.scopeHasRunningTask(ctx, tx, run, scope)
		if err != nil {
			return err
		}
		if busy {
			return ErrCollectionTaskUnavailable
		}
		var windows []CollectionRunWindow
		if constant.CollectionTaskWindowed(run.TaskType) {
			if run.TotalWindows <= 0 {
				return ErrCollectionTaskUnavailable
			}
			windowOrder := "hour_ts DESC, id DESC"
			if run.Priority == constant.CollectionPriorityInitialBackfill {
				windowOrder = "hour_ts ASC, id ASC"
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("run_id = ? AND status = 'pending' AND COALESCE(next_retry_at, 0) <= ?", run.ID, options.Now).
				Order(windowOrder).Limit(options.MaxWindow).Find(&windows).Error; err != nil {
				return err
			}
			if len(windows) == 0 {
				var next *int64
				if err := tx.Model(&CollectionRunWindow{}).Select("MIN(next_retry_at)").
					Where("run_id = ? AND status = 'pending'", run.ID).Scan(&next).Error; err != nil {
					return err
				}
				if next != nil {
					if err := tx.Model(&CollectionRun{}).Where("id = ? AND status = 'pending'", run.ID).
						Update("next_attempt_at", *next).Error; err != nil {
						return err
					}
				}
				return ErrCollectionTaskUnavailable
			}
			ids := make([]int64, len(windows))
			for index := range windows {
				ids[index] = windows[index].ID
			}
			update := tx.Model(&CollectionRunWindow{}).Where("id IN ? AND status = 'pending'", ids).
				Updates(map[string]any{
					"status": CollectionTaskStatusRunning, "attempt_count": gorm.Expr("attempt_count + 1"),
					"next_retry_at": nil, "started_at": options.Now, "finished_at": nil, "updated_at": options.Now,
				})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != int64(len(ids)) {
				return ErrCollectionTaskClaimLost
			}
			if err := tx.Where("id IN ?", ids).Order(windowOrder).Find(&windows).Error; err != nil {
				return err
			}
		}
		parentUpdates := map[string]any{
			"status": CollectionTaskStatusRunning, "heartbeat_at": options.Now,
			"started_at":      gorm.Expr("COALESCE(started_at, ?)", options.Now),
			"last_request_id": options.RequestID, "updated_at": options.Now,
		}
		if !constant.CollectionTaskWindowed(run.TaskType) {
			parentUpdates["retry_count"] = gorm.Expr("retry_count + 1")
		}
		parentUpdate := tx.Model(&CollectionRun{}).
			Where("id = ? AND status = 'pending' AND last_request_id = ?", run.ID, run.LastRequestID).
			Updates(parentUpdates)
		if parentUpdate.Error != nil {
			return parentUpdate.Error
		}
		if parentUpdate.RowsAffected != 1 {
			return ErrCollectionTaskClaimLost
		}
		run.Status = CollectionTaskStatusRunning
		run.HeartbeatAt = int64Pointer(options.Now)
		run.LastRequestID = options.RequestID
		if !constant.CollectionTaskWindowed(run.TaskType) {
			run.RetryCount++
		}
		if run.StartedAt == nil {
			run.StartedAt = int64Pointer(options.Now)
		}
		run.UpdatedAt = options.Now
		claim = CollectionTaskClaim{Run: run, Windows: windows, RequestID: options.RequestID}
		return repository.recalculateTaskBackfillStatuses(ctx, tx, run, options.Now)
	})
	return claim, err
}

type CollectionTaskWindowKey struct {
	SiteID int64
	HourTS int64
}

func (repository *CollectionTaskRepository) PendingMaterializationRunIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var ids []int64
	err := repository.db.WithContext(ctx).Model(&CollectionRun{}).Where(
		"status = 'pending' AND windows_initialized_at IS NULL",
	).Order("priority DESC, created_at ASC, id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (repository *CollectionTaskRepository) MaterializeRunWindows(
	ctx context.Context,
	runID int64,
	now int64,
	batchSize int,
) (CollectionRun, error) {
	if repository == nil || repository.db == nil || runID <= 0 || now <= 0 {
		return CollectionRun{}, ErrCollectionRunContract
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 1000
	}
	expected, baseRun, err := repository.loadMaterializationExpectation(ctx, runID)
	if err != nil {
		return CollectionRun{}, err
	}
	for offset := 0; offset < len(expected); offset += batchSize {
		end := offset + batchSize
		if end > len(expected) {
			end = len(expected)
		}
		chunk := expected[offset:end]
		err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			lockedRun, lockedExpected, err := repository.lockMaterializationExpectation(ctx, tx, baseRun)
			if err != nil {
				return err
			}
			if !equalCollectionTaskWindowKeys(expected, lockedExpected) {
				return ErrCollectionRunContract
			}
			rows := make([]CollectionRunWindow, len(chunk))
			for index, key := range chunk {
				rows[index] = CollectionRunWindow{
					RunID: lockedRun.ID, SiteID: key.SiteID, HourTS: key.HourTS,
					Status: CollectionTaskStatusPending, UpdatedAt: now,
				}
			}
			if len(rows) == 0 {
				return nil
			}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
		})
		if err != nil {
			return CollectionRun{}, err
		}
	}
	var result CollectionRun
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedRun, lockedExpected, err := repository.lockMaterializationExpectation(ctx, tx, baseRun)
		if err != nil {
			return err
		}
		if !equalCollectionTaskWindowKeys(expected, lockedExpected) {
			return ErrCollectionRunContract
		}
		var windows []CollectionRunWindow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", lockedRun.ID).
			Order("site_id ASC, hour_ts ASC, id ASC").Find(&windows).Error; err != nil {
			return err
		}
		if len(windows) != len(expected) {
			return ErrCollectionRunContract
		}
		for index, window := range windows {
			if window.SiteID != expected[index].SiteID || window.HourTS != expected[index].HourTS ||
				window.Status != CollectionTaskStatusPending {
				return ErrCollectionRunContract
			}
		}
		update := tx.Model(&CollectionRun{}).
			Where("id = ? AND status = 'pending' AND windows_initialized_at IS NULL", lockedRun.ID).
			Updates(map[string]any{"total_windows": len(expected), "windows_initialized_at": now, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCollectionTaskClaimLost
		}
		lockedRun.TotalWindows = len(expected)
		lockedRun.WindowsInitializedAt = int64Pointer(now)
		lockedRun.UpdatedAt = now
		if err := NewSiteRepository(tx).RecalculateLockedCollectionRun(ctx, &lockedRun, windows, now, nil); err != nil {
			return err
		}
		if err := repository.finalizeLocalRebuildLifecycle(ctx, tx, lockedRun, now); err != nil {
			return err
		}
		if err := repository.recalculateTaskBackfillStatuses(ctx, tx, lockedRun, now); err != nil {
			return err
		}
		result = lockedRun
		return nil
	})
	return result, err
}

func (repository *CollectionTaskRepository) loadMaterializationExpectation(
	ctx context.Context,
	runID int64,
) ([]CollectionTaskWindowKey, CollectionRun, error) {
	var result []CollectionTaskWindowKey
	var run CollectionRun
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&run, runID).Error; err != nil {
			return err
		}
		locked, expected, err := repository.lockMaterializationExpectation(ctx, tx, run)
		if err != nil {
			return err
		}
		run = locked
		result = expected
		return nil
	})
	return result, run, err
}

func (repository *CollectionTaskRepository) lockMaterializationExpectation(
	ctx context.Context,
	tx *gorm.DB,
	base CollectionRun,
) (CollectionRun, []CollectionTaskWindowKey, error) {
	scope, err := repository.resolveTaskScope(ctx, tx, base)
	if err != nil {
		return CollectionRun{}, nil, err
	}
	if err := repository.lockTaskScope(ctx, tx, base, scope, false); err != nil {
		return CollectionRun{}, nil, err
	}
	var run CollectionRun
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, base.ID).Error; err != nil {
		return CollectionRun{}, nil, err
	}
	if run.Status != CollectionTaskStatusPending || run.WindowsInitializedAt != nil ||
		run.TaskType != base.TaskType || run.TargetType != base.TargetType || run.TargetID != base.TargetID ||
		!equalInt64Pointers(run.SiteID, base.SiteID) || !equalInt64Pointers(run.StartTimestamp, base.StartTimestamp) ||
		!equalInt64Pointers(run.EndTimestamp, base.EndTimestamp) {
		return CollectionRun{}, nil, ErrCollectionTaskClaimLost
	}
	if err := repository.verifyTaskScopeLocked(ctx, tx, run, scope); err != nil {
		return CollectionRun{}, nil, err
	}
	expected, err := repository.expectedTaskWindowKeys(ctx, tx, run, scope)
	return run, expected, err
}

func (repository *CollectionTaskRepository) expectedTaskWindowKeys(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	scope collectionTaskScope,
) ([]CollectionTaskWindowKey, error) {
	if !constant.CollectionTaskWindowed(run.TaskType) || run.StartTimestamp == nil || run.EndTimestamp == nil {
		return nil, ErrCollectionRunContract
	}
	var siteIDs []int64
	switch run.TargetType {
	case "site":
		hours, err := NewSiteRepository(tx).expectedSiteRunWindowHours(ctx, run)
		if err != nil {
			return nil, err
		}
		keys := make([]CollectionTaskWindowKey, len(hours))
		for index, hour := range hours {
			keys[index] = CollectionTaskWindowKey{SiteID: *run.SiteID, HourTS: hour}
		}
		return keys, nil
	case "account", "customer":
		siteIDs = append(siteIDs, scope.SiteIDs...)
	default:
		return nil, ErrCollectionRunContract
	}
	keys := make([]CollectionTaskWindowKey, 0, len(siteIDs)*int((*run.EndTimestamp-*run.StartTimestamp)/3600))
	for _, siteID := range siteIDs {
		for hour := *run.StartTimestamp; hour < *run.EndTimestamp; hour += 3600 {
			keys = append(keys, CollectionTaskWindowKey{SiteID: siteID, HourTS: hour})
		}
	}
	return keys, nil
}

func equalCollectionTaskWindowKeys(first, second []CollectionTaskWindowKey) bool {
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

type CollectionTaskWindowResult struct {
	WindowID     int64
	AttemptCount int
	Status       string
	NextRetryAt  *int64
	FetchedRows  int64
	WrittenRows  int64
	ErrorCode    string
	ErrorParams  []byte
}

type CollectionTaskWindowMutationScope struct {
	Site   Site
	Run    CollectionRun
	Window CollectionRunWindow
}

type CollectionTaskWindowMutationResult struct {
	FetchedRows int64
	WrittenRows int64
}

// CollectionTaskWindowMutation defers all database writes produced by a
// window handler until the repository has revalidated the active claim and
// site configuration inside the completion transaction.
type CollectionTaskWindowMutation interface {
	ApplyCollectionTaskWindow(
		context.Context,
		*gorm.DB,
		CollectionTaskWindowMutationScope,
	) (CollectionTaskWindowMutationResult, error)
}

type CompleteClaimedWindowRequest struct {
	RunID     int64
	RequestID string
	Now       int64
	Window    CollectionTaskWindowResult
	Mutation  CollectionTaskWindowMutation
}

type CollectionTaskCommitRequest struct {
	RunID         int64
	RequestID     string
	Now           int64
	RunStatus     string
	NextAttemptAt *int64
	FetchedRows   int64
	WrittenRows   int64
	ErrorCode     string
	ErrorParams   []byte
	Windows       []CollectionTaskWindowResult
}

func (repository *CollectionTaskRepository) CommitClaim(ctx context.Context, request CollectionTaskCommitRequest) (CollectionRun, error) {
	if repository == nil || repository.db == nil || request.RunID <= 0 || request.Now <= 0 || !validCollectionRequestID(request.RequestID) {
		return CollectionRun{}, ErrCollectionRunContract
	}
	var result CollectionRun
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base CollectionRun
		if err := tx.First(&base, request.RunID).Error; err != nil {
			return err
		}
		scope, err := repository.resolveTaskScope(ctx, tx, base)
		if err != nil {
			return err
		}
		if err := repository.lockTaskScope(ctx, tx, base, scope, false); err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, request.RunID).Error; err != nil {
			return err
		}
		if run.Status != CollectionTaskStatusRunning || run.LastRequestID != request.RequestID {
			return ErrCollectionTaskClaimLost
		}
		if err := repository.verifyTaskScopeLocked(ctx, tx, run, scope); err != nil {
			return err
		}
		if constant.CollectionTaskWindowed(run.TaskType) {
			if request.RunStatus != "" || len(request.Windows) == 0 {
				return ErrCollectionRunContract
			}
			if err := repository.commitWindowResults(ctx, tx, &run, request); err != nil {
				return err
			}
			if err := repository.finalizeLocalRebuildLifecycle(ctx, tx, run, request.Now); err != nil {
				return err
			}
		} else {
			if len(request.Windows) != 0 {
				return ErrCollectionRunContract
			}
			if err := repository.commitNonWindowResult(tx, &run, request); err != nil {
				return err
			}
		}
		if err := repository.recalculateTaskBackfillStatuses(ctx, tx, run, request.Now); err != nil {
			return err
		}
		result = run
		return nil
	})
	return result, err
}

// CompleteClaimedWindow commits one claimed window and its optional business
// mutation atomically. Remote I/O must be completed before this method is
// called; the mutation is applied only after all claim and config fences pass.
func (repository *CollectionTaskRepository) CompleteClaimedWindow(
	ctx context.Context,
	request CompleteClaimedWindowRequest,
) (CollectionRun, error) {
	if repository == nil || repository.db == nil || request.RunID <= 0 || request.Now <= 0 ||
		!validCollectionRequestID(request.RequestID) || request.Window.WindowID <= 0 {
		return CollectionRun{}, ErrCollectionRunContract
	}
	var result CollectionRun
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base CollectionRun
		if err := tx.First(&base, request.RunID).Error; err != nil {
			return err
		}
		scope, err := repository.resolveTaskScope(ctx, tx, base)
		if err != nil {
			return err
		}
		if err := repository.lockTaskScope(ctx, tx, base, scope, false); err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, request.RunID).Error; err != nil {
			return err
		}
		if run.Status != CollectionTaskStatusRunning || run.LastRequestID != request.RequestID ||
			!constant.CollectionTaskWindowed(run.TaskType) {
			return ErrCollectionTaskClaimLost
		}
		if err := repository.verifyTaskScopeLocked(ctx, tx, run, scope); err != nil {
			return err
		}
		var window CollectionRunWindow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND run_id = ?", request.Window.WindowID, run.ID).First(&window).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCollectionTaskClaimLost
			}
			return err
		}
		if window.Status != CollectionTaskStatusRunning || window.AttemptCount != request.Window.AttemptCount {
			return ErrCollectionTaskClaimLost
		}
		if request.Mutation != nil {
			if window.SiteID <= 0 || !collectionTaskScopeContainsSite(scope, window.SiteID) {
				return ErrCollectionRunContract
			}
			var site Site
			if err := tx.First(&site, window.SiteID).Error; err != nil {
				return err
			}
			mutationResult, err := request.Mutation.ApplyCollectionTaskWindow(
				ctx,
				tx,
				CollectionTaskWindowMutationScope{Site: site, Run: run, Window: window},
			)
			if err != nil {
				return err
			}
			request.Window.FetchedRows = mutationResult.FetchedRows
			request.Window.WrittenRows = mutationResult.WrittenRows
		}
		commit := CollectionTaskCommitRequest{
			RunID: request.RunID, RequestID: request.RequestID, Now: request.Now,
			Windows: []CollectionTaskWindowResult{request.Window},
		}
		if err := repository.commitWindowResults(ctx, tx, &run, commit); err != nil {
			return err
		}
		if err := repository.finalizeLocalRebuildLifecycle(ctx, tx, run, request.Now); err != nil {
			return err
		}
		if err := repository.recalculateTaskBackfillStatuses(ctx, tx, run, request.Now); err != nil {
			return err
		}
		result = run
		return nil
	})
	return result, err
}

func collectionTaskScopeContainsSite(scope collectionTaskScope, siteID int64) bool {
	for _, candidate := range scope.SiteIDs {
		if candidate == siteID {
			return true
		}
	}
	return false
}

func (repository *CollectionTaskRepository) commitWindowResults(
	ctx context.Context,
	tx *gorm.DB,
	run *CollectionRun,
	request CollectionTaskCommitRequest,
) error {
	ids := make([]int64, len(request.Windows))
	byID := make(map[int64]CollectionTaskWindowResult, len(request.Windows))
	for index, outcome := range request.Windows {
		if outcome.WindowID <= 0 || outcome.AttemptCount <= 0 || outcome.FetchedRows < 0 || outcome.WrittenRows < 0 {
			return ErrCollectionRunContract
		}
		if _, duplicate := byID[outcome.WindowID]; duplicate {
			return ErrCollectionRunContract
		}
		switch outcome.Status {
		case CollectionTaskStatusPending:
			if outcome.NextRetryAt == nil || *outcome.NextRetryAt <= 0 {
				return ErrCollectionRunContract
			}
		case CollectionTaskStatusSuccess, CollectionTaskStatusUnavailable:
		case CollectionTaskStatusFailed:
			if outcome.ErrorCode == "" {
				return ErrCollectionRunContract
			}
		default:
			return ErrCollectionRunContract
		}
		ids[index] = outcome.WindowID
		byID[outcome.WindowID] = outcome
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var claimed []CollectionRunWindow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND id IN ?", run.ID, ids).
		Order("hour_ts ASC, id ASC").Find(&claimed).Error; err != nil {
		return err
	}
	if len(claimed) != len(ids) {
		return ErrCollectionTaskClaimLost
	}
	var fetched, written int64
	for _, window := range claimed {
		outcome := byID[window.ID]
		if window.Status != CollectionTaskStatusRunning || window.AttemptCount != outcome.AttemptCount {
			return ErrCollectionTaskClaimLost
		}
		updates := map[string]any{
			"status": outcome.Status, "next_retry_at": outcome.NextRetryAt,
			"fetched_rows": outcome.FetchedRows, "written_rows": outcome.WrittenRows,
			"error_code": outcome.ErrorCode, "error_params": nullableJSON(outcome.ErrorParams),
			"error_message": nil, "updated_at": request.Now,
		}
		if outcome.Status == CollectionTaskStatusSuccess || outcome.Status == CollectionTaskStatusFailed ||
			outcome.Status == CollectionTaskStatusUnavailable {
			updates["finished_at"] = request.Now
		} else {
			updates["finished_at"] = nil
		}
		update := tx.Model(&CollectionRunWindow{}).
			Where("id = ? AND run_id = ? AND status = 'running' AND attempt_count = ?", window.ID, run.ID, outcome.AttemptCount).
			Updates(updates)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCollectionTaskClaimLost
		}
		fetched += outcome.FetchedRows
		written += outcome.WrittenRows
	}
	parentUpdate := tx.Model(&CollectionRun{}).
		Where("id = ? AND status = 'running' AND last_request_id = ?", run.ID, request.RequestID).
		Updates(map[string]any{
			"fetched_rows":    gorm.Expr("fetched_rows + ?", fetched),
			"written_rows":    gorm.Expr("written_rows + ?", written),
			"last_request_id": request.RequestID,
		})
	if parentUpdate.Error != nil {
		return parentUpdate.Error
	}
	if parentUpdate.RowsAffected != 1 {
		return ErrCollectionTaskClaimLost
	}
	run.FetchedRows += fetched
	run.WrittenRows += written
	var allWindows []CollectionRunWindow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", run.ID).
		Order("hour_ts ASC, id ASC").Find(&allWindows).Error; err != nil {
		return err
	}
	if err := NewSiteRepository(tx).RecalculateLockedCollectionRun(ctx, run, allWindows, request.Now, nil); err != nil {
		return err
	}
	if run.Status == CollectionTaskStatusPending {
		run.HeartbeatAt = nil
		update := tx.Model(&CollectionRun{}).
			Where("id = ? AND status = 'pending' AND last_request_id = ?", run.ID, request.RequestID).
			Update("heartbeat_at", nil)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCollectionTaskClaimLost
		}
	}
	return nil
}

func (repository *CollectionTaskRepository) commitNonWindowResult(
	tx *gorm.DB,
	run *CollectionRun,
	request CollectionTaskCommitRequest,
) error {
	if request.FetchedRows < 0 || request.WrittenRows < 0 {
		return ErrCollectionRunContract
	}
	updates := map[string]any{
		"fetched_rows":    gorm.Expr("fetched_rows + ?", request.FetchedRows),
		"written_rows":    gorm.Expr("written_rows + ?", request.WrittenRows),
		"last_request_id": request.RequestID, "updated_at": request.Now,
	}
	switch request.RunStatus {
	case CollectionTaskStatusSuccess:
		updates["status"] = CollectionTaskStatusSuccess
		updates["active_key"] = nil
		updates["heartbeat_at"] = nil
		updates["finished_at"] = request.Now
		updates["error_code"] = ""
		updates["error_params"] = nil
	case CollectionTaskStatusPending:
		if request.NextAttemptAt == nil || *request.NextAttemptAt <= 0 {
			return ErrCollectionRunContract
		}
		updates["status"] = CollectionTaskStatusPending
		updates["heartbeat_at"] = nil
		updates["next_attempt_at"] = *request.NextAttemptAt
		updates["error_code"] = request.ErrorCode
		updates["error_params"] = nullableJSON(request.ErrorParams)
	case CollectionTaskStatusFailed:
		if request.ErrorCode == "" {
			return ErrCollectionRunContract
		}
		updates["status"] = CollectionTaskStatusFailed
		updates["active_key"] = nil
		updates["heartbeat_at"] = nil
		updates["finished_at"] = request.Now
		updates["error_code"] = request.ErrorCode
		updates["error_params"] = nullableJSON(request.ErrorParams)
	default:
		return ErrCollectionRunContract
	}
	update := tx.Model(&CollectionRun{}).
		Where("id = ? AND status = 'running' AND last_request_id = ?", run.ID, request.RequestID).
		Updates(updates)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return ErrCollectionTaskClaimLost
	}
	run.FetchedRows += request.FetchedRows
	run.WrittenRows += request.WrittenRows
	run.Status = request.RunStatus
	run.UpdatedAt = request.Now
	if request.RunStatus == CollectionTaskStatusPending {
		run.NextAttemptAt = *request.NextAttemptAt
		run.HeartbeatAt = nil
	} else {
		run.ActiveKey = nil
		run.HeartbeatAt = nil
		run.FinishedAt = int64Pointer(request.Now)
	}
	return nil
}

func (repository *CollectionTaskRepository) ReleaseClaim(
	ctx context.Context,
	claim CollectionTaskClaim,
	now int64,
) (CollectionRun, error) {
	if constant.CollectionTaskWindowed(claim.Run.TaskType) {
		outcomes := make([]CollectionTaskWindowResult, len(claim.Windows))
		for index, window := range claim.Windows {
			next := now
			outcomes[index] = CollectionTaskWindowResult{
				WindowID: window.ID, AttemptCount: window.AttemptCount,
				Status: CollectionTaskStatusPending, NextRetryAt: &next,
			}
		}
		if len(outcomes) == 0 {
			return CollectionRun{}, ErrCollectionRunContract
		}
		return repository.CommitClaim(ctx, CollectionTaskCommitRequest{
			RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now, Windows: outcomes,
		})
	}
	next := now
	return repository.CommitClaim(ctx, CollectionTaskCommitRequest{
		RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now,
		RunStatus: CollectionTaskStatusPending, NextAttemptAt: &next,
	})
}

func (repository *CollectionTaskRepository) Heartbeat(ctx context.Context, runID int64, requestID string, now int64) error {
	if repository == nil || repository.db == nil || runID <= 0 || !validCollectionRequestID(requestID) || now <= 0 {
		return ErrCollectionRunContract
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run CollectionRun
		if err := tx.First(&run, runID).Error; err != nil {
			return err
		}
		scope, err := repository.resolveTaskScope(ctx, tx, run)
		if err != nil {
			return err
		}
		if err := repository.lockTaskScope(ctx, tx, run, scope, false); err != nil {
			return err
		}
		update := tx.Model(&CollectionRun{}).
			Where("id = ? AND status = 'running' AND last_request_id = ?", runID, requestID).
			Updates(map[string]any{"heartbeat_at": now, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCollectionTaskClaimLost
		}
		return nil
	})
}

type CollectionTaskAttemptPolicy struct {
	DefaultMaxAttempts int
	MaxAttempts        map[string]int
}

func (policy CollectionTaskAttemptPolicy) maxAttempts(taskType string) int {
	if value := policy.MaxAttempts[taskType]; value > 0 {
		return value
	}
	if policy.DefaultMaxAttempts > 0 {
		return policy.DefaultMaxAttempts
	}
	return 3
}

func (repository *CollectionTaskRepository) RecoverRunning(
	ctx context.Context,
	now int64,
	staleBefore *int64,
	policy CollectionTaskAttemptPolicy,
) (int, error) {
	if repository == nil || repository.db == nil || now <= 0 {
		return 0, ErrCollectionRunContract
	}
	query := repository.db.WithContext(ctx).Model(&CollectionRun{}).Where("status = 'running'")
	if staleBefore != nil {
		query = query.Where("heartbeat_at IS NULL OR heartbeat_at < ?", *staleBefore)
	}
	var ids []int64
	if err := query.Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	recovered := 0
	for _, id := range ids {
		changed, err := repository.recoverRun(ctx, id, now, staleBefore, policy)
		if err != nil {
			return recovered, err
		}
		if changed {
			recovered++
		}
	}
	return recovered, nil
}

func (repository *CollectionTaskRepository) recoverRun(
	ctx context.Context,
	runID, now int64,
	staleBefore *int64,
	policy CollectionTaskAttemptPolicy,
) (bool, error) {
	changed := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base CollectionRun
		if err := tx.First(&base, runID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		scope, err := repository.resolveTaskScope(ctx, tx, base)
		if err != nil {
			return err
		}
		if err := repository.lockTaskScope(ctx, tx, base, scope, false); err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		if run.Status != CollectionTaskStatusRunning ||
			(staleBefore != nil && run.HeartbeatAt != nil && *run.HeartbeatAt >= *staleBefore) {
			return nil
		}
		maxAttempts := policy.maxAttempts(run.TaskType)
		if constant.CollectionTaskWindowed(run.TaskType) {
			var windows []CollectionRunWindow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", run.ID).
				Order("hour_ts ASC, id ASC").Find(&windows).Error; err != nil {
				return err
			}
			for index := range windows {
				if windows[index].Status != CollectionTaskStatusRunning {
					continue
				}
				updates := map[string]any{"updated_at": now, "finished_at": nil}
				if windows[index].AttemptCount < maxAttempts {
					updates["status"] = CollectionTaskStatusPending
					updates["next_retry_at"] = now
					windows[index].Status = CollectionTaskStatusPending
					next := now
					windows[index].NextRetryAt = &next
				} else {
					updates["status"] = CollectionTaskStatusFailed
					updates["next_retry_at"] = nil
					updates["finished_at"] = now
					updates["error_code"] = CollectionTaskLeaseLostCode
					updates["error_params"] = nil
					windows[index].Status = CollectionTaskStatusFailed
					windows[index].ErrorCode = CollectionTaskLeaseLostCode
					windows[index].FinishedAt = int64Pointer(now)
				}
				if err := tx.Model(&CollectionRunWindow{}).Where("id = ? AND status = 'running'", windows[index].ID).
					Updates(updates).Error; err != nil {
					return err
				}
			}
			if err := NewSiteRepository(tx).RecalculateLockedCollectionRun(ctx, &run, windows, now, nil); err != nil {
				return err
			}
			if run.Status == CollectionTaskStatusPending {
				run.HeartbeatAt = nil
				if err := tx.Model(&CollectionRun{}).Where("id = ?", run.ID).Update("heartbeat_at", nil).Error; err != nil {
					return err
				}
			}
		} else {
			attempt := run.RetryCount
			updates := map[string]any{"heartbeat_at": nil, "updated_at": now, "error_code": CollectionTaskLeaseLostCode}
			if attempt < maxAttempts {
				updates["status"] = CollectionTaskStatusPending
				updates["next_attempt_at"] = now
				run.Status = CollectionTaskStatusPending
				run.NextAttemptAt = now
			} else {
				updates["status"] = CollectionTaskStatusFailed
				updates["active_key"] = nil
				updates["finished_at"] = now
				run.Status = CollectionTaskStatusFailed
				run.ActiveKey = nil
				run.FinishedAt = int64Pointer(now)
			}
			if err := tx.Model(&CollectionRun{}).Where("id = ? AND status = 'running'", run.ID).Updates(updates).Error; err != nil {
				return err
			}
			run.HeartbeatAt = nil
		}
		if err := repository.recalculateTaskBackfillStatuses(ctx, tx, run, now); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

type collectionTaskAccountRef struct {
	ID         int64 `gorm:"column:id"`
	SiteID     int64 `gorm:"column:site_id"`
	CustomerID int64 `gorm:"column:customer_id"`
}

type collectionTaskScope struct {
	SiteIDs     []int64
	CustomerIDs []int64
	Accounts    []collectionTaskAccountRef
}

func (repository *CollectionTaskRepository) resolveTaskScope(
	ctx context.Context,
	db *gorm.DB,
	run CollectionRun,
) (collectionTaskScope, error) {
	scope := collectionTaskScope{}
	switch run.TargetType {
	case "site":
		if run.SiteID == nil || *run.SiteID <= 0 || *run.SiteID != run.TargetID {
			return scope, ErrCollectionRunContract
		}
		scope.SiteIDs = []int64{*run.SiteID}
	case "account":
		var account collectionTaskAccountRef
		if err := db.WithContext(ctx).Table("account").Select("id, site_id, customer_id").
			Where("id = ?", run.TargetID).Take(&account).Error; err != nil {
			return scope, err
		}
		scope.Accounts = []collectionTaskAccountRef{account}
		scope.SiteIDs = []int64{account.SiteID}
		scope.CustomerIDs = []int64{account.CustomerID}
	case "customer":
		scope.CustomerIDs = []int64{run.TargetID}
		if err := db.WithContext(ctx).Table("account").Select("id, site_id, customer_id").
			Where("customer_id = ?", run.TargetID).Order("id ASC").Find(&scope.Accounts).Error; err != nil {
			return scope, err
		}
		for _, account := range scope.Accounts {
			scope.SiteIDs = appendUniqueSortedID(scope.SiteIDs, account.SiteID)
		}
	default:
		return scope, ErrCollectionRunContract
	}
	return scope, nil
}

func (repository *CollectionTaskRepository) lockTaskScope(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	scope collectionTaskScope,
	skipLocked bool,
) error {
	locking := clause.Locking{Strength: "UPDATE"}
	if skipLocked {
		locking.Options = "SKIP LOCKED"
	}
	if len(scope.SiteIDs) > 0 {
		var sites []Site
		if err := tx.WithContext(ctx).Clauses(locking).Where("id IN ?", scope.SiteIDs).Order("id ASC").Find(&sites).Error; err != nil {
			return err
		}
		if len(sites) != len(scope.SiteIDs) {
			if skipLocked {
				return ErrCollectionTaskUnavailable
			}
			return ErrCollectionRunContract
		}
	}
	if len(scope.CustomerIDs) > 0 {
		var customers []Customer
		if err := tx.WithContext(ctx).Clauses(locking).Select("id").Where("id IN ?", scope.CustomerIDs).
			Order("id ASC").Find(&customers).Error; err != nil {
			return err
		}
		if len(customers) != len(scope.CustomerIDs) {
			if skipLocked {
				return ErrCollectionTaskUnavailable
			}
			return ErrCollectionRunContract
		}
	}
	if len(scope.Accounts) > 0 {
		ids := make([]int64, len(scope.Accounts))
		for index := range scope.Accounts {
			ids[index] = scope.Accounts[index].ID
		}
		var accounts []collectionTaskAccountRef
		if err := tx.WithContext(ctx).Table("account").Select("id, site_id, customer_id").Clauses(locking).
			Where("id IN ?", ids).Order("id ASC").Find(&accounts).Error; err != nil {
			return err
		}
		if len(accounts) != len(scope.Accounts) {
			if skipLocked {
				return ErrCollectionTaskUnavailable
			}
			return ErrCollectionRunContract
		}
		for index := range accounts {
			if accounts[index] != scope.Accounts[index] {
				return ErrCollectionRunContract
			}
		}
	}
	_ = run
	return nil
}

func (repository *CollectionTaskRepository) verifyTaskScopeLocked(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	scope collectionTaskScope,
) error {
	if run.TargetType == "site" {
		var site Site
		if err := tx.WithContext(ctx).First(&site, run.TargetID).Error; err != nil {
			return err
		}
		if run.SiteID == nil || *run.SiteID != site.ID || run.SiteConfigVersion != site.ConfigVersion || !siteAllowsTask(site, run.TaskType) {
			return ErrSiteRunConfigChanged
		}
		if constant.CollectionTaskWindowed(run.TaskType) {
			var capabilities []SiteCapability
			if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", site.ID).
				Order("capability_key ASC").Find(&capabilities).Error; err != nil {
				return err
			}
			if err := ValidateRunnableSiteSnapshot(RunnableSiteSnapshot{Site: site, Capabilities: capabilities}, run.SiteConfigVersion); err != nil {
				return err
			}
		}
		return nil
	}
	if run.SiteID != nil || run.SiteConfigVersion != 0 {
		return ErrCollectionRunContract
	}
	if run.TargetType == "account" {
		if len(scope.Accounts) != 1 || scope.Accounts[0].ID != run.TargetID {
			return ErrCollectionRunContract
		}
	} else if run.TargetType == "customer" {
		if len(scope.CustomerIDs) != 1 || scope.CustomerIDs[0] != run.TargetID {
			return ErrCollectionRunContract
		}
	}
	return nil
}

func siteAllowsTask(site Site, taskType string) bool {
	if site.ConfigVersion <= 0 {
		return false
	}
	if taskType == constant.TaskTypeSiteProbe {
		return site.ManagementStatus == constant.SiteManagementActive && site.StatisticsEndAt == nil
	}
	if site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized ||
		site.StatisticsEndAt != nil {
		return false
	}
	if constant.CollectionTaskWindowed(taskType) && !site.DataExportEnabled {
		return false
	}
	return true
}

func (repository *CollectionTaskRepository) scopeHasRunningTask(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	scope collectionTaskScope,
) (bool, error) {
	var count int64
	if run.TargetType == "site" {
		if err := tx.WithContext(ctx).Model(&CollectionRun{}).
			Where("site_id = ? AND task_type = ? AND status = 'running' AND id <> ?", scope.SiteIDs[0], run.TaskType, run.ID).Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	}
	if len(scope.CustomerIDs) != 1 {
		return false, ErrCollectionRunContract
	}
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM collection_run r
LEFT JOIN account a ON r.target_type = 'account' AND r.target_id = a.id
WHERE r.status = 'running' AND r.id <> ?
  AND r.task_type = ?
  AND ((r.target_type = 'customer' AND r.target_id = ?) OR a.customer_id = ?)`,
		run.ID, run.TaskType, scope.CustomerIDs[0], scope.CustomerIDs[0]).Scan(&count).Error
	return count > 0, err
}

func (repository *CollectionTaskRepository) recalculateTaskBackfillStatuses(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	now int64,
) error {
	if run.TargetType != "account" && run.TargetType != "customer" {
		return nil
	}
	plan := siteFencePlan{Runs: []CollectionRun{run}, AccountRefs: []siteFenceAccountRef{}, CustomerIDs: []int64{}}
	if run.TargetType == "account" {
		var account siteFenceAccountRef
		if err := tx.WithContext(ctx).Table("account").Select("id, site_id, customer_id").Where("id = ?", run.TargetID).
			Take(&account).Error; err != nil {
			return err
		}
		plan.AccountRefs = append(plan.AccountRefs, account)
		plan.CustomerIDs = append(plan.CustomerIDs, account.CustomerID)
	} else {
		plan.CustomerIDs = append(plan.CustomerIDs, run.TargetID)
		if err := tx.WithContext(ctx).Table("account").Select("id, site_id, customer_id").Where("customer_id = ?", run.TargetID).
			Order("id ASC").Find(&plan.AccountRefs).Error; err != nil {
			return err
		}
	}
	return NewSiteRepository(tx).recalculateSiteFenceBackfillStatuses(ctx, plan, now)
}

type SiteUserObservation struct {
	RemoteUserID    int64
	RemoteCreatedAt int64
	Username        string
	DisplayName     string
	RemoteGroup     string
	RemoteStatus    int
	RemoteRole      int
	Quota           int64
	UsedQuota       int64
	RequestCount    int64
	LastLoginAt     int64
	Deleted         bool
}

// ApplySiteUserSnapshot applies one already-validated complete upstream user
// snapshot atomically. A prior identity_mismatch is deliberately sticky.
func (repository *CollectionTaskRepository) ApplySiteUserSnapshot(
	ctx context.Context,
	expectedSite Site,
	observedAt int64,
	pauseAt int64,
	observations []SiteUserObservation,
) (int64, error) {
	return repository.applySiteUserSnapshot(ctx, expectedSite, observedAt, pauseAt, observations, true)
}

// ApplyAuthorizationSiteUserSnapshot accepts a paused lifecycle because
// authorization is explicitly allowed while a site is disabled. It retains the
// same frozen configuration and credential fence as periodic user sync.
func (repository *CollectionTaskRepository) ApplyAuthorizationSiteUserSnapshot(
	ctx context.Context,
	expectedSite Site,
	observedAt int64,
	pauseAt int64,
	observations []SiteUserObservation,
) (int64, error) {
	return repository.applySiteUserSnapshot(ctx, expectedSite, observedAt, pauseAt, observations, false)
}

func (repository *CollectionTaskRepository) applySiteUserSnapshot(
	ctx context.Context,
	expectedSite Site,
	observedAt int64,
	pauseAt int64,
	observations []SiteUserObservation,
	requireRunnable bool,
) (int64, error) {
	if repository == nil || repository.db == nil || expectedSite.ID <= 0 || expectedSite.ConfigVersion <= 0 ||
		observedAt <= 0 || pauseAt <= 0 {
		return 0, ErrCollectionRunContract
	}
	byRemoteID := make(map[int64]SiteUserObservation, len(observations))
	for _, observation := range observations {
		if observation.RemoteUserID <= 0 || observation.RemoteCreatedAt <= 0 {
			return 0, ErrAccountObservationInvalid
		}
		if _, duplicate := byRemoteID[observation.RemoteUserID]; duplicate {
			return 0, ErrAccountObservationInvalid
		}
		byRemoteID[observation.RemoteUserID] = observation
	}
	var updated int64
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, expectedSite.ID).Error; err != nil {
			return err
		}
		if site.ConfigVersion != expectedSite.ConfigVersion || site.BaseURL != expectedSite.BaseURL ||
			!equalInt64Pointers(site.RootUserID, expectedSite.RootUserID) ||
			!equalStringPointers(site.AccessTokenEncrypted, expectedSite.AccessTokenEncrypted) ||
			site.AuthStatus != constant.SiteAuthAuthorized ||
			(requireRunnable && !siteAllowsTask(site, constant.TaskTypeUserSync)) {
			return ErrSiteRunConfigChanged
		}
		var accounts []Account
		inventoryWrites, err := applySiteUserInventorySnapshot(tx, site, observedAt, pauseAt, observations)
		if err != nil {
			return err
		}
		updated += inventoryWrites
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", expectedSite.ID).
			Order("id ASC").Find(&accounts).Error; err != nil {
			return err
		}
		statisticsPauses := make([]statisticsPauseAccount, 0)
		for _, account := range accounts {
			if !isNewerAccountObservation(account, observedAt) {
				continue
			}
			observation, exists := byRemoteID[account.RemoteUserID]
			updates := map[string]any{
				"last_synced_at": observedAt,
				"updated_at":     monotonicAccountUpdatedAt(account, observedAt),
			}
			switch {
			case !exists || observation.Deleted:
				if account.RemoteState != AccountRemoteStateIdentityMismatch {
					nextCount := account.RemoteMissingCount
					if nextCount < int(^uint32(0)>>1) {
						nextCount++
					}
					updates["remote_missing_count"] = nextCount
					if nextCount >= 2 {
						updates["remote_state"] = AccountRemoteStateMissing
					}
				}
			case observation.RemoteCreatedAt != account.RemoteCreatedAt:
				effectivePause := pauseAt
				if account.StatisticsPausedAt != nil && *account.StatisticsPausedAt < effectivePause {
					effectivePause = *account.StatisticsPausedAt
				}
				updates["remote_state"] = AccountRemoteStateIdentityMismatch
				updates["statistics_paused_at"] = effectivePause
				statisticsPauses = append(statisticsPauses, statisticsPauseAccount{
					ID: account.ID, SiteID: account.SiteID, CustomerID: account.CustomerID, PauseAt: effectivePause,
				})
			case account.RemoteState == AccountRemoteStateIdentityMismatch:
				// The conflicting interval cannot be attributed safely, even when the
				// original identity appears again in a later snapshot.
				if account.StatisticsPausedAt != nil {
					statisticsPauses = append(statisticsPauses, statisticsPauseAccount{
						ID: account.ID, SiteID: account.SiteID, CustomerID: account.CustomerID,
						PauseAt: *account.StatisticsPausedAt,
					})
				}
			default:
				updates["username"] = observation.Username
				updates["display_name"] = observation.DisplayName
				updates["remote_group"] = observation.RemoteGroup
				updates["remote_status"] = observation.RemoteStatus
				updates["quota"] = observation.Quota
				updates["used_quota"] = observation.UsedQuota
				updates["request_count"] = observation.RequestCount
				updates["remote_state"] = AccountRemoteStateNormal
				updates["remote_missing_count"] = 0
				updates["last_remote_seen_at"] = observedAt
			}
			result := accountObservationCAS(tx, account).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountObservationCAS
			}
			updated++
		}
		return cleanupStatisticsForPause(ctx, tx, statisticsPauses, nil, observedAt)
	})
	return updated, err
}

func (repository *SiteRepository) ApplySiteUserSnapshot(
	ctx context.Context,
	expectedSite Site,
	observedAt int64,
	pauseAt int64,
	observations []SiteUserObservation,
) (int64, error) {
	return NewCollectionTaskRepository(repository.db).ApplySiteUserSnapshot(
		ctx, expectedSite, observedAt, pauseAt, observations,
	)
}

func (repository *SiteRepository) ApplyAuthorizationSiteUserSnapshot(
	ctx context.Context,
	expectedSite Site,
	observedAt int64,
	pauseAt int64,
	observations []SiteUserObservation,
) (int64, error) {
	return NewCollectionTaskRepository(repository.db).ApplyAuthorizationSiteUserSnapshot(
		ctx, expectedSite, observedAt, pauseAt, observations,
	)
}

func equalStringPointers(first, second *string) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

type SiteStatusMinutely struct {
	ID                  int64    `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID              int64    `gorm:"column:site_id"`
	MinuteTS            int64    `gorm:"column:minute_ts"`
	InstanceCount       int      `gorm:"column:instance_count"`
	OnlineInstanceCount int      `gorm:"column:online_instance_count"`
	CPUMaxPercent       *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent       *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent    *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent    *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent  *float64 `gorm:"column:disk_max_used_percent"`
	HealthStatus        string   `gorm:"column:health_status"`
	CreatedAt           int64    `gorm:"column:created_at"`
}

func (SiteStatusMinutely) TableName() string { return "site_status_minutely" }

func (repository *SiteRepository) UpsertSiteStatusMinute(ctx context.Context, sample SiteStatusMinutely) error {
	if sample.SiteID <= 0 || sample.MinuteTS <= 0 || sample.MinuteTS%60 != 0 || sample.CreatedAt <= 0 || sample.HealthStatus == "" {
		return ErrCollectionRunContract
	}
	return repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "minute_ts"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"instance_count", "online_instance_count", "cpu_max_percent", "cpu_avg_percent",
			"memory_max_percent", "memory_avg_percent", "disk_max_used_percent", "health_status", "created_at",
		}),
	}).Create(&sample).Error
}
