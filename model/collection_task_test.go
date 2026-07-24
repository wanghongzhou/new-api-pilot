package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

func TestCollectionTaskConcurrentClaimCrossSiteAndPerSiteSerial(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	firstSite := createRunnableSite(t, database, "run-b3a-claim-one", now)
	secondSite := createRunnableSite(t, database, "run-b3a-claim-two", now)
	firstRun := createB3AWindowRun(t, database, firstSite, constant.TaskTypeUsageHour,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityUsageRealtime, now-3600, now, "req_b3a_claim_one", now)
	secondRun := createB3AWindowRun(t, database, secondSite, constant.TaskTypeUsageHour,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityUsageRealtime, now-3600, now, "req_b3a_claim_two", now)
	repository := NewCollectionTaskRepository(database.GORM)
	for _, run := range []CollectionRun{firstRun, secondRun} {
		materialized, err := repository.MaterializeRunWindows(ctx, run.ID, now, 1000)
		if err != nil || materialized.TotalWindows != 1 || materialized.WindowsInitializedAt == nil {
			t.Fatalf("materialize run %d = %#v, %v", run.ID, materialized, err)
		}
	}

	type claimResult struct {
		claim CollectionTaskClaim
		err   error
	}
	results := make(chan claimResult, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			claim, err := NewCollectionTaskRepository(database.GORM).ClaimNext(ctx, CollectionTaskClaimOptions{
				TaskTypes: []string{constant.TaskTypeUsageHour}, Now: now,
				RequestID: fmt.Sprintf("req_b3a_worker_%d", index), MaxWindow: 24,
			})
			results <- claimResult{claim: claim, err: err}
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	claims := make([]CollectionTaskClaim, 0, 2)
	seenRuns := map[int64]struct{}{}
	seenSites := map[int64]struct{}{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent claim: %v", result.err)
		}
		if len(result.claim.Windows) != 1 || result.claim.Windows[0].AttemptCount != 1 {
			t.Fatalf("claim windows = %#v", result.claim.Windows)
		}
		if _, duplicate := seenRuns[result.claim.Run.ID]; duplicate {
			t.Fatalf("run %d was claimed twice", result.claim.Run.ID)
		}
		seenRuns[result.claim.Run.ID] = struct{}{}
		seenSites[*result.claim.Run.SiteID] = struct{}{}
		claims = append(claims, result.claim)
	}
	if len(seenRuns) != 2 || len(seenSites) != 2 {
		t.Fatalf("cross-site claims runs=%#v sites=%#v", seenRuns, seenSites)
	}

	probe, err := NewSiteCollectionRun(firstSite, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_b3a_probe_serial", Now: now,
	})
	if err != nil {
		t.Fatalf("build serial probe: %v", err)
	}
	if _, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &probe); err != nil {
		t.Fatalf("create serial probe: %v", err)
	}
	probeClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now,
		RequestID: "req_b3a_probe_blocked", MaxWindow: 24,
	})
	if err != nil || probeClaim.Run.TaskType != constant.TaskTypeSiteProbe {
		t.Fatalf("same-site probe should run independently of usage hour: %#v, %v", probeClaim.Run, err)
	}
	if _, err := repository.ReleaseClaim(ctx, probeClaim, now+1); err != nil {
		t.Fatalf("release probe claim: %v", err)
	}
	for _, taskType := range []string{constant.TaskTypeUserSync, constant.TaskTypeChannelSync} {
		metadataRun, err := NewSiteCollectionRun(firstSite, SiteRunSpec{
			TaskType: taskType, TriggerType: constant.CollectionTriggerSchedule,
			Priority: 0, RequestID: "req_b3a_metadata_" + taskType, Now: now,
		})
		if err != nil {
			t.Fatalf("build %s: %v", taskType, err)
		}
		if _, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &metadataRun); err != nil {
			t.Fatalf("create %s: %v", taskType, err)
		}
		metadataClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
			TaskTypes: []string{taskType}, Now: now, RequestID: "req_b3a_claim_" + taskType, MaxWindow: 24,
		})
		if err != nil || metadataClaim.Run.TaskType != taskType {
			t.Fatalf("%s should run independently of usage hour: %#v, %v", taskType, metadataClaim.Run, err)
		}
		if _, err := repository.ReleaseClaim(ctx, metadataClaim, now+1); err != nil {
			t.Fatalf("release %s: %v", taskType, err)
		}
	}
	for _, claim := range claims {
		if _, err := repository.ReleaseClaim(ctx, claim, now+1); err != nil {
			t.Fatalf("release claim %d: %v", claim.Run.ID, err)
		}
	}
}

