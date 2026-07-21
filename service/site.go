package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const (
	sitePreflightLifetime       = 10 * time.Minute
	defaultInstanceStaleSeconds = 90
)

type SiteService struct {
	sites                *model.SiteRepository
	clients              SiteClientFactory
	cipher               *common.Cipher
	clock                common.Clock
	preflightSecret      []byte
	postCommit           PostCommitNotifier
	maintenance          DataMaintenanceNotifier
	performanceCache     *sitePerformanceCache
	performanceRefreshes chan struct{}
}

type SiteServiceOptions struct {
	Repository      *model.SiteRepository
	ClientFactory   SiteClientFactory
	Cipher          *common.Cipher
	Clock           common.Clock
	PreflightSecret []byte
	PostCommit      PostCommitNotifier
	Maintenance     DataMaintenanceNotifier
}

type DataMaintenanceNotifier interface {
	NotifyAuthorizationPricingSync()
}

func NewSiteService(options SiteServiceOptions) (*SiteService, error) {
	if options.Repository == nil || options.ClientFactory == nil || options.Cipher == nil || options.Clock == nil {
		return nil, errors.New("site service dependencies are required")
	}
	if len(options.PreflightSecret) < 32 {
		return nil, errors.New("site preflight secret must contain at least 32 bytes")
	}
	return &SiteService{
		sites: options.Repository, clients: options.ClientFactory, cipher: options.Cipher,
		clock:           options.Clock,
		preflightSecret: append([]byte(nil), options.PreflightSecret...), postCommit: options.PostCommit,
		maintenance:          options.Maintenance,
		performanceCache:     newSitePerformanceCache(),
		performanceRefreshes: make(chan struct{}, 4),
	}, nil
}

func (service *SiteService) Create(ctx context.Context, request dto.SiteCreateRequest) (dto.SiteDetail, error) {
	baseURL, err := NormalizeUpstreamBaseURL(request.BaseURL)
	if err != nil {
		return dto.SiteDetail{}, ErrSiteInvalidBaseURL
	}
	client, err := service.clients.NewPublic(baseURL)
	if err != nil {
		return dto.SiteDetail{}, err
	}
	client.CloseIdleConnections()
	now := service.clock.Now().Unix()
	site := model.Site{
		Name: request.Name, BaseURL: baseURL, ConfigVersion: 1, Remark: request.Remark,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineUnknown,
		AuthStatus: constant.SiteAuthUnauthorized, StatisticsStatus: constant.SiteStatisticsPendingConfig,
		HealthStatus: constant.SiteHealthUnavailable, CreatedAt: now, UpdatedAt: now,
	}
	if err := service.sites.Create(ctx, &site); err != nil {
		if model.IsDuplicateKey(err) {
			return dto.SiteDetail{}, ErrSiteConflict
		}
		return dto.SiteDetail{}, fmt.Errorf("create site: %w", err)
	}
	return service.Get(ctx, site.ID)
}

func (service *SiteService) Get(ctx context.Context, id int64) (dto.SiteDetail, error) {
	site, err := service.sites.FindByID(ctx, id)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteDetail{}, ErrSiteNotFound
		}
		return dto.SiteDetail{}, fmt.Errorf("find site: %w", err)
	}
	if err := validatePersistedSite(site); err != nil {
		return dto.SiteDetail{}, err
	}
	return service.detailFromModel(ctx, site)
}

func (service *SiteService) PerformanceSummary(ctx context.Context, id int64, hours int, requestID string) (dto.SitePerformanceSummary, error) {
	site, err := service.sites.FindByID(ctx, id)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SitePerformanceSummary{}, ErrSiteNotFound
		}
		return dto.SitePerformanceSummary{}, fmt.Errorf("find site: %w", err)
	}
	if err := validatePersistedSite(site); err != nil {
		return dto.SitePerformanceSummary{}, err
	}
	_, client, err := service.periodicAuthenticatedClient(ctx, id, site.ConfigVersion)
	if err != nil {
		if errors.Is(err, model.ErrSiteRunConfigChanged) {
			return dto.SitePerformanceSummary{}, ErrSiteInvalidState
		}
		return dto.SitePerformanceSummary{}, err
	}
	defer client.CloseIdleConnections()
	upstream, err := client.PerformanceSummary(ctx, requestID, hours)
	if err != nil {
		return dto.SitePerformanceSummary{}, service.periodicTaskError(ctx, id, site.ConfigVersion, err)
	}
	result := sitePerformanceSummary(hours, service.clock.Now().Unix(), upstream)
	if hours == 24 {
		service.performanceCache.Store(id, site.ConfigVersion, result, service.clock.Now().Unix())
	}
	return result, nil
}

