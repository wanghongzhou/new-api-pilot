package integration_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const (
	a85ScenarioID = "alert_warning_critical_recovery"
	a85FakeHost   = "a85-dingtalk.invalid"
)

var a85MetricKeys = []string{"cpu_high", "memory_high", "disk_high"}

type a85Fixture struct {
	SchemaVersion int    `json:"schema_version"`
	FixtureID     string `json:"fixture_id"`
	Description   string `json:"description"`
	Clock         struct {
		Timezone           string `json:"timezone"`
		Now                string `json:"now"`
		NowUnix            int64  `json:"now_unix"`
		CurrentHour        int64  `json:"current_hour"`
		LatestCompleteHour int64  `json:"latest_complete_hour"`
	} `json:"clock"`
	Scenarios []json.RawMessage `json:"scenarios"`
}

type a85Scenario struct {
	ID      string `json:"id"`
	Initial struct {
		TargetKey         string `json:"target_key"`
		WarningThreshold  string `json:"warning_threshold"`
		CriticalThreshold string `json:"critical_threshold"`
		ForTimes          int    `json:"for_times"`
	} `json:"initial"`
	Samples             []string `json:"samples"`
	ExpectedTransitions []string `json:"expected_transitions"`
	MaximumActiveEvents int      `json:"maximum_active_events_per_target"`
}

type a85SeedEvidence struct {
	RuleKey         string `json:"rule_key"`
	Level           string `json:"level"`
	Metric          string `json:"metric"`
	CompareOperator string `json:"compare_operator"`
	Threshold       string `json:"threshold"`
	ForTimes        int    `json:"for_times"`
}

type a85TransitionEvidence struct {
	SampleIndex int    `json:"sample_index"`
	SampleAt    int64  `json:"sample_at"`
	Value       string `json:"value"`
	Transition  string `json:"transition"`
	EventID     string `json:"event_id"`
	DeliveryID  string `json:"delivery_id"`
}

type a85MetricEvidence struct {
	RuleKey               string                  `json:"rule_key"`
	Samples               []string                `json:"samples"`
	Transitions           []a85TransitionEvidence `json:"transitions"`
	MaximumActiveObserved int                     `json:"maximum_active_observed"`
	FinalCursor           a85CursorEvidence       `json:"final_cursor"`
}

type a85DuplicateEvidence struct {
	SampleIndex         int   `json:"sample_index"`
	CursorCountBefore   int   `json:"cursor_count_before"`
	CursorCountAfter    int   `json:"cursor_count_after"`
	DeliveryCountBefore int64 `json:"delivery_count_before"`
	DeliveryCountAfter  int64 `json:"delivery_count_after"`
}

type a85RequestEvidence struct {
	Sequence      int    `json:"sequence"`
	RequestID     string `json:"request_id"`
	EventType     string `json:"event_type"`
	Title         string `json:"title"`
	StatusCode    int    `json:"status_code"`
	PayloadSHA256 string `json:"payload_sha256"`
}

type a85CursorEvidence struct {
	ActiveKey     string `json:"active_key"`
	LastSampleAt  int64  `json:"last_sample_at"`
	LastSampleKey string `json:"last_sample_key"`
}

type a85FinalEvidence struct {
	EventCount            int64 `json:"event_count"`
	ActiveOrPendingEvents int64 `json:"active_or_pending_events"`
	PendingDeliveries     int64 `json:"pending_deliveries"`
	SuccessfulDeliveries  int64 `json:"successful_deliveries"`
	CursorCount           int64 `json:"cursor_count"`
}

type a85Report struct {
	SchemaVersion int    `json:"schema_version"`
	AcceptanceID  string `json:"acceptance_id"`
	Status        string `json:"status"`
	Fixture       struct {
		ID                  string   `json:"id"`
		SHA256              string   `json:"sha256"`
		ScenarioID          string   `json:"scenario_id"`
		TargetKey           string   `json:"target_key"`
		CanonicalTargetKey  string   `json:"canonical_target_key"`
		Samples             []string `json:"samples"`
		ExpectedTransitions []string `json:"expected_transitions"`
		MaximumActiveEvents int      `json:"maximum_active_events_per_rule_target"`
	} `json:"fixture"`
	SeedRules      []a85SeedEvidence      `json:"seed_rules"`
	Metrics        []a85MetricEvidence    `json:"metrics"`
	DuplicateScans []a85DuplicateEvidence `json:"duplicate_scans"`
	Requests       []a85RequestEvidence   `json:"requests"`
	Final          a85FinalEvidence       `json:"final"`
}

type a85CursorSnapshot struct {
	ActiveKey     string `gorm:"column:active_key"`
	LastSampleAt  int64  `gorm:"column:last_sample_at"`
	LastSampleKey string `gorm:"column:last_sample_key"`
	CreatedAt     int64  `gorm:"column:created_at"`
	UpdatedAt     int64  `gorm:"column:updated_at"`
}

type a85DeliveryRow struct {
	DeliveryID int64  `gorm:"column:delivery_id"`
	EventID    int64  `gorm:"column:event_id"`
	EventType  string `gorm:"column:event_type"`
	RuleKey    string `gorm:"column:rule_key"`
	Level      string `gorm:"column:level"`
	TargetKey  string `gorm:"column:target_key"`
}

type a85FakeRecorder struct {
	mu            sync.Mutex
	requests      []a85RequestEvidence
	requestEvents map[string]string
}

const alertAcceptanceDatabaseLock = "new-api-pilot-alert-service-integration"

var alertAcceptanceSequence atomic.Int64

type alertAcceptanceRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper alertAcceptanceRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestA42A43A53A78AlertStateMachineAcceptance(t *testing.T) {
	acceptanceID := strings.TrimSpace(os.Getenv("ACCEPTANCE_ID"))
	database := openAlertAcceptanceDatabase(t, acceptanceID)
	fixture, _, _ := loadA85Scenario(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	clock := testsupport.NewFakeClock(time.Unix(fixture.Clock.NowUnix, 0))
	site := createAlertAcceptanceHealthySite(t, database.GORM, fixture.Clock.NowUnix)
	t.Cleanup(func() { cleanupAlertAcceptanceSite(database.GORM, site.ID) })

	alerts, err := service.NewAlertService(service.AlertServiceOptions{Database: database.GORM, Clock: clock})
	if err != nil {
		t.Fatalf("create alert acceptance service: %v", err)
	}
	scanSequence := atomic.Int64{}
	scanner, err := service.NewAlertEvaluationScanner(service.AlertEvaluationScannerOptions{
		Database: database.GORM, Evaluator: alerts, Clock: clock,
		RequestIDGenerator: func() (string, error) {
			return fmt.Sprintf("alert_acceptance_scan_%03d", scanSequence.Add(1)), nil
		},
	})
	if err != nil {
		t.Fatalf("create alert acceptance scanner: %v", err)
	}
	repository := model.NewSiteRepository(database.GORM)
	nodeName := "alert-acceptance-node"
	targetKey := strconv.FormatInt(site.ID, 10) + "/" + nodeName
	now := fixture.Clock.NowUnix

	for offset := int64(0); offset < 3; offset++ {
		sampledAt := now + offset*60
		clock.Set(time.Unix(sampledAt, 0))
		writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, sampledAt, 96)
		runAlertAcceptanceScan(t, scanner, ctx)
	}
	critical := findAlertAcceptanceEvent(t, database.GORM, site.ID, "cpu_high", targetKey)
	assertAlertAcceptanceEvent(t, critical, dto.AlertStatusFiring, dto.AlertLevelCritical, 3)
	criticalCursor := findAlertAcceptanceCursor(t, database.GORM, "cpu_high", "instance", targetKey)

	// A42: a stale resource sample is unknown, so it cannot advance or resolve a firing event.
	unknownAt := now + 5*60
	if err := database.GORM.Model(&model.SiteInstance{}).
		Where("site_id = ? AND node_name = ?", site.ID, nodeName).
		Updates(map[string]any{"last_seen_at": unknownAt, "last_synced_at": unknownAt, "updated_at": unknownAt}).Error; err != nil {
		t.Fatalf("keep instance liveness current for unknown resource sample: %v", err)
	}
	clock.Set(time.Unix(unknownAt, 0))
	runAlertAcceptanceScan(t, scanner, ctx)
	afterUnknown := findAlertAcceptanceEventByID(t, database.GORM, critical.ID)
	if afterUnknown.ResolvedAt != nil || afterUnknown.ConsecutiveCount != critical.ConsecutiveCount ||
		afterUnknown.LastFiredAt == nil || critical.LastFiredAt == nil || *afterUnknown.LastFiredAt != *critical.LastFiredAt {
		t.Fatalf("A42 unknown sample changed firing event: %#v", afterUnknown)
	}
	unknownCursor := findAlertAcceptanceCursor(t, database.GORM, "cpu_high", "instance", targetKey)
	if unknownCursor.LastSampleAt != criticalCursor.LastSampleAt || unknownCursor.LastSampleKey != criticalCursor.LastSampleKey {
		t.Fatal("A42 unknown sample advanced the evaluation cursor")
	}

	clock.Set(time.Unix(unknownAt, 0))
	writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, unknownAt, 84)
	runAlertAcceptanceScan(t, scanner, ctx)
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, critical.ID), dto.AlertStatusResolved, dto.AlertLevelCritical, 3)

	// A43: downgrading a firing critical event creates a new warning series that starts at one.
	for offset := int64(6); offset < 9; offset++ {
		sampledAt := now + offset*60
		clock.Set(time.Unix(sampledAt, 0))
		writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, sampledAt, 96)
		runAlertAcceptanceScan(t, scanner, ctx)
	}
	secondCritical := findAlertAcceptanceEvent(t, database.GORM, site.ID, "cpu_high", targetKey)
	assertAlertAcceptanceEvent(t, secondCritical, dto.AlertStatusFiring, dto.AlertLevelCritical, 3)
	downgradeAt := now + 9*60
	clock.Set(time.Unix(downgradeAt, 0))
	writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, downgradeAt, 90)
	runAlertAcceptanceScan(t, scanner, ctx)
	downgraded := findAlertAcceptanceEvent(t, database.GORM, site.ID, "cpu_high", targetKey)
	if downgraded.ID == secondCritical.ID {
		t.Fatal("A43 downgrade reused the resolved critical event")
	}
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, secondCritical.ID), dto.AlertStatusResolved, dto.AlertLevelCritical, 3)
	assertAlertAcceptanceEvent(t, downgraded, dto.AlertStatusPending, dto.AlertLevelWarning, 1)
	assertAlertAcceptanceAtMostOneActive(t, database.GORM, "cpu_high", targetKey)
	for offset := int64(10); offset < 12; offset++ {
		sampledAt := now + offset*60
		clock.Set(time.Unix(sampledAt, 0))
		writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, sampledAt, 90)
		runAlertAcceptanceScan(t, scanner, ctx)
	}
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, downgraded.ID), dto.AlertStatusFiring, dto.AlertLevelWarning, 3)

	// A53: lifecycle and authorization changes resolve the current event with scope_inactive.
	disabledAt := now + 12*60
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"management_status": constant.SiteManagementDisabled, "disabled_at": disabledAt, "updated_at": disabledAt,
	}).Error; err != nil {
		t.Fatalf("disable A53 site: %v", err)
	}
	clock.Set(time.Unix(disabledAt, 0))
	runAlertAcceptanceScan(t, scanner, ctx)
	assertAlertAcceptanceScopeInactive(t, findAlertAcceptanceEventByID(t, database.GORM, downgraded.ID), "site", site.ID)

	restoredAt := now + 13*60
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"management_status": constant.SiteManagementActive, "disabled_at": nil,
		"auth_status": constant.SiteAuthAuthorized, "updated_at": restoredAt,
	}).Error; err != nil {
		t.Fatalf("restore A53 site: %v", err)
	}
	clock.Set(time.Unix(restoredAt, 0))
	writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, restoredAt, 90)
	runAlertAcceptanceScan(t, scanner, ctx)
	reopened := findAlertAcceptanceEvent(t, database.GORM, site.ID, "cpu_high", targetKey)
	if reopened.ID == downgraded.ID {
		t.Fatal("A53 restore did not start a new warning series")
	}
	assertAlertAcceptanceEvent(t, reopened, dto.AlertStatusPending, dto.AlertLevelWarning, 1)
	for offset := int64(14); offset < 16; offset++ {
		sampledAt := now + offset*60
		clock.Set(time.Unix(sampledAt, 0))
		writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, sampledAt, 90)
		runAlertAcceptanceScan(t, scanner, ctx)
	}
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, reopened.ID), dto.AlertStatusFiring, dto.AlertLevelWarning, 3)

	authInactiveAt := now + 16*60
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"auth_status": constant.SiteAuthExpired, "updated_at": authInactiveAt}).Error; err != nil {
		t.Fatalf("expire A53 site authorization: %v", err)
	}
	clock.Set(time.Unix(authInactiveAt, 0))
	runAlertAcceptanceScan(t, scanner, ctx)
	assertAlertAcceptanceScopeInactive(t, findAlertAcceptanceEventByID(t, database.GORM, reopened.ID), "site", site.ID)

	authRestoredAt := now + 17*60
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).
		Updates(map[string]any{"auth_status": constant.SiteAuthAuthorized, "updated_at": authRestoredAt}).Error; err != nil {
		t.Fatalf("restore A53 site authorization: %v", err)
	}
	clock.Set(time.Unix(authRestoredAt, 0))
	writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, authRestoredAt, 90)
	runAlertAcceptanceScan(t, scanner, ctx)
	authReopened := findAlertAcceptanceEvent(t, database.GORM, site.ID, "cpu_high", targetKey)
	if authReopened.ID == reopened.ID {
		t.Fatal("A53 authorization restoration did not start a new sample series")
	}
	assertAlertAcceptanceEvent(t, authReopened, dto.AlertStatusPending, dto.AlertLevelWarning, 1)

	accountAt := now + 18*60
	clock.Set(time.Unix(accountAt, 0))
	writeAlertAcceptanceResourceSample(t, repository, site.ID, nodeName, accountAt, 84)
	account := createAlertAcceptanceQuotaAccount(t, database.GORM, site.ID, accountAt)
	runAlertAcceptanceScan(t, scanner, ctx)
	accountEvent := findAlertAcceptanceEvent(t, database.GORM, site.ID, "account_quota_empty", strconv.FormatInt(account.ID, 10))
	assertAlertAcceptanceEvent(t, accountEvent, dto.AlertStatusFiring, dto.AlertLevelWarning, 1)
	accountArchivedAt := now + 19*60
	if err := database.GORM.Model(&model.Account{}).Where("id = ?", account.ID).
		Updates(map[string]any{"managed_status": model.AccountManagedStatusArchived, "updated_at": accountArchivedAt}).Error; err != nil {
		t.Fatalf("archive A53 account: %v", err)
	}
	clock.Set(time.Unix(accountArchivedAt, 0))
	runAlertAcceptanceScan(t, scanner, ctx)
	assertAlertAcceptanceScopeInactive(t, findAlertAcceptanceEventByID(t, database.GORM, accountEvent.ID), "account", account.ID)

	// A78: both global rules and their site overrides are locked while concurrent samples switch levels.
	warningBase := findAlertAcceptanceGlobalRule(t, database.GORM, "cpu_high", dto.AlertLevelWarning)
	criticalBase := findAlertAcceptanceGlobalRule(t, database.GORM, "cpu_high", dto.AlertLevelCritical)
	warningThreshold, warningForTimes := "80", 2
	warningOverride, err := alerts.CreateOverride(ctx, dto.AlertRuleOverrideRequest{
		BaseRuleID: strconv.FormatInt(warningBase.ID, 10), SiteID: strconv.FormatInt(site.ID, 10),
		ThresholdValue: &warningThreshold, ForTimes: &warningForTimes,
	})
	if err != nil || warningOverride.OverrideRuleID == nil || warningOverride.BaseRuleID != strconv.FormatInt(warningBase.ID, 10) {
		t.Fatalf("create A78 warning override: %#v, %v", warningOverride, err)
	}
	criticalThreshold, criticalForTimes := "90", 1
	criticalOverride, err := alerts.CreateOverride(ctx, dto.AlertRuleOverrideRequest{
		BaseRuleID: strconv.FormatInt(criticalBase.ID, 10), SiteID: strconv.FormatInt(site.ID, 10),
		ThresholdValue: &criticalThreshold, ForTimes: &criticalForTimes,
	})
	if err != nil || criticalOverride.OverrideRuleID == nil || criticalOverride.BaseRuleID != strconv.FormatInt(criticalBase.ID, 10) {
		t.Fatalf("create A78 critical override: %#v, %v", criticalOverride, err)
	}
	warningOverrideID := parseAlertAcceptanceID(t, *warningOverride.OverrideRuleID)
	criticalOverrideID := parseAlertAcceptanceID(t, *criticalOverride.OverrideRuleID)
	overrideTarget := strconv.FormatInt(site.ID, 10) + "/override-node"
	criticalAt := now + 20*60
	clock.Set(time.Unix(criticalAt, 0))
	criticalResults := evaluateAlertAcceptanceConcurrently(t, alerts, site.ID, overrideTarget, "96", criticalAt, "a78-critical")
	if criticalResults[0].EventID <= 0 || criticalResults[0].EventID != criticalResults[1].EventID {
		t.Fatalf("A78 concurrent critical results = %#v", criticalResults)
	}
	concurrentCritical := findAlertAcceptanceEventByID(t, database.GORM, criticalResults[0].EventID)
	if concurrentCritical.RuleID != criticalOverrideID {
		t.Fatalf("A78 critical event rule=%d, want site override=%d", concurrentCritical.RuleID, criticalOverrideID)
	}
	assertAlertAcceptanceEvent(t, concurrentCritical, dto.AlertStatusFiring, dto.AlertLevelCritical, 1)
	assertAlertAcceptanceAtMostOneActive(t, database.GORM, "cpu_high", overrideTarget)

	downgradeConcurrentAt := now + 21*60
	clock.Set(time.Unix(downgradeConcurrentAt, 0))
	downgradeResults := evaluateAlertAcceptanceConcurrently(t, alerts, site.ID, overrideTarget, "85", downgradeConcurrentAt, "a78-downgrade")
	if downgradeResults[0].EventID <= 0 || downgradeResults[0].EventID != downgradeResults[1].EventID ||
		downgradeResults[0].EventID == concurrentCritical.ID {
		t.Fatalf("A78 concurrent downgrade results = %#v", downgradeResults)
	}
	concurrentWarning := findAlertAcceptanceEventByID(t, database.GORM, downgradeResults[0].EventID)
	if concurrentWarning.RuleID != warningOverrideID {
		t.Fatalf("A78 warning event rule=%d, want site override=%d", concurrentWarning.RuleID, warningOverrideID)
	}
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, concurrentCritical.ID), dto.AlertStatusResolved, dto.AlertLevelCritical, 1)
	assertAlertAcceptanceEvent(t, concurrentWarning, dto.AlertStatusPending, dto.AlertLevelWarning, 1)
	assertAlertAcceptanceAtMostOneActive(t, database.GORM, "cpu_high", overrideTarget)

	firingAt := now + 22*60
	clock.Set(time.Unix(firingAt, 0))
	value := "85"
	fired, err := alerts.Evaluate(ctx, service.AlertEvaluation{
		RuleKey: "cpu_high", SiteID: &site.ID, TargetType: "instance", TargetKey: overrideTarget,
		TargetName: "override-node", State: service.AlertSampleKnown, CurrentValue: &value,
		Source: "acceptance_a78", RequestID: "alert_acceptance_a78", ObservedAt: firingAt,
		SampleKey: "acceptance:a78:firing",
	})
	if err != nil || fired.EventID != concurrentWarning.ID || fired.Transition != "firing" {
		t.Fatalf("A78 warning reaccumulation = %#v, %v", fired, err)
	}
	assertAlertAcceptanceEvent(t, findAlertAcceptanceEventByID(t, database.GORM, concurrentWarning.ID), dto.AlertStatusFiring, dto.AlertLevelWarning, 2)
	assertAlertAcceptanceAtMostOneActive(t, database.GORM, "cpu_high", overrideTarget)
}

