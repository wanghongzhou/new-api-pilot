package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"new-api-pilot/constant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestCanonicalUsageFactsPreserveFlowParityDimensions(t *testing.T) {
	hour := int64(1_752_948_000)
	base := UsageFactInput{RemoteUserID: 1, UsernameSnapshot: "user", ModelName: "model", ChannelID: 2,
		UseGroup: "group-a", TokenID: 9, TokenName: "zeta", NodeName: "node-a",
		RequestCount: 1, Quota: 2, TokenUsed: 3}
	duplicate := base
	duplicate.TokenName = "alpha"
	group := base
	group.UseGroup = "group-b"
	node := base
	node.NodeName = "node-b"

	forward, forwardHash, err := canonicalUsageFacts(1, hour, hour+3600, []UsageFactInput{base, group, node, duplicate})
	if err != nil {
		t.Fatalf("canonical usage facts: %v", err)
	}
	reverse, reverseHash, err := canonicalUsageFacts(1, hour, hour+3600, []UsageFactInput{duplicate, node, group, base})
	if err != nil {
		t.Fatalf("reverse canonical usage facts: %v", err)
	}
	if len(forward) != 3 || len(reverse) != 3 || forwardHash != reverseHash {
		t.Fatalf("canonical parity facts/hash = %d/%d %s/%s", len(forward), len(reverse), forwardHash, reverseHash)
	}
	forwardJSON, _ := json.Marshal(forward)
	reverseJSON, _ := json.Marshal(reverse)
	if !bytes.Equal(forwardJSON, reverseJSON) {
		t.Fatalf("input order changed canonical facts: %s / %s", forwardJSON, reverseJSON)
	}
	if forward[0].TokenName != "alpha" || forward[0].RequestCount != 2 {
		t.Fatalf("canonical token snapshot = %+v", forward[0])
	}

	changed := base
	changed.TokenID++
	_, changedHash, err := canonicalUsageFacts(1, hour, hour+3600, []UsageFactInput{changed})
	if err != nil || changedHash == forwardHash {
		t.Fatalf("token dimension did not affect hash: %s / %s, %v", forwardHash, changedHash, err)
	}
}

