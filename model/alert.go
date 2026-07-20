package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAlertRecordNotFound = errors.New("alert record not found")

type AlertRule struct {
	ID              int64   `gorm:"column:id;primaryKey"`
	RuleKey         string  `gorm:"column:rule_key"`
	Name            string  `gorm:"column:name"`
	Enabled         bool    `gorm:"column:enabled"`
	Level           string  `gorm:"column:level"`
	Metric          string  `gorm:"column:metric"`
	CompareOperator string  `gorm:"column:compare_operator"`
	ThresholdValue  *string `gorm:"column:threshold_value"`
	ForTimes        int     `gorm:"column:for_times"`
	ScopeType       string  `gorm:"column:scope_type"`
	ScopeID         int64   `gorm:"column:scope_id"`
	CreatedAt       int64   `gorm:"column:created_at"`
	UpdatedAt       int64   `gorm:"column:updated_at"`
}

func (AlertRule) TableName() string { return "alert_rule" }

type EffectiveAlertRule struct {
	AlertRule
	BaseRuleID     int64
	OverrideRuleID *int64
	Inherited      bool
}

type AlertRulePatch struct {
	Enabled        *bool
	ThresholdValue *string
	ForTimes       *int
	UpdatedAt      int64
}

type AlertEvent struct {
	ID               int64   `gorm:"column:id;primaryKey"`
	RuleID           int64   `gorm:"column:rule_id"`
	RuleKey          string  `gorm:"column:rule_key"`
	SiteID           *int64  `gorm:"column:site_id"`
	TargetType       string  `gorm:"column:target_type"`
	TargetKey        string  `gorm:"column:target_key"`
	ActiveKey        *string `gorm:"column:active_key"`
	Level            string  `gorm:"column:level"`
	Status           string  `gorm:"column:status"`
	ConsecutiveCount int     `gorm:"column:consecutive_count"`
	CurrentValue     *string `gorm:"column:current_value"`
	ThresholdValue   *string `gorm:"column:threshold_value"`
	MessageCode      string  `gorm:"column:message_code"`
	MessageParams    *string `gorm:"column:message_params"`
	Message          string  `gorm:"column:message"`
	FirstObservedAt  int64   `gorm:"column:first_observed_at"`
	FirstFiredAt     *int64  `gorm:"column:first_fired_at"`
	LastFiredAt      *int64  `gorm:"column:last_fired_at"`
	ResolvedAt       *int64  `gorm:"column:resolved_at"`
	ResolutionReason *string `gorm:"column:resolution_reason"`
	CreatedAt        int64   `gorm:"column:created_at"`
	UpdatedAt        int64   `gorm:"column:updated_at"`
}

func (AlertEvent) TableName() string { return "alert_event" }

type AlertEventView struct {
	AlertEvent
	SiteName   string
	TargetName string
}

type AlertDelivery struct {
	ID              int64   `gorm:"column:id;primaryKey"`
	AlertEventID    *int64  `gorm:"column:alert_event_id"`
	EventType       string  `gorm:"column:event_type"`
	Channel         string  `gorm:"column:channel"`
	Status          string  `gorm:"column:status"`
	AttemptCount    int     `gorm:"column:attempt_count"`
	ClaimToken      *string `gorm:"column:claim_token"`
	LeaseExpiresAt  *int64  `gorm:"column:lease_expires_at"`
	PayloadSnapshot []byte  `gorm:"column:payload_snapshot;type:json"`
	ErrorCode       string  `gorm:"column:error_code"`
	ResponseCode    *int    `gorm:"column:response_code"`
	ResponseMessage *string `gorm:"column:response_message"`
	NextRetryAt     *int64  `gorm:"column:next_retry_at"`
	SentAt          *int64  `gorm:"column:sent_at"`
	CreatedAt       int64   `gorm:"column:created_at"`
	UpdatedAt       int64   `gorm:"column:updated_at"`
}

func (AlertDelivery) TableName() string { return "alert_delivery" }

type AlertEvaluationCursor struct {
	ActiveKey     string `gorm:"column:active_key;primaryKey"`
	LastSampleAt  int64  `gorm:"column:last_sample_at"`
	LastSampleKey string `gorm:"column:last_sample_key"`
	CreatedAt     int64  `gorm:"column:created_at"`
	UpdatedAt     int64  `gorm:"column:updated_at"`
}

