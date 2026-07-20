package worker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type SchedulerOptions struct {
	Repository      *model.CollectionTaskRepository
	Settings        *model.CollectorSettingRepository
	Clock           common.Clock
	Tick            time.Duration
	SiteJobs        SitePeriodicJobRunner
	Metrics         SchedulerMetricsRecorder
	FastTaskHistory *common.RedisStore
}

type Scheduler struct {
	repository      *model.CollectionTaskRepository
	settings        *model.CollectorSettingRepository
	clock           common.Clock
	tick            time.Duration
	siteJobs        SitePeriodicJobRunner
	metrics         SchedulerMetricsRecorder
	fastTaskHistory *common.RedisStore
	fastTasks       *fastTaskDispatcher
	fastTaskCtx     context.Context
	fastTaskCancel  context.CancelFunc

	mu                          sync.Mutex
	fastLifecycleMu             sync.Mutex
	initialized                 bool
	fastStartup                 bool
	lastFastState               map[string]fastScheduleState
	lastMetadataHour            int64
	lastUsageHourBySite         map[int64]int64
	lastDailyDateBySite         map[int64]int64
	lastWeeklyYearAndWeekBySite map[int64]int64
}

type fastScheduleState struct {
	intervalSeconds int64
	slotStart       int64
}

func NewScheduler(options SchedulerOptions) (*Scheduler, error) {
	if options.Repository == nil || options.Settings == nil || options.Clock == nil {
		return nil, fmt.Errorf("scheduler dependencies are required")
	}
	if options.Tick <= 0 {
		options.Tick = time.Minute
	}
	scheduler := &Scheduler{
		repository: options.Repository, settings: options.Settings, clock: options.Clock,
		tick: options.Tick, siteJobs: options.SiteJobs, metrics: options.Metrics,
		fastTaskHistory:             options.FastTaskHistory,
		lastFastState:               make(map[string]fastScheduleState),
		lastUsageHourBySite:         make(map[int64]int64),
		lastDailyDateBySite:         make(map[int64]int64),
		lastWeeklyYearAndWeekBySite: make(map[int64]int64),
	}
	scheduler.fastTaskCtx, scheduler.fastTaskCancel = context.WithCancel(context.Background())
	scheduler.fastTasks = newFastTaskDispatcher(scheduler.executeFastTask)
	return scheduler, nil
}

func (scheduler *Scheduler) Startup(ctx context.Context) error {
	scheduler.startFastTaskLifecycle(ctx)
	if err := scheduler.runOnce(ctx, scheduler.clock.Now(), true); err != nil {
		scheduler.shutdownFastTasks()
		return err
	}
	return nil
}

func (scheduler *Scheduler) RunOnce(ctx context.Context) error {
	return scheduler.runOnce(ctx, scheduler.clock.Now(), false)
}

