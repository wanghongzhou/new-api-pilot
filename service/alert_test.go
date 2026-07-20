package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestAlertEvaluationStateMachineAndLevelSwitch(t *testing.T) {
	tx := openAlertTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	site := newAlertTestSite(clock.Now().Unix(), "https://b6-alert-state.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create state-machine site: %v", err)
	}
	ruleKey := "cpu_high"
	if result := tx.Model(&model.AlertRule{}).
		Where("rule_key = ? AND level = ? AND scope_type = 'global'", ruleKey, dto.AlertLevelCritical).
		Update("for_times", 1); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("set critical test cadence: rows=%d err=%v", result.RowsAffected, result.Error)
	}
	targetKey := strconv.FormatInt(site.ID, 10) + "/node-alpha"

	evaluate := func(state AlertSampleState, value string) AlertEvaluationResult {
		t.Helper()
		var current *string
		if state == AlertSampleKnown {
			current = &value
		}
		result, evaluationErr := alerts.Evaluate(context.Background(), AlertEvaluation{
			RuleKey: ruleKey, SiteID: &site.ID, TargetType: "instance", TargetKey: targetKey, TargetName: "node-alpha",
			State: state, CurrentValue: current, Message: alertCPUMessage(value),
			ScopeType: "site", ScopeID: site.ID, ScopeName: site.Name, Source: "resource_snapshot", RequestID: "req-b6-critical",
			ObservedAt: clock.Now().Unix(), SampleKey: "test:" + strconv.FormatInt(clock.Now().Unix(), 10),
		})
		if evaluationErr != nil {
			t.Fatalf("evaluate %s/%s: %v", state, value, evaluationErr)
		}
		clock.Advance(time.Minute)
		return result
	}

	first := evaluate(AlertSampleKnown, "90")
	if first.Status != dto.AlertStatusPending || first.Transition != "pending" {
		t.Fatalf("first sample = %#v", first)
	}
	unknown := evaluate(AlertSampleUnknown, "")
	if unknown.EventID != first.EventID || unknown.Status != dto.AlertStatusPending || unknown.Transition != "unknown" {
		t.Fatalf("unknown sample = %#v", unknown)
	}
	assertAlertEventState(t, tx, first.EventID, dto.AlertStatusPending, dto.AlertLevelWarning, 1)

	second := evaluate(AlertSampleKnown, "90")
	if second.EventID != first.EventID || second.Status != dto.AlertStatusPending {
		t.Fatalf("second sample = %#v", second)
	}
	third := evaluate(AlertSampleKnown, "90")
	if third.EventID != first.EventID || third.Status != dto.AlertStatusFiring || third.Transition != "firing" {
		t.Fatalf("third sample = %#v", third)
	}

	critical := evaluate(AlertSampleKnown, "96")
	if critical.EventID == first.EventID || critical.Level != dto.AlertLevelCritical || critical.Status != dto.AlertStatusFiring {
		t.Fatalf("critical switch = %#v", critical)
	}
	var evidence struct {
		CurrentValue   string
		ThresholdValue string
		Level          string
		MessageCode    string
		MessageParams  string
		Message        string
	}
	if err := tx.Raw(`SELECT CAST(current_value AS CHAR) AS current_value,
CAST(threshold_value AS CHAR) AS threshold_value, level, message_code,
CAST(message_params AS CHAR) AS message_params, message
FROM alert_event WHERE id = ?`, critical.EventID).Scan(&evidence).Error; err != nil {
		t.Fatalf("read critical evidence: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(evidence.MessageParams), &params); err != nil {
		t.Fatalf("decode critical message params: %v", err)
	}
	if evidence.CurrentValue != "96.0000000000" || evidence.ThresholdValue != "95.0000000000" ||
		evidence.Level != dto.AlertLevelCritical || evidence.MessageCode != string(constant.MessageAlertCPUHigh) ||
		params["value"] != evidence.CurrentValue || params["threshold"] != evidence.ThresholdValue ||
		params["target_type"] != "instance" || params["site_id"] != strconv.FormatInt(site.ID, 10) ||
		!strings.Contains(evidence.Message, "test evidence") || !strings.Contains(evidence.Message, "source=resource_snapshot") ||
		!strings.Contains(evidence.Message, "request_id=req-b6-critical") {
		t.Fatalf("critical evidence mismatch: row=%#v params=%#v", evidence, params)
	}
	assertAlertEventState(t, tx, first.EventID, dto.AlertStatusResolved, dto.AlertLevelWarning, 3)

	warningAgain := evaluate(AlertSampleKnown, "90")
	if warningAgain.EventID == critical.EventID || warningAgain.Level != dto.AlertLevelWarning || warningAgain.Status != dto.AlertStatusPending {
		t.Fatalf("critical downgrade = %#v", warningAgain)
	}
	assertAlertEventState(t, tx, critical.EventID, dto.AlertStatusResolved, dto.AlertLevelCritical, 1)
	assertAlertEventEvidence(t, tx, critical.EventID, dto.AlertStatusResolved, "90.0000000000", "95.0000000000", dto.AlertLevelCritical)
	assertAlertEventEvidence(t, tx, warningAgain.EventID, dto.AlertStatusPending, "90.0000000000", "85.0000000000", dto.AlertLevelWarning)

	healthy := evaluate(AlertSampleKnown, "10")
	if healthy.EventID != warningAgain.EventID || healthy.Status != dto.AlertStatusResolved {
		t.Fatalf("healthy recovery = %#v", healthy)
	}
	assertAlertEventEvidence(t, tx, warningAgain.EventID, dto.AlertStatusResolved, "10.0000000000", "85.0000000000", dto.AlertLevelWarning)
	reopened := evaluate(AlertSampleKnown, "90")
	if reopened.EventID == warningAgain.EventID || reopened.Status != dto.AlertStatusPending {
		t.Fatalf("reopened event = %#v", reopened)
	}
	inactive := evaluate(AlertSampleScopeInactive, "")
	if inactive.EventID != reopened.EventID || inactive.Status != dto.AlertStatusResolved {
		t.Fatalf("scope inactive = %#v", inactive)
	}
	var messageCode string
	if err := tx.Raw("SELECT message_code FROM alert_event WHERE id = ?", reopened.EventID).Scan(&messageCode).Error; err != nil || messageCode != string(constant.MessageAlertScopeInactive) {
		t.Fatalf("scope inactive message code = %q, %v", messageCode, err)
	}
	directCritical := evaluate(AlertSampleKnown, "96")
	if directCritical.Level != dto.AlertLevelCritical || directCritical.Status != dto.AlertStatusFiring {
		t.Fatalf("direct critical sample = %#v", directCritical)
	}
	directHealthy := evaluate(AlertSampleKnown, "10")
	if directHealthy.EventID != directCritical.EventID || directHealthy.Status != dto.AlertStatusResolved {
		t.Fatalf("direct critical recovery = %#v", directHealthy)
	}
	assertAlertEventEvidence(t, tx, directCritical.EventID, dto.AlertStatusResolved, "10.0000000000", "95.0000000000", dto.AlertLevelCritical)

	var activeCount int64
	if err := tx.Raw("SELECT COUNT(*) FROM alert_event WHERE rule_key = ? AND target_key = ? AND active_key IS NOT NULL", ruleKey, targetKey).Scan(&activeCount).Error; err != nil || activeCount != 0 {
		t.Fatalf("active event count = %d, %v", activeCount, err)
	}
}