func (AlertEvaluationCursor) TableName() string { return "alert_evaluation_cursor" }

type AlertSummary struct {
	FiringCount        int64
	CriticalCount      int64
	WarningCount       int64
	ResolvedTodayCount int64
}

type AlertEventFilter struct {
	Offset         int
	Limit          int
	Statuses       []string
	Levels         []string
	TargetTypes    []string
	SiteID         *int64
	StartTimestamp *int64
	EndTimestamp   *int64
	SortBy         string
	SortOrder      string
}

type AlertRepository struct{ db *gorm.DB }

func NewAlertRepository(db *gorm.DB) *AlertRepository { return &AlertRepository{db: db} }

func (repository *AlertRepository) Transaction(ctx context.Context, callback func(*AlertRepository) error) error {
	if repository == nil || repository.db == nil || callback == nil {
		return errors.New("alert repository transaction dependencies are required")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return callback(NewAlertRepository(tx))
	})
}

func (repository *AlertRepository) ListRules(ctx context.Context, scopeType string, siteID int64) ([]EffectiveAlertRule, error) {
	if scopeType == "global" {
		var rules []AlertRule
		err := repository.db.WithContext(ctx).Raw(`SELECT id, rule_key, name, enabled, level, metric,
compare_operator, CAST(threshold_value AS CHAR) AS threshold_value, for_times,
scope_type, scope_id, created_at, updated_at
FROM alert_rule WHERE scope_type = 'global' AND scope_id = 0
ORDER BY rule_key, FIELD(level, 'info', 'warning', 'critical'), id`).Scan(&rules).Error
		if err != nil {
			return nil, fmt.Errorf("list global alert rules: %w", err)
		}
		result := make([]EffectiveAlertRule, len(rules))
		for index := range rules {
			result[index] = EffectiveAlertRule{AlertRule: rules[index], BaseRuleID: rules[index].ID}
		}
		return result, nil
	}
	if scopeType != "site" || siteID <= 0 {
		return nil, errors.New("invalid alert rule scope")
	}
	var rules []EffectiveAlertRule
	err := repository.db.WithContext(ctx).Raw(`SELECT
COALESCE(o.id, g.id) AS id, g.id AS base_rule_id, o.id AS override_rule_id,
COALESCE(o.rule_key, g.rule_key) AS rule_key, COALESCE(o.name, g.name) AS name,
COALESCE(o.enabled, g.enabled) AS enabled, COALESCE(o.level, g.level) AS level,
COALESCE(o.metric, g.metric) AS metric,
COALESCE(o.compare_operator, g.compare_operator) AS compare_operator,
CAST(COALESCE(o.threshold_value, g.threshold_value) AS CHAR) AS threshold_value,
COALESCE(o.for_times, g.for_times) AS for_times,
CASE WHEN o.id IS NULL THEN 'global' ELSE 'site' END AS scope_type,
CASE WHEN o.id IS NULL THEN 0 ELSE ? END AS scope_id,
COALESCE(o.created_at, g.created_at) AS created_at,
COALESCE(o.updated_at, g.updated_at) AS updated_at,
(o.id IS NULL) AS inherited
FROM alert_rule g
LEFT JOIN alert_rule o ON o.rule_key = g.rule_key AND o.level = g.level
 AND o.scope_type = 'site' AND o.scope_id = ?
WHERE g.scope_type = 'global' AND g.scope_id = 0
ORDER BY g.rule_key, FIELD(g.level, 'info', 'warning', 'critical'), g.id`, siteID, siteID).Scan(&rules).Error
	if err != nil {
		return nil, fmt.Errorf("list effective site alert rules: %w", err)
	}
	return rules, nil
}

func (repository *AlertRepository) FindRule(ctx context.Context, id int64) (AlertRule, error) {
	var rule AlertRule
	err := repository.db.WithContext(ctx).Raw(`SELECT id, rule_key, name, enabled, level, metric,
compare_operator, CAST(threshold_value AS CHAR) AS threshold_value, for_times,
scope_type, scope_id, created_at, updated_at
FROM alert_rule WHERE id = ? AND scope_type IN ('global', 'site')
AND LEFT(rule_key, 10) <> '__deleted_'`, id).Scan(&rule).Error
	if err != nil {
		return AlertRule{}, fmt.Errorf("find alert rule: %w", err)
	}
	if rule.ID == 0 {
		return AlertRule{}, ErrAlertRecordNotFound
	}
	return rule, nil
}

