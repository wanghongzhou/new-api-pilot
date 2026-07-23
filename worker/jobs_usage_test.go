package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestUsageExecutionErrorClassification(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		wantCode  string
		retryable bool
	}{
		{name: "mismatch", cause: service.ErrUpstreamDataMismatch, wantCode: string(constant.MessageDataValidationMismatch)},
		{name: "invalid", cause: service.ErrUpstreamResponseInvalid, wantCode: string(constant.MessageUpstreamResponseInvalid)},
		{name: "oversized", cause: service.ErrUpstreamResponseTooLarge, wantCode: string(constant.MessageUpstreamResponseTooLarge)},
		{name: "network", cause: service.ErrUpstreamUnavailable, wantCode: string(constant.MessageDataUpstreamUnavailable), retryable: true},
		{name: "timeout", cause: context.DeadlineExceeded, wantCode: string(constant.MessageDataUpstreamUnavailable), retryable: true},
		{name: "auth", cause: service.ErrUpstreamAuthExpired, wantCode: constant.CodeUpstreamUnavailable},
		{name: "config", cause: model.ErrSiteRunConfigChanged, wantCode: constant.CodeSiteConfigChanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := &service.UsageCollectionFailure{Code: constant.CodeUpstreamUnavailable, Params: []byte(`{"site_id":"1"}`)}
			err := classifyUsageExecutionError(test.cause, failure)
			var executionError *TaskExecutionError
			if !errors.As(err, &executionError) || executionError.Code != test.wantCode || executionError.Retryable != test.retryable ||
				string(executionError.Params) != string(failure.Params) {
				t.Fatalf("classified usage error = %#v, %v", executionError, err)
			}
		})
	}
	rateLimited := &service.UpstreamRequestError{Kind: service.UpstreamErrorRateLimited, RetryAfter: 2 * time.Hour}
	err := classifyUsageExecutionError(rateLimited, &service.UsageCollectionFailure{Code: constant.CodeUpstreamUnavailable})
	var executionError *TaskExecutionError
	if !errors.As(err, &executionError) || !executionError.Retryable || executionError.RetryAfter != 2*time.Hour {
		t.Fatalf("rate-limited usage error = %#v, %v", executionError, err)
	}
	for _, status := range []int{401, 404, 500} {
		err := classifyUsageExecutionError(
			&service.UpstreamRequestError{Kind: service.UpstreamErrorResponseInvalid, StatusCode: status},
			&service.UsageCollectionFailure{Code: string(constant.MessageUpstreamResponseInvalid)},
		)
		if !errors.As(err, &executionError) || !executionError.Retryable {
			t.Fatalf("HTTP %d usage error was not retryable: %#v", status, executionError)
		}
	}
	validResponseError := classifyUsageExecutionError(
		service.ErrUpstreamResponseInvalid,
		&service.UsageCollectionFailure{Code: string(constant.MessageUpstreamResponseInvalid)},
	)
	if !errors.As(validResponseError, &executionError) || executionError.Retryable {
		t.Fatalf("HTTP 200 validation error became retryable: %#v", executionError)
	}
	if !errors.Is(classifyUsageExecutionError(context.Canceled, nil), context.Canceled) {
		t.Fatal("usage cancellation did not remain context.Canceled")
	}
}

func TestUsageWorkerCommitsFactsTwelveSummariesCursorAndTaskAtomically(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 2, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "success")
	repository := model.NewCollectionTaskRepository(database.GORM)
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "success")
	facts := []model.UsageFactInput{{
		RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "Model-A", ChannelID: 7,
		RequestCount: 3, Quota: 30, TokenUsed: 300,
	}}
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		return completeUsageWorkerResult(t, request, now.Unix(), facts), nil
	})
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(now), collector, claim)

	assertUsageWorkerTaskState(t, database.GORM, claim, model.CollectionTaskStatusSuccess, 1, 2, 1)
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 1)
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactDaily{}, "site_id = ?", []any{fixture.site.ID}, 1)
	assertUsageWorkerSummaryCounts(t, database.GORM, fixture, hour, 1)
	cursor, err := model.FindUsageCursor(context.Background(), database.GORM, fixture.site.ID)
	if err != nil || cursor.LastCompleteHour == nil || *cursor.LastCompleteHour != hour {
		t.Fatalf("usage cursor = %#v, %v", cursor, err)
	}
}

