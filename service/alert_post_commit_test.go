package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type postCommitSnapshotReader struct {
	snapshot          model.AlertEvaluationSnapshot
	calls             []AlertSampleSource
	contexts          []context.Context
	channelSiteID     int64
	channelHourTS     int64
	channelObservedAt int64
}

func (reader *postCommitSnapshotReader) record(ctx context.Context, source AlertSampleSource) (model.AlertEvaluationSnapshot, error) {
	reader.calls = append(reader.calls, source)
	reader.contexts = append(reader.contexts, ctx)
	return reader.snapshot, nil
}

func (reader *postCommitSnapshotReader) LoadProbeAlertSnapshot(ctx context.Context, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceProbe)
}

func (reader *postCommitSnapshotReader) LoadResourceAlertSnapshot(ctx context.Context, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceResource)
}

func (reader *postCommitSnapshotReader) LoadUserAlertSnapshot(ctx context.Context, _, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceUser)
}

func (reader *postCommitSnapshotReader) LoadChannelAlertSnapshot(ctx context.Context, siteID, hourTS, observedAt int64) (model.AlertEvaluationSnapshot, error) {
	reader.channelSiteID, reader.channelHourTS, reader.channelObservedAt = siteID, hourTS, observedAt
	return reader.record(ctx, AlertSampleSourceChannel)
}

func (reader *postCommitSnapshotReader) LoadWindowAlertSnapshot(ctx context.Context, _ string, _, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceWindow)
}

func (reader *postCommitSnapshotReader) LoadAuthAlertSnapshot(ctx context.Context, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceAuth)
}

func (reader *postCommitSnapshotReader) LoadLifecycleAlertSnapshot(ctx context.Context, _ string, _, _ int64) (model.AlertEvaluationSnapshot, error) {
	return reader.record(ctx, AlertSampleSourceLifecycle)
}

type postCommitBatchHookFunc func(context.Context, []AlertEvaluation) ([]AlertEvaluationResult, error)

func (function postCommitBatchHookFunc) EvaluateBatchAfterCommit(
	ctx context.Context,
	evaluations []AlertEvaluation,
) ([]AlertEvaluationResult, error) {
	return function(ctx, evaluations)
}

func TestAlertPostCommitCoordinatorRoutesExactChannelSnapshot(t *testing.T) {
	now := int64(1_752_400_800)
	hour := now - now%3600
	count, status, balance, response, availability := int64(2), "complete", "90", "1500", "0.95"
	reader := &postCommitSnapshotReader{snapshot: model.AlertEvaluationSnapshot{
		Channels: []model.AlertChannelEvaluationSnapshot{{
			SiteID: 9, SiteName: "Channel Hook", SiteConfigVersion: 1, ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, SiteUpdatedAt: now,
			HourTS: &hour, CollectedAt: &now, ChannelCount: &count, DataStatus: &status, ConfigVersion: intPointer(1), BalanceTotal: &balance,
			ResponseTimeAvgMS: &response, AvailabilityRate: &availability,
		}},
	}}
	var received []AlertEvaluation
	coordinator, err := NewAlertPostCommitCoordinator(AlertPostCommitCoordinatorOptions{
		Reader: reader, Clock: testsupport.NewFakeClock(time.Unix(now, 0)),
		RequestIDGenerator: func() (string, error) { return "req_channel_hook", nil },
		Hook: postCommitBatchHookFunc(func(_ context.Context, evaluations []AlertEvaluation) ([]AlertEvaluationResult, error) {
			received = append(received, evaluations...)
			return make([]AlertEvaluationResult, len(evaluations)), nil
		}),
	})
	if err != nil {
		t.Fatalf("create channel post-commit coordinator: %v", err)
	}
	coordinator.NotifyAfterCommit(context.Background(), AlertPostCommitTrigger{
		Source: AlertSampleSourceChannel, SiteID: 9, HourTS: hour, ObservedAt: now,
	})
	if len(reader.calls) != 1 || reader.calls[0] != AlertSampleSourceChannel ||
		reader.channelSiteID != 9 || reader.channelHourTS != hour || reader.channelObservedAt != now {
		t.Fatalf("channel snapshot read = calls:%#v site:%d hour:%d observed:%d", reader.calls, reader.channelSiteID, reader.channelHourTS, reader.channelObservedAt)
	}
	if len(received) != 3 {
		t.Fatalf("channel post-commit evaluations = %#v", received)
	}
	for _, evaluation := range received {
		if evaluation.State != AlertSampleKnown || evaluation.TargetType != "site" || evaluation.TargetKey != "9" ||
			!strings.HasPrefix(evaluation.RuleKey, "channel_") {
			t.Fatalf("channel post-commit evaluation = %#v", evaluation)
		}
	}
}

