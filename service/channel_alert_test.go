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

func TestChannelAlertEvaluationsUseOnlyCompleteFreshHourlyMetrics(t *testing.T) {
	now := int64(1_752_400_800)
	hour := now - now%3600
	count, status, balance, response, availability := int64(4), "complete", "125.5000000000", "750.2500000000", "0.7500000000"
	collectedAt := now
	base := model.AlertChannelEvaluationSnapshot{
		SiteID: 7, SiteName: "Channel Site", SiteConfigVersion: 1, ManagementStatus: constant.SiteManagementActive,
		AuthStatus: constant.SiteAuthAuthorized, HourTS: &hour, CollectedAt: &collectedAt,
		ChannelCount: &count, DataStatus: &status, ConfigVersion: intPointer(1), BalanceTotal: &balance, ResponseTimeAvgMS: &response,
		AvailabilityRate: &availability, SiteUpdatedAt: now,
	}
	evaluations, err := channelAlertEvaluations(base, now, "req_channel_builder")
	if err != nil || len(evaluations) != 3 {
		t.Fatalf("complete channel evaluations = %#v, %v", evaluations, err)
	}
	want := map[string]string{
		"channel_balance_low": balance, "channel_response_time_high": response, "channel_availability_low": availability,
	}
	for _, evaluation := range evaluations {
		if evaluation.State != AlertSampleKnown || evaluation.CurrentValue == nil || *evaluation.CurrentValue != want[evaluation.RuleKey] ||
			evaluation.TargetType != "site" || evaluation.TargetKey != "7" || !strings.HasPrefix(evaluation.SampleKey, "v1:channel:") {
			t.Fatalf("complete channel evaluation = %#v", evaluation)
		}
	}

	staleAt := now - int64(defaultAlertChannelFreshness/time.Second) - 1
	stale := base
	stale.CollectedAt = &staleAt
	staleEvaluations, err := channelAlertEvaluations(stale, now, "req_channel_stale")
	if err != nil {
		t.Fatalf("stale channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, staleEvaluations, AlertSampleUnknown)

	partialStatus := "partial"
	partial := base
	partial.DataStatus = &partialStatus
	partialEvaluations, err := channelAlertEvaluations(partial, now, "req_channel_partial")
	if err != nil {
		t.Fatalf("partial channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, partialEvaluations, AlertSampleUnknown)

	configMismatch := base
	configMismatch.ConfigVersion = intPointer(2)
	configMismatchEvaluations, err := channelAlertEvaluations(configMismatch, now, "req_channel_config_mismatch")
	if err != nil {
		t.Fatalf("config mismatch channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, configMismatchEvaluations, AlertSampleUnknown)

	missing := base
	missing.HourTS, missing.CollectedAt, missing.ChannelCount, missing.DataStatus = nil, nil, nil, nil
	missing.BalanceTotal, missing.ResponseTimeAvgMS, missing.AvailabilityRate = nil, nil, nil
	missingEvaluations, err := channelAlertEvaluations(missing, now, "req_channel_missing")
	if err != nil {
		t.Fatalf("missing channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, missingEvaluations, AlertSampleUnknown)

	inactive := base
	inactive.ManagementStatus = constant.SiteManagementDisabled
	inactiveEvaluations, err := channelAlertEvaluations(inactive, now, "req_channel_inactive")
	if err != nil {
		t.Fatalf("inactive channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, inactiveEvaluations, AlertSampleScopeInactive)

	other := base
	other.SiteID = 8
	otherEvaluations, err := channelAlertEvaluations(other, now, "req_channel_other")
	if err != nil || otherEvaluations[0].SampleKey == evaluations[0].SampleKey {
		t.Fatalf("channel site identities are not isolated: first=%q second=%q err=%v", evaluations[0].SampleKey, otherEvaluations[0].SampleKey, err)
	}
}

func TestChannelAlertThresholdsOverridesRecoveryAndDeliveryDeduplication(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create channel alert service: %v", err)
	}
	if err := tx.Table("platform_setting").Where("setting_key = 'notification.dingtalk.enabled'").
		Update("setting_value", "true").Error; err != nil {
		t.Fatalf("enable alert deliveries: %v", err)
	}
	first := newAlertTestSite(now, "https://channel-alert-first.example")
	first.Name, first.AuthStatus = "Channel First", constant.SiteAuthAuthorized
	second := newAlertTestSite(now+1, "https://channel-alert-second.example")
	second.Name, second.AuthStatus = "Channel Second", constant.SiteAuthAuthorized
	for _, site := range []*model.Site{&first, &second} {
		if err := model.NewSiteRepository(tx).Create(context.Background(), site); err != nil {
			t.Fatalf("create channel alert site: %v", err)
		}
	}

	var warningBaseID int64
	if err := tx.Raw(`SELECT id FROM alert_rule
WHERE rule_key = 'channel_balance_low' AND level = 'warning' AND scope_type = 'global'`).Scan(&warningBaseID).Error; err != nil || warningBaseID <= 0 {
		t.Fatalf("load channel warning base rule: id=%d err=%v", warningBaseID, err)
	}
	overrideThreshold := "50"
	if _, err := alerts.CreateOverride(context.Background(), dto.AlertRuleOverrideRequest{
		BaseRuleID: strconv.FormatInt(warningBaseID, 10), SiteID: strconv.FormatInt(second.ID, 10),
		ThresholdValue: &overrideThreshold,
	}); err != nil {
		t.Fatalf("create low-direction channel override: %v", err)
	}

	evaluate := func(site model.Site, ruleKey, value string, state AlertSampleState, observedAt int64) AlertEvaluationResult {
		t.Helper()
		identity, identityErr := BuildChannelAlertSampleIdentity(site.ID, observedAt-observedAt%3600, observedAt, "value:"+value, "state:"+string(state))
		if identityErr != nil {
			t.Fatalf("build channel sample identity: %v", identityErr)
		}
		var current *string
		if state == AlertSampleKnown {
			current = &value
		}
		result, evaluationErr := alerts.Evaluate(context.Background(), AlertEvaluation{
			RuleKey: ruleKey, SiteID: &site.ID, TargetType: "site", TargetKey: strconv.FormatInt(site.ID, 10), TargetName: site.Name,
			State: state, CurrentValue: current, Source: "channel_sync", RequestID: "req_channel_state",
			ObservedAt: identity.ObservedAt, SampleKey: identity.SampleKey,
		})
		if evaluationErr != nil {
			t.Fatalf("evaluate %s/%s/%s: %v", site.Name, ruleKey, value, evaluationErr)
		}
		clock.Advance(time.Hour)
		return result
	}

	balanceFiring := evaluate(first, "channel_balance_low", "75", AlertSampleKnown, now)
	if balanceFiring.Transition != "firing" || balanceFiring.Level != dto.AlertLevelWarning {
		t.Fatalf("global balance threshold result = %#v", balanceFiring)
	}
	duplicate := evaluate(first, "channel_balance_low", "75", AlertSampleKnown, now)
	if duplicate.EventID != balanceFiring.EventID || duplicate.Transition != "duplicate" {
		t.Fatalf("duplicate channel sample result = %#v", duplicate)
	}
	assertDeliveryEvents(t, tx, balanceFiring.EventID, []string{model.AlertDeliveryEventFiring})
	unknown := evaluate(first, "channel_balance_low", "", AlertSampleUnknown, now+3600)
	if unknown.EventID != balanceFiring.EventID || unknown.Transition != "unknown" {
		t.Fatalf("unknown channel sample result = %#v", unknown)
	}
	assertAlertEventState(t, tx, balanceFiring.EventID, dto.AlertStatusFiring, dto.AlertLevelWarning, 1)
	resolved := evaluate(first, "channel_balance_low", "150", AlertSampleKnown, now+7200)
	if resolved.EventID != balanceFiring.EventID || resolved.Transition != "resolved" {
		t.Fatalf("channel balance recovery result = %#v", resolved)
	}
	assertDeliveryEvents(t, tx, balanceFiring.EventID, []string{model.AlertDeliveryEventFiring, model.AlertDeliveryEventResolved})

	overrideHealthy := evaluate(second, "channel_balance_low", "75", AlertSampleKnown, now+10800)
	if overrideHealthy.Transition != "unchanged" || overrideHealthy.EventID != 0 {
		t.Fatalf("site override did not replace global threshold: %#v", overrideHealthy)
	}
	criticalBalance := evaluate(first, "channel_balance_low", "0", AlertSampleKnown, now+14400)
	if criticalBalance.Transition != "firing" || criticalBalance.Level != dto.AlertLevelCritical {
		t.Fatalf("critical balance threshold result = %#v", criticalBalance)
	}

	responseAt := now + 18000
	firstResponse := evaluate(first, "channel_response_time_high", "1500", AlertSampleKnown, responseAt)
	if firstResponse.Transition != "pending" {
		t.Fatalf("first response threshold result = %#v", firstResponse)
	}
	responseUnknown := evaluate(first, "channel_response_time_high", "", AlertSampleUnknown, responseAt+3600)
	if responseUnknown.Transition != "unknown" {
		t.Fatalf("response unknown result = %#v", responseUnknown)
	}
	secondResponse := evaluate(first, "channel_response_time_high", "1500", AlertSampleKnown, responseAt+7200)
	thirdResponse := evaluate(first, "channel_response_time_high", "1500", AlertSampleKnown, responseAt+10800)
	if secondResponse.Transition != "pending" || thirdResponse.Transition != "firing" || thirdResponse.Level != dto.AlertLevelWarning {
		t.Fatalf("response consecutive results = second:%#v third:%#v", secondResponse, thirdResponse)
	}
	responseResolved := evaluate(first, "channel_response_time_high", "500", AlertSampleKnown, responseAt+14400)
	if responseResolved.Transition != "resolved" {
		t.Fatalf("response recovery result = %#v", responseResolved)
	}

	availabilityAt := responseAt + 18000
	criticalAvailability := evaluate(first, "channel_availability_low", "0.80", AlertSampleKnown, availabilityAt)
	if criticalAvailability.Transition != "firing" || criticalAvailability.Level != dto.AlertLevelCritical {
		t.Fatalf("critical availability threshold result = %#v", criticalAvailability)
	}
	availabilityResolved := evaluate(first, "channel_availability_low", "1", AlertSampleKnown, availabilityAt+3600)
	if availabilityResolved.Transition != "resolved" {
		t.Fatalf("availability recovery result = %#v", availabilityResolved)
	}

	payload := (&DingTalkService{publicOrigin: "https://pilot.example"}).alertPayload(dingTalkDeliverySnapshot{
		Version: 1, Kind: "alert", AlertEventID: strconv.FormatInt(criticalAvailability.EventID, 10),
		EventType: model.AlertDeliveryEventFiring, RuleKey: "channel_availability_low", Level: dto.AlertLevelCritical,
		SiteName: first.Name, TargetName: first.Name,
	})
	wantLink := "https://pilot.example/alerts?alertId=" + strconv.FormatInt(criticalAvailability.EventID, 10)
	if !strings.Contains(payload.Markdown.Text, wantLink) {
		t.Fatalf("channel alert deep link missing from payload: %q", payload.Markdown.Text)
	}
}

func TestChannelAlertRepositoryLoadsLatestAndExactCommittedHourlySnapshot(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	site := newAlertTestSite(now, "https://channel-alert-snapshot.example")
	site.Name, site.AuthStatus = "Channel Snapshot", constant.SiteAuthAuthorized
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create channel snapshot site: %v", err)
	}
	hour1 := now - now%3600
	hour2 := hour1 + 3600
	collected1, collected2 := now, now+3600
	rows := []model.SiteChannelInventoryHourly{
		{SiteID: site.ID, HourTS: hour1, ChannelCount: 2, AvailableCount: 1, UnavailableCount: 1,
			BalanceTotal: "75", ResponseTimeAvgMS: "1500", ResponseTimeMaxMS: 2000, AvailabilityRate: "0.5", DataStatus: "complete", ConfigVersion: 1, CollectedAt: collected1},
		{SiteID: site.ID, HourTS: hour2, ChannelCount: 4, AvailableCount: 4, UnavailableCount: 0,
			BalanceTotal: "200", ResponseTimeAvgMS: "500", ResponseTimeMaxMS: 700, AvailabilityRate: "1", DataStatus: "complete", ConfigVersion: 1, CollectedAt: collected2},
	}
	if err := tx.Create(&rows).Error; err != nil {
		t.Fatalf("create channel hourly snapshots: %v", err)
	}
	repository := model.NewAlertEvaluationRepository(tx)
	snapshot, err := repository.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatalf("load channel alert scan snapshot: %v", err)
	}
	var latest *model.AlertChannelEvaluationSnapshot
	for index := range snapshot.Channels {
		if snapshot.Channels[index].SiteID == site.ID {
			latest = &snapshot.Channels[index]
			break
		}
	}
	if latest == nil || latest.HourTS == nil || *latest.HourTS != hour2 || latest.CollectedAt == nil || *latest.CollectedAt != collected2 ||
		latest.BalanceTotal == nil || *latest.BalanceTotal != "200.0000000000" {
		t.Fatalf("latest channel alert snapshot = %#v", latest)
	}
	exact, err := repository.LoadChannelAlertSnapshot(context.Background(), site.ID, hour1, collected1)
	if err != nil || len(exact.Channels) != 1 || exact.Channels[0].HourTS == nil || *exact.Channels[0].HourTS != hour1 ||
		exact.Channels[0].AvailabilityRate == nil || *exact.Channels[0].AvailabilityRate != "0.5000000000" {
		t.Fatalf("exact channel alert snapshot = %#v, %v", exact.Channels, err)
	}
	missing, err := repository.LoadChannelAlertSnapshot(context.Background(), site.ID, hour1, collected1+1)
	if err != nil || len(missing.Channels) != 1 || missing.Channels[0].HourTS != nil {
		t.Fatalf("missing exact channel snapshot = %#v, %v", missing.Channels, err)
	}
	evaluations, err := channelAlertEvaluations(missing.Channels[0], collected2, "req_channel_exact_missing")
	if err != nil {
		t.Fatalf("build missing exact channel evaluations: %v", err)
	}
	assertChannelAlertStates(t, evaluations, AlertSampleUnknown)
}

func assertChannelAlertStates(t *testing.T, evaluations []AlertEvaluation, state AlertSampleState) {
	t.Helper()
	if len(evaluations) != 3 {
		t.Fatalf("channel evaluation count = %d", len(evaluations))
	}
	for _, evaluation := range evaluations {
		if evaluation.State != state {
			t.Fatalf("channel evaluation state = %#v, want %s", evaluation, state)
		}
	}
}

func intPointer(value int) *int { return &value }
