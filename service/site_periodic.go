package service

import (
	"context"
	"errors"
	"strings"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

// ExecutePeriodicSiteTask is the worker integration boundary for current-state
// and metadata jobs. Usage facts and aggregation are intentionally not handled
// here.
func (service *SiteService) ExecutePeriodicSiteTask(
	ctx context.Context,
	taskType string,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	if siteID <= 0 || expectedConfigVersion <= 0 || requestID == "" {
		return 0, 0, model.ErrCollectionRunContract
	}
	switch taskType {
	case constant.TaskTypeSiteProbe:
		return service.executePeriodicProbe(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeRealtimeStat:
		return service.executePeriodicRealtime(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeResourceSnapshot:
		return service.executePeriodicResources(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeUserSync:
		return service.executePeriodicUsers(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeChannelSync:
		return service.executePeriodicChannels(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypePerformanceSync:
		return service.executePeriodicPerformance(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeTopupSync:
		return service.executePeriodicTopups(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeRedemptionSync:
		return service.executePeriodicRedemptions(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeUpstreamTaskSync:
		return service.executePeriodicUpstreamTasks(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeModelMetaSync:
		return service.executePeriodicModelMeta(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypePlanSync:
		return service.executePeriodicPlans(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypePricingSync:
		return service.executePeriodicPricing(ctx, siteID, expectedConfigVersion, requestID)
	case constant.TaskTypeSystemTaskSync:
		return service.executePeriodicSystemTasks(ctx, siteID, expectedConfigVersion, requestID)
	default:
		return 0, 0, model.ErrCollectionRunContract
	}
}

type performanceHistoryClient interface {
	PerformanceHistory(context.Context, string, int) (dto.UpstreamPerformanceHistory, error)
}

type topupSnapshotClient interface {
	SnapshotTopups(context.Context, string) (dto.UpstreamTopupSnapshot, error)
}
type redemptionSnapshotClient interface {
	SnapshotRedemptions(context.Context, string) (dto.UpstreamRedemptionSnapshot, error)
}
type upstreamTaskSnapshotClient interface {
	SnapshotUpstreamTasks(context.Context, string, int64, int64, []string) (dto.UpstreamTaskSnapshot, error)
}
type modelMetaSnapshotClient interface {
	SnapshotModelMeta(context.Context, string) (dto.UpstreamModelMetaSnapshot, error)
}
type subscriptionPlanSnapshotClient interface {
	SnapshotSubscriptionPlans(context.Context, string) (dto.UpstreamSubscriptionPlanSnapshot, error)
}
type pricingCatalogSnapshotClient interface {
	SnapshotPricingGroups(context.Context, string) (dto.UpstreamPricingGroupSnapshot, error)
	SnapshotPricing(context.Context, string) (dto.UpstreamPricingOnlySnapshot, error)
}
type systemTaskSnapshotClient interface {
	SnapshotSystemTasks(context.Context, string) (dto.UpstreamSystemTaskSnapshot, error)
}

func (service *SiteService) executePeriodicSystemTasks(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(systemTaskSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	snapshot, err := source.SnapshotSystemTasks(ctx, requestID)
	now := service.clock.Now().Unix()
	if err != nil {
		_ = service.sites.MarkSystemTaskCollectionFailure(ctx, site, now, "SYSTEM_TASK_UPSTREAM_UNAVAILABLE")
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	var written int64
	err = service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
		current, e := r.FindByIDForUpdate(ctx, site.ID)
		if e != nil {
			return e
		}
		if e = validatePeriodicCommit(current, site, expectedConfigVersion); e != nil {
			return e
		}
		written, e = r.SyncSystemTasks(ctx, current, now, snapshot)
		return e
	})
	if err != nil {
		return int64(len(snapshot.Items)), written, err
	}
	retention, e := service.sites.IntSetting(ctx, "system_task_terminal_retention_days", 1, 3650)
	if e != nil {
		return int64(len(snapshot.Items)), written, e
	}
	if e = service.sites.DeleteTerminalSystemTasksBefore(ctx, now-int64(retention)*86400); e != nil {
		return int64(len(snapshot.Items)), written, e
	}
	return int64(len(snapshot.Items)), written, nil
}

func (service *SiteService) executePeriodicPricing(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(pricingCatalogSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	now := service.clock.Now().Unix()
	groups, groupErr := source.SnapshotPricingGroups(ctx, requestID+"_groups")
	pricing, pricingErr := source.SnapshotPricing(ctx, requestID+"_pricing")
	var fetched, written int64
	commit := func(kind string, apply func(*model.SiteRepository, model.Site) (int64, error), sourceErr error) error {
		if sourceErr != nil {
			_ = service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
				return r.MarkPricingResourceFailure(ctx, site, now, kind, "PRICING_"+strings.ToUpper(kind)+"_UPSTREAM_UNAVAILABLE")
			})
			return service.periodicTaskError(ctx, site.ID, expectedConfigVersion, sourceErr)
		}
		return service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
			current, err := r.FindByIDForUpdate(ctx, site.ID)
			if err != nil {
				return err
			}
			if err = validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
				return err
			}
			count, err := apply(r, current)
			written += count
			return err
		})
	}
	groupCommitErr := commit("group", func(r *model.SiteRepository, current model.Site) (int64, error) {
		fetched += int64(len(groups.Groups))
		return r.SyncPricingGroups(ctx, current, now, groups)
	}, groupErr)
	pricingCommitErr := commit("pricing", func(r *model.SiteRepository, current model.Site) (int64, error) {
		fetched += int64(len(pricing.Items))
		return r.SyncPricingItems(ctx, current, now, pricing)
	}, pricingErr)
	return fetched, written, errors.Join(groupCommitErr, pricingCommitErr)
}

func (service *SiteService) executePeriodicPlans(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(subscriptionPlanSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	snapshot, err := source.SnapshotSubscriptionPlans(ctx, requestID)
	now := service.clock.Now().Unix()
	if err != nil {
		_ = service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
			return r.MarkSubscriptionPlanFailure(ctx, site, now, "PLAN_UPSTREAM_UNAVAILABLE")
		})
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	var written int64
	err = service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
		current, err := r.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err = validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		written, err = r.SyncSubscriptionPlans(ctx, current, now, snapshot)
		return err
	})
	return int64(len(snapshot.Items)), written, err
}

func (service *SiteService) executePeriodicModelMeta(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(modelMetaSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	snapshot, err := source.SnapshotModelMeta(ctx, requestID)
	if err != nil {
		now := service.clock.Now().Unix()
		_ = service.sites.WithTransaction(ctx, func(r *model.SiteRepository) error {
			return r.MarkModelCatalogFailure(ctx, site, now, "MODEL_META_UPSTREAM_UNAVAILABLE")
		})
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	now := service.clock.Now().Unix()
	var written int64
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err = validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		written, err = repository.SyncModelCatalog(ctx, current, now, snapshot)
		return err
	})
	return snapshot.Total, written, err
}

func (service *SiteService) executePeriodicUpstreamTasks(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(upstreamTaskSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	unfinished, err := service.sites.ListUnfinishedUpstreamTaskIDs(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	now := service.clock.Now().Unix()
	overlap := now - 48*3600
	snapshot, err := source.SnapshotUpstreamTasks(ctx, requestID, overlap, now+1, unfinished)
	if err != nil {
		_ = service.sites.MarkUpstreamTaskCollectionFailure(ctx, site, now, "UPSTREAM_TASK_UNAVAILABLE")
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	var written int64
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		written, err = repository.SyncUpstreamTasks(ctx, current, now, overlap, snapshot)
		return err
	})
	if err != nil {
		return int64(len(snapshot.Items)), 0, err
	}
	retention, err := service.sites.IntSetting(ctx, "task.retention_days", 1, 3650)
	if err != nil {
		return int64(len(snapshot.Items)), written, err
	}
	if err := service.sites.DeleteTerminalUpstreamTasksBefore(ctx, now-int64(retention)*86400); err != nil {
		return int64(len(snapshot.Items)), written, err
	}
	return int64(len(snapshot.Items)), written, nil
}

func (service *SiteService) executePeriodicTopups(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(topupSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	snapshot, err := source.SnapshotTopups(ctx, requestID)
	if err != nil {
		_ = service.sites.MarkFinanceCollectionFailure(ctx, site, service.clock.Now().Unix(), "topup", "TOPUP_UPSTREAM_UNAVAILABLE")
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	now := service.clock.Now().Unix()
	var written int64
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		written, err = repository.SyncTopups(ctx, current, now, snapshot)
		return err
	})
	if err != nil {
		return snapshot.Total, 0, err
	}
	return snapshot.Total, written, nil
}

func (service *SiteService) executePeriodicRedemptions(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	source, ok := client.(redemptionSnapshotClient)
	if !ok {
		return 0, 0, model.ErrCollectionRunContract
	}
	snapshot, err := source.SnapshotRedemptions(ctx, requestID)
	if err != nil {
		_ = service.sites.MarkFinanceCollectionFailure(ctx, site, service.clock.Now().Unix(), "redemption", "REDEMPTION_UPSTREAM_UNAVAILABLE")
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	now := service.clock.Now().Unix()
	var written int64
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		written, err = repository.SyncRedemptions(ctx, current, now, snapshot)
		return err
	})
	if err != nil {
		return snapshot.Total, 0, err
	}
	return snapshot.Total, written, nil
}

func (service *SiteService) executePeriodicPerformance(ctx context.Context, siteID int64, expectedConfigVersion int, requestID string) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	historyClient, ok := client.(performanceHistoryClient)
	if !ok {
		now := service.clock.Now().Unix()
		_ = service.sites.MarkPerformanceUnavailable(ctx, site, now, "PERFORMANCE_COUNTER_API_UNAVAILABLE")
		return 0, 0, model.ErrCollectionRunContract
	}
	history, err := historyClient.PerformanceHistory(ctx, requestID, 24)
	now := service.clock.Now().Unix()
	if err != nil {
		_ = service.sites.MarkPerformanceUnavailable(ctx, site, now, "PERFORMANCE_UPSTREAM_UNAVAILABLE")
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	start := floorHour(now - 24*3600)
	written, err := service.sites.ApplyPerformanceHistorySnapshot(ctx, site, now, start, now+1, history)
	if err != nil {
		return int64(len(history.Models)), 0, err
	}
	retention, settingErr := service.sites.IntSetting(ctx, "performance.retention_days", 1, 3650)
	if settingErr != nil {
		return int64(len(history.Models)), written, settingErr
	}
	if err := service.sites.DeletePerformanceBefore(ctx, now-int64(retention)*86400); err != nil {
		return int64(len(history.Models)), written, err
	}
	return int64(len(history.Models)), written, nil
}

func (service *SiteService) executePeriodicProbe(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	if err := validatePeriodicProbeSnapshot(site, expectedConfigVersion); err != nil {
		return 0, 0, model.ErrSiteRunConfigChanged
	}
	if _, err := service.probeWithSnapshot(ctx, site, expectedConfigVersion, requestID, true); err != nil {
		return 1, 0, err
	}
	return 1, 1, nil
}

func (service *SiteService) executePeriodicRealtime(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	stat, err := client.LogStat(ctx, requestID)
	if err != nil {
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	now := service.clock.Now().Unix()
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		committedAt := monotonicMutationTime(now, current.UpdatedAt)
		current.CurrentRPM = stat.RPM
		current.CurrentTPM = stat.TPM
		current.LastRealtimeStatAt = &now
		current.UpdatedAt = committedAt
		return repository.Save(ctx, &current)
	})
	if err != nil {
		return 1, 0, err
	}
	return 1, 1, nil
}

func (service *SiteService) executePeriodicChannels(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	snapshot, err := client.SnapshotChannels(ctx, requestID)
	if err != nil {
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	now := service.clock.Now().Unix()
	committedAt := now
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		committedAt = monotonicMutationTime(now, current.UpdatedAt)
		return repository.SyncChannels(ctx, current.ID, committedAt, channelModels(snapshot))
	})
	if err != nil {
		return snapshot.Total, 0, err
	}
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceChannel, SiteID: siteID,
			HourTS: floorHour(committedAt), ObservedAt: committedAt,
		})
	}
	return snapshot.Total, int64(len(snapshot.Items)), nil
}

func (service *SiteService) executePeriodicUsers(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	snapshot, err := client.SnapshotUsers(ctx, requestID)
	if err != nil {
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	observations := siteUserObservations(snapshot)
	now := service.clock.Now().Unix()
	updated, err := service.sites.ApplySiteUserSnapshot(ctx, site, now, floorHour(now), observations)
	if err != nil {
		return snapshot.Total, 0, err
	}
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceUser, SiteID: siteID, ObservedAt: now,
		})
	}
	return snapshot.Total, updated, nil
}

