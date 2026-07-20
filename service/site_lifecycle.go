package service

import (
	"context"
	"errors"
	"net/http"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func (service *SiteService) Disable(ctx context.Context, siteID int64) (dto.SiteDetail, error) {
	committedAt := int64(0)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		if site.ManagementStatus == constant.SiteManagementDisabled {
			return nil
		}
		now := service.clock.Now().Unix()
		committedAt = monotonicMutationTime(now, site.UpdatedAt)
		if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
			return err
		}
		disabledAt := floorHour(now)
		site.ManagementStatus = constant.SiteManagementDisabled
		site.StatisticsStatus = constant.SiteStatisticsPaused
		site.DisabledAt = &disabledAt
		site.UpdatedAt = committedAt
		if err := repository.OpenMonitoringPause(ctx, site.ID, floorMinute(now), committedAt); err != nil {
			return err
		}
		return repository.Save(ctx, &site)
	})
	if err != nil {
		return dto.SiteDetail{}, err
	}
	service.notifySiteLifecycleAfterCommit(ctx, siteID, committedAt)
	return service.Get(ctx, siteID)
}

func (service *SiteService) Enable(ctx context.Context, siteID int64, requestID string) (dto.CollectionRunItem, error) {
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
		if site.ManagementStatus == constant.SiteManagementActive {
			run, findErr := repository.LatestActiveRecoveryRun(ctx, site.ID)
			if findErr != nil {
				if model.IsNotFound(findErr) {
					return ErrSiteInvalidState
				}
				return findErr
			}
			result = collectionRunItemFromModel(run, true)
			return nil
		}
		if site.StatisticsEndAt != nil {
			return ErrSiteInvalidState
		}
		if site.AuthStatus != constant.SiteAuthAuthorized || site.StatisticsStartAt == nil {
			return ErrSiteIncompatible
		}
		if !site.DataExportEnabled {
			return ErrSiteExportDisabled
		}
		ready, err := requiredCapabilitiesReady(ctx, repository, site.ID)
		if err != nil {
			return err
		}
		if !ready {
			return ErrSiteIncompatible
		}
		start := *site.StatisticsStartAt
		latestComplete, err := repository.LatestCompleteHour(ctx, site.ID)
		if err != nil {
			return err
		}
		if latestComplete != nil && *latestComplete+3600 > start {
			start = *latestComplete + 3600
		}
		now := service.clock.Now().Unix()
		committedAt = monotonicMutationTime(now, site.UpdatedAt)
		end := floorHour(now)
		if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
			return err
		}
		if err := repository.CloseMonitoringPause(ctx, site.ID, floorMinute(now)); err != nil {
			return err
		}
		site.ManagementStatus = constant.SiteManagementActive
		site.StatisticsStatus = constant.SiteStatisticsBackfilling
		site.UpdatedAt = committedAt
		if err := repository.Save(ctx, &site); err != nil {
			return err
		}
		scope, err := model.NewUsageBackfillRunScope(true)
		if err != nil {
			return err
		}
		createdResult, err := repository.CreateSiteWindowRun(ctx, model.SiteWindowRunCreateRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
			TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerRecovery,
			StartTimestamp: start, EndTimestamp: end, Scope: scope, Priority: constant.CollectionPrioritySiteRecovery,
			RequestID: requestID, Now: committedAt, Mode: model.SiteWindowRunStrict,
		})
		if err != nil {
			return siteRunServiceError(err)
		}
		if len(createdResult.Runs) != 1 {
			return model.ErrCollectionRunContract
		}
		created := createdResult.Runs[0].Run
		deduplicated := createdResult.Runs[0].Deduplicated
		if start == end && created.Status == "pending" {
			expectation, expectationErr := model.NewSiteRunWindowMaterializationExpectation(created, nil)
			if expectationErr != nil {
				return siteRunServiceError(expectationErr)
			}
			created, err = repository.CompleteSiteRunWindowMaterialization(
				ctx, site.ID, created.ID, site.ConfigVersion, expectation, committedAt,
			)
			if err != nil {
				return siteRunServiceError(err)
			}
		}
		for _, taskType := range []string{constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot} {
			run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
				TaskType: taskType, TriggerType: constant.CollectionTriggerRecovery,
				Priority: 0, RequestID: requestID, Now: committedAt,
			})
			if err != nil {
				return err
			}
			if _, _, err := repository.CreateOrGetRun(ctx, &run); err != nil {
				return err
			}
		}
		if created.Status == "success" {
			site.StatisticsStatus = constant.SiteStatisticsReady
			site.DisabledAt = nil
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

func (service *SiteService) EndStatistics(ctx context.Context, siteID int64, statisticsEndAt int64) (dto.SiteDetail, error) {
	committedAt := int64(0)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		if site.ManagementStatus != constant.SiteManagementDisabled || site.DisabledAt == nil || site.StatisticsStartAt == nil {
			return ErrSiteInvalidState
		}
		minimum := *site.StatisticsStartAt
		latestComplete, err := repository.LatestCompleteHour(ctx, site.ID)
		if err != nil {
			return err
		}
		if latestComplete != nil && *latestComplete+3600 > minimum {
			minimum = *latestComplete + 3600
		}
		if statisticsEndAt < minimum || statisticsEndAt > *site.DisabledAt || statisticsEndAt%3600 != 0 {
			return ErrSiteInvalidStatisticsEnd
		}
		if site.StatisticsEndAt != nil && *site.StatisticsEndAt == statisticsEndAt {
			return nil
		}
		site.StatisticsEndAt = &statisticsEndAt
		site.StatisticsStatus = constant.SiteStatisticsPaused
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), site.UpdatedAt)
		site.UpdatedAt = committedAt
		return repository.Save(ctx, &site)
	})
	if err != nil {
		return dto.SiteDetail{}, err
	}
	service.notifySiteLifecycleAfterCommit(ctx, siteID, committedAt)
	return service.Get(ctx, siteID)
}

