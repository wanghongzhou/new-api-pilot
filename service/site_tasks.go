package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func (service *SiteService) QueueRefresh(ctx context.Context, siteIDs []int64, requestID string) ([]dto.CollectionRunItem, error) {
	ids := append([]int64(nil), siteIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]dto.CollectionRunItem, 0, len(ids)*3)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		for _, siteID := range ids {
			site, err := repository.FindByIDForUpdate(ctx, siteID)
			if err != nil {
				if model.IsNotFound(err) {
					return ErrSiteNotFound
				}
				return err
			}
			if site.ManagementStatus != constant.SiteManagementActive {
				return ErrSiteInvalidState
			}
			for _, taskType := range []string{
				constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot,
			} {
				run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
					TaskType: taskType, TriggerType: constant.CollectionTriggerManual,
					Priority: 0, RequestID: requestID, Now: service.clock.Now().Unix(),
				})
				if err != nil {
					return err
				}
				created, deduplicated, err := repository.CreateOrGetRun(ctx, &run)
				if err != nil {
					return err
				}
				result = append(result, collectionRunItemFromModel(created, deduplicated))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (service *SiteService) Backfill(ctx context.Context, siteID int64, request dto.SiteBackfillRequest, requestID string) (dto.CollectionRunItem, error) {
	var result dto.CollectionRunItem
	committedAt := int64(0)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		if site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized || site.StatisticsEndAt != nil {
			return ErrSiteInvalidState
		}
		ready, err := requiredCapabilitiesReady(ctx, repository, site.ID)
		if err != nil {
			return err
		}
		if !ready {
			return ErrSiteCapabilitiesPending
		}
		if site.StatisticsStartAt == nil {
			return ErrSiteInvalidState
		}
		now := service.clock.Now().Unix()
		currentHour := floorHour(now)
		start := *site.StatisticsStartAt
		end := currentHour
		if request.StartTimestamp != nil {
			start = *request.StartTimestamp
		}
		if request.EndTimestamp != nil {
			end = *request.EndTimestamp
		}
		maximumDays, err := repository.IntSetting(ctx, "collector.manual_backfill_max_days", 1, 3660)
		if err != nil {
			return err
		}
		if start < *site.StatisticsStartAt || end <= start || end > currentHour ||
			end-start > int64(maximumDays)*24*3600 {
			return ErrSiteInvalidBackfillRange
		}
		scope, err := model.NewUsageBackfillRunScope(request.MissingOnly())
		if err != nil {
			return err
		}
		createdResult, err := repository.CreateSiteWindowRun(ctx, model.SiteWindowRunCreateRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
			TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerManual,
			StartTimestamp: start, EndTimestamp: end, Scope: scope, Priority: constant.CollectionPriorityManualBackfill,
			RequestID: requestID, Now: now, Mode: model.SiteWindowRunStrict,
		})
		if err != nil {
			return siteRunServiceError(err)
		}
		if len(createdResult.Runs) != 1 {
			return model.ErrCollectionRunContract
		}
		created := createdResult.Runs[0].Run
		deduplicated := createdResult.Runs[0].Deduplicated
		if !deduplicated {
			committedAt = monotonicMutationTime(now, site.UpdatedAt)
			site.StatisticsStatus = constant.SiteStatisticsBackfilling
			site.UpdatedAt = committedAt
			if err := repository.Save(ctx, &site); err != nil {
				return err
			}
		}
		result = collectionRunItemFromModel(created, deduplicated)
		return nil
	})
	if err != nil {
		return result, err
	}
	service.notifySiteLifecycleAfterCommit(ctx, siteID, committedAt)
	return result, nil
}

