package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const (
	dingTalkHTTPTimeout       = 10 * time.Second
	dingTalkDeliveryLease     = 30 * time.Second
	dingTalkRetryAfterMaximum = time.Hour
	dingTalkResponseLimit     = 64 * 1024
	dingTalkMessageLimit      = 2000
)

var (
	ErrDingTalkServiceDependencies = errors.New("dingtalk service dependencies are required")
	errDingTalkRedirectForbidden   = errors.New("dingtalk redirect is forbidden")
)

type DingTalkRequestIDGenerator func() (string, error)

type DingTalkServiceOptions struct {
	Database           *gorm.DB
	Clock              common.Clock
	Cipher             *common.Cipher
	HTTPClient         *http.Client
	AllowedHosts       []string
	PublicOrigin       string
	PollInterval       time.Duration
	RequestIDGenerator DingTalkRequestIDGenerator
	Metrics            AlertDeliveryMetricsRecorder
}

type DingTalkService struct {
	deliveries   *model.AlertDeliveryRepository
	alerts       *model.AlertRepository
	clock        common.Clock
	cipher       *common.Cipher
	httpClient   *http.Client
	allowedHosts map[string]struct{}
	publicOrigin string
	pollInterval time.Duration
	requestID    DingTalkRequestIDGenerator
	metrics      AlertDeliveryMetricsRecorder
}

type dingTalkPreparedConfig struct {
	webhook *url.URL
	secret  string
}

type dingTalkPreflightFailure struct {
	code constant.MessageCode
}

type dingTalkAttemptResult struct {
	success         bool
	retryable       bool
	retryAfter      time.Duration
	hasRetryAfter   bool
	errorCode       constant.MessageCode
	errCode         string
	responseCode    *int
	responseMessage string
}

type dingTalkMarkdownPayload struct {
	MessageType string               `json:"msgtype"`
	Markdown    dingTalkMarkdownBody `json:"markdown"`
}

type dingTalkMarkdownBody struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type dingTalkDeliverySnapshot struct {
	Version        int     `json:"version"`
	Kind           string  `json:"kind"`
	AlertEventID   string  `json:"alert_event_id,omitempty"`
	EventType      string  `json:"event_type,omitempty"`
	RuleKey        string  `json:"rule_key,omitempty"`
	Level          string  `json:"level,omitempty"`
	SiteName       string  `json:"site_name,omitempty"`
	TargetName     string  `json:"target_name,omitempty"`
	CurrentValue   *string `json:"current_value,omitempty"`
	ThresholdValue *string `json:"threshold_value,omitempty"`
	FirstFiredAt   *int64  `json:"first_fired_at,omitempty"`
	ResolvedAt     *int64  `json:"resolved_at,omitempty"`
}

func NewDingTalkService(options DingTalkServiceOptions) (*DingTalkService, error) {
	if options.Database == nil || options.Clock == nil || options.Cipher == nil {
		return nil, ErrDingTalkServiceDependencies
	}
	allowedHosts, err := normalizeDingTalkAllowedHosts(options.AllowedHosts)
	if err != nil {
		return nil, err
	}
	publicOrigin, err := normalizeNotificationOrigin(options.PublicOrigin)
	if err != nil {
		return nil, err
	}
	client := http.DefaultClient
	if options.HTTPClient != nil {
		client = options.HTTPClient
	}
	clientCopy := *client
	clientCopy.Timeout = dingTalkHTTPTimeout
	clientCopy.CheckRedirect = dingTalkRedirectPolicy(allowedHosts)
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.RequestIDGenerator == nil {
		options.RequestIDGenerator = newDingTalkRequestID
	}
	return &DingTalkService{
		deliveries: model.NewAlertDeliveryRepository(options.Database), alerts: model.NewAlertRepository(options.Database),
		clock: options.Clock, cipher: options.Cipher, httpClient: &clientCopy, allowedHosts: allowedHosts,
		publicOrigin: publicOrigin, pollInterval: options.PollInterval, requestID: options.RequestIDGenerator,
		metrics: options.Metrics,
	}, nil
}

