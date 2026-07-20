package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSchedulerDefaultTickIsOneMinute(t *testing.T) {
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(nil),
		Settings:   model.NewCollectorSettingRepository(nil),
		Clock:      testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if scheduler.tick != time.Minute {
		t.Fatalf("default scheduler tick = %s, want %s", scheduler.tick, time.Minute)
	}
}

func TestSchedulerFastFamiliesRemainEligibleWhileOnlineStatusUnknown(t *testing.T) {
	site := model.Site{
		ID: 42, ConfigVersion: 2,
		ManagementStatus:  constant.SiteManagementActive,
		AuthStatus:        constant.SiteAuthAuthorized,
		OnlineStatus:      constant.SiteOnlineUnknown,
		StatisticsStatus:  constant.SiteStatisticsBackfilling,
		DataExportEnabled: true,
	}
	for _, taskType := range []string{
		constant.TaskTypeSiteProbe,
		constant.TaskTypeRealtimeStat,
		constant.TaskTypeResourceSnapshot,
	} {
		if !schedulerSiteEligible(site, taskType) {
			t.Fatalf("task type %s became ineligible while online status is unknown", taskType)
		}
	}
}

func TestFastTaskDispatcherDeduplicatesAndSerializesEachSite(t *testing.T) {
	started := make(chan scheduledFastTask, 3)
	release := make(chan struct{})
	var active int32
	var maximum int32
	dispatcher := newFastTaskDispatcher(func(task scheduledFastTask) {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		started <- task
		<-release
		atomic.AddInt32(&active, -1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := scheduledFastTask{ctx: ctx, site: model.Site{ID: 1}, taskType: constant.TaskTypeSiteProbe, requestID: "fast_one", concurrency: 2}
	if !dispatcher.Enqueue(first) {
		t.Fatal("enqueue first site task")
	}
	if dispatcher.Enqueue(first) {
		t.Fatal("duplicate fast task was accepted")
	}
	if !dispatcher.Enqueue(scheduledFastTask{ctx: ctx, site: model.Site{ID: 1}, taskType: constant.TaskTypeRealtimeStat, requestID: "fast_two", concurrency: 2}) {
		t.Fatal("enqueue second family for same site")
	}
	if !dispatcher.Enqueue(scheduledFastTask{ctx: ctx, site: model.Site{ID: 2}, taskType: constant.TaskTypeSiteProbe, requestID: "fast_three", concurrency: 2}) {
		t.Fatal("enqueue different site task")
	}

	firstStarted := <-started
	secondStarted := <-started
	if firstStarted.site.ID == secondStarted.site.ID {
		t.Fatalf("same site ran concurrently: %d and %d", firstStarted.site.ID, secondStarted.site.ID)
	}
	if atomic.LoadInt32(&maximum) != 2 {
		t.Fatalf("fast concurrency = %d, want 2", maximum)
	}
	close(release)
	thirdStarted := <-started
	if thirdStarted.site.ID != 1 || thirdStarted.taskType != constant.TaskTypeRealtimeStat {
		t.Fatalf("serialized task = %#v", thirdStarted)
	}
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	dispatcher.Shutdown(shutdownContext)
}

func TestSchedulerPerformanceSyncUsesDurableRunAndIndependentQueue(t *testing.T) {
	database := openWorkerTestDatabase(t)
	if err := model.NewSeeder(database.SQL).Run(context.Background()); err != nil {
		t.Fatalf("seed scheduler settings: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "durable-performance", now.Unix())
	makeSchedulerSiteRunnable(t, database, site, now.Unix())
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("scheduler startup: %v", err)
	}
	defer scheduler.shutdownFastTasks()

	var runs []model.CollectionRun
	if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypePerformanceSync).
		Order("id ASC").Find(&runs).Error; err != nil {
		t.Fatalf("load performance runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != model.CollectionTaskStatusPending || runs[0].ActiveKey == nil {
		t.Fatalf("durable performance runs = %#v", runs)
	}
	if queue, ok := QueueForTask(constant.TaskTypePerformanceSync); !ok || queue != QueuePerformance {
		t.Fatalf("performance queue = %q, valid=%t", queue, ok)
	}
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("repeat performance slot: %v", err)
	}
	var count int64
	if err := database.GORM.Model(&model.CollectionRun{}).
		Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypePerformanceSync).
		Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("same-slot performance runs = %d, %v", count, err)
	}
	repository := model.NewCollectionTaskRepository(database.GORM)
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypePerformanceSync}, Now: now.Unix(),
		RequestID: "wrk_performance_first", MaxWindow: 24,
	})
	if err != nil || claim.Run.ID != runs[0].ID || claim.Run.Status != model.CollectionTaskStatusRunning || claim.Run.RetryCount != 1 {
		t.Fatalf("claim performance run = %#v, %v", claim.Run, err)
	}
	if err := repository.Heartbeat(context.Background(), claim.Run.ID, claim.RequestID, now.Unix()+1); err != nil {
		t.Fatalf("heartbeat performance run: %v", err)
	}
	if recovered, err := repository.RecoverRunning(context.Background(), now.Unix()+2, nil, defaultAttemptPolicy()); err != nil || recovered != 1 {
		t.Fatalf("restart recovery = %d, %v", recovered, err)
	}
	recovered, err := model.NewSiteRepository(database.GORM).FindCollectionRunByID(context.Background(), claim.Run.ID)
	if err != nil || recovered.Status != model.CollectionTaskStatusPending || recovered.RetryCount != 1 || recovered.ActiveKey == nil {
		t.Fatalf("recovered performance run = %#v, %v", recovered, err)
	}
}

