package model

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
)

const LocalRebuildDependencyPendingCode = "LOCAL_REBUILD_DEPENDENCY_PENDING"

var ErrLocalRebuildDependencyPending = errors.New("local rebuild dependency is not complete")

type LocalRebuildMutationRequest struct {
	RunID        int64
	WindowID     int64
	SiteID       int64
	HourTS       int64
	AttemptCount int
	RequestID    string
	Now          int64
	TaskType     string
	TargetID     int64
}

type LocalRebuildMutation struct {
	request LocalRebuildMutationRequest
}

func NewLocalRebuildMutation(request LocalRebuildMutationRequest) (LocalRebuildMutation, error) {
	if request.RunID <= 0 || request.WindowID <= 0 || request.SiteID <= 0 || request.HourTS <= 0 ||
		request.HourTS%3600 != 0 || request.HourTS > math.MaxInt64-3600 || request.AttemptCount <= 0 ||
		!validCollectionRequestID(request.RequestID) || request.Now <= 0 || request.TargetID <= 0 ||
		(request.TaskType != constant.TaskTypeAccountRebuild && request.TaskType != constant.TaskTypeCustomerRebuild) {
		return LocalRebuildMutation{}, ErrCollectionRunContract
	}
	return LocalRebuildMutation{request: request}, nil
}

func (mutation LocalRebuildMutation) ApplyCollectionTaskWindow(
	ctx context.Context,
	tx *gorm.DB,
	scope CollectionTaskWindowMutationScope,
) (CollectionTaskWindowMutationResult, error) {
	request := mutation.request
	run := scope.Run
	window := scope.Window
	expectedTarget := "account"
	if request.TaskType == constant.TaskTypeCustomerRebuild {
		expectedTarget = "customer"
	}
	if tx == nil || scope.Site.ID != request.SiteID || run.ID != request.RunID ||
		run.TaskType != request.TaskType || run.TargetType != expectedTarget || run.TargetID != request.TargetID ||
		run.SiteID != nil || run.SiteConfigVersion != 0 || run.StartTimestamp == nil || run.EndTimestamp == nil ||
		request.HourTS < *run.StartTimestamp || request.HourTS >= *run.EndTimestamp ||
		run.Status != CollectionTaskStatusRunning || run.LastRequestID != request.RequestID ||
		window.ID != request.WindowID || window.RunID != run.ID || window.SiteID != request.SiteID ||
		window.HourTS != request.HourTS || window.AttemptCount != request.AttemptCount ||
		window.Status != CollectionTaskStatusRunning {
		return CollectionTaskWindowMutationResult{}, ErrCollectionRunContract
	}
	factWindow, exists, err := lockCollectionWindow(ctx, tx, request.SiteID, request.HourTS)
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	if !exists || factWindow.Status != CollectionWindowStatusComplete {
		return CollectionTaskWindowMutationResult{}, ErrLocalRebuildDependencyPending
	}
	dateKey, dateStart, dateEnd, err := UsageDateBucket(request.HourTS)
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	keys, err := usageAggregationBucketKeys(
		ctx, tx, request.SiteID, request.HourTS, dateKey, dateStart, dateEnd, nil,
	)
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	if err := lockUsageAggregationBuckets(ctx, tx, keys, request.Now); err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	options, err := localRebuildAggregationOptions(ctx, tx, run)
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	rebuilt, err := rebuildUsageAggregationBuckets(
		ctx, tx, request.SiteID, request.HourTS, dateKey, dateStart, dateEnd, request.Now, options,
	)
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	if rebuilt.HourlyRows < 0 || rebuilt.DailyRows < 0 || rebuilt.HourlyRows > math.MaxInt64-rebuilt.DailyRows {
		return CollectionTaskWindowMutationResult{}, ErrCollectionRunContract
	}
	return CollectionTaskWindowMutationResult{WrittenRows: rebuilt.HourlyRows + rebuilt.DailyRows}, nil
}

func localRebuildAggregationOptions(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
) (usageAggregationRebuildOptions, error) {
	options := usageAggregationRebuildOptions{
		includePausedAccountIDs:  make(map[int64]struct{}),
		includePausedCustomerIDs: make(map[int64]struct{}),
	}
	switch run.TaskType {
	case constant.TaskTypeAccountRebuild:
		if run.TargetType != "account" {
			return usageAggregationRebuildOptions{}, ErrCollectionRunContract
		}
		options.includePausedAccountIDs[run.TargetID] = struct{}{}
	case constant.TaskTypeCustomerRebuild:
		if run.TargetType != "customer" {
			return usageAggregationRebuildOptions{}, ErrCollectionRunContract
		}
		options.includePausedCustomerIDs[run.TargetID] = struct{}{}
		var accountIDs []int64
		if err := tx.WithContext(ctx).Model(&Account{}).
			Where("customer_id = ? AND managed_status <> ? AND remote_state <> ?", run.TargetID,
				AccountManagedStatusArchived, AccountRemoteStateIdentityMismatch).
			Order("id ASC").Pluck("id", &accountIDs).Error; err != nil {
			return usageAggregationRebuildOptions{}, err
		}
		for _, accountID := range accountIDs {
			options.includePausedAccountIDs[accountID] = struct{}{}
		}
	default:
		return usageAggregationRebuildOptions{}, ErrCollectionRunContract
	}
	return options, options.validate()
}