func TestCollectionTaskPriorityAndMaterializationRestart(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	highSite := createRunnableSite(t, database, "run-b3a-priority-high", now)
	lowSite := createRunnableSite(t, database, "run-b3a-priority-low", now)
	highRun := createB3AWindowRun(t, database, highSite, constant.TaskTypeUsageHour,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityUsageRealtime, now-3600, now, "req_b3a_priority_high", now)
	lowRun := createB3AWindowRun(t, database, lowSite, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerRecovery, constant.CollectionPriorityInitialBackfill, now-3600, now, "req_b3a_priority_low", now)
	repository := NewCollectionTaskRepository(database.GORM)
	for _, run := range []CollectionRun{lowRun, highRun} {
		if _, err := repository.MaterializeRunWindows(ctx, run.ID, now, 1000); err != nil {
			t.Fatalf("materialize priority run %d: %v", run.ID, err)
		}
	}
	claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageHour, constant.TaskTypeUsageBackfill},
		Now:       now, RequestID: "req_b3a_priority_claim", MaxWindow: 24,
	})
	if err != nil || claim.Run.ID != highRun.ID || claim.Run.Priority != constant.CollectionPriorityUsageRealtime {
		t.Fatalf("priority claim = %#v, %v", claim.Run, err)
	}
	if _, err := repository.ReleaseClaim(ctx, claim, now+1); err != nil {
		t.Fatalf("release priority claim: %v", err)
	}

	restartSite := createRunnableSite(t, database, "run-b3a-materialize-restart", now)
	startHour := now - 2_001*3600
	restartRun := createB3AWindowRun(t, database, restartSite, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerRecovery, constant.CollectionPriorityInitialBackfill,
		startHour, now, "req_b3a_materialize_restart", now)
	partial := make([]CollectionRunWindow, 500)
	for index := range partial {
		partial[index] = CollectionRunWindow{
			RunID: restartRun.ID, SiteID: restartSite.ID, HourTS: startHour + int64(index)*3600,
			Status: CollectionTaskStatusPending, UpdatedAt: now,
		}
	}
	if err := database.GORM.CreateInBatches(&partial, 500).Error; err != nil {
		t.Fatalf("create partial materialization: %v", err)
	}
	materialized, err := repository.MaterializeRunWindows(ctx, restartRun.ID, now+1, 1000)
	if err != nil || materialized.TotalWindows != 2_001 || materialized.WindowsInitializedAt == nil {
		t.Fatalf("resume materialization = total:%d initialized:%v err:%v",
			materialized.TotalWindows, materialized.WindowsInitializedAt, err)
	}
	var count int64
	if err := database.GORM.Model(&CollectionRunWindow{}).Where("run_id = ?", restartRun.ID).Count(&count).Error; err != nil || count != 2_001 {
		t.Fatalf("resumed window count = %d, %v", count, err)
	}
}