func TestA44DingTalkWebhookBoundaryAcceptance(t *testing.T) {
	acceptanceID := strings.TrimSpace(os.Getenv("ACCEPTANCE_ID"))
	database := openAlertAcceptanceDatabase(t, acceptanceID)
	fixture, _, _ := loadA85Scenario(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	clock := testsupport.NewFakeClock(time.Unix(fixture.Clock.NowUnix, 0))
	cipher := newA85Cipher(t)
	settingService, err := service.NewSettingService(service.SettingServiceOptions{
		Repository: model.NewSettingRepository(database.GORM), Cipher: cipher, Clock: clock,
		AppEnv: config.EnvironmentTest, DingTalkHosts: []string{"allowed.alert.test", "redirect.alert.test"},
	})
	if err != nil {
		t.Fatalf("create A44 setting service: %v", err)
	}
	settingRows := snapshotAlertAcceptanceDingTalkSettings(t, database.GORM)
	t.Cleanup(func() { restoreAlertAcceptanceDingTalkSettings(database.GORM, settingRows) })

	for _, webhook := range []string{
		"http://allowed.alert.test/robot/send?access_token=a44-private-token",
		"https://untrusted.alert.test/robot/send?access_token=a44-private-token",
	} {
		before := snapshotAlertAcceptanceDingTalkSettings(t, database.GORM)
		_, updateErr := settingService.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{{
			Key: "notification.dingtalk.webhook", Value: alertAcceptanceJSONString(t, webhook),
		}}})
		var validation *service.SettingValidationError
		if !errors.As(updateErr, &validation) || validation.Fields["items[0].value"] == "" {
			t.Fatal("A44 unsafe webhook configuration was not rejected with a field error")
		}
		assertAlertAcceptanceNoSecrets(t, updateErr.Error())
		after := snapshotAlertAcceptanceDingTalkSettings(t, database.GORM)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("A44 rejected webhook configuration changed stored settings")
		}
	}

	validWebhook := "https://allowed.alert.test/robot/send?access_token=a44-private-token"
	secret := "a44-private-secret"
	if _, err := settingService.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "notification.dingtalk.enabled", Value: json.RawMessage("true")},
		{Key: "notification.dingtalk.webhook", Value: alertAcceptanceJSONString(t, validWebhook)},
		{Key: "notification.dingtalk.secret", Value: alertAcceptanceJSONString(t, secret)},
	}}); err != nil {
		t.Fatalf("configure A44 trusted webhook: %v", err)
	}

	var requests []string
	var requestsMu sync.Mutex
	httpClient := &http.Client{Transport: alertAcceptanceRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestsMu.Lock()
		requests = append(requests, request.URL.Hostname())
		requestsMu.Unlock()
		if request.Method != http.MethodPost || request.URL.Hostname() != "allowed.alert.test" {
			return nil, errors.New("unexpected dingtalk redirect request")
		}
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://redirect.alert.test/robot/send?access_token=a44-redirect-token"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}
	dingTalk, err := service.NewDingTalkService(service.DingTalkServiceOptions{
		Database: database.GORM, Clock: clock, Cipher: cipher, HTTPClient: httpClient,
		AllowedHosts: []string{"allowed.alert.test", "redirect.alert.test"},
	})
	if err != nil {
		t.Fatalf("create A44 dingtalk service: %v", err)
	}
	result, err := dingTalk.Test(ctx, "alert_acceptance_a44")
	if err != nil || result.Status != model.AlertDeliveryStatusFailed || result.DeliveryID == nil ||
		result.Message.Code != constant.MessageDingTalkAddressForbidden {
		t.Fatalf("A44 cross-host redirect result=%#v err=%v", result, err)
	}
	requestsMu.Lock()
	visited := append([]string(nil), requests...)
	requestsMu.Unlock()
	if !reflect.DeepEqual(visited, []string{"allowed.alert.test"}) {
		t.Fatalf("A44 redirect contacted unexpected hosts: %v", visited)
	}
	deliveryID := parseAlertAcceptanceID(t, *result.DeliveryID)
	var delivery model.AlertDelivery
	if err := database.GORM.Where("id = ?", deliveryID).Take(&delivery).Error; err != nil {
		t.Fatalf("read A44 delivery: %v", err)
	}
	if delivery.ErrorCode != string(constant.MessageDingTalkAddressForbidden) || delivery.ResponseMessage == nil {
		t.Fatalf("A44 rejected delivery state = %#v", delivery)
	}
	encodedResult, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal A44 result: %v", marshalErr)
	}
	assertAlertAcceptanceNoSecrets(t, string(encodedResult), *delivery.ResponseMessage, string(delivery.PayloadSnapshot))
}

func openAlertAcceptanceDatabase(t *testing.T, acceptanceID string) *model.Database {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		if alertAcceptanceIDRequiresDatabase(acceptanceID) {
			t.Fatalf("%s requires TEST_DATABASE_DSN", acceptanceID)
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 12, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open alert acceptance MySQL: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve alert acceptance lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", alertAcceptanceDatabaseLock).Scan(&acquired); err != nil ||
		!acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire alert acceptance database lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run alert acceptance migrations: %v", err)
	}
	seeder := model.NewSeeder(database.SQL)
	seeder.Now = func() time.Time { return time.Unix(1_768_622_400, 0) }
	if err := seeder.Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", alertAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("seed alert acceptance database: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", alertAcceptanceDatabaseLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return database
}

func alertAcceptanceIDRequiresDatabase(id string) bool {
	switch id {
	case "A42", "A43", "A44", "A53", "A78":
		return true
	default:
		return false
	}
}

func createAlertAcceptanceHealthySite(t *testing.T, database *gorm.DB, now int64) model.Site {
	t.Helper()
	name := fmt.Sprintf("alert-acceptance-%d", alertAcceptanceSequence.Add(1))
	probeAt := now
	statisticsStartAt := now - 3600
	monitoringStartAt := now
	site := model.Site{
		Name: name, BaseURL: "https://" + name + ".invalid", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, Version: "alert-acceptance-v1", DataExportEnabled: true,
		ProbeFailCount: 0, LastProbeAt: &probeAt, LastProbeSuccessAt: &probeAt,
		StatisticsStartAt: &statisticsStartAt, MonitoringStartAt: &monitoringStartAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewSiteRepository(database).Create(context.Background(), &site); err != nil {
		t.Fatalf("create alert acceptance site: %v", err)
	}
	return site
}

func createAlertAcceptanceQuotaAccount(t *testing.T, database *gorm.DB, siteID, now int64) model.Account {
	t.Helper()
	name := fmt.Sprintf("alert-acceptance-customer-%d", alertAcceptanceSequence.Add(1))
	customer := model.Customer{
		Name: name, Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create A53 customer: %v", err)
	}
	account := model.Account{
		SiteID: siteID, CustomerID: customer.ID, RemoteUserID: 900_000 + customer.ID,
		RemoteCreatedAt: now - 3600, Username: name + "-account", DisplayName: name + " Account",
		RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal, LastRemoteSeenAt: &now,
		Quota: 0, ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none",
		LastSyncedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&account).Error; err != nil {
		t.Fatalf("create A53 account: %v", err)
	}
	return account
}

func cleanupAlertAcceptanceSite(database *gorm.DB, siteID int64) {
	if database == nil || siteID <= 0 {
		return
	}
	var accountIDs []int64
	_ = database.Model(&model.Account{}).Where("site_id = ?", siteID).Pluck("id", &accountIDs).Error
	var customerIDs []int64
	if len(accountIDs) > 0 {
		_ = database.Model(&model.Account{}).Where("id IN ?", accountIDs).Pluck("customer_id", &customerIDs).Error
	}
	_ = database.Exec(`DELETE d FROM alert_delivery d
JOIN alert_event e ON e.id = d.alert_event_id
WHERE e.site_id = ?`, siteID).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.AlertEvent{}).Error
	cleanupAlertAcceptanceCursors(database, siteID, accountIDs)
	_ = database.Where("scope_type = 'site' AND scope_id = ?", siteID).Delete(&model.AlertRule{}).Error
	if len(accountIDs) > 0 {
		_ = database.Where("id IN ?", accountIDs).Delete(&model.Account{}).Error
	}
	if len(customerIDs) > 0 {
		_ = database.Where("id IN ?", customerIDs).Delete(&model.Customer{}).Error
	}
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteInstanceStatusMinutely{}).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteStatusMinutely{}).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteInstanceLifecycle{}).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteInstance{}).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteCapability{}).Error
	_ = database.Where("site_id = ?", siteID).Delete(&model.SiteMonitoringPause{}).Error
	_ = database.Where("id = ?", siteID).Delete(&model.Site{}).Error
}

func cleanupAlertAcceptanceCursors(database *gorm.DB, siteID int64, accountIDs []int64) {
	siteKey := strconv.FormatInt(siteID, 10)
	instanceTargets := []string{siteKey + "/alert-acceptance-node", siteKey + "/override-node"}
	keys := make([]string, 0, 24+len(accountIDs)*4)
	for _, ruleKey := range []string{"site_offline", "site_auth_expired", "site_export_disabled", "site_no_instance"} {
		keys = append(keys, a85ActiveKey(ruleKey, "site", siteKey))
	}
	for _, target := range instanceTargets {
		for _, ruleKey := range []string{"instance_stale", "instance_offline", "cpu_high", "memory_high", "disk_high"} {
			keys = append(keys, a85ActiveKey(ruleKey, "instance", target))
		}
	}
	for _, accountID := range accountIDs {
		target := strconv.FormatInt(accountID, 10)
		for _, ruleKey := range []string{"account_missing", "account_identity_mismatch", "account_disabled", "account_quota_empty"} {
			keys = append(keys, a85ActiveKey(ruleKey, "account", target))
		}
	}
	if len(keys) > 0 {
		_ = database.Where("active_key IN ?", keys).Delete(&model.AlertEvaluationCursor{}).Error
	}
}

