package model

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

func TestSiteCollectionRunCanonicalConstruction(t *testing.T) {
	now := int64(1_752_400_800)
	site := Site{ID: 7, ConfigVersion: 3}
	nonWindow, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_probe", Now: now,
	})
	if err != nil {
		t.Fatalf("build non-window run: %v", err)
	}
	if nonWindow.WindowsInitializedAt == nil || *nonWindow.WindowsInitializedAt != now ||
		nonWindow.ActiveKey == nil || *nonWindow.ActiveKey != "site_probe:site:7::" {
		t.Fatalf("non-window run = %#v", nonWindow)
	}
	start, end := int64(3600), int64(7200)
	windowed, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerManual,
		StartTimestamp: &start, EndTimestamp: &end, Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_backfill", Now: now,
	})
	if err != nil {
		t.Fatalf("build window run: %v", err)
	}
	if windowed.WindowsInitializedAt != nil || windowed.ActiveKey == nil ||
		*windowed.ActiveKey != "usage_backfill:site:7:3600:7200" {
		t.Fatalf("window run = %#v", windowed)
	}
	if _, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerRecovery,
		StartTimestamp: &start, EndTimestamp: &end, Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_wrong_priority", Now: now,
	}); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("wrong recovery priority error = %v", err)
	}
	canonical, err := CanonicalCollectionRunScope(constant.TaskTypeUsageBackfill, []byte(`{ "only_missing" : false }`))
	if err != nil || !bytes.Equal(canonical, []byte(`{"only_missing":false}`)) {
		t.Fatalf("canonical backfill scope = %s, %v", canonical, err)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"only_missing":true,"access_token":"secret"}`),
		[]byte(`{"only_missing":true,"only_missing":false}`),
		[]byte(`{"password":"secret"}`),
	} {
		if _, err := CanonicalCollectionRunScope(constant.TaskTypeUsageBackfill, invalid); !errors.Is(err, ErrCollectionRunContract) {
			t.Fatalf("invalid scope %s error = %v", invalid, err)
		}
	}
}

func TestCreateSiteWindowRunConcurrentDedupeOverlapAndScheduleGaps(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_400_800)
	start := now - now%3600 - 4*3600

	dedupeSite := createRunnableSite(t, database, "run-dedupe", now)
	trueScope, err := NewUsageBackfillRunScope(true)
	if err != nil {
		t.Fatalf("build true scope: %v", err)
	}
	falseScope, err := NewUsageBackfillRunScope(false)
	if err != nil {
		t.Fatalf("build false scope: %v", err)
	}
	requests := []SiteWindowRunCreateRequest{{
		SiteID: dedupeSite.ID, ExpectedConfigVersion: dedupeSite.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerManual,
		StartTimestamp: start, EndTimestamp: start + 2*3600, Scope: trueScope, Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_concurrent_dedupe", Now: now, Mode: SiteWindowRunStrict,
	}, {
		SiteID: dedupeSite.ID, ExpectedConfigVersion: dedupeSite.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerManual,
		StartTimestamp: start, EndTimestamp: start + 2*3600, Scope: falseScope, Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_concurrent_scope", Now: now, Mode: SiteWindowRunStrict,
	}}
	results := make([]SiteWindowRunCreateResult, 2)
	errs := make([]error, 2)
	ready := make(chan struct{})
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-ready
			results[index], errs[index] = NewSiteRepository(database.GORM).CreateSiteWindowRun(ctx, requests[index])
		}(index)
	}
	close(ready)
	wait.Wait()
	createdCount, deduplicatedCount := 0, 0
	var runID int64
	for index := range results {
		if errs[index] != nil || len(results[index].Runs) != 1 {
			t.Fatalf("concurrent result %d = %#v, %v", index, results[index], errs[index])
		}
		item := results[index].Runs[0]
		if runID == 0 {
			runID = item.Run.ID
		} else if item.Run.ID != runID {
			t.Fatalf("concurrent dedupe IDs differ: %d and %d", runID, item.Run.ID)
		}
		if item.Deduplicated {
			deduplicatedCount++
		} else {
			createdCount++
		}
	}
	if createdCount != 1 || deduplicatedCount != 1 {
		t.Fatalf("concurrent flags created=%d deduplicated=%d", createdCount, deduplicatedCount)
	}
	firstScope, err := UsageBackfillOnlyMissing(results[0].Runs[0].Run.Scope)
	if err != nil {
		t.Fatalf("decode first concurrent scope: %v", err)
	}
	secondScope, err := UsageBackfillOnlyMissing(results[1].Runs[0].Run.Scope)
	if err != nil || firstScope != secondScope {
		t.Fatalf("deduplicated scopes differ first=%t second=%t err=%v", firstScope, secondScope, err)
	}

	overlapSite := createRunnableSite(t, database, "run-overlap", now)
	overlapRequests := []SiteWindowRunCreateRequest{
		{
			SiteID: overlapSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
			TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 2*3600,
			Priority: constant.CollectionPriorityManualBackfill, RequestID: "req_overlap_one", Now: now, Mode: SiteWindowRunStrict,
		},
		{
			SiteID: overlapSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
			TriggerType: constant.CollectionTriggerManual, StartTimestamp: start + 3600, EndTimestamp: start + 3*3600,
			Priority: constant.CollectionPriorityManualBackfill, RequestID: "req_overlap_two", Now: now, Mode: SiteWindowRunStrict,
		},
	}
	overlapErrors := make([]error, 2)
	ready = make(chan struct{})
	wait = sync.WaitGroup{}
	for index := range overlapRequests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-ready
			_, overlapErrors[index] = NewSiteRepository(database.GORM).CreateSiteWindowRun(ctx, overlapRequests[index])
		}(index)
	}
	close(ready)
	wait.Wait()
	overlapCount, successCount := 0, 0
	for _, err := range overlapErrors {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrSiteWindowRunOverlap):
			overlapCount++
		default:
			t.Fatalf("unexpected overlap race error: %v", err)
		}
	}
	if overlapCount != 1 || successCount != 1 {
		t.Fatalf("overlap race success=%d overlap=%d", successCount, overlapCount)
	}

	scheduleSite := createRunnableSite(t, database, "run-schedule", now)
	middle, err := NewSiteRepository(database.GORM).CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: scheduleSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start + 3600, EndTimestamp: start + 2*3600,
		Priority: constant.CollectionPriorityManualBackfill, RequestID: "req_middle", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil || len(middle.Runs) != 1 {
		t.Fatalf("create middle coverage = %#v, %v", middle, err)
	}
	gaps, err := NewSiteRepository(database.GORM).CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: scheduleSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageHour,
		TriggerType: constant.CollectionTriggerSchedule, StartTimestamp: start, EndTimestamp: start + 3*3600,
		Priority: constant.CollectionPriorityUsageRealtime, RequestID: "req_schedule_gaps", Now: now, Mode: SiteWindowRunSchedule,
	})
	if err != nil || len(gaps.Runs) != 2 {
		t.Fatalf("schedule gaps = %#v, %v", gaps, err)
	}
	if *gaps.Runs[0].Run.StartTimestamp != start || *gaps.Runs[0].Run.EndTimestamp != start+3600 ||
		*gaps.Runs[1].Run.StartTimestamp != start+2*3600 || *gaps.Runs[1].Run.EndTimestamp != start+3*3600 {
		t.Fatalf("unexpected schedule gap ranges: %#v", gaps)
	}
	covered, err := NewSiteRepository(database.GORM).CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: scheduleSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageHour,
		TriggerType: constant.CollectionTriggerSchedule, StartTimestamp: start, EndTimestamp: start + 3*3600,
		Priority: constant.CollectionPriorityUsageRealtime, RequestID: "req_schedule_covered", Now: now, Mode: SiteWindowRunSchedule,
	})
	if err != nil || len(covered.Runs) != 0 {
		t.Fatalf("fully covered schedule = %#v, %v", covered, err)
	}
}

func TestMaterializationUnavailableProgressAndFencedParentImmutability(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	repository := NewSiteRepository(database.GORM)
	now := int64(1_752_400_800)
	start := now - now%3600 - 4*3600
	site := createRunnableSite(t, database, "run-terminal", now)
	created, err := repository.CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: site.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 3*3600,
		Scope:    []byte(`{"only_missing":false}`),
		Priority: constant.CollectionPriorityManualBackfill, RequestID: "req_terminal", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil || len(created.Runs) != 1 || created.Runs[0].Run.WindowsInitializedAt != nil {
		t.Fatalf("create terminal test run = %#v, %v", created, err)
	}
	run := created.Runs[0].Run
	reloadedBeforeMaterialization, err := repository.FindCollectionRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("reload run before materialization: %v", err)
	}
	onlyMissing, err := UsageBackfillOnlyMissing(reloadedBeforeMaterialization.Scope)
	if err != nil || onlyMissing {
		t.Fatalf("restart scope only_missing=%t error=%v raw=%s", onlyMissing, err, reloadedBeforeMaterialization.Scope)
	}
	for offset := int64(0); offset < 3; offset++ {
		window := CollectionRunWindow{RunID: run.ID, SiteID: site.ID, HourTS: start + offset*3600, Status: "pending", UpdatedAt: now}
		if err := database.GORM.Create(&window).Error; err != nil {
			t.Fatalf("create window %d: %v", offset, err)
		}
	}
	expectation, err := NewSiteRunWindowMaterializationExpectation(run, []int64{start, start + 3600, start + 2*3600})
	if err != nil {
		t.Fatalf("build materialization expectation: %v", err)
	}
	materialized, err := repository.CompleteSiteRunWindowMaterialization(ctx, site.ID, run.ID, 1, expectation, now)
	if err != nil || materialized.WindowsInitializedAt == nil || materialized.Status != "pending" {
		t.Fatalf("complete materialization = %#v, %v", materialized, err)
	}
	statuses := []string{"success", "unavailable", "success"}
	for offset, status := range statuses {
		if err := database.GORM.Model(&CollectionRunWindow{}).
			Where("run_id = ? AND hour_ts = ?", run.ID, start+int64(offset)*3600).
			Updates(map[string]any{"status": status, "finished_at": now, "updated_at": now}).Error; err != nil {
			t.Fatalf("finish window %d: %v", offset, err)
		}
	}
	terminal, err := repository.RecalculateSiteCollectionRun(ctx, site.ID, run.ID, now+1)
	if err != nil || terminal.Status != "success" || terminal.CompletedWindows != 2 ||
		terminal.FailedWindows != 0 || terminal.UnavailableWindows != 1 || terminal.ActiveKey != nil {
		t.Fatalf("terminal unavailable run = %#v, %v", terminal, err)
	}
	hydrated, err := repository.FindCollectionRunByID(ctx, run.ID)
	if err != nil || hydrated.UnavailableWindows != 1 {
		t.Fatalf("hydrated unavailable count = %#v, %v", hydrated, err)
	}

	fencedSite := createRunnableSite(t, database, "run-fenced", now)
	fencedCreate, err := repository.CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: fencedSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 3600,
		Priority: constant.CollectionPriorityManualBackfill, RequestID: "req_fenced", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil {
		t.Fatalf("create fenced run: %v", err)
	}
	fencedRun := fencedCreate.Runs[0].Run
	window := CollectionRunWindow{RunID: fencedRun.ID, SiteID: fencedSite.ID, HourTS: start, Status: "pending", UpdatedAt: now}
	if err := database.GORM.Create(&window).Error; err != nil {
		t.Fatalf("create fenced window: %v", err)
	}
	fencedExpectation, err := NewSiteRunWindowMaterializationExpectation(fencedRun, []int64{start})
	if err != nil {
		t.Fatalf("build fenced materialization expectation: %v", err)
	}
	if _, err := repository.CompleteSiteRunWindowMaterialization(ctx, fencedSite.ID, fencedRun.ID, 1, fencedExpectation, now); err != nil {
		t.Fatalf("materialize fenced run: %v", err)
	}
	if err := database.GORM.Model(&CollectionRunWindow{}).Where("id = ?", window.ID).
		Updates(map[string]any{"status": "success", "finished_at": now, "updated_at": now}).Error; err != nil {
		t.Fatalf("finish child before fence: %v", err)
	}
	lockedSite, err := repository.FindByID(ctx, fencedSite.ID)
	if err != nil {
		t.Fatalf("load fence site: %v", err)
	}
	if err := repository.WithTransaction(ctx, func(txRepository *SiteRepository) error {
		current, err := txRepository.FindByIDForUpdate(ctx, lockedSite.ID)
		if err != nil {
			return err
		}
		return txRepository.BumpSiteFence(ctx, &current, now+1)
	}); err != nil {
		t.Fatalf("bump fence: %v", err)
	}
	if _, err := repository.RecalculateSiteCollectionRun(ctx, fencedSite.ID, fencedRun.ID, now+2); err != nil {
		t.Fatalf("stale recalculate fenced parent: %v", err)
	}
	fencedParent, err := repository.FindCollectionRunByID(ctx, fencedRun.ID)
	if err != nil || fencedParent.Status != "failed" || fencedParent.ActiveKey != nil ||
		fencedParent.ErrorCode != constant.CodeSiteConfigChanged {
		t.Fatalf("fenced parent was revived: %#v, %v", fencedParent, err)
	}
}

func TestMaterializationValidatesWindowSetAndFrozenMissingScope(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	repository := NewSiteRepository(database.GORM)
	now := int64(1_752_400_800)
	start := now - now%3600 - 6*3600

	fullSite := createRunnableSite(t, database, "run-materialization-full", now)
	fullCreate, err := repository.CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: fullSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 4*3600,
		Scope: []byte(`{"only_missing":false}`), Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_materialization_full", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil {
		t.Fatalf("create full materialization run: %v", err)
	}
	fullRun := fullCreate.Runs[0].Run
	if _, err := NewSiteRunWindowMaterializationExpectation(fullRun, []int64{start + 1}); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("unaligned expectation error = %v", err)
	}
	if _, err := NewSiteRunWindowMaterializationExpectation(fullRun, []int64{start, start}); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("duplicate expectation error = %v", err)
	}
	if _, err := NewSiteRunWindowMaterializationExpectation(fullRun, []int64{start + 4*3600}); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("out-of-range expectation error = %v", err)
	}
	wrongHours := []int64{start, start + 3600, start + 3*3600}
	for _, hour := range wrongHours {
		if err := database.GORM.Create(&CollectionRunWindow{
			RunID: fullRun.ID, SiteID: fullSite.ID, HourTS: hour, Status: "pending", UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("create wrong-set window %d: %v", hour, err)
		}
	}
	wrongExpectation, err := NewSiteRunWindowMaterializationExpectation(fullRun, wrongHours)
	if err != nil {
		t.Fatalf("build wrong-set expectation: %v", err)
	}
	if _, err := repository.CompleteSiteRunWindowMaterialization(ctx, fullSite.ID, fullRun.ID, 1, wrongExpectation, now); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("wrong full-set materialization error = %v", err)
	}

	sparseSite := createRunnableSite(t, database, "run-materialization-sparse", now)
	sparseCreate, err := repository.CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: sparseSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 4*3600,
		Scope: []byte(`{"only_missing":true}`), Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_materialization_sparse", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil {
		t.Fatalf("create sparse materialization run: %v", err)
	}
	sparseRun := sparseCreate.Runs[0].Run
	for offset, status := range []string{"complete", "missing", "unavailable"} {
		if err := database.GORM.Exec(`INSERT INTO collection_window (site_id, hour_ts, status, updated_at)