func (service *SiteService) ListCollectionRuns(ctx context.Context, siteID int64, query dto.CollectionRunListQuery) (common.PageData[dto.CollectionRunItem], error) {
	if _, err := service.sites.FindByID(ctx, siteID); err != nil {
		if model.IsNotFound(err) {
			return common.PageData[dto.CollectionRunItem]{}, ErrSiteNotFound
		}
		return common.PageData[dto.CollectionRunItem]{}, err
	}
	runs, total, err := service.sites.ListCollectionRuns(ctx, model.CollectionRunFilter{
		SiteID: &siteID, TaskType: query.TaskType, Status: query.Status,
		SortBy: query.SortBy, SortOrder: query.SortOrder,
		Offset: (query.Page - 1) * query.PageSize, Limit: query.PageSize,
	})
	if err != nil {
		return common.PageData[dto.CollectionRunItem]{}, fmt.Errorf("list site collection runs: %w", err)
	}
	items := make([]dto.CollectionRunItem, 0, len(runs))
	for _, run := range runs {
		items = append(items, collectionRunItemFromModel(run, false))
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *SiteService) GetCollectionRun(ctx context.Context, runID int64) (dto.CollectionRunItem, error) {
	run, err := service.sites.FindCollectionRunByID(ctx, runID)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.CollectionRunItem{}, ErrSiteNotFound
		}
		return dto.CollectionRunItem{}, err
	}
	return collectionRunItemFromModel(run, false), nil
}

func (service *SiteService) ListCollectionRunWindows(ctx context.Context, runID int64, query dto.CollectionRunWindowListQuery) (common.PageData[dto.CollectionRunWindowItem], error) {
	if _, err := service.sites.FindCollectionRunByID(ctx, runID); err != nil {
		if model.IsNotFound(err) {
			return common.PageData[dto.CollectionRunWindowItem]{}, ErrSiteNotFound
		}
		return common.PageData[dto.CollectionRunWindowItem]{}, err
	}
	windows, total, err := service.sites.ListCollectionRunWindows(ctx, model.CollectionRunWindowFilter{
		RunID: runID, Status: query.Status, Offset: (query.Page - 1) * query.PageSize, Limit: query.PageSize,
	})
	if err != nil {
		return common.PageData[dto.CollectionRunWindowItem]{}, fmt.Errorf("list collection run windows: %w", err)
	}
	items := make([]dto.CollectionRunWindowItem, 0, len(windows))
	for _, window := range windows {
		items = append(items, collectionRunWindowItemFromModel(window))
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *SiteService) enqueueInitialBackfill(ctx context.Context, repository *model.SiteRepository, site model.Site, requestID string) (*model.CollectionRun, error) {
	if site.StatisticsStartAt == nil {
		return nil, ErrSiteInvalidState
	}
	start := *site.StatisticsStartAt
	end := floorHour(service.clock.Now().Unix())
	scope, err := model.NewUsageBackfillRunScope(true)
	if err != nil {
		return nil, err
	}
	createdResult, err := repository.CreateSiteWindowRun(ctx, model.SiteWindowRunCreateRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerRecovery,
		StartTimestamp: start, EndTimestamp: end, Scope: scope, Priority: constant.CollectionPriorityInitialBackfill,
		RequestID: requestID, Now: service.clock.Now().Unix(), Mode: model.SiteWindowRunStrict,
	})
	if err != nil {
		return nil, siteRunServiceError(err)
	}
	if len(createdResult.Runs) != 1 {
		return nil, model.ErrCollectionRunContract
	}
	created := createdResult.Runs[0].Run
	if start == end && created.Status == "pending" {
		expectation, expectationErr := model.NewSiteRunWindowMaterializationExpectation(created, nil)
		if expectationErr != nil {
			return nil, siteRunServiceError(expectationErr)
		}
		created, err = repository.CompleteSiteRunWindowMaterialization(
			ctx, site.ID, created.ID, site.ConfigVersion, expectation, service.clock.Now().Unix(),
		)
		if err != nil {
			return nil, siteRunServiceError(err)
		}
	}
	return &created, nil
}

func siteRunServiceError(err error) error {
	switch {
	case model.IsNotFound(err):
		return ErrSiteNotFound
	case errors.Is(err, model.ErrSiteRunConfigChanged):
		return ErrSiteConfigChanged
	case errors.Is(err, model.ErrSiteRunManagementInactive), errors.Is(err, model.ErrSiteRunStatisticsEnded):
		return ErrSiteInvalidState
	case errors.Is(err, model.ErrSiteRunAuthorizationNeeded), errors.Is(err, model.ErrSiteRunCapabilitiesPending):
		return ErrSiteIncompatible
	case errors.Is(err, model.ErrSiteRunExportDisabled):
		return ErrSiteExportDisabled
	case errors.Is(err, model.ErrSiteWindowRunOverlap):
		return ErrSiteTaskOverlap
	default:
		return err
	}
}