func TestUsageWindowReplacementMismatchAndCursorRepair(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_950_000)
	hour0 := now - now%3600 - 4*3600
	fixture := createUsageMutationFixture(t, database, hour0, 4, constant.TaskTypeUsageBackfill, now)

	hour2Mutation, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(2, now+1, []UsageFactInput{{
		RemoteUserID: 3, UsernameSnapshot: "hour-two", ModelName: "Model-A", ChannelID: 0,
		RequestCount: 1, Quota: 10, TokenUsed: 100,
	}}))
	if err != nil {
		t.Fatalf("plan out-of-order complete: %v", err)
	}
	applyUsageMutation(t, database, fixture, 2, hour2Mutation)
	assertUsageCursor(t, database, fixture.site.ID, nil)

	hour0Input := []UsageFactInput{
		{RemoteUserID: 1, UsernameSnapshot: "zeta", ModelName: "Model-A", ChannelID: 0, RequestCount: 2, Quota: 20, TokenUsed: 200},
		{RemoteUserID: 1, UsernameSnapshot: "alpha", ModelName: "Model-A", ChannelID: 0, RequestCount: 3, Quota: 30, TokenUsed: 300},
		{RemoteUserID: 2, UsernameSnapshot: "", ModelName: "model-a", ChannelID: 7, RequestCount: 4, Quota: 40, TokenUsed: 400},
	}
	hour0Mutation, planned, err := NewCompleteUsageWindowMutation(fixture.completeRequest(0, now+2, hour0Input))
	if err != nil || planned.WrittenRows != 2 || len(planned.SourceHash) != 64 {
		t.Fatalf("plan canonical complete = %#v, %v", planned, err)
	}
	first := applyUsageMutation(t, database, fixture, 0, hour0Mutation)
	second := applyUsageMutation(t, database, fixture, 0, hour0Mutation)
	if first.SourceHash != second.SourceHash || first.WrittenRows != 2 || second.WrittenRows != 2 {
		t.Fatalf("idempotent complete results = %#v / %#v", first, second)
	}
	var hour0Facts []UsageFactHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour0).Order("remote_user_id, model_name, channel_id").Find(&hour0Facts).Error; err != nil {
		t.Fatalf("read canonical hour zero facts: %v", err)
	}
	if len(hour0Facts) != 2 || hour0Facts[0].UsernameSnapshot != "alpha" || hour0Facts[0].RequestCount != 5 ||
		hour0Facts[0].Quota != 50 || hour0Facts[0].TokenUsed != 500 || hour0Facts[1].ModelName != "model-a" {
		t.Fatalf("canonical facts = %#v", hour0Facts)
	}
	assertUsageCursor(t, database, fixture.site.ID, &hour0)

	hour1Mutation, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(1, now+3, []UsageFactInput{{
		RemoteUserID: 1, UsernameSnapshot: "old", ModelName: "old-model", ChannelID: 2,
		RequestCount: 9, Quota: 90, TokenUsed: 900,
	}}))
	if err != nil {
		t.Fatalf("plan hour one complete: %v", err)
	}
	applyUsageMutation(t, database, fixture, 1, hour1Mutation)
	assertUsageCursor(t, database, fixture.site.ID, &fixture.hours[2])

	mismatch, err := NewFailedUsageWindowMutation(fixture.failedRequest(1, now+4, true))
	if err != nil {
		t.Fatalf("plan mismatch: %v", err)
	}
	mismatchResult := applyUsageMutation(t, database, fixture, 1, mismatch)
	if mismatchResult.CollectionStatus != CollectionWindowStatusMissing || mismatchResult.ReasonCode != string(constant.MessageDataValidationMismatch) {
		t.Fatalf("mismatch result = %#v", mismatchResult)
	}
	assertUsageFactCount(t, database, fixture.site.ID, fixture.hours[1], 1)
	assertUsageWindowStatus(t, database, fixture.site.ID, fixture.hours[1], CollectionWindowStatusMissing)
	assertUsageCursor(t, database, fixture.site.ID, &hour0)

	executionFailure, err := NewFailedUsageWindowMutation(fixture.failedRequest(0, now+5, false))
	if err != nil {
		t.Fatalf("plan execution failure: %v", err)
	}
	executionResult := applyUsageMutation(t, database, fixture, 0, executionFailure)
	if executionResult.CollectionStatus != CollectionWindowStatusComplete {
		t.Fatalf("execution failure hid complete facts: %#v", executionResult)
	}
	assertUsageWindowStatus(t, database, fixture.site.ID, hour0, CollectionWindowStatusComplete)

	emptyRepair, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(1, now+6, nil))
	if err != nil {
		t.Fatalf("plan empty repair: %v", err)
	}
	emptyResult := applyUsageMutation(t, database, fixture, 1, emptyRepair)
	if emptyResult.WrittenRows != 0 || emptyResult.CollectionStatus != CollectionWindowStatusComplete {
		t.Fatalf("empty replacement = %#v", emptyResult)
	}
	assertUsageFactCount(t, database, fixture.site.ID, fixture.hours[1], 0)
	assertUsageCursor(t, database, fixture.site.ID, &fixture.hours[2])

	lastRunID := fixture.run.ID
	unavailable := CollectionWindow{
		SiteID: fixture.site.ID, HourTS: fixture.hours[3], Status: CollectionWindowStatusUnavailable,
		LastFactRunID: &lastRunID, LastErrorCode: string(constant.MessageDataUpstreamUnavailable),
		LastErrorParams: []byte(`{"site_id":"1","start_timestamp":1,"end_timestamp":2}`), UpdatedAt: now + 7,
	}
	if err := database.GORM.Create(&unavailable).Error; err != nil {
		t.Fatalf("create unavailable usage window: %v", err)
	}
	unavailableFailure, err := NewFailedUsageWindowMutation(fixture.failedRequest(3, now+8, false))
	if err != nil {
		t.Fatalf("plan unavailable execution failure: %v", err)
	}
	unavailableResult := applyUsageMutation(t, database, fixture, 3, unavailableFailure)
	if unavailableResult.CollectionStatus != CollectionWindowStatusUnavailable {
		t.Fatalf("execution failure downgraded unavailable facts: %#v", unavailableResult)
	}
	var preserved CollectionWindow
	if err := database.GORM.First(&preserved, unavailable.ID).Error; err != nil {
		t.Fatalf("read preserved unavailable window: %v", err)
	}
	if preserved.Status != CollectionWindowStatusUnavailable ||
		preserved.LastErrorCode != string(constant.MessageDataUpstreamUnavailable) {
		t.Fatalf("preserved unavailable window = %#v", preserved)
	}
}

