package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

// This file is the executable acceptance entry point for collection. The
// lower-level model and worker tests exercise the full retry and lock matrix;
// these cases keep the user-visible invariants connected through the public
// service and repository boundaries.
func TestA08A12A13A28CollectionHourReplacementAndVisibility(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	hour := coreFloorHour(now - 3600)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCollectionSiteClient(now)
	factory := &collectionSiteClientFactory{client: client}
	site := createCoreAuthorizedSite(t, database, cipher, now)
	repository := model.NewCollectionTaskRepository(database)
	collector, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create usage collector: %v", err)
	}

	client.flow = []dto.UpstreamFlowRow{{
		UserID: 1, Username: "root", ModelName: "Model-A", ChannelID: 1,
		RequestCount: 2, Quota: 20, TokenUsed: 200,
	}}
	client.data = []dto.UpstreamDataRow{{
		ModelName: "Model-A", CreatedAt: hour, RequestCount: 2, Quota: 20, TokenUsed: 200,
	}}
	first := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now, "a08-first")
	coreCollectAndCommitUsageWindow(t, repository, collector, first, now+1)
	coreUsageFactCount(t, database, site.ID, hour, 1)

	// Both upstream APIs must be consulted. A non-negative data-flow residual
	// is preserved as an explicit legacy-unattributed fact instead of hiding
	// authoritative usage totals.
	client.data = []dto.UpstreamDataRow{{
		ModelName: "Model-A", CreatedAt: hour, RequestCount: 2, Quota: 21, TokenUsed: 200,
	}}
	mismatch := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageValidation,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityDailyValidation, hour, hour+3600, now+2, "a08-mismatch")
	result := coreCollectAndCommitUsageWindow(t, repository, collector, mismatch, now+3)
	if result.Failure != nil || result.Planned.AttributionStatus != model.UsageAttributionLegacyUnattributed ||
		client.flowCalls != 2 || client.dataCalls != 2 {
		t.Fatalf("A08 reconciled collection result=%#v flow=%d data=%d", result, client.flowCalls, client.dataCalls)
	}
	coreCollectionWindowStatus(t, database, site.ID, hour, model.CollectionWindowStatusComplete)
	coreUsageFactCount(t, database, site.ID, hour, 2)
	var reconciled model.SiteStatHourly
	if err := database.Where("site_id = ? AND hour_ts = ?", site.ID, hour).First(&reconciled).Error; err != nil ||
		reconciled.RequestCount != 2 || reconciled.Quota != 21 || reconciled.TokenUsed != 200 || reconciled.ActiveUsers != 1 {
		t.Fatalf("A08 reconciled site totals = %#v, %v", reconciled, err)
	}

	// A successful empty replacement removes stale facts and exposes a complete
	// zero; missing data remains unknown rather than becoming a zero.
	client.flow = nil
	client.data = nil
	empty := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now+4, "a12-empty")
	coreCollectAndCommitUsageWindow(t, repository, collector, empty, now+5)
	coreCollectionWindowStatus(t, database, site.ID, hour, model.CollectionWindowStatusComplete)
	coreUsageFactCount(t, database, site.ID, hour, 0)
	statistics, err := service.NewStatisticsService(service.StatisticsServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create statistics service: %v", err)
	}
	query := dto.StatisticsQuery{
		StartTimestamp: hour, EndTimestamp: hour + 3600, Granularity: dto.StatisticsGranularityHour,
		SiteIDs: []int64{site.ID}, Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	}
	complete, err := statistics.Sites(context.Background(), query)
	if err != nil || len(complete.Trend) != 1 || complete.Trend[0].DataStatus != model.CollectionWindowStatusComplete ||
		complete.Trend[0].RequestCount == nil || *complete.Trend[0].RequestCount != "0" {
		t.Fatalf("A12 complete empty hour = %#v, %v", complete, err)
	}

	client.flow = []dto.UpstreamFlowRow{{
		UserID: 1, Username: "root", ModelName: "Model-A", ChannelID: 1,
		RequestCount: 1, Quota: 12, TokenUsed: 100,
	}}
	client.data = []dto.UpstreamDataRow{{
		ModelName: "Model-A", CreatedAt: hour, RequestCount: 1, Quota: 11, TokenUsed: 100,
	}}
	missing := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageValidation,
		constant.CollectionTriggerSchedule, constant.CollectionPriorityDailyValidation, hour, hour+3600, now+6, "a13-missing")
	coreCollectAndCommitUsageWindow(t, repository, collector, missing, now+7)
	unknown, err := statistics.Sites(context.Background(), query)
	if err != nil || len(unknown.Trend) != 1 || unknown.Trend[0].DataStatus != model.CollectionWindowStatusMissing ||
		unknown.Trend[0].RequestCount != nil {
		t.Fatalf("A13 missing empty hour = %#v, %v", unknown, err)
	}
}