func writeAlertAcceptanceResourceSample(
	t *testing.T,
	repository *model.SiteRepository,
	siteID int64,
	nodeName string,
	sampledAt int64,
	cpuPercent float64,
) {
	t.Helper()
	if repository == nil || cpuPercent < 0 || cpuPercent > 100 {
		t.Fatal("invalid alert acceptance resource sample")
	}
	lastSeenAt := sampledAt
	healthyPercent := 10.0
	health := constant.SiteHealthOK
	if cpuPercent >= 95 {
		health = constant.SiteHealthCritical
	} else if cpuPercent >= 85 {
		health = constant.SiteHealthWarning
	}
	write := model.SiteInstanceWrite{
		Instance: model.SiteInstance{
			SiteID: siteID, NodeName: nodeName, Hostname: "alert-acceptance-host", IsMaster: true,
			RuntimeVersion: "go1.25", GOOS: "linux", GOARCH: "amd64", UpstreamStatus: "online",
			CurrentStatus: "online", FirstSeenAt: sampledAt, LastSeenAt: &lastSeenAt, LastSyncedAt: sampledAt,
			CreatedAt: sampledAt, UpdatedAt: sampledAt,
		},
		Sample: model.SiteInstanceStatusMinutely{
			SiteID: siteID, NodeName: nodeName, MinuteTS: sampledAt, Status: "online",
			CPUPercent: &cpuPercent, MemoryPercent: &healthyPercent, DiskUsedPercent: &healthyPercent,
			LastSeenAt: &lastSeenAt, CreatedAt: sampledAt,
		},
	}
	if err := repository.SyncInstances(context.Background(), []model.SiteInstanceWrite{write}); err != nil {
		t.Fatalf("write alert acceptance instance sample: %v", err)
	}
	if err := repository.UpsertSiteStatusMinute(context.Background(), model.SiteStatusMinutely{
		SiteID: siteID, MinuteTS: sampledAt, InstanceCount: 1, OnlineInstanceCount: 1,
		CPUMaxPercent: &cpuPercent, CPUAvgPercent: &cpuPercent,
		MemoryMaxPercent: &healthyPercent, MemoryAvgPercent: &healthyPercent,
		DiskMaxUsedPercent: &healthyPercent, HealthStatus: health, CreatedAt: sampledAt,
	}); err != nil {
		t.Fatalf("write alert acceptance site sample: %v", err)
	}
}

func runAlertAcceptanceScan(t *testing.T, scanner *service.AlertEvaluationScanner, ctx context.Context) service.AlertScanResult {
	t.Helper()
	result, err := scanner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run alert acceptance scan: %v", err)
	}
	return result
}

func findAlertAcceptanceEvent(t *testing.T, database *gorm.DB, siteID int64, ruleKey, targetKey string) model.AlertEvent {
	t.Helper()
	var event model.AlertEvent
	if err := database.Where("site_id = ? AND rule_key = ? AND target_key = ?", siteID, ruleKey, targetKey).
		Order("id DESC").Take(&event).Error; err != nil {
		t.Fatalf("find alert acceptance event %s/%s: %v", ruleKey, targetKey, err)
	}
	return event
}

func findAlertAcceptanceEventByID(t *testing.T, database *gorm.DB, id int64) model.AlertEvent {
	t.Helper()
	var event model.AlertEvent
	if err := database.Where("id = ?", id).Take(&event).Error; err != nil {
		t.Fatalf("find alert acceptance event %d: %v", id, err)
	}
	return event
}

func assertAlertAcceptanceEvent(t *testing.T, event model.AlertEvent, status, level string, consecutive int) {
	t.Helper()
	if event.Status != status || event.Level != level || event.ConsecutiveCount != consecutive {
		t.Fatalf("unexpected alert event state: %#v", event)
	}
	if status == dto.AlertStatusResolved {
		if event.ActiveKey != nil || event.ResolvedAt == nil {
			t.Fatalf("resolved alert event is still active: %#v", event)
		}
		return
	}
	if event.ActiveKey == nil || event.ResolvedAt != nil {
		t.Fatalf("active alert event is not active: %#v", event)
	}
}

func assertAlertAcceptanceScopeInactive(t *testing.T, event model.AlertEvent, scopeType string, scopeID int64) {
	t.Helper()
	assertAlertAcceptanceEvent(t, event, dto.AlertStatusResolved, event.Level, event.ConsecutiveCount)
	if event.MessageCode != string(constant.MessageAlertScopeInactive) || event.MessageParams == nil {
		t.Fatalf("scope inactive alert evidence = %#v", event)
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(*event.MessageParams), &params); err != nil {
		t.Fatalf("decode scope inactive params: %v", err)
	}
	if params["scope_type"] != scopeType || params["scope_id"] != strconv.FormatInt(scopeID, 10) {
		t.Fatalf("scope inactive params = %#v", params)
	}
}

func findAlertAcceptanceCursor(t *testing.T, database *gorm.DB, ruleKey, targetType, targetKey string) model.AlertEvaluationCursor {
	t.Helper()
	activeKey := a85ActiveKey(ruleKey, targetType, targetKey)
	var cursor model.AlertEvaluationCursor
	if err := database.Where("active_key = ?", activeKey).Take(&cursor).Error; err != nil {
		t.Fatalf("find alert acceptance cursor: %v", err)
	}
	return cursor
}

func assertAlertAcceptanceAtMostOneActive(t *testing.T, database *gorm.DB, ruleKey, targetKey string) {
	t.Helper()
	var count int64
	if err := database.Model(&model.AlertEvent{}).
		Where("rule_key = ? AND target_key = ? AND active_key IS NOT NULL", ruleKey, targetKey).
		Count(&count).Error; err != nil || count > 1 {
		t.Fatalf("active alert count=%d err=%v", count, err)
	}
}

func findAlertAcceptanceGlobalRule(t *testing.T, database *gorm.DB, ruleKey, level string) model.AlertRule {
	t.Helper()
	var rule model.AlertRule
	if err := database.Where("rule_key = ? AND level = ? AND scope_type = 'global' AND scope_id = 0", ruleKey, level).
		Take(&rule).Error; err != nil {
		t.Fatalf("find alert acceptance global rule %s/%s: %v", ruleKey, level, err)
	}
	return rule
}

func parseAlertAcceptanceID(t *testing.T, value string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		t.Fatalf("invalid alert acceptance identifier")
	}
	return id
}

func evaluateAlertAcceptanceConcurrently(
	t *testing.T,
	alerts *service.AlertService,
	siteID int64,
	targetKey string,
	value string,
	observedAt int64,
	sampleSuffix string,
) []service.AlertEvaluationResult {
	t.Helper()
	results := make([]service.AlertEvaluationResult, 2)
	errorsSeen := make([]error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			currentValue := value
			results[index], errorsSeen[index] = alerts.Evaluate(context.Background(), service.AlertEvaluation{
				RuleKey: "cpu_high", SiteID: &siteID, TargetType: "instance", TargetKey: targetKey,
				TargetName: "override-node", State: service.AlertSampleKnown, CurrentValue: &currentValue,
				Source: "acceptance_a78", RequestID: "alert_acceptance_a78", ObservedAt: observedAt,
				SampleKey: "acceptance:" + sampleSuffix,
			})
		}(index)
	}
	close(start)
	group.Wait()
	for _, evaluationErr := range errorsSeen {
		if evaluationErr != nil {
			t.Fatalf("concurrent alert evaluation: %v", evaluationErr)
		}
	}
	return results
}

func snapshotAlertAcceptanceDingTalkSettings(t *testing.T, database *gorm.DB) []model.PlatformSetting {
	t.Helper()
	keys := []string{
		"notification.dingtalk.enabled",
		"notification.dingtalk.webhook",
		"notification.dingtalk.secret",
	}
	var rows []model.PlatformSetting
	if err := database.Where("setting_key IN ?", keys).Order("setting_key ASC").Find(&rows).Error; err != nil || len(rows) != len(keys) {
		t.Fatalf("snapshot A44 settings rows=%d err=%v", len(rows), err)
	}
	return rows
}

func restoreAlertAcceptanceDingTalkSettings(database *gorm.DB, rows []model.PlatformSetting) {
	if database == nil {
		return
	}
	for _, row := range rows {
		_ = database.Model(&model.PlatformSetting{}).Where("id = ?", row.ID).Updates(map[string]any{
			"setting_value": row.Value, "value_type": row.ValueType, "is_secret": row.Secret, "updated_at": row.UpdatedAt,
		}).Error
	}
}

func alertAcceptanceJSONString(t *testing.T, value string) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode alert acceptance setting: %v", err)
	}
	return encoded
}

func assertAlertAcceptanceNoSecrets(t *testing.T, values ...string) {
	t.Helper()
	for _, value := range values {
		lower := strings.ToLower(value)
		for _, forbidden := range []string{
			"a44-private-token", "a44-private-secret", "a44-redirect-token", "access_token", "?timestamp=", "?sign=",
			"allowed.alert.test", "redirect.alert.test",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatal("A44 exposed a webhook URL, query parameter, or secret")
			}
		}
	}
}

