package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestRuntimeStartStopReadyAndImmediateSiteJobs(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "runtime", now.Unix())
	runner := &recordingSiteJobRunner{calls: make(chan string, 16)}
	runtime, err := NewRuntime(RuntimeOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock, SiteJobs: runner, PollInterval: 100 * time.Millisecond, SchedulerTick: time.Second,
	})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	if !runtime.Ready() {
		t.Fatal("runtime did not report ready after durable initialization")
	}
	waitForScheduledSiteJobFamilies(t, clock, runner)
	stopContext, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopContext); err != nil {
		t.Fatalf("stop runtime: %v", err)
	}
	if runtime.Ready() {
		t.Fatal("stopped runtime still reports ready")
	}
	restartedParent, restartCancel := context.WithCancel(context.Background())
	defer restartCancel()
	if err := runtime.Start(restartedParent); err != nil {
		t.Fatalf("restart runtime: %v", err)
	}
	if !runtime.Ready() {
		t.Fatal("restarted runtime did not report ready")
	}
	waitForScheduledSiteJobFamilies(t, clock, runner)
	restartStopContext, restartStopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer restartStopCancel()
	if err := runtime.Stop(restartStopContext); err != nil {
		t.Fatalf("stop restarted runtime: %v", err)
	}
	var scheduledRuns int64
	if err := database.GORM.Model(&model.CollectionRun{}).
		Where("site_id = ? AND task_type IN ?", site.ID,
			[]string{constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot}).
		Count(&scheduledRuns).Error; err != nil || scheduledRuns != 0 {
		t.Fatalf("scheduled fast collection runs = %d, %v", scheduledRuns, err)
	}
}