func (service *SiteService) List(ctx context.Context, query dto.SiteListQuery) (common.PageData[dto.SiteListItem], error) {
	sites, total, err := service.sites.List(ctx, model.SiteFilter{
		Keyword: query.Keyword, ManagementStatuses: query.ManagementStatuses,
		OnlineStatuses: query.OnlineStatuses, AuthStatuses: query.AuthStatuses,
		StatisticsStatuses: query.StatisticsStatuses, HealthStatuses: query.HealthStatuses,
		SortBy: query.SortBy, SortOrder: query.SortOrder,
		Offset: (query.Page - 1) * query.PageSize, Limit: query.PageSize,
	})
	if err != nil {
		return common.PageData[dto.SiteListItem]{}, fmt.Errorf("list sites: %w", err)
	}
	items := make([]dto.SiteListItem, 0, len(sites))
	siteIDs := make([]int64, 0, len(sites))
	for _, site := range sites {
		siteIDs = append(siteIDs, site.ID)
	}
	resources, err := service.sites.ListLatestResourceSummaries(ctx, siteIDs)
	if err != nil {
		return common.PageData[dto.SiteListItem]{}, fmt.Errorf("list latest site resources: %w", err)
	}
	nowTime := service.clock.Now()
	now := nowTime.Unix()
	usageStart, usageEnd := siteTodayUsageRange(nowTime)
	usage, err := service.sites.ListUsageOverviews(ctx, siteIDs, usageStart, usageEnd)
	if err != nil {
		return common.PageData[dto.SiteListItem]{}, fmt.Errorf("list site usage overviews: %w", err)
	}
	performance := service.listPerformanceSummaries(sites, now)
	for _, site := range sites {
		if err := validatePersistedSite(site); err != nil {
			return common.PageData[dto.SiteListItem]{}, err
		}
		items = append(items, siteListItemFromModel(site, now, resources[site.ID], usage[site.ID], performance[site.ID]))
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *SiteService) Update(ctx context.Context, id int64, request dto.SiteUpdateRequest) (dto.SiteDetail, error) {
	baseURL, err := NormalizeUpstreamBaseURL(request.BaseURL)
	if err != nil {
		return dto.SiteDetail{}, ErrSiteInvalidBaseURL
	}
	committedAt := int64(0)
	authChanged := false
	err = service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, lockErr := repository.FindByIDForUpdate(ctx, id)
		if lockErr != nil {
			if model.IsNotFound(lockErr) {
				return ErrSiteNotFound
			}
			return lockErr
		}
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), site.UpdatedAt)
		baseChanged := site.BaseURL != baseURL
		if baseChanged {
			if !request.ConfirmSameSite || request.BaseURLPreflightToken == "" {
				return ErrBaseURLPreflightRequired
			}
			preflight, verifyErr := service.verifyPreflightToken(request.BaseURLPreflightToken)
			if verifyErr != nil || preflight.SiteID != site.ID ||
				preflight.ConfigVersion != site.ConfigVersion || preflight.BaseURL != baseURL {
				return ErrBaseURLPreflightRequired
			}
			if err := repository.BumpSiteFence(ctx, &site, committedAt); err != nil {
				return err
			}
			changeType := compareSiteBaseURLs(site.BaseURL, baseURL)
			site.BaseURL = baseURL
			site.StatisticsStatus = constant.SiteStatisticsPendingConfig
			if site.ManagementStatus == constant.SiteManagementDisabled {
				site.StatisticsStatus = constant.SiteStatisticsPaused
			}
			if changeType == "origin" {
				site.AuthStatus = constant.SiteAuthUnauthorized
				site.AccessTokenEncrypted = nil
				authChanged = true
			}
			if err := repository.DeleteCapabilities(ctx, site.ID); err != nil {
				return err
			}
		}
		site.Name = request.Name
		site.Remark = request.Remark
		site.UpdatedAt = committedAt
		if err := repository.Save(ctx, &site); err != nil {
			if model.IsDuplicateKey(err) {
				return ErrSiteConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return dto.SiteDetail{}, err
	}
	service.notifySiteLifecycleAfterCommit(ctx, id, committedAt)
	if authChanged && service.postCommit != nil {
		service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
			Source: AlertSampleSourceAuth, SiteID: id, ObservedAt: committedAt,
		})
	}
	return service.Get(ctx, id)
}