func TestCollectionTaskLeaseRecoveryPreservesAttemptBudget(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	site := createRunnableSite(t, database, "run-b3a-lease", now)
	run := createB3AWindowRun(t, database, site, constant.TaskTypeUsageHour,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityUsageRealtime,
		now-3600, now, "req_b3a_lease", now)
	repository := NewCollectionTaskRepository(database.GORM)
	if _, err := repository.MaterializeRunWindows(ctx, run.ID, now, 1000); err != nil {
		t.Fatalf("materialize lease run: %v", err)
	}
	claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageHour}, Now: now,
		RequestID: "req_b3a_lease_first", MaxWindow: 24,
	})
	if err != nil || claim.Windows[0].AttemptCount != 1 {
		t.Fatalf("first lease claim = %#v, %v", claim, err)
	}
	policy := CollectionTaskAttemptPolicy{DefaultMaxAttempts: 2}
	cutoff := now - 1
	if recovered, err := repository.RecoverRunning(ctx, now+1, &cutoff, policy); err != nil || recovered != 0 {
		t.Fatalf("fresh lease recovery = %d, %v", recovered, err)
	}
	if recovered, err := repository.RecoverRunning(ctx, now+1, nil, policy); err != nil || recovered != 1 {
		t.Fatalf("startup takeover = %d, %v", recovered, err)
	}
	if recovered, err := repository.RecoverRunning(ctx, now+2, nil, policy); err != nil || recovered != 0 {
		t.Fatalf("duplicate takeover = %d, %v", recovered, err)
	}
	claim, err = repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageHour}, Now: now + 1,
		RequestID: "req_b3a_lease_second", MaxWindow: 24,
	})
	if err != nil || claim.Windows[0].AttemptCount != 2 {
		t.Fatalf("second lease claim = %#v, %v", claim, err)
	}
	staleCutoff := now + 2
	if recovered, err := repository.RecoverRunning(ctx, now+302, &staleCutoff, policy); err != nil || recovered != 1 {
		t.Fatalf("stale lease recovery = %d, %v", recovered, err)
	}
	loaded, err := NewSiteRepository(database.GORM).FindCollectionRunByID(ctx, run.ID)
	if err != nil || loaded.Status != CollectionTaskStatusFailed || loaded.ActiveKey != nil || loaded.RetryCount != 0 {
		t.Fatalf("terminal recovered run = %#v, %v", loaded, err)
	}
	var window CollectionRunWindow
	if err := database.GORM.Where("run_id = ?", run.ID).First(&window).Error; err != nil ||
		window.Status != CollectionTaskStatusFailed || window.AttemptCount != 2 || window.ErrorCode != CollectionTaskLeaseLostCode {
		t.Fatalf("terminal recovered window = %#v, %v", window, err)
	}
}

func TestCollectionTaskLeaseTokenRejectsOldWorkerAndCountsClaims(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	site := createRunnableSite(t, database, "run-b3a-token-cas", now)
	run, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_b3a_token_create", Now: now,
	})
	if err != nil {
		t.Fatalf("build token run: %v", err)
	}
	created, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &run)
	if err != nil {
		t.Fatalf("create token run: %v", err)
	}
	repository := NewCollectionTaskRepository(database.GORM)
	oldClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now,
		RequestID: "wrk_old_boot_1", MaxWindow: 24,
	})
	if err != nil || oldClaim.Run.ID != created.ID || oldClaim.Run.RetryCount != 1 {
		t.Fatalf("old token claim = %#v, %v", oldClaim.Run, err)
	}
	policy := CollectionTaskAttemptPolicy{DefaultMaxAttempts: 3}
	if recovered, err := repository.RecoverRunning(ctx, now+1, nil, policy); err != nil || recovered != 1 {
		t.Fatalf("recover old token lease = %d, %v", recovered, err)
	}
	afterRecovery, err := NewSiteRepository(database.GORM).FindCollectionRunByID(ctx, created.ID)
	if err != nil || afterRecovery.Status != CollectionTaskStatusPending || afterRecovery.RetryCount != 1 {
		t.Fatalf("recovered non-window attempt = %#v, %v", afterRecovery, err)
	}
	newClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now + 1,
		RequestID: "wrk_new_boot_1", MaxWindow: 24,
	})
	if err != nil || newClaim.Run.ID != created.ID || newClaim.Run.RetryCount != 2 {
		t.Fatalf("new token claim = %#v, %v", newClaim.Run, err)
	}
	if err := repository.Heartbeat(ctx, created.ID, oldClaim.RequestID, now+2); !errors.Is(err, ErrCollectionTaskClaimLost) {
		t.Fatalf("old token heartbeat error = %v", err)
	}
	if _, err := repository.CommitClaim(ctx, CollectionTaskCommitRequest{
		RunID: created.ID, RequestID: oldClaim.RequestID, Now: now + 2, RunStatus: CollectionTaskStatusSuccess,
	}); !errors.Is(err, ErrCollectionTaskClaimLost) {
		t.Fatalf("old token commit error = %v", err)
	}
	if _, err := repository.ReleaseClaim(ctx, oldClaim, now+2); !errors.Is(err, ErrCollectionTaskClaimLost) {
		t.Fatalf("old token release error = %v", err)
	}
	stillOwned, err := NewSiteRepository(database.GORM).FindCollectionRunByID(ctx, created.ID)
	if err != nil || stillOwned.Status != CollectionTaskStatusRunning || stillOwned.LastRequestID != newClaim.RequestID ||
		stillOwned.HeartbeatAt == nil || *stillOwned.HeartbeatAt != now+1 || stillOwned.RetryCount != 2 {
		t.Fatalf("new lease after old writes = %#v, %v", stillOwned, err)
	}
	if err := repository.Heartbeat(ctx, created.ID, newClaim.RequestID, now+3); err != nil {
		t.Fatalf("new token heartbeat: %v", err)
	}
	finished, err := repository.CommitClaim(ctx, CollectionTaskCommitRequest{
		RunID: created.ID, RequestID: newClaim.RequestID, Now: now + 4, RunStatus: CollectionTaskStatusSuccess,
	})
	if err != nil || finished.Status != CollectionTaskStatusSuccess || finished.RetryCount != 2 {
		t.Fatalf("new token commit = %#v, %v", finished, err)
	}
}

