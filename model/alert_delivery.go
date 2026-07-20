package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAlertDeliveryUnavailable = errors.New("alert delivery unavailable")
	ErrAlertDeliveryLeaseLost   = errors.New("alert delivery lease lost")
	ErrDingTalkSettingInvalid   = errors.New("dingtalk setting contract is invalid")
)

const (
	AlertDeliveryEventFiring   = "firing"
	AlertDeliveryEventResolved = "resolved"
	AlertDeliveryEventTest     = "test"
	AlertDeliveryChannel       = "dingtalk"

	AlertDeliveryStatusPending = "pending"
	AlertDeliveryStatusSuccess = "success"
	AlertDeliveryStatusFailed  = "failed"
	AlertDeliveryMaxAttempts   = 5
)

type DingTalkSettingValues struct {
	Enabled           bool
	WebhookCiphertext string
	SecretCiphertext  string
}

type AlertDeliveryClaim struct {
	Delivery       AlertDelivery
	ClaimToken     string
	LeaseExpiresAt int64
	Takeover       bool
}

type AlertDeliveryCompletion struct {
	DeliveryID      int64
	AttemptCount    int
	ClaimToken      string
	Status          string
	ErrorCode       string
	ResponseCode    *int
	ResponseMessage *string
	NextRetryAt     *int64
	SentAt          *int64
	Now             int64
}

type AlertDeliveryRepository struct{ db *gorm.DB }

func NewAlertDeliveryRepository(db *gorm.DB) *AlertDeliveryRepository {
	return &AlertDeliveryRepository{db: db}
}

func (repository *AlertDeliveryRepository) Transaction(
	ctx context.Context,
	callback func(*AlertDeliveryRepository) error,
) error {
	if repository == nil || repository.db == nil || callback == nil {
		return errors.New("alert delivery repository transaction dependencies are required")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return callback(NewAlertDeliveryRepository(tx))
	})
}

func (repository *AlertDeliveryRepository) LoadDingTalkSettings(ctx context.Context) (DingTalkSettingValues, error) {
	if repository == nil || repository.db == nil {
		return DingTalkSettingValues{}, errors.New("alert delivery repository is required")
	}
	type settingRow struct {
		Key    string `gorm:"column:setting_key"`
		Value  string `gorm:"column:setting_value"`
		Type   string `gorm:"column:value_type"`
		Secret bool   `gorm:"column:is_secret"`
	}
	keys := []string{
		"notification.dingtalk.enabled",
		"notification.dingtalk.webhook",
		"notification.dingtalk.secret",
	}
	var rows []settingRow
	if err := repository.db.WithContext(ctx).Table("platform_setting").
		Select("setting_key, setting_value, value_type, is_secret").
		Where("setting_key IN ?", keys).Find(&rows).Error; err != nil {
		return DingTalkSettingValues{}, fmt.Errorf("load dingtalk settings: %w", err)
	}
	values := make(map[string]settingRow, len(rows))
	for _, row := range rows {
		values[row.Key] = row
	}
	enabled, enabledOK := values[keys[0]]
	webhook, webhookOK := values[keys[1]]
	secret, secretOK := values[keys[2]]
	if !enabledOK || !webhookOK || !secretOK || enabled.Type != "bool" || enabled.Secret ||
		webhook.Type != "string" || !webhook.Secret || secret.Type != "string" || !secret.Secret ||
		(enabled.Value != "true" && enabled.Value != "false") {
		return DingTalkSettingValues{}, ErrDingTalkSettingInvalid
	}
	return DingTalkSettingValues{
		Enabled: enabled.Value == "true", WebhookCiphertext: webhook.Value, SecretCiphertext: secret.Value,
	}, nil
}

func (repository *AlertDeliveryRepository) LockDingTalkEnabled(ctx context.Context) (bool, error) {
	type settingRow struct {
		Value  string `gorm:"column:setting_value"`
		Type   string `gorm:"column:value_type"`
		Secret bool   `gorm:"column:is_secret"`
	}
	var row settingRow
	err := repository.db.WithContext(ctx).Table("platform_setting").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("setting_value, value_type, is_secret").
		Where("setting_key = ?", "notification.dingtalk.enabled").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, ErrDingTalkSettingInvalid
	}
	if err != nil {
		return false, fmt.Errorf("lock dingtalk enabled setting: %w", err)
	}
	if row.Type != "bool" || row.Secret || (row.Value != "true" && row.Value != "false") {
		return false, ErrDingTalkSettingInvalid
	}
	return row.Value == "true", nil
}