func TestAlertRuleOverrideInheritanceValidationAndHistoryPreservation(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_401_000)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	site := newAlertTestSite(now, "https://b6-alert-rules.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	ruleKey := "b6_override_" + strconv.FormatInt(now, 10)
	warningID := insertAlertRule(t, tx, ruleKey, dto.AlertLevelWarning, "85", 3, now)
	insertAlertRule(t, tx, ruleKey, dto.AlertLevelCritical, "95", 1, now)

	threshold := "90"
	created, err := alerts.CreateOverride(context.Background(), dto.AlertRuleOverrideRequest{
		BaseRuleID: strconv.FormatInt(warningID, 10), SiteID: strconv.FormatInt(site.ID, 10), ThresholdValue: &threshold,
	})
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	if created.Inherited || created.OverrideRuleID == nil || created.ThresholdValue == nil || *created.ThresholdValue != "90.0000000000" {
		t.Fatalf("created override = %#v", created)
	}

	rules, err := alerts.ListRules(context.Background(), dto.AlertScopeSite, site.ID)
	if err != nil {
		t.Fatalf("list effective rules: %v", err)
	}
	var inheritedCritical bool
	for _, rule := range rules {
		if rule.RuleKey == ruleKey && rule.Level == dto.AlertLevelCritical {
			inheritedCritical = rule.Inherited
		}
	}
	if !inheritedCritical {
		t.Fatalf("site rules do not inherit critical pair: %#v", rules)
	}

	invalid := "96"
	_, err = alerts.UpdateRule(context.Background(), mustAlertID(t, created.ID), dto.AlertRuleUpdateRequest{ThresholdValue: &invalid})
	var validation *AlertValidationError
	if !errors.As(err, &validation) || validation.Fields["threshold_value"] == "" {
		t.Fatalf("pair validation error = %v", err)
	}

	overrideID := mustAlertID(t, created.ID)
	params := `{"scope_type":"account","scope_id":"42","scope_name":"history"}`
	resolvedAt := now
	historical := model.AlertEvent{
		RuleID: overrideID, RuleKey: ruleKey, SiteID: &site.ID, TargetType: "account", TargetKey: "42",
		Level: dto.AlertLevelWarning, Status: dto.AlertStatusResolved, ConsecutiveCount: 1,
		MessageCode: string(constant.MessageAlertScopeInactive), MessageParams: &params, Message: "history",
		FirstObservedAt: now, ResolvedAt: &resolvedAt, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&historical).Error; err != nil {
		t.Fatalf("create historical event: %v", err)
	}
	if err := alerts.DeleteOverride(context.Background(), overrideID); err != nil {
		t.Fatalf("delete override: %v", err)
	}

	rules, err = alerts.ListRules(context.Background(), dto.AlertScopeSite, site.ID)
	if err != nil {
		t.Fatalf("list inherited rules after delete: %v", err)
	}
	var fallback dto.AlertRuleItem
	for _, rule := range rules {
		if rule.RuleKey == ruleKey && rule.Level == dto.AlertLevelWarning {
			fallback = rule
		}
	}
	if !fallback.Inherited || fallback.OverrideRuleID != nil || fallback.EffectiveRuleID != strconv.FormatInt(warningID, 10) {
		t.Fatalf("override fallback = %#v", fallback)
	}
	var tombstone struct {
		RuleKey   string
		ScopeType string
		ScopeID   int64
	}
	if err := tx.Raw("SELECT rule_key, scope_type, scope_id FROM alert_rule WHERE id = ?", overrideID).Scan(&tombstone).Error; err != nil || tombstone.RuleKey != "__deleted_"+strconv.FormatInt(overrideID, 10) || tombstone.ScopeType != dto.AlertScopeSite || tombstone.ScopeID != site.ID {
		t.Fatalf("historical override tombstone = %#v, %v", tombstone, err)
	}
	var historyCount int64
	if err := tx.Raw("SELECT COUNT(*) FROM alert_event WHERE id = ? AND rule_id = ?", historical.ID, overrideID).Scan(&historyCount).Error; err != nil || historyCount != 1 {
		t.Fatalf("historical FK count = %d, %v", historyCount, err)
	}
}

