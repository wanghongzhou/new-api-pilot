package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type alertSnapshotReaderFunc func(context.Context) (model.AlertEvaluationSnapshot, error)

func (function alertSnapshotReaderFunc) LoadSnapshot(ctx context.Context) (model.AlertEvaluationSnapshot, error) {
	return function(ctx)
}

type alertEvaluatorFunc func(context.Context, AlertEvaluation) (AlertEvaluationResult, error)

func (function alertEvaluatorFunc) Evaluate(ctx context.Context, evaluation AlertEvaluation) (AlertEvaluationResult, error) {
	return function(ctx, evaluation)
}

func TestAlertEvaluationScannerCoversEveryBuiltInRule(t *testing.T) {
	now := int64(1_752_400_800)
	lastProbe, resourceAt, lastSeen, lastSynced := now, now, now-100, now
	instanceCount := 0
	sampleStatus := "offline"
	cpu, memory, disk := "96.0000", "90.0000", "84.0000"
	statisticsStart := now - 24*3600
	hour := now - now%3600 - 3600
	snapshot := model.AlertEvaluationSnapshot{
		Sites: []model.AlertSiteEvaluationSnapshot{{
			ID: 1, Name: "矩阵站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, Version: "v-test", DataExportEnabled: true,
			ProbeFailCount: 3, LastProbeAt: &lastProbe, StatisticsStartAt: &statisticsStart,
			ResourceSampleAt: &resourceAt, ResourceInstanceCount: &instanceCount,
		}},
		Instances: []model.AlertInstanceEvaluationSnapshot{{
			SiteID: 1, SiteName: "矩阵站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, NodeName: "node-1", CurrentStatus: "offline",
			LastSeenAt: &lastSeen, LastSyncedAt: lastSynced, SampledAt: &resourceAt,
			SampleStatus: &sampleStatus,
			CPUPercent:   &cpu, MemoryPercent: &memory, DiskUsedPercent: &disk,
		}},
		Accounts: []model.AlertAccountEvaluationSnapshot{{
			ID: 2, SiteID: 1, Username: "managed-user", ManagedStatus: model.AccountManagedStatusActive,
			RemoteState: model.AccountRemoteStateNormal, RemoteStatus: 0, Quota: 0, LastSyncedAt: &lastSynced,
			CustomerStatus: dto.CustomerStatusUsing, SiteManagement: constant.SiteManagementActive,
			SiteAuthStatus: constant.SiteAuthAuthorized,
		}},
		CollectionWindows: []model.AlertCollectionEvaluationSnapshot{{
			SiteID: 1, SiteName: "矩阵站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, StatisticsStartAt: &statisticsStart,
			HourTS: hour, Status: model.CollectionWindowStatusMissing,
			LastErrorCode: string(constant.MessageDataValidationMismatch),
		}},
		Backfills: []model.AlertBackfillEvaluationSnapshot{{
			RunID: 3, SiteID: 1, SiteName: "矩阵站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true,
			Status: model.CollectionTaskStatusFailed, HasWindows: true,
		}},
	}
	seen := map[string]AlertEvaluation{}
	scanner, err := NewAlertEvaluationScanner(AlertEvaluationScannerOptions{
		Reader: alertSnapshotReaderFunc(func(context.Context) (model.AlertEvaluationSnapshot, error) {
			return snapshot, nil
		}),
		Evaluator: alertEvaluatorFunc(func(_ context.Context, evaluation AlertEvaluation) (AlertEvaluationResult, error) {
			if _, duplicate := seen[evaluation.RuleKey]; duplicate {
				t.Fatalf("duplicate rule evaluation %s", evaluation.RuleKey)
			}
			seen[evaluation.RuleKey] = evaluation
			return AlertEvaluationResult{Transition: "unchanged"}, nil
		}),
		Clock:              testsupport.NewFakeClock(time.Unix(now, 0)),
		RequestIDGenerator: func() (string, error) { return "als_matrix", nil },
	})
	if err != nil {
		t.Fatalf("create scanner: %v", err)
	}
	result, err := scanner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	expected := []string{
		"site_offline", "site_auth_expired", "site_export_disabled",
		"collection_missing", "backfill_failed", "validation_failed",
		"instance_stale", "instance_offline", "site_no_instance",
		"cpu_high", "memory_high", "disk_high",
		"account_missing", "account_identity_mismatch", "account_disabled", "account_quota_empty",
	}
	if result.EvaluationCount != len(expected) || len(seen) != len(expected) {
		t.Fatalf("evaluation count = %#v seen=%d", result, len(seen))
	}
	for _, ruleKey := range expected {
		evaluation, exists := seen[ruleKey]
		if !exists {
			t.Errorf("rule %s was not evaluated", ruleKey)
			continue
		}
		if evaluation.RequestID != "als_matrix" {
			t.Errorf("rule %s request ID = %q", ruleKey, evaluation.RequestID)
		}
	}
	assertScannerValue(t, seen["site_offline"], AlertSampleKnown, "3")
	assertScannerValue(t, seen["site_no_instance"], AlertSampleKnown, "0")
	assertScannerValue(t, seen["instance_stale"], AlertSampleKnown, "100")
	assertScannerValue(t, seen["instance_offline"], AlertSampleKnown, "0")
	assertScannerValue(t, seen["account_disabled"], AlertSampleKnown, "0")
	assertScannerValue(t, seen["account_quota_empty"], AlertSampleKnown, "0")
	if validation := seen["validation_failed"]; validation.Source != "data_mismatch" {
		t.Fatalf("validation source = %q", validation.Source)
	}
}