func (repository *AlertDeliveryRepository) Enqueue(
	ctx context.Context,
	alertEventID *int64,
	eventType string,
	payloadSnapshot []byte,
	now int64,
) (AlertDelivery, bool, error) {
	if repository == nil || repository.db == nil || now <= 0 ||
		(eventType != AlertDeliveryEventFiring && eventType != AlertDeliveryEventResolved && eventType != AlertDeliveryEventTest) ||
		(eventType == AlertDeliveryEventTest && alertEventID != nil) ||
		(eventType != AlertDeliveryEventTest && (alertEventID == nil || *alertEventID <= 0)) ||
		!validAlertDeliverySnapshot(payloadSnapshot) {
		return AlertDelivery{}, false, errors.New("invalid alert delivery enqueue request")
	}
	delivery := AlertDelivery{
		AlertEventID: alertEventID, EventType: eventType, Channel: AlertDeliveryChannel,
		Status: AlertDeliveryStatusPending, PayloadSnapshot: append([]byte(nil), payloadSnapshot...),
		NextRetryAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	result := repository.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery)
	if result.Error != nil {
		return AlertDelivery{}, false, fmt.Errorf("enqueue alert delivery: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return delivery, false, nil
	}
	if alertEventID == nil {
		return AlertDelivery{}, false, errors.New("test alert delivery unexpectedly deduplicated")
	}
	err := repository.db.WithContext(ctx).
		Where("alert_event_id = ? AND event_type = ? AND channel = ?", *alertEventID, eventType, AlertDeliveryChannel).
		First(&delivery).Error
	if err != nil {
		return AlertDelivery{}, false, fmt.Errorf("read deduplicated alert delivery: %w", err)
	}
	return delivery, true, nil
}

func (repository *AlertDeliveryRepository) DisablePending(ctx context.Context, now int64) (int64, error) {
	if repository == nil || repository.db == nil || now <= 0 {
		return 0, errors.New("invalid disable pending deliveries request")
	}
	message := "notification disabled"
	result := repository.db.WithContext(ctx).Model(&AlertDelivery{}).
		Where("status = ?", AlertDeliveryStatusPending).
		Updates(map[string]any{
			"status": AlertDeliveryStatusFailed, "error_code": "NOTIFICATION_DISABLED",
			"response_code": nil, "response_message": message, "next_retry_at": nil,
			"claim_token": nil, "lease_expires_at": nil, "updated_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("disable pending alert deliveries: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (repository *AlertDeliveryRepository) FailExhaustedLeases(ctx context.Context, now int64) (int64, error) {
	if repository == nil || repository.db == nil || now <= 0 {
		return 0, errors.New("invalid exhausted alert delivery request")
	}
	message := "delivery retry budget exhausted"
	result := repository.db.WithContext(ctx).Model(&AlertDelivery{}).
		Where("status = ? AND claim_token IS NULL AND attempt_count >= ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?",
			AlertDeliveryStatusPending, AlertDeliveryMaxAttempts, now).
		Updates(map[string]any{
			"status": AlertDeliveryStatusFailed, "error_code": "DELIVERY_RETRY_EXHAUSTED",
			"response_message": message, "next_retry_at": nil, "claim_token": nil,
			"lease_expires_at": nil, "updated_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("fail exhausted alert delivery leases: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (repository *AlertDeliveryRepository) ClaimNext(
	ctx context.Context,
	now int64,
	leaseUntil int64,
) (AlertDeliveryClaim, error) {
	return repository.claim(ctx, nil, now, leaseUntil)
}

func (repository *AlertDeliveryRepository) ClaimByID(
	ctx context.Context,
	id int64,
	now int64,
	leaseUntil int64,
) (AlertDeliveryClaim, error) {
	if id <= 0 {
		return AlertDeliveryClaim{}, errors.New("invalid alert delivery ID")
	}
	return repository.claim(ctx, &id, now, leaseUntil)
}

func (repository *AlertDeliveryRepository) claim(
	ctx context.Context,
	id *int64,
	now int64,
	leaseUntil int64,
) (AlertDeliveryClaim, error) {
	if repository == nil || repository.db == nil || now <= 0 || leaseUntil <= now {
		return AlertDeliveryClaim{}, errors.New("invalid alert delivery claim request")
	}
	var claim AlertDeliveryClaim
	err := repository.Transaction(ctx, func(txRepository *AlertDeliveryRepository) error {
		var delivery AlertDelivery
		query := txRepository.db.WithContext(ctx).Table("alert_delivery AS candidate").Select("candidate.*").
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(`candidate.status = ? AND (
  (candidate.claim_token IS NULL AND candidate.attempt_count < ?
    AND candidate.next_retry_at IS NOT NULL AND candidate.next_retry_at <= ?)
  OR
  (candidate.claim_token IS NOT NULL AND candidate.attempt_count BETWEEN 1 AND ?
    AND candidate.lease_expires_at IS NOT NULL AND candidate.lease_expires_at <= ?)
)`, AlertDeliveryStatusPending, AlertDeliveryMaxAttempts, now, AlertDeliveryMaxAttempts, now).
			Where(`candidate.event_type <> ? OR NOT EXISTS (
  SELECT 1 FROM alert_delivery firing
  WHERE firing.alert_event_id = candidate.alert_event_id
    AND firing.event_type = ? AND firing.channel = candidate.channel
    AND firing.status = ?
)`, AlertDeliveryEventResolved, AlertDeliveryEventFiring, AlertDeliveryStatusPending)
		if id != nil {
			query = query.Where("candidate.id = ?", *id)
		} else {
			query = query.Order("COALESCE(candidate.lease_expires_at, candidate.next_retry_at) ASC, candidate.id ASC")
		}
		err := query.Take(&delivery).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAlertDeliveryUnavailable
		}
		if err != nil {
			return fmt.Errorf("claim alert delivery: %w", err)
		}
		token, err := newAlertDeliveryClaimToken()
		if err != nil {
			return err
		}
		takeover := delivery.ClaimToken != nil
		attempt := delivery.AttemptCount
		update := txRepository.db.WithContext(ctx).Model(&AlertDelivery{}).
			Where("id = ? AND status = ? AND attempt_count = ?", delivery.ID, AlertDeliveryStatusPending, delivery.AttemptCount)
		if takeover {
			update = update.Where("claim_token = ? AND lease_expires_at = ?", *delivery.ClaimToken, *delivery.LeaseExpiresAt)
		} else {
			attempt++
			update = update.Where("claim_token IS NULL AND lease_expires_at IS NULL AND next_retry_at = ?", *delivery.NextRetryAt)
		}
		result := update.Updates(map[string]any{
			"attempt_count": attempt, "claim_token": token, "lease_expires_at": leaseUntil,
			"next_retry_at": nil, "updated_at": now,
		})
		if result.Error != nil {
			return fmt.Errorf("lease alert delivery: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAlertDeliveryUnavailable
		}
		delivery.AttemptCount, delivery.ClaimToken, delivery.LeaseExpiresAt = attempt, &token, &leaseUntil
		delivery.NextRetryAt, delivery.UpdatedAt = nil, now
		claim = AlertDeliveryClaim{
			Delivery: delivery, ClaimToken: token, LeaseExpiresAt: leaseUntil, Takeover: takeover,
		}
		return nil
	})
	return claim, err
}

func (repository *AlertDeliveryRepository) Complete(
	ctx context.Context,
	completion AlertDeliveryCompletion,
) error {
	if repository == nil || repository.db == nil || completion.DeliveryID <= 0 || completion.AttemptCount <= 0 ||
		completion.ClaimToken == "" || completion.Now <= 0 ||
		(completion.Status != AlertDeliveryStatusPending && completion.Status != AlertDeliveryStatusSuccess && completion.Status != AlertDeliveryStatusFailed) ||
		(completion.Status == AlertDeliveryStatusPending && completion.NextRetryAt == nil) ||
		(completion.Status != AlertDeliveryStatusPending && completion.NextRetryAt != nil) {
		return errors.New("invalid alert delivery completion")
	}
	updates := map[string]any{
		"status": completion.Status, "error_code": completion.ErrorCode,
		"response_code": completion.ResponseCode, "response_message": completion.ResponseMessage,
		"next_retry_at": completion.NextRetryAt, "claim_token": nil, "lease_expires_at": nil,
		"sent_at": completion.SentAt, "updated_at": completion.Now,
	}
	result := repository.db.WithContext(ctx).Model(&AlertDelivery{}).
		Where("id = ? AND status = ? AND attempt_count = ? AND claim_token = ?",
			completion.DeliveryID, AlertDeliveryStatusPending, completion.AttemptCount, completion.ClaimToken).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("complete alert delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAlertDeliveryLeaseLost
	}
	return nil
}

func (repository *AlertDeliveryRepository) Find(ctx context.Context, id int64) (AlertDelivery, error) {
	if repository == nil || repository.db == nil || id <= 0 {
		return AlertDelivery{}, errors.New("invalid alert delivery lookup")
	}
	var delivery AlertDelivery
	err := repository.db.WithContext(ctx).Where("id = ?", id).First(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AlertDelivery{}, ErrAlertRecordNotFound
	}
	if err != nil {
		return AlertDelivery{}, fmt.Errorf("find alert delivery: %w", err)
	}
	return delivery, nil
}

func (repository *AlertRepository) LockDingTalkEnabled(ctx context.Context) (bool, error) {
	return NewAlertDeliveryRepository(repository.db).LockDingTalkEnabled(ctx)
}

func (repository *AlertRepository) EnqueueDelivery(
	ctx context.Context,
	alertEventID int64,
	eventType string,
	payloadSnapshot []byte,
	now int64,
) (AlertDelivery, bool, error) {
	return NewAlertDeliveryRepository(repository.db).Enqueue(ctx, &alertEventID, eventType, payloadSnapshot, now)
}

func newAlertDeliveryClaimToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate alert delivery claim token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func validAlertDeliverySnapshot(value []byte) bool {
	if len(value) == 0 || len(value) > 64*1024 || !json.Valid(value) {
		return false
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false
	}
	return !alertDeliverySnapshotHasSensitiveKey(decoded)
}

func alertDeliverySnapshotHasSensitiveKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			switch normalized {
			case "webhook", "webhook_url", "secret", "sign", "signature", "access_token", "password", "encryption_key":
				return true
			}
			if alertDeliverySnapshotHasSensitiveKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if alertDeliverySnapshotHasSensitiveKey(child) {
				return true
			}
		}
	}
	return false
}