func TestSchedulerDurableSyncUsesIndependentQueues(t *testing.T) {
	database := openWorkerTestDatabase(t)
	if err := model.NewSeeder(database.SQL).Run(context.Background()); err != nil {
		t.Fatalf("seed scheduler settings: %v", err)
	}
	now := time.Unix(1_752_401_400, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "durable-finance", now.Unix())
	makeSchedulerSiteRunnable(t, database, site, now.Unix())
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("scheduler startup: %v", err)
	}
	defer scheduler.shutdownFastTasks()

	repository := model.NewCollectionTaskRepository(database.GORM)
	for _, test := range []struct {
		taskType string
		queue    QueueKind
	}{
		{constant.TaskTypeTopupSync, QueueTopup},
		{constant.TaskTypeRedemptionSync, QueueRedemption},
		{constant.TaskTypeUpstreamTaskSync, QueueMetadata},
		{constant.TaskTypeModelMetaSync, QueueMetadata},
	} {
		var runs []model.CollectionRun
		if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, test.taskType).
			Order("id ASC").Find(&runs).Error; err != nil {
			t.Fatalf("load %s runs: %v", test.taskType, err)
		}
		if len(runs) != 1 || runs[0].Status != model.CollectionTaskStatusPending || runs[0].ActiveKey == nil {
			t.Fatalf("durable %s runs = %#v", test.taskType, runs)
		}
		if queue, ok := QueueForTask(test.taskType); !ok || queue != test.queue {
			t.Fatalf("%s queue = %q, valid=%t", test.taskType, queue, ok)
		}
		claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
			TaskTypes: []string{test.taskType}, Now: now.Unix(),
			RequestID: "wrk_" + test.taskType, MaxWindow: 24,
		})
		if err != nil || claim.Run.ID != runs[0].ID || claim.Run.RetryCount != 1 {
			t.Fatalf("claim %s = %#v, %v", test.taskType, claim.Run, err)
		}
		if err := repository.Heartbeat(context.Background(), claim.Run.ID, claim.RequestID, now.Unix()+1); err != nil {
			t.Fatalf("heartbeat %s: %v", test.taskType, err)
		}
	}
	if recovered, err := repository.RecoverRunning(context.Background(), now.Unix()+2, nil, defaultAttemptPolicy()); err != nil || recovered != 4 {
		t.Fatalf("durable restart recovery = %d, %v", recovered, err)
	}
	for _, taskType := range []string{constant.TaskTypeTopupSync, constant.TaskTypeRedemptionSync, constant.TaskTypeUpstreamTaskSync, constant.TaskTypeModelMetaSync} {
		var run model.CollectionRun
		if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, taskType).Take(&run).Error; err != nil || run.Status != model.CollectionTaskStatusPending || run.RetryCount != 1 || run.ActiveKey == nil {
			t.Fatalf("recovered %s = %#v, %v", taskType, run, err)
		}
	}
}