func TestAlertEvaluationScannerUsesUnknownInsteadOfInventingHealth(t *testing.T) {
	now := int64(1_752_400_800)
	oldSample := now - 600
	instanceCount := 1
	metric := "10"
	hour := now - now%3600 - 3600
	snapshot := model.AlertEvaluationSnapshot{
		Sites: []model.AlertSiteEvaluationSnapshot{{
			ID: 1, Name: "未知站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, Version: "", DataExportEnabled: true,
			ResourceSampleAt: &oldSample, ResourceInstanceCount: &instanceCount,
		}},
		Instances: []model.AlertInstanceEvaluationSnapshot{{
			SiteID: 1, SiteName: "未知站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, NodeName: "node-unknown", CurrentStatus: "online",
			SampledAt: &oldSample, CPUPercent: &metric, MemoryPercent: &metric, DiskUsedPercent: &metric,
		}},
		Accounts: []model.AlertAccountEvaluationSnapshot{{
			ID: 2, SiteID: 1, Username: "unknown-account", ManagedStatus: model.AccountManagedStatusActive,
			RemoteState: model.AccountRemoteStateNormal, RemoteStatus: 1, CustomerStatus: dto.CustomerStatusUsing,
			SiteManagement: constant.SiteManagementActive, SiteAuthStatus: constant.SiteAuthAuthorized,
		}},
		CollectionWindows: []model.AlertCollectionEvaluationSnapshot{{
			SiteID: 1, SiteName: "未知站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, HourTS: hour,
			Status: model.CollectionWindowStatusPending,
		}},
	}
	evaluations, err := buildAlertEvaluations(snapshot, now, 2*time.Minute, "als_unknown")
	if err != nil {
		t.Fatalf("build unknown evaluations: %v", err)
	}
	byRule := map[string][]AlertEvaluation{}
	for _, evaluation := range evaluations {
		byRule[evaluation.RuleKey] = append(byRule[evaluation.RuleKey], evaluation)
	}
	for _, ruleKey := range []string{
		"site_offline", "site_no_instance",
		"instance_stale", "instance_offline", "cpu_high", "memory_high", "disk_high",
		"account_missing", "account_identity_mismatch", "account_disabled", "account_quota_empty",
		"collection_missing", "validation_failed",
	} {
		values := byRule[ruleKey]
		if len(values) != 1 || values[0].State != AlertSampleUnknown || values[0].CurrentValue != nil {
			t.Errorf("rule %s evaluations = %#v", ruleKey, values)
		}
	}
}