func TestUsageWindowValidationVerifiedOnlyAndConfigFence(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_960_000)
	hour := now - now%3600 - 2*3600
	fixture := createUsageMutationFixture(t, database, hour, 2, constant.TaskTypeUsageValidation, now)
	input := []UsageFactInput{{
		RemoteUserID: 1, UsernameSnapshot: "", ModelName: "", ChannelID: 0,
		RequestCount: 1, Quota: 2, TokenUsed: 3,
	}}
	initialRequest := fixture.completeRequest(0, now+1, input)
	initialRequest.Validation = false
	initial, _, err := NewCompleteUsageWindowMutation(initialRequest)
	if err != nil {
		t.Fatalf("plan initial facts: %v", err)
	}
	applyUsageMutation(t, database, fixture, 0, initial)

	validationRequest := fixture.completeRequest(0, now+2, input)
	validationRequest.Validation = true
	validation, _, err := NewCompleteUsageWindowMutation(validationRequest)
	if err != nil {
		t.Fatalf("plan validation: %v", err)
	}
	verified := applyUsageMutation(t, database, fixture, 0, validation)
	if !verified.VerifiedOnly || verified.WrittenRows != 0 {
		t.Fatalf("identical validation rewrote facts: %#v", verified)
	}
	assertUsageFactCount(t, database, fixture.site.ID, hour, 1)

	staleRequest := fixture.completeRequest(1, now+3, input)
	staleRequest.Validation = true
	stale, _, err := NewCompleteUsageWindowMutation(staleRequest)
	if err != nil {
		t.Fatalf("plan stale response: %v", err)
	}
	if err := database.GORM.Model(&Site{}).Where("id = ?", fixture.site.ID).
		Updates(map[string]any{"config_version": 2, "updated_at": now + 3}).Error; err != nil {
		t.Fatalf("bump site config version: %v", err)
	}
	declaredScope := UsageWindowMutationScope{
		Site: fixture.site, Run: fixture.run, Window: fixture.windows[1],
	}
	_, err = applyUsageMutationResultWithDeclaredScope(database, fixture, 1, stale, &declaredScope)
	if !errors.Is(err, ErrSiteRunConfigChanged) {
		t.Fatalf("stale response fence error = %v", err)
	}
	assertUsageFactCount(t, database, fixture.site.ID, fixture.hours[1], 0)
}

func TestUsageMutationRejectsHourBeforeStatisticsStart(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_965_000)
	hour := now - now%3600 - 3600
	fixture := createUsageMutationFixture(t, database, hour, 1, constant.TaskTypeUsageBackfill, now)
	statisticsStart := hour + 3600
	if err := database.GORM.Model(&Site{}).Where("id = ?", fixture.site.ID).
		Update("statistics_start_at", statisticsStart).Error; err != nil {
		t.Fatalf("move usage statistics start: %v", err)
	}
	mutation, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(0, now+1, nil))
	if err != nil {
		t.Fatalf("plan usage before statistics start: %v", err)
	}
	_, err = applyUsageMutationResult(database, fixture, 0, mutation)
	if !errors.Is(err, ErrSiteRunConfigChanged) {
		t.Fatalf("usage before statistics start error = %v", err)
	}
}