VALUES (?, ?, ?, ?)`, sparseSite.ID, start+int64(offset)*3600, status, now).Error; err != nil {
			t.Fatalf("create collection fact window %d: %v", offset, err)
		}
	}
	sparseHours := []int64{start + 3600, start + 3*3600}
	for _, hour := range sparseHours {
		if err := database.GORM.Create(&CollectionRunWindow{
			RunID: sparseRun.ID, SiteID: sparseSite.ID, HourTS: hour, Status: "pending", UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("create sparse run window %d: %v", hour, err)
		}
	}
	sparseExpectation, err := NewSiteRunWindowMaterializationExpectation(sparseRun, sparseHours)
	if err != nil {
		t.Fatalf("build sparse expectation: %v", err)
	}
	materialized, err := repository.CompleteSiteRunWindowMaterialization(
		ctx, sparseSite.ID, sparseRun.ID, 1, sparseExpectation, now,
	)
	if err != nil || materialized.TotalWindows != 2 || materialized.WindowsInitializedAt == nil {
		t.Fatalf("sparse materialization = %#v, %v", materialized, err)
	}

	staleSite := createRunnableSite(t, database, "run-materialization-stale", now)
	staleCreate, err := repository.CreateSiteWindowRun(ctx, SiteWindowRunCreateRequest{
		SiteID: staleSite.ID, ExpectedConfigVersion: 1, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: start, EndTimestamp: start + 2*3600,
		Scope: []byte(`{"only_missing":true}`), Priority: constant.CollectionPriorityManualBackfill,
		RequestID: "req_materialization_stale", Now: now, Mode: SiteWindowRunStrict,
	})
	if err != nil {
		t.Fatalf("create stale expectation run: %v", err)
	}
	staleRun := staleCreate.Runs[0].Run
	staleHours := []int64{start, start + 3600}
	for _, hour := range staleHours {
		if err := database.GORM.Create(&CollectionRunWindow{
			RunID: staleRun.ID, SiteID: staleSite.ID, HourTS: hour, Status: "pending", UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("create stale run window %d: %v", hour, err)
		}
	}
	staleExpectation, err := NewSiteRunWindowMaterializationExpectation(staleRun, staleHours)
	if err != nil {
		t.Fatalf("build stale expectation: %v", err)
	}
	if err := database.GORM.Exec(`INSERT INTO collection_window (site_id, hour_ts, status, updated_at)