func (service *SiteService) Delete(ctx context.Context, id int64) error {
	return service.sites.WithTransaction(ctx, func(repository *model.SiteRepository) error {
		site, err := repository.FindByIDForUpdate(ctx, id)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		dependencyTypes, err := repository.SiteDeleteBlockers(ctx, id)
		if err != nil {
			return err
		}
		if len(dependencyTypes) > 0 {
			return &SiteDeleteRestrictedError{DependencyTypes: dependencyTypes}
		}
		if err := repository.DeleteOwnedMetadata(ctx, id); err != nil {
			return err
		}
		return repository.Delete(ctx, &site)
	})
}

func (service *SiteService) ListInstances(ctx context.Context, siteID int64) ([]dto.SiteInstanceItem, error) {
	if _, err := service.sites.FindByID(ctx, siteID); err != nil {
		if model.IsNotFound(err) {
			return nil, ErrSiteNotFound
		}
		return nil, err
	}
	staleSeconds, err := service.sites.EffectiveInstanceStaleSeconds(ctx, siteID)
	if err != nil {
		return nil, err
	}
	snapshots, err := service.sites.ListInstanceSnapshots(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("list site instances: %w", err)
	}
	items := make([]dto.SiteInstanceItem, 0, len(snapshots))
	now := service.clock.Now().Unix()
	currentMinute := floorMinute(now)
	for _, snapshot := range snapshots {
		currentSample := snapshot.SampledAt != nil && *snapshot.SampledAt == currentMinute
		item := dto.SiteInstanceItem{
			SiteID: strconv.FormatInt(siteID, 10), NodeName: snapshot.NodeName,
			Hostname: snapshot.Hostname, IsMaster: snapshot.IsMaster, RuntimeVersion: snapshot.RuntimeVersion,
			GOOS: snapshot.GOOS, GOARCH: snapshot.GOARCH, UpstreamStatus: snapshot.UpstreamStatus,
			UpstreamStaleAfterSeconds:  snapshot.UpstreamStaleAfterSeconds,
			CurrentStatus:              effectiveInstanceStatus(snapshot, now, int64(staleSeconds), currentSample),
			EffectiveStaleAfterSeconds: staleSeconds,
			SampledAt:                  snapshot.SampledAt, DataStatus: "missing", FirstSeenAt: snapshot.FirstSeenAt,
			StartedAt: snapshot.StartedAt, LastSeenAt: snapshot.LastSeenAt, LastSyncedAt: snapshot.LastSyncedAt,
		}
		if currentSample {
			item.DataStatus = "complete"
			item.CPUPercent = snapshot.CPUPercent
			item.MemoryPercent = snapshot.MemoryPercent
			item.DiskUsedPercent = snapshot.DiskUsedPercent
			if snapshot.DiskTotalBytes != nil {
				value := strconv.FormatInt(*snapshot.DiskTotalBytes, 10)
				item.DiskTotalBytes = &value
			}
			if snapshot.DiskUsedBytes != nil {
				value := strconv.FormatInt(*snapshot.DiskUsedBytes, 10)
				item.DiskUsedBytes = &value
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func effectiveInstanceStatus(snapshot model.SiteInstanceSnapshot, now, staleSeconds int64, currentSample bool) string {
	if currentSample && snapshot.SampleStatus != nil {
		switch *snapshot.SampleStatus {
		case "offline":
			return "offline"
		case "stale":
			return "stale"
		case "online":
			if snapshot.LastSeenAt == nil || *snapshot.LastSeenAt <= 0 {
				return "unknown"
			}
			if now-*snapshot.LastSeenAt >= staleSeconds {
				return "stale"
			}
			return "online"
		default:
			return "unknown"
		}
	}
	if snapshot.CurrentStatus == "offline" {
		return "offline"
	}
	if snapshot.LastSeenAt != nil && *snapshot.LastSeenAt > 0 && now-*snapshot.LastSeenAt >= staleSeconds {
		return "stale"
	}
	return "unknown"
}

func (service *SiteService) detailFromModel(ctx context.Context, site model.Site) (dto.SiteDetail, error) {
	nowTime := service.clock.Now()
	now := nowTime.Unix()
	resources, err := service.sites.ListLatestResourceSummaries(ctx, []int64{site.ID})
	if err != nil {
		return dto.SiteDetail{}, fmt.Errorf("read latest site resource: %w", err)
	}
	usageStart, usageEnd := siteTodayUsageRange(nowTime)
	usage, err := service.sites.ListUsageOverviews(ctx, []int64{site.ID}, usageStart, usageEnd)
	if err != nil {
		return dto.SiteDetail{}, fmt.Errorf("read site usage overview: %w", err)
	}
	detail := dto.SiteDetail{
		SiteListItem: siteListItemFromModel(site, now, resources[site.ID], usage[site.ID], service.listPerformanceSummaries([]model.Site{site}, now)[site.ID]), Remark: site.Remark, ConfigVersion: site.ConfigVersion,
		RootCreatedAt: site.RootCreatedAt, StatisticsStartAt: site.StatisticsStartAt,
		StatisticsStartSource: site.StatisticsStartSource, StatisticsEndAt: site.StatisticsEndAt,
		MonitoringStartAt: site.MonitoringStartAt, LastProbeAt: site.LastProbeAt,
		LastProbeSuccessAt: site.LastProbeSuccessAt,
		Backfill:           emptyBackfillSummary(), Completeness: emptyCompleteness(site),
	}
	if site.RootUserID != nil {
		value := strconv.FormatInt(*site.RootUserID, 10)
		detail.RootUserID = &value
	}
	run, err := service.sites.LatestBackfillRun(ctx, site.ID)
	if err == nil {
		detail.Backfill = backfillSummaryFromRun(run)
	} else if !model.IsNotFound(err) {
		return dto.SiteDetail{}, fmt.Errorf("read site backfill: %w", err)
	}
	return detail, nil
}

func siteTodayUsageRange(now time.Time) (int64, int64) {
	start, _ := dashboardTodayRange(now)
	return start, now.Unix()
}

func siteListItemFromModel(site model.Site, now int64, resource model.SiteStatusMinutely, usage model.SiteUsageOverview, performance dto.SitePerformanceSummary) dto.SiteListItem {
	zeroCount := 0
	zeroPercent := 0.0
	zeroMetric := "0"
	item := dto.SiteListItem{
		ID: strconv.FormatInt(site.ID, 10), Name: site.Name, BaseURL: site.BaseURL,
		ManagementStatus: site.ManagementStatus, OnlineStatus: site.OnlineStatus, AuthStatus: site.AuthStatus,
		StatisticsStatus: site.StatisticsStatus, HealthStatus: site.HealthStatus,
		Rate:     dto.RateInfo{QuotaPerUnit: site.QuotaPerUnit, USDExchangeRate: site.USDExchangeRate, Source: "unavailable", UpdatedAt: site.LastRateAt},
		Realtime: dto.SiteRealtimeInfo{UpdatedAt: site.LastRealtimeStatAt, Expired: true},
		Resource: dto.SiteResourceSummary{
			InstanceCount: &zeroCount, OnlineInstanceCount: &zeroCount,
			CPUMaxPercent: &zeroPercent, MemoryMaxPercent: &zeroPercent,
			DiskMaxUsedPercent: &zeroPercent, DataStatus: "missing",
		},
		Today: dto.UsageSummary{
			RequestCount: &zeroMetric, Quota: &zeroMetric, TokenUsed: &zeroMetric,
			ActiveUsers: &zeroMetric, AvgRPM: &zeroMetric, AvgTPM: &zeroMetric, DataStatus: "missing",
		}, DisabledAt: site.DisabledAt, UpdatedAt: site.UpdatedAt,
	}
	if site.Version != "" {
		item.Version = stringPointer(site.Version)
		item.SystemName = stringPointer(site.SystemName)
		value := site.DataExportEnabled
		item.DataExportEnabled = &value
	}
	if site.QuotaPerUnit != nil && site.USDExchangeRate != nil {
		item.Rate.Source = "site"
	}
	rpm := strconv.FormatInt(site.CurrentRPM, 10)
	tpm := strconv.FormatInt(site.CurrentTPM, 10)
	item.Realtime.RPM = &rpm
	item.Realtime.TPM = &tpm
	if site.LastRealtimeStatAt != nil {
		item.Realtime.Expired = now-*site.LastRealtimeStatAt > 120
	}
	if resource.ID != 0 {
		item.Resource.InstanceCount = &resource.InstanceCount
		item.Resource.OnlineInstanceCount = &resource.OnlineInstanceCount
		item.Resource.UpdatedAt = &resource.MinuteTS
		item.Resource.DataStatus = "complete"
		if resource.CPUMaxPercent != nil {
			item.Resource.CPUMaxPercent = resource.CPUMaxPercent
		}
		if resource.MemoryMaxPercent != nil {
			item.Resource.MemoryMaxPercent = resource.MemoryMaxPercent
		}
		if resource.DiskMaxUsedPercent != nil {
			item.Resource.DiskMaxUsedPercent = resource.DiskMaxUsedPercent
		}
	}
	if usage.SiteID != 0 {
		activeUsers := strconv.FormatInt(usage.ActiveUsers, 10)
		item.Today = dto.UsageSummary{
			RequestCount: &usage.RequestCount, Quota: &usage.Quota, TokenUsed: &usage.TokenUsed,
			ActiveUsers: &activeUsers, AvgRPM: &usage.AvgRPM, AvgTPM: &usage.AvgTPM,
			AsOf: usage.AsOf, DataStatus: "complete",
		}
	}
	return item
}

func emptyBackfillSummary() dto.BackfillSummary {
	return dto.BackfillSummary{Status: "none"}
}

func backfillSummaryFromRun(run model.CollectionRun) dto.BackfillSummary {
	progress := 0.0
	terminal := run.CompletedWindows + run.FailedWindows + run.UnavailableWindows
	if run.TotalWindows > 0 {
		progress = float64(terminal) / float64(run.TotalWindows)
	} else if run.Status == "success" {
		progress = 1
	}
	status := run.Status
	if status == "success" {
		status = "none"
	}
	result := dto.BackfillSummary{
		Status: status, Progress: progress, TotalWindows: run.TotalWindows,
		CompletedWindows: run.CompletedWindows, FailedWindows: run.FailedWindows,
		StartTimestamp: run.StartTimestamp, EndTimestamp: run.EndTimestamp,
	}
	runID := strconv.FormatInt(run.ID, 10)
	result.RunID = &runID
	return result
}

func emptyCompleteness(site model.Site) dto.Completeness {
	status := "missing"
	if site.ManagementStatus == constant.SiteManagementDisabled {
		status = "paused"
	}
	return dto.Completeness{
		DataStatus: status, UnitType: "site_hour", MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{},
	}
}

func validatePersistedSite(site model.Site) error {
	if site.ID <= 0 || site.ConfigVersion <= 0 || !dto.ValidSiteManagementStatus(site.ManagementStatus) ||
		!dto.ValidSiteOnlineStatus(site.OnlineStatus) || !dto.ValidSiteAuthStatus(site.AuthStatus) ||
		!dto.ValidSiteStatisticsStatus(site.StatisticsStatus) || !dto.ValidSiteHealthStatus(site.HealthStatus) {
		return fmt.Errorf("invalid persisted site state: %w", ErrSiteIncompatible)
	}
	return nil
}

func stringPointer(value string) *string {
	result := value
	return &result
}