func TestCollectionTaskClaimScansPastBlockedHead(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	blockedSite := createRunnableSite(t, database, "run-b3a-blocked-head", now)
	runnableSite := createRunnableSite(t, database, "run-b3a-after-head", now)
	initialized := now
	blockedSiteID := blockedSite.ID
	runningKey := "b3a:blocked:running"
	running := CollectionRun{
		SiteID: &blockedSiteID, SiteConfigVersion: blockedSite.ConfigVersion,
		TaskType: constant.TaskTypeSiteProbe, TargetType: "site", TargetID: blockedSite.ID,
		TriggerType: constant.CollectionTriggerManual, Scope: []byte("{}"), ActiveKey: &runningKey,
		Status: CollectionTaskStatusRunning, RetryCount: 1, Priority: 0, NextAttemptAt: now,
		HeartbeatAt: &now, WindowsInitializedAt: &initialized,
		CreatedRequestID: "req_b3a_blocking_running", LastRequestID: "wrk_b3a_blocking_running",
		StartedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&running).Error; err != nil {
		t.Fatalf("create blocking run: %v", err)
	}
	blocked := make([]CollectionRun, 64)
	for index := range blocked {
		key := fmt.Sprintf("b3a:blocked:%d", index)
		requestID := fmt.Sprintf("req_b3a_blocked_%d", index)
		blocked[index] = CollectionRun{
			SiteID: &blockedSiteID, SiteConfigVersion: blockedSite.ConfigVersion,
			TaskType: constant.TaskTypeSiteProbe, TargetType: "site", TargetID: blockedSite.ID,
			TriggerType: constant.CollectionTriggerManual, Scope: []byte("{}"), ActiveKey: &key,
			Status: CollectionTaskStatusPending, Priority: 0, NextAttemptAt: now,
			WindowsInitializedAt: &initialized, CreatedRequestID: requestID, LastRequestID: requestID,
			CreatedAt: now + int64(index+1), UpdatedAt: now + int64(index+1),
		}
	}
	if err := database.GORM.CreateInBatches(&blocked, 64).Error; err != nil {
		t.Fatalf("create blocked head: %v", err)
	}
	runnable, err := NewSiteCollectionRun(runnableSite, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_b3a_after_blocked_head", Now: now + 100,
	})
	if err != nil {
		t.Fatalf("build runnable tail: %v", err)
	}
	runnableCreated, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &runnable)
	if err != nil {
		t.Fatalf("create runnable tail: %v", err)
	}
	repository := NewCollectionTaskRepository(database.GORM)
	claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now + 100,
		RequestID: "wrk_b3a_after_blocked_head", MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil || claim.Run.ID != runnableCreated.ID {
		t.Fatalf("claim after blocked head = %#v, %v", claim.Run, err)
	}
	if _, err := repository.ReleaseClaim(ctx, claim, now+101); err != nil {
		t.Fatalf("release runnable tail: %v", err)
	}
}

