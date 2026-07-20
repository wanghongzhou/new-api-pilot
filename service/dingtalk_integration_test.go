package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestAlertTransitionsCreateDeduplicatedDeliveriesInSameTransaction(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	if err := tx.Model(&model.AlertRule{}).
		Where("rule_key = 'cpu_high' AND level = 'critical' AND scope_type = 'global'").
		Update("for_times", 1).Error; err != nil {
		t.Fatalf("set critical cadence: %v", err)
	}
	if err := tx.Table("platform_setting").Where("setting_key = 'notification.dingtalk.enabled'").
		Update("setting_value", "true").Error; err != nil {
		t.Fatalf("enable notification: %v", err)
	}
	site := newAlertTestSite(now, "https://b6b-delivery-transition.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create alert service: %v", err)
	}
	target := strconv.FormatInt(site.ID, 10) + "/node-delivery"
	sampleAt := now
	evaluate := func(value string) AlertEvaluationResult {
		t.Helper()
		result, err := alerts.Evaluate(context.Background(), AlertEvaluation{
			RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "instance", TargetKey: target,
			TargetName: "node-delivery", State: AlertSampleKnown, CurrentValue: &value,
			Source: "resource_snapshot", RequestID: "req_b6b_transition",
			ObservedAt: sampleAt, SampleKey: "test:delivery:" + strconv.FormatInt(sampleAt, 10),
		})
		if err != nil {
			t.Fatalf("evaluate %s: %v", value, err)
		}
		sampleAt++
		return result
	}
	firing := evaluate("96")
	if firing.Transition != "firing" {
		t.Fatalf("firing result = %#v", firing)
	}
	evaluate("97")
	assertDeliveryEvents(t, tx, firing.EventID, []string{model.AlertDeliveryEventFiring})
	resolved := evaluate("10")
	if resolved.Transition != "resolved" || resolved.EventID != firing.EventID {
		t.Fatalf("resolved result = %#v", resolved)
	}
	evaluate("10")
	assertDeliveryEvents(t, tx, firing.EventID, []string{model.AlertDeliveryEventFiring, model.AlertDeliveryEventResolved})
}

func TestDingTalkTestUsesSavedConfigAndPersistsRealAttempt(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	cipher := newDingTalkTestCipher(t)
	configureDingTalkSettings(t, tx, cipher, true, "https://oapi.dingtalk.com/robot/send?access_token=private", "test-secret")
	var capturedRequestID string
	service, err := NewDingTalkService(DingTalkServiceOptions{
		Database: tx, Clock: clock, Cipher: cipher, PublicOrigin: "https://pilot.example.com",
		HTTPClient: &http.Client{Transport: dingTalkRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			capturedRequestID = request.Header.Get("X-Request-ID")
			if request.URL.Query().Get("sign") == "" || request.URL.Query().Get("timestamp") == "" {
				t.Fatal("signed query was not added")
			}
			return dingTalkHTTPResponse(http.StatusOK, `{"errcode":0}`, nil), nil
		})},
	})
	if err != nil {
		t.Fatalf("create dingtalk service: %v", err)
	}
	result, err := service.Test(context.Background(), "req_b6b_test")
	if err != nil {
		t.Fatalf("send test notification: %v", err)
	}
	if capturedRequestID != "req_b6b_test" || result.Status != model.AlertDeliveryStatusSuccess || result.DeliveryID == nil ||
		result.Message.Code != constant.MessageNotificationTestSucceeded {
		t.Fatalf("test result = %#v request_id=%q", result, capturedRequestID)
	}
	id, _ := strconv.ParseInt(*result.DeliveryID, 10, 64)
	delivery, err := model.NewAlertDeliveryRepository(tx).Find(context.Background(), id)
	if err != nil || delivery.AlertEventID != nil || delivery.EventType != model.AlertDeliveryEventTest ||
		delivery.Status != model.AlertDeliveryStatusSuccess || delivery.AttemptCount != 1 || delivery.SentAt == nil {
		t.Fatalf("persisted test delivery = %#v, %v", delivery, err)
	}

	configureDingTalkSettings(t, tx, cipher, true, "http://oapi.dingtalk.com/robot/send?access_token=private", "test-secret")
	var before, after int64
	if err := tx.Model(&model.AlertDelivery{}).Where("event_type = 'test'").Count(&before).Error; err != nil {
		t.Fatalf("count test deliveries: %v", err)
	}
	failed, err := service.Test(context.Background(), "req_b6b_bad_address")
	if err != nil {
		t.Fatalf("preflight unsafe address: %v", err)
	}
	if err := tx.Model(&model.AlertDelivery{}).Where("event_type = 'test'").Count(&after).Error; err != nil {
		t.Fatalf("recount test deliveries: %v", err)
	}
	if failed.Status != model.AlertDeliveryStatusFailed || failed.DeliveryID != nil ||
		failed.Message.Code != constant.MessageDingTalkAddressForbidden || after != before {
		t.Fatalf("unsafe preflight result = %#v counts=%d/%d", failed, before, after)
	}
}