func TestUsageMutationRejectsUnfinishedWindowWithoutWrites(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	hour := int64(1_752_969_600)
	now := hour + 300
	fixture := createUsageMutationFixture(t, database, hour, 1, constant.TaskTypeUsageBackfill, now)
	complete, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(0, now, []UsageFactInput{{
		RemoteUserID: 1, ModelName: "unfinished", RequestCount: 1, Quota: 2, TokenUsed: 3,
	}}))
	if err != nil {
		t.Fatalf("plan unfinished complete: %v", err)
	}
	if _, err := applyUsageMutationResult(database, fixture, 0, complete); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("apply unfinished complete error = %v", err)
	}
	failedRequest := fixture.failedRequest(0, now, false)
	failedRequest.ReasonCode = string(constant.MessageDataUpstreamUnavailable)
	failed, err := NewFailedUsageWindowMutation(failedRequest)
	if err != nil {
		t.Fatalf("plan unfinished failure: %v", err)
	}
	if _, err := applyUsageMutationResult(database, fixture, 0, failed); !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("apply unfinished failure error = %v", err)
	}
	assertUsageFactCount(t, database, fixture.site.ID, hour, 0)
	var windowCount int64
	if err := database.GORM.Model(&CollectionWindow{}).
		Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).Count(&windowCount).Error; err != nil {
		t.Fatalf("count unfinished collection windows: %v", err)
	}
	if windowCount != 0 {
		t.Fatalf("unfinished collection window count = %d", windowCount)
	}
	if _, err := FindUsageCursor(context.Background(), database.GORM, fixture.site.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unfinished usage cursor error = %v", err)
	}
}

func TestFailedUsageWindowPersistsSpecificReasonWithoutPermanentUnavailable(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_980_000)
	hour := now - now%3600 - 3600
	fixture := createUsageMutationFixture(t, database, hour, 1, constant.TaskTypeUsageBackfill, now)
	reasonParams := []byte(fmt.Sprintf(
		`{"site_id":"%d","start_timestamp":%d,"end_timestamp":%d}`,
		fixture.site.ID, hour, hour+3600,
	))
	request := fixture.failedRequest(0, now+1, false)
	request.ReasonCode = string(constant.MessageDataUpstreamUnavailable)
	request.ReasonParams = reasonParams
	mutation, err := NewFailedUsageWindowMutation(request)
	if err != nil {
		t.Fatalf("plan specific usage failure: %v", err)
	}
	result := applyUsageMutation(t, database, fixture, 0, mutation)
	if result.CollectionStatus != CollectionWindowStatusMissing ||
		result.ReasonCode != string(constant.MessageDataUpstreamUnavailable) {
		t.Fatalf("specific usage failure result = %#v", result)
	}
	var window CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&window).Error; err != nil {
		t.Fatalf("read specific usage failure window: %v", err)
	}
	if window.Status != CollectionWindowStatusMissing ||
		window.LastErrorCode != string(constant.MessageDataUpstreamUnavailable) {
		t.Fatalf("specific usage failure window = %#v", window)
	}
	var persisted struct {
		SiteID         string `json:"site_id"`
		StartTimestamp int64  `json:"start_timestamp"`
		EndTimestamp   int64  `json:"end_timestamp"`
	}
	if err := json.Unmarshal(window.LastErrorParams, &persisted); err != nil {
		t.Fatalf("decode specific usage failure params: %v", err)
	}
	if persisted.SiteID != fmt.Sprintf("%d", fixture.site.ID) ||
		persisted.StartTimestamp != hour || persisted.EndTimestamp != hour+3600 {
		t.Fatalf("specific usage failure params = %#v", persisted)
	}
}

