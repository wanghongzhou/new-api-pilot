package integration_test

import (
	"context"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
	"new-api-pilot/worker"
)

func TestA30A84SchedulerCadenceAndRealtimePriority(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400) // 2026-01-17T12:00:00+08:00
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	site := createCoreAuthorizedSite(t, database, newCoreCipher(t), now)
	repository := model.NewCollectionTaskRepository(database)
	scheduler, err := worker.NewScheduler(worker.SchedulerOptions{
		Repository: repository, Settings: model.NewCollectorSettingRepository(database), Clock: clock,
	})
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("scheduler startup: %v", err)
	}

	start := now - 2*3600
	end := now - 3600
	scope, err := model.NewUsageBackfillRunScope(false)
	if err != nil {
		t.Fatalf("build backfill scope: %v", err)
	}
	backfill, deduplicated, err := repository.EnqueueSiteTask(context.Background(), model.SiteTaskEnqueueRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: constant.TaskTypeUsageBackfill,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: &start, EndTimestamp: &end,
		Scope: scope, Priority: constant.CollectionPriorityManualBackfill, RequestID: "a30-backfill", Now: now,
		Mode: model.SiteWindowRunStrict,
	})
	if err != nil || deduplicated {
		t.Fatalf("enqueue A30 backfill = %#v deduplicated=%t err=%v", backfill, deduplicated, err)
	}
	if _, err := repository.MaterializeRunWindows(context.Background(), backfill.ID, now, 1000); err != nil {
		t.Fatalf("materialize A30 backfill: %v", err)
	}

	clock.Advance(15 * time.Minute)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("schedule delayed usage hour: %v", err)
	}
	var realtime model.CollectionRun
	if err := database.Where("site_id = ? AND task_type = ? AND trigger_type = ?", site.ID,
		constant.TaskTypeUsageHour, constant.CollectionTriggerSchedule).Take(&realtime).Error; err != nil {
		t.Fatalf("load scheduled realtime run: %v", err)
	}
	if _, err := repository.MaterializeRunWindows(context.Background(), realtime.ID, clock.Now().Unix(), 1000); err != nil {
		t.Fatalf("materialize scheduled realtime run: %v", err)
	}
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageHour, constant.TaskTypeUsageBackfill},
		Now:       clock.Now().Unix(), RequestID: "wrk_a30_priority", MaxWindow: 24,
	})
	if err != nil || claim.Run.TaskType != constant.TaskTypeUsageHour || claim.Run.Priority != constant.CollectionPriorityUsageRealtime {
		t.Fatalf("A30 realtime priority claim = %#v, %v", claim, err)
	}
	if _, err := repository.ReleaseClaim(context.Background(), claim, clock.Now().Unix()+1); err != nil {
		t.Fatalf("release realtime priority claim: %v", err)
	}

	location := time.FixedZone("Asia/Shanghai", 8*3600)
	monday := time.Date(2026, time.January, 19, 2, 0, 0, 0, location)
	clock.Set(monday)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("A84 daily and weekly schedule: %v", err)
	}
	var daily, weekly int64
	if err := database.Model(&model.CollectionRun{}).Where("site_id = ? AND task_type = ? AND priority = ?", site.ID,
		constant.TaskTypeUsageValidation, constant.CollectionPriorityDailyValidation).Count(&daily).Error; err != nil {
		t.Fatalf("count daily validation runs: %v", err)
	}
	if err := database.Model(&model.CollectionRun{}).Where("site_id = ? AND task_type = ? AND priority = ?", site.ID,
		constant.TaskTypeUsageValidation, constant.CollectionPriorityWeeklyValidation).Count(&weekly).Error; err != nil {
		t.Fatalf("count weekly validation runs: %v", err)
	}
	if daily == 0 || weekly == 0 {
		t.Fatalf("A84 validation cadence daily=%d weekly=%d", daily, weekly)
	}
	beforeDaily, beforeWeekly := daily, weekly
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("repeat A84 scheduler slot: %v", err)
	}
	if err := database.Model(&model.CollectionRun{}).Where("site_id = ? AND task_type = ? AND priority = ?", site.ID,
		constant.TaskTypeUsageValidation, constant.CollectionPriorityDailyValidation).Count(&daily).Error; err != nil {
		t.Fatalf("recount daily validation runs: %v", err)
	}
	if err := database.Model(&model.CollectionRun{}).Where("site_id = ? AND task_type = ? AND priority = ?", site.ID,
		constant.TaskTypeUsageValidation, constant.CollectionPriorityWeeklyValidation).Count(&weekly).Error; err != nil {
		t.Fatalf("recount weekly validation runs: %v", err)
	}
	if daily != beforeDaily || weekly != beforeWeekly {
		t.Fatalf("A84 duplicate scheduler slot daily=%d/%d weekly=%d/%d", daily, beforeDaily, weekly, beforeWeekly)
	}
}