func TestDingTalkEnabledWithEmptySecretFailsStablyWithoutHTTP(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := time.Unix(1_752_400_800, 0)
	cipher := newDingTalkTestCipher(t)
	configureDingTalkSettings(
		t,
		tx,
		cipher,
		true,
		"https://oapi.dingtalk.com/robot/send?access_token=private",
		"",
	)
	var httpCalls atomic.Int32
	service, err := NewDingTalkService(DingTalkServiceOptions{
		Database: tx, Clock: testsupport.NewFakeClock(now), Cipher: cipher,
		HTTPClient: &http.Client{Transport: dingTalkRoundTripFunc(func(*http.Request) (*http.Response, error) {
			httpCalls.Add(1)
			return dingTalkHTTPResponse(http.StatusOK, `{"errcode":0}`, nil), nil
		})},
	})
	if err != nil {
		t.Fatalf("create dingtalk service: %v", err)
	}
	var before int64
	if err := tx.Model(&model.AlertDelivery{}).Count(&before).Error; err != nil {
		t.Fatalf("count deliveries before empty-secret test: %v", err)
	}
	for attempt := range 2 {
		result, err := service.Test(context.Background(), "req_b6b_empty_secret_"+strconv.Itoa(attempt))
		if err != nil || result.Status != model.AlertDeliveryStatusFailed || result.DeliveryID != nil ||
			result.Message.Code != constant.MessageNotificationNotConfigured {
			t.Fatalf("empty-secret result %d = %#v, %v", attempt, result, err)
		}
	}
	var after int64
	if err := tx.Model(&model.AlertDelivery{}).Count(&after).Error; err != nil {
		t.Fatalf("count deliveries after empty-secret test: %v", err)
	}
	if before != after || httpCalls.Load() != 0 {
		t.Fatalf("empty-secret side effects deliveries=%d/%d http=%d", before, after, httpCalls.Load())
	}
}

func TestDingTalkRetryPersistsAttemptAndEventuallySucceeds(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	cipher := newDingTalkTestCipher(t)
	configureDingTalkSettings(t, tx, cipher, true, "https://oapi.dingtalk.com/robot/send?access_token=private", "retry-secret")
	var attempts atomic.Int32
	service, err := NewDingTalkService(DingTalkServiceOptions{
		Database: tx, Clock: clock, Cipher: cipher,
		RequestIDGenerator: func() (string, error) { return "wrk_b6b_retry", nil },
		HTTPClient: &http.Client{Transport: dingTalkRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if attempts.Add(1) == 1 {
				return dingTalkHTTPResponse(http.StatusBadGateway, "", nil), nil
			}
			return dingTalkHTTPResponse(http.StatusOK, `{"errcode":0}`, nil), nil
		})},
	})
	if err != nil {
		t.Fatalf("create dingtalk service: %v", err)
	}
	first, err := service.Test(context.Background(), "req_b6b_retry_first")
	if err != nil || first.DeliveryID == nil || first.Status != model.AlertDeliveryStatusFailed {
		t.Fatalf("first attempt = %#v, %v", first, err)
	}
	if first.Message.Code != constant.MessageDeliveryRetryScheduled || first.Message.Params["next_retry_at"] != "1752400860" {
		t.Fatalf("scheduled retry result = %#v", first)
	}
	deliveryID, _ := strconv.ParseInt(*first.DeliveryID, 10, 64)
	delivery, err := model.NewAlertDeliveryRepository(tx).Find(context.Background(), deliveryID)
	if err != nil || delivery.Status != model.AlertDeliveryStatusPending || delivery.AttemptCount != 1 ||
		delivery.NextRetryAt == nil || *delivery.NextRetryAt != now.Add(time.Minute).Unix() {
		t.Fatalf("pending retry = %#v, %v", delivery, err)
	}
	clock.Advance(time.Minute)
	processed, err := service.ProcessNext(context.Background())
	if err != nil || !processed {
		t.Fatalf("process retry = %t, %v", processed, err)
	}
	delivery, err = model.NewAlertDeliveryRepository(tx).Find(context.Background(), deliveryID)
	if err != nil || delivery.Status != model.AlertDeliveryStatusSuccess || delivery.AttemptCount != 2 || delivery.SentAt == nil {
		t.Fatalf("successful retry = %#v, %v", delivery, err)
	}
}