func TestAlertPostCommitCoordinatorUsesBoundedDetachedContextAndContainsHookFailure(t *testing.T) {
	now := int64(1_752_400_800)
	probeAt := now
	reader := &postCommitSnapshotReader{snapshot: model.AlertEvaluationSnapshot{
		Sites: []model.AlertSiteEvaluationSnapshot{{
			ID: 7, Name: "site-7", ConfigVersion: 3, ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, Version: "v-test", LastProbeAt: &probeAt, UpdatedAt: now,
		}},
	}}
	want := errors.New("injected hook failure")
	logs := []string{}
	coordinator, err := NewAlertPostCommitCoordinator(AlertPostCommitCoordinatorOptions{
		Reader: reader,
		Hook: postCommitBatchHookFunc(func(ctx context.Context, evaluations []AlertEvaluation) ([]AlertEvaluationResult, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("post-commit hook context has no deadline")
			}
			if ctx.Err() != nil {
				t.Fatalf("post-commit hook inherited request cancellation: %v", ctx.Err())
			}
			if len(evaluations) != 2 {
				t.Fatalf("probe evaluation count = %d", len(evaluations))
			}
			seen := map[string]bool{}
			for _, evaluation := range evaluations {
				seen[evaluation.RuleKey] = true
			}
			for _, ruleKey := range []string{"site_offline", "site_export_disabled"} {
				if !seen[ruleKey] {
					t.Fatalf("probe evaluations missing %q: %#v", ruleKey, evaluations)
				}
			}
			return nil, want
		}),
		Clock:              testsupport.NewFakeClock(time.Unix(now, 0)),
		RequestIDGenerator: func() (string, error) { return "pc_probe_7", nil },
		Logf:               func(format string, args ...any) { logs = append(logs, format) },
	})
	if err != nil {
		t.Fatalf("create post-commit coordinator: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	coordinator.NotifyAfterCommit(canceled, AlertPostCommitTrigger{
		Source: AlertSampleSourceProbe, SiteID: 7, ObservedAt: now,
	})
	if len(reader.calls) != 1 || reader.calls[0] != AlertSampleSourceProbe {
		t.Fatalf("reader calls = %#v", reader.calls)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "error_type=%T") || strings.Contains(logs[0], want.Error()) {
		t.Fatalf("failure logs = %#v", logs)
	}
}

