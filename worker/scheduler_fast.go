package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

const (
	fastTaskQueueCapacity             = 256
	fastTaskShutdownTimeout           = 5 * time.Second
	scheduledStateTransitionErrorCode = "SCHEDULED_STATE_TRANSITION"
)

type scheduledFastTask struct {
	ctx         context.Context
	site        model.Site
	taskType    string
	requestID   string
	concurrency int
}

// fastTaskDispatcher is intentionally process-local. Fast collection updates
// current state and minute samples, while durable window work remains owned by
// collection_run and collection_cursor.
type fastTaskDispatcher struct {
	execute func(scheduledFastTask)

	mu            sync.Mutex
	pending       []scheduledFastTask
	queued        map[string]struct{}
	running       map[string]struct{}
	runningSites  map[int64]struct{}
	activeByType  map[string]int
	limitsByType  map[string]int
	stopped       bool
	activeWorkers sync.WaitGroup
}

func newFastTaskDispatcher(execute func(scheduledFastTask)) *fastTaskDispatcher {
	return &fastTaskDispatcher{
		execute: execute, queued: make(map[string]struct{}), running: make(map[string]struct{}),
		runningSites: make(map[int64]struct{}), activeByType: make(map[string]int), limitsByType: make(map[string]int),
	}
}

func (dispatcher *fastTaskDispatcher) Enqueue(task scheduledFastTask) bool {
	if dispatcher == nil || task.ctx == nil || task.site.ID <= 0 || task.taskType == "" || task.requestID == "" {
		return false
	}
	if task.concurrency <= 0 {
		task.concurrency = 1
	}
	key := fastScheduleKey(task.taskType, task.site.ID)
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if dispatcher.stopped {
		return false
	}
	dispatcher.limitsByType[task.taskType] = task.concurrency
	if _, exists := dispatcher.queued[key]; exists {
		return false
	}
	if _, exists := dispatcher.running[key]; exists || len(dispatcher.pending) >= fastTaskQueueCapacity {
		return false
	}
	dispatcher.pending = append(dispatcher.pending, task)
	dispatcher.queued[key] = struct{}{}
	dispatcher.startReadyLocked()
	return true
}

func (dispatcher *fastTaskDispatcher) startReadyLocked() {
	for !dispatcher.stopped {
		index := -1
		for candidate, task := range dispatcher.pending {
			if _, siteBusy := dispatcher.runningSites[task.site.ID]; siteBusy {
				continue
			}
			if dispatcher.activeByType[task.taskType] >= dispatcher.limitsByType[task.taskType] {
				continue
			}
			index = candidate
			break
		}
		if index < 0 {
			return
		}
		task := dispatcher.pending[index]
		dispatcher.pending = append(dispatcher.pending[:index], dispatcher.pending[index+1:]...)
		key := fastScheduleKey(task.taskType, task.site.ID)
		delete(dispatcher.queued, key)
		dispatcher.running[key] = struct{}{}
		dispatcher.runningSites[task.site.ID] = struct{}{}
		dispatcher.activeByType[task.taskType]++
		dispatcher.activeWorkers.Add(1)
		go func(task scheduledFastTask) {
			defer dispatcher.activeWorkers.Done()
			if dispatcher.execute != nil {
				dispatcher.execute(task)
			}
			dispatcher.complete(task)
		}(task)
	}
}

func (dispatcher *fastTaskDispatcher) complete(task scheduledFastTask) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	key := fastScheduleKey(task.taskType, task.site.ID)
	delete(dispatcher.running, key)
	delete(dispatcher.runningSites, task.site.ID)
	if dispatcher.activeByType[task.taskType] > 0 {
		dispatcher.activeByType[task.taskType]--
	}
	dispatcher.startReadyLocked()
}