func collectionRunItemFromModel(run model.CollectionRun, deduplicated bool) dto.CollectionRunItem {
	item := dto.CollectionRunItem{
		ID: strconv.FormatInt(run.ID, 10), SiteConfigVersion: run.SiteConfigVersion,
		TaskType: run.TaskType, TargetType: run.TargetType, TargetID: strconv.FormatInt(run.TargetID, 10),
		TriggerType: run.TriggerType, StartTimestamp: run.StartTimestamp, EndTimestamp: run.EndTimestamp,
		Status: run.Status, Priority: run.Priority, TotalWindows: run.TotalWindows,
		CompletedWindows: run.CompletedWindows, FailedWindows: run.FailedWindows,
		WindowsInitialized: run.WindowsInitializedAt != nil, CreatedRequestID: run.CreatedRequestID,
		LastRequestID: run.LastRequestID, FetchedRows: strconv.FormatInt(run.FetchedRows, 10),
		WrittenRows: strconv.FormatInt(run.WrittenRows, 10), RetryCount: run.RetryCount,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		CreatedAt: run.CreatedAt, Deduplicated: deduplicated,
	}
	if run.Status == "pending" || run.Status == "running" {
		item.NextAttemptAt = &run.NextAttemptAt
	}
	if run.SiteID != nil {
		value := strconv.FormatInt(*run.SiteID, 10)
		item.SiteID = &value
	}
	terminal := run.CompletedWindows + run.FailedWindows + run.UnavailableWindows
	if run.TotalWindows > 0 {
		item.Progress = float64(terminal) / float64(run.TotalWindows)
	} else if run.Status == "success" {
		item.Progress = 1
	}
	if run.ErrorCode != "" {
		params := map[string]any{}
		if len(run.ErrorParams) > 0 {
			_ = common.Unmarshal(run.ErrorParams, &params)
		}
		message, err := dto.NewMessageRef(constant.MessageCode(run.ErrorCode), params, "")
		if err == nil {
			item.Error = &message
		}
	}
	return item
}

func collectionRunWindowItemFromModel(window model.CollectionRunWindowSnapshot) dto.CollectionRunWindowItem {
	item := dto.CollectionRunWindowItem{
		ID: strconv.FormatInt(window.ID, 10), RunID: strconv.FormatInt(window.RunID, 10),
		SiteID: strconv.FormatInt(window.SiteID, 10), HourTS: window.HourTS, Status: window.Status,
		FactStatus: window.FactStatus, FetchedRows: strconv.FormatInt(window.FetchedRows, 10),
		WrittenRows: strconv.FormatInt(window.WrittenRows, 10), AttemptCount: window.AttemptCount,
		NextRetryAt: window.NextRetryAt, VerifiedAt: window.VerifiedAt, StartedAt: window.StartedAt,
		FinishedAt: window.FinishedAt, UpdatedAt: window.UpdatedAt,
	}
	if window.ErrorCode != "" {
		params := map[string]any{}
		if len(window.ErrorParams) > 0 {
			_ = common.Unmarshal(window.ErrorParams, &params)
		}
		message, err := dto.NewMessageRef(constant.MessageCode(window.ErrorCode), params, "")
		if err == nil {
			item.Error = &message
		}
	}
	return item
}

func requiredCapabilitiesReady(ctx context.Context, repository *model.SiteRepository, siteID int64) (bool, error) {
	capabilities, err := repository.ListCapabilities(ctx, siteID)
	if err != nil {
		return false, err
	}
	statuses := make(map[string]string, len(capabilities))
	for _, capability := range capabilities {
		statuses[capability.CapabilityKey] = capability.Status
	}
	for _, key := range constant.SiteCapabilityKeys() {
		status, exists := statuses[key]
		if !exists || status == constant.CapabilityStatusFailed {
			return false, nil
		}
		if status == constant.CapabilityStatusSkipped && key != constant.CapabilityFlowDataConsistency {
			return false, nil
		}
	}
	return true, nil
}

func floorHour(timestamp int64) int64 {
	return timestamp - timestamp%3600
}

func floorMinute(timestamp int64) int64 {
	return timestamp - timestamp%60
}