func TestRuntimeRejectsMissingRequiredTaskHandler(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	options := RuntimeOptions{
		Repository:        model.NewCollectionTaskRepository(nil),
		Settings:          model.NewCollectorSettingRepository(nil),
		Clock:             clock,
		RequiredTaskTypes: []string{constant.TaskTypeAccountRebuild},
	}
	runtime, err := NewRuntime(options)
	if err != nil {
		t.Fatalf("construct runtime with deferred required-handler validation: %v", err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("runtime started with a missing required account rebuild handler")
	}
	options.Handlers = map[string]JobHandler{
		constant.TaskTypeAccountRebuild: JobHandlerFunc(func(context.Context, JobExecution) (JobOutcome, error) {
			return JobOutcome{}, nil
		}),
	}
	runtime, err = NewRuntime(options)
	if err != nil {
		t.Fatalf("runtime rejected registered required handler: %v", err)
	}
	if !runtime.executor.hasHandler(constant.TaskTypeAccountRebuild) {
		t.Fatal("runtime lost a registered required handler")
	}
}

func TestRuntimeStartupMaterializesBeforeRecoveringRunningTasks(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	repository := model.NewCollectionTaskRepository(database.GORM)

	pendingSite := createWorkerTestSite(t, database, "startup-materialize", now.Unix())
	pending := createWorkerWindowRun(t, database, repository, pendingSite,
		constant.TaskTypeUsageBackfill, constant.CollectionTriggerRecovery,
		constant.CollectionPrioritySiteRecovery, []byte(`{"only_missing":false}`),
		now.Unix()-3600, now.Unix(), "req_startup_materialize", now.Unix())
	if err := database.GORM.Where("run_id = ?", pending.ID).Delete(&model.CollectionRunWindow{}).Error; err != nil {
		t.Fatalf("reset pending materialization windows: %v", err)
	}
	if err := database.GORM.Model(&model.CollectionRun{}).Where("id = ?", pending.ID).Updates(map[string]any{
		"windows_initialized_at": nil, "total_windows": 0, "updated_at": now.Unix(),
	}).Error; err != nil {
		t.Fatalf("reset pending materialization parent: %v", err)
	}

	runningSite := createWorkerTestSite(t, database, "startup-takeover", now.Unix())
	running := createWorkerWindowRun(t, database, repository, runningSite,
		constant.TaskTypeUsageBackfill, constant.CollectionTriggerRecovery,
		constant.CollectionPrioritySiteRecovery, []byte(`{"only_missing":false}`),
		now.Unix()-3600, now.Unix(), "req_startup_takeover", now.Unix()+1)
	if err := database.GORM.Model(&model.CollectionRun{}).Where("id = ?", running.ID).Updates(map[string]any{
		"status": model.CollectionTaskStatusRunning, "heartbeat_at": now.Unix(),
		"last_request_id": "wrk_crashed_runtime", "started_at": now.Unix(),
	}).Error; err != nil {
		t.Fatalf("mark takeover parent running: %v", err)
	}
	if err := database.GORM.Model(&model.CollectionRunWindow{}).Where("run_id = ?", running.ID).Updates(map[string]any{
		"status": model.CollectionTaskStatusRunning, "attempt_count": 1,
		"started_at": now.Unix(), "updated_at": now.Unix(),
	}).Error; err != nil {
		t.Fatalf("mark takeover window running: %v", err)
	}

	runtime, err := NewRuntime(RuntimeOptions{
		Repository: repository, Settings: model.NewCollectorSettingRepository(database.GORM), Clock: clock,
	})
	if err != nil {
		t.Fatalf("create mixed startup runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("start mixed recovery runtime: %v", err)
	}
	stopContext, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	defer func() { _ = runtime.Stop(stopContext) }()

	var materialized model.CollectionRun
	if err := database.GORM.First(&materialized, pending.ID).Error; err != nil ||
		materialized.WindowsInitializedAt == nil || materialized.TotalWindows != 1 {
		t.Fatalf("startup materialized run = %#v, %v", materialized, err)
	}
	var recovered model.CollectionRun
	if err := database.GORM.First(&recovered, running.ID).Error; err != nil ||
		recovered.Status != model.CollectionTaskStatusPending || recovered.HeartbeatAt != nil {
		t.Fatalf("startup recovered run = %#v, %v", recovered, err)
	}
	var recoveredWindow model.CollectionRunWindow
	if err := database.GORM.Where("run_id = ?", running.ID).Take(&recoveredWindow).Error; err != nil ||
		recoveredWindow.Status != model.CollectionTaskStatusPending || recoveredWindow.AttemptCount != 1 {
		t.Fatalf("startup recovered window = %#v, %v", recoveredWindow, err)
	}
}

func TestRuntimeQuiesceDrainsAndDeadlineReleasesRunningClaim(t *testing.T) {
	t.Run("graceful_drain", func(t *testing.T) {
		database := openWorkerTestDatabase(t)
		now := time.Unix(1_752_400_800, 0)
		clock := testsupport.NewFakeClock(now)
		run := createBlockingProbeRun(t, database, "graceful-drain", now.Unix())
		started := make(chan context.Context, 1)
		release := make(chan struct{})
		handler := JobHandlerFunc(func(ctx context.Context, _ JobExecution) (JobOutcome, error) {
			started <- ctx
			select {
			case <-release:
				return JobOutcome{}, nil
			case <-ctx.Done():
				return JobOutcome{}, ctx.Err()
			}
		})
		runtime, err := NewRuntime(RuntimeOptions{
			Repository: model.NewCollectionTaskRepository(database.GORM),
			Settings:   model.NewCollectorSettingRepository(database.GORM), Clock: clock,
			Handlers: map[string]JobHandler{constant.TaskTypeSiteProbe: handler},
		})
		if err != nil {
			t.Fatalf("create drain runtime: %v", err)
		}
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := runtime.Start(parent); err != nil {
			t.Fatalf("start drain runtime: %v", err)
		}
		var executionContext context.Context
		select {
		case executionContext = <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("probe handler was not claimed")
		}
		if err := runtime.Quiesce(); err != nil {
			t.Fatalf("quiesce runtime: %v", err)
		}
		select {
		case <-executionContext.Done():
			t.Fatal("quiesce canceled an in-flight execution before the drain deadline")
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		stopContext, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := runtime.Stop(stopContext); err != nil {
			t.Fatalf("stop drained runtime: %v", err)
		}
		var completed model.CollectionRun
		if err := database.GORM.First(&completed, run.ID).Error; err != nil ||
			completed.Status != model.CollectionTaskStatusSuccess {
			t.Fatalf("drained probe run = %#v, %v", completed, err)
		}
	})

	t.Run("deadline_releases_pending", func(t *testing.T) {
		database := openWorkerTestDatabase(t)
		now := time.Unix(1_752_400_800, 0)
		clock := testsupport.NewFakeClock(now)
		repository := model.NewCollectionTaskRepository(database.GORM)
		site := createWorkerTestSite(t, database, "deadline-running", now.Unix())
		run := createWorkerWindowRun(t, database, repository, site,
			constant.TaskTypeUsageBackfill, constant.CollectionTriggerRecovery,
			constant.CollectionPrioritySiteRecovery, []byte(`{"only_missing":false}`),
			now.Unix()-3600, now.Unix(), "req_deadline_running", now.Unix())
		started := make(chan struct{}, 1)
		executionCanceled := make(chan struct{})
		releaseHandler := make(chan struct{})
		handler := JobHandlerFunc(func(ctx context.Context, _ JobExecution) (JobOutcome, error) {
			started <- struct{}{}
			<-ctx.Done()
			close(executionCanceled)
			<-releaseHandler
			return JobOutcome{}, ctx.Err()
		})
		runtime, err := NewRuntime(RuntimeOptions{
			Repository: repository,
			Settings:   model.NewCollectorSettingRepository(database.GORM), Clock: clock,
			Handlers: map[string]JobHandler{constant.TaskTypeUsageBackfill: handler},
		})
		if err != nil {
			t.Fatalf("create deadline runtime: %v", err)
		}
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := runtime.Start(parent); err != nil {
			t.Fatalf("start deadline runtime: %v", err)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("deadline probe handler was not claimed")
		}
		stopContext, stopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer stopCancel()
		stopDone := make(chan error, 1)
		go func() { stopDone <- runtime.Stop(stopContext) }()
		select {
		case <-executionCanceled:
		case <-time.After(5 * time.Second):
			t.Fatal("deadline did not cancel the execution context")
		}
		select {
		case err := <-stopDone:
			if !errors.Is(err, ErrRuntimeStopTimeout) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("deadline stop error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runtime Stop ignored its hard deadline")
		}
		runtime.mu.Lock()
		done := runtime.done
		runtime.mu.Unlock()
		close(releaseHandler)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("deadline-canceled runtime did not stop")
		}
		var preserved model.CollectionRun
		if err := database.GORM.First(&preserved, run.ID).Error; err != nil ||
			preserved.Status != model.CollectionTaskStatusPending || preserved.HeartbeatAt != nil || preserved.RetryCount != 0 {
			t.Fatalf("deadline-released run = %#v, %v", preserved, err)
		}
		var preservedWindow model.CollectionRunWindow
		if err := database.GORM.Where("run_id = ?", run.ID).Take(&preservedWindow).Error; err != nil ||
			preservedWindow.Status != model.CollectionTaskStatusPending || preservedWindow.NextRetryAt == nil || preservedWindow.AttemptCount != 1 {
			t.Fatalf("deadline-released window = %#v, %v", preservedWindow, err)
		}
	})
}

func createBlockingProbeRun(t *testing.T, database *model.Database, suffix string, now int64) model.CollectionRun {
	t.Helper()
	site := createWorkerTestSite(t, database, suffix, now)
	run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		RequestID: "req_" + suffix, Now: now,
	})
	if err != nil {
		t.Fatalf("build blocking probe run: %v", err)
	}
	created, _, err := model.NewSiteRepository(database.GORM).CreateOrGetRun(context.Background(), &run)
	if err != nil {
		t.Fatalf("create blocking probe run: %v", err)
	}
	return created
}