VALUES (?, ?, 'complete', ?)`, staleSite.ID, start, now).Error; err != nil {
		t.Fatalf("change missing snapshot before CAS: %v", err)
	}
	if _, err := repository.CompleteSiteRunWindowMaterialization(
		ctx, staleSite.ID, staleRun.ID, 1, staleExpectation, now,
	); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("stale expectation CAS error = %v", err)
	}
	staleExpectation.SetHash = strings.Repeat("0", 64)
	if _, err := repository.CompleteSiteRunWindowMaterialization(
		ctx, staleSite.ID, staleRun.ID, 1, staleExpectation, now,
	); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("tampered expectation hash error = %v", err)
	}
}

func TestBumpSiteFenceRecalculatesAccountAndCustomerBackfillStatus(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	repository := NewSiteRepository(database.GORM)
	now := int64(1_752_400_800)
	start := now - now%3600 - 3600
	site := createRunnableSite(t, database, "run-fence-objects", now)
	otherSite := createRunnableSite(t, database, "run-fence-objects-other", now)

	failedCustomer := createFenceCustomer(t, database, "Run Customer Failed", "running", now)
	failedAccount := createFenceAccount(t, database, site.ID, failedCustomer, 101, "running", now)
	failedRun := createFenceLocalRun(t, database, "account", failedAccount, constant.TaskTypeAccountRebuild, site.ID, start, "running", now)

	runningCustomer := createFenceCustomer(t, database, "Run Customer Running", "running", now)
	localCustomerAccount := createFenceAccount(t, database, site.ID, runningCustomer, 102, "none", now)
	otherAccount := createFenceAccount(t, database, otherSite.ID, runningCustomer, 103, "running", now)
	failedCustomerRun := createFenceLocalRun(t, database, "customer", runningCustomer, constant.TaskTypeCustomerRebuild, site.ID, start, "pending", now)
	otherRun := createFenceLocalRun(t, database, "account", otherAccount, constant.TaskTypeAccountRebuild, otherSite.ID, start, "running", now)
	plan, err := repository.buildSiteFencePlan(ctx, site.ID)
	if err != nil {
		t.Fatalf("build object fence plan: %v", err)
	}
	plannedAccounts := make(map[int64]struct{}, len(plan.AccountRefs))
	for _, account := range plan.AccountRefs {
		plannedAccounts[account.ID] = struct{}{}
	}
	if len(plannedAccounts) != 2 {
		t.Fatalf("planned account count = %d, want 2: %#v", len(plannedAccounts), plan.AccountRefs)
	}
	for _, accountID := range []int64{failedAccount, localCustomerAccount} {
		if _, found := plannedAccounts[accountID]; !found {
			t.Fatalf("required local account %d is absent from fence plan: %#v", accountID, plan.AccountRefs)
		}
	}
	if _, found := plannedAccounts[otherAccount]; found {
		t.Fatalf("other-site customer account %d was expanded into fence plan: %#v", otherAccount, plan.AccountRefs)
	}

	err = repository.WithTransaction(ctx, func(transaction *SiteRepository) error {
		current, err := transaction.FindByIDForUpdate(ctx, site.ID)
		if err != nil {
			return err
		}
		return transaction.BumpSiteFence(ctx, &current, now+1)
	})
	if err != nil {
		t.Fatalf("bump object fence: %v", err)
	}
	for _, runID := range []int64{failedRun.ID, failedCustomerRun.ID} {
		var run CollectionRun
		if err := database.GORM.First(&run, runID).Error; err != nil || run.Status != "failed" ||
			run.ActiveKey != nil || run.ErrorCode != constant.CodeSiteConfigChanged {
			t.Fatalf("fenced object run %d = %#v, %v", runID, run, err)
		}
		var window CollectionRunWindow
		if err := database.GORM.Where("run_id = ?", runID).First(&window).Error; err != nil || window.Status != "failed" {
			t.Fatalf("fenced object window %d = %#v, %v", runID, window, err)
		}
	}
	var accountStatus, failedCustomerStatus, runningCustomerStatus string
	if err := database.SQL.QueryRowContext(ctx, "SELECT statistics_backfill_status FROM account WHERE id = ?", failedAccount).Scan(&accountStatus); err != nil {
		t.Fatalf("read failed account status: %v", err)
	}
	if err := database.SQL.QueryRowContext(ctx, "SELECT statistics_backfill_status FROM customer WHERE id = ?", failedCustomer).Scan(&failedCustomerStatus); err != nil {
		t.Fatalf("read failed customer status: %v", err)
	}
	if err := database.SQL.QueryRowContext(ctx, "SELECT statistics_backfill_status FROM customer WHERE id = ?", runningCustomer).Scan(&runningCustomerStatus); err != nil {
		t.Fatalf("read running customer status: %v", err)
	}
	if accountStatus != "failed" || failedCustomerStatus != "failed" || runningCustomerStatus != "running" {
		t.Fatalf("recalculated statuses account=%s failed_customer=%s running_customer=%s",
			accountStatus, failedCustomerStatus, runningCustomerStatus)
	}
	var untouched CollectionRun
	if err := database.GORM.First(&untouched, otherRun.ID).Error; err != nil || untouched.Status != "running" || untouched.ActiveKey == nil {
		t.Fatalf("other-site account run was changed: %#v, %v", untouched, err)
	}
}

func TestBumpSiteFenceWithTenThousandUnrelatedAccountsUsesConstantWorkAndNoObjectLocks(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	repository := NewSiteRepository(database.GORM)
	now := int64(1_752_400_800)
	site := createRunnableSite(t, database, "run-fence-scale", now)
	customerID := createFenceCustomer(t, database, "Run Customer Scale", "none", now)
	accounts := make([]Account, 10_000)
	for index := range accounts {
		remoteUserID := int64(10_000 + index)
		accounts[index] = Account{
			SiteID: site.ID, CustomerID: customerID, RemoteUserID: remoteUserID,
			RemoteCreatedAt: now - 3600, Username: fmt.Sprintf("scale-user-%d", remoteUserID),
			RemoteStatus: 1, RemoteState: AccountRemoteStateNormal, ManagedStatus: AccountManagedStatusActive,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := database.GORM.CreateInBatches(&accounts, 500).Error; err != nil {
		t.Fatalf("create scale accounts: %v", err)
	}
	directRun, err := NewSiteCollectionRun(site, SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_fence_scale", Now: now,
	})
	if err != nil {
		t.Fatalf("build scale site run: %v", err)
	}
	createdRun, _, err := repository.CreateOrGetRun(ctx, &directRun)
	if err != nil {
		t.Fatalf("create scale site run: %v", err)
	}
	plan, err := repository.buildSiteFencePlan(ctx, site.ID)
	if err != nil {
		t.Fatalf("build scale fence plan: %v", err)
	}
	if len(plan.Runs) != 1 || plan.Runs[0].ID != createdRun.ID {
		t.Fatalf("scale plan runs = %#v, want direct run %d", plan.Runs, createdRun.ID)
	}
	if len(plan.AccountRefs) != 0 || len(plan.CustomerIDs) != 0 {
		t.Fatalf("scale plan expanded unrelated objects: accounts=%d customers=%d", len(plan.AccountRefs), len(plan.CustomerIDs))
	}

	accountLock := database.GORM.Begin()
	if accountLock.Error != nil {
		t.Fatalf("begin unrelated account lock: %v", accountLock.Error)
	}
	defer accountLock.Rollback()
	var lockedAccountID int64
	if err := accountLock.Raw("SELECT id FROM account WHERE id = ? FOR UPDATE", accounts[len(accounts)/2].ID).
		Scan(&lockedAccountID).Error; err != nil || lockedAccountID <= 0 {
		t.Fatalf("lock unrelated account = %d, %v", lockedAccountID, err)
	}

	countedDB, sqlCounter := newTestSQLCountingDB(database.GORM)
	countedRepository := NewSiteRepository(countedDB)

	bumpContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	startedAt := time.Now()
	err = countedRepository.WithTransaction(bumpContext, func(transaction *SiteRepository) error {
		current, findErr := transaction.FindByID(bumpContext, site.ID)
		if findErr != nil {
			return findErr
		}
		return transaction.BumpSiteFence(bumpContext, &current, now+1)
	})
	elapsed := time.Since(startedAt)
	if err != nil {
		t.Fatalf("bump scale fence while unrelated account is locked after %s: %v", elapsed, err)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("scale fence took %s while an unrelated account was locked", elapsed)
	}
	if got := sqlCounter.countStatementsContaining("for update", "from `account`"); got != 0 {
		t.Fatalf("scale fence account FOR UPDATE statements = %d, want 0", got)
	}
	if got := sqlCounter.countStatementsContaining("for update", "from `customer`"); got != 0 {
		t.Fatalf("scale fence customer FOR UPDATE statements = %d, want 0", got)
	}
	if got := sqlCounter.countStatementsContaining("for update", "from `site`"); got != 1 {
		t.Fatalf("scale fence site FOR UPDATE statements = %d, want 1", got)
	}
	snapshot := sqlCounter.snapshot()
	t.Logf("F05 fence SQL: total=%d exec=%d query=%d row=%d create=%d update=%d delete=%d begin=%d commit=%d rollback=%d",
		snapshot.Total, snapshot.Exec, snapshot.Query, snapshot.Row, snapshot.Create, snapshot.Update, snapshot.Delete,
		snapshot.Begin, snapshot.Commit, snapshot.Rollback)
	if snapshot.Begin != 1 || snapshot.Commit != 1 || snapshot.Rollback != 0 {
		t.Fatalf("scale fence transaction count = begin:%d commit:%d rollback:%d, want 1/1/0",
			snapshot.Begin, snapshot.Commit, snapshot.Rollback)
	}
	if snapshot.Total == 0 || snapshot.Total > 20 {
		t.Fatalf("scale fence SQL count = %d for 10,000 unrelated accounts, want between 1 and 20", snapshot.Total)
	}
}

func TestBumpSiteFenceConcurrentClaimAndReaperNoDeadlock(t *testing.T) {
	for _, operation := range []string{"claim", "reaper"} {
		t.Run(operation, func(t *testing.T) {
			database := openLockedSiteRunDatabase(t)
			now := int64(1_752_400_800)
			start := now - now%3600 - 3600
			site := createRunnableSite(t, database, "run-fence-"+operation, now)
			customerID := createFenceCustomer(t, database, "Run Customer "+operation, operationStatus(operation), now)
			accountID := createFenceAccount(t, database, site.ID, customerID, 200+int64(len(operation)), operationStatus(operation), now)
			run := createFenceLocalRun(t, database, "account", accountID, constant.TaskTypeAccountRebuild, site.ID, start, operationStatus(operation), now)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			locked := make(chan struct{})
			release := make(chan struct{})
			actorDone := make(chan error, 1)
			go func() {
				actorDone <- simulateFenceConcurrentWorker(ctx, database.GORM, operation, site.ID, customerID, accountID, run.ID, locked, release, now+1)
			}()
			select {
			case <-locked:
			case <-ctx.Done():
				t.Fatalf("%s did not acquire ordered locks: %v", operation, ctx.Err())
			}
			repository := NewSiteRepository(database.GORM)
			fenceDone := make(chan error, 1)
			go func() {
				fenceDone <- repository.WithTransaction(ctx, func(transaction *SiteRepository) error {
					current, err := transaction.FindByID(ctx, site.ID)
					if err != nil {
						return err
					}
					return transaction.BumpSiteFence(ctx, &current, now+2)
				})
			}()
			select {
			case err := <-fenceDone:
				t.Fatalf("fence bypassed %s site lock: %v", operation, err)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			select {
			case err := <-actorDone:
				if err != nil {
					t.Fatalf("%s transaction: %v", operation, err)
				}
			case <-ctx.Done():
				t.Fatalf("%s transaction timed out: %v", operation, ctx.Err())
			}
			select {
			case err := <-fenceDone:
				if err != nil {
					t.Fatalf("fence after %s: %v", operation, err)
				}
			case <-ctx.Done():
				t.Fatalf("fence deadlocked with %s: %v", operation, ctx.Err())
			}
			var persisted CollectionRun
			if err := database.GORM.First(&persisted, run.ID).Error; err != nil || persisted.Status != "failed" || persisted.ActiveKey != nil {
				t.Fatalf("run after concurrent %s/fence = %#v, %v", operation, persisted, err)
			}
			var accountStatus, customerStatus string
			if err := database.SQL.QueryRowContext(ctx, "SELECT statistics_backfill_status FROM account WHERE id = ?", accountID).Scan(&accountStatus); err != nil {
				t.Fatalf("read account after %s: %v", operation, err)
			}
			if err := database.SQL.QueryRowContext(ctx, "SELECT statistics_backfill_status FROM customer WHERE id = ?", customerID).Scan(&customerStatus); err != nil {
				t.Fatalf("read customer after %s: %v", operation, err)
			}
			if accountStatus != "failed" || customerStatus != "failed" {
				t.Fatalf("statuses after %s/fence account=%s customer=%s", operation, accountStatus, customerStatus)
			}
		})
	}
}

func operationStatus(operation string) string {
	if operation == "reaper" {
		return "running"
	}
	return "pending"
}

func simulateFenceConcurrentWorker(
	ctx context.Context,
	database *gorm.DB,
	operation string,
	siteID, customerID, accountID, runID int64,
	locked chan<- struct{},
	release <-chan struct{},
	now int64,
) error {
	return database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		locks := []struct {
			query string
			id    int64
		}{
			{query: "SELECT id FROM site WHERE id = ? FOR UPDATE", id: siteID},
			{query: "SELECT id FROM customer WHERE id = ? FOR UPDATE", id: customerID},
			{query: "SELECT id FROM account WHERE id = ? FOR UPDATE", id: accountID},
			{query: "SELECT id FROM collection_run WHERE id = ? FOR UPDATE", id: runID},
		}
		for _, lock := range locks {
			var id int64
			if err := transaction.Raw(lock.query, lock.id).Scan(&id).Error; err != nil || id != lock.id {
				return fmt.Errorf("lock ordered object %d: %w", lock.id, err)
			}
		}
		var windowID int64
		if err := transaction.Raw(`SELECT id FROM collection_run_window