func TestAlertConcurrentGlobalAndSiteUpdatesPreserveEffectivePair(t *testing.T) {
	database := openAlertConcurrentDatabase(t)
	now := time.Now().Unix()
	ruleKey := "b6_pair_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	warningID := insertAlertRule(t, database.GORM, ruleKey, dto.AlertLevelWarning, "85", 3, now)
	criticalID := insertAlertRule(t, database.GORM, ruleKey, dto.AlertLevelCritical, "95", 1, now)
	site := newAlertTestSite(now, "https://"+ruleKey+".example")
	if err := model.NewSiteRepository(database.GORM).Create(context.Background(), &site); err != nil {
		t.Fatalf("create pair-validation site: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_rule WHERE rule_key = ? AND scope_type = 'site'", ruleKey)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_rule WHERE rule_key = ? AND scope_type = 'global'", ruleKey)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	alerts, err := NewAlertService(AlertServiceOptions{Database: database.GORM, Clock: testsupport.NewFakeClock(time.Unix(now, 0))})
	if err != nil {
		t.Fatalf("create pair-validation alert service: %v", err)
	}
	initialCritical := "96"
	override, err := alerts.CreateOverride(context.Background(), dto.AlertRuleOverrideRequest{
		BaseRuleID: strconv.FormatInt(criticalID, 10), SiteID: strconv.FormatInt(site.ID, 10), ThresholdValue: &initialCritical,
	})
	if err != nil {
		t.Fatalf("create critical override: %v", err)
	}
	overrideID := mustAlertID(t, override.ID)

	type updateResult struct{ err error }
	results := make(chan updateResult, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		threshold := "94"
		_, updateErr := alerts.UpdateRule(context.Background(), warningID, dto.AlertRuleUpdateRequest{ThresholdValue: &threshold})
		results <- updateResult{err: updateErr}
	}()
	go func() {
		defer group.Done()
		<-start
		threshold := "90"
		_, updateErr := alerts.UpdateRule(context.Background(), overrideID, dto.AlertRuleUpdateRequest{ThresholdValue: &threshold})
		results <- updateResult{err: updateErr}
	}()
	close(start)
	group.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for result := range results {
		if result.err == nil {
			succeeded++
			continue
		}
		var validation *AlertValidationError
		if errors.As(result.err, &validation) && validation.Fields["threshold_value"] != "" {
			rejected++
			continue
		}
		t.Fatalf("unexpected concurrent pair update error: %v", result.err)
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent pair updates succeeded=%d rejected=%d", succeeded, rejected)
	}
	effective, err := alerts.ListRules(context.Background(), dto.AlertScopeSite, site.ID)
	if err != nil {
		t.Fatalf("list pair-validation rules: %v", err)
	}
	var warning, critical *big.Rat
	for _, rule := range effective {
		if rule.RuleKey != ruleKey || rule.ThresholdValue == nil {
			continue
		}
		value, _, valid := parseAlertDecimal(*rule.ThresholdValue, true)
		if !valid {
			t.Fatalf("invalid persisted pair threshold %q", *rule.ThresholdValue)
		}
		if rule.Level == dto.AlertLevelWarning {
			warning = value
		} else if rule.Level == dto.AlertLevelCritical {
			critical = value
		}
	}
	if warning == nil || critical == nil || warning.Cmp(critical) >= 0 {
		t.Fatalf("invalid effective pair after concurrent updates: warning=%v critical=%v", warning, critical)
	}
}

func TestAlertBuiltInTargetContractAndCanonicalCollectionKey(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_401_100)
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: testsupport.NewFakeClock(time.Unix(now, 0))})
	if err != nil {
		t.Fatalf("create target-contract alert service: %v", err)
	}
	site := newAlertTestSite(now, "https://b6-alert-target.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create target-contract site: %v", err)
	}
	value := "1"
	invalid := []AlertEvaluation{
		{RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "account", TargetKey: "42", State: AlertSampleKnown, CurrentValue: &value},
		{RuleKey: "cpu_high", TargetType: "instance", TargetKey: strconv.FormatInt(site.ID, 10) + "/node", State: AlertSampleKnown, CurrentValue: &value},
	}
	for _, suffix := range []string{"01", "0", "-1"} {
		invalid = append(invalid, AlertEvaluation{
			RuleKey: "backfill_failed", SiteID: &site.ID, TargetType: "collection",
			TargetKey: strconv.FormatInt(site.ID, 10) + "/" + suffix, State: AlertSampleKnown, CurrentValue: &value,
		})
	}
	for index, evaluation := range invalid {
		if _, err := alerts.Evaluate(context.Background(), evaluation); err == nil {
			t.Fatalf("invalid target case %d was accepted: %#v", index, evaluation)
		}
	}

	canonical := strconv.FormatInt(site.ID, 10) + "/123"
	evaluation := AlertEvaluation{
		RuleKey: "backfill_failed", SiteID: &site.ID, TargetType: "collection", TargetKey: " " + canonical + " ",
		State: AlertSampleKnown, CurrentValue: &value, Source: "collection_run", RequestID: "req-b6-collection",
		Message:    dto.MessageRef{TechnicalDetail: "collection evidence"},
		ObservedAt: now, SampleKey: "test:collection:123",
	}
	first, err := alerts.Evaluate(context.Background(), evaluation)
	if err != nil {
		t.Fatalf("evaluate canonical collection target: %v", err)
	}
	evaluation.TargetKey = canonical
	second, err := alerts.Evaluate(context.Background(), evaluation)
	if err != nil {
		t.Fatalf("reevaluate canonical collection target: %v", err)
	}
	if first.EventID <= 0 || second.EventID != first.EventID {
		t.Fatalf("canonical collection target did not deduplicate: first=%#v second=%#v", first, second)
	}
	var persisted struct {
		TargetKey string
		Count     int64
	}
	if err := tx.Raw(`SELECT MIN(target_key) AS target_key, COUNT(*) AS count
FROM alert_event WHERE rule_key = 'backfill_failed' AND site_id = ? AND active_key IS NOT NULL`, site.ID).Scan(&persisted).Error; err != nil {
		t.Fatalf("read canonical collection event: %v", err)
	}
	if persisted.TargetKey != canonical || persisted.Count != 1 {
		t.Fatalf("canonical collection persistence = %#v", persisted)
	}
}