// Recover performs the durable queue checks required before the alert runtime
// can report ready. It deliberately does not send external requests.
func (service *DingTalkService) Recover(ctx context.Context) error {
	_, preflight, err := service.prepare(ctx)
	if err != nil {
		return err
	}
	now := service.clock.Now().Unix()
	if now <= 0 {
		return errors.New("dingtalk recovery clock is invalid")
	}
	if preflight != nil && preflight.code == constant.MessageNotificationDisabled {
		changed, err := service.deliveries.DisablePending(ctx, now)
		if err != nil {
			return err
		}
		service.addDeliveryMetrics("disabled", changed)
	}
	exhausted, err := service.deliveries.FailExhaustedLeases(ctx, now)
	if err == nil {
		service.addDeliveryMetrics("exhausted", exhausted)
	}
	return err
}

func (service *DingTalkService) Run(ctx context.Context) error {
	for {
		processed, err := service.ProcessNext(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if processed {
			continue
		}
		ticker := service.clock.NewTimer(service.pollInterval)
		select {
		case <-ctx.Done():
			ticker.Stop()
			return nil
		case <-ticker.C():
			ticker.Stop()
		}
	}
}

func (service *DingTalkService) ProcessNext(ctx context.Context) (bool, error) {
	prepared, preflight, err := service.prepare(ctx)
	if err != nil {
		return false, err
	}
	now := service.clock.Now().Unix()
	if preflight != nil && preflight.code == constant.MessageNotificationDisabled {
		changed, disableErr := service.deliveries.DisablePending(ctx, now)
		if disableErr == nil {
			service.addDeliveryMetrics("disabled", changed)
		}
		return changed > 0, disableErr
	}
	exhausted, err := service.deliveries.FailExhaustedLeases(ctx, now)
	if err != nil {
		return false, err
	}
	service.addDeliveryMetrics("exhausted", exhausted)
	claim, err := service.deliveries.ClaimNext(ctx, now, service.clock.Now().Add(dingTalkDeliveryLease).Unix())
	if errors.Is(err, model.ErrAlertDeliveryUnavailable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if claim.Takeover {
		service.recordDeliveryMetric("takeover")
	}
	requestID, err := service.requestID()
	if err != nil || !validDingTalkRequestID(requestID) {
		return true, errors.New("generate dingtalk request ID")
	}
	if preflight != nil {
		result := dingTalkAttemptResult{errorCode: preflight.code, responseMessage: safeDeliveryMessage(preflight.code)}
		return true, service.completeAttempt(claim, result)
	}
	payload, err := service.payloadForClaim(ctx, claim)
	if err != nil {
		return true, err
	}
	result := service.send(ctx, prepared, payload, requestID)
	return true, service.completeAttempt(claim, result)
}

func (service *DingTalkService) Test(ctx context.Context, requestID string) (dto.NotificationTestResult, error) {
	if !validDingTalkRequestID(requestID) {
		return dto.NotificationTestResult{}, errors.New("invalid dingtalk test request ID")
	}
	prepared, preflight, err := service.prepare(ctx)
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	if preflight != nil {
		message, messageErr := notificationMessage(preflight.code, nil, nil, "", nil)
		if messageErr != nil {
			return dto.NotificationTestResult{}, messageErr
		}
		return dto.NotificationTestResult{Status: model.AlertDeliveryStatusFailed, Message: message}, nil
	}
	now := service.clock.Now().Unix()
	payload, err := marshalDingTalkDeliverySnapshot(dingTalkDeliverySnapshot{Version: 1, Kind: "test"})
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	delivery, _, err := service.deliveries.Enqueue(ctx, nil, model.AlertDeliveryEventTest, payload, now)
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	claim, err := service.deliveries.ClaimByID(ctx, delivery.ID, now, service.clock.Now().Add(dingTalkDeliveryLease).Unix())
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	result := service.send(ctx, prepared, dingTalkTestPayload(), requestID)
	if err := service.completeAttempt(claim, result); err != nil {
		return dto.NotificationTestResult{}, err
	}
	deliveryID := strconv.FormatInt(delivery.ID, 10)
	if result.success {
		message, err := notificationMessage(constant.MessageNotificationTestSucceeded, nil, &deliveryID, "", nil)
		if err != nil {
			return dto.NotificationTestResult{}, err
		}
		return dto.NotificationTestResult{
			DeliveryID: &deliveryID, Status: model.AlertDeliveryStatusSuccess,
			ResponseCode: result.responseCode, Message: message,
		}, nil
	}
	persisted, err := service.deliveries.Find(ctx, delivery.ID)
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	code := constant.MessageCode(persisted.ErrorCode)
	var nextRetryAt *int64
	if persisted.Status == model.AlertDeliveryStatusPending && persisted.NextRetryAt != nil {
		code, nextRetryAt = constant.MessageDeliveryRetryScheduled, persisted.NextRetryAt
	}
	message, err := notificationMessage(code, nil, &deliveryID, result.errCode, nextRetryAt)
	if err != nil {
		return dto.NotificationTestResult{}, err
	}
	return dto.NotificationTestResult{
		DeliveryID: &deliveryID, Status: model.AlertDeliveryStatusFailed,
		ResponseCode: result.responseCode, Message: message,
	}, nil
}

func (service *DingTalkService) prepare(ctx context.Context) (*dingTalkPreparedConfig, *dingTalkPreflightFailure, error) {
	settings, err := service.deliveries.LoadDingTalkSettings(ctx)
	if errors.Is(err, model.ErrDingTalkSettingInvalid) {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !settings.Enabled {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationDisabled}, nil
	}
	if settings.WebhookCiphertext == "" {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	webhookBytes, err := service.cipher.Decrypt(
		settings.WebhookCiphertext,
		"setting:notification.dingtalk.webhook",
	)
	if err != nil {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	webhook, err := validateDingTalkWebhook(string(webhookBytes), service.allowedHosts)
	if err != nil {
		return nil, &dingTalkPreflightFailure{code: constant.MessageDingTalkAddressForbidden}, nil
	}
	if settings.SecretCiphertext == "" {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	secretBytes, decryptErr := service.cipher.Decrypt(
		settings.SecretCiphertext,
		"setting:notification.dingtalk.secret",
	)
	if decryptErr != nil {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	secret := string(secretBytes)
	if secret == "" || len(secret) > 1024 || strings.ContainsAny(secret, "\x00\r\n") {
		return nil, &dingTalkPreflightFailure{code: constant.MessageNotificationNotConfigured}, nil
	}
	return &dingTalkPreparedConfig{webhook: webhook, secret: secret}, nil, nil
}

func (service *DingTalkService) payloadForClaim(
	_ context.Context,
	claim model.AlertDeliveryClaim,
) (dingTalkMarkdownPayload, error) {
	var snapshot dingTalkDeliverySnapshot
	if err := json.Unmarshal(claim.Delivery.PayloadSnapshot, &snapshot); err != nil || snapshot.Version != 1 {
		return dingTalkMarkdownPayload{}, errors.New("invalid notification payload snapshot")
	}
	if snapshot.Kind == "test" && claim.Delivery.EventType == model.AlertDeliveryEventTest {
		return dingTalkTestPayload(), nil
	}
	if snapshot.Kind == "legacy" {
		return dingTalkLegacyPayload(snapshot), nil
	}
	if snapshot.Kind != "alert" || claim.Delivery.AlertEventID == nil ||
		snapshot.AlertEventID != strconv.FormatInt(*claim.Delivery.AlertEventID, 10) ||
		snapshot.EventType != claim.Delivery.EventType {
		return dingTalkMarkdownPayload{}, errors.New("notification payload snapshot does not match delivery")
	}
	return service.alertPayload(snapshot), nil
}

func (service *DingTalkService) alertPayload(event dingTalkDeliverySnapshot) dingTalkMarkdownPayload {
	severity := "[Warning]"
	if event.Level == dto.AlertLevelCritical {
		severity = "[Critical]"
	}
	title := severity + " " + alertChineseName(event.RuleKey)
	state := "告警触发"
	eventAt := event.FirstFiredAt
	if event.EventType == model.AlertDeliveryEventResolved {
		state = "告警恢复"
		eventAt = event.ResolvedAt
		title += " 已恢复"
	}
	lines := []string{
		"### " + dingTalkMarkdownEscape(title),
		"- 状态：" + state,
		"- 站点：" + dingTalkMarkdownEscape(emptyAsUnknown(event.SiteName)),
		"- 目标：" + dingTalkMarkdownEscape(emptyAsUnknown(event.TargetName)),
		"- 指标：" + dingTalkMarkdownEscape(alertChineseName(event.RuleKey)),
		"- 当前值：" + dingTalkMarkdownEscape(pointerValue(event.CurrentValue)),
		"- 阈值：" + dingTalkMarkdownEscape(pointerValue(event.ThresholdValue)),
		"- 时间：" + formatNotificationTime(eventAt),
	}
	if event.EventType == model.AlertDeliveryEventResolved && event.FirstFiredAt != nil && event.ResolvedAt != nil {
		duration := *event.ResolvedAt - *event.FirstFiredAt
		if duration < 0 {
			duration = 0
		}
		lines = append(lines, "- 持续时长："+formatNotificationDuration(duration))
	}
	if service.publicOrigin != "" {
		link := service.publicOrigin + "/alerts?alertId=" + event.AlertEventID
		lines = append(lines, "- [查看详情]("+link+")")
	}
	return dingTalkMarkdownPayload{
		MessageType: "markdown",
		Markdown:    dingTalkMarkdownBody{Title: title, Text: strings.Join(lines, "\n")},
	}
}

func newDingTalkAlertSnapshot(event model.AlertEventView, eventType string) dingTalkDeliverySnapshot {
	return dingTalkDeliverySnapshot{
		Version: 1, Kind: "alert", AlertEventID: strconv.FormatInt(event.ID, 10), EventType: eventType,
		RuleKey: event.RuleKey, Level: event.Level, SiteName: event.SiteName, TargetName: event.TargetName,
		CurrentValue: event.CurrentValue, ThresholdValue: event.ThresholdValue,
		FirstFiredAt: event.FirstFiredAt, ResolvedAt: event.ResolvedAt,
	}
}

func marshalDingTalkDeliverySnapshot(snapshot dingTalkDeliverySnapshot) ([]byte, error) {
	value, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal dingtalk delivery snapshot: %w", err)
	}
	return value, nil
}

func dingTalkLegacyPayload(snapshot dingTalkDeliverySnapshot) dingTalkMarkdownPayload {
	title := "[Warning] 历史告警通知"
	text := "### " + title + "\n- 告警事件：" + snapshot.AlertEventID +
		"\n- 状态：" + dingTalkMarkdownEscape(snapshot.EventType)
	return dingTalkMarkdownPayload{
		MessageType: "markdown", Markdown: dingTalkMarkdownBody{Title: title, Text: text},
	}
}

func dingTalkTestPayload() dingTalkMarkdownPayload {
	return dingTalkMarkdownPayload{
		MessageType: "markdown",
		Markdown: dingTalkMarkdownBody{
			Title: "钉钉通知测试",
			Text:  "### 钉钉通知测试\n\n平台通知链路工作正常。",
		},
	}
}

func (service *DingTalkService) send(
	ctx context.Context,
	prepared *dingTalkPreparedConfig,
	payload dingTalkMarkdownPayload,
	requestID string,
) dingTalkAttemptResult {
	if prepared == nil || prepared.webhook == nil || !validDingTalkRequestID(requestID) {
		return dingTalkAttemptResult{
			errorCode:       constant.MessageNotificationNotConfigured,
			responseMessage: safeDeliveryMessage(constant.MessageNotificationNotConfigured),
		}
	}
	endpoint := *prepared.webhook
	if prepared.secret != "" {
		timestamp := strconv.FormatInt(service.clock.Now().UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte(prepared.secret))
		_, _ = mac.Write([]byte(timestamp + "\n" + prepared.secret))
		query := endpoint.Query()
		query.Set("timestamp", timestamp)
		query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		endpoint.RawQuery = query.Encode()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return dingTalkAttemptResult{errorCode: constant.MessageDingTalkRejected, errCode: "INVALID_PAYLOAD", responseMessage: "invalid notification payload"}
	}
	requestContext, cancel := context.WithTimeout(ctx, dingTalkHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return dingTalkAttemptResult{errorCode: constant.MessageDingTalkAddressForbidden, responseMessage: "invalid notification address"}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	response, err := service.httpClient.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if errors.Is(err, errDingTalkRedirectForbidden) {
			return dingTalkAttemptResult{errorCode: constant.MessageDingTalkAddressForbidden, responseMessage: "notification redirect forbidden"}
		}
		return dingTalkAttemptResult{retryable: true, responseMessage: "notification network request failed"}
	}
	defer response.Body.Close()
	status := response.StatusCode
	responseCode := status
	if status == http.StatusTooManyRequests {
		retryAfter, present := parseDingTalkRetryAfter(response.Header.Get("Retry-After"), service.clock.Now())
		return dingTalkAttemptResult{
			retryable: true, retryAfter: retryAfter, hasRetryAfter: present, responseCode: &responseCode,
			responseMessage: "HTTP 429",
		}
	}
	if status >= http.StatusInternalServerError {
		return dingTalkAttemptResult{retryable: true, responseCode: &responseCode, responseMessage: "HTTP " + strconv.Itoa(status)}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return dingTalkAttemptResult{
			errorCode: constant.MessageDingTalkRejected, errCode: "HTTP_" + strconv.Itoa(status),
			responseCode: &responseCode, responseMessage: "HTTP " + strconv.Itoa(status),
		}
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, dingTalkResponseLimit+1))
	if readErr != nil {
		return dingTalkAttemptResult{
			errorCode: constant.MessageDingTalkRejected, errCode: "INVALID_RESPONSE",
			responseCode: &responseCode, responseMessage: "invalid notification response",
		}
	}
	if len(responseBody) > dingTalkResponseLimit {
		return dingTalkAttemptResult{
			errorCode: constant.MessageDingTalkRejected, errCode: "RESPONSE_TOO_LARGE",
			responseCode: &responseCode, responseMessage: "notification response too large",
		}
	}
	errCode, valid := decodeDingTalkErrCode(responseBody)
	if !valid {
		return dingTalkAttemptResult{
			errorCode: constant.MessageDingTalkRejected, errCode: "INVALID_RESPONSE",
			responseCode: &responseCode, responseMessage: "invalid notification response",
		}
	}
	if errCode != "0" {
		return dingTalkAttemptResult{
			retryable: true, errorCode: constant.MessageDingTalkRejected, errCode: errCode,
			responseCode: &responseCode, responseMessage: safeDingTalkErrCodeMessage(status, errCode),
		}
	}
	return dingTalkAttemptResult{
		success: true, responseCode: &responseCode,
		responseMessage: "HTTP " + strconv.Itoa(status) + "; errcode=0",
	}
}

func (service *DingTalkService) completeAttempt(
	claim model.AlertDeliveryClaim,
	result dingTalkAttemptResult,
) error {
	now := service.clock.Now().Unix()
	message := truncateDeliveryMessage(result.responseMessage)
	completion := model.AlertDeliveryCompletion{
		DeliveryID: claim.Delivery.ID, AttemptCount: claim.Delivery.AttemptCount,
		ClaimToken: claim.ClaimToken, ResponseCode: result.responseCode, ResponseMessage: &message, Now: now,
	}
	if result.success {
		completion.Status, completion.SentAt = model.AlertDeliveryStatusSuccess, &now
		err := service.deliveries.Complete(context.Background(), completion)
		if err == nil {
			service.recordDeliveryMetric("success")
		}
		return err
	}
	if result.retryable && claim.Delivery.AttemptCount < model.AlertDeliveryMaxAttempts {
		delay := dingTalkRetryDelay(claim.Delivery.AttemptCount)
		if result.hasRetryAfter {
			delay = result.retryAfter
			if delay > dingTalkRetryAfterMaximum {
				delay = dingTalkRetryAfterMaximum
			}
		}
		next := service.clock.Now().Add(delay).Unix()
		completion.Status, completion.NextRetryAt = model.AlertDeliveryStatusPending, &next
		completion.ErrorCode = string(result.errorCode)
		err := service.deliveries.Complete(context.Background(), completion)
		if err == nil {
			service.recordDeliveryMetric("retry")
		}
		return err
	}
	completion.Status = model.AlertDeliveryStatusFailed
	if result.retryable {
		completion.ErrorCode = string(constant.MessageDeliveryRetryExhausted)
	} else if result.errorCode != "" {
		completion.ErrorCode = string(result.errorCode)
	} else {
		completion.ErrorCode = string(constant.MessageDingTalkRejected)
	}
	err := service.deliveries.Complete(context.Background(), completion)
	if err == nil {
		metricResult := "failed"
		if result.retryable {
			metricResult = "exhausted"
		}
		service.recordDeliveryMetric(metricResult)
	}
	return err
}

func (service *DingTalkService) recordDeliveryMetric(result string) {
	if service == nil || service.metrics == nil {
		return
	}
	recordServiceMetric(func() {
		service.metrics.IncrementAlertDelivery(model.AlertDeliveryChannel, result)
	})
}

func (service *DingTalkService) addDeliveryMetrics(result string, count int64) {
	if service == nil || service.metrics == nil || count <= 0 {
		return
	}
	recordServiceMetric(func() {
		service.metrics.AddAlertDeliveries(model.AlertDeliveryChannel, result, float64(count))
	})
}

func notificationMessage(
	code constant.MessageCode,
	alertEventID *string,
	deliveryID *string,
	errCode string,
	nextRetryAt *int64,
) (dto.MessageRef, error) {
	params := map[string]any{}
	switch code {
	case constant.MessageNotificationDisabled, constant.MessageNotificationNotConfigured, constant.MessageDingTalkAddressForbidden:
		params["alert_event_id"], params["delivery_id"] = nullableString(alertEventID), nullableString(deliveryID)
	case constant.MessageNotificationTestSucceeded:
		if deliveryID == nil {
			return dto.MessageRef{}, errors.New("notification success delivery ID is required")
		}
		params["delivery_id"] = *deliveryID
	case constant.MessageDingTalkRejected:
		if deliveryID == nil || errCode == "" {
			return dto.MessageRef{}, errors.New("dingtalk rejection evidence is required")
		}
		params["alert_event_id"], params["delivery_id"], params["errcode"] = nullableString(alertEventID), *deliveryID, errCode
	case constant.MessageDeliveryRetryExhausted:
		if deliveryID == nil {
			return dto.MessageRef{}, errors.New("delivery retry evidence is required")
		}
		params["alert_event_id"], params["delivery_id"] = nullableString(alertEventID), *deliveryID
	case constant.MessageDeliveryRetryScheduled:
		if deliveryID == nil || nextRetryAt == nil || *nextRetryAt < 0 {
			return dto.MessageRef{}, errors.New("scheduled delivery retry evidence is required")
		}
		params["delivery_id"] = *deliveryID
		params["next_retry_at"] = strconv.FormatInt(*nextRetryAt, 10)
	default:
		return dto.MessageRef{}, errors.New("unsupported notification message code")
	}
	return dto.NewMessageRef(code, params, "")
}

func normalizeDingTalkAllowedHosts(values []string) (map[string]struct{}, error) {
	result := map[string]struct{}{"oapi.dingtalk.com": {}}
	for _, value := range values {
		host, err := normalizeDingTalkHost(value)
		if err != nil {
			return nil, err
		}
		result[host] = struct{}{}
	}
	return result, nil
}

func normalizeDingTalkHost(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/:@") {
		return "", errors.New("invalid dingtalk allowed host")
	}
	host, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(value), "."))
	if err != nil || host == "" || net.ParseIP(host) != nil {
		return "", errors.New("invalid dingtalk allowed host")
	}
	return host, nil
}

func validateDingTalkWebhook(raw string, allowed map[string]struct{}) (*url.URL, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || len(raw) > 4096 {
		return nil, errors.New("invalid dingtalk webhook")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" ||
		parsed.Fragment != "" || parsed.Path == "" {
		return nil, errors.New("invalid dingtalk webhook")
	}
	host, err := normalizeDingTalkHost(parsed.Hostname())
	if err != nil {
		return nil, errors.New("invalid dingtalk webhook")
	}
	if _, exists := allowed[host]; !exists {
		return nil, errors.New("dingtalk webhook host is forbidden")
	}
	return parsed, nil
}

func dingTalkRedirectPolicy(allowed map[string]struct{}) func(*http.Request, []*http.Request) error {
	return func(request *http.Request, via []*http.Request) error {
		if len(via) == 0 || len(via) >= 3 || request.Method != http.MethodPost {
			return errDingTalkRedirectForbidden
		}
		candidate, err := validateDingTalkWebhook(request.URL.String(), allowed)
		if err != nil {
			return errDingTalkRedirectForbidden
		}
		candidateHost, _ := normalizeDingTalkHost(candidate.Hostname())
		originalHost, err := normalizeDingTalkHost(via[0].URL.Hostname())
		if err != nil || candidateHost != originalHost {
			return errDingTalkRedirectForbidden
		}
		return nil
	}
}

func normalizeNotificationOrigin(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid notification public origin")
	}
	return strings.TrimSuffix(raw, "/"), nil
}

func parseDingTalkRetryAfter(raw string, now time.Time) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if dingTalkRetryAfterDigits(raw) {
		maximumSeconds := uint64(dingTalkRetryAfterMaximum / time.Second)
		seconds, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || seconds >= maximumSeconds {
			return dingTalkRetryAfterMaximum, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	if strings.HasPrefix(raw, "-") && dingTalkRetryAfterDigits(strings.TrimPrefix(raw, "-")) {
		return 0, false
	}
	parsed, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	duration := parsed.Sub(now)
	if duration < 0 {
		duration = 0
	}
	if duration > dingTalkRetryAfterMaximum {
		duration = dingTalkRetryAfterMaximum
	}
	return duration, true
}

func dingTalkRetryAfterDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func decodeDingTalkErrCode(body []byte) (string, bool) {
	var response struct {
		ErrCode json.RawMessage `json:"errcode"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&response); err != nil || len(response.ErrCode) == 0 {
		return "", false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", false
	}
	raw := strings.TrimSpace(string(response.ErrCode))
	if strings.HasPrefix(raw, "\"") {
		var text string
		if json.Unmarshal(response.ErrCode, &text) != nil {
			return "", false
		}
		raw = text
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || strconv.FormatInt(value, 10) != raw {
		return "", false
	}
	return raw, true
}

func dingTalkRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func newDingTalkRequestID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "dt_" + hex.EncodeToString(random[:]), nil
}

func validDingTalkRequestID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func safeDeliveryMessage(code constant.MessageCode) string {
	switch code {
	case constant.MessageNotificationDisabled:
		return "notification disabled"
	case constant.MessageNotificationNotConfigured:
		return "notification configuration unavailable"
	case constant.MessageDingTalkAddressForbidden:
		return "notification address forbidden"
	default:
		return "notification delivery failed"
	}
}

func safeDingTalkErrCodeMessage(status int, errCode string) string {
	return truncateDeliveryMessage("HTTP " + strconv.Itoa(status) + "; errcode=" + errCode)
}

func truncateDeliveryMessage(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > dingTalkMessageLimit {
		message = message[:dingTalkMessageLimit]
	}
	return message
}

func alertChineseName(ruleKey string) string {
	names := map[string]string{
		"site_offline": "站点离线", "site_auth_expired": "站点授权过期",
		"site_export_disabled": "站点未开启数据导出",
		"collection_missing":   "采集窗口缺失", "backfill_failed": "历史回填失败",
		"validation_failed": "统计校验失败", "instance_stale": "实例数据陈旧",
		"instance_offline": "实例离线", "site_no_instance": "站点无可用实例",
		"cpu_high": "CPU 使用率过高", "memory_high": "内存使用率过高", "disk_high": "磁盘使用率过高",
		"account_missing": "纳管账户不存在", "account_identity_mismatch": "纳管账户身份不匹配",
		"account_disabled": "纳管账户已禁用", "account_quota_empty": "远程账户余额为空",
		"channel_balance_low": "渠道余额过低", "channel_response_time_high": "渠道响应时间过高",
		"channel_availability_low": "渠道可用率过低",
	}
	if name := names[ruleKey]; name != "" {
		return name
	}
	return "平台告警"
}

func dingTalkMarkdownEscape(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	replacer := strings.NewReplacer(
		"\\", "\\\\", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)",
		"*", "\\*", "_", "\\_", "#", "\\#", "`", "\\`", ">", "\\>",
	)
	return replacer.Replace(value)
}

func emptyAsUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未知"
	}
	return value
}

func pointerValue(value *string) string {
	if value == nil || *value == "" {
		return "未知"
	}
	return *value
}

func formatNotificationTime(timestamp *int64) string {
	if timestamp == nil {
		return "未知"
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return time.Unix(*timestamp, 0).In(location).Format("2006-01-02 15:04:05")
}

func formatNotificationDuration(seconds int64) string {
	duration := time.Duration(seconds) * time.Second
	if duration < time.Minute {
		return strconv.FormatInt(seconds, 10) + " 秒"
	}
	if duration < time.Hour {
		return strconv.FormatInt(int64(duration/time.Minute), 10) + " 分钟"
	}
	hours := duration / time.Hour
	minutes := (duration % time.Hour) / time.Minute
	if minutes == 0 {
		return strconv.FormatInt(int64(hours), 10) + " 小时"
	}
	return strconv.FormatInt(int64(hours), 10) + " 小时 " + strconv.FormatInt(int64(minutes), 10) + " 分钟"
}