func TestUsageWorkerCompletesEmptyHourWithoutSparseSummaries(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 3, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "empty")
	repository := model.NewCollectionTaskRepository(database.GORM)
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageBackfill, hour, now.Unix(), "empty")
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		return completeUsageWorkerResult(t, request, now.Unix(), nil), nil
	})
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(now), collector, claim)

	assertUsageWorkerTaskState(t, database.GORM, claim, model.CollectionTaskStatusSuccess, 1, 0, 0)
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
	assertEmptyUsageWorkerSummaries(t, database.GORM, fixture, hour)
	var window model.CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&window).Error; err != nil ||
		window.Status != model.CollectionWindowStatusComplete {
		t.Fatalf("empty collection window = %#v, %v", window, err)
	}
	cursor, err := model.FindUsageCursor(context.Background(), database.GORM, fixture.site.ID)
	if err != nil || cursor.LastCompleteHour == nil || *cursor.LastCompleteHour != hour {
		t.Fatalf("empty usage cursor = %#v, %v", cursor, err)
	}
}

func TestUsageWorkerMismatchKeepsOldFactsButIsolatesSummaries(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 4, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "mismatch")
	repository := model.NewCollectionTaskRepository(database.GORM)
	facts := []model.UsageFactInput{{
		RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "old", ChannelID: 1,
		RequestCount: 4, Quota: 40, TokenUsed: 400,
	}}
	seed := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageBackfill, hour, now.Unix(), "mismatch-seed")
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(now), usageCollectorFunc(
		func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
			return completeUsageWorkerResult(t, request, now.Unix(), facts), nil
		},
	), seed)

	clock := testsupport.NewFakeClock(now.Add(time.Minute))
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageValidation, hour, clock.Now().Unix(), "mismatch-check")
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		return failedUsageWorkerResult(t, request, clock.Now().Unix(), facts, service.ErrUpstreamDataMismatch, true), nil
	})
	executeUsageWorkerClaim(t, database, repository, clock, collector, claim)

	assertUsageWorkerTaskState(t, database.GORM, claim, model.CollectionTaskStatusFailed, 0, 1, 0)
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 1)
	assertUsageWorkerSummaryCounts(t, database.GORM, fixture, hour, 0)
	var collectionWindow model.CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&collectionWindow).Error; err != nil ||
		collectionWindow.Status != model.CollectionWindowStatusMissing ||
		collectionWindow.LastErrorCode != string(constant.MessageDataValidationMismatch) {
		t.Fatalf("mismatch collection window = %#v, %v", collectionWindow, err)
	}
}