func (service *SiteService) ClearStatisticsEnd(ctx context.Context, siteID int64) (dto.SiteDetail, error) {
	committedAt := int64(0)
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		if site.ManagementStatus != constant.SiteManagementDisabled {
			return ErrSiteInvalidState
		}
		if site.StatisticsEndAt == nil {
			return nil
		}
		site.StatisticsEndAt = nil
		site.StatisticsStatus = constant.SiteStatisticsPaused
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), site.UpdatedAt)
		site.UpdatedAt = committedAt
		return repository.Save(ctx, &site)
	})
	if err != nil {
		return dto.SiteDetail{}, err
	}
	service.notifySiteLifecycleAfterCommit(ctx, siteID, committedAt)
	return service.Get(ctx, siteID)
}

func (service *SiteService) notifySiteLifecycleAfterCommit(ctx context.Context, siteID int64, observedAt int64) {
	if service.postCommit == nil || observedAt <= 0 {
		return
	}
	service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
		Source: AlertSampleSourceLifecycle, ScopeType: "site", ScopeID: siteID, ObservedAt: observedAt,
	})
}

func (service *SiteService) Probe(ctx context.Context, siteID int64, requestID string) (dto.SiteProbeResult, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteProbeResult{}, ErrSiteNotFound
		}
		return dto.SiteProbeResult{}, err
	}
	return service.probeWithSnapshot(ctx, site, site.ConfigVersion, requestID, false)
}