func (repository *CollectionTaskRepository) finalizeLocalRebuildLifecycle(
	ctx context.Context,
	tx *gorm.DB,
	run CollectionRun,
	now int64,
) error {
	if run.Status != CollectionTaskStatusSuccess ||
		(run.TaskType != constant.TaskTypeAccountRebuild && run.TaskType != constant.TaskTypeCustomerRebuild) {
		return nil
	}
	var latest CollectionRun
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("target_type = ? AND target_id = ? AND task_type = ?", run.TargetType, run.TargetID, run.TaskType).
		Order("id DESC").Take(&latest).Error; err != nil {
		return err
	}
	if latest.ID != run.ID || latest.Status != CollectionTaskStatusSuccess ||
		latest.StartTimestamp == nil || latest.EndTimestamp == nil {
		return ErrRebuildRunNotReady
	}
	if run.TaskType == constant.TaskTypeAccountRebuild {
		return finalizeAccountRebuildLifecycle(ctx, tx, latest, now)
	}
	return finalizeCustomerRebuildLifecycle(ctx, tx, latest, now)
}

func finalizeAccountRebuildLifecycle(ctx context.Context, tx *gorm.DB, run CollectionRun, now int64) error {
	var account Account
	if err := tx.WithContext(ctx).First(&account, run.TargetID).Error; err != nil {
		return err
	}
	if account.ManagedStatus == AccountManagedStatusActive && account.StatisticsPausedAt == nil {
		return nil
	}
	var customer Customer
	if err := tx.WithContext(ctx).First(&customer, account.CustomerID).Error; err != nil {
		return err
	}
	if customer.Status == "disabled" {
		return ErrCustomerDisabled
	}
	if account.RemoteState == AccountRemoteStateIdentityMismatch ||
		account.ManagedStatus != AccountManagedStatusArchived || account.StatisticsPausedAt == nil ||
		!localRebuildRunCoversPause(run, account.StatisticsPausedAt) {
		return ErrAccountRestoreContract
	}
	return tx.WithContext(ctx).Model(&Account{}).Where("id = ?", account.ID).Updates(map[string]any{
		"managed_status":             AccountManagedStatusActive,
		"statistics_paused_at":       nil,
		"statistics_backfill_status": "none",
		"updated_at":                 now,
	}).Error
}

func finalizeCustomerRebuildLifecycle(ctx context.Context, tx *gorm.DB, run CollectionRun, now int64) error {
	var customer Customer
	if err := tx.WithContext(ctx).First(&customer, run.TargetID).Error; err != nil {
		return err
	}
	if customer.Status == "using" && customer.StatisticsPausedAt == nil {
		return nil
	}
	if customer.Status != "disabled" || customer.StatisticsPausedAt == nil ||
		!localRebuildRunCoversPause(run, customer.StatisticsPausedAt) {
		return ErrCustomerLifecycleContract
	}
	var accounts []Account
	if err := tx.WithContext(ctx).Where("customer_id = ?", customer.ID).Order("id ASC").Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		if account.ManagedStatus == AccountManagedStatusArchived || account.RemoteState == AccountRemoteStateIdentityMismatch {
			continue
		}
		if account.StatisticsPausedAt != nil && !localRebuildRunCoversPause(run, account.StatisticsPausedAt) {
			return ErrCustomerEnableNotReady
		}
		if err := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", account.ID).Updates(map[string]any{
			"statistics_paused_at":       nil,
			"statistics_backfill_status": "none",
			"updated_at":                 now,
		}).Error; err != nil {
			return err
		}
	}
	return tx.WithContext(ctx).Model(&Customer{}).Where("id = ?", customer.ID).Updates(map[string]any{
		"status":                     "using",
		"statistics_paused_at":       nil,
		"statistics_backfill_status": "none",
		"updated_at":                 now,
	}).Error
}

func localRebuildRunCoversPause(run CollectionRun, pause *int64) bool {
	return pause != nil && run.StartTimestamp != nil && run.EndTimestamp != nil &&
		*run.StartTimestamp <= *pause && *run.EndTimestamp >= *pause
}