func TestAlertDeliveryLeaseCASRejectsOldAttempt(t *testing.T) {
	tx := openAlertTestTransaction(t)
	repository := model.NewAlertDeliveryRepository(tx)
	now := int64(1_752_400_800)
	payload, err := marshalDingTalkDeliverySnapshot(dingTalkDeliverySnapshot{Version: 1, Kind: "test"})
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	delivery, _, err := repository.Enqueue(context.Background(), nil, model.AlertDeliveryEventTest, payload, now)
	if err != nil {
		t.Fatalf("enqueue delivery: %v", err)
	}
	first, err := repository.ClaimByID(context.Background(), delivery.ID, now, now+30)
	if err != nil {
		t.Fatalf("claim first attempt: %v", err)
	}
	second, err := repository.ClaimByID(context.Background(), delivery.ID, now+31, now+61)
	if err != nil {
		t.Fatalf("claim recovered attempt: %v", err)
	}
	message := "old response"
	err = repository.Complete(context.Background(), model.AlertDeliveryCompletion{
		DeliveryID: delivery.ID, AttemptCount: first.Delivery.AttemptCount, ClaimToken: first.ClaimToken,
		Status: model.AlertDeliveryStatusSuccess, ResponseMessage: &message, SentAt: &now, Now: now + 31,
	})
	if !errors.Is(err, model.ErrAlertDeliveryLeaseLost) {
		t.Fatalf("old attempt completion error = %v", err)
	}
	sentAt := now + 32
	message = "new response"
	if err := repository.Complete(context.Background(), model.AlertDeliveryCompletion{
		DeliveryID: delivery.ID, AttemptCount: second.Delivery.AttemptCount, ClaimToken: second.ClaimToken,
		Status: model.AlertDeliveryStatusSuccess, ResponseMessage: &message, SentAt: &sentAt, Now: sentAt,
	}); err != nil {
		t.Fatalf("complete current attempt: %v", err)
	}
}

func TestDingTalkCrashBeforeHTTPReusesReservedAttempt(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	cipher := newDingTalkTestCipher(t)
	configureDingTalkSettings(
		t,
		tx,
		cipher,
		true,
		"https://oapi.dingtalk.com/robot/send?access_token=private",
		"crash-secret",
	)
	payload, err := marshalDingTalkDeliverySnapshot(dingTalkDeliverySnapshot{Version: 1, Kind: "test"})
	if err != nil {
		t.Fatalf("marshal crash payload: %v", err)
	}
	repository := model.NewAlertDeliveryRepository(tx)
	delivery, _, err := repository.Enqueue(context.Background(), nil, model.AlertDeliveryEventTest, payload, now.Unix())
	if err != nil {
		t.Fatalf("enqueue crash delivery: %v", err)
	}
	var httpCalls atomic.Int32
	service, err := NewDingTalkService(DingTalkServiceOptions{
		Database: tx, Clock: clock, Cipher: cipher,
		RequestIDGenerator: func() (string, error) { return "", errors.New("injected request id crash") },
		HTTPClient: &http.Client{Transport: dingTalkRoundTripFunc(func(*http.Request) (*http.Response, error) {
			httpCalls.Add(1)
			return dingTalkHTTPResponse(http.StatusOK, `{"errcode":0}`, nil), nil
		})},
	})
	if err != nil {
		t.Fatalf("create crash dingtalk service: %v", err)
	}
	processed, err := service.ProcessNext(context.Background())
	if !processed || err == nil || httpCalls.Load() != 0 {
		t.Fatalf("pre-http crash processed=%t err=%v http=%d", processed, err, httpCalls.Load())
	}
	claimed, err := repository.Find(context.Background(), delivery.ID)
	if err != nil || claimed.AttemptCount != 1 || claimed.ClaimToken == nil || claimed.LeaseExpiresAt == nil {
		t.Fatalf("reserved crashed attempt = %#v, %v", claimed, err)
	}
	clock.Advance(dingTalkDeliveryLease + time.Second)
	service.requestID = func() (string, error) { return "wrk_b6b_crash_takeover", nil }
	processed, err = service.ProcessNext(context.Background())
	if err != nil || !processed || httpCalls.Load() != 1 {
		t.Fatalf("takeover processed=%t err=%v http=%d", processed, err, httpCalls.Load())
	}
	completed, err := repository.Find(context.Background(), delivery.ID)
	if err != nil || completed.Status != model.AlertDeliveryStatusSuccess || completed.AttemptCount != 1 ||
		completed.ClaimToken != nil || completed.LeaseExpiresAt != nil {
		t.Fatalf("completed takeover = %#v, %v", completed, err)
	}
}