func TestRapidResourceUpsertsShareCanonicalSampleAndCountOnce(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	site := newAlertTestSite(now, "https://post-commit-resource.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create resource site: %v", err)
	}
	if result := tx.Model(&model.AlertRule{}).
		Where("rule_key = 'cpu_high' AND level = ? AND scope_type = 'global'", dto.AlertLevelCritical).
		Update("for_times", 1); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("set resource rule cadence: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	minute := now - now%60
	lastSeen := now
	sampleID := int64(91)
	status := "online"
	cpu, zero := "96.0000", "0.0000"
	reader := &postCommitSnapshotReader{}
	reader.snapshot = resourcePostCommitSnapshot(site.ID, minute, sampleID, lastSeen, status, cpu, zero)
	hook, err := NewAlertPostCommitHook(alerts)
	if err != nil {
		t.Fatalf("create resource hook: %v", err)
	}
	coordinator, err := NewAlertPostCommitCoordinator(AlertPostCommitCoordinatorOptions{
		Reader: reader, Hook: hook, Clock: clock,
		RequestIDGenerator: func() (string, error) { return "pc_resource", nil },
		Logf:               func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("create resource coordinator: %v", err)
	}
	trigger := AlertPostCommitTrigger{Source: AlertSampleSourceResource, SiteID: site.ID, ObservedAt: minute}
	coordinator.NotifyAfterCommit(context.Background(), trigger)
	reader.snapshot = resourcePostCommitSnapshot(site.ID, minute, sampleID, lastSeen, status, "10.0000", zero)
	coordinator.NotifyAfterCommit(context.Background(), trigger)

	var event model.AlertEvent
	if err := tx.Where("rule_key = 'cpu_high' AND site_id = ?", site.ID).Take(&event).Error; err != nil {
		t.Fatalf("load resource event: %v", err)
	}
	if event.Status != dto.AlertStatusFiring || event.ConsecutiveCount != 1 ||
		event.CurrentValue == nil || *event.CurrentValue == "10.0000" {
		t.Fatalf("resource event after same-minute upsert = %#v", event)
	}
	var cursor model.AlertEvaluationCursor
	if event.ActiveKey == nil {
		t.Fatal("resource event has no active key")
	}
	if err := tx.Where("active_key = ?", *event.ActiveKey).Take(&cursor).Error; err != nil {
		t.Fatalf("load resource cursor: %v", err)
	}
	if cursor.LastSampleAt != minute || !strings.HasPrefix(cursor.LastSampleKey, "v1:resource:") {
		t.Fatalf("resource cursor = %#v", cursor)
	}
}

func resourcePostCommitSnapshot(
	siteID int64,
	minute int64,
	sampleID int64,
	lastSeen int64,
	status string,
	cpu string,
	zero string,
) model.AlertEvaluationSnapshot {
	return model.AlertEvaluationSnapshot{Instances: []model.AlertInstanceEvaluationSnapshot{{
		ID: 44, SiteID: siteID, SiteName: "resource-site", ManagementStatus: constant.SiteManagementActive,
		AuthStatus: constant.SiteAuthAuthorized, NodeName: "node-a", CurrentStatus: status,
		LastSeenAt: &lastSeen, LastSyncedAt: minute, SampledAt: &minute, SampleID: &sampleID,
		SampleStatus: &status, CPUPercent: &cpu, MemoryPercent: &zero, DiskUsedPercent: &zero,
		UpdatedAt: minute,
	}}}
}

func TestSameClockTickAuthSamplesUseMonotonicCommittedTimes(t *testing.T) {
	now := int64(1_752_400_800)
	firstCommitted := monotonicMutationTime(now, now)
	secondCommitted := monotonicMutationTime(now, firstCommitted)
	first, err := BuildAuthAlertSampleIdentity(9, firstCommitted, "config_version:2")
	if err != nil {
		t.Fatalf("build first auth identity: %v", err)
	}
	second, err := BuildAuthAlertSampleIdentity(9, secondCommitted, "config_version:3")
	if err != nil {
		t.Fatalf("build second auth identity: %v", err)
	}
	if first.ObservedAt != now+1 || second.ObservedAt != now+2 || first.SampleKey == second.SampleKey {
		t.Fatalf("same-tick auth identities first=%#v second=%#v", first, second)
	}
}

func TestAlertPostCommitRepositoryLoadsExactResourceMinute(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	oldMinute := now - now%60
	newMinute := oldMinute + 60
	site := newAlertTestSite(now, "https://post-commit-exact-resource.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create exact-resource site: %v", err)
	}
	oldCPU, newCPU := 10.0, 99.0
	lastSeen := now
	repository := model.NewSiteRepository(tx)
	write := func(minute int64, cpu *float64) {
		t.Helper()
		if err := repository.SyncInstances(context.Background(), []model.SiteInstanceWrite{{
			Instance: model.SiteInstance{
				SiteID: site.ID, NodeName: "node-exact", Hostname: "host-exact", CurrentStatus: "online",
				UpstreamStatus: "online", FirstSeenAt: now, LastSeenAt: &lastSeen,
				LastSyncedAt: minute, CreatedAt: now, UpdatedAt: minute,
			},
			Sample: model.SiteInstanceStatusMinutely{
				SiteID: site.ID, NodeName: "node-exact", MinuteTS: minute, Status: "online",
				CPUPercent: cpu, LastSeenAt: &lastSeen, CreatedAt: minute,
			},
		}}); err != nil {
			t.Fatalf("write exact-resource minute %d: %v", minute, err)
		}
		if err := repository.UpsertSiteStatusMinute(context.Background(), model.SiteStatusMinutely{
			SiteID: site.ID, MinuteTS: minute, InstanceCount: 1, OnlineInstanceCount: 1,
			CPUMaxPercent: cpu, CPUAvgPercent: cpu, HealthStatus: constant.SiteHealthOK, CreatedAt: minute,
		}); err != nil {
			t.Fatalf("write exact site minute %d: %v", minute, err)
		}
	}
	write(oldMinute, &oldCPU)
	write(newMinute, &newCPU)

	snapshot, err := model.NewAlertEvaluationRepository(tx).LoadResourceAlertSnapshot(
		context.Background(), site.ID, oldMinute,
	)
	if err != nil {
		t.Fatalf("load exact resource snapshot: %v", err)
	}
	if len(snapshot.Sites) != 1 || snapshot.Sites[0].ResourceSampleAt == nil ||
		*snapshot.Sites[0].ResourceSampleAt != oldMinute || len(snapshot.Instances) != 1 ||
		snapshot.Instances[0].SampledAt == nil || *snapshot.Instances[0].SampledAt != oldMinute ||
		snapshot.Instances[0].CPUPercent == nil || *snapshot.Instances[0].CPUPercent != "10.0000" {
		t.Fatalf("exact resource snapshot = %#v", snapshot)
	}
}

