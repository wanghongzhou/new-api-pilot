package service

import (
	"context"
	"errors"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

var ErrDataMaintenanceInvalid = errors.New("data maintenance service is invalid")

type AuthorizationPricingIntentProcessor interface {
	ProcessAuthorizationPricingIntent(context.Context, int64) (model.AuthorizationPricingProcessResult, error)
	RedactCollectionRunErrors(context.Context, int, int64, int, int64) (model.DataMaintenanceBatchResult, error)
	CleanupMetadataDiagnosticRuns(context.Context, int, int64, int, int64) (model.DataMaintenanceBatchResult, error)
	RepairResourceRollupGaps(context.Context, int, int64, int64, int, int64) (model.ResourceMaintenanceBatchResult, error)
	FinalizeResourceDaily(context.Context, int, int64, int64, int, int64) (model.ResourceMaintenanceBatchResult, error)
}

type ScheduledDataMaintenanceResult struct {
	GapRepair       model.ResourceMaintenanceBatchResult
	DailyFinalize   model.ResourceMaintenanceBatchResult
	ErrorRedaction  model.DataMaintenanceBatchResult
	MetadataCleanup model.DataMaintenanceBatchResult
}

func (service *DataMaintenanceService) RunScheduledMaintenance(ctx context.Context) (ScheduledDataMaintenanceResult, error) {
	if service == nil || service.repository == nil || service.clock == nil {
		return ScheduledDataMaintenanceResult{}, ErrDataMaintenanceInvalid
	}
	now := service.clock.Now()
	unix := now.Unix()
	if unix <= 0 {
		return ScheduledDataMaintenanceResult{}, ErrDataMaintenanceInvalid
	}
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(beijing)
	dateKey := local.Year()*10000 + int(local.Month())*100 + local.Day()
	previous := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, beijing).AddDate(0, 0, -1)
	previousDateKey := previous.Year()*10000 + int(previous.Month())*100 + previous.Day()
	previousStart, previousEnd := previous.Unix(), previous.AddDate(0, 0, 1).Unix()
	result := ScheduledDataMaintenanceResult{}
	var resultErr error
	if local.Hour() >= 3 {
		finalize, err := service.repository.FinalizeResourceDaily(ctx, previousDateKey, previousStart, previousEnd, 100, unix)
		result.DailyFinalize = finalize
		resultErr = errors.Join(resultErr, err)
	}
	if local.Hour() > 3 || (local.Hour() == 3 && local.Minute() >= 20) {
		gap, err := service.repository.RepairResourceRollupGaps(ctx, previousDateKey, previousStart, previousEnd, 100, unix)
		result.GapRepair = gap
		resultErr = errors.Join(resultErr, err)
	}
	if local.Hour() >= 4 {
		redaction, err := service.repository.RedactCollectionRunErrors(ctx, dateKey, unix-90*24*60*60, 500, unix)
		result.ErrorRedaction = redaction
		resultErr = errors.Join(resultErr, err)
		cleanup, err := service.repository.CleanupMetadataDiagnosticRuns(ctx, dateKey, unix-30*24*60*60, 500, unix)
		result.MetadataCleanup = cleanup
		resultErr = errors.Join(resultErr, err)
	}
	return result, resultErr
}

type DataMaintenanceService struct {
	repository AuthorizationPricingIntentProcessor
	clock      common.Clock
}

func NewDataMaintenanceService(repository AuthorizationPricingIntentProcessor, clock common.Clock) (*DataMaintenanceService, error) {
	if repository == nil || clock == nil {
		return nil, ErrDataMaintenanceInvalid
	}
	return &DataMaintenanceService{repository: repository, clock: clock}, nil
}

func (service *DataMaintenanceService) ProcessAuthorizationPricingIntent(ctx context.Context) (model.AuthorizationPricingProcessResult, error) {
	if service == nil || service.repository == nil || service.clock == nil {
		return model.AuthorizationPricingProcessResult{}, ErrDataMaintenanceInvalid
	}
	return service.repository.ProcessAuthorizationPricingIntent(ctx, service.clock.Now().Unix())
}
