package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type dingTalkRoundTripFunc func(*http.Request) (*http.Response, error)

func (function dingTalkRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestDingTalkSendSignsRequestAndKeepsEvidenceSafe(t *testing.T) {
	now := time.Unix(1_752_400_800, 123_000_000)
	secret := "signed-test-secret"
	var captured *http.Request
	service := newDingTalkUnitService(t, now, func(request *http.Request) (*http.Response, error) {
		captured = request.Clone(context.Background())
		return dingTalkHTTPResponse(http.StatusOK, `{"errcode":0,"errmsg":"ok"}`, nil), nil
	})
	webhook, err := url.Parse("https://oapi.dingtalk.com/robot/send?access_token=private-token")
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}
	result := service.send(context.Background(), &dingTalkPreparedConfig{webhook: webhook, secret: secret}, dingTalkTestPayload(), "req_test_dingtalk")
	if !result.success || result.retryable || result.responseCode == nil || *result.responseCode != http.StatusOK {
		t.Fatalf("send result = %#v", result)
	}
	if captured == nil || captured.Method != http.MethodPost || captured.Header.Get("X-Request-ID") != "req_test_dingtalk" ||
		captured.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("captured request = %#v", captured)
	}
	timestamp := captured.URL.Query().Get("timestamp")
	if timestamp != "1752400800123" {
		t.Fatalf("timestamp = %q", timestamp)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	wantSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if captured.URL.Query().Get("sign") != wantSign || captured.URL.Query().Get("access_token") != "private-token" {
		t.Fatalf("signature query did not match DingTalk contract")
	}
	for _, secretValue := range []string{"private-token", secret, wantSign, "access_token="} {
		if strings.Contains(result.responseMessage, secretValue) {
			t.Fatalf("response evidence leaked secret %q: %q", secretValue, result.responseMessage)
		}
	}
}

func TestDingTalkSendUsesRealTLSFakeServer(t *testing.T) {
	now := time.Unix(1_752_400_800, 0)
	received := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("X-Request-ID") != "req_tls_fake" ||
			request.URL.Query().Get("timestamp") == "" || request.URL.Query().Get("sign") == "" {
			t.Errorf("unexpected fake-server request: method=%s request_id=%q query=%v", request.Method, request.Header.Get("X-Request-ID"), request.URL.Query())
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || !strings.Contains(string(body), `"msgtype":"markdown"`) {
			t.Errorf("fake-server body = %q, %v", body, err)
		}
		received <- struct{}{}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL + "/robot/send?access_token=fake")
	if err != nil {
		t.Fatalf("parse fake-server URL: %v", err)
	}
	service := &DingTalkService{clock: testsupport.NewFakeClock(now), httpClient: server.Client()}
	result := service.send(context.Background(), &dingTalkPreparedConfig{webhook: endpoint, secret: "fake-secret"}, dingTalkTestPayload(), "req_tls_fake")
	if !result.success {
		t.Fatalf("fake-server send result = %#v", result)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("fake server did not receive request")
	}
}

func TestDingTalkSendRetryClassification(t *testing.T) {
	tests := []struct {
		name          string
		response      *http.Response
		err           error
		wantRetryable bool
		wantCode      constant.MessageCode
		wantErrCode   string
		wantDelay     time.Duration
	}{
		{name: "rate limited", response: dingTalkHTTPResponse(http.StatusTooManyRequests, "", map[string]string{"Retry-After": "7200"}), wantRetryable: true, wantDelay: time.Hour},
		{name: "server error", response: dingTalkHTTPResponse(http.StatusBadGateway, "gateway includes https://example.com/?token=secret", nil), wantRetryable: true},
		{name: "network error", err: errors.New("Post https://oapi.dingtalk.com/robot/send?access_token=secret: reset"), wantRetryable: true},
		{name: "dingtalk rejection", response: dingTalkHTTPResponse(http.StatusOK, `{"errcode":310000,"errmsg":"sign invalid"}`, nil), wantRetryable: true, wantCode: constant.MessageDingTalkRejected, wantErrCode: "310000"},
		{name: "client response", response: dingTalkHTTPResponse(http.StatusBadRequest, "", nil), wantCode: constant.MessageDingTalkRejected, wantErrCode: "HTTP_400"},
		{name: "invalid success response", response: dingTalkHTTPResponse(http.StatusOK, `{"errmsg":"missing code"}`, nil), wantCode: constant.MessageDingTalkRejected, wantErrCode: "INVALID_RESPONSE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newDingTalkUnitService(t, time.Unix(1_752_400_800, 0), func(*http.Request) (*http.Response, error) {
				return test.response, test.err
			})
			webhook, _ := url.Parse("https://oapi.dingtalk.com/robot/send?access_token=private-token")
			result := service.send(context.Background(), &dingTalkPreparedConfig{webhook: webhook}, dingTalkTestPayload(), "req_retry")
			if result.retryable != test.wantRetryable || result.errorCode != test.wantCode || result.errCode != test.wantErrCode || result.retryAfter != test.wantDelay {
				t.Fatalf("send result = %#v", result)
			}
			if strings.Contains(result.responseMessage, "secret") || strings.Contains(result.responseMessage, "access_token") || strings.Contains(result.responseMessage, "https://") {
				t.Fatalf("unsafe response message = %q", result.responseMessage)
			}
		})
	}
}