func TestCollectionTaskClaimKeysetDoesNotSkipWhenEarlierPageShrinks(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	blockedSite := createRunnableSite(t, database, "run-b3a-shrinking-page", now)
	runnableSite := createRunnableSite(t, database, "run-b3a-shrinking-tail", now)
	initialized := now
	blockedSiteID := blockedSite.ID
	runningKey := "b3a:shrinking:running"
	running := CollectionRun{
		SiteID: &blockedSiteID, SiteConfigVersion: blockedSite.ConfigVersion,
		TaskType: constant.TaskTypeSiteProbe, TargetType: "site", TargetID: blockedSite.ID,
		TriggerType: constant.CollectionTriggerManual, Scope: []byte("{}"), ActiveKey: &runningKey,
		Status: CollectionTaskStatusRunning, RetryCount: 1, Priority: 0, NextAttemptAt: now,
		HeartbeatAt: &now, WindowsInitializedAt: &initialized,
		CreatedRequestID: "req_b3a_shrinking_running", LastRequestID: "wrk_b3a_shrinking_running",
		StartedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&running).Error; err != nil {
		t.Fatalf("create shrinking-page blocker: %v", err)
	}
	blocked := make([]CollectionRun, 127)
	for index := range blocked {
		key := fmt.Sprintf("b3a:shrinking:%d", index)
		requestID := fmt.Sprintf("req_b3a_shrinking_%d", index)
		blocked[index] = CollectionRun{
			SiteID: &blockedSiteID, SiteConfigVersion: blockedSite.ConfigVersion,
			TaskType: constant.TaskTypeSiteProbe, TargetType: "site", TargetID: blockedSite.ID,
			TriggerType: constant.CollectionTriggerManual, Scope: []byte("{}"), ActiveKey: &key,
			Status: CollectionTaskStatusPending, Priority: 0, NextAttemptAt: now,
			WindowsInitializedAt: &initialized, CreatedRequestID: requestID, LastRequestID: requestID,
			CreatedAt: now + int64(index+1), UpdatedAt: now + int64(index+1),
		}
	}
	if err := database.GORM.CreateInBatches(&blocked, 127).Error; err != nil {
		t.Fatalf("create shrinking-page candidates: %v", err)
	}
	runnable, err := NewSiteCollectionRun(runnableSite, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_b3a_shrinking_tail", Now: now + 128,
	})
	if err != nil {
		t.Fatalf("build shrinking-page tail: %v", err)
	}
	runnableCreated, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &runnable)
	if err != nil {
		t.Fatalf("create shrinking-page tail: %v", err)
	}

	firstPageRead := make(chan struct{})
	resumeScan := make(chan struct{})
	var barrier sync.Once
	const callbackName = "test:b3a_claim_shrinking_page"
	if err := database.GORM.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		candidates, ok := tx.Statement.Dest.(*[]CollectionRun)
		if !ok || len(*candidates) != 64 {
			return
		}
		barrier.Do(func() {
			close(firstPageRead)
			<-resumeScan
		})
	}); err != nil {
		t.Fatalf("register shrinking-page barrier: %v", err)
	}
	t.Cleanup(func() {
		_ = database.GORM.Callback().Query().Remove(callbackName)
	})

	type claimResult struct {
		claim CollectionTaskClaim
		err   error
	}
	result := make(chan claimResult, 1)
	go func() {
		claim, claimErr := NewCollectionTaskRepository(database.GORM).ClaimNext(ctx, CollectionTaskClaimOptions{
			TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now + 128,
			RequestID: "wrk_b3a_shrinking_tail", MaxWindow: 24, ScanLimit: 64,
		})
		result <- claimResult{claim: claim, err: claimErr}
	}()
	select {
	case <-firstPageRead:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for first claim page")
	}
	firstPageIDs := make([]int64, 64)
	for index := range firstPageIDs {
		firstPageIDs[index] = blocked[index].ID
	}
	update := database.GORM.Model(&CollectionRun{}).Where("id IN ? AND status = 'pending'", firstPageIDs).Updates(map[string]any{
		"status": CollectionTaskStatusSuccess, "active_key": nil, "finished_at": now + 128, "updated_at": now + 128,
	})
	if update.Error != nil || update.RowsAffected != 64 {
		close(resumeScan)
		t.Fatalf("shrink first claim page: rows=%d err=%v", update.RowsAffected, update.Error)
	}
	close(resumeScan)
	select {
	case got := <-result:
		if got.err != nil || got.claim.Run.ID != runnableCreated.ID {
			t.Fatalf("claim after concurrent page shrink = %#v, %v", got.claim.Run, got.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out claiming after first page shrank")
	}
}

func TestCollectionTaskWindowOrderingNewestFirst(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	repository := NewCollectionTaskRepository(database.GORM)
	olderSite := createRunnableSite(t, database, "run-b3a-order-older", now)
	newerSite := createRunnableSite(t, database, "run-b3a-order-newer", now)
	older := createB3AWindowRun(t, database, olderSite, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill,
		now-6*3600, now-3*3600, "req_b3a_order_older", now)
	newer := createB3AWindowRun(t, database, newerSite, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill,
		now-3*3600, now, "req_b3a_order_newer", now+1)
	for _, run := range []CollectionRun{older, newer} {
		if _, err := repository.MaterializeRunWindows(ctx, run.ID, now+2, 1000); err != nil {
			t.Fatalf("materialize ordered run %d: %v", run.ID, err)
		}
	}
	newerClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now + 2,
		RequestID: "wrk_b3a_order_newer", MaxWindow: 24,
	})
	if err != nil || newerClaim.Run.ID != newer.ID || len(newerClaim.Windows) != 3 ||
		newerClaim.Windows[0].HourTS != now-3600 || newerClaim.Windows[2].HourTS != now-3*3600 {
		t.Fatalf("newest-first claim = run:%d windows:%#v err:%v", newerClaim.Run.ID, newerClaim.Windows, err)
	}
	commitSuccessfulWindows(t, repository, newerClaim, now+3)
	olderClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now + 3,
		RequestID: "wrk_b3a_order_older", MaxWindow: 24,
	})
	if err != nil || olderClaim.Run.ID != older.ID {
		t.Fatalf("older same-priority claim = %#v, %v", olderClaim.Run, err)
	}
	commitSuccessfulWindows(t, repository, olderClaim, now+4)

	initialSite := createRunnableSite(t, database, "run-b3a-order-initial", now)
	initialSite.StatisticsStatus = constant.SiteStatisticsBackfilling
	if err := database.GORM.Save(&initialSite).Error; err != nil {
		t.Fatalf("mark initial site backfilling: %v", err)
	}
	initial := createB3AWindowRun(t, database, initialSite, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerRecovery, constant.CollectionPriorityInitialBackfill,
		now-3*3600, now, "req_b3a_order_initial", now+5)
	if _, err := repository.MaterializeRunWindows(ctx, initial.ID, now+5, 1000); err != nil {
		t.Fatalf("materialize initial run: %v", err)
	}
	initialClaim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now + 5,
		RequestID: "wrk_b3a_order_initial", MaxWindow: 24,
	})
	if err != nil || len(initialClaim.Windows) != 3 || initialClaim.Windows[0].HourTS != now-3600 ||
		initialClaim.Windows[2].HourTS != now-3*3600 {
		t.Fatalf("initial newest-first claim = %#v, %v", initialClaim.Windows, err)
	}
	commitSuccessfulWindows(t, repository, initialClaim, now+6)
	if err := database.GORM.First(&initialSite, initialSite.ID).Error; err != nil ||
		initialSite.StatisticsStatus != constant.SiteStatisticsReady {
		t.Fatalf("initial site terminal status = %q, %v", initialSite.StatisticsStatus, err)
	}
}