func TestSchedulerControllableClockCadenceAndNoDuplicates(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "scheduler", now.Unix())
	runner := &recordingSiteJobRunner{calls: make(chan string, 32)}
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM), Clock: clock, SiteJobs: runner,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("scheduler startup: %v", err)
	}
	waitForSiteJobCalls(t, runner, 3)
	assertWorkerRunCount(t, database, site.ID, 8)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("repeat startup slot: %v", err)
	}
	assertWorkerRunCount(t, database, site.ID, 8)
	restartedScheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM), Clock: clock, SiteJobs: runner,
	})
	if err != nil {
		t.Fatalf("recreate scheduler: %v", err)
	}
	if err := restartedScheduler.Startup(context.Background()); err != nil {
		t.Fatalf("restarted scheduler startup: %v", err)
	}
	waitForSiteJobCalls(t, runner, 3)
	assertWorkerRunCount(t, database, site.ID, 8)

	jitter := time.Duration(stableSiteJitterSeconds(site.ID)) * time.Second
	clock.Advance(time.Minute)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("minute schedule before jitter: %v", err)
	}
	if jitter == 0 {
		waitForSiteJobCalls(t, runner, 3)
	} else {
		assertWorkerRunCount(t, database, site.ID, 8)
		clock.Advance(jitter)
		if err := scheduler.RunOnce(context.Background()); err != nil {
			t.Fatalf("minute schedule at jitter: %v", err)
		}
		waitForSiteJobCalls(t, runner, 3)
	}
	assertWorkerRunCount(t, database, site.ID, 8)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("repeat minute slot: %v", err)
	}
	assertWorkerRunCount(t, database, site.ID, 8)

	clock.Advance(59 * time.Minute)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("hour schedule: %v", err)
	}
	var metadata int64
	if err := database.GORM.Model(&model.CollectionRun{}).Where(
		"site_id = ? AND task_type IN ?", site.ID,
		[]string{constant.TaskTypeUserSync, constant.TaskTypeChannelSync, constant.TaskTypeLogSync},
	).Count(&metadata).Error; err != nil || metadata != 3 {
		t.Fatalf("hourly metadata runs = %d, %v", metadata, err)
	}
	assertWorkerRunCount(t, database, site.ID, 11)
}

func TestQueueConcurrencyUsesLiveSettings(t *testing.T) {
	settings := model.CollectorSettings{
		ProbeConcurrency: 21, RealtimeConcurrency: 11, ResourceConcurrency: 12,
		MetadataConcurrency: 6, UsageConcurrency: 7, BackfillConcurrency: 3,
	}
	wants := map[QueueKind]int{
		QueueProbe: 21, QueueRealtime: 11, QueueResource: 12,
		QueuePerformance: 12, QueueTopup: 12, QueueRedemption: 12, QueueMetadata: 6, QueueUsage: 7, QueueBackfill: 3, QueueValidation: 3,
		QueueAccountRebuild: 3, QueueCustomerRebuild: 3,
	}
	for queue, want := range wants {
		if got := queueConcurrency(queue, settings); got != want {
			t.Errorf("queue %s concurrency = %d, want %d", queue, got, want)
		}
	}
}