func (service *SiteService) executePeriodicResources(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	site, client, err := service.periodicAuthenticatedClient(ctx, siteID, expectedConfigVersion)
	if err != nil {
		return 0, 0, err
	}
	defer client.CloseIdleConnections()
	instances, err := client.Instances(ctx, requestID)
	if err != nil {
		return 0, 0, service.periodicTaskError(ctx, site.ID, expectedConfigVersion, err)
	}
	staleSeconds, err := service.sites.EffectiveInstanceStaleSeconds(ctx, siteID)
	if err != nil {
		return int64(len(instances)), 0, err
	}
	now := service.clock.Now().Unix()
	var writes []model.SiteInstanceWrite
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		current, err := repository.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		if err := validatePeriodicCommit(current, site, expectedConfigVersion); err != nil {
			return err
		}
		committedAt := monotonicMutationTime(now, current.UpdatedAt)
		known, err := repository.ListInstanceResourceStatesForUpdate(ctx, siteID, floorMinute(now))
		if err != nil {
			return err
		}
		var sample model.SiteStatusMinutely
		var health string
		writes, sample, health = periodicResourceModels(siteID, now, int64(staleSeconds), instances, known)
		for index := range writes {
			writes[index].Instance.LastSyncedAt = committedAt
			writes[index].Instance.UpdatedAt = committedAt
			writes[index].Sample.CreatedAt = committedAt
		}
		sample.CreatedAt = committedAt
		if current.MonitoringStartAt == nil {
			minute := floorMinute(now)
			current.MonitoringStartAt = &minute
		}
		nodeNames := make([]string, 0, len(instances))
		for _, instance := range instances {
			nodeNames = append(nodeNames, instance.NodeName)
		}
		if err := repository.RetireMissingInstances(ctx, siteID, committedAt, nodeNames); err != nil {
			return err
		}
		if err := repository.SyncInstances(ctx, writes); err != nil {
			return err
		}
		if err := repository.UpsertSiteStatusMinute(ctx, sample); err != nil {
			return err
		}
		current.HealthStatus = health
		current.UpdatedAt = committedAt
		return repository.Save(ctx, &current)
	})
	if err != nil {
		return int64(len(instances)), 0, err
	}
	if service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceResource, SiteID: siteID, ObservedAt: floorMinute(now),
		})
	}
	return int64(len(instances)), int64(len(writes) + 1), nil
}