func (service *SiteService) probeWithSnapshot(
	ctx context.Context,
	site model.Site,
	expectedConfigVersion int,
	requestID string,
	periodic bool,
) (dto.SiteProbeResult, error) {
	now := service.clock.Now().Unix()
	committedAt := int64(0)
	dataExportChanged := false
	fenceChanged := false
	status := dto.UpstreamStatus{}
	probeErr := error(nil)
	client, clientErr := service.clients.NewPublic(site.BaseURL)
	if clientErr != nil {
		probeErr = clientErr
	} else {
		defer client.CloseIdleConnections()
		status, probeErr = client.Status(ctx, requestID)
	}
	reached := probeReachedUpstream(probeErr)
	contractStatus := "unavailable"
	if reached {
		contractStatus = "compatible"
		if probeErr != nil {
			contractStatus = "incompatible"
		}
	}
	result := dto.SiteProbeResult{ProbeSuccess: reached, ContractStatus: contractStatus, ProbedAt: now}
	if status.Version != "" {
		result.Version = stringPointer(status.Version)
		result.SystemName = stringPointer(status.SystemName)
		exportEnabled := status.DataExportEnabled
		result.DataExportEnabled = &exportEnabled
	}
	err := service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, lockErr := repository.FindByIDForUpdate(ctx, site.ID)
		if lockErr != nil {
			if model.IsNotFound(lockErr) {
				return ErrSiteNotFound
			}
			return lockErr
		}
		if current.ConfigVersion != expectedConfigVersion || current.ConfigVersion != site.ConfigVersion ||
			current.BaseURL != site.BaseURL || !sameOptionalInt64(current.RootUserID, site.RootUserID) ||
			!sameOptionalString(current.AccessTokenEncrypted, site.AccessTokenEncrypted) {
			if periodic {
				return model.ErrSiteRunConfigChanged
			}
			return ErrSiteConfigChanged
		}
		if periodic && (current.ManagementStatus != constant.SiteManagementActive || current.StatisticsEndAt != nil) {
			return model.ErrSiteRunConfigChanged
		}
		committedAt = monotonicMutationTime(now, current.UpdatedAt)
		current.LastProbeAt = &committedAt
		if !reached {
			current.ProbeFailCount++
			if current.ProbeFailCount >= 3 {
				current.OnlineStatus = constant.SiteOnlineOffline
			}
			current.UpdatedAt = committedAt
			result.OnlineStatus = current.OnlineStatus
			return repository.Save(ctx, &current)
		}
		current.OnlineStatus = constant.SiteOnlineOnline
		current.ProbeFailCount = 0
		current.LastProbeSuccessAt = &committedAt
		capabilityResults := service.probeCapabilityResults(current.ID, status, probeErr)
		existing, err := repository.ListCapabilities(ctx, current.ID)
		if err != nil {
			return err
		}
		if probeCapabilityFailureTransition(current, existing, capabilityResults) {
			fenceChanged = true
			if err := repository.BumpSiteFence(ctx, &current, committedAt); err != nil {
				return err
			}
			if current.ManagementStatus == constant.SiteManagementDisabled {
				current.StatisticsStatus = constant.SiteStatisticsPaused
			} else if probeHasOnlyConfigurationFailures(capabilityResults) {
				current.StatisticsStatus = constant.SiteStatisticsPendingConfig
			} else {
				current.StatisticsStatus = constant.SiteStatisticsError
			}
		}
		if status.Version != "" {
			dataExportChanged = current.DataExportEnabled != status.DataExportEnabled
			current.Version = status.Version
			current.SystemName = status.SystemName
			current.DataExportEnabled = status.DataExportEnabled
			quota := status.QuotaPerUnit
			rate := status.USDExchangeRate
			current.QuotaPerUnit = &quota
			current.USDExchangeRate = &rate
			current.LastRateAt = &committedAt
		}
		capabilityResults = preserveFailedProbeCapabilities(existing, capabilityResults)
		models, err := capabilityModels(current.ID, capabilityResults, committedAt)
		if err != nil {
			return err
		}
		if err := repository.UpsertCapabilities(ctx, current.ID, models); err != nil {
			return err
		}
		current.UpdatedAt = committedAt
		result.OnlineStatus = current.OnlineStatus
		return repository.Save(ctx, &current)
	})
	if err != nil {
		return result, err
	}
	result.ProbedAt = committedAt
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceProbe, SiteID: site.ID, ObservedAt: committedAt,
		})
		if dataExportChanged || fenceChanged {
			service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
				Source: AlertSampleSourceLifecycle, ScopeType: "site", ScopeID: site.ID, ObservedAt: committedAt,
			})
		}
	}
	return result, nil
}