func TestWorkerClaimTokensAndRetryMatrix(t *testing.T) {
	firstNonce, err := newWorkerBootNonce()
	if err != nil {
		t.Fatalf("first worker nonce: %v", err)
	}
	secondNonce, err := newWorkerBootNonce()
	if err != nil {
		t.Fatalf("second worker nonce: %v", err)
	}
	if len(firstNonce) != 32 || len(secondNonce) != 32 || firstNonce == secondNonce {
		t.Fatalf("worker boot nonces = %q, %q", firstNonce, secondNonce)
	}
	firstExecutor := &Executor{bootNonce: firstNonce}
	secondExecutor := &Executor{bootNonce: secondNonce}
	firstToken := firstExecutor.nextRequestID(1_752_400_800)
	if firstToken == firstExecutor.nextRequestID(1_752_400_800) || firstToken == secondExecutor.nextRequestID(1_752_400_800) {
		t.Fatalf("worker claim tokens collided at the same second")
	}

	policy := defaultAttemptPolicy()
	maxCases := map[string]int{
		constant.TaskTypeSiteProbe: 1, constant.TaskTypePerformanceSync: 3, constant.TaskTypeTopupSync: 3, constant.TaskTypeRedemptionSync: 3, constant.TaskTypeUsageHour: 4,
		constant.TaskTypeUsageBackfill: 5, constant.TaskTypeUsageValidation: 5,
		constant.TaskTypeAccountRebuild: 5, constant.TaskTypeCustomerRebuild: 5,
	}
	if retryable, _, _ := retryDecision(constant.TaskTypePerformanceSync, 3, errors.New("exhausted"), policy); retryable {
		t.Error("third performance attempt remained retryable")
	}
	for taskType, want := range maxCases {
		if got := maxAttempts(policy, taskType); got != want {
			t.Errorf("max attempts for %s = %d, want %d", taskType, got, want)
		}
	}
	delayCases := []struct {
		taskType string
		attempt  int
		want     time.Duration
	}{
		{constant.TaskTypeUsageHour, 1, time.Minute},
		{constant.TaskTypeUsageHour, 2, 5 * time.Minute},
		{constant.TaskTypeUsageHour, 3, 15 * time.Minute},
		{constant.TaskTypeUsageBackfill, 1, time.Minute},
		{constant.TaskTypeUsageBackfill, 2, 5 * time.Minute},
		{constant.TaskTypeUsageBackfill, 3, 15 * time.Minute},
		{constant.TaskTypeUsageBackfill, 4, time.Hour},
		{constant.TaskTypeUsageValidation, 1, 5 * time.Minute},
		{constant.TaskTypeUsageValidation, 2, 15 * time.Minute},
		{constant.TaskTypeUsageValidation, 3, time.Hour},
		{constant.TaskTypeUsageValidation, 4, 6 * time.Hour},
		{constant.TaskTypeAccountRebuild, 1, time.Minute},
		{constant.TaskTypeAccountRebuild, 2, 5 * time.Minute},
		{constant.TaskTypeAccountRebuild, 3, 15 * time.Minute},
		{constant.TaskTypeAccountRebuild, 4, time.Hour},
		{constant.TaskTypeCustomerRebuild, 1, time.Minute},
		{constant.TaskTypeCustomerRebuild, 2, 5 * time.Minute},
		{constant.TaskTypeCustomerRebuild, 3, 15 * time.Minute},
		{constant.TaskTypeCustomerRebuild, 4, time.Hour},
	}
	for _, test := range delayCases {
		retryable, got, _ := retryDecision(test.taskType, test.attempt, errors.New("retryable"), policy)
		if !retryable || got != test.want {
			t.Errorf("retry %s attempt %d = %t/%s, want true/%s", test.taskType, test.attempt, retryable, got, test.want)
		}
	}
	for _, taskType := range []string{
		constant.TaskTypeUsageBackfill, constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild,
	} {
		if retryable, _, _ := retryDecision(taskType, 5, errors.New("exhausted"), policy); retryable {
			t.Errorf("fifth %s attempt remained retryable", taskType)
		}
	}
	for _, test := range []struct {
		name          string
		retryAfter    time.Duration
		hasRetryAfter bool
		want          time.Duration
	}{
		{name: "short_header_overrides_default", retryAfter: 10 * time.Minute, hasRetryAfter: true, want: 10 * time.Minute},
		{name: "long_header_is_capped", retryAfter: 2 * time.Hour, hasRetryAfter: true, want: time.Hour},
		{name: "missing_header_uses_default", want: 6 * time.Hour},
		{name: "malformed_header_uses_default", want: 6 * time.Hour},
	} {
		t.Run("validation_attempt_4_"+test.name, func(t *testing.T) {
			retryable, delay, _ := retryDecision(constant.TaskTypeUsageValidation, 4, &TaskExecutionError{
				Code: "UPSTREAM_RATE_LIMITED", Retryable: true,
				RetryAfter: test.retryAfter, HasRetryAfter: test.hasRetryAfter,
			}, policy)
			if !retryable || delay != test.want {
				t.Fatalf("Retry-After decision = %t/%s, want true/%s", retryable, delay, test.want)
			}
		})
	}
}