func TestScheduledFastFailureTransitionWritesOneTerminalDiagnostic(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "fast-transition", now.Unix())
	runner := sitePeriodicRunnerFunc(func(_ context.Context, taskType string, siteID int64, _ int, _ string) (int64, int64, error) {
		if taskType != constant.TaskTypeSiteProbe {
			return 1, 1, nil
		}
		if err := database.GORM.Model(&model.Site{}).Where("id = ?", siteID).Updates(map[string]any{
			"online_status":    constant.SiteOnlineOffline,
			"probe_fail_count": 3,
			"updated_at":       now.Unix() + 1,
		}).Error; err != nil {
			return 0, 0, err
		}
		// Probe reachability failures are intentionally represented as a soft
		// result by SiteService; this verifies the transition is still recorded.
		return 1, 0, nil
	})
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
		SiteJobs:   runner,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	task := scheduledFastTask{
		ctx: context.Background(), site: site, taskType: constant.TaskTypeSiteProbe,
		requestID: "sch_site_probe_transition", concurrency: 1,
	}
	scheduler.executeFastTask(task)
	scheduler.executeFastTask(scheduledFastTask{
		ctx: context.Background(), site: site, taskType: constant.TaskTypeSiteProbe,
		requestID: "sch_site_probe_transition_repeat", concurrency: 1,
	})
	var diagnostics []model.CollectionRun
	if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypeSiteProbe).
		Order("id ASC").Find(&diagnostics).Error; err != nil {
		t.Fatalf("load fast diagnostics: %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Status != model.CollectionTaskStatusFailed ||
		diagnostics[0].ActiveKey != nil || diagnostics[0].ErrorCode != scheduledStateTransitionErrorCode {
		t.Fatalf("fast diagnostics = %#v", diagnostics)
	}
}

func TestManualFastTaskRemainsPersistent(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	site := createWorkerTestSite(t, database, "manual-fast", now.Unix())
	created, deduplicated, err := model.NewCollectionTaskRepository(database.GORM).EnqueueSiteTask(context.Background(), model.SiteTaskEnqueueRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: constant.TaskTypeSiteProbe,
		TriggerType: constant.CollectionTriggerManual, RequestID: "manual_fast_probe", Now: now.Unix(),
	})
	if err != nil || deduplicated || created.Status != model.CollectionTaskStatusPending {
		t.Fatalf("manual fast enqueue = %#v, deduplicated=%t, err=%v", created, deduplicated, err)
	}
}

func TestSchedulerQueuesMissedUsageHourWhenSiteRecoversInSameHour(t *testing.T) {
	database := openWorkerTestDatabase(t)
	base := time.Unix(1_752_400_800, 0)
	hourStart := base.Unix() - base.Unix()%3600
	clock := testsupport.NewFakeClock(time.Unix(hourStart+6*60, 0))
	site := createWorkerTestSite(t, database, "usage-recovery", clock.Now().Unix())
	makeSchedulerSiteRunnable(t, database, site, clock.Now().Unix())
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"online_status": constant.SiteOnlineOffline, "updated_at": clock.Now().Unix()}).Error; err != nil {
		t.Fatalf("mark site offline: %v", err)
	}
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("start scheduler while offline: %v", err)
	}
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"online_status": constant.SiteOnlineOnline, "updated_at": clock.Now().Unix() + 1}).Error; err != nil {
		t.Fatalf("recover site: %v", err)
	}
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("run scheduler after recovery: %v", err)
	}
	var runs []model.CollectionRun
	if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypeUsageHour).
		Order("id ASC").Find(&runs).Error; err != nil {
		t.Fatalf("load recovered usage run: %v", err)
	}
	if len(runs) != 1 || runs[0].StartTimestamp == nil || runs[0].EndTimestamp == nil ||
		*runs[0].StartTimestamp != hourStart-3600 || *runs[0].EndTimestamp != hourStart {
		t.Fatalf("recovered usage runs = %#v", runs)
	}
}