func TestCommittedBackfillLoadsRepairedActiveFailuresForImmediateRecovery(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	hour := now - now%3600 - 3600
	site := newAlertTestSite(now, "https://post-commit-backfill-recovery.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create backfill recovery site: %v", err)
	}
	start, end := hour, hour+3600
	finished := now
	oldRun := model.CollectionRun{
		SiteID: &site.ID, SiteConfigVersion: site.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TargetType: "site", TargetID: site.ID,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: &start, EndTimestamp: &end,
		Scope:  []byte(`{}`),
		Status: model.CollectionTaskStatusFailed, ErrorCode: constant.CodeUpstreamUnavailable,
		NextAttemptAt: now, WindowsInitializedAt: &now, TotalWindows: 1, FailedWindows: 1,
		CreatedRequestID: "req_old_backfill", LastRequestID: "req_old_backfill",
		FinishedAt: &finished, CreatedAt: now - 10, UpdatedAt: now - 10,
	}
	if err := tx.Create(&oldRun).Error; err != nil {
		t.Fatalf("create old failed backfill: %v", err)
	}
	oldWindow := model.CollectionRunWindow{
		RunID: oldRun.ID, SiteID: site.ID, HourTS: hour, Status: model.CollectionTaskStatusFailed,
		AttemptCount: 1, ErrorCode: constant.CodeUpstreamUnavailable, FinishedAt: &finished, UpdatedAt: now - 10,
	}
	if err := tx.Create(&oldWindow).Error; err != nil {
		t.Fatalf("create old backfill window: %v", err)
	}
	if err := tx.Create(&model.CollectionWindow{
		SiteID: site.ID, HourTS: hour, Status: model.CollectionWindowStatusComplete,
		SourceHash: strings.Repeat("a", 64), UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create repaired collection window: %v", err)
	}
	var rule model.AlertRule
	if err := tx.Where("rule_key = ?", "backfill_failed").First(&rule).Error; err != nil {
		t.Fatalf("load backfill rule: %v", err)
	}
	targetKey := strconv.FormatInt(site.ID, 10) + "/" + strconv.FormatInt(oldRun.ID, 10)
	activeKey := "test-backfill-recovery-" + targetKey
	value, threshold := "1", "0"
	if err := tx.Create(&model.AlertEvent{
		RuleID: rule.ID, RuleKey: rule.RuleKey, SiteID: &site.ID,
		TargetType: "collection", TargetKey: targetKey, ActiveKey: &activeKey,
		Level: rule.Level, Status: dto.AlertStatusFiring, ConsecutiveCount: 1,
		CurrentValue: &value, ThresholdValue: &threshold,
		MessageCode: string(constant.MessageAlertBackfillFailed), Message: "backfill failed",
		FirstObservedAt: now - 10, FirstFiredAt: &finished, LastFiredAt: &finished,
		CreatedAt: now - 10, UpdatedAt: now - 10,
	}).Error; err != nil {
		t.Fatalf("create active backfill event: %v", err)
	}
	newRun := model.CollectionRun{
		SiteID: &site.ID, SiteConfigVersion: site.ConfigVersion,
		TaskType: constant.TaskTypeUsageBackfill, TargetType: "site", TargetID: site.ID,
		TriggerType: constant.CollectionTriggerManual, StartTimestamp: &start, EndTimestamp: &end,
		Scope:  []byte(`{}`),
		Status: model.CollectionTaskStatusSuccess, NextAttemptAt: now,
		WindowsInitializedAt: &now, TotalWindows: 1, CompletedWindows: 1,
		CreatedRequestID: "req_new_backfill", LastRequestID: "req_new_backfill",
		FinishedAt: &finished, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&newRun).Error; err != nil {
		t.Fatalf("create successful repair backfill: %v", err)
	}

	snapshot, err := model.NewAlertEvaluationRepository(tx).LoadWindowAlertSnapshot(
		context.Background(), string(AlertWindowSourceBackfill), newRun.ID, 0, now,
	)
	if err != nil {
		t.Fatalf("load repair backfill snapshot: %v", err)
	}
	if len(snapshot.Backfills) != 2 {
		t.Fatalf("repair backfill snapshots = %#v", snapshot.Backfills)
	}
	var recovered *model.AlertBackfillEvaluationSnapshot
	for index := range snapshot.Backfills {
		if snapshot.Backfills[index].RunID == oldRun.ID {
			recovered = &snapshot.Backfills[index]
			break
		}
	}
	if recovered == nil || !recovered.HasWindows || !recovered.FactsRepaired || recovered.UpdatedAt != now {
		t.Fatalf("repaired active backfill snapshot = %#v", recovered)
	}
}

