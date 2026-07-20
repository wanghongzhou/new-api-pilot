package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/constant"
)

const (
	MaintenanceAuthorizePricingSync = "authorize_pricing_group_sync"
	MaintenanceResourceDaily        = "resource_daily_finalize"
	MaintenanceResourceGapRepair    = "resource_rollup_gap_repair"
	MaintenanceRunErrorRedaction    = "collection_run_error_redaction"
	MaintenanceMetadataRunCleanup   = "metadata_diagnostic_run_cleanup"

	MaintenanceStatusPending  = "pending"
	MaintenanceStatusRunning  = "running"
	MaintenanceStatusComplete = "complete"
	MaintenanceStatusFailed   = "failed"
)

var ErrDataMaintenanceContract = errors.New("data maintenance contract is invalid")

const (
	MaintenanceErrorSiteConfigChanged = "SITE_CONFIG_CHANGED"
	MaintenanceErrorSiteIneligible    = "SITE_INELIGIBLE"
	MaintenanceErrorCapabilities      = "CAPABILITIES_NOT_READY"
	MaintenanceErrorEnqueue           = "ENQUEUE_FAILED"
)

type DataMaintenanceState struct {
	ID                int64  `gorm:"column:id;primaryKey;autoIncrement"`
	OperationID       string `gorm:"column:operation_id"`
	ScopeKey          string `gorm:"column:scope_key"`
	ScopeRevision     string `gorm:"column:scope_revision"`
	DateKey           int    `gorm:"column:date_key"`
	Status            string `gorm:"column:status"`
	CursorKind        string `gorm:"column:cursor_kind"`
	CursorSiteID      int64  `gorm:"column:cursor_site_id"`
	CursorNodeName    string `gorm:"column:cursor_node_name"`
	CursorBucketStart int64  `gorm:"column:cursor_bucket_start"`
	CursorID          int64  `gorm:"column:cursor_id"`
	SiteID            *int64 `gorm:"column:site_id"`
	SiteConfigVersion *int64 `gorm:"column:site_config_version"`
	RequestID         string `gorm:"column:request_id"`
	RunID             *int64 `gorm:"column:run_id"`
	ErrorCode         string `gorm:"column:error_code"`
	AttemptCount      int    `gorm:"column:attempt_count"`
	NextAttemptAt     int64  `gorm:"column:next_attempt_at"`
	LastAttemptAt     *int64 `gorm:"column:last_attempt_at"`
	LastSuccessAt     *int64 `gorm:"column:last_success_at"`
	LastFailureAt     *int64 `gorm:"column:last_failure_at"`
	CreatedAt         int64  `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt         int64  `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (DataMaintenanceState) TableName() string { return "data_maintenance_state" }

type DataMaintenanceRepository struct{ db *gorm.DB }

func NewDataMaintenanceRepository(db *gorm.DB) *DataMaintenanceRepository {
	return &DataMaintenanceRepository{db: db}
}

func (repository *SiteRepository) EnsureAuthorizationPricingIntent(
	ctx context.Context,
	siteID, configVersion int64,
	requestID string,
	now int64,
) error {
	if repository == nil || repository.db == nil {
		return ErrDataMaintenanceContract
	}
	return NewDataMaintenanceRepository(repository.db).EnsureAuthorizationPricingIntent(
		ctx, siteID, configVersion, requestID, now,
	)
}

func AuthorizationPricingScope(siteID, configVersion int64) string {
	return fmt.Sprintf("site:%d:config:%d", siteID, configVersion)
}

func maintenanceAuthorizationRequestID(siteID, configVersion int64, requestID string) string {
	if validCollectionRequestID(requestID) {
		return requestID
	}
	return fmt.Sprintf("maintenance-pricing-%d-%d", siteID, configVersion)
}

func (repository *DataMaintenanceRepository) EnsureAuthorizationPricingIntent(
	ctx context.Context,
	siteID, configVersion int64,
	requestID string,
	now int64,
) error {
	if repository == nil || repository.db == nil || siteID <= 0 || configVersion <= 0 ||
		now <= 0 {
		return ErrDataMaintenanceContract
	}
	requestID = maintenanceAuthorizationRequestID(siteID, configVersion, requestID)
	scope := AuthorizationPricingScope(siteID, configVersion)
	state := DataMaintenanceState{
		OperationID: MaintenanceAuthorizePricingSync, ScopeKey: scope,
		Status: MaintenanceStatusPending, SiteID: &siteID, SiteConfigVersion: &configVersion,
		RequestID: requestID, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "operation_id"}, {Name: "scope_key"}},
		DoNothing: true,
	}).Create(&state).Error
}

func (repository *DataMaintenanceRepository) LoadStateForUpdate(
	ctx context.Context,
	operationID, scopeKey string,
) (DataMaintenanceState, error) {
	if repository == nil || repository.db == nil || operationID == "" || scopeKey == "" {
		return DataMaintenanceState{}, ErrDataMaintenanceContract
	}
	var state DataMaintenanceState
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("operation_id = ? AND scope_key = ?", operationID, scopeKey).First(&state).Error
	return state, err
}

func (repository *DataMaintenanceRepository) SaveState(
	ctx context.Context,
	state *DataMaintenanceState,
) error {
	if repository == nil || repository.db == nil || state == nil || state.ID <= 0 ||
		state.OperationID == "" || state.ScopeKey == "" || state.UpdatedAt <= 0 {
		return ErrDataMaintenanceContract
	}
	return repository.db.WithContext(ctx).Save(state).Error
}