func (scheduler *Scheduler) runOnce(ctx context.Context, now time.Time, startup bool) error {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	settings, err := scheduler.settings.Load(ctx)
	if err != nil {
		return err
	}
	sites, err := scheduler.repository.ListSitesForScheduling(ctx)
	if err != nil {
		return err
	}
	fastTasks := []struct {
		taskType string
		interval int
	}{
		{constant.TaskTypeSiteProbe, settings.ProbeIntervalSeconds},
		{constant.TaskTypeRealtimeStat, settings.RealtimeIntervalSeconds},
		{constant.TaskTypeResourceSnapshot, settings.ResourceIntervalSeconds},
	}
	forceFastStartup := startup && scheduler.fastStartup
	for _, task := range fastTasks {
		interval := int64(task.interval)
		slotStart := now.Unix() - now.Unix()%interval
		for _, site := range sites {
			if !schedulerSiteEligible(site, task.taskType) {
				continue
			}
			key := fastScheduleKey(task.taskType, site.ID)
			state, scheduled := scheduler.lastFastState[key]
			if scheduled && state.intervalSeconds == interval && state.slotStart == slotStart && !forceFastStartup {
				continue
			}
			jitter := stableSiteJitterSeconds(site.ID)
			if jitter >= interval {
				jitter = interval - 1
			}
			if !forceFastStartup && now.Unix() < slotStart+jitter {
				continue
			}
			if scheduler.siteJobs == nil {
				continue
			}
			if scheduler.fastTasks != nil && scheduler.fastTasks.Enqueue(scheduledFastTask{
				ctx: scheduler.fastTaskCtx, site: site, taskType: task.taskType,
				requestID:   scheduledRequestID(task.taskType, site.ID, slotStart),
				concurrency: fastTaskConcurrency(task.taskType, settings),
			}) {
				scheduler.lastFastState[key] = fastScheduleState{intervalSeconds: interval, slotStart: slotStart}
			}
		}
	}
	durableResourceInterval := int64(settings.ResourceIntervalSeconds)
	durableResourceSlotStart := now.Unix() - now.Unix()%durableResourceInterval
	for _, taskType := range []string{
		constant.TaskTypePerformanceSync,
		constant.TaskTypeTopupSync,
		constant.TaskTypeRedemptionSync,
		constant.TaskTypeUpstreamTaskSync,
		constant.TaskTypeModelMetaSync,
		constant.TaskTypePlanSync,
		constant.TaskTypePricingSync,
		constant.TaskTypeSystemTaskSync,
	} {
		for _, site := range sites {
			if !schedulerSiteEligible(site, taskType) {
				continue
			}
			jitter := stableSiteJitterSeconds(site.ID)
			if jitter >= durableResourceInterval {
				jitter = durableResourceInterval - 1
			}
			if !forceFastStartup && now.Unix() < durableResourceSlotStart+jitter {
				continue
			}
			if err := scheduler.enqueueNonWindowForSite(
				ctx, site, taskType, durableResourceSlotStart, now.Unix(),
			); err != nil {
				return err
			}
		}
	}
	hour := now.Unix() / 3600
	if startup && !scheduler.initialized {
		scheduler.lastMetadataHour = hour
	} else if scheduler.lastMetadataHour != hour {
		for _, taskType := range []string{constant.TaskTypeUserSync, constant.TaskTypeChannelSync, constant.TaskTypeLogSync} {
			if err := scheduler.enqueueNonWindowForSites(ctx, sites, taskType, hour, now.Unix()); err != nil {
				return err
			}
		}
		scheduler.lastMetadataHour = hour
	}
	hourStart := now.Unix() - now.Unix()%3600
	if now.Unix() >= hourStart+int64(settings.UsageDelayMinutes)*60 && hourStart >= 3600 {
		if err := scheduler.enqueueWindowForEligibleSites(
			ctx, sites, constant.TaskTypeUsageHour, constant.CollectionPriorityUsageRealtime,
			hourStart-3600, hourStart, hourStart, now.Unix(), scheduler.lastUsageHourBySite,
		); err != nil {
			return err
		}
	}
	localNow := now.In(beijingLocation)
	dateKey := localNow.Year()*10000 + int(localNow.Month())*100 + localNow.Day()
	if localNow.Hour() >= 2 {
		today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, beijingLocation)
		if err := scheduler.enqueueWindowForEligibleSites(
			ctx, sites, constant.TaskTypeUsageValidation, constant.CollectionPriorityDailyValidation,
			today.Add(-24*time.Hour).Unix(), today.Unix(), int64(dateKey), now.Unix(), scheduler.lastDailyDateBySite,
		); err != nil {
			return err
		}
	}
	year, week := localNow.ISOWeek()
	weekKey := year*100 + week
	if localNow.Weekday() == time.Monday && localNow.Hour() >= 2 {
		today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, beijingLocation)
		if err := scheduler.enqueueWindowForEligibleSites(
			ctx, sites, constant.TaskTypeUsageValidation, constant.CollectionPriorityWeeklyValidation,
			today.Add(-7*24*time.Hour).Unix(), today.Unix(), int64(weekKey), now.Unix(), scheduler.lastWeeklyYearAndWeekBySite,
		); err != nil {
			return err
		}
	}
	scheduler.initialized = true
	scheduler.fastStartup = false
	recordWorkerMetric(func() { scheduler.metrics.SetSchedulerHeartbeat(now) })
	return nil
}

func (scheduler *Scheduler) enqueueNonWindowForSites(
	ctx context.Context,
	sites []model.Site,
	taskType string,
	slot int64,
	now int64,
) error {
	for _, site := range sites {
		if !schedulerSiteEligible(site, taskType) {
			continue
		}
		if err := scheduler.enqueueNonWindowForSite(ctx, site, taskType, slot, now); err != nil {
			return err
		}
	}
	return nil
}