func TestA85AlertFixtureDeliveryDrill(t *testing.T) {
	assertA85DecimalCanonicalization(t)
	acceptance := strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) == "A85"
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		if acceptance {
			t.Fatal("A85 requires an isolated MySQL database")
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	if os.Getenv("A85_ISOLATED_MYSQL") != "true" {
		if acceptance {
			t.Fatal("A85_ISOLATED_MYSQL=true is required")
		}
		t.Skip("A85 requires A85_ISOLATED_MYSQL=true")
	}
	evidenceDir := strings.TrimSpace(os.Getenv("ACCEPTANCE_EVIDENCE_DIR"))
	if acceptance {
		assertA85EvidenceDirectory(t, evidenceDir)
	}

	fixture, scenario, fixtureSHA := loadA85Scenario(t)
	siteID, nodeName := parseA85Target(t, scenario.Initial.TargetKey)
	canonicalTarget := strconv.FormatInt(siteID, 10) + "/" + nodeName
	assertA85FixtureContract(t, fixture, scenario)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{
		DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open isolated A85 MySQL: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("run A85 migrations: %v", err)
	}
	seeder := model.NewSeeder(database.SQL)
	seeder.Now = func() time.Time { return time.Unix(fixture.Clock.NowUnix, 0) }
	if err := seeder.Run(ctx); err != nil {
		t.Fatalf("run A85 seeds: %v", err)
	}
	assertA85DatabaseStartsEmpty(t, database.GORM)
	seedRules := readAndAssertA85SeedRules(t, database.GORM, scenario)

	clock := testsupport.NewFakeClock(time.Unix(fixture.Clock.NowUnix, 0))
	createA85HealthyScope(t, database.GORM, siteID, fixture.Clock.NowUnix)
	alertService, err := service.NewAlertService(service.AlertServiceOptions{
		Database: database.GORM,
		Clock:    clock,
	})
	if err != nil {
		t.Fatalf("create A85 alert service: %v", err)
	}
	var scanSequence atomic.Int64
	scanner, err := service.NewAlertEvaluationScanner(service.AlertEvaluationScannerOptions{
		Database:  database.GORM,
		Evaluator: alertService,
		Clock:     clock,
		RequestIDGenerator: func() (string, error) {
			return fmt.Sprintf("a85_scan_%03d", scanSequence.Add(1)), nil
		},
	})
	if err != nil {
		t.Fatalf("create A85 alert scanner: %v", err)
	}
	recorder := &a85FakeRecorder{requestEvents: make(map[string]string)}
	server, client, webhook := newA85TLSServer(t, recorder)
	defer server.Close()
	cipher := newA85Cipher(t)
	configureA85DingTalk(t, database.GORM, cipher, webhook)
	var requestSequence atomic.Int64
	dingTalk, err := service.NewDingTalkService(service.DingTalkServiceOptions{
		Database:     database.GORM,
		Clock:        clock,
		Cipher:       cipher,
		HTTPClient:   client,
		AllowedHosts: []string{a85FakeHost},
		RequestIDGenerator: func() (string, error) {
			requestID := fmt.Sprintf("a85_delivery_%03d", requestSequence.Add(1))
			var claimed model.AlertDelivery
			if err := database.GORM.Where("status = ? AND claim_token IS NOT NULL", model.AlertDeliveryStatusPending).
				Take(&claimed).Error; err != nil {
				return "", fmt.Errorf("correlate claimed A85 delivery: %w", err)
			}
			recorder.expect(requestID, claimed.EventType)
			return requestID, nil
		},
	})
	if err != nil {
		t.Fatalf("create A85 dingtalk service: %v", err)
	}

	metricEvidence := make(map[string]*a85MetricEvidence, len(a85MetricKeys))
	for _, key := range a85MetricKeys {
		metricEvidence[key] = &a85MetricEvidence{RuleKey: key, Samples: append([]string(nil), scenario.Samples...)}
	}
	duplicateEvidence := make([]a85DuplicateEvidence, 0, len(scenario.Samples))
	lastDeliveryID := int64(0)
	repository := model.NewSiteRepository(database.GORM)
	for index, sample := range scenario.Samples {
		sampleAt := fixture.Clock.NowUnix + int64(index)*60
		clock.Set(time.Unix(sampleAt, 0))
		writeA85ResourceSample(t, repository, siteID, nodeName, sampleAt, sample)
		assertA85EvaluationInput(t, ctx, database.GORM, siteID, nodeName, sampleAt, sample)
		if _, err := scanner.RunOnce(ctx); err != nil {
			t.Fatalf("primary alert scan sample=%d: %v", index, err)
		}
		rows := readA85DeliveriesAfter(t, database.GORM, lastDeliveryID)
		for _, row := range rows {
			if row.DeliveryID > lastDeliveryID {
				lastDeliveryID = row.DeliveryID
			}
			metric := metricEvidence[row.RuleKey]
			if metric == nil || row.TargetKey != canonicalTarget {
				t.Fatalf("unexpected A85 delivery rule=%q target=%q", row.RuleKey, row.TargetKey)
			}
			metric.Transitions = append(metric.Transitions, a85TransitionEvidence{
				SampleIndex: index,
				SampleAt:    sampleAt,
				Value:       sample,
				Transition:  row.Level + "_" + row.EventType,
				EventID:     strconv.FormatInt(row.EventID, 10),
				DeliveryID:  strconv.FormatInt(row.DeliveryID, 10),
			})
		}
		assertA85ActiveEventLimits(t, database.GORM, canonicalTarget, scenario.MaximumActiveEvents, metricEvidence)

		cursorBefore := readA85CursorSnapshot(t, database.GORM)
		deliveryBefore := countA85Rows(t, database.GORM, &model.AlertDelivery{}, "")
		duplicateResult, err := scanner.RunOnce(ctx)
		if err != nil {
			t.Fatalf("duplicate alert scan sample=%d: %v", index, err)
		}
		if duplicateResult.FiringCount != 0 || duplicateResult.ResolvedCount != 0 {
			t.Fatalf("duplicate scan sample=%d created transitions: %#v", index, duplicateResult)
		}
		cursorAfter := readA85CursorSnapshot(t, database.GORM)
		deliveryAfter := countA85Rows(t, database.GORM, &model.AlertDelivery{}, "")
		if !reflect.DeepEqual(cursorBefore, cursorAfter) || deliveryBefore != deliveryAfter {
			t.Fatalf("duplicate scan sample=%d changed cursors or deliveries", index)
		}
		duplicateEvidence = append(duplicateEvidence, a85DuplicateEvidence{
			SampleIndex: index, CursorCountBefore: len(cursorBefore), CursorCountAfter: len(cursorAfter),
			DeliveryCountBefore: deliveryBefore, DeliveryCountAfter: deliveryAfter,
		})
	}

	finalIdentity := readA85FinalResourceIdentity(t, database.GORM, siteID, nodeName)
	metrics := make([]a85MetricEvidence, 0, len(a85MetricKeys))
	for _, key := range a85MetricKeys {
		metric := metricEvidence[key]
		actual := make([]string, 0, len(metric.Transitions))
		for _, transition := range metric.Transitions {
			actual = append(actual, transition.Transition)
		}
		if !reflect.DeepEqual(actual, scenario.ExpectedTransitions) {
			t.Fatalf("%s transitions = %v, want %v", key, actual, scenario.ExpectedTransitions)
		}
		assertA85TransitionIndices(t, key, metric.Transitions)
		metric.FinalCursor = readA85MetricCursor(t, database.GORM, key, canonicalTarget, finalIdentity)
		metrics = append(metrics, *metric)
	}
	assertA85OnlyExpectedEvents(t, database.GORM, siteID, canonicalTarget)

	expectedRequests := len(a85MetricKeys) * len(scenario.ExpectedTransitions)
	pendingBefore := countA85Deliveries(t, database.GORM, model.AlertDeliveryStatusPending)
	if pendingBefore != int64(expectedRequests) {
		t.Fatalf("pending deliveries before send = %d, want %d", pendingBefore, expectedRequests)
	}
	processed := 0
	for {
		didProcess, err := dingTalk.ProcessNext(ctx)
		if err != nil {
			t.Fatalf("process A85 delivery %d: %v", processed+1, err)
		}
		if !didProcess {
			break
		}
		processed++
		if processed > expectedRequests {
			t.Fatalf("processed more than %d A85 deliveries", expectedRequests)
		}
	}
	if processed != expectedRequests {
		t.Fatalf("processed deliveries = %d, want %d", processed, expectedRequests)
	}
	requests := recorder.snapshot()
	if len(requests) != expectedRequests {
		t.Fatalf("fake server requests = %d, want %d", len(requests), expectedRequests)
	}
	assertA85RequestEvidence(t, requests)
	assertA85SuccessfulDeliveries(t, database.GORM, expectedRequests)

	final := a85FinalEvidence{
		EventCount:            countA85Rows(t, database.GORM, &model.AlertEvent{}, ""),
		ActiveOrPendingEvents: countA85ActiveOrPendingEvents(t, database.GORM),
		PendingDeliveries:     countA85Deliveries(t, database.GORM, model.AlertDeliveryStatusPending),
		SuccessfulDeliveries:  countA85Deliveries(t, database.GORM, model.AlertDeliveryStatusSuccess),
		CursorCount:           countA85Rows(t, database.GORM, &model.AlertEvaluationCursor{}, ""),
	}
	if final.EventCount != 9 || final.ActiveOrPendingEvents != 0 || final.PendingDeliveries != 0 ||
		final.SuccessfulDeliveries != int64(expectedRequests) {
		t.Fatalf("unexpected A85 final state: %#v", final)
	}

	report := a85Report{
		SchemaVersion: 1, AcceptanceID: "A85", Status: "passed", SeedRules: seedRules, Metrics: metrics,
		DuplicateScans: duplicateEvidence, Requests: requests, Final: final,
	}
	report.Fixture.ID = fixture.FixtureID
	report.Fixture.SHA256 = fixtureSHA
	report.Fixture.ScenarioID = scenario.ID
	report.Fixture.TargetKey = scenario.Initial.TargetKey
	report.Fixture.CanonicalTargetKey = canonicalTarget
	report.Fixture.Samples = append([]string(nil), scenario.Samples...)
	report.Fixture.ExpectedTransitions = append([]string(nil), scenario.ExpectedTransitions...)
	report.Fixture.MaximumActiveEvents = scenario.MaximumActiveEvents
	if acceptance {
		writeA85Report(t, evidenceDir, report)
	}
}