func TestRetryDelayWithJitterIsStableAndDispersed(t *testing.T) {
	base := time.Minute
	first := retryDelayWithJitter(base, constant.TaskTypeUsageBackfill, 10, 20, 1_752_397_200, 2)
	if first != retryDelayWithJitter(base, constant.TaskTypeUsageBackfill, 10, 20, 1_752_397_200, 2) {
		t.Fatal("retry jitter is not deterministic")
	}
	if first < base || first > base+base/5 {
		t.Fatalf("retry jitter = %s, want within [%s,%s]", first, base, base+base/5)
	}
	second := retryDelayWithJitter(base, constant.TaskTypeUsageBackfill, 10, 20, 1_752_400_800, 2)
	if second == first {
		t.Fatalf("different windows received identical retry delay %s", first)
	}
}

func TestInitialBackfillClaimExecutesWindowsConcurrently(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	repository := model.NewCollectionTaskRepository(database.GORM)
	site := createWorkerTestSite(t, database, "initial-parallel", now.Unix())
	scope, err := model.NewUsageBackfillRunScope(true)
	if err != nil {
		t.Fatalf("build initial backfill scope: %v", err)
	}
	run := createWorkerWindowRun(t, database, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerRecovery, constant.CollectionPriorityInitialBackfill, scope,
		now.Unix()-12*3600, now.Unix(), "req_initial_parallel", now.Unix())
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now.Unix(), RequestID: "wrk_initial_parallel",
		MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil || claim.Run.ID != run.ID || len(claim.Windows) != 12 {
		t.Fatalf("initial backfill claim = run:%d windows:%d err:%v", claim.Run.ID, len(claim.Windows), err)
	}
	var mu sync.Mutex
	active, maximum := 0, 0
	handler := JobHandlerFunc(func(context.Context, JobExecution) (JobOutcome, error) {
		mu.Lock()
		active++
		if active > maximum {
			maximum = active
		}
		mu.Unlock()
		time.Sleep(25 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return JobOutcome{}, nil
	})
	executor, err := NewExecutor(ExecutorOptions{
		Repository: repository, Settings: model.NewCollectorSettingRepository(database.GORM), Clock: clock,
		Handlers: map[string]JobHandler{constant.TaskTypeUsageBackfill: handler},
	})
	if err != nil {
		t.Fatalf("create initial parallel executor: %v", err)
	}
	executor.executeClaim(context.Background(), claim)
	if maximum < 2 {
		t.Fatalf("initial backfill maximum concurrency = %d, want at least 2", maximum)
	}
	var completed int64
	if err := database.GORM.Model(&model.CollectionRunWindow{}).Where("run_id = ? AND status = ?", run.ID, model.CollectionTaskStatusSuccess).Count(&completed).Error; err != nil {
		t.Fatalf("count completed initial windows: %v", err)
	}
	if completed != 12 {
		t.Fatalf("completed initial windows = %d, want 12", completed)
	}
}

func TestExecutorDispatchSharedHonorsGlobalPriorityAcrossTaskTypes(t *testing.T) {
	database := openWorkerTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	repository := model.NewCollectionTaskRepository(database.GORM)
	usage := createWorkerWindowRun(t, database, repository, createWorkerTestSite(t, database, "priority-usage", now.Unix()),
		constant.TaskTypeUsageHour, constant.CollectionTriggerSchedule, constant.CollectionPriorityUsageRealtime,
		[]byte("{}"), now.Unix()-3600, now.Unix(), "req_b3a_worker_priority_usage", now.Unix())
	backfillScope, err := model.NewUsageBackfillRunScope(false)
	if err != nil {
		t.Fatalf("build worker backfill scope: %v", err)
	}
	backfill := createWorkerWindowRun(t, database, repository, createWorkerTestSite(t, database, "priority-backfill", now.Unix()),
		constant.TaskTypeUsageBackfill, constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill,
		backfillScope, now.Unix()-3600, now.Unix(), "req_b3a_worker_priority_backfill", now.Unix())
	validation := createWorkerWindowRun(t, database, repository, createWorkerTestSite(t, database, "priority-validation", now.Unix()),
		constant.TaskTypeUsageValidation, constant.CollectionTriggerSchedule, constant.CollectionPriorityDailyValidation,
		[]byte("{}"), now.Unix()-3600, now.Unix(), "req_b3a_worker_priority_validation", now.Unix())

	release := make(chan struct{})
	handler := JobHandlerFunc(func(ctx context.Context, _ JobExecution) (JobOutcome, error) {
		select {
		case <-release:
			return JobOutcome{}, nil
		case <-ctx.Done():
			return JobOutcome{}, ctx.Err()
		}
	})
	executor, err := NewExecutor(ExecutorOptions{
		Repository: repository, Settings: model.NewCollectorSettingRepository(database.GORM), Clock: clock,
		Handlers: map[string]JobHandler{
			constant.TaskTypeUsageHour: handler, constant.TaskTypeUsageBackfill: handler,
			constant.TaskTypeUsageValidation: handler,
		},
	})
	if err != nil {
		t.Fatalf("create shared-priority executor: %v", err)
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
		executor.active.Wait()
	}()
	settings := model.CollectorSettings{UsageConcurrency: 1, BackfillConcurrency: 1}
	if err := executor.dispatchShared(ctx, ctx, settings); err != nil {
		t.Fatalf("dispatch shared priority: %v", err)
	}

	loaded := make([]model.CollectionRun, 0, 3)
	if err := database.GORM.Where("id IN ?", []int64{usage.ID, backfill.ID, validation.ID}).Find(&loaded).Error; err != nil {
		t.Fatalf("load shared-priority runs: %v", err)
	}
	byID := make(map[int64]model.CollectionRun, len(loaded))
	for _, run := range loaded {
		byID[run.ID] = run
	}
	wantFirstRequest := "wrk_" + executor.bootNonce + "_2"
	wantSecondRequest := "wrk_" + executor.bootNonce + "_3"
	if got := byID[usage.ID]; got.Status != model.CollectionTaskStatusRunning || got.LastRequestID != wantFirstRequest {
		t.Errorf("usage global-priority claim = status:%s request:%s, want running/%s", got.Status, got.LastRequestID, wantFirstRequest)
	}
	if got := byID[backfill.ID]; got.Status != model.CollectionTaskStatusRunning || got.LastRequestID != wantSecondRequest {
		t.Errorf("backfill global-priority claim = status:%s request:%s, want running/%s", got.Status, got.LastRequestID, wantSecondRequest)
	}
	if got := byID[validation.ID]; got.Status != model.CollectionTaskStatusPending || got.LastRequestID != validation.LastRequestID {
		t.Errorf("validation after shared capacity filled = status:%s request:%s, want pending/%s",
			got.Status, got.LastRequestID, validation.LastRequestID)
	}
	close(release)
	released = true
	executor.active.Wait()
}

func TestSharedCollectionQueuesUseTheConfiguredCapacityPool(t *testing.T) {
	for _, test := range []struct {
		queue QueueKind
		want  QueueKind
	}{
		{queue: QueueUsage, want: QueueUsage},
		{queue: QueueBackfill, want: QueueBackfill},
		{queue: QueueInitialBackfill, want: QueueInitialBackfill},
		{queue: QueueValidation, want: QueueBackfill},
		{queue: QueueAccountRebuild, want: QueueBackfill},
		{queue: QueueCustomerRebuild, want: QueueBackfill},
	} {
		if got := queueCapacityKey(test.queue); got != test.want {
			t.Errorf("capacity key for %s = %s, want %s", test.queue, got, test.want)
		}
	}
}

func TestInitialBackfillUsesConfiguredBackfillConcurrency(t *testing.T) {
	settings := model.CollectorSettings{BackfillConcurrency: 2}
	if queueConcurrency(QueueBackfill, settings) != 2 {
		t.Fatal("ordinary backfill concurrency changed unexpectedly")
	}
	if queueConcurrency(QueueInitialBackfill, settings) != 0 {
		t.Fatal("initial backfill must use the explicit shared-dispatch setting path")
	}
}

func TestCollectorSettingsHotReloadCadenceAndConcurrency(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	for _, key := range []string{
		"collector.probe_interval_seconds",
		"collector.realtime_interval_seconds",
		"collector.resource_interval_seconds",
	} {
		overrideWorkerSetting(t, database, key, "60")
	}
	overrideWorkerSetting(t, database, "collector.usage_concurrency", "1")
	settingsRepository := model.NewCollectorSettingRepository(database.GORM)
	site := createWorkerTestSite(t, database, "hot-settings", now.Unix())
	runner := &recordingSiteJobRunner{calls: make(chan string, 32)}
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM), Settings: settingsRepository, Clock: clock, SiteJobs: runner,
	})
	if err != nil {
		t.Fatalf("create hot settings scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("hot settings startup: %v", err)
	}
	waitForSiteJobCalls(t, runner, 3)
	assertWorkerRunCount(t, database, site.ID, 8)
	for _, key := range []string{
		"collector.probe_interval_seconds",
		"collector.realtime_interval_seconds",
		"collector.resource_interval_seconds",
	} {
		writeWorkerSetting(t, database, key, "120")
	}
	clock.Advance(time.Minute)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reload 120 second cadence: %v", err)
	}
	assertWorkerRunCount(t, database, site.ID, 8)
	jitter := time.Duration(stableSiteJitterSeconds(site.ID)) * time.Second
	clock.Advance(time.Minute + jitter)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("run reloaded 120 second cadence: %v", err)
	}
	waitForSiteJobCalls(t, runner, 3)
	assertWorkerRunCount(t, database, site.ID, 8)
	for _, key := range []string{
		"collector.probe_interval_seconds",
		"collector.realtime_interval_seconds",
		"collector.resource_interval_seconds",
	} {
		writeWorkerSetting(t, database, key, "60")
	}
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reload 60 second cadence: %v", err)
	}
	assertWorkerRunCount(t, database, site.ID, 8)
	clock.Advance(time.Minute)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("run reloaded 60 second cadence: %v", err)
	}
	waitForSiteJobCalls(t, runner, 3)
	assertWorkerRunCount(t, database, site.ID, 8)

	handler := JobHandlerFunc(func(context.Context, JobExecution) (JobOutcome, error) { return JobOutcome{}, nil })
	executor, err := NewExecutor(ExecutorOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM), Settings: settingsRepository, Clock: clock,
		Handlers: map[string]JobHandler{
			constant.TaskTypeUsageHour: handler, constant.TaskTypeUsageBackfill: handler,
			constant.TaskTypeUsageValidation: handler,
		},
	})
	if err != nil {
		t.Fatalf("create hot settings executor: %v", err)
	}
	settings, err := settingsRepository.Load(context.Background())
	if err != nil {
		t.Fatalf("load concurrency 1: %v", err)
	}
	if !executor.limiter.tryAcquire(QueueUsage, settings.UsageConcurrency) {
		t.Fatal("reserve usage concurrency slot")
	}
	if taskTypes := executor.registeredSharedTaskTypes(settings); containsTaskType(taskTypes, constant.TaskTypeUsageHour) ||
		!containsTaskType(taskTypes, constant.TaskTypeUsageBackfill) ||
		!containsTaskType(taskTypes, constant.TaskTypeUsageValidation) {
		t.Fatalf("shared task types at usage limit = %#v", taskTypes)
	}
	writeWorkerSetting(t, database, "collector.usage_concurrency", "2")
	settings, err = settingsRepository.Load(context.Background())
	if err != nil {
		t.Fatalf("reload concurrency 2: %v", err)
	}
	if taskTypes := executor.registeredSharedTaskTypes(settings); !containsTaskType(taskTypes, constant.TaskTypeUsageHour) ||
		!containsTaskType(taskTypes, constant.TaskTypeUsageBackfill) ||
		!containsTaskType(taskTypes, constant.TaskTypeUsageValidation) {
		t.Fatalf("shared task types after concurrency reload = %#v", taskTypes)
	}
	executor.limiter.release(QueueUsage)
}