func (dispatcher *fastTaskDispatcher) Shutdown(ctx context.Context) {
	if dispatcher == nil {
		return
	}
	dispatcher.mu.Lock()
	dispatcher.stopped = true
	dispatcher.pending = nil
	dispatcher.queued = make(map[string]struct{})
	dispatcher.mu.Unlock()
	done := make(chan struct{})
	go func() {
		dispatcher.activeWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (scheduler *Scheduler) startFastTaskLifecycle(ctx context.Context) {
	if scheduler == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scheduler.fastLifecycleMu.Lock()
	defer scheduler.fastLifecycleMu.Unlock()

	scheduler.mu.Lock()
	previous := scheduler.fastTasks
	previousCancel := scheduler.fastTaskCancel
	scheduler.fastTasks = nil
	scheduler.fastTaskCtx = nil
	scheduler.fastTaskCancel = nil
	scheduler.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	shutdownFastTaskDispatcher(previous)

	fastTaskCtx, fastTaskCancel := context.WithCancel(ctx)
	scheduler.mu.Lock()
	scheduler.fastTasks = newFastTaskDispatcher(scheduler.executeFastTask)
	scheduler.fastTaskCtx = fastTaskCtx
	scheduler.fastTaskCancel = fastTaskCancel
	scheduler.fastStartup = true
	scheduler.mu.Unlock()
}

func (scheduler *Scheduler) shutdownFastTasks() {
	if scheduler == nil {
		return
	}
	scheduler.fastLifecycleMu.Lock()
	defer scheduler.fastLifecycleMu.Unlock()

	scheduler.mu.Lock()
	dispatcher := scheduler.fastTasks
	cancel := scheduler.fastTaskCancel
	scheduler.fastTasks = nil
	scheduler.fastTaskCtx = nil
	scheduler.fastTaskCancel = nil
	scheduler.fastStartup = false
	scheduler.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	shutdownFastTaskDispatcher(dispatcher)
}

func shutdownFastTaskDispatcher(dispatcher *fastTaskDispatcher) {
	if dispatcher == nil {
		return
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), fastTaskShutdownTimeout)
	defer cancel()
	dispatcher.Shutdown(shutdownContext)
}

func (scheduler *Scheduler) executeFastTask(task scheduledFastTask) {
	if scheduler == nil || scheduler.siteJobs == nil {
		return
	}
	started := scheduler.clock.Now()
	status := "success"
	var executionErr error
	defer func() {
		if executionErr != nil {
			status = "failed"
		}
		if scheduler.fastTaskHistory != nil {
			finished := scheduler.clock.Now()
			_ = scheduler.fastTaskHistory.Add(context.Background(), common.FastTaskHistoryRecord{
				SiteID: task.site.ID, TaskType: task.taskType, StartedAt: started.Unix(), FinishedAt: finished.Unix(),
				Status: status, DurationMS: finished.Sub(started).Milliseconds(), Error: errorString(executionErr), RequestID: task.requestID,
			})
		}
	}()
	before, err := scheduler.repository.FindSiteForScheduling(task.ctx, task.site.ID)
	if err != nil || before.ConfigVersion != task.site.ConfigVersion || !schedulerSiteEligible(before, task.taskType) {
		executionErr = err
		if executionErr == nil {
			executionErr = fmt.Errorf("site configuration changed or task ineligible")
		}
		return
	}
	_, _, taskErr := scheduler.siteJobs.ExecutePeriodicSiteTask(
		task.ctx, task.taskType, task.site.ID, task.site.ConfigVersion, task.requestID,
	)
	executionErr = taskErr
	after, err := scheduler.repository.FindSiteForScheduling(task.ctx, task.site.ID)
	if err != nil || !fastTaskFailureStateChanged(task.taskType, before, after) {
		return
	}
	errorCode := scheduledStateTransitionErrorCode
	if taskErr != nil {
		errorCode = model.CollectionTaskExecutionFailedCode
	}
	_, _, _ = scheduler.repository.RecordScheduledSiteTaskDiagnostic(task.ctx, model.ScheduledSiteTaskDiagnosticRequest{
		SiteID: task.site.ID, TaskType: task.taskType, RequestID: task.requestID + "_diag",
		ErrorCode: errorCode, Now: scheduler.clock.Now().Unix(),
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func fastTaskConcurrency(taskType string, settings model.CollectorSettings) int {
	switch taskType {
	case constant.TaskTypeSiteProbe:
		return settings.ProbeConcurrency
	case constant.TaskTypeRealtimeStat:
		return settings.RealtimeConcurrency
	case constant.TaskTypeResourceSnapshot:
		return settings.ResourceConcurrency
	default:
		return 1
	}
}

func fastTaskFailureStateChanged(taskType string, before, after model.Site) bool {
	if before.AuthStatus != after.AuthStatus && after.AuthStatus == constant.SiteAuthExpired {
		return true
	}
	if taskType != constant.TaskTypeSiteProbe {
		return false
	}
	if before.OnlineStatus != constant.SiteOnlineOffline && after.OnlineStatus == constant.SiteOnlineOffline {
		return true
	}
	if before.DataExportEnabled && !after.DataExportEnabled {
		return true
	}
	return before.StatisticsStatus != after.StatisticsStatus &&
		(after.StatisticsStatus == constant.SiteStatisticsPendingConfig || after.StatisticsStatus == constant.SiteStatisticsError)
}