func commitSuccessfulWindows(
	t *testing.T,
	repository *CollectionTaskRepository,
	claim CollectionTaskClaim,
	now int64,
) {
	t.Helper()
	outcomes := make([]CollectionTaskWindowResult, len(claim.Windows))
	for index, window := range claim.Windows {
		outcomes[index] = CollectionTaskWindowResult{
			WindowID: window.ID, AttemptCount: window.AttemptCount, Status: CollectionTaskStatusSuccess,
		}
	}
	if _, err := repository.CommitClaim(context.Background(), CollectionTaskCommitRequest{
		RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now, Windows: outcomes,
	}); err != nil {
		t.Fatalf("commit successful windows for run %d: %v", claim.Run.ID, err)
	}
}

func createB3AWindowRun(
	t *testing.T,
	database *Database,
	site Site,
	taskType, triggerType string,
	priority int,
	start, end int64,
	requestID string,
	now int64,
) CollectionRun {
	t.Helper()
	scope := []byte("{}")
	if taskType == constant.TaskTypeUsageBackfill {
		var err error
		scope, err = NewUsageBackfillRunScope(false)
		if err != nil {
			t.Fatalf("build backfill scope: %v", err)
		}
	}
	run, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: taskType, TriggerType: triggerType, StartTimestamp: &start, EndTimestamp: &end,
		Scope: scope, Priority: priority, RequestID: requestID, Now: now,
	})
	if err != nil {
		t.Fatalf("build B3a run: %v", err)
	}
	created, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(context.Background(), &run)
	if err != nil {
		t.Fatalf("create B3a run: %v", err)
	}
	return created
}