func TestAlertEvaluationCursorWatermarkUnknownAndConflict(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_401_200)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create cursor alert service: %v", err)
	}
	site := newAlertTestSite(now, "https://b6-alert-cursor.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create cursor site: %v", err)
	}
	target := strconv.FormatInt(site.ID, 10) + "/node-cursor"
	value := "90"
	evaluate := func(observedAt int64, sampleKey string, state AlertSampleState, current *string) (AlertEvaluationResult, error) {
		return alerts.Evaluate(context.Background(), AlertEvaluation{
			RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "instance", TargetKey: target,
			TargetName: "node-cursor", State: state, CurrentValue: current,
			ObservedAt: observedAt, SampleKey: sampleKey,
		})
	}
	first, err := evaluate(now, "test:cursor:1", AlertSampleKnown, &value)
	if err != nil || first.Status != dto.AlertStatusPending || first.Transition != "pending" {
		t.Fatalf("first cursor sample = %#v, %v", first, err)
	}
	for _, sample := range []struct {
		observedAt int64
		key        string
	}{{observedAt: now, key: "test:cursor:1"}, {observedAt: now - 1, key: "test:cursor:old"}} {
		replayed, err := evaluate(sample.observedAt, sample.key, AlertSampleKnown, &value)
		if err != nil || replayed.Transition != "duplicate" || replayed.EventID != first.EventID {
			t.Fatalf("replayed cursor sample = %#v, %v", replayed, err)
		}
	}
	assertAlertEventState(t, tx, first.EventID, dto.AlertStatusPending, dto.AlertLevelWarning, 1)
	unknown, err := evaluate(now+1, "test:cursor:unknown", AlertSampleUnknown, nil)
	if err != nil || unknown.Transition != "unknown" {
		t.Fatalf("unknown cursor sample = %#v, %v", unknown, err)
	}
	second, err := evaluate(now+1, "test:cursor:2", AlertSampleKnown, &value)
	if err != nil || second.EventID != first.EventID || second.Status != dto.AlertStatusPending {
		t.Fatalf("known sample after unknown = %#v, %v", second, err)
	}
	assertAlertEventState(t, tx, first.EventID, dto.AlertStatusPending, dto.AlertLevelWarning, 2)
	if _, err := evaluate(now+1, "test:cursor:conflict", AlertSampleKnown, &value); !errors.Is(err, ErrAlertSampleConflict) {
		t.Fatalf("same-time cursor conflict error = %v", err)
	}
	assertAlertEventState(t, tx, first.EventID, dto.AlertStatusPending, dto.AlertLevelWarning, 2)
	healthy := "10"
	resolved, err := evaluate(now+2, "test:cursor:3", AlertSampleKnown, &healthy)
	if err != nil || resolved.EventID != first.EventID || resolved.Transition != "resolved" {
		t.Fatalf("cursor resolved sample = %#v, %v", resolved, err)
	}
	reopened, err := evaluate(now+3, "test:cursor:4", AlertSampleKnown, &value)
	if err != nil || reopened.EventID == first.EventID || reopened.Status != dto.AlertStatusPending {
		t.Fatalf("cursor reopened sample = %#v, %v", reopened, err)
	}
	activeKey := alertActiveKey("cpu_high", "instance", target)
	var cursors []model.AlertEvaluationCursor
	if err := tx.Where("active_key = ?", activeKey).Find(&cursors).Error; err != nil || len(cursors) != 1 ||
		cursors[0].LastSampleAt != now+3 || cursors[0].LastSampleKey != "test:cursor:4" {
		t.Fatalf("persisted alert cursor = %#v, %v", cursors, err)
	}
}