func (repository *AlertRepository) LockRule(ctx context.Context, id int64) (AlertRule, error) {
	var rule AlertRule
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND scope_type IN ? AND LEFT(rule_key, 10) <> ?", id, []string{"global", "site"}, "__deleted_").First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AlertRule{}, ErrAlertRecordNotFound
	}
	if err != nil {
		return AlertRule{}, fmt.Errorf("lock alert rule: %w", err)
	}
	return rule, nil
}

func (repository *AlertRepository) LockRulesByKey(ctx context.Context, ruleKey string, siteID int64) ([]AlertRule, error) {
	var rules []AlertRule
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("rule_key = ? AND ((scope_type = 'global' AND scope_id = 0) OR (scope_type = 'site' AND scope_id = ?))", ruleKey, siteID).
		Order("id").Find(&rules).Error
	if err != nil {
		return nil, fmt.Errorf("lock effective alert rules: %w", err)
	}
	return rules, nil
}

func (repository *AlertRepository) LockAllRulesByKey(ctx context.Context, ruleKey string) ([]AlertRule, error) {
	var rules []AlertRule
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("rule_key = ?", ruleKey).Order("id").Find(&rules).Error
	if err != nil {
		return nil, fmt.Errorf("lock all alert rules: %w", err)
	}
	return rules, nil
}

func (repository *AlertRepository) UpdateRule(ctx context.Context, id int64, patch AlertRulePatch) error {
	updates := map[string]any{"updated_at": patch.UpdatedAt}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.ThresholdValue != nil {
		updates["threshold_value"] = *patch.ThresholdValue
	}
	if patch.ForTimes != nil {
		updates["for_times"] = *patch.ForTimes
	}
	result := repository.db.WithContext(ctx).Model(&AlertRule{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update alert rule: %w", result.Error)
	}
	return nil
}

func (repository *AlertRepository) OverrideExists(ctx context.Context, base AlertRule, siteID int64) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&AlertRule{}).
		Where("rule_key = ? AND level = ? AND scope_type = 'site' AND scope_id = ?", base.RuleKey, base.Level, siteID).
		Count(&count).Error
	return count > 0, err
}

func (repository *AlertRepository) SiteExists(ctx context.Context, siteID int64) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Table("site").Where("id = ?", siteID).Count(&count).Error
	return count == 1, err
}

func (repository *AlertRepository) CreateOverride(ctx context.Context, base AlertRule, siteID, now int64, patch AlertRulePatch) (int64, error) {
	rule := AlertRule{
		RuleKey: base.RuleKey, Name: base.Name, Enabled: base.Enabled, Level: base.Level,
		Metric: base.Metric, CompareOperator: base.CompareOperator, ThresholdValue: base.ThresholdValue,
		ForTimes: base.ForTimes, ScopeType: "site", ScopeID: siteID, CreatedAt: now, UpdatedAt: now,
	}
	if patch.Enabled != nil {
		rule.Enabled = *patch.Enabled
	}
	if patch.ThresholdValue != nil {
		rule.ThresholdValue = patch.ThresholdValue
	}
	if patch.ForTimes != nil {
		rule.ForTimes = *patch.ForTimes
	}
	if err := repository.db.WithContext(ctx).Create(&rule).Error; err != nil {
		return 0, fmt.Errorf("create alert rule override: %w", err)
	}
	return rule.ID, nil
}