func TestUsageWorkerRejectsStaleConfigAfterCollectionWithoutPollution(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 5, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "stale")
	repository := model.NewCollectionTaskRepository(database.GORM)
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "stale")
	calls := 0
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		calls++
		result := completeUsageWorkerResult(t, request, now.Unix(), []model.UsageFactInput{{
			RemoteUserID: 1, ModelName: "stale", ChannelID: 1, RequestCount: 1,
		}})
		if err := database.GORM.Model(&model.Site{}).Where("id = ?", fixture.site.ID).
			Update("config_version", fixture.site.ConfigVersion+1).Error; err != nil {
			t.Fatalf("bump stale config: %v", err)
		}
		return result, nil
	})
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(now), collector, claim)
	if calls != 1 {
		t.Fatalf("stale collector calls = %d, want 1", calls)
	}
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
	assertUsageWorkerCount(t, database.GORM, &model.CollectionWindow{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
	var window model.CollectionRunWindow
	if err := database.GORM.First(&window, claim.Windows[0].ID).Error; err != nil ||
		window.Status != model.CollectionTaskStatusPending || window.NextRetryAt == nil {
		t.Fatalf("stale run window = %#v, %v", window, err)
	}
	var run model.CollectionRun
	if err := database.GORM.First(&run, claim.Run.ID).Error; err != nil || run.Status != model.CollectionTaskStatusPending || run.HeartbeatAt != nil {
		t.Fatalf("stale run parent = %#v, %v", run, err)
	}
}

func TestUsageWorkerAuthorizationFailureFencesConcurrentOldResponsesWithoutPollution(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		kind   service.UpstreamErrorKind
		cause  error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, kind: service.UpstreamErrorAuthExpired, cause: service.ErrUpstreamAuthExpired},
		{name: "forbidden", status: http.StatusForbidden, kind: service.UpstreamErrorPermissionDenied, cause: service.ErrUpstreamPermissionDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := openWorkerTestDatabase(t)
			now := time.Date(2032, 1, 5, 13, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
			hour := now.Unix() - now.Unix()%3600 - 3600
			fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "auth-"+test.name)
			repository := model.NewCollectionTaskRepository(database.GORM)
			claim := createUsageWorkerClaim(
				t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "auth-hour-"+test.name,
			)
			pendingRun := createWorkerWindowRun(
				t, database, repository, fixture.site, constant.TaskTypeUsageValidation, constant.CollectionTriggerSchedule,
				constant.CollectionPriorityDailyValidation, []byte("{}"), hour, hour+3600,
				"req_usage_worker_auth_pending_"+test.name, now.Unix(),
			)
			var pendingWindow model.CollectionRunWindow
			if err := database.GORM.Where("run_id = ?", pendingRun.ID).First(&pendingWindow).Error; err != nil {
				t.Fatalf("read pending usage window: %v", err)
			}
			client := newUsageAuthorizationBarrierClient(&service.UpstreamRequestError{
				Kind: test.kind, StatusCode: test.status,
			})
			defer client.releaseAll()
			collector := newUsageWorkerCollectionService(t, database, fixture.site, now, client)
			clock := testsupport.NewFakeClock(now)
			var wait sync.WaitGroup
			wait.Add(2)
			go func() {
				defer wait.Done()
				executeUsageWorkerClaim(t, database, repository, clock, collector, claim)
			}()
			var duplicateErr error
			go func() {
				defer wait.Done()
				_, duplicateErr = collector.CollectHour(context.Background(), service.UsageCollectionRequest{
					Run: claim.Run, Window: claim.Windows[0], RequestID: claim.RequestID,
				})
			}()
			for index := 0; index < 2; index++ {
				select {
				case <-client.arrived:
				case <-time.After(10 * time.Second):
					client.releaseAll()
					wait.Wait()
					t.Fatalf("usage authorization responses reached barrier = %d, want 2", index)
				}
			}
			client.releaseAll()
			wait.Wait()
			if !errors.Is(duplicateErr, test.cause) {
				t.Fatalf("duplicate old usage response error = %v, want %v", duplicateErr, test.cause)
			}

			var persisted model.Site
			if err := database.GORM.First(&persisted, fixture.site.ID).Error; err != nil {
				t.Fatalf("read expired usage site: %v", err)
			}
			if persisted.ConfigVersion != fixture.site.ConfigVersion+1 || persisted.AuthStatus != constant.SiteAuthExpired ||
				persisted.StatisticsStatus != constant.SiteStatisticsError {
				t.Fatalf("expired usage site = %#v", persisted)
			}
			for _, expected := range []struct {
				runID    int64
				windowID int64
			}{
				{runID: claim.Run.ID, windowID: claim.Windows[0].ID},
				{runID: pendingRun.ID, windowID: pendingWindow.ID},
			} {
				var run model.CollectionRun
				var window model.CollectionRunWindow
				if err := database.GORM.First(&run, expected.runID).Error; err != nil ||
					run.Status != model.CollectionTaskStatusFailed || run.ActiveKey != nil || run.ErrorCode != constant.CodeSiteConfigChanged {
					t.Fatalf("fenced usage run = %#v, %v", run, err)
				}
				if err := database.GORM.First(&window, expected.windowID).Error; err != nil ||
					window.Status != model.CollectionTaskStatusFailed || window.ErrorCode != constant.CodeSiteConfigChanged {
					t.Fatalf("fenced usage window = %#v, %v", window, err)
				}
			}
			assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
			assertUsageWorkerCount(t, database.GORM, &model.UsageFactDaily{}, "site_id = ?", []any{fixture.site.ID}, 0)
			assertUsageWorkerCount(t, database.GORM, &model.CollectionWindow{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
			assertUsageWorkerSummaryCounts(t, database.GORM, fixture, hour, 0)
			if _, err := model.FindUsageCursor(context.Background(), database.GORM, fixture.site.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("expired usage cursor error = %v, want record not found", err)
			}

			var usageRunsBefore int64
			usageTypes := []string{constant.TaskTypeUsageHour, constant.TaskTypeUsageBackfill, constant.TaskTypeUsageValidation}
			if err := database.GORM.Model(&model.CollectionRun{}).
				Where("site_id = ? AND task_type IN ?", fixture.site.ID, usageTypes).Count(&usageRunsBefore).Error; err != nil {
				t.Fatalf("count usage runs before scheduler: %v", err)
			}
			schedulerClock := testsupport.NewFakeClock(now.Add(time.Hour))
			scheduler, err := NewScheduler(SchedulerOptions{
				Repository: repository, Settings: model.NewCollectorSettingRepository(database.GORM), Clock: schedulerClock,
			})
			if err != nil {
				t.Fatalf("create expired-site scheduler: %v", err)
			}
			if err := scheduler.RunOnce(context.Background()); err != nil {
				t.Fatalf("schedule expired site: %v", err)
			}
			var usageRunsAfter int64
			if err := database.GORM.Model(&model.CollectionRun{}).
				Where("site_id = ? AND task_type IN ?", fixture.site.ID, usageTypes).Count(&usageRunsAfter).Error; err != nil ||
				usageRunsAfter != usageRunsBefore {
				t.Fatalf("expired-site usage runs after scheduler = %d, before=%d, err=%v", usageRunsAfter, usageRunsBefore, err)
			}
		})
	}
}