func TestAlertEvaluationReconcilesPriorValidationIdentityWithoutReplayingEvent(t *testing.T) {
	tx := openAlertTestTransaction(t)
	observedAt := int64(1_752_436_087)
	hourTS := observedAt - observedAt%3600 - 3600
	clock := testsupport.NewFakeClock(time.Unix(observedAt+60, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	site := newAlertTestSite(observedAt-3600, "https://validation-identity-upgrade.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.DataExportEnabled = true
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create validation identity site: %v", err)
	}
	factComplete := model.CollectionWindowStatusComplete
	validation := model.AlertValidationEvaluationSnapshot{
		RunWindowID: 993, SiteID: site.ID, SiteName: site.Name,
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		DataExportEnabled: true, HourTS: hourTS, Status: model.CollectionTaskStatusFailed,
		ErrorCode: "WORKER_LEASE_LOST", FactStatus: &factComplete, UpdatedAt: observedAt,
	}
	legacy, err := validationFailedEvaluation(validationEvaluationTarget{validation: &validation}, observedAt, "legacy_validation")
	if err != nil {
		t.Fatalf("build legacy validation evaluation: %v", err)
	}
	firing, err := alerts.Evaluate(context.Background(), legacy)
	if err != nil || firing.Transition != "firing" || firing.EventID <= 0 {
		t.Fatalf("evaluate legacy validation sample = %#v, %v", firing, err)
	}
	deliveryEventID := firing.EventID
	preservedDelivery := model.AlertDelivery{
		AlertEventID: &deliveryEventID, EventType: model.AlertDeliveryEventFiring,
		Channel: "dingtalk", Status: model.AlertDeliveryStatusPending,
		PayloadSnapshot: []byte(`{"schema_version":"v1"}`), CreatedAt: observedAt, UpdatedAt: observedAt,
	}
	if err := tx.Create(&preservedDelivery).Error; err != nil {
		t.Fatalf("seed preserved delivery history: %v", err)
	}

	collection := model.AlertCollectionEvaluationSnapshot{
		ID: 101, SiteID: site.ID, SiteName: site.Name,
		ManagementStatus: constant.SiteManagementActive, AuthStatus: constant.SiteAuthAuthorized,
		DataExportEnabled: true, HourTS: hourTS, Status: model.CollectionWindowStatusComplete,
		UpdatedAt: observedAt - 60,
	}
	current, err := validationFailedEvaluation(validationEvaluationTarget{
		collection: &collection, validation: &validation,
	}, observedAt, "current_validation")
	if err != nil {
		t.Fatalf("build current validation evaluation: %v", err)
	}
	if current.SampleKey == legacy.SampleKey || len(current.PriorSampleKeys) != 1 || current.PriorSampleKeys[0] != legacy.SampleKey {
		t.Fatalf("identity upgrade aliases = current:%q legacy:%q prior:%v", current.SampleKey, legacy.SampleKey, current.PriorSampleKeys)
	}
	for attempt := 0; attempt < 2; attempt++ {
		duplicate, evaluationErr := alerts.Evaluate(context.Background(), current)
		if evaluationErr != nil || duplicate.Transition != "duplicate" || duplicate.EventID != firing.EventID {
			t.Fatalf("reconcile attempt %d = %#v, %v", attempt+1, duplicate, evaluationErr)
		}
	}

	activeKey := alertActiveKey(current.RuleKey, current.TargetType, current.TargetKey)
	var cursor model.AlertEvaluationCursor
	if err := tx.Where("active_key = ?", activeKey).First(&cursor).Error; err != nil {
		t.Fatalf("load reconciled cursor: %v", err)
	}
	if cursor.LastSampleAt != observedAt || cursor.LastSampleKey != current.SampleKey {
		t.Fatalf("reconciled cursor = %#v", cursor)
	}
	var eventCount, deliveryCount int64
	if err := tx.Model(&model.AlertEvent{}).Where("id = ?", firing.EventID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count preserved event: %v", err)
	}
	if err := tx.Model(&model.AlertDelivery{}).Where("id = ? AND alert_event_id = ?", preservedDelivery.ID, firing.EventID).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count preserved delivery: %v", err)
	}
	if eventCount != 1 || deliveryCount != 1 {
		t.Fatalf("preserved history counts = events:%d deliveries:%d", eventCount, deliveryCount)
	}
}

func TestAlertConcurrentEvaluationKeepsOneActiveEvent(t *testing.T) {
	database := openAlertConcurrentDatabase(t)
	now := time.Now().Unix()
	ruleKey := "account_quota_empty"
	targetKey := strconv.FormatInt(time.Now().UnixNano(), 10)
	site := newAlertTestSite(now, "https://b6-alert-concurrent-"+targetKey+".example")
	if err := model.NewSiteRepository(database.GORM).Create(context.Background(), &site); err != nil {
		t.Fatalf("create concurrent alert site: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_event WHERE rule_key = ? AND target_key = ?", ruleKey, targetKey)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	alerts, err := NewAlertService(AlertServiceOptions{Database: database.GORM, Clock: testsupport.NewFakeClock(time.Unix(now, 0))})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	value := "0"
	evaluation := AlertEvaluation{
		RuleKey: ruleKey, SiteID: &site.ID, TargetType: "account", TargetKey: targetKey, TargetName: "concurrent",
		State: AlertSampleKnown, CurrentValue: &value, Message: dto.MessageRef{TechnicalDetail: "concurrent evidence"},
		ObservedAt: now, SampleKey: "test:concurrent:" + targetKey,
	}

	results := make([]AlertEvaluationResult, 2)
	errorsSeen := make([]error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index], errorsSeen[index] = alerts.Evaluate(context.Background(), evaluation)
		}(index)
	}
	close(start)
	group.Wait()
	for index, evaluationErr := range errorsSeen {
		if evaluationErr != nil {
			t.Fatalf("evaluation %d: %v", index, evaluationErr)
		}
	}
	if results[0].EventID <= 0 || results[0].EventID != results[1].EventID {
		t.Fatalf("concurrent results = %#v", results)
	}
	var activeCount int64
	if err := database.GORM.Raw("SELECT COUNT(*) FROM alert_event WHERE rule_key = ? AND target_key = ? AND active_key IS NOT NULL", ruleKey, targetKey).Scan(&activeCount).Error; err != nil || activeCount != 1 {
		t.Fatalf("active event count = %d, %v", activeCount, err)
	}
}