func (repository *AlertRepository) DeleteOverride(ctx context.Context, id int64) error {
	var eventCount int64
	if err := repository.db.WithContext(ctx).Model(&AlertEvent{}).Where("rule_id = ?", id).Count(&eventCount).Error; err != nil {
		return fmt.Errorf("check alert override history: %w", err)
	}
	var result *gorm.DB
	if eventCount == 0 {
		result = repository.db.WithContext(ctx).Delete(&AlertRule{}, id)
	} else {
		result = repository.db.WithContext(ctx).Model(&AlertRule{}).Where("id = ? AND scope_type = 'site'", id).
			Update("rule_key", "__deleted_"+strconv.FormatInt(id, 10))
	}
	if result.Error != nil {
		return fmt.Errorf("delete alert rule override: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAlertRecordNotFound
	}
	return nil
}

func (repository *AlertRepository) LockActiveEvent(ctx context.Context, activeKey string) (AlertEvent, error) {
	var event AlertEvent
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("active_key = ?", activeKey).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AlertEvent{}, ErrAlertRecordNotFound
	}
	if err != nil {
		return AlertEvent{}, fmt.Errorf("lock active alert event: %w", err)
	}
	return event, nil
}

func (repository *AlertRepository) LockEvaluationCursor(
	ctx context.Context,
	activeKey string,
) (AlertEvaluationCursor, bool, error) {
	var cursor AlertEvaluationCursor
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("active_key = ?", activeKey).First(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AlertEvaluationCursor{}, false, nil
	}
	if err != nil {
		return AlertEvaluationCursor{}, false, fmt.Errorf("lock alert evaluation cursor: %w", err)
	}
	return cursor, true, nil
}

func (repository *AlertRepository) CreateEvaluationCursor(
	ctx context.Context,
	cursor *AlertEvaluationCursor,
) error {
	if cursor == nil || cursor.ActiveKey == "" || cursor.LastSampleAt <= 0 || cursor.LastSampleKey == "" ||
		cursor.CreatedAt <= 0 || cursor.UpdatedAt <= 0 {
		return errors.New("invalid alert evaluation cursor")
	}
	if err := repository.db.WithContext(ctx).Create(cursor).Error; err != nil {
		return fmt.Errorf("create alert evaluation cursor: %w", err)
	}
	return nil
}

func (repository *AlertRepository) AdvanceEvaluationCursor(
	ctx context.Context,
	cursor *AlertEvaluationCursor,
) error {
	if cursor == nil || cursor.ActiveKey == "" || cursor.LastSampleAt <= 0 || cursor.LastSampleKey == "" ||
		cursor.UpdatedAt <= 0 {
		return errors.New("invalid alert evaluation cursor advance")
	}
	result := repository.db.WithContext(ctx).Model(&AlertEvaluationCursor{}).
		Where("active_key = ?", cursor.ActiveKey).
		Updates(map[string]any{
			"last_sample_at": cursor.LastSampleAt, "last_sample_key": cursor.LastSampleKey, "updated_at": cursor.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("advance alert evaluation cursor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAlertRecordNotFound
	}
	return nil
}

func (repository *AlertRepository) CreateEvent(ctx context.Context, event *AlertEvent) error {
	if err := repository.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("create alert event: %w", err)
	}
	return nil
}

func (repository *AlertRepository) SaveEvent(ctx context.Context, event *AlertEvent) error {
	result := repository.db.WithContext(ctx).Save(event)
	if result.Error != nil {
		return fmt.Errorf("save alert event: %w", result.Error)
	}
	return nil
}

func (repository *AlertRepository) Summary(ctx context.Context, todayStart int64) (AlertSummary, error) {
	var summary AlertSummary
	err := repository.db.WithContext(ctx).Raw(`SELECT
COALESCE(SUM(status = 'firing'), 0) AS firing_count,
COALESCE(SUM(status = 'firing' AND level = 'critical'), 0) AS critical_count,
COALESCE(SUM(status = 'firing' AND level = 'warning'), 0) AS warning_count,
COALESCE(SUM(status = 'resolved' AND resolved_at >= ?), 0) AS resolved_today_count
FROM alert_event`, todayStart).Scan(&summary).Error
	if err != nil {
		return AlertSummary{}, fmt.Errorf("read alert summary: %w", err)
	}
	return summary, nil
}

func (repository *AlertRepository) ListEvents(ctx context.Context, filter AlertEventFilter) ([]AlertEventView, int64, error) {
	where, args := alertEventWhere(filter)
	var total int64
	if err := repository.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM alert_event e "+where, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count alert events: %w", err)
	}
	order := alertEventOrder(filter.SortBy, filter.SortOrder)
	queryArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	var events []AlertEventView
	err := repository.db.WithContext(ctx).Raw(alertEventSelect+" "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", queryArgs...).Scan(&events).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list alert events: %w", err)
	}
	return events, total, nil
}