func TestCollectionTaskGracefulReleaseLeavesPending(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := int64(1_752_400_800)
	site := createRunnableSite(t, database, "run-b3a-release", now)
	run, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_b3a_release", Now: now,
	})
	if err != nil {
		t.Fatalf("build release run: %v", err)
	}
	created, _, err := NewSiteRepository(database.GORM).CreateOrGetRun(ctx, &run)
	if err != nil {
		t.Fatalf("create release run: %v", err)
	}
	repository := NewCollectionTaskRepository(database.GORM)
	claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now,
		RequestID: "req_b3a_release_claim", MaxWindow: 24,
	})
	if err != nil || claim.Run.ID != created.ID {
		t.Fatalf("claim release run = %#v, %v", claim.Run, err)
	}
	released, err := repository.ReleaseClaim(ctx, claim, now+1)
	if err != nil || released.Status != CollectionTaskStatusPending || released.HeartbeatAt != nil ||
		released.ActiveKey == nil || released.RetryCount != 1 {
		t.Fatalf("released run = %#v, %v", released, err)
	}
}

func TestReleaseOwnedRunningHandlesPartialWindowCommit(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	site := createRunnableSite(t, database, "run-owned-partial-release", now)
	run := createB3AWindowRun(t, database, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill,
		now-2*3600, now, "req_owned_partial", now)
	repository := NewCollectionTaskRepository(database.GORM)
	if _, err := repository.MaterializeRunWindows(ctx, run.ID, now, 1000); err != nil {
		t.Fatalf("materialize partial release run: %v", err)
	}
	claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now,
		RequestID: "wrk_owned_partial", MaxWindow: 24,
	})
	if err != nil || len(claim.Windows) != 2 {
		t.Fatalf("claim partial release run = windows:%d err:%v", len(claim.Windows), err)
	}
	if _, err := repository.CompleteClaimedWindow(ctx, CompleteClaimedWindowRequest{
		RunID: run.ID, RequestID: claim.RequestID, Now: now + 1,
		Window: CollectionTaskWindowResult{
			WindowID: claim.Windows[0].ID, AttemptCount: claim.Windows[0].AttemptCount,
			Status: CollectionTaskStatusSuccess,
		},
	}); err != nil {
		t.Fatalf("complete first window: %v", err)
	}
	released, err := repository.ReleaseOwnedRunning(ctx, run.ID, claim.RequestID, now+2)
	if err != nil || released != 1 {
		t.Fatalf("release owned windows = %d, %v", released, err)
	}
	loaded, err := NewSiteRepository(database.GORM).FindCollectionRunByID(ctx, run.ID)
	if err != nil || loaded.Status != CollectionTaskStatusPending || loaded.HeartbeatAt != nil || loaded.CompletedWindows != 1 {
		t.Fatalf("released parent = %#v, %v", loaded, err)
	}
	var windows []CollectionRunWindow
	if err := database.GORM.Where("run_id = ?", run.ID).Order("hour_ts ASC").Find(&windows).Error; err != nil {
		t.Fatalf("load released windows: %v", err)
	}
	statuses := map[string]int{}
	for _, window := range windows {
		statuses[window.Status]++
		if window.Status == CollectionTaskStatusPending && (window.NextRetryAt == nil || *window.NextRetryAt != now+2) {
			t.Fatalf("pending released window = %#v", window)
		}
	}
	if statuses[CollectionTaskStatusSuccess] != 1 || statuses[CollectionTaskStatusPending] != 1 {
		t.Fatalf("released window statuses = %#v", statuses)
	}
}