WHERE run_id = ? ORDER BY hour_ts ASC, id ASC LIMIT 1 FOR UPDATE`, runID).Scan(&windowID).Error; err != nil || windowID <= 0 {
			return fmt.Errorf("lock run window: %w", err)
		}
		close(locked)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		status := "running"
		if operation == "reaper" {
			status = "pending"
		}
		if err := transaction.Model(&CollectionRunWindow{}).Where("id = ?", windowID).
			Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
			return err
		}
		return transaction.Model(&CollectionRun{}).Where("id = ?", runID).
			Updates(map[string]any{"status": status, "updated_at": now}).Error
	})
}

func createFenceCustomer(t *testing.T, database *Database, name, backfillStatus string, now int64) int64 {
	t.Helper()
	result, err := database.SQL.ExecContext(context.Background(), `INSERT INTO customer
  (name, contact, remark, status, statistics_backfill_status, created_at, updated_at)
VALUES (?, '', '', 'using', ?, ?, ?)`, name, backfillStatus, now, now)
	if err != nil {
		t.Fatalf("create fence customer: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read fence customer id: %v", err)
	}
	return id
}

func createFenceAccount(
	t *testing.T,
	database *Database,
	siteID, customerID, remoteUserID int64,
	backfillStatus string,
	now int64,
) int64 {
	t.Helper()
	result, err := database.SQL.ExecContext(context.Background(), `INSERT INTO account
  (site_id, customer_id, remote_user_id, remote_created_at, username, remote_status,
   managed_status, statistics_backfill_status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 1, 'active', ?, ?, ?)`,
		siteID, customerID, remoteUserID, now-3600, fmt.Sprintf("user-%d", remoteUserID), backfillStatus, now, now)
	if err != nil {
		t.Fatalf("create fence account: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read fence account id: %v", err)
	}
	return id
}

func createFenceLocalRun(
	t *testing.T,
	database *Database,
	targetType string,
	targetID int64,
	taskType string,
	windowSiteID, start int64,
	status string,
	now int64,
) CollectionRun {
	t.Helper()
	end := start + 3600
	activeKey, err := CollectionRunActiveKey(taskType, targetType, targetID, &start, &end)
	if err != nil {
		t.Fatalf("build local run key: %v", err)
	}
	initializedAt := now
	run := CollectionRun{
		TaskType: taskType, TargetType: targetType, TargetID: targetID,
		TriggerType: constant.CollectionTriggerDependency, StartTimestamp: &start, EndTimestamp: &end,
		Scope: []byte("{}"), ActiveKey: &activeKey, Status: status,
		Priority: constant.CollectionPriorityLocalRebuild, NextAttemptAt: now,
		WindowsInitializedAt: &initializedAt, TotalWindows: 1,
		CreatedRequestID: "req_local_rebuild", LastRequestID: "req_local_rebuild",
		CreatedAt: now, UpdatedAt: now,
	}
	if status == "running" {
		run.StartedAt = &now
		run.HeartbeatAt = &now
	}
	if err := database.GORM.Create(&run).Error; err != nil {
		t.Fatalf("create local rebuild run: %v", err)
	}
	window := CollectionRunWindow{
		RunID: run.ID, SiteID: windowSiteID, HourTS: start, Status: status, UpdatedAt: now,
	}
	if status == "running" {
		window.StartedAt = &now
	}
	if err := database.GORM.Create(&window).Error; err != nil {
		t.Fatalf("create local rebuild window: %v", err)
	}
	return run
}

func createRunnableSite(t *testing.T, database *Database, suffix string, now int64) Site {
	t.Helper()
	site := Site{
		Name: "Run " + suffix, BaseURL: "https://" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, Version: "v-test", DataExportEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create runnable site %s: %v", suffix, err)
	}
	capabilities := make([]SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, SiteCapability{
			SiteID: site.ID, CapabilityKey: key, Status: status, CheckedAt: now,
		})
	}
	if err := NewSiteRepository(database.GORM).ReplaceCapabilities(context.Background(), site.ID, capabilities); err != nil {
		t.Fatalf("create runnable capabilities %s: %v", suffix, err)
	}
	return site
}

func openLockedSiteRunDatabase(t *testing.T) *Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 4, MaxOpen: 20, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open collection run database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve collection run lock: %v", err)
	}
	var acquired sql.NullInt64
	const lockName = "new-api-pilot-site-service-integration"
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", lockName).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire collection run lock = %v, %v", acquired, err)
	}
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		for _, statement := range []string{
			"DELETE rw FROM collection_run_window rw JOIN site s ON s.id = rw.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE cw FROM collection_window cw JOIN site s ON s.id = cw.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE cr FROM collection_run cr JOIN account a ON cr.target_type = 'account' AND cr.target_id = a.id JOIN site s ON s.id = a.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE cr FROM collection_run cr JOIN customer c ON cr.target_type = 'customer' AND cr.target_id = c.id WHERE c.name LIKE 'Run Customer %'",
			"DELETE cr FROM collection_run cr JOIN site s ON s.id = cr.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE a FROM account a JOIN site s ON s.id = a.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE FROM customer WHERE name LIKE 'Run Customer %'",
			"DELETE sc FROM site_capability sc JOIN site s ON s.id = sc.site_id WHERE s.name LIKE 'Run run-%'",
			"DELETE FROM site WHERE name LIKE 'Run run-%'",
		} {
			_, _ = database.SQL.ExecContext(cleanupContext, statement)
		}
		_, _ = connection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
	})
	return database
}