func loadA85Scenario(t *testing.T) (a85Fixture, a85Scenario, string) {
	t.Helper()
	path := testsupport.DesignFixturePath("f04-state-machines.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read F04 fixture: %v", err)
	}
	digest := sha256.Sum256(contents)
	fixtureSHA := hex.EncodeToString(digest[:])
	assertA85FixtureManifest(t, fixtureSHA)
	var fixture a85Fixture
	if err := decodeA85Strict(contents, &fixture); err != nil {
		t.Fatalf("decode F04 fixture: %v", err)
	}
	var scenario a85Scenario
	matches := 0
	for _, raw := range fixture.Scenarios {
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			t.Fatalf("decode F04 scenario identity: %v", err)
		}
		if identity.ID != a85ScenarioID {
			continue
		}
		matches++
		if err := decodeA85Strict(raw, &scenario); err != nil {
			t.Fatalf("decode A85 scenario: %v", err)
		}
	}
	if matches != 1 {
		t.Fatalf("F04 scenario %q count = %d, want 1", a85ScenarioID, matches)
	}
	return fixture, scenario, fixtureSHA
}

func decodeA85Strict(contents []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func assertA85FixtureManifest(t *testing.T, fixtureSHA string) {
	t.Helper()
	manifestPath := testsupport.DesignFixturePath("manifest.sha256")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	found := false
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "testdata/design/f04-state-machines.json" {
			found = true
			if fields[0] != fixtureSHA {
				t.Fatalf("F04 fixture checksum mismatch")
			}
		}
	}
	if !found {
		t.Fatal("F04 fixture is missing from manifest.sha256")
	}
}

func assertA85FixtureContract(t *testing.T, fixture a85Fixture, scenario a85Scenario) {
	t.Helper()
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F04" || fixture.Clock.NowUnix <= 0 || fixture.Clock.NowUnix%60 != 0 {
		t.Fatalf("invalid F04 fixture identity or clock")
	}
	if scenario.ID != a85ScenarioID || scenario.Initial.WarningThreshold != "85" ||
		scenario.Initial.CriticalThreshold != "95" || scenario.Initial.ForTimes != 3 ||
		scenario.MaximumActiveEvents != 1 {
		t.Fatalf("invalid A85 scenario contract")
	}
	wantSamples := []string{"84", "85", "85", "85", "95", "95", "95", "90", "90", "90", "84"}
	wantTransitions := []string{
		"warning_firing", "warning_resolved", "critical_firing",
		"critical_resolved", "warning_firing", "warning_resolved",
	}
	if !reflect.DeepEqual(scenario.Samples, wantSamples) ||
		!reflect.DeepEqual(scenario.ExpectedTransitions, wantTransitions) {
		t.Fatalf("unexpected A85 samples or transitions")
	}
}

func parseA85Target(t *testing.T, target string) (int64, string) {
	t.Helper()
	parts := strings.Split(target, ":")
	if len(parts) != 4 || parts[0] != "site" || parts[2] != "node" || parts[3] == "" || strings.TrimSpace(parts[3]) != parts[3] {
		t.Fatalf("invalid A85 fixture target key %q", target)
	}
	siteID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || siteID <= 0 || strconv.FormatInt(siteID, 10) != parts[1] {
		t.Fatalf("invalid A85 fixture site ID")
	}
	return siteID, parts[3]
}

func assertA85EvidenceDirectory(t *testing.T, directory string) {
	t.Helper()
	if directory == "" {
		t.Fatal("ACCEPTANCE_EVIDENCE_DIR is required for A85")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("ACCEPTANCE_EVIDENCE_DIR must be absolute")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		t.Fatal("ACCEPTANCE_EVIDENCE_DIR must identify an existing directory")
	}
}

func assertA85DatabaseStartsEmpty(t *testing.T, database *gorm.DB) {
	t.Helper()
	for name, value := range map[string]any{
		"site": &model.Site{}, "customer": &model.Customer{}, "account": &model.Account{},
		"alert_event": &model.AlertEvent{}, "alert_delivery": &model.AlertDelivery{},
	} {
		if count := countA85Rows(t, database, value, ""); count != 0 {
			t.Fatalf("isolated A85 database contains %d pre-existing %s rows", count, name)
		}
	}
}

func readAndAssertA85SeedRules(t *testing.T, database *gorm.DB, scenario a85Scenario) []a85SeedEvidence {
	t.Helper()
	var rows []struct {
		RuleKey         string `gorm:"column:rule_key"`
		Level           string `gorm:"column:level"`
		Metric          string `gorm:"column:metric"`
		CompareOperator string `gorm:"column:compare_operator"`
		Threshold       string `gorm:"column:threshold"`
		ForTimes        int    `gorm:"column:for_times"`
		Enabled         bool   `gorm:"column:enabled"`
	}
	if err := database.Raw(`SELECT rule_key, level, metric, compare_operator,
CAST(threshold_value AS CHAR) AS threshold, for_times, enabled
FROM alert_rule WHERE scope_type = 'global' AND scope_id = 0
ORDER BY rule_key, level`).Scan(&rows).Error; err != nil {
		t.Fatalf("read A85 seed rules: %v", err)
	}
	expected := map[string]a85SeedEvidence{
		"account_disabled/warning":            {"account_disabled", "warning", "account.remote_enabled", "==", "0", 1},
		"account_identity_mismatch/critical":  {"account_identity_mismatch", "critical", "account.identity_match", "==", "0", 1},
		"account_missing/critical":            {"account_missing", "critical", "account.remote_exists", "==", "0", 1},
		"account_quota_empty/warning":         {"account_quota_empty", "warning", "account.quota", "<=", "0", 1},
		"backfill_failed/warning":             {"backfill_failed", "warning", "collection.backfill_failed", ">=", "1", 1},
		"collection_missing/critical":         {"collection_missing", "critical", "collection.missing", ">=", "1", 1},
		"channel_availability_low/critical":   {"channel_availability_low", "critical", "channel.availability_rate", "<=", "0.9", 1},
		"channel_availability_low/warning":    {"channel_availability_low", "warning", "channel.availability_rate", "<=", "0.99", 3},
		"channel_balance_low/critical":        {"channel_balance_low", "critical", "channel.balance_total", "<=", "0", 1},
		"channel_balance_low/warning":         {"channel_balance_low", "warning", "channel.balance_total", "<=", "100", 1},
		"channel_response_time_high/critical": {"channel_response_time_high", "critical", "channel.response_time_avg_ms", ">=", "3000", 1},
		"channel_response_time_high/warning":  {"channel_response_time_high", "warning", "channel.response_time_avg_ms", ">=", "1000", 3},
		"cpu_high/critical":                   {"cpu_high", "critical", "instance.cpu_percent", ">=", scenario.Initial.CriticalThreshold, scenario.Initial.ForTimes},
		"cpu_high/warning":                    {"cpu_high", "warning", "instance.cpu_percent", ">=", scenario.Initial.WarningThreshold, scenario.Initial.ForTimes},
		"disk_high/critical":                  {"disk_high", "critical", "instance.disk_percent", ">=", scenario.Initial.CriticalThreshold, 1},
		"disk_high/warning":                   {"disk_high", "warning", "instance.disk_percent", ">=", scenario.Initial.WarningThreshold, scenario.Initial.ForTimes},
		"instance_offline/critical":           {"instance_offline", "critical", "instance.online", "==", "0", 3},
		"instance_stale/warning":              {"instance_stale", "warning", "instance.stale_seconds", ">=", "90", 1},
		"memory_high/critical":                {"memory_high", "critical", "instance.memory_percent", ">=", scenario.Initial.CriticalThreshold, scenario.Initial.ForTimes},
		"memory_high/warning":                 {"memory_high", "warning", "instance.memory_percent", ">=", scenario.Initial.WarningThreshold, scenario.Initial.ForTimes},
		"site_auth_expired/critical":          {"site_auth_expired", "critical", "site.auth_expired", "==", "1", 1},
		"site_export_disabled/warning":        {"site_export_disabled", "warning", "site.data_export_enabled", "==", "0", 1},
		"site_no_instance/critical":           {"site_no_instance", "critical", "site.online_instances", "<=", "0", 1},
		"site_offline/critical":               {"site_offline", "critical", "site.probe_fail_count", ">=", "3", 1},
		"validation_failed/critical":          {"validation_failed", "critical", "collection.validation_failed", ">=", "1", 1},
	}
	if len(rows) != len(expected) {
		t.Fatalf("A85 seed rule count = %d, want %d", len(rows), len(expected))
	}
	if total := countA85Rows(t, database, &model.AlertRule{}, ""); total != int64(len(expected)) {
		t.Fatalf("total A85 seed rule count = %d, want %d", total, len(expected))
	}
	evidence := make([]a85SeedEvidence, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		canonicalThreshold, err := canonicalA85Decimal(row.Threshold)
		if err != nil {
			t.Fatalf("invalid A85 seed threshold for %s/%s: %v", row.RuleKey, row.Level, err)
		}
		row.Threshold = canonicalThreshold
		identity := row.RuleKey + "/" + row.Level
		want, exists := expected[identity]
		_, duplicate := seen[identity]
		if !exists || duplicate || !row.Enabled || row.Metric != want.Metric ||
			row.CompareOperator != want.CompareOperator || row.Threshold != want.Threshold || row.ForTimes != want.ForTimes {
			t.Fatalf("unexpected A85 seed rule contract for %s", identity)
		}
		seen[identity] = struct{}{}
		evidence = append(evidence, a85SeedEvidence{
			RuleKey: row.RuleKey, Level: row.Level, Metric: row.Metric,
			CompareOperator: row.CompareOperator, Threshold: row.Threshold, ForTimes: row.ForTimes,
		})
	}
	return evidence
}