func probeReachedUpstream(err error) bool {
	if err == nil || errors.Is(err, ErrUpstreamExportDisabled) {
		return true
	}
	var requestErr *UpstreamRequestError
	if errors.As(err, &requestErr) && requestErr.StatusCode != 0 {
		return requestErr.StatusCode >= http.StatusOK && requestErr.StatusCode < http.StatusMultipleChoices
	}
	return errors.Is(err, ErrUpstreamResponseInvalid) || errors.Is(err, ErrUpstreamEnvelopeInvalid) ||
		errors.Is(err, ErrUpstreamResponseTooLarge)
}

func (service *SiteService) probeCapabilityResults(siteID int64, status dto.UpstreamStatus, probeErr error) []dto.SiteCapabilityResult {
	states := map[string]capabilityCheck{
		constant.CapabilityStatusContract:    {status: constant.CapabilityStatusPassed},
		constant.CapabilityDataExportEnabled: {status: constant.CapabilityStatusPassed},
	}
	switch {
	case probeErr == nil:
	case errors.Is(probeErr, ErrUpstreamExportDisabled):
		states[constant.CapabilityDataExportEnabled] = capabilityCheck{status: constant.CapabilityStatusFailed, err: probeErr}
	default:
		for _, key := range []string{constant.CapabilityStatusContract, constant.CapabilityDataExportEnabled} {
			states[key] = capabilityCheck{status: constant.CapabilityStatusFailed, err: probeErr}
		}
	}
	results := make([]dto.SiteCapabilityResult, 0, len(states))
	for _, key := range []string{constant.CapabilityStatusContract, constant.CapabilityDataExportEnabled} {
		state := states[key]
		results = append(results, service.capabilityResult(siteID, key, state.status, state.err, status))
	}
	return results
}

func probeCapabilityFailureTransition(site model.Site, existing []model.SiteCapability, results []dto.SiteCapabilityResult) bool {
	statuses := make(map[string]string, len(existing))
	for _, capability := range existing {
		statuses[capability.CapabilityKey] = capability.Status
	}
	for _, result := range results {
		if result.Status != constant.CapabilityStatusFailed || statuses[result.Key] == constant.CapabilityStatusFailed {
			continue
		}
		if statuses[result.Key] == constant.CapabilityStatusPassed {
			return true
		}
		if site.AuthStatus == constant.SiteAuthAuthorized {
			switch result.Key {
			case constant.CapabilityStatusContract:
				return site.Version != ""
			case constant.CapabilityDataExportEnabled:
				return site.DataExportEnabled
			}
		}
	}
	return false
}

func preserveFailedProbeCapabilities(existing []model.SiteCapability, results []dto.SiteCapabilityResult) []dto.SiteCapabilityResult {
	failed := make(map[string]struct{}, len(existing))
	for _, capability := range existing {
		if capability.Status == constant.CapabilityStatusFailed {
			failed[capability.CapabilityKey] = struct{}{}
		}
	}
	filtered := results[:0]
	for _, result := range results {
		if _, exists := failed[result.Key]; exists && result.Status == constant.CapabilityStatusPassed {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func probeHasOnlyConfigurationFailures(results []dto.SiteCapabilityResult) bool {
	failure := false
	for _, result := range results {
		if result.Status != constant.CapabilityStatusFailed {
			continue
		}
		failure = true
		if result.Key != constant.CapabilityDataExportEnabled {
			return false
		}
	}
	return failure
}