func TestUsageFactsRejectMetricOverflow(t *testing.T) {
	now := int64(1_752_970_000)
	hour := now - now%3600 - 3600
	_, _, err := NewCompleteUsageWindowMutation(CompleteUsageWindowRequest{
		RunID: 1, WindowID: 1, SiteID: 1, ExpectedConfigVersion: 1,
		HourTS: hour, AttemptCount: 1, RequestID: "req_usage_overflow", Now: now, FetchedRows: 2,
		Facts: []UsageFactInput{
			{RemoteUserID: 1, ModelName: "m", RequestCount: math.MaxInt64},
			{RemoteUserID: 1, ModelName: "m", RequestCount: 1},
		},
	})
	if !errors.Is(err, ErrCollectionRunContract) {
		t.Fatalf("overflow error = %v", err)
	}
}

type usageMutationFixture struct {
	site    Site
	run     CollectionRun
	windows []CollectionRunWindow
	hours   []int64
}

func createUsageMutationFixture(
	t *testing.T,
	database *Database,
	start int64,
	windowCount int,
	taskType string,
	now int64,
) usageMutationFixture {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	statisticsStart := start
	rootID := int64(1)
	site := Site{
		Name: "Run run-usage-" + suffix, BaseURL: "https://usage-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true,
		RootUserID: &rootID, StatisticsStartAt: &statisticsStart, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create usage site: %v", err)
	}
	end := start + int64(windowCount)*3600
	initialized := now
	run := CollectionRun{
		SiteID: &site.ID, SiteConfigVersion: 1, TaskType: taskType, TargetType: "site", TargetID: site.ID,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: &start, EndTimestamp: &end,
		Scope: []byte("{}"), Status: CollectionTaskStatusRunning, Priority: constant.CollectionPriorityManualBackfill,
		NextAttemptAt: now, WindowsInitializedAt: &initialized, TotalWindows: windowCount,
		CreatedRequestID: "req_usage_fixture", LastRequestID: "req_usage_fixture",
		StartedAt: &now, HeartbeatAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&run).Error; err != nil {
		t.Fatalf("create usage run: %v", err)
	}
	windows := make([]CollectionRunWindow, windowCount)
	hours := make([]int64, windowCount)
	for index := range windows {
		hours[index] = start + int64(index)*3600
		windows[index] = CollectionRunWindow{
			RunID: run.ID, SiteID: site.ID, HourTS: hours[index], Status: CollectionTaskStatusRunning,
			AttemptCount: 1, StartedAt: &now, UpdatedAt: now,
		}
	}
	if err := database.GORM.Create(&windows).Error; err != nil {
		t.Fatalf("create usage run windows: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = database.GORM.WithContext(ctx).Where("site_id = ?", site.ID).Delete(&UsageFactHourly{}).Error
		_ = database.GORM.WithContext(ctx).Where("site_id = ?", site.ID).Delete(&CollectionCursor{}).Error
		_ = database.GORM.WithContext(ctx).Where("site_id = ?", site.ID).Delete(&CollectionWindow{}).Error
		_ = database.GORM.WithContext(ctx).Where("run_id = ?", run.ID).Delete(&CollectionRunWindow{}).Error
		_ = database.GORM.WithContext(ctx).Delete(&CollectionRun{}, run.ID).Error
		_ = database.GORM.WithContext(ctx).Delete(&Site{}, site.ID).Error
	})
	return usageMutationFixture{site: site, run: run, windows: windows, hours: hours}
}

func (fixture usageMutationFixture) completeRequest(index int, now int64, facts []UsageFactInput) CompleteUsageWindowRequest {
	return CompleteUsageWindowRequest{
		RunID: fixture.run.ID, WindowID: fixture.windows[index].ID, SiteID: fixture.site.ID,
		ExpectedConfigVersion: fixture.run.SiteConfigVersion, HourTS: fixture.hours[index],
		AttemptCount: fixture.windows[index].AttemptCount, RequestID: fixture.run.LastRequestID,
		Now: now, FetchedRows: int64(len(facts)), Facts: facts,
	}
}