func assertA85DecimalCanonicalization(t *testing.T) {
	t.Helper()
	for input, expected := range map[string]string{
		"0.0000000000":  "0",
		"85.0000000000": "85",
		"95.0000000000": "95",
		"10.5000000000": "10.5",
	} {
		actual, err := canonicalA85Decimal(input)
		if err != nil || actual != expected {
			t.Fatalf("canonical A85 decimal %q = %q, %v; want %q", input, actual, err, expected)
		}
	}
}

func canonicalA85Decimal(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("decimal must be non-empty and trimmed")
	}
	integer, fraction, hasFraction := strings.Cut(value, ".")
	if integer == "" || !a85DecimalDigits(integer) {
		return "", fmt.Errorf("decimal integer part is invalid")
	}
	if !hasFraction {
		return integer, nil
	}
	if fraction == "" || !a85DecimalDigits(fraction) {
		return "", fmt.Errorf("decimal fraction part is invalid")
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return integer, nil
	}
	return integer + "." + fraction, nil
}

func a85DecimalDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func createA85HealthyScope(t *testing.T, database *gorm.DB, siteID, now int64) {
	t.Helper()
	probeAt := now
	statisticsStartAt := now - 3600
	monitoringStartAt := now
	site := model.Site{
		ID: siteID, Name: "A85 Site", BaseURL: "https://a85-site.invalid", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, Version: "a85-supported", DataExportEnabled: true,
		ProbeFailCount: 0, LastProbeAt: &probeAt, LastProbeSuccessAt: &probeAt,
		StatisticsStartAt: &statisticsStartAt, MonitoringStartAt: &monitoringStartAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewSiteRepository(database).Create(context.Background(), &site); err != nil {
		t.Fatalf("create A85 site: %v", err)
	}
	customer := model.Customer{
		ID: 202, Name: "A85 Customer", Status: dto.CustomerStatusUsing,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create A85 customer: %v", err)
	}
	account := model.Account{
		ID: 301, SiteID: siteID, CustomerID: customer.ID, RemoteUserID: 1001,
		RemoteCreatedAt: now - 86400, Username: "a85-account", DisplayName: "A85 Account",
		RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal, LastRemoteSeenAt: &probeAt,
		Quota: 1, ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none",
		LastSyncedAt: &probeAt, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&account).Error; err != nil {
		t.Fatalf("create A85 account: %v", err)
	}
}

func writeA85ResourceSample(
	t *testing.T,
	repository *model.SiteRepository,
	siteID int64,
	nodeName string,
	sampleAt int64,
	value string,
) {
	t.Helper()
	percent, err := strconv.ParseFloat(value, 64)
	if err != nil || percent < 0 || percent > 100 {
		t.Fatalf("invalid A85 resource sample %q", value)
	}
	lastSeenAt := sampleAt
	write := model.SiteInstanceWrite{
		Instance: model.SiteInstance{
			SiteID: siteID, NodeName: nodeName, Hostname: "a85-host", IsMaster: true,
			RuntimeVersion: "go1.25", GOOS: "linux", GOARCH: "amd64",
			UpstreamStatus: "online", CurrentStatus: "online", FirstSeenAt: sampleAt,
			LastSeenAt: &lastSeenAt, LastSyncedAt: sampleAt, CreatedAt: sampleAt, UpdatedAt: sampleAt,
		},
		Sample: model.SiteInstanceStatusMinutely{
			SiteID: siteID, NodeName: nodeName, MinuteTS: sampleAt, Status: "online",
			CPUPercent: &percent, MemoryPercent: &percent, DiskUsedPercent: &percent,
			LastSeenAt: &lastSeenAt, CreatedAt: sampleAt,
		},
	}
	if err := repository.SyncInstances(context.Background(), []model.SiteInstanceWrite{write}); err != nil {
		t.Fatalf("write A85 instance sample: %v", err)
	}
	health := constant.SiteHealthOK
	if percent >= 95 {
		health = constant.SiteHealthCritical
	} else if percent >= 85 {
		health = constant.SiteHealthWarning
	}
	if err := repository.UpsertSiteStatusMinute(context.Background(), model.SiteStatusMinutely{
		SiteID: siteID, MinuteTS: sampleAt, InstanceCount: 1, OnlineInstanceCount: 1,
		CPUMaxPercent: &percent, CPUAvgPercent: &percent,
		MemoryMaxPercent: &percent, MemoryAvgPercent: &percent,
		DiskMaxUsedPercent: &percent, HealthStatus: health, CreatedAt: sampleAt,
	}); err != nil {
		t.Fatalf("write A85 site resource sample: %v", err)
	}
}

func assertA85EvaluationInput(
	t *testing.T,
	ctx context.Context,
	database *gorm.DB,
	siteID int64,
	nodeName string,
	sampleAt int64,
	expectedValue string,
) {
	t.Helper()
	snapshot, err := model.NewAlertEvaluationRepository(database).LoadSnapshot(ctx)
	if err != nil {
		t.Fatalf("load A85 evaluation input at %d: %v", sampleAt, err)
	}
	if len(snapshot.Instances) != 1 {
		t.Fatalf("A85 evaluation input instance count at %d = %d, want 1", sampleAt, len(snapshot.Instances))
	}
	instance := snapshot.Instances[0]
	if instance.SiteID != siteID || instance.NodeName != nodeName ||
		instance.ManagementStatus != constant.SiteManagementActive ||
		instance.AuthStatus != constant.SiteAuthAuthorized || instance.StatisticsEndAt != nil ||
		instance.SampleID == nil || *instance.SampleID <= 0 ||
		instance.SampledAt == nil || *instance.SampledAt != sampleAt ||
		instance.SampleStatus == nil || *instance.SampleStatus != "online" {
		t.Fatalf("A85 evaluation input scope or identity mismatch at %d", sampleAt)
	}
	for metric, value := range map[string]*string{
		"cpu": instance.CPUPercent, "memory": instance.MemoryPercent, "disk": instance.DiskUsedPercent,
	} {
		if value == nil {
			t.Fatalf("A85 evaluation input %s is null at %d", metric, sampleAt)
		}
		canonical, err := canonicalA85Decimal(*value)
		if err != nil || canonical != expectedValue {
			t.Fatalf("A85 evaluation input %s at %d = %q, %v; want %q",
				metric, sampleAt, canonical, err, expectedValue)
		}
	}
}

func readA85DeliveriesAfter(t *testing.T, database *gorm.DB, afterID int64) []a85DeliveryRow {
	t.Helper()
	var rows []a85DeliveryRow
	if err := database.Raw(`SELECT d.id AS delivery_id, e.id AS event_id, d.event_type,
e.rule_key, e.level, e.target_key
FROM alert_delivery d
JOIN alert_event e ON e.id = d.alert_event_id
WHERE d.id > ? ORDER BY d.id`, afterID).Scan(&rows).Error; err != nil {
		t.Fatalf("read A85 transition deliveries: %v", err)
	}
	return rows
}

func assertA85ActiveEventLimits(
	t *testing.T,
	database *gorm.DB,
	targetKey string,
	maximum int,
	metrics map[string]*a85MetricEvidence,
) {
	t.Helper()
	for _, key := range a85MetricKeys {
		var count int64
		if err := database.Model(&model.AlertEvent{}).
			Where("rule_key = ? AND target_type = 'instance' AND target_key = ? AND active_key IS NOT NULL", key, targetKey).
			Count(&count).Error; err != nil {
			t.Fatalf("count active %s events: %v", key, err)
		}
		if count > int64(maximum) {
			t.Fatalf("active %s events = %d, maximum=%d", key, count, maximum)
		}
		if int(count) > metrics[key].MaximumActiveObserved {
			metrics[key].MaximumActiveObserved = int(count)
		}
	}
}

func readA85CursorSnapshot(t *testing.T, database *gorm.DB) []a85CursorSnapshot {
	t.Helper()
	var rows []a85CursorSnapshot
	if err := database.Model(&model.AlertEvaluationCursor{}).
		Order("active_key").Find(&rows).Error; err != nil {
		t.Fatalf("read A85 cursor snapshot: %v", err)
	}
	return rows
}

func assertA85TransitionIndices(t *testing.T, ruleKey string, transitions []a85TransitionEvidence) {
	t.Helper()
	expected := map[string][]int{
		"cpu_high":    {3, 4, 6, 7, 9, 10},
		"memory_high": {3, 4, 6, 7, 9, 10},
		"disk_high":   {3, 4, 4, 7, 9, 10},
	}[ruleKey]
	if len(transitions) != len(expected) {
		t.Fatalf("%s transition count = %d, want %d", ruleKey, len(transitions), len(expected))
	}
	for index, transition := range transitions {
		if transition.SampleIndex != expected[index] {
			t.Fatalf("%s transition %d sample index = %d, want %d",
				ruleKey, index, transition.SampleIndex, expected[index])
		}
	}
}

func readA85FinalResourceIdentity(
	t *testing.T,
	database *gorm.DB,
	siteID int64,
	nodeName string,
) service.AlertSampleIdentity {
	t.Helper()
	var row struct {
		InstanceID int64 `gorm:"column:instance_id"`
		SampleID   int64 `gorm:"column:sample_id"`
		SampledAt  int64 `gorm:"column:sampled_at"`
	}
	if err := database.Raw(`SELECT i.id AS instance_id, m.id AS sample_id, m.minute_ts AS sampled_at
FROM site_instance i
JOIN site_instance_status_minutely m ON m.site_id = i.site_id AND m.node_name = i.node_name
WHERE i.site_id = ? AND i.node_name = ?
ORDER BY m.minute_ts DESC LIMIT 1`, siteID, nodeName).Scan(&row).Error; err != nil {
		t.Fatalf("read final A85 resource identity: %v", err)
	}
	if row.InstanceID <= 0 || row.SampleID <= 0 || row.SampledAt <= 0 {
		t.Fatal("final A85 resource identity is incomplete")
	}
	identity, err := service.BuildResourceAlertSampleIdentity(
		siteID,
		nodeName,
		row.SampledAt,
		"instance_id:"+strconv.FormatInt(row.InstanceID, 10),
		"sample_id:"+strconv.FormatInt(row.SampleID, 10),
	)
	if err != nil {
		t.Fatalf("build final A85 resource identity: %v", err)
	}
	return identity
}

func readA85MetricCursor(
	t *testing.T,
	database *gorm.DB,
	ruleKey string,
	targetKey string,
	expected service.AlertSampleIdentity,
) a85CursorEvidence {
	t.Helper()
	activeKey := a85ActiveKey(ruleKey, "instance", targetKey)
	var cursor model.AlertEvaluationCursor
	if err := database.Where("active_key = ?", activeKey).Take(&cursor).Error; err != nil {
		t.Fatalf("read %s final cursor: %v", ruleKey, err)
	}
	if cursor.LastSampleAt != expected.ObservedAt || cursor.LastSampleKey != expected.SampleKey {
		t.Fatalf("%s final cursor does not match the final persisted resource sample", ruleKey)
	}
	return a85CursorEvidence{
		ActiveKey: cursor.ActiveKey, LastSampleAt: cursor.LastSampleAt, LastSampleKey: cursor.LastSampleKey,
	}
}

func a85ActiveKey(ruleKey, targetType, targetKey string) string {
	digest := sha256.Sum256([]byte(ruleKey + "\x00" + targetType + "\x00" + targetKey))
	return "v1:" + hex.EncodeToString(digest[:])
}

func assertA85OnlyExpectedEvents(t *testing.T, database *gorm.DB, siteID int64, targetKey string) {
	t.Helper()
	var unexpected int64
	if err := database.Model(&model.AlertEvent{}).
		Where("site_id IS NULL OR site_id <> ? OR target_type <> 'instance' OR target_key <> ? OR rule_key NOT IN ?", siteID, targetKey, a85MetricKeys).
		Count(&unexpected).Error; err != nil {
		t.Fatalf("count unexpected A85 events: %v", err)
	}
	if unexpected != 0 {
		t.Fatalf("A85 created %d out-of-scope alert events", unexpected)
	}
}

func newA85Cipher(t *testing.T) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher([]byte("a85-0123456789abcdef0123456789ab"))
	if err != nil {
		t.Fatalf("create A85 cipher: %v", err)
	}
	return cipher
}