func TestCompleteClaimedUsageWindowRejectsOldTokenAfterReclaim(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 6, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "token")
	repository := model.NewCollectionTaskRepository(database.GORM)
	oldClaim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "token")
	oldResult := completeUsageWorkerResult(t, service.UsageCollectionRequest{
		Run: oldClaim.Run, Window: oldClaim.Windows[0], RequestID: oldClaim.RequestID,
	}, now.Unix(), []model.UsageFactInput{{RemoteUserID: 1, ModelName: "old", ChannelID: 1, RequestCount: 1}})
	if _, err := repository.ReleaseClaim(context.Background(), oldClaim, now.Add(time.Minute).Unix()); err != nil {
		t.Fatalf("release old usage claim: %v", err)
	}
	newClaim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeUsageHour}, Now: now.Add(time.Minute).Unix(),
		RequestID: "wrk_usage_token_new", MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil {
		t.Fatalf("reclaim usage window: %v", err)
	}
	_, err = repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
		RunID: oldClaim.Run.ID, RequestID: oldClaim.RequestID, Now: now.Add(time.Minute).Unix(),
		Window: model.CollectionTaskWindowResult{
			WindowID: oldClaim.Windows[0].ID, AttemptCount: oldClaim.Windows[0].AttemptCount,
			Status: model.CollectionTaskStatusSuccess,
		},
		Mutation: oldResult.Commit,
	})
	if !errors.Is(err, model.ErrCollectionTaskClaimLost) {
		t.Fatalf("old token completion error = %v", err)
	}
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 0)
	newResult := completeUsageWorkerResult(t, service.UsageCollectionRequest{
		Run: newClaim.Run, Window: newClaim.Windows[0], RequestID: newClaim.RequestID,
	}, now.Add(time.Minute).Unix(), []model.UsageFactInput{{RemoteUserID: 1, ModelName: "new", ChannelID: 1, RequestCount: 2}})
	if _, err := repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
		RunID: newClaim.Run.ID, RequestID: newClaim.RequestID, Now: now.Add(time.Minute).Unix(),
		Window: model.CollectionTaskWindowResult{
			WindowID: newClaim.Windows[0].ID, AttemptCount: newClaim.Windows[0].AttemptCount,
			Status: model.CollectionTaskStatusSuccess,
		},
		Mutation: newResult.Commit,
	}); err != nil {
		t.Fatalf("new token completion: %v", err)
	}
	assertUsageWorkerCount(t, database.GORM, &model.UsageFactHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 1)
}

func TestUsageWorkerRetryExhaustionFailsWithoutUnavailableFactState(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 7, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "exhaustion")
	repository := model.NewCollectionTaskRepository(database.GORM)
	clock := testsupport.NewFakeClock(now)
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "exhaustion")
	calls := 0
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		calls++
		return failedUsageWorkerResult(t, request, clock.Now().Unix(), nil, service.ErrUpstreamUnavailable, false), nil
	})
	for attempt := 1; attempt <= 4; attempt++ {
		executeUsageWorkerClaim(t, database, repository, clock, collector, claim)
		if attempt == 4 {
			break
		}
		clock.Advance(2 * time.Hour)
		var err error
		claim, err = repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
			TaskTypes: []string{constant.TaskTypeUsageHour}, Now: clock.Now().Unix(),
			RequestID: fmt.Sprintf("wrk_usage_retry_%d", attempt+1), MaxWindow: 24, ScanLimit: 64,
		})
		if err != nil {
			t.Fatalf("claim usage retry %d: %v", attempt+1, err)
		}
	}
	if calls != 4 {
		t.Fatalf("usage retry calls = %d, want 4", calls)
	}
	assertUsageWorkerTaskState(t, database.GORM, claim, model.CollectionTaskStatusFailed, 0, 0, 0)
	var collectionWindow model.CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&collectionWindow).Error; err != nil ||
		collectionWindow.Status != model.CollectionWindowStatusMissing ||
		collectionWindow.Status == model.CollectionWindowStatusUnavailable {
		t.Fatalf("exhausted fact state = %#v, %v", collectionWindow, err)
	}
	var runWindow model.CollectionRunWindow
	if err := database.GORM.First(&runWindow, claim.Windows[0].ID).Error; err != nil ||
		runWindow.AttemptCount != 4 || runWindow.ErrorCode != string(constant.MessageDataUpstreamUnavailable) {
		t.Fatalf("exhausted run window = %#v, %v", runWindow, err)
	}
}