func TestDingTalkAddressAndRedirectPolicy(t *testing.T) {
	allowed, err := normalizeDingTalkAllowedHosts([]string{"robot.example.com"})
	if err != nil {
		t.Fatalf("normalize hosts: %v", err)
	}
	valid := "https://robot.example.com/robot/send?access_token=secret"
	if _, err := validateDingTalkWebhook(valid, allowed); err != nil {
		t.Fatalf("valid webhook rejected: %v", err)
	}
	for _, raw := range []string{
		"http://robot.example.com/robot/send?access_token=secret",
		"https://evil.example.com/robot/send?access_token=secret",
		"https://user@robot.example.com/robot/send?access_token=secret",
		"https://robot.example.com/robot/send?access_token=secret#fragment",
	} {
		if _, err := validateDingTalkWebhook(raw, allowed); err == nil {
			t.Errorf("unsafe webhook accepted: %s", raw)
		}
	}
	policy := dingTalkRedirectPolicy(allowed)
	original, _ := http.NewRequest(http.MethodPost, valid, nil)
	sameHost, _ := http.NewRequest(http.MethodPost, "https://robot.example.com/robot/next", nil)
	if err := policy(sameHost, []*http.Request{original}); err != nil {
		t.Fatalf("same-host POST redirect rejected: %v", err)
	}
	crossHost, _ := http.NewRequest(http.MethodPost, "https://oapi.dingtalk.com/robot/send", nil)
	if !errors.Is(policy(crossHost, []*http.Request{original}), errDingTalkRedirectForbidden) {
		t.Fatal("cross-host redirect was accepted")
	}
	changedMethod, _ := http.NewRequest(http.MethodGet, valid, nil)
	if !errors.Is(policy(changedMethod, []*http.Request{original}), errDingTalkRedirectForbidden) {
		t.Fatal("redirect that changed POST to GET was accepted")
	}
}

func TestNotificationMessageContracts(t *testing.T) {
	deliveryID := "42"
	message, err := notificationMessage(constant.MessageNotificationTestSucceeded, nil, &deliveryID, "", nil)
	if err != nil || message.Code != constant.MessageNotificationTestSucceeded || message.Params["delivery_id"] != deliveryID {
		t.Fatalf("success message = %#v, %v", message, err)
	}
	message, err = notificationMessage(constant.MessageNotificationNotConfigured, nil, nil, "", nil)
	if err != nil || message.Params["alert_event_id"] != nil || message.Params["delivery_id"] != nil {
		t.Fatalf("preflight message = %#v, %v", message, err)
	}
	nextRetryAt := int64(1_752_400_860)
	message, err = notificationMessage(constant.MessageDeliveryRetryScheduled, nil, &deliveryID, "", &nextRetryAt)
	if err != nil || message.Params["delivery_id"] != deliveryID || message.Params["next_retry_at"] != "1752400860" {
		t.Fatalf("scheduled retry message = %#v, %v", message, err)
	}
}

func TestDingTalkRetryAfterParsesHTTPDateAndCaps(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	if duration, valid := parseDingTalkRetryAfter(now.Add(30*time.Minute).Format(http.TimeFormat), now); !valid || duration != 30*time.Minute {
		t.Fatalf("HTTP date retry-after = %s/%t", duration, valid)
	}
	if duration, valid := parseDingTalkRetryAfter("7200", now); !valid || duration != time.Hour {
		t.Fatalf("capped retry-after = %s/%t", duration, valid)
	}
	for _, value := range []string{"9223372036854775807", "184467440737095516160000"} {
		if duration, valid := parseDingTalkRetryAfter(value, now); !valid || duration != time.Hour {
			t.Fatalf("overflow retry-after %q = %s/%t", value, duration, valid)
		}
	}
	if duration, valid := parseDingTalkRetryAfter("3599", now); !valid || duration != 3599*time.Second {
		t.Fatalf("bounded retry-after = %s/%t", duration, valid)
	}
	if _, valid := parseDingTalkRetryAfter("-1", now); valid {
		t.Fatal("negative retry-after accepted")
	}
	if _, valid := parseDingTalkRetryAfter("invalid", now); valid {
		t.Fatal("invalid retry-after accepted")
	}
}

func TestDingTalkLegacySnapshotAcceptsBigintString(t *testing.T) {
	service := newDingTalkUnitService(t, time.Unix(1_752_400_800, 0), func(*http.Request) (*http.Response, error) {
		return nil, errors.New("HTTP must not be called while rendering")
	})
	eventID := int64(42)
	payload, err := service.payloadForClaim(context.Background(), model.AlertDeliveryClaim{Delivery: model.AlertDelivery{
		AlertEventID: &eventID, EventType: model.AlertDeliveryEventFiring,
		PayloadSnapshot: []byte(`{"version":1,"kind":"legacy","alert_event_id":"42","event_type":"firing"}`),
	}})
	if err != nil || !strings.Contains(payload.Markdown.Text, "42") || !strings.Contains(payload.Markdown.Text, "firing") {
		t.Fatalf("legacy payload = %#v, %v", payload, err)
	}
}

func newDingTalkUnitService(
	t *testing.T,
	now time.Time,
	roundTrip dingTalkRoundTripFunc,
) *DingTalkService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	service, err := NewDingTalkService(DingTalkServiceOptions{
		Database: &gorm.DB{}, Clock: testsupport.NewFakeClock(now), Cipher: cipher,
		HTTPClient: &http.Client{Transport: roundTrip}, PublicOrigin: "https://pilot.example.com",
	})
	if err != nil {
		t.Fatalf("create dingtalk service: %v", err)
	}
	return service
}

func dingTalkHTTPResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{
		StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)),
	}
}