func insertAlertRule(t *testing.T, database *gorm.DB, ruleKey, level, threshold string, forTimes int, now int64) int64 {
	t.Helper()
	result := database.Exec(`INSERT INTO alert_rule
(rule_key, name, enabled, level, metric, compare_operator, threshold_value, for_times, scope_type, scope_id, created_at, updated_at)
VALUES (?, ?, 1, ?, 'instance.cpu_percent', '>=', ?, ?, 'global', 0, ?, ?)`, ruleKey, ruleKey+" "+level, level, threshold, forTimes, now, now)
	if result.Error != nil {
		t.Fatalf("insert alert rule: %v", result.Error)
	}
	var id int64
	if err := database.Raw("SELECT id FROM alert_rule WHERE rule_key = ? AND level = ? AND scope_type = 'global'", ruleKey, level).Scan(&id).Error; err != nil {
		t.Fatalf("read alert rule ID: %v", err)
	}
	return id
}

func assertAlertEventState(t *testing.T, database *gorm.DB, id int64, status, level string, consecutive int) {
	t.Helper()
	var state struct {
		Status           string
		Level            string
		ConsecutiveCount int
	}
	if err := database.Raw("SELECT status, level, consecutive_count FROM alert_event WHERE id = ?", id).Scan(&state).Error; err != nil {
		t.Fatalf("read alert event %d: %v", id, err)
	}
	if state.Status != status || state.Level != level || state.ConsecutiveCount != consecutive {
		t.Fatalf("alert event %d = %#v", id, state)
	}
}