func (repository *DataMaintenanceRepository) Transaction(
	ctx context.Context,
	operation func(*DataMaintenanceRepository) error,
) error {
	if repository == nil || repository.db == nil || operation == nil {
		return ErrDataMaintenanceContract
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(NewDataMaintenanceRepository(tx))
	})
}

type AuthorizationPricingProcessResult struct {
	Attempted bool
	Completed bool
	RunID     int64
}

// ProcessAuthorizationPricingIntent claims one due durable intent and resolves
// it in the same transaction as collection_run creation. A failed enqueue is
// retained as a safe, retryable diagnostic; no upstream or database error text
// is persisted.
func (repository *DataMaintenanceRepository) ProcessAuthorizationPricingIntent(
	ctx context.Context,
	now int64,
) (AuthorizationPricingProcessResult, error) {
	if repository == nil || repository.db == nil || now <= 0 {
		return AuthorizationPricingProcessResult{}, ErrDataMaintenanceContract
	}
	result := AuthorizationPricingProcessResult{}
	var candidate DataMaintenanceState
	err := repository.db.WithContext(ctx).
		Where("operation_id = ? AND status IN ? AND next_attempt_at <= ?", MaintenanceAuthorizePricingSync,
			[]string{MaintenanceStatusPending, MaintenanceStatusFailed}, now).
		Order("next_attempt_at ASC, id ASC").First(&candidate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state DataMaintenanceState
		var site Site
		if candidate.SiteID == nil || candidate.SiteConfigVersion == nil || candidate.RequestID == "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, candidate.ID).Error; err != nil {
				return err
			}
			result.Attempted = true
			return failAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorEnqueue)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, *candidate.SiteID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, candidate.ID).Error; lockErr != nil {
					return lockErr
				}
				result.Attempted = true
				return completeAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorSiteConfigChanged, 0, &result)
			}
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, candidate.ID).Error; err != nil {
			return err
		}
		if state.OperationID != MaintenanceAuthorizePricingSync ||
			(state.Status != MaintenanceStatusPending && state.Status != MaintenanceStatusFailed) || state.NextAttemptAt > now {
			return nil
		}
		result.Attempted = true
		state.Status = MaintenanceStatusRunning
		state.AttemptCount++
		state.LastAttemptAt = &now
		state.UpdatedAt = now
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		if int64(site.ConfigVersion) != *state.SiteConfigVersion {
			return completeAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorSiteConfigChanged, 0, &result)
		}
		if !authorizationPricingSiteEligible(site) {
			return completeAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorSiteIneligible, 0, &result)
		}
		var capabilities []SiteCapability
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", site.ID).
			Order("capability_key ASC").Find(&capabilities).Error; err != nil {
			return err
		}
		if !maintenanceCapabilitiesReady(capabilities) {
			return failAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorCapabilities)
		}
		run, err := NewSiteCollectionRun(site, SiteRunSpec{
			TaskType: constant.TaskTypePricingSync, TriggerType: constant.CollectionTriggerDependency,
			Priority: 0, RequestID: state.RequestID, Now: now,
		})
		if err != nil {
			return failAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorEnqueue)
		}
		created, _, err := NewSiteRepository(tx).CreateOrGetRun(ctx, &run)
		if err != nil {
			if errors.Is(err, ErrSiteRunConfigChanged) || errors.Is(err, ErrCollectionRunContract) {
				return completeAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorSiteConfigChanged, 0, &result)
			}
			if saveErr := failAuthorizationPricingIntent(tx, &state, now, MaintenanceErrorEnqueue); saveErr != nil {
				return saveErr
			}
			return nil
		}
		return completeAuthorizationPricingIntent(tx, &state, now, "", created.ID, &result)
	})
	return result, err
}

// Authorization pricing eligibility intentionally ignores the volatile online
// status. Authorization may commit just before the immediate probe updates that
// status, and that race must never discard the durable pricing intent.
func authorizationPricingSiteEligible(site Site) bool {
	return site.ConfigVersion > 0 && site.ManagementStatus == constant.SiteManagementActive &&
		site.AuthStatus == constant.SiteAuthAuthorized && site.StatisticsEndAt == nil
}

func maintenanceCapabilitiesReady(capabilities []SiteCapability) bool {
	statuses := make(map[string]string, len(capabilities))
	for _, capability := range capabilities {
		statuses[capability.CapabilityKey] = capability.Status
	}
	for _, key := range constant.SiteCapabilityKeys() {
		status, exists := statuses[key]
		if !exists || status == constant.CapabilityStatusFailed ||
			(status == constant.CapabilityStatusSkipped && key != constant.CapabilityFlowDataConsistency) {
			return false
		}
	}
	return true
}

func failAuthorizationPricingIntent(tx *gorm.DB, state *DataMaintenanceState, now int64, code string) error {
	state.Status = MaintenanceStatusFailed
	state.ErrorCode = code
	state.LastFailureAt = &now
	state.NextAttemptAt = now + int64(time.Minute/time.Second)
	state.UpdatedAt = now
	return tx.Save(state).Error
}

func completeAuthorizationPricingIntent(
	tx *gorm.DB,
	state *DataMaintenanceState,
	now int64,
	code string,
	runID int64,
	result *AuthorizationPricingProcessResult,
) error {
	state.Status = MaintenanceStatusComplete
	state.ErrorCode = code
	state.NextAttemptAt = 0
	state.LastSuccessAt = &now
	state.UpdatedAt = now
	if runID > 0 {
		state.RunID = &runID
		result.RunID = runID
	}
	result.Completed = true
	return tx.Save(state).Error
}