func containsTaskType(taskTypes []string, want string) bool {
	for _, taskType := range taskTypes {
		if taskType == want {
			return true
		}
	}
	return false
}

func overrideWorkerSetting(t *testing.T, database *model.Database, key, value string) {
	t.Helper()
	type settingState struct {
		Value     string `gorm:"column:setting_value"`
		UpdatedAt int64  `gorm:"column:updated_at"`
	}
	var original settingState
	if err := database.GORM.Table("platform_setting").Select("setting_value, updated_at").
		Where("setting_key = ?", key).Take(&original).Error; err != nil {
		t.Fatalf("load original setting %s: %v", key, err)
	}
	t.Cleanup(func() {
		_ = database.GORM.Table("platform_setting").Where("setting_key = ?", key).Updates(map[string]any{
			"setting_value": original.Value, "updated_at": original.UpdatedAt,
		}).Error
	})
	writeWorkerSetting(t, database, key, value)
}

func writeWorkerSetting(t *testing.T, database *model.Database, key, value string) {
	t.Helper()
	result := database.GORM.Table("platform_setting").Where("setting_key = ?", key).
		Updates(map[string]any{"setting_value": value, "updated_at": time.Now().Unix()})
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("write setting %s=%s: rows=%d err=%v", key, value, result.RowsAffected, result.Error)
	}
}