func TestAlertDeliveryCrashTakeoverReusesEveryAttemptIncludingFifth(t *testing.T) {
	tx := openAlertTestTransaction(t)
	repository := model.NewAlertDeliveryRepository(tx)
	now := int64(1_752_400_800)
	payload, err := marshalDingTalkDeliverySnapshot(dingTalkDeliverySnapshot{Version: 1, Kind: "test"})
	if err != nil {
		t.Fatalf("marshal takeover payload: %v", err)
	}
	delivery, _, err := repository.Enqueue(context.Background(), nil, model.AlertDeliveryEventTest, payload, now)
	if err != nil {
		t.Fatalf("enqueue takeover delivery: %v", err)
	}
	for attempt := 1; attempt <= model.AlertDeliveryMaxAttempts; attempt++ {
		claim, err := repository.ClaimByID(context.Background(), delivery.ID, now, now+30)
		if err != nil || claim.Takeover || claim.Delivery.AttemptCount != attempt {
			t.Fatalf("new attempt %d claim = %#v, %v", attempt, claim, err)
		}
		expiredAt := claim.LeaseExpiresAt + 1
		if attempt == model.AlertDeliveryMaxAttempts {
			changed, err := repository.FailExhaustedLeases(context.Background(), expiredAt)
			if err != nil || changed != 0 {
				t.Fatalf("fifth crashed attempt exhausted early: changed=%d err=%v", changed, err)
			}
		}
		takeover, err := repository.ClaimByID(context.Background(), delivery.ID, expiredAt, expiredAt+30)
		if err != nil || !takeover.Takeover || takeover.Delivery.AttemptCount != attempt ||
			takeover.ClaimToken == claim.ClaimToken {
			t.Fatalf("attempt %d takeover = %#v, %v", attempt, takeover, err)
		}
		if attempt == model.AlertDeliveryMaxAttempts {
			sentAt := expiredAt
			if err := repository.Complete(context.Background(), model.AlertDeliveryCompletion{
				DeliveryID: delivery.ID, AttemptCount: attempt, ClaimToken: takeover.ClaimToken,
				Status: model.AlertDeliveryStatusSuccess, SentAt: &sentAt, Now: expiredAt,
			}); err != nil {
				t.Fatalf("complete fifth takeover: %v", err)
			}
			break
		}
		nextRetryAt := expiredAt + 1
		if err := repository.Complete(context.Background(), model.AlertDeliveryCompletion{
			DeliveryID: delivery.ID, AttemptCount: attempt, ClaimToken: takeover.ClaimToken,
			Status: model.AlertDeliveryStatusPending, NextRetryAt: &nextRetryAt, Now: expiredAt,
		}); err != nil {
			t.Fatalf("schedule attempt %d retry: %v", attempt, err)
		}
		now = nextRetryAt
	}
	persisted, err := repository.Find(context.Background(), delivery.ID)
	if err != nil || persisted.Status != model.AlertDeliveryStatusSuccess ||
		persisted.AttemptCount != model.AlertDeliveryMaxAttempts {
		t.Fatalf("fifth takeover persistence = %#v, %v", persisted, err)
	}
}