func TestUsageValidationSameHashUsesActualVerifiedOnlyCounts(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 8, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	fixture := createUsageWorkerSite(t, database, hour, now.Unix(), "verified")
	repository := model.NewCollectionTaskRepository(database.GORM)
	facts := []model.UsageFactInput{{RemoteUserID: 1, ModelName: "stable", ChannelID: 1, RequestCount: 5}}
	seed := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageHour, hour, now.Unix(), "verified-seed")
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(now), usageCollectorFunc(
		func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
			return completeUsageWorkerResult(t, request, now.Unix(), facts), nil
		},
	), seed)
	var original model.UsageFactHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&original).Error; err != nil {
		t.Fatalf("read original validation fact: %v", err)
	}

	validationNow := now.Add(time.Hour)
	claim := createUsageWorkerClaim(t, database, repository, fixture.site, constant.TaskTypeUsageValidation, hour, validationNow.Unix(), "verified-check")
	executeUsageWorkerClaim(t, database, repository, testsupport.NewFakeClock(validationNow), usageCollectorFunc(
		func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
			return completeUsageWorkerResult(t, request, validationNow.Unix(), facts), nil
		},
	), claim)
	assertUsageWorkerTaskState(t, database.GORM, claim, model.CollectionTaskStatusSuccess, 1, 2, 0)
	var current model.UsageFactHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&current).Error; err != nil ||
		current.ID != original.ID || current.CollectedAt != original.CollectedAt {
		t.Fatalf("verified-only fact = %#v, original=%#v, err=%v", current, original, err)
	}
}

func TestUsageWorkerConcurrentSitesDoNotLoseGlobalTotals(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2032, 1, 9, 12, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	hour := now.Unix() - now.Unix()%3600 - 3600
	repository := model.NewCollectionTaskRepository(database.GORM)
	first := createUsageWorkerSite(t, database, hour, now.Unix(), "concurrent-first")
	second := createUsageWorkerSite(t, database, hour, now.Unix(), "concurrent-second")
	firstClaim := createUsageWorkerClaim(t, database, repository, first.site, constant.TaskTypeUsageHour, hour, now.Unix(), "concurrent-first")
	secondClaim := createUsageWorkerClaim(t, database, repository, second.site, constant.TaskTypeUsageHour, hour, now.Unix(), "concurrent-second")
	metrics := map[int64]int64{first.site.ID: 10, second.site.ID: 20}
	collector := usageCollectorFunc(func(_ context.Context, request service.UsageCollectionRequest) (service.UsageCollectionResult, error) {
		value := metrics[*request.Run.SiteID]
		return completeUsageWorkerResult(t, request, now.Unix(), []model.UsageFactInput{{
			RemoteUserID: 1, ModelName: "shared", ChannelID: 1,
			RequestCount: value, Quota: value * 10, TokenUsed: value * 100,
		}}), nil
	})
	clock := testsupport.NewFakeClock(now)
	var wait sync.WaitGroup
	for _, claim := range []model.CollectionTaskClaim{firstClaim, secondClaim} {
		claim := claim
		wait.Add(1)
		go func() {
			defer wait.Done()
			executeUsageWorkerClaim(t, database, repository, clock, collector, claim)
		}()
	}
	wait.Wait()
	var hourly model.GlobalStatHourly
	if err := database.GORM.Where("hour_ts = ?", hour).First(&hourly).Error; err != nil ||
		hourly.RequestCount != 30 || hourly.Quota != 300 || hourly.TokenUsed != 3000 || hourly.ActiveUsers != 2 {
		t.Fatalf("concurrent global hourly = %#v, %v", hourly, err)
	}
	dateKey, _, _, err := model.UsageDateBucket(hour)
	if err != nil {
		t.Fatalf("concurrent date key: %v", err)
	}
	var daily model.GlobalStatDaily
	if err := database.GORM.Where("date_key = ?", dateKey).First(&daily).Error; err != nil ||
		daily.RequestCount != 30 || daily.Quota != 300 || daily.TokenUsed != 3000 || daily.ActiveUsers != 2 {
		t.Fatalf("concurrent global daily = %#v, %v", daily, err)
	}
}

type usageCollectorFunc func(context.Context, service.UsageCollectionRequest) (service.UsageCollectionResult, error)

func (function usageCollectorFunc) CollectHour(
	ctx context.Context,
	request service.UsageCollectionRequest,
) (service.UsageCollectionResult, error) {
	return function(ctx, request)
}