func (service *SiteService) periodicAuthenticatedClient(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
) (model.Site, SiteUpstreamClient, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return model.Site{}, nil, err
	}
	if site.ConfigVersion != expectedConfigVersion || site.ManagementStatus != constant.SiteManagementActive ||
		site.AuthStatus != constant.SiteAuthAuthorized || site.StatisticsEndAt != nil ||
		site.RootUserID == nil || site.AccessTokenEncrypted == nil {
		return model.Site{}, nil, model.ErrSiteRunConfigChanged
	}
	plaintext, err := service.cipher.Decrypt(*site.AccessTokenEncrypted, siteTokenAAD(site.ID))
	if err != nil {
		if expireErr := expireSiteAuthorization(
			ctx, service.sites, service.clock, service.postCommit, site.ID, expectedConfigVersion,
		); expireErr != nil {
			return model.Site{}, nil, expireErr
		}
		return model.Site{}, nil, ErrUpstreamAuthExpired
	}
	client, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, string(plaintext), *site.RootUserID)
	if err != nil {
		if upstreamAuthorizationFailure(err) {
			if expireErr := expireSiteAuthorization(
				ctx, service.sites, service.clock, service.postCommit, site.ID, expectedConfigVersion,
			); expireErr != nil {
				return model.Site{}, nil, expireErr
			}
		}
		return model.Site{}, nil, err
	}
	return site, client, nil
}