func TestResolvedDeliveryWaitsForFiringAndUsesFrozenSnapshots(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	if err := tx.Model(&model.AlertRule{}).
		Where("rule_key = 'cpu_high' AND level = 'critical' AND scope_type = 'global'").
		Update("for_times", 1).Error; err != nil {
		t.Fatalf("set frozen snapshot cadence: %v", err)
	}
	if err := tx.Table("platform_setting").Where("setting_key = 'notification.dingtalk.enabled'").
		Update("setting_value", "true").Error; err != nil {
		t.Fatalf("enable frozen snapshot notification: %v", err)
	}
	site := newAlertTestSite(now, "https://b6b-frozen-snapshot.example")
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create frozen snapshot site: %v", err)
	}
	alerts, err := NewAlertService(AlertServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create frozen snapshot alert service: %v", err)
	}
	target := strconv.FormatInt(site.ID, 10) + "/node-frozen"
	firingValue := "96"
	firing, err := alerts.Evaluate(context.Background(), AlertEvaluation{
		RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "instance", TargetKey: target,
		TargetName: "node-frozen", State: AlertSampleKnown, CurrentValue: &firingValue,
		ObservedAt: now, SampleKey: "test:frozen:firing",
	})
	if err != nil || firing.Transition != "firing" {
		t.Fatalf("create frozen firing = %#v, %v", firing, err)
	}
	resolvedValue := "10"
	resolved, err := alerts.Evaluate(context.Background(), AlertEvaluation{
		RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "instance", TargetKey: target,
		TargetName: "node-frozen", State: AlertSampleKnown, CurrentValue: &resolvedValue,
		ObservedAt: now + 1, SampleKey: "test:frozen:resolved",
	})
	if err != nil || resolved.Transition != "resolved" || resolved.EventID != firing.EventID {
		t.Fatalf("create frozen resolved = %#v, %v", resolved, err)
	}
	if err := tx.Model(&model.AlertEvent{}).Where("id = ?", firing.EventID).
		Updates(map[string]any{"current_value": "999", "message": "mutated after enqueue"}).Error; err != nil {
		t.Fatalf("mutate alert after enqueue: %v", err)
	}
	var deliveries []model.AlertDelivery
	if err := tx.Where("alert_event_id = ?", firing.EventID).Order("id").Find(&deliveries).Error; err != nil || len(deliveries) != 2 {
		t.Fatalf("load frozen deliveries = %#v, %v", deliveries, err)
	}
	repository := model.NewAlertDeliveryRepository(tx)
	firingClaim, err := repository.ClaimByID(context.Background(), deliveries[0].ID, now+2, now+32)
	if err != nil || firingClaim.Delivery.EventType != model.AlertDeliveryEventFiring {
		t.Fatalf("claim frozen firing = %#v, %v", firingClaim, err)
	}
	if _, err := repository.ClaimByID(context.Background(), deliveries[1].ID, now+2, now+32); !errors.Is(err, model.ErrAlertDeliveryUnavailable) {
		t.Fatalf("resolved claimed before firing terminal: %v", err)
	}
	cipher := newDingTalkTestCipher(t)
	notifier, err := NewDingTalkService(DingTalkServiceOptions{Database: tx, Clock: clock, Cipher: cipher})
	if err != nil {
		t.Fatalf("create frozen snapshot renderer: %v", err)
	}
	firingPayload, err := notifier.payloadForClaim(context.Background(), firingClaim)
	if err != nil || !strings.Contains(firingPayload.Markdown.Text, "96.0000000000") ||
		strings.Contains(firingPayload.Markdown.Text, "999") {
		t.Fatalf("frozen firing payload = %#v, %v", firingPayload, err)
	}
	sentAt := now + 2
	if err := repository.Complete(context.Background(), model.AlertDeliveryCompletion{
		DeliveryID: deliveries[0].ID, AttemptCount: firingClaim.Delivery.AttemptCount,
		ClaimToken: firingClaim.ClaimToken, Status: model.AlertDeliveryStatusSuccess, SentAt: &sentAt, Now: sentAt,
	}); err != nil {
		t.Fatalf("complete frozen firing: %v", err)
	}
	resolvedClaim, err := repository.ClaimByID(context.Background(), deliveries[1].ID, now+3, now+33)
	if err != nil || resolvedClaim.Delivery.EventType != model.AlertDeliveryEventResolved {
		t.Fatalf("claim frozen resolved = %#v, %v", resolvedClaim, err)
	}
	resolvedPayload, err := notifier.payloadForClaim(context.Background(), resolvedClaim)
	if err != nil || !strings.Contains(resolvedPayload.Markdown.Text, "10.0000000000") ||
		strings.Contains(resolvedPayload.Markdown.Text, "999") {
		t.Fatalf("frozen resolved payload = %#v, %v", resolvedPayload, err)
	}
}

