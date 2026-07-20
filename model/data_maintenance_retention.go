package model

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/constant"
)

const maximumDataMaintenanceBatchSize = 5000

type DataMaintenanceBatchResult struct {
	Attempted bool
	Affected  int64
	Complete  bool
	CursorID  int64
}

func (repository *DataMaintenanceRepository) RedactCollectionRunErrors(
	ctx context.Context, dateKey int, cutoff int64, maximumRows int, now int64,
) (DataMaintenanceBatchResult, error) {
	return repository.runCollectionRunRetentionBatch(ctx, MaintenanceRunErrorRedaction, dateKey, cutoff, maximumRows, now)
}

func (repository *DataMaintenanceRepository) CleanupMetadataDiagnosticRuns(
	ctx context.Context, dateKey int, cutoff int64, maximumRows int, now int64,
) (DataMaintenanceBatchResult, error) {
	return repository.runCollectionRunRetentionBatch(ctx, MaintenanceMetadataRunCleanup, dateKey, cutoff, maximumRows, now)
}

func (repository *DataMaintenanceRepository) runCollectionRunRetentionBatch(
	ctx context.Context, operation string, dateKey int, cutoff int64, maximumRows int, now int64,
) (DataMaintenanceBatchResult, error) {
	if repository == nil || repository.db == nil || dateKey <= 0 || cutoff <= 0 || now <= 0 ||
		maximumRows <= 0 || maximumRows > maximumDataMaintenanceBatchSize ||
		(operation != MaintenanceRunErrorRedaction && operation != MaintenanceMetadataRunCleanup) {
		return DataMaintenanceBatchResult{}, ErrDataMaintenanceContract
	}
	if err := repository.ensureGlobalState(ctx, operation, now); err != nil {
		return DataMaintenanceBatchResult{}, err
	}
	result := DataMaintenanceBatchResult{Attempted: true}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state DataMaintenanceState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("operation_id = ? AND scope_key = 'global'", operation).First(&state).Error; err != nil {
			return err
		}
		if state.DateKey != dateKey {
			state.DateKey, state.CursorID, state.Status = dateKey, 0, MaintenanceStatusPending
		}
		if state.Status == MaintenanceStatusComplete {
			result.Attempted, result.Complete, result.CursorID = false, true, state.CursorID
			return nil
		}
		state.Status = MaintenanceStatusRunning
		state.AttemptCount++
		state.LastAttemptAt = &now
		state.UpdatedAt = now
		var rows []CollectionRun
		query := tx.Model(&CollectionRun{}).Where("id > ? AND finished_at < ?", state.CursorID, cutoff)
		if operation == MaintenanceRunErrorRedaction {
			query = query.Where("error_message IS NOT NULL")
		} else {
			query = query.Where("status = ? AND trigger_type = ? AND task_type IN ? AND start_timestamp IS NULL AND end_timestamp IS NULL",
				CollectionTaskStatusSuccess, constant.CollectionTriggerSchedule,
				[]string{constant.TaskTypeUserSync, constant.TaskTypeChannelSync}).
				Where("NOT EXISTS (SELECT 1 FROM collection_run_window child WHERE child.run_id = collection_run.id)")
		}
		if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Limit(maximumRows).Find(&rows).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if len(ids) > 0 {
			if operation == MaintenanceRunErrorRedaction {
				updated := tx.Model(&CollectionRun{}).
					Where("id IN ? AND finished_at < ? AND error_message IS NOT NULL", ids, cutoff).
					Update("error_message", nil)
				if updated.Error != nil {
					return updated.Error
				}
				result.Affected = updated.RowsAffected
			} else {
				deleted := tx.Where("id IN ? AND finished_at < ? AND status = ? AND trigger_type = ? AND task_type IN ? AND start_timestamp IS NULL AND end_timestamp IS NULL",
					ids, cutoff, CollectionTaskStatusSuccess, constant.CollectionTriggerSchedule,
					[]string{constant.TaskTypeUserSync, constant.TaskTypeChannelSync}).
					Where("NOT EXISTS (SELECT 1 FROM collection_run_window child WHERE child.run_id = collection_run.id)").Delete(&CollectionRun{})
				if deleted.Error != nil {
					return deleted.Error
				}
				if deleted.RowsAffected != int64(len(ids)) {
					return ErrDataMaintenanceContract
				}
				result.Affected = deleted.RowsAffected
			}
			state.CursorID = ids[len(ids)-1]
		}
		result.CursorID = state.CursorID
		result.Complete = len(ids) < maximumRows
		if result.Complete {
			state.Status = MaintenanceStatusComplete
			state.LastSuccessAt = &now
			state.NextAttemptAt = 0
			state.ErrorCode = ""
		} else {
			state.Status = MaintenanceStatusPending
			state.NextAttemptAt = now
		}
		state.UpdatedAt = now
		return tx.Save(&state).Error
	})
	if err != nil {
		_ = repository.markGlobalFailure(ctx, operation, now, "BATCH_FAILED")
		return DataMaintenanceBatchResult{}, fmt.Errorf("run %s batch: %w", operation, err)
	}
	return result, nil
}

func (repository *DataMaintenanceRepository) ensureGlobalState(ctx context.Context, operation string, now int64) error {
	state := DataMaintenanceState{
		OperationID: operation, ScopeKey: "global", Status: MaintenanceStatusPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "operation_id"}, {Name: "scope_key"}}, DoNothing: true,
	}).Create(&state).Error
}

func (repository *DataMaintenanceRepository) markGlobalFailure(ctx context.Context, operation string, now int64, code string) error {
	return repository.db.WithContext(ctx).Model(&DataMaintenanceState{}).
		Where("operation_id = ? AND scope_key = 'global'", operation).
		Updates(map[string]any{
			"status": MaintenanceStatusFailed, "error_code": code,
			"attempt_count": gorm.Expr("attempt_count + 1"), "last_attempt_at": now,
			"last_failure_at": now, "next_attempt_at": now + 60, "updated_at": now,
		}).Error
}