func configureA85DingTalk(t *testing.T, database *gorm.DB, cipher *common.Cipher, webhook string) {
	t.Helper()
	values := map[string]string{"notification.dingtalk.enabled": "true"}
	plaintext := map[string]string{
		"notification.dingtalk.webhook": webhook,
		"notification.dingtalk.secret":  "a85-signing-value",
	}
	for key, value := range plaintext {
		encrypted, err := cipher.Encrypt([]byte(value), "setting:"+key)
		if err != nil {
			t.Fatalf("encrypt A85 notification setting: %v", err)
		}
		values[key] = encrypted
	}
	for key, value := range values {
		result := database.Table("platform_setting").Where("setting_key = ?", key).Update("setting_value", value)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("configure A85 notification setting: rows=%d error=%v", result.RowsAffected, result.Error)
		}
	}
}

func newA85TLSServer(t *testing.T, recorder *a85FakeRecorder) (*httptest.Server, *http.Client, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		status := http.StatusOK
		requestID := request.Header.Get("X-Request-ID")
		eventType, hasExpectedEvent := recorder.expectedEvent(requestID)
		query := request.URL.Query()
		if request.Method != http.MethodPost || request.URL.Path != "/robot/send" || requestID == "" ||
			query.Get("timestamp") == "" || query.Get("sign") == "" || query.Get("access_token") == "" ||
			!hasExpectedEvent {
			status = http.StatusBadRequest
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 64*1024+1))
		if err != nil || len(body) == 0 || len(body) > 64*1024 {
			status = http.StatusBadRequest
		}
		var payload struct {
			MessageType string `json:"msgtype"`
			Markdown    struct {
				Title string `json:"title"`
			} `json:"markdown"`
		}
		if json.Unmarshal(body, &payload) != nil || payload.MessageType != "markdown" || payload.Markdown.Title == "" {
			status = http.StatusBadRequest
		}
		digest := sha256.Sum256(body)
		recorder.record(a85RequestEvidence{
			RequestID: requestID, EventType: eventType, Title: payload.Markdown.Title,
			StatusCode: status, PayloadSHA256: hex.EncodeToString(digest[:]),
		})
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = response.Write([]byte(`{"errcode":0}`))
			return
		}
		_, _ = response.Write([]byte(`{"errcode":400}`))
	}))
	parsed, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("parse A85 fake server address: %v", err)
	}
	port := parsed.Port()
	if port == "" {
		server.Close()
		t.Fatal("A85 fake server has no port")
	}
	transport, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		server.Close()
		t.Fatal("A85 fake server transport is unavailable")
	}
	transport = transport.Clone()
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil ||
		transport.TLSClientConfig.InsecureSkipVerify {
		server.Close()
		t.Fatal("A85 fake server transport must use trusted certificate verification")
	}
	tlsConfig := transport.TLSClientConfig.Clone()
	tlsConfig.ServerName = parsed.Hostname()
	transport.TLSClientConfig = tlsConfig
	serverAddress := server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		return dialer.DialContext(ctx, network, serverAddress)
	}
	client := &http.Client{Transport: transport}
	endpoint := "https://" + net.JoinHostPort(a85FakeHost, port) + "/robot/send?access_token=a85-private-value"
	return server, client, endpoint
}

func (recorder *a85FakeRecorder) record(request a85RequestEvidence) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	request.Sequence = len(recorder.requests) + 1
	recorder.requests = append(recorder.requests, request)
}

func (recorder *a85FakeRecorder) expect(requestID, eventType string) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.requestEvents[requestID] = eventType
}

func (recorder *a85FakeRecorder) expectedEvent(requestID string) (string, bool) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	eventType, exists := recorder.requestEvents[requestID]
	return eventType, exists
}

func (recorder *a85FakeRecorder) snapshot() []a85RequestEvidence {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	result := make([]a85RequestEvidence, len(recorder.requests))
	copy(result, recorder.requests)
	return result
}

func assertA85RequestEvidence(t *testing.T, requests []a85RequestEvidence) {
	t.Helper()
	counts := map[string]int{}
	for index, request := range requests {
		wantRequestID := fmt.Sprintf("a85_delivery_%03d", index+1)
		if request.Sequence != index+1 || request.RequestID != wantRequestID ||
			request.StatusCode != http.StatusOK || request.Title == "" || len(request.PayloadSHA256) != 64 ||
			(request.EventType != model.AlertDeliveryEventFiring && request.EventType != model.AlertDeliveryEventResolved) {
			t.Fatalf("invalid redacted A85 request evidence at sequence %d", index+1)
		}
		counts[request.EventType]++
	}
	if counts[model.AlertDeliveryEventFiring] != 9 || counts[model.AlertDeliveryEventResolved] != 9 {
		t.Fatalf("A85 request event counts = %v", counts)
	}
}

func assertA85SuccessfulDeliveries(t *testing.T, database *gorm.DB, expected int) {
	t.Helper()
	var deliveries []model.AlertDelivery
	if err := database.Order("id").Find(&deliveries).Error; err != nil {
		t.Fatalf("read final A85 deliveries: %v", err)
	}
	if len(deliveries) != expected {
		t.Fatalf("final A85 delivery count = %d, want %d", len(deliveries), expected)
	}
	for _, delivery := range deliveries {
		if delivery.Status != model.AlertDeliveryStatusSuccess || delivery.AttemptCount != 1 ||
			delivery.ResponseCode == nil || *delivery.ResponseCode != http.StatusOK || delivery.SentAt == nil ||
			delivery.NextRetryAt != nil || delivery.ClaimToken != nil || delivery.LeaseExpiresAt != nil {
			t.Fatalf("A85 delivery %d did not complete exactly once", delivery.ID)
		}
	}
}

func countA85Deliveries(t *testing.T, database *gorm.DB, status string) int64 {
	t.Helper()
	return countA85Rows(t, database, &model.AlertDelivery{}, "status = ?", status)
}

func countA85ActiveOrPendingEvents(t *testing.T, database *gorm.DB) int64 {
	t.Helper()
	return countA85Rows(t, database, &model.AlertEvent{}, "active_key IS NOT NULL OR status IN ?", []string{"pending", "firing"})
}

func countA85Rows(t *testing.T, database *gorm.DB, value any, clause string, args ...any) int64 {
	t.Helper()
	query := database.Model(value)
	if clause != "" {
		query = query.Where(clause, args...)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count A85 rows: %v", err)
	}
	return count
}

func writeA85Report(t *testing.T, directory string, report a85Report) {
	t.Helper()
	sort.Slice(report.SeedRules, func(first, second int) bool {
		if report.SeedRules[first].RuleKey == report.SeedRules[second].RuleKey {
			return report.SeedRules[first].Level < report.SeedRules[second].Level
		}
		return report.SeedRules[first].RuleKey < report.SeedRules[second].RuleKey
	})
	contents, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode A85 report: %v", err)
	}
	lower := strings.ToLower(string(contents))
	for _, forbidden := range []string{
		"test_database_dsn", "dsn", "webhook", "access_token", "signing-value", "private-value",
		"?sign=", "?timestamp=", "robot/send", "markdown\"", "msgtype\"",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("A85 report contains forbidden sensitive evidence category")
		}
	}
	temporary, err := os.CreateTemp(directory, ".a85-report-*.tmp")
	if err != nil {
		t.Fatalf("create temporary A85 report: %v", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		t.Fatalf("secure temporary A85 report: %v", err)
	}
	if _, err := temporary.Write(append(contents, '\n')); err != nil {
		t.Fatalf("write temporary A85 report: %v", err)
	}
	if err := temporary.Sync(); err != nil {
		t.Fatalf("sync temporary A85 report: %v", err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatalf("close temporary A85 report: %v", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "a85-report.json")); err != nil {
		t.Fatalf("publish A85 report: %v", err)
	}
}