func TestAlertEvaluationScannerMarksInapplicableScopes(t *testing.T) {
	now := int64(1_752_400_800)
	disabledAt := now - 60
	hour := now - now%3600 - 3600
	snapshot := model.AlertEvaluationSnapshot{
		Sites: []model.AlertSiteEvaluationSnapshot{{
			ID: 1, Name: "停用站点", ManagementStatus: constant.SiteManagementDisabled,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsEndAt: &disabledAt,
		}},
		Instances: []model.AlertInstanceEvaluationSnapshot{{
			SiteID: 1, SiteName: "停用站点", ManagementStatus: constant.SiteManagementDisabled,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsEndAt: &disabledAt, NodeName: "node-1",
		}},
		Accounts: []model.AlertAccountEvaluationSnapshot{{
			ID: 2, SiteID: 1, Username: "archived", ManagedStatus: model.AccountManagedStatusArchived,
			CustomerStatus: dto.CustomerStatusUsing,
		}},
		CollectionWindows: []model.AlertCollectionEvaluationSnapshot{{
			SiteID: 1, SiteName: "停用站点", ManagementStatus: constant.SiteManagementDisabled,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, StatisticsEndAt: &disabledAt,
			HourTS: hour, Status: model.CollectionWindowStatusMissing,
		}},
		Backfills: []model.AlertBackfillEvaluationSnapshot{{
			RunID: 3, SiteID: 1, SiteName: "停用站点", ManagementStatus: constant.SiteManagementDisabled,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, StatisticsEndAt: &disabledAt,
			Status: model.CollectionTaskStatusFailed,
		}},
	}
	evaluations, err := buildAlertEvaluations(snapshot, now, 2*time.Minute, "als_inactive")
	if err != nil {
		t.Fatalf("build inactive evaluations: %v", err)
	}
	for _, evaluation := range evaluations {
		switch evaluation.RuleKey {
		case "site_auth_expired":
			continue
		}
		if evaluation.State != AlertSampleScopeInactive || evaluation.ScopeID <= 0 || evaluation.CurrentValue != nil {
			t.Errorf("inapplicable evaluation = %#v", evaluation)
		}
	}
}

func TestValidationEvaluationDistinguishesExecutionFailureAndRecovery(t *testing.T) {
	siteID, hour := int64(1), int64(1_752_398_400)
	factComplete := model.CollectionWindowStatusComplete
	failure, err := validationFailedEvaluation(validationEvaluationTarget{
		validation: &model.AlertValidationEvaluationSnapshot{
			SiteID: siteID, SiteName: "站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, HourTS: hour,
			Status: model.CollectionTaskStatusFailed, FactStatus: &factComplete,
		},
	}, hour+3600, "als_validation")
	if err != nil {
		t.Fatalf("build validation failure: %v", err)
	}
	assertScannerValue(t, failure, AlertSampleKnown, "1")
	if failure.Source != "execution_failed" {
		t.Fatalf("failure source = %q", failure.Source)
	}
	recovery, err := validationFailedEvaluation(validationEvaluationTarget{
		validation: &model.AlertValidationEvaluationSnapshot{
			SiteID: siteID, SiteName: "站点", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, HourTS: hour,
			Status: model.CollectionTaskStatusSuccess, FactStatus: &factComplete,
		},
	}, hour+3600, "als_validation")
	if err != nil {
		t.Fatalf("build validation recovery: %v", err)
	}
	assertScannerValue(t, recovery, AlertSampleKnown, "0")
}