type recordingSiteJobRunner struct {
	mu    sync.Mutex
	calls chan string
}

func (runner *recordingSiteJobRunner) ExecutePeriodicSiteTask(
	_ context.Context,
	taskType string,
	_ int64,
	_ int,
	_ string,
) (int64, int64, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls <- taskType
	return 1, 1, nil
}

func allSiteJobsCalled(calls map[string]bool) bool {
	for _, called := range calls {
		if !called {
			return false
		}
	}
	return true
}

func waitForSiteJobCalls(t *testing.T, runner *recordingSiteJobRunner, count int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for received := 0; received < count; received++ {
		select {
		case <-runner.calls:
		case <-deadline:
			t.Fatalf("received %d scheduled site jobs, want %d", received, count)
		}
	}
}

func waitForScheduledSiteJobFamilies(t *testing.T, clock *testsupport.FakeClock, runner *recordingSiteJobRunner) {
	t.Helper()
	want := map[string]bool{
		constant.TaskTypeSiteProbe: false, constant.TaskTypeRealtimeStat: false, constant.TaskTypeResourceSnapshot: false,
	}
	deadline := time.Now().Add(10 * time.Second)
	for !allSiteJobsCalled(want) && time.Now().Before(deadline) {
		clock.Advance(time.Second)
		select {
		case taskType := <-runner.calls:
			if _, expected := want[taskType]; expected {
				want[taskType] = true
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !allSiteJobsCalled(want) {
		t.Fatalf("scheduled site jobs = %#v", want)
	}
}

func openWorkerTestDatabase(t *testing.T) *model.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 4, MaxOpen: 20, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open worker test database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve worker test lock connection: %v", err)
	}
	var acquired sql.NullInt64
	const lockName = "new-api-pilot-site-service-integration"
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", lockName).Scan(&acquired); err != nil ||
		!acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire worker test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run worker test migrations: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		for _, statement := range []string{
			"DELETE rw FROM collection_run_window rw JOIN collection_run r ON r.id = rw.run_id JOIN site s ON s.id = r.site_id WHERE s.name LIKE 'Run run-b3a-worker-%'",
			"DELETE r FROM collection_run r JOIN site s ON s.id = r.site_id WHERE s.name LIKE 'Run run-b3a-worker-%'",
			"DELETE c FROM site_capability c JOIN site s ON s.id = c.site_id WHERE s.name LIKE 'Run run-b3a-worker-%'",
			"DELETE FROM site WHERE name LIKE 'Run run-b3a-worker-%'",
		} {
			_, _ = database.SQL.ExecContext(cleanupContext, statement)
		}
		_, _ = connection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
	})
	return database
}