type usageAuthorizationBarrierClient struct {
	service.SiteUpstreamClient
	failure     error
	arrived     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newUsageAuthorizationBarrierClient(failure error) *usageAuthorizationBarrierClient {
	return &usageAuthorizationBarrierClient{
		failure: failure, arrived: make(chan struct{}, 2), release: make(chan struct{}),
	}
}

func (client *usageAuthorizationBarrierClient) FlowHour(context.Context, string, int64) ([]dto.UpstreamFlowRow, error) {
	return []dto.UpstreamFlowRow{{
		UserID: 1, Username: "root", ModelName: "must-not-commit", ChannelID: 1,
		RequestCount: 7, Quota: 70, TokenUsed: 700,
	}}, nil
}

func (client *usageAuthorizationBarrierClient) DataHour(context.Context, string, int64) ([]dto.UpstreamDataRow, error) {
	client.arrived <- struct{}{}
	<-client.release
	return nil, client.failure
}

func (client *usageAuthorizationBarrierClient) CloseIdleConnections() {}

func (client *usageAuthorizationBarrierClient) releaseAll() {
	client.releaseOnce.Do(func() { close(client.release) })
}

type usageWorkerClientFactory struct{ client service.SiteUpstreamClient }

func (factory usageWorkerClientFactory) NewPublic(string) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}

func (factory usageWorkerClientFactory) NewAuthenticated(string, string, string, int64) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}

func newUsageWorkerCollectionService(
	t *testing.T,
	database *model.Database,
	site model.Site,
	now time.Time,
	client service.SiteUpstreamClient,
) *service.UsageCollectionService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create usage worker cipher: %v", err)
	}
	encrypted, err := cipher.Encrypt([]byte("usage-worker-secret"), fmt.Sprintf("site:%d:access_token", site.ID))
	if err != nil {
		t.Fatalf("encrypt usage worker credential: %v", err)
	}
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).
		Update("access_token_encrypted", encrypted).Error; err != nil {
		t.Fatalf("store usage worker credential: %v", err)
	}
	collector, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: model.NewSiteRepository(database.GORM), ClientFactory: usageWorkerClientFactory{client: client},
		Cipher: cipher, Clock: testsupport.NewFakeClock(now),
	})
	if err != nil {
		t.Fatalf("create real usage worker collector: %v", err)
	}
	return collector
}

type usageWorkerSiteFixture struct {
	site       model.Site
	account    model.Account
	customerID int64
}

func createUsageWorkerSite(
	t *testing.T,
	database *model.Database,
	hour int64,
	now int64,
	suffix string,
) usageWorkerSiteFixture {
	t.Helper()
	site := createWorkerTestSite(t, database, "usage-"+suffix, now)
	rootID := int64(1)
	statisticsStart := hour
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"root_user_id": rootID, "statistics_start_at": statisticsStart,
	}).Error; err != nil {
		t.Fatalf("configure usage worker site: %v", err)
	}
	site.RootUserID = &rootID
	site.StatisticsStartAt = &statisticsStart
	customer := model.Customer{
		Name: "Usage Worker " + suffix, Status: "using", StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create usage worker customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 1, RemoteCreatedAt: hour - 3600,
		Username: "root", RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create usage worker account: %v", err)
	}
	fixture := usageWorkerSiteFixture{site: site, account: account, customerID: customer.ID}
	t.Cleanup(func() { cleanupUsageWorkerSite(database.GORM, fixture, hour) })
	return fixture
}

func createUsageWorkerClaim(
	t *testing.T,
	database *model.Database,
	repository *model.CollectionTaskRepository,
	site model.Site,
	taskType string,
	hour int64,
	now int64,
	suffix string,
) model.CollectionTaskClaim {
	t.Helper()
	scope := []byte("{}")
	triggerType := constant.CollectionTriggerSchedule
	priority := constant.CollectionPriorityUsageRealtime
	if taskType == constant.TaskTypeUsageBackfill {
		var err error
		scope, err = model.NewUsageBackfillRunScope(false)
		if err != nil {
			t.Fatalf("build usage worker scope: %v", err)
		}
		triggerType = constant.CollectionTriggerManual
		priority = constant.CollectionPriorityManualBackfill
	} else if taskType == constant.TaskTypeUsageValidation {
		priority = constant.CollectionPriorityDailyValidation
	}
	createWorkerWindowRun(
		t, database, repository, site, taskType, triggerType,
		priority, scope, hour, hour+3600,
		"req_usage_worker_"+suffix, now,
	)
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{taskType}, Now: now, RequestID: "wrk_usage_" + suffix,
		MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil {
		t.Fatalf("claim usage worker task %s: %v", taskType, err)
	}
	if len(claim.Windows) != 1 {
		t.Fatalf("claimed usage windows = %d, want 1", len(claim.Windows))
	}
	return claim
}

func executeUsageWorkerClaim(
	t *testing.T,
	database *model.Database,
	repository *model.CollectionTaskRepository,
	clock *testsupport.FakeClock,
	collector UsageHourCollector,
	claim model.CollectionTaskClaim,
) {
	t.Helper()
	executor, err := NewExecutor(ExecutorOptions{
		Repository: repository, Settings: model.NewCollectorSettingRepository(database.GORM), Clock: clock,
		Handlers: UsageJobHandlers(collector), AttemptPolicy: defaultAttemptPolicy(),
	})
	if err != nil {
		t.Fatalf("create usage worker executor: %v", err)
	}
	executor.executeClaim(context.Background(), claim)
}