func TestA09A79CollectionWindowDeduplicationAndOverlap(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	site := createCoreAuthorizedSite(t, database, newCoreCipher(t), now)
	hour := coreFloorHour(now - 3600)
	scope, err := model.NewUsageBackfillRunScope(false)
	if err != nil {
		t.Fatalf("build manual backfill scope: %v", err)
	}
	request := model.SiteWindowRunCreateRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TriggerType: constant.CollectionTriggerManual,
		StartTimestamp: hour, EndTimestamp: hour + 2*3600, Scope: scope,
		Priority: constant.CollectionPriorityManualBackfill, RequestID: "a09-first", Now: now,
		Mode: model.SiteWindowRunStrict,
	}
	first, err := model.NewSiteRepository(database).CreateSiteWindowRun(context.Background(), request)
	if err != nil || len(first.Runs) != 1 || first.Runs[0].Deduplicated {
		t.Fatalf("A09 first window task = %#v, %v", first, err)
	}
	request.RequestID = "a09-repeat"
	repeated, err := model.NewSiteRepository(database).CreateSiteWindowRun(context.Background(), request)
	if err != nil || len(repeated.Runs) != 1 || !repeated.Runs[0].Deduplicated ||
		repeated.Runs[0].Run.ID != first.Runs[0].Run.ID {
		t.Fatalf("A09 identical task dedupe = %#v, %v", repeated, err)
	}
	request.StartTimestamp = hour + 3600
	request.EndTimestamp = hour + 3*3600
	request.RequestID = "a79-overlap"
	if _, err := model.NewSiteRepository(database).CreateSiteWindowRun(context.Background(), request); !errors.Is(err, model.ErrSiteWindowRunOverlap) {
		t.Fatalf("A79 overlapping manual task error = %v", err)
	}
}

func TestA23AuthorizationCreatesFrozenInitialBackfillRange(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCollectionSiteClient(now)
	factory := &collectionSiteClientFactory{client: client}
	sites, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create A23 site service: %v", err)
	}
	site := createCorePendingSite(t, database, now)
	token := "a23-existing-token"
	result, err := sites.Authorize(context.Background(), site.ID, dto.SiteAuthorizeRequest{
		Mode: "existing_token", RootUserID: stringPointerForCore("1"), AccessToken: &token,
	}, "a23-authorize")
	if err != nil || result.BackfillRunID == nil {
		t.Fatalf("A23 authorization result = %#v, %v", result, err)
	}
	persisted, err := model.NewSiteRepository(database).FindByID(context.Background(), site.ID)
	if err != nil || persisted.StatisticsStartAt == nil || *persisted.StatisticsStartAt != coreFloorHour(client.root.CreatedAt) ||
		persisted.StatisticsStartSource == nil || *persisted.StatisticsStartSource != "root_created_at" {
		t.Fatalf("A23 immutable statistics start = %#v, %v", persisted, err)
	}
	run, err := model.NewSiteRepository(database).LatestBackfillRun(context.Background(), site.ID)
	if err != nil || run.TaskType != constant.TaskTypeUsageBackfill || run.TriggerType != constant.CollectionTriggerRecovery ||
		run.Priority != constant.CollectionPriorityInitialBackfill || run.StartTimestamp == nil || run.EndTimestamp == nil ||
		*run.StartTimestamp != *persisted.StatisticsStartAt || *run.EndTimestamp != coreFloorHour(now) {
		t.Fatalf("A23 initial backfill run = %#v, %v", run, err)
	}
	materialized, err := model.NewCollectionTaskRepository(database).MaterializeRunWindows(context.Background(), run.ID, now, 1000)
	if err != nil || materialized.TotalWindows != int((*run.EndTimestamp-*run.StartTimestamp)/3600) || materialized.WindowsInitializedAt == nil {
		t.Fatalf("A23 initial backfill windows = %#v, %v", materialized, err)
	}
}