func createWorkerTestSite(t *testing.T, database *model.Database, suffix string, now int64) model.Site {
	t.Helper()
	unique := fmt.Sprintf("run-b3a-worker-%s-%d", suffix, time.Now().UnixNano())
	site := model.Site{
		Name: "Run " + unique, BaseURL: "https://" + unique + ".example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create worker site: %v", err)
	}
	return site
}

func createWorkerWindowRun(
	t *testing.T,
	database *model.Database,
	repository *model.CollectionTaskRepository,
	site model.Site,
	taskType, triggerType string,
	priority int,
	scope []byte,
	start, end int64,
	requestID string,
	now int64,
) model.CollectionRun {
	t.Helper()
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: site.ID, CapabilityKey: key, Status: status, CheckedAt: now,
		})
	}
	if err := model.NewSiteRepository(database.GORM).ReplaceCapabilities(context.Background(), site.ID, capabilities); err != nil {
		t.Fatalf("create worker window capabilities %s: %v", taskType, err)
	}
	run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
		TaskType: taskType, TriggerType: triggerType, StartTimestamp: &start, EndTimestamp: &end,
		Scope: scope, Priority: priority, RequestID: requestID, Now: now,
	})
	if err != nil {
		t.Fatalf("build worker window run %s: %v", taskType, err)
	}
	created, _, err := model.NewSiteRepository(database.GORM).CreateOrGetRun(context.Background(), &run)
	if err != nil {
		t.Fatalf("create worker window run %s: %v", taskType, err)
	}
	created, err = repository.MaterializeRunWindows(context.Background(), created.ID, now, 1000)
	if err != nil {
		t.Fatalf("materialize worker window run %s: %v", taskType, err)
	}
	return created
}

func assertWorkerRunCount(t *testing.T, database *model.Database, siteID int64, expected int64) {
	t.Helper()
	var count int64
	if err := database.GORM.Model(&model.CollectionRun{}).Where("site_id = ?", siteID).Count(&count).Error; err != nil || count != expected {
		t.Fatalf("site %d run count = %d, want %d, err=%v", siteID, count, expected, err)
	}
}

func finishWorkerRuns(t *testing.T, database *model.Database, siteID, now int64) {
	t.Helper()
	if err := database.GORM.Model(&model.CollectionRun{}).
		Where("site_id = ? AND status IN ('pending','running')", siteID).
		Updates(map[string]any{
			"status": "success", "active_key": nil, "heartbeat_at": nil,
			"finished_at": now, "updated_at": now,
		}).Error; err != nil {
		t.Fatalf("finish worker runs: %v", err)
	}
}