func completeUsageWorkerResult(
	t *testing.T,
	request service.UsageCollectionRequest,
	now int64,
	facts []model.UsageFactInput,
) service.UsageCollectionResult {
	t.Helper()
	fetchedRows := int64(0)
	flowRows := int64(0)
	dataRows := int64(0)
	if len(facts) > 0 {
		flowRows = int64(len(facts))
		dataRows = 1
		fetchedRows = flowRows + dataRows
	}
	mutation, planned, err := model.NewCompleteUsageWindowMutation(model.CompleteUsageWindowRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: request.Window.SiteID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now,
		FetchedRows: fetchedRows, Validation: request.Run.TaskType == constant.TaskTypeUsageValidation, Facts: facts,
	})
	if err != nil {
		t.Fatalf("build complete usage worker mutation: %v", err)
	}
	commit, err := model.NewUsageAggregationCommit(model.UsageAggregationMutationRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: request.Window.SiteID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now, NewFacts: facts,
	}, mutation)
	if err != nil {
		t.Fatalf("build complete usage worker commit: %v", err)
	}
	return service.UsageCollectionResult{
		Commit: commit, Planned: planned, FlowRows: flowRows, DataRows: dataRows, SourceRequestID: request.RequestID,
	}
}

func failedUsageWorkerResult(
	t *testing.T,
	request service.UsageCollectionRequest,
	now int64,
	facts []model.UsageFactInput,
	cause error,
	mismatch bool,
) service.UsageCollectionResult {
	t.Helper()
	reason := string(constant.MessageDataUpstreamUnavailable)
	code := constant.CodeUpstreamUnavailable
	if mismatch {
		reason = string(constant.MessageDataValidationMismatch)
		code = reason
	}
	params := []byte(fmt.Sprintf(`{"site_id":"%d","start_timestamp":%d,"end_timestamp":%d}`,
		request.Window.SiteID, request.Window.HourTS, request.Window.HourTS+3600))
	mutation, err := model.NewFailedUsageWindowMutation(model.FailedUsageWindowRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: request.Window.SiteID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now,
		FetchedRows: int64(len(facts)), ReasonCode: reason, ReasonParams: params, DataMismatch: mismatch,
	})
	if err != nil {
		t.Fatalf("build failed usage worker mutation: %v", err)
	}
	commit, err := model.NewUsageAggregationCommit(model.UsageAggregationMutationRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: request.Window.SiteID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now, NewFacts: facts,
	}, mutation)
	if err != nil {
		t.Fatalf("build failed usage worker commit: %v", err)
	}
	return service.UsageCollectionResult{
		Commit: commit, Failure: &service.UsageCollectionFailure{Code: code, Params: params, Cause: cause},
		FlowRows: int64(len(facts)), SourceRequestID: request.RequestID,
	}
}

func assertUsageWorkerTaskState(
	t *testing.T,
	db *gorm.DB,
	claim model.CollectionTaskClaim,
	wantStatus string,
	wantCompleted int,
	wantFetched int64,
	wantWritten int64,
) {
	t.Helper()
	var run model.CollectionRun
	if err := db.First(&run, claim.Run.ID).Error; err != nil {
		t.Fatalf("read usage run: %v", err)
	}
	var window model.CollectionRunWindow
	if err := db.First(&window, claim.Windows[0].ID).Error; err != nil {
		t.Fatalf("read usage run window: %v", err)
	}
	if run.Status != wantStatus || window.Status != wantStatus || run.CompletedWindows != wantCompleted ||
		window.FetchedRows != wantFetched || window.WrittenRows != wantWritten ||
		run.FetchedRows != wantFetched || run.WrittenRows != wantWritten {
		t.Fatalf("usage task state run=%#v window=%#v", run, window)
	}
}

func assertUsageWorkerSummaryCounts(
	t *testing.T,
	db *gorm.DB,
	fixture usageWorkerSiteFixture,
	hour int64,
	want int64,
) {
	t.Helper()
	dateKey, _, _, err := model.UsageDateBucket(hour)
	if err != nil {
		t.Fatalf("usage worker date bucket: %v", err)
	}
	checks := []struct {
		value any
		where string
		args  []any
	}{
		{&model.AccountStatHourly{}, "account_id = ? AND hour_ts = ?", []any{fixture.account.ID, hour}},
		{&model.CustomerStatHourly{}, "customer_id = ? AND site_id = ? AND hour_ts = ?", []any{fixture.customerID, fixture.site.ID, hour}},
		{&model.SiteStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.ModelStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.ChannelStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.GlobalStatHourly{}, "hour_ts = ?", []any{hour}},
		{&model.AccountStatDaily{}, "account_id = ? AND date_key = ?", []any{fixture.account.ID, dateKey}},
		{&model.CustomerStatDaily{}, "customer_id = ? AND site_id = ? AND date_key = ?", []any{fixture.customerID, fixture.site.ID, dateKey}},
		{&model.SiteStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.ModelStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.ChannelStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.GlobalStatDaily{}, "date_key = ?", []any{dateKey}},
	}
	for _, check := range checks {
		assertUsageWorkerCount(t, db, check.value, check.where, check.args, want)
	}
}