func TestFenceTerminatedRunsResolveFailureAlertsWithoutRefiring(t *testing.T) {
	siteID, hour := int64(1), int64(1_752_398_400)
	factComplete := model.CollectionWindowStatusComplete
	validation, err := validationFailedEvaluation(validationEvaluationTarget{
		validation: &model.AlertValidationEvaluationSnapshot{
			SiteID: siteID, SiteName: "fenced-site", ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, DataExportEnabled: true, HourTS: hour,
			Status: model.CollectionTaskStatusFailed, ErrorCode: constant.CodeSiteConfigChanged,
			FactStatus: &factComplete, UpdatedAt: hour + 3600,
		},
	}, hour+3600, "als_fenced_validation")
	if err != nil {
		t.Fatalf("build fenced validation evaluation: %v", err)
	}
	assertScannerValue(t, validation, AlertSampleKnown, "0")

	backfill, err := backfillFailedEvaluation(model.AlertBackfillEvaluationSnapshot{
		RunID: 9, SiteID: siteID, SiteName: "fenced-site",
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		DataExportEnabled: true, Status: model.CollectionTaskStatusFailed,
		ErrorCode: constant.CodeSiteConfigChanged, UpdatedAt: hour + 3600,
	}, hour+3600, "als_fenced_backfill")
	if err != nil {
		t.Fatalf("build fenced backfill evaluation: %v", err)
	}
	assertScannerValue(t, backfill, AlertSampleKnown, "0")
}

func TestAlertScannerCountsEachNonOnlineRawSampleExactlyOnce(t *testing.T) {
	tx := openAlertTestTransaction(t)
	minute := int64(1_752_400_800)
	minute -= minute % 60
	now := minute + 30
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	site := newAlertTestSite(now, "https://b6b-offline-raw-sample.example")
	site.AuthStatus, site.Version, site.DataExportEnabled = constant.SiteAuthAuthorized, "v-test", true
	site.LastProbeAt = &now
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create raw-sample site: %v", err)
	}
	lastSeen := minute - 10
	instance := model.SiteInstance{
		SiteID: site.ID, NodeName: "node-raw-sample", CurrentStatus: "online",
		FirstSeenAt: minute - 60, LastSeenAt: &lastSeen, LastSyncedAt: minute,
		CreatedAt: minute, UpdatedAt: minute,
	}
	if err := tx.Create(&instance).Error; err != nil {
		t.Fatalf("create raw-sample instance: %v", err)
	}
	insertSample := func(sampleMinute int64) {
		t.Helper()
		if err := tx.Create(&model.SiteInstanceStatusMinutely{
			SiteID: site.ID, NodeName: instance.NodeName, MinuteTS: sampleMinute,
			Status: "degraded", LastSeenAt: &lastSeen, CreatedAt: sampleMinute,
		}).Error; err != nil {
			t.Fatalf("create non-online raw sample %d: %v", sampleMinute, err)
		}
		if err := tx.Create(&model.SiteStatusMinutely{
			SiteID: site.ID, MinuteTS: sampleMinute, InstanceCount: 1, OnlineInstanceCount: 0,
			HealthStatus: constant.SiteHealthWarning, CreatedAt: sampleMinute,
		}).Error; err != nil {
			t.Fatalf("create site raw sample %d: %v", sampleMinute, err)
		}
	}
	insertSample(minute)
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create raw-sample alert service: %v", err)
	}
	scanner, err := NewAlertEvaluationScanner(AlertEvaluationScannerOptions{
		Database: tx, Evaluator: alerts, Clock: clock,
		RequestIDGenerator: func() (string, error) { return "als_raw_sample", nil },
	})
	if err != nil {
		t.Fatalf("create raw-sample scanner: %v", err)
	}
	if _, err := scanner.RunOnce(context.Background()); err != nil {
		t.Fatalf("scan first raw sample: %v", err)
	}
	target := strconv.FormatInt(site.ID, 10) + "/" + instance.NodeName
	assertOfflineEvent := func(status string, consecutive int) {
		t.Helper()
		var event model.AlertEvent
		err := tx.Where("rule_key = 'instance_offline' AND target_key = ? AND active_key IS NOT NULL", target).Take(&event).Error
		if err != nil || event.Status != status || event.ConsecutiveCount != consecutive {
			t.Fatalf("offline event = %#v, %v; want status=%s consecutive=%d", event, err, status, consecutive)
		}
	}
	assertOfflineEvent(dto.AlertStatusPending, 1)
	clock.Advance(10 * time.Second)
	if _, err := scanner.RunOnce(context.Background()); err != nil {
		t.Fatalf("rescan same raw sample: %v", err)
	}
	assertOfflineEvent(dto.AlertStatusPending, 1)
	for sampleNumber := 2; sampleNumber <= 3; sampleNumber++ {
		clock.Advance(50 * time.Second)
		sampleMinute := minute + int64(sampleNumber-1)*60
		insertSample(sampleMinute)
		if _, err := scanner.RunOnce(context.Background()); err != nil {
			t.Fatalf("scan raw sample %d: %v", sampleNumber, err)
		}
		status := dto.AlertStatusPending
		if sampleNumber == 3 {
			status = dto.AlertStatusFiring
		}
		assertOfflineEvent(status, sampleNumber)
	}
	var firingEvent model.AlertEvent
	if err := tx.Where("rule_key = 'instance_offline' AND target_key = ? AND active_key IS NOT NULL", target).
		Take(&firingEvent).Error; err != nil {
		t.Fatalf("load firing event before lifecycle transition: %v", err)
	}
	lifecycleAt := minute + 180
	if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"management_status": constant.SiteManagementDisabled, "updated_at": lifecycleAt}).Error; err != nil {
		t.Fatalf("disable raw-sample site: %v", err)
	}
	clock.Advance(40 * time.Second)
	if _, err := scanner.RunOnce(context.Background()); err != nil {
		t.Fatalf("scan lifecycle transition: %v", err)
	}
	var resolved model.AlertEvent
	if err := tx.First(&resolved, firingEvent.ID).Error; err != nil || resolved.Status != dto.AlertStatusResolved || resolved.ActiveKey != nil ||
		resolved.ResolutionReason == nil || *resolved.ResolutionReason != alertResolutionRetired {
		t.Fatalf("lifecycle-resolved event = %#v, %v", resolved, err)
	}
	activeKey := alertActiveKey("instance_offline", "instance", target)
	var cursor model.AlertEvaluationCursor
	if err := tx.Where("active_key = ?", activeKey).Take(&cursor).Error; err != nil || cursor.LastSampleAt != lifecycleAt ||
		!strings.HasPrefix(cursor.LastSampleKey, "v1:lifecycle:") {
		t.Fatalf("lifecycle cursor = %#v, %v", cursor, err)
	}
}