func (service *SiteService) periodicTaskError(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
	cause error,
) error {
	if !upstreamAuthorizationFailure(cause) {
		return cause
	}
	if err := expireSiteAuthorization(
		ctx, service.sites, service.clock, service.postCommit, siteID, expectedConfigVersion,
	); err != nil {
		return err
	}
	return cause
}

func siteUserObservations(snapshot dto.UpstreamUserSnapshot) []model.SiteUserObservation {
	observations := make([]model.SiteUserObservation, 0, len(snapshot.Items))
	for _, user := range snapshot.Items {
		observations = append(observations, model.SiteUserObservation{
			RemoteUserID: user.ID, RemoteCreatedAt: user.CreatedAt,
			Username: user.Username, DisplayName: user.DisplayName, RemoteGroup: user.Group,
			RemoteStatus: int(user.Status), Quota: user.Quota, UsedQuota: user.UsedQuota,
			RemoteRole: int(user.Role), RequestCount: user.RequestCount, LastLoginAt: user.LastLoginAt, Deleted: user.Deleted,
		})
	}
	return observations
}

func validatePeriodicCommit(current, original model.Site, expectedConfigVersion int) error {
	if current.ConfigVersion != expectedConfigVersion || current.ConfigVersion != original.ConfigVersion ||
		current.BaseURL != original.BaseURL || current.ManagementStatus != constant.SiteManagementActive ||
		current.AuthStatus != constant.SiteAuthAuthorized || current.StatisticsEndAt != nil ||
		!sameOptionalInt64(current.RootUserID, original.RootUserID) ||
		!sameOptionalString(current.AccessTokenEncrypted, original.AccessTokenEncrypted) {
		return model.ErrSiteRunConfigChanged
	}
	return nil
}