func (scheduler *Scheduler) enqueueNonWindowForSite(
	ctx context.Context,
	site model.Site,
	taskType string,
	slot int64,
	now int64,
) error {
	_, _, err := scheduler.repository.EnqueueSiteTask(ctx, model.SiteTaskEnqueueRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
		TaskType: taskType, TriggerType: constant.CollectionTriggerSchedule,
		Priority: 0, RequestID: scheduledRequestID(taskType, site.ID, slot), Now: now,
	})
	if err != nil && !scheduleDomainSkip(err) {
		return err
	}
	return nil
}

func (scheduler *Scheduler) enqueueWindowForSites(
	ctx context.Context,
	sites []model.Site,
	taskType string,
	priority int,
	start, end, slot, now int64,
) error {
	for _, site := range sites {
		if !schedulerSiteEligible(site, taskType) {
			continue
		}
		startCopy, endCopy := start, end
		_, err := scheduler.repository.EnqueueScheduledSiteWindowTask(ctx, model.SiteTaskEnqueueRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
			TaskType: taskType, TriggerType: constant.CollectionTriggerSchedule,
			StartTimestamp: &startCopy, EndTimestamp: &endCopy, Scope: []byte("{}"),
			Priority: priority, RequestID: scheduledRequestID(taskType, site.ID, slot), Now: now,
			Mode: model.SiteWindowRunSchedule,
		})
		if err != nil && !scheduleDomainSkip(err) {
			return err
		}
	}
	return nil
}

// enqueueWindowForEligibleSites tracks completion of a scheduling decision per
// site instead of globally. An offline site therefore remains eligible to
// receive the same closed window as soon as its probe recovers it.
func (scheduler *Scheduler) enqueueWindowForEligibleSites(
	ctx context.Context,
	sites []model.Site,
	taskType string,
	priority int,
	start, end, slot, now int64,
	lastScheduled map[int64]int64,
) error {
	for _, site := range sites {
		if !schedulerSiteEligible(site, taskType) || lastScheduled[site.ID] == slot {
			continue
		}
		if err := scheduler.enqueueWindowForSites(ctx, []model.Site{site}, taskType, priority, start, end, slot, now); err != nil {
			return err
		}
		lastScheduled[site.ID] = slot
	}
	return nil
}

func schedulerSiteEligible(site model.Site, taskType string) bool {
	if taskType == constant.TaskTypeSiteProbe {
		return site.ID > 0 && site.ConfigVersion > 0 &&
			site.ManagementStatus == constant.SiteManagementActive && site.StatisticsEndAt == nil
	}
	if site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized ||
		site.StatisticsEndAt != nil || site.OnlineStatus == constant.SiteOnlineOffline {
		return false
	}
	if constant.CollectionTaskWindowed(taskType) && !site.DataExportEnabled {
		return false
	}
	return true
}

func scheduledRequestID(taskType string, siteID, slot int64) string {
	return "sch_" + taskType + "_" + strconv.FormatInt(siteID, 10) + "_" + strconv.FormatInt(slot, 10)
}

func fastScheduleKey(taskType string, siteID int64) string {
	return taskType + ":" + strconv.FormatInt(siteID, 10)
}

func stableSiteJitterSeconds(siteID int64) int64 {
	if siteID <= 0 {
		return 0
	}
	value := uint64(siteID)
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	return int64(value % 11)
}

func scheduleDomainSkip(err error) bool {
	return errors.Is(err, model.ErrSiteRunConfigChanged) ||
		errors.Is(err, model.ErrSiteRunManagementInactive) ||
		errors.Is(err, model.ErrSiteRunAuthorizationNeeded) ||
		errors.Is(err, model.ErrSiteRunStatisticsEnded) ||
		errors.Is(err, model.ErrSiteRunExportDisabled) ||
		errors.Is(err, model.ErrSiteRunCapabilitiesPending) ||
		errors.Is(err, model.ErrSiteWindowRunOverlap)
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	defer scheduler.shutdownFastTasks()
	ticker := scheduler.clock.NewTicker(scheduler.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if err := scheduler.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}