func assertEmptyUsageWorkerSummaries(t *testing.T, db *gorm.DB, fixture usageWorkerSiteFixture, hour int64) {
	t.Helper()
	dateKey, _, _, err := model.UsageDateBucket(hour)
	if err != nil {
		t.Fatalf("empty usage worker date bucket: %v", err)
	}
	for _, check := range []struct {
		value any
		where string
		args  []any
	}{
		{&model.AccountStatHourly{}, "account_id = ? AND hour_ts = ?", []any{fixture.account.ID, hour}},
		{&model.CustomerStatHourly{}, "customer_id = ? AND site_id = ? AND hour_ts = ?", []any{fixture.customerID, fixture.site.ID, hour}},
		{&model.SiteStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.ModelStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.ChannelStatHourly{}, "site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}},
		{&model.UsageFactDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.ModelStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.ChannelStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
	} {
		assertUsageWorkerCount(t, db, check.value, check.where, check.args, 0)
	}
	assertEmptyUsageWorkerGlobalHour(t, db, hour)
	for _, check := range []struct {
		value any
		where string
		args  []any
	}{
		{&model.AccountStatDaily{}, "account_id = ? AND date_key = ?", []any{fixture.account.ID, dateKey}},
		{&model.CustomerStatDaily{}, "customer_id = ? AND site_id = ? AND date_key = ?", []any{fixture.customerID, fixture.site.ID, dateKey}},
		{&model.SiteStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}},
		{&model.GlobalStatDaily{}, "date_key = ?", []any{dateKey}},
	} {
		assertUsageWorkerCount(t, db, check.value, check.where, check.args, 1)
	}
}

func assertEmptyUsageWorkerGlobalHour(t *testing.T, db *gorm.DB, hour int64) {
	t.Helper()
	var global model.GlobalStatHourly
	err := db.Where("hour_ts = ?", hour).Take(&global).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if err != nil {
		t.Fatalf("load empty usage worker global hour: %v", err)
	}
	if global.RequestCount != 0 || global.Quota != 0 || global.TokenUsed != 0 || global.ActiveUsers != 0 ||
		global.DataStatus != model.UsageAggregationStatusPartial {
		t.Fatalf("empty usage worker global hour = %#v, want zero partial row when other expected sites are incomplete", global)
	}
}

func assertUsageWorkerCount(t *testing.T, db *gorm.DB, value any, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Where(where, args...).Count(&count).Error; err != nil || count != want {
		t.Fatalf("usage worker row count for %T = %d, want %d, err=%v", value, count, want, err)
	}
}

func cleanupUsageWorkerSite(db *gorm.DB, fixture usageWorkerSiteFixture, hour int64) {
	ctx := context.Background()
	dateKey, _, _, _ := model.UsageDateBucket(hour)
	_ = db.WithContext(ctx).Where("account_id = ?", fixture.account.ID).Delete(&model.AccountStatHourly{}).Error
	_ = db.WithContext(ctx).Where("account_id = ?", fixture.account.ID).Delete(&model.AccountStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.CustomerStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.CustomerStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.SiteStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.SiteStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.ModelStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.ModelStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.ChannelStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.ChannelStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.UsageFactDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.UsageFactHourly{}).Error
	_ = db.WithContext(ctx).Where("hour_ts = ?", hour).Delete(&model.GlobalStatHourly{}).Error
	_ = db.WithContext(ctx).Where("date_key = ?", dateKey).Delete(&model.GlobalStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.CollectionCursor{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.CollectionWindow{}).Error
	_ = db.WithContext(ctx).Exec("DELETE rw FROM collection_run_window rw JOIN collection_run r ON r.id = rw.run_id WHERE r.site_id = ?", fixture.site.ID).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.CollectionRun{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&model.SiteCapability{}).Error
	_ = db.WithContext(ctx).Delete(&model.Account{}, fixture.account.ID).Error
	_ = db.WithContext(ctx).Delete(&model.Customer{}, fixture.customerID).Error
	_ = db.WithContext(ctx).Delete(&model.Site{}, fixture.site.ID).Error
}