func validatePeriodicProbeSnapshot(site model.Site, expectedConfigVersion int) error {
	if site.ConfigVersion != expectedConfigVersion || site.ManagementStatus != constant.SiteManagementActive ||
		site.StatisticsEndAt != nil {
		return model.ErrSiteRunConfigChanged
	}
	return nil
}

func sameOptionalInt64(first, second *int64) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

func sameOptionalString(first, second *string) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return *first == *second
}

func periodicResourceModels(
	siteID int64,
	now int64,
	staleSeconds int64,
	instances []dto.UpstreamInstance,
	known []model.SiteInstanceResourceState,
) ([]model.SiteInstanceWrite, model.SiteStatusMinutely, string) {
	writes := instanceModels(siteID, now, instances)
	knownByNode := make(map[string]model.SiteInstanceResourceState, len(known))
	for _, state := range known {
		knownByNode[state.Instance.NodeName] = state
	}
	seen := make(map[string]struct{}, len(instances))
	online := 0
	var cpuValues, memoryValues, diskValues []float64
	for index, instance := range instances {
		seen[instance.NodeName] = struct{}{}
		status := "stale"
		if instance.Status == "online" && instance.LastSeenAt > 0 && now-instance.LastSeenAt < staleSeconds {
			status = "online"
			online++
		} else if knownByNode[instance.NodeName].PriorTwoNonOnline {
			status = "offline"
		}
		writes[index].Instance.CurrentStatus = status
		writes[index].Sample.Status = status
		appendMetric := func(values *[]float64, value *float64) {
			if value != nil {
				*values = append(*values, *value)
			}
		}
		appendMetric(&cpuValues, instance.CPUPercent)
		appendMetric(&memoryValues, instance.MemoryPercent)
		appendMetric(&diskValues, instance.StorageUsedPercent)
	}
	minute := floorMinute(now)
	health := constant.SiteHealthOK
	for _, write := range writes {
		switch write.Sample.Status {
		case "offline":
			health = constant.SiteHealthCritical
		case "stale":
			if health == constant.SiteHealthOK {
				health = constant.SiteHealthWarning
			}
		}
	}
	sample := model.SiteStatusMinutely{
		SiteID: siteID, MinuteTS: minute, InstanceCount: len(instances),
		OnlineInstanceCount: online, HealthStatus: health, CreatedAt: now,
	}
	sample.CPUMaxPercent, sample.CPUAvgPercent = metricMaxAverage(cpuValues)
	sample.MemoryMaxPercent, sample.MemoryAvgPercent = metricMaxAverage(memoryValues)
	sample.DiskMaxUsedPercent, _ = metricMaxAverage(diskValues)
	return writes, sample, health
}

func metricMaxAverage(values []float64) (*float64, *float64) {
	if len(values) == 0 {
		return nil, nil
	}
	maximum := values[0]
	total := 0.0
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
		total += value
	}
	average := total / float64(len(values))
	return &maximum, &average
}