func coreClaimUsageWindow(
	t *testing.T,
	repository *model.CollectionTaskRepository,
	site model.Site,
	taskType, triggerType string,
	priority int,
	start, end, now int64,
	requestID string,
) model.CollectionTaskClaim {
	t.Helper()
	scope := []byte("{}")
	if taskType == constant.TaskTypeUsageBackfill {
		var err error
		scope, err = model.NewUsageBackfillRunScope(false)
		if err != nil {
			t.Fatalf("build usage backfill scope: %v", err)
		}
	}
	run, deduplicated, err := repository.EnqueueSiteTask(context.Background(), model.SiteTaskEnqueueRequest{
		SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: taskType, TriggerType: triggerType,
		StartTimestamp: &start, EndTimestamp: &end, Scope: scope, Priority: priority,
		RequestID: requestID, Now: now, Mode: model.SiteWindowRunStrict,
	})
	if err != nil || deduplicated {
		t.Fatalf("enqueue usage run %s = %#v deduplicated=%t err=%v", taskType, run, deduplicated, err)
	}
	if _, err := repository.MaterializeRunWindows(context.Background(), run.ID, now, 1000); err != nil {
		t.Fatalf("materialize usage run %d: %v", run.ID, err)
	}
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{taskType}, Now: now, RequestID: "wrk_" + requestID, MaxWindow: 24,
	})
	wantWindows := int((end - start) / 3600)
	if wantWindows > 24 {
		wantWindows = 24
	}
	if err != nil || claim.Run.ID != run.ID || len(claim.Windows) != wantWindows {
		t.Fatalf("claim usage run %d = %#v, %v", run.ID, claim, err)
	}
	return claim
}

func coreCollectAndCommitUsageWindow(
	t *testing.T,
	repository *model.CollectionTaskRepository,
	collector *service.UsageCollectionService,
	claim model.CollectionTaskClaim,
	now int64,
) service.UsageCollectionResult {
	t.Helper()
	if len(claim.Windows) != 1 {
		t.Fatalf("collect helper requires one window, got %#v", claim.Windows)
	}
	window := claim.Windows[0]
	result, err := collector.CollectHour(context.Background(), service.UsageCollectionRequest{
		Run: claim.Run, Window: window, RequestID: claim.RequestID,
	})
	if err != nil || !result.Commit.Valid() {
		t.Fatalf("collect usage hour = %#v, %v", result, err)
	}
	outcome := model.CollectionTaskWindowResult{
		WindowID: window.ID, AttemptCount: window.AttemptCount, Status: model.CollectionTaskStatusSuccess,
	}
	if result.Failure != nil {
		outcome.Status = model.CollectionTaskStatusFailed
		outcome.ErrorCode = result.Failure.Code
	}
	if _, err := repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
		RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now, Window: outcome, Mutation: result.Commit,
	}); err != nil {
		t.Fatalf("commit usage hour = %#v, %v", result, err)
	}
	return result
}

func coreUsageFactCount(t *testing.T, database *gorm.DB, siteID, hour, want int64) {
	t.Helper()
	var count int64
	if err := database.Model(&model.UsageFactHourly{}).Where("site_id = ? AND hour_ts = ?", siteID, hour).Count(&count).Error; err != nil || count != want {
		t.Fatalf("usage fact count site=%d hour=%d = %d, want %d, err=%v", siteID, hour, count, want, err)
	}
}

func coreCollectionWindowStatus(t *testing.T, database *gorm.DB, siteID, hour int64, want string) {
	t.Helper()
	var window model.CollectionWindow
	if err := database.Where("site_id = ? AND hour_ts = ?", siteID, hour).Take(&window).Error; err != nil || window.Status != want {
		t.Fatalf("collection window site=%d hour=%d = %#v, want status %s, err=%v", siteID, hour, window, want, err)
	}
}

func coreUsageStatRowCount(t *testing.T, database *gorm.DB, value any, siteID, hour, want int64) {
	t.Helper()
	var count int64
	if err := database.Model(value).Where("site_id = ? AND hour_ts = ?", siteID, hour).Count(&count).Error; err != nil || count != want {
		t.Fatalf("usage statistic row count = %d, want %d, err=%v", count, want, err)
	}
}