func assertAlertEventEvidence(
	t *testing.T,
	database *gorm.DB,
	id int64,
	status, currentValue, thresholdValue, level string,
) {
	t.Helper()
	var evidence struct {
		Status         string
		CurrentValue   string
		ThresholdValue string
		Level          string
		MessageParams  string
		Message        string
	}
	if err := database.Raw(`SELECT status, CAST(current_value AS CHAR) AS current_value,
CAST(threshold_value AS CHAR) AS threshold_value, level,
CAST(message_params AS CHAR) AS message_params, message
FROM alert_event WHERE id = ?`, id).Scan(&evidence).Error; err != nil {
		t.Fatalf("read alert event evidence %d: %v", id, err)
	}
	params := map[string]any{}
	if err := json.Unmarshal([]byte(evidence.MessageParams), &params); err != nil {
		t.Fatalf("decode alert event evidence %d: %v", id, err)
	}
	if evidence.Status != status || evidence.CurrentValue != currentValue || evidence.ThresholdValue != thresholdValue ||
		evidence.Level != level || params["value"] != currentValue || params["threshold"] != thresholdValue {
		t.Fatalf("alert event evidence %d = row:%#v params:%#v", id, evidence, params)
	}
	if status == dto.AlertStatusResolved && strings.Contains(evidence.Message, "test evidence") {
		t.Fatalf("resolved alert event %d retained unchecked technical detail: %q", id, evidence.Message)
	}
}