func TestAlertDeliveryConcurrentClaimHasSingleOwner(t *testing.T) {
	database := openAlertConcurrentDatabase(t)
	repository := model.NewAlertDeliveryRepository(database.GORM)
	now := time.Now().Unix()
	payload, err := marshalDingTalkDeliverySnapshot(dingTalkDeliverySnapshot{Version: 1, Kind: "test"})
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	delivery, _, err := repository.Enqueue(context.Background(), nil, model.AlertDeliveryEventTest, payload, now)
	if err != nil {
		t.Fatalf("enqueue concurrent delivery: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_delivery WHERE id = ?", delivery.ID)
	})
	type claimResult struct {
		claim model.AlertDeliveryClaim
		err   error
	}
	results := make(chan claimResult, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			claim, claimErr := repository.ClaimByID(context.Background(), delivery.ID, now, now+30)
			results <- claimResult{claim: claim, err: claimErr}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	owned, unavailable := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			owned++
			if result.claim.Delivery.AttemptCount != 1 {
				t.Fatalf("owned claim = %#v", result.claim)
			}
		case errors.Is(result.err, model.ErrAlertDeliveryUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected claim error: %v", result.err)
		}
	}
	if owned != 1 || unavailable != 1 {
		t.Fatalf("claim results owned=%d unavailable=%d", owned, unavailable)
	}
}