func TestSameClockTickCustomerDisableAndEnableIntentAdvanceLifecycleVersion(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	customer := model.Customer{
		Name: "same-tick-customer", Status: dto.CustomerStatusUsing,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	repository := model.NewCustomerRepository(tx)
	if err := repository.Create(context.Background(), &customer); err != nil {
		t.Fatalf("create same-tick customer: %v", err)
	}
	if err := repository.Disable(context.Background(), customer.ID, now-now%3600, now); err != nil {
		t.Fatalf("disable same-tick customer: %v", err)
	}
	disabled, err := repository.FindByID(context.Background(), customer.ID)
	if err != nil {
		t.Fatalf("load disabled same-tick customer: %v", err)
	}
	if err := repository.BeginEnable(context.Background(), customer.ID, now); err != nil {
		t.Fatalf("begin same-tick customer enable: %v", err)
	}
	enabling, err := repository.FindByID(context.Background(), customer.ID)
	if err != nil {
		t.Fatalf("load enabling same-tick customer: %v", err)
	}
	if disabled.UpdatedAt != now+1 || enabling.UpdatedAt != now+2 {
		t.Fatalf("same-tick lifecycle versions disabled=%d enabling=%d", disabled.UpdatedAt, enabling.UpdatedAt)
	}
	first, _ := BuildLifecycleAlertSampleIdentity("customer", customer.ID, disabled.UpdatedAt)
	second, _ := BuildLifecycleAlertSampleIdentity("customer", customer.ID, enabling.UpdatedAt)
	if first.ObservedAt >= second.ObservedAt {
		t.Fatalf("same-tick lifecycle identities first=%#v second=%#v", first, second)
	}
}