func (repository *AlertRepository) FindEvent(ctx context.Context, id int64) (AlertEventView, error) {
	var event AlertEventView
	err := repository.db.WithContext(ctx).Raw(alertEventSelect+" WHERE e.id = ?", id).Scan(&event).Error
	if err != nil {
		return AlertEventView{}, fmt.Errorf("find alert event: %w", err)
	}
	if event.ID == 0 {
		return AlertEventView{}, ErrAlertRecordNotFound
	}
	return event, nil
}

func (repository *AlertRepository) ListDeliveries(ctx context.Context, eventID int64) ([]AlertDelivery, error) {
	var deliveries []AlertDelivery
	err := repository.db.WithContext(ctx).Raw(`SELECT id, event_type, status, attempt_count, error_code,
response_code, response_message, next_retry_at, sent_at
FROM alert_delivery WHERE alert_event_id = ? ORDER BY id`, eventID).Scan(&deliveries).Error
	if err != nil {
		return nil, fmt.Errorf("list alert deliveries: %w", err)
	}
	return deliveries, nil
}

const alertEventSelect = `SELECT e.id, e.rule_id, e.rule_key, e.site_id,
e.target_type, e.target_key, e.active_key, e.level, e.status, e.consecutive_count,
CAST(e.current_value AS CHAR) AS current_value,
CAST(e.threshold_value AS CHAR) AS threshold_value,
e.message_code, CAST(e.message_params AS CHAR) AS message_params, e.message,
e.first_observed_at, e.first_fired_at, e.last_fired_at, e.resolved_at, e.resolution_reason,
e.created_at, e.updated_at, COALESCE(s.name, '') AS site_name,
CASE
 WHEN e.target_type = 'site' THEN COALESCE(s.name, e.target_key)
 WHEN e.target_type = 'account' THEN COALESCE(NULLIF(a.display_name, ''), a.username, e.target_key)
	WHEN e.target_type = 'instance' THEN SUBSTRING(e.target_key, LOCATE('/', e.target_key) + 1)
 ELSE e.target_key
END AS target_name
FROM alert_event e
LEFT JOIN site s ON s.id = e.site_id
LEFT JOIN account a ON e.target_type = 'account' AND CAST(a.id AS CHAR) = e.target_key`

func alertEventWhere(filter AlertEventFilter) (string, []any) {
	conditions := []string{"1 = 1"}
	args := []any{}
	if len(filter.Statuses) > 0 {
		conditions = append(conditions, "e.status IN ?")
		args = append(args, filter.Statuses)
	}
	if len(filter.Levels) > 0 {
		conditions = append(conditions, "e.level IN ?")
		args = append(args, filter.Levels)
	}
	if len(filter.TargetTypes) > 0 {
		conditions = append(conditions, "e.target_type IN ?")
		args = append(args, filter.TargetTypes)
	}
	if filter.SiteID != nil {
		conditions = append(conditions, "e.site_id = ?")
		args = append(args, *filter.SiteID)
	}
	if filter.StartTimestamp != nil {
		conditions = append(conditions, "e.first_observed_at >= ?")
		args = append(args, *filter.StartTimestamp)
	}
	if filter.EndTimestamp != nil {
		conditions = append(conditions, "e.first_observed_at < ?")
		args = append(args, *filter.EndTimestamp)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func alertEventOrder(sortBy, sortOrder string) string {
	direction := "DESC"
	if sortOrder == "asc" {
		direction = "ASC"
	}
	columns := map[string]string{
		"status":         "FIELD(e.status, 'firing', 'pending', 'resolved')",
		"level":          "FIELD(e.level, 'critical', 'warning', 'info')",
		"first_fired_at": "e.first_fired_at",
		"last_fired_at":  "e.last_fired_at",
	}
	if column, exists := columns[sortBy]; exists {
		return column + " " + direction + ", e.id DESC"
	}
	return "FIELD(e.status, 'firing', 'pending', 'resolved'), " +
		"FIELD(e.level, 'critical', 'warning', 'info'), " +
		"COALESCE(e.last_fired_at, e.first_observed_at) DESC, e.id DESC"
}
