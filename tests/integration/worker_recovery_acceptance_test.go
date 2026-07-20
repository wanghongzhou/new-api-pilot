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

func TestA14A31A58A59A60A61WorkerRecoveryAndWindowOwnership(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	site := createCoreAuthorizedSite(t, database, newCoreCipher(t), now)
	start := now - 49*3600
	if err := database.Model(&model.Site{}).Where("id = ?", site.ID).Update("statistics_start_at", start).Error; err != nil {
		t.Fatalf("extend recovery fixture statistics start: %v", err)
	}
	var err error
	site, err = model.NewSiteRepository(database).FindByID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("reload recovery fixture site: %v", err)
	}
	repository := model.NewCollectionTaskRepository(database)
	claim := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill,
		start, now-3600, now, "a58-48-hour-run")
	if len(claim.Windows) != 24 {
		t.Fatalf("A58 first slice windows=%d, want 24", len(claim.Windows))
	}
	initiallyClaimed := make(map[int64]struct{}, len(claim.Windows))
	for _, window := range claim.Windows {
		initiallyClaimed[window.ID] = struct{}{}
	}
	first := claim.Windows[0]
	if _, err := repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
		RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now + 1,
		Window: model.CollectionTaskWindowResult{
			WindowID: first.ID, AttemptCount: first.AttemptCount, Status: model.CollectionTaskStatusSuccess,
		},
	}); err != nil {
		t.Fatalf("complete first 48-hour window: %v", err)
	}
	partial, err := model.NewSiteRepository(database).FindCollectionRunByID(context.Background(), claim.Run.ID)
	if err != nil || partial.Status != model.CollectionTaskStatusRunning || partial.TotalWindows != 48 || partial.CompletedWindows != 1 {
		t.Fatalf("A58 partial run = %#v, %v", partial, err)
	}

	reaper, err := worker.NewReaper(worker.ReaperOptions{
		Repository: repository, Clock: clock, AttemptPolicy: model.CollectionTaskAttemptPolicy{DefaultMaxAttempts: 5},
	})
	if err != nil {
		t.Fatalf("create recovery reaper: %v", err)
	}
	if recovered, err := reaper.Takeover(context.Background()); err != nil || recovered != 1 {
		t.Fatalf("A31/A59 startup takeover recovered=%d err=%v", recovered, err)
	}
	recovered, err := model.NewSiteRepository(database).FindCollectionRunByID(context.Background(), claim.Run.ID)
	if err != nil || recovered.Status != model.CollectionTaskStatusPending || recovered.CompletedWindows != 1 || recovered.TotalWindows != 48 || recovered.HeartbeatAt != nil {
		t.Fatalf("A14 recovered run = %#v, %v", recovered, err)
	}
	var completed model.CollectionRunWindow
	if err := database.Where("id = ?", first.ID).Take(&completed).Error; err != nil ||
		completed.Status != model.CollectionTaskStatusSuccess || completed.AttemptCount != 1 {
		t.Fatalf("A14 completed child after recovery = %#v, %v", completed, err)
	}
	resumed, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageBackfill}, Now: now + 2, RequestID: "wrk_a14_resumed", MaxWindow: 24,
	})
	if err != nil || resumed.Run.ID != claim.Run.ID {
		t.Fatalf("A14 resumed claim = %#v, %v", resumed, err)
	}
	for _, window := range resumed.Windows {
		if window.ID == first.ID {
			t.Fatalf("A14 resumed an already completed or reset window: %#v", window)
		}
		if _, wasClaimed := initiallyClaimed[window.ID]; wasClaimed && window.AttemptCount != 2 {
			t.Fatalf("A14 reclaimed window reset its attempt count: %#v", window)
		}
		if _, wasClaimed := initiallyClaimed[window.ID]; !wasClaimed && window.AttemptCount != 1 {
			t.Fatalf("A14 first-claimed window has an invalid attempt count: %#v", window)
		}
	}
	if _, err := repository.ReleaseClaim(context.Background(), resumed, now+3); err != nil {
		t.Fatalf("release resumed recovery slice: %v", err)
	}

	// A newly created manual run owns a fresh budget after an older run has
	// exhausted its lease. Reclaiming the old run never resets its attempt.
	second := createCoreAuthorizedSite(t, database, newCoreCipher(t), now)
	hour := now - 3600
	exhausted := coreClaimUsageWindow(t, repository, second, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now+4, "a60-exhausted")
	exhaustedReaper, err := worker.NewReaper(worker.ReaperOptions{
		Repository: repository, Clock: clock, AttemptPolicy: model.CollectionTaskAttemptPolicy{DefaultMaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("create exhausted reaper: %v", err)
	}
	if recoveredCount, err := exhaustedReaper.Takeover(context.Background()); err != nil || recoveredCount != 1 {
		t.Fatalf("A60 exhaust claimed run recovered=%d err=%v", recoveredCount, err)
	}
	failed, err := model.NewSiteRepository(database).FindCollectionRunByID(context.Background(), exhausted.Run.ID)
	if err != nil || failed.Status != model.CollectionTaskStatusFailed {
		t.Fatalf("A60 exhausted parent = %#v, %v", failed, err)
	}
	replacement := coreClaimUsageWindow(t, repository, second, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now+5, "a60-new-manual")
	if replacement.Run.ID == exhausted.Run.ID || len(replacement.Windows) != 1 || replacement.Windows[0].AttemptCount != 1 {
		t.Fatalf("A60 new manual claim = %#v, old=%d", replacement, exhausted.Run.ID)
	}
}