func TestAlertEvaluationRepositoryLoadsAllSnapshotFamilies(t *testing.T) {
	tx := openAlertTestTransaction(t)
	now := int64(1_752_400_800)
	hour := now - now%3600 - 3600
	statisticsStart := hour - 3600
	site := newAlertTestSite(now, "https://b6b-alert-snapshot.example")
	site.AuthStatus, site.Version, site.DataExportEnabled = constant.SiteAuthAuthorized, "v-test", true
	site.StatisticsStartAt, site.LastProbeAt = &statisticsStart, &now
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create snapshot site: %v", err)
	}
	instance := model.SiteInstance{
		SiteID: site.ID, NodeName: "node-snapshot", CurrentStatus: "online", FirstSeenAt: now - 60,
		LastSeenAt: &now, LastSyncedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&instance).Error; err != nil {
		t.Fatalf("create snapshot instance: %v", err)
	}
	minute := now - now%60
	cpu := 50.0
	if err := tx.Create(&model.SiteInstanceStatusMinutely{
		SiteID: site.ID, NodeName: instance.NodeName, MinuteTS: minute, Status: "online",
		CPUPercent: &cpu, LastSeenAt: &now, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create instance sample: %v", err)
	}
	if err := tx.Create(&model.SiteStatusMinutely{
		SiteID: site.ID, MinuteTS: minute, InstanceCount: 1, OnlineInstanceCount: 1,
		HealthStatus: constant.SiteHealthOK, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create site resource sample: %v", err)
	}
	customer := model.Customer{Name: "快照客户", Status: dto.CustomerStatusUsing, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create snapshot customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 10, RemoteCreatedAt: now - 100,
		Username: "snapshot-account", RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal,
		ManagedStatus: model.AccountManagedStatusActive, LastSyncedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create snapshot account: %v", err)
	}
	window := model.CollectionWindow{
		SiteID: site.ID, HourTS: hour, Status: model.CollectionWindowStatusMissing,
		LastErrorCode: string(constant.MessageDataValidationMismatch), UpdatedAt: now,
	}
	if err := tx.Create(&window).Error; err != nil {
		t.Fatalf("create collection window: %v", err)
	}
	validationRun := createAlertSnapshotRun(t, tx, site.ID, constant.TaskTypeUsageValidation, model.CollectionTaskStatusFailed, now)
	if err := tx.Create(&model.CollectionRunWindow{
		RunID: validationRun.ID, SiteID: site.ID, HourTS: hour, Status: model.CollectionTaskStatusFailed,
		AttemptCount: 5, ErrorCode: "UPSTREAM_UNAVAILABLE", UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create validation window: %v", err)
	}
	backfillRun := createAlertSnapshotRun(t, tx, site.ID, constant.TaskTypeUsageBackfill, model.CollectionTaskStatusFailed, now)
	if err := tx.Create(&model.CollectionRunWindow{
		RunID: backfillRun.ID, SiteID: site.ID, HourTS: hour, Status: model.CollectionTaskStatusFailed,
		AttemptCount: 5, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create backfill window: %v", err)
	}
	snapshot, err := model.NewAlertEvaluationRepository(tx).LoadSnapshot(context.Background())
	if err != nil {
		t.Fatalf("load alert evaluation snapshot: %v", err)
	}
	var siteFound, instanceFound, accountFound, collectionFound bool
	for _, row := range snapshot.Sites {
		siteFound = siteFound || row.ID == site.ID
	}
	for _, row := range snapshot.Instances {
		instanceFound = instanceFound || (row.SiteID == site.ID && row.NodeName == instance.NodeName && row.CPUPercent != nil)
	}
	for _, row := range snapshot.Accounts {
		accountFound = accountFound || row.ID == account.ID
	}
	for _, row := range snapshot.CollectionWindows {
		collectionFound = collectionFound || (row.SiteID == site.ID && row.HourTS == hour)
	}
	var validationFound, backfillFound bool
	for _, row := range snapshot.Validations {
		validationFound = validationFound || (row.SiteID == site.ID && row.HourTS == hour && row.FactStatus != nil)
	}
	for _, row := range snapshot.Backfills {
		backfillFound = backfillFound || (row.RunID == backfillRun.ID && row.HasWindows && !row.FactsRepaired)
	}
	if !siteFound || !instanceFound || !accountFound || !collectionFound || !validationFound || !backfillFound {
		t.Fatalf("snapshot targets found site=%t instance=%t account=%t collection=%t validation=%t backfill=%t",
			siteFound, instanceFound, accountFound, collectionFound, validationFound, backfillFound)
	}
}

func assertDeliveryEvents(t *testing.T, database *gorm.DB, eventID int64, expected []string) {
	t.Helper()
	var events []string
	if err := database.Raw("SELECT event_type FROM alert_delivery WHERE alert_event_id = ? ORDER BY id", eventID).Scan(&events).Error; err != nil {
		t.Fatalf("read alert deliveries: %v", err)
	}
	if len(events) != len(expected) {
		t.Fatalf("delivery events = %v, want %v", events, expected)
	}
	for index := range expected {
		if events[index] != expected[index] {
			t.Fatalf("delivery events = %v, want %v", events, expected)
		}
	}
}

func newDingTalkTestCipher(t *testing.T) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create dingtalk cipher: %v", err)
	}
	return cipher
}

func configureDingTalkSettings(
	t *testing.T,
	database *gorm.DB,
	cipher *common.Cipher,
	enabled bool,
	webhook string,
	secret string,
) {
	t.Helper()
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	values := map[string]string{"notification.dingtalk.enabled": enabledValue}
	for key, plaintext := range map[string]string{
		"notification.dingtalk.webhook": webhook,
		"notification.dingtalk.secret":  secret,
	} {
		value := ""
		if plaintext != "" {
			var err error
			value, err = cipher.Encrypt([]byte(plaintext), "setting:"+key)
			if err != nil {
				t.Fatalf("encrypt %s: %v", key, err)
			}
		}
		values[key] = value
	}
	for key, value := range values {
		result := database.Table("platform_setting").Where("setting_key = ?", key).Update("setting_value", value)
		if result.Error != nil {
			t.Fatalf("update %s: rows=%d error=%v", key, result.RowsAffected, result.Error)
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := database.Table("platform_setting").Where("setting_key = ? AND setting_value = ?", key, value).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("verify unchanged %s: count=%d error=%v", key, count, err)
			}
		}
	}
}

func createAlertSnapshotRun(
	t *testing.T,
	database *gorm.DB,
	siteID int64,
	taskType string,
	status string,
	now int64,
) model.CollectionRun {
	t.Helper()
	windowsInitialized := now
	run := model.CollectionRun{
		SiteID: &siteID, SiteConfigVersion: 1, TaskType: taskType, TargetType: "site", TargetID: siteID,
		TriggerType: constant.CollectionTriggerSchedule, Scope: []byte("{}"), Status: status,
		NextAttemptAt: now, WindowsInitializedAt: &windowsInitialized, TotalWindows: 1, FailedWindows: 1,
		CreatedRequestID: "req_b6b_snapshot", LastRequestID: "req_b6b_snapshot", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&run).Error; err != nil {
		t.Fatalf("create %s run: %v", taskType, err)
	}
	return run
}
