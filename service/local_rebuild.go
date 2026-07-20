package service

import (
	"context"
	"errors"
	"fmt"
	"math"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type LocalRebuildServiceOptions struct {
	Database *gorm.DB
	Clock    common.Clock
}

type LocalRebuildService struct {
	database *gorm.DB
	clock    common.Clock
}

type LocalRebuildRequest struct {
	Run       model.CollectionRun
	Window    model.CollectionRunWindow
	RequestID string
}

func NewLocalRebuildService(options LocalRebuildServiceOptions) (*LocalRebuildService, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("local rebuild dependencies are required")
	}
	return &LocalRebuildService{database: options.Database, clock: options.Clock}, nil
}

func (service *LocalRebuildService) PrepareWindow(
	ctx context.Context,
	request LocalRebuildRequest,
) (model.LocalRebuildMutation, error) {
	run := request.Run
	window := request.Window
	if service == nil || service.database == nil || service.clock == nil || run.ID <= 0 || window.ID <= 0 ||
		window.RunID != run.ID || window.SiteID <= 0 || window.HourTS <= 0 || window.HourTS%3600 != 0 ||
		window.HourTS > math.MaxInt64-3600 || window.Status != model.CollectionTaskStatusRunning ||
		run.Status != model.CollectionTaskStatusRunning || run.LastRequestID != request.RequestID || request.RequestID == "" ||
		(run.TaskType != constant.TaskTypeAccountRebuild && run.TaskType != constant.TaskTypeCustomerRebuild) {
		return model.LocalRebuildMutation{}, model.ErrCollectionRunContract
	}
	now := service.clock.Now().Unix()
	if now <= 0 {
		return model.LocalRebuildMutation{}, model.ErrCollectionRunContract
	}
	var factWindow model.CollectionWindow
	err := service.database.WithContext(ctx).Where("site_id = ? AND hour_ts = ?", window.SiteID, window.HourTS).
		Take(&factWindow).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.LocalRebuildMutation{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || factWindow.Status != model.CollectionWindowStatusComplete {
		if err := service.enqueueDependency(ctx, request, now); err != nil {
			return model.LocalRebuildMutation{}, err
		}
		return model.LocalRebuildMutation{}, model.ErrLocalRebuildDependencyPending
	}
	return model.NewLocalRebuildMutation(model.LocalRebuildMutationRequest{
		RunID: run.ID, WindowID: window.ID, SiteID: window.SiteID, HourTS: window.HourTS,
		AttemptCount: window.AttemptCount, RequestID: request.RequestID, Now: now,
		TaskType: run.TaskType, TargetID: run.TargetID,
	})
}

func (service *LocalRebuildService) enqueueDependency(
	ctx context.Context,
	request LocalRebuildRequest,
	now int64,
) error {
	site, err := model.NewSiteRepository(service.database).FindByID(ctx, request.Window.SiteID)
	if err != nil {
		return err
	}
	scope, err := model.NewUsageBackfillRunScope(true)
	if err != nil {
		return err
	}
	start := request.Window.HourTS
	end := start + 3600
	requestID := fmt.Sprintf("dep_%d_%d", request.Run.ID, request.Window.ID)
	_, _, err = model.NewCollectionTaskRepository(service.database).EnqueueSiteTask(ctx, model.SiteTaskEnqueueRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerDependency,
		StartTimestamp: &start, EndTimestamp: &end, Scope: scope,
		Priority: constant.CollectionPrioritySiteRecovery, RequestID: requestID,
		Now: now, Mode: model.SiteWindowRunStrict,
	})
	if errors.Is(err, model.ErrSiteWindowRunOverlap) {
		return nil
	}
	return err
}