func alertCPUMessage(value string) dto.MessageRef {
	if value == "" {
		value = "unknown"
	}
	return dto.MustMessageRef(constant.MessageAlertCPUHigh, map[string]any{
		"site_id": "1", "target_type": "account", "target_name": "managed-account", "value": value, "threshold": "85",
	}, "test evidence")
}

func mustAlertID(t *testing.T, value string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		t.Fatalf("invalid alert ID %q", value)
	}
	return id
}

func openAlertConcurrentDatabase(t *testing.T) *model.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open alert test database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve alert test lock: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", alertIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire alert test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run alert test seeds: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return database
}

const alertIntegrationLock = "new-api-pilot-alert-service-integration"

func openAlertTestTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open alert test database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve alert test lock: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", alertIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire alert test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run alert test seeds: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		t.Fatalf("begin alert test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", alertIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return tx
}

func newAlertTestSite(now int64, baseURL string) model.Site {
	return model.Site{
		Name: "Alert Test Site", BaseURL: baseURL, ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineUnknown,
		AuthStatus: constant.SiteAuthUnauthorized, StatisticsStatus: constant.SiteStatisticsPendingConfig,
		HealthStatus: constant.SiteHealthUnavailable, CreatedAt: now, UpdatedAt: now,
	}
}