func TestRetiredInstanceEvaluationsEndEveryInstanceRule(t *testing.T) {
	now := int64(1_752_400_800)
	retiredAt := now - 60
	evaluations, err := instanceAlertEvaluations(model.AlertInstanceEvaluationSnapshot{
		ID: 44, SiteID: 9, SiteName: "retired-site", NodeName: "node-retired", RetiredAt: &retiredAt,
	}, now, time.Minute, "als_retired")
	if err != nil {
		t.Fatalf("build retired instance evaluations: %v", err)
	}
	if len(evaluations) != 5 {
		t.Fatalf("retired instance evaluations = %d, want 5", len(evaluations))
	}
	for _, evaluation := range evaluations {
		if evaluation.State != AlertSampleScopeInactive || evaluation.ScopeType != "instance" || evaluation.ScopeID != 44 ||
			evaluation.ObservedAt != retiredAt {
			t.Fatalf("retired instance evaluation = %#v", evaluation)
		}
	}
}

func assertScannerValue(t *testing.T, evaluation AlertEvaluation, state AlertSampleState, value string) {
	t.Helper()
	if evaluation.State != state || evaluation.CurrentValue == nil || *evaluation.CurrentValue != value {
		t.Fatalf("evaluation = %#v, want state=%s value=%s", evaluation, state, value)
	}
}