func (fixture usageMutationFixture) failedRequest(index int, now int64, mismatch bool) FailedUsageWindowRequest {
	return FailedUsageWindowRequest{
		RunID: fixture.run.ID, WindowID: fixture.windows[index].ID, SiteID: fixture.site.ID,
		ExpectedConfigVersion: fixture.run.SiteConfigVersion, HourTS: fixture.hours[index],
		AttemptCount: fixture.windows[index].AttemptCount, RequestID: fixture.run.LastRequestID,
		Now: now, FetchedRows: 2, ReasonCode: string(constant.MessageDataValidationMismatch),
		ReasonParams: []byte(`{"site_id":"1"}`), DataMismatch: mismatch,
	}
}

func applyUsageMutation(
	t *testing.T,
	database *Database,
	fixture usageMutationFixture,
	index int,
	mutation UsageFactMutation,
) UsageWindowMutationResult {
	t.Helper()
	result, err := applyUsageMutationResult(database, fixture, index, mutation)
	if err != nil {
		t.Fatalf("apply usage mutation hour %d: %v", index, err)
	}
	return result
}

func applyUsageMutationResult(
	database *Database,
	fixture usageMutationFixture,
	index int,
	mutation UsageFactMutation,
) (UsageWindowMutationResult, error) {
	return applyUsageMutationResultWithDeclaredScope(database, fixture, index, mutation, nil)
}

func applyUsageMutationResultWithDeclaredScope(
	database *Database,
	fixture usageMutationFixture,
	index int,
	mutation UsageFactMutation,
	declaredScope *UsageWindowMutationScope,
) (UsageWindowMutationResult, error) {
	var result UsageWindowMutationResult
	err := database.GORM.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, fixture.site.ID).Error; err != nil {
			return err
		}
		var run CollectionRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, fixture.run.ID).Error; err != nil {
			return err
		}
		var window CollectionRunWindow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&window, fixture.windows[index].ID).Error; err != nil {
			return err
		}
		scope := UsageWindowMutationScope{Site: site, Run: run, Window: window}
		if declaredScope != nil {
			scope = *declaredScope
		}
		var err error
		result, err = mutation.apply(context.Background(), tx, scope)
		return err
	})
	return result, err
}

func assertUsageCursor(t *testing.T, database *Database, siteID int64, expected *int64) {
	t.Helper()
	cursor, err := FindUsageCursor(context.Background(), database.GORM, siteID)
	if err != nil {
		t.Fatalf("find usage cursor: %v", err)
	}
	if (cursor.LastCompleteHour == nil) != (expected == nil) ||
		(cursor.LastCompleteHour != nil && *cursor.LastCompleteHour != *expected) {
		t.Fatalf("usage cursor = %#v, want %v", cursor.LastCompleteHour, expected)
	}
}

func assertUsageFactCount(t *testing.T, database *Database, siteID, hourTS, expected int64) {
	t.Helper()
	var count int64
	if err := database.GORM.Model(&UsageFactHourly{}).Where("site_id = ? AND hour_ts = ?", siteID, hourTS).Count(&count).Error; err != nil {
		t.Fatalf("count usage facts: %v", err)
	}
	if count != expected {
		t.Fatalf("usage fact count = %d, want %d", count, expected)
	}
}

func assertUsageWindowStatus(t *testing.T, database *Database, siteID, hourTS int64, expected string) {
	t.Helper()
	var window CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", siteID, hourTS).First(&window).Error; err != nil {
		t.Fatalf("find collection window: %v", err)
	}
	if window.Status != expected {
		t.Fatalf("collection window status = %s, want %s", window.Status, expected)
	}
}