func TestSchedulerRunErrorCancelsFastTasks(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	createWorkerTestSite(t, database, "run-error-fast-cleanup", now.Unix())
	runner := newBlockingSiteJobRunner()
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
		Tick:       time.Second,
		SiteJobs:   runner,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	startupContext, cancelStartup := context.WithCancel(context.Background())
	defer cancelStartup()
	if err := scheduler.Startup(startupContext); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}
	waitForBlockingSiteJobStart(t, runner)

	scheduler.mu.Lock()
	scheduler.settings = model.NewCollectorSettingRepository(nil)
	scheduler.mu.Unlock()
	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runDone := make(chan error, 1)
	go func() {
		runDone <- scheduler.Run(runContext)
	}()

	deadline := time.After(5 * time.Second)
	for {
		clock.Advance(time.Second)
		select {
		case err := <-runDone:
			if err == nil {
				t.Fatal("scheduler run returned nil after a settings failure")
			}
			waitForBlockingSiteJobCancellation(t, runner)
			assertFastTaskLifecycleStopped(t, scheduler)
			return
		case <-deadline:
			t.Fatal("scheduler run did not return after a non-cancellation error")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSchedulerStartupFailureCancelsFastTasks(t *testing.T) {
	database := openWorkerTestDatabase(t)
	base := time.Unix(1_752_400_800, 0)
	hourStart := base.Unix() - base.Unix()%3600
	now := time.Unix(hourStart+6*60, 0)
	clock := testsupport.NewFakeClock(now)
	site := createWorkerTestSite(t, database, "startup-fast-cleanup", now.Unix())
	makeSchedulerSiteRunnable(t, database, site, now.Unix())
	runner := newBlockingSiteJobRunner()
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      clock,
		SiteJobs:   runner,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}

	lockContext, cancelLock := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelLock()
	lockConnection, err := database.SQL.Conn(lockContext)
	if err != nil {
		t.Fatalf("reserve site lock connection: %v", err)
	}
	defer lockConnection.Close()
	lockTransaction, err := lockConnection.BeginTx(lockContext, nil)
	if err != nil {
		t.Fatalf("begin site lock transaction: %v", err)
	}
	defer lockTransaction.Rollback()
	var lockedSiteID int64
	if err := lockTransaction.QueryRowContext(lockContext, "SELECT id FROM site WHERE id = ? FOR UPDATE", site.ID).Scan(&lockedSiteID); err != nil || lockedSiteID != site.ID {
		t.Fatalf("lock scheduler site = %d, %v", lockedSiteID, err)
	}

	startupContext, cancelStartup := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStartup()
	startupDone := make(chan error, 1)
	go func() {
		startupDone <- scheduler.Startup(startupContext)
	}()
	waitForBlockingSiteJobStart(t, runner)
	select {
	case err := <-startupDone:
		if err == nil {
			t.Fatal("scheduler startup succeeded while its site row was locked")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler startup did not fail after the locked usage enqueue timed out")
	}
	waitForBlockingSiteJobCancellation(t, runner)
	assertFastTaskLifecycleStopped(t, scheduler)
}

type blockingSiteJobRunner struct {
	started  chan struct{}
	canceled chan struct{}
}

func newBlockingSiteJobRunner() *blockingSiteJobRunner {
	return &blockingSiteJobRunner{
		started:  make(chan struct{}, 1),
		canceled: make(chan struct{}, 1),
	}
}

func (runner *blockingSiteJobRunner) ExecutePeriodicSiteTask(
	ctx context.Context,
	_ string,
	_ int64,
	_ int,
	_ string,
) (int64, int64, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case runner.canceled <- struct{}{}:
	default:
	}
	return 0, 0, ctx.Err()
}

func waitForBlockingSiteJobStart(t *testing.T, runner *blockingSiteJobRunner) {
	t.Helper()
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("fast site job did not start")
	}
}

func waitForBlockingSiteJobCancellation(t *testing.T, runner *blockingSiteJobRunner) {
	t.Helper()
	select {
	case <-runner.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("fast site job was not canceled")
	}
}

func assertFastTaskLifecycleStopped(t *testing.T, scheduler *Scheduler) {
	t.Helper()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.fastTasks != nil || scheduler.fastTaskCtx != nil || scheduler.fastTaskCancel != nil || scheduler.fastStartup {
		t.Fatalf(
			"fast task lifecycle remains attached: dispatcher=%p ctx=%v cancel=%t startup=%t",
			scheduler.fastTasks, scheduler.fastTaskCtx, scheduler.fastTaskCancel != nil, scheduler.fastStartup,
		)
	}
}

type sitePeriodicRunnerFunc func(context.Context, string, int64, int, string) (int64, int64, error)

func (runner sitePeriodicRunnerFunc) ExecutePeriodicSiteTask(
	ctx context.Context,
	taskType string,
	siteID int64,
	expectedConfigVersion int,
	requestID string,
) (int64, int64, error) {
	return runner(ctx, taskType, siteID, expectedConfigVersion, requestID)
}

func makeSchedulerSiteRunnable(t *testing.T, database *model.Database, site model.Site, now int64) {
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
		t.Fatalf("make scheduler site runnable: %v", err)
	}
}
