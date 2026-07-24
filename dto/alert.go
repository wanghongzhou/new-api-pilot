package dto

import (
	"strconv"
	"strings"
)

const (
	AlertScopeGlobal = "global"
	AlertScopeSite   = "site"

	AlertLevelInfo     = "info"
	AlertLevelWarning  = "warning"
	AlertLevelCritical = "critical"

	AlertStatusPending  = "pending"
	AlertStatusFiring   = "firing"
	AlertStatusResolved = "resolved"

	AlertRuleCategorySite       = "site"
	AlertRuleCategoryCollection = "collection"
	AlertRuleCategoryInstance   = "instance"
	AlertRuleCategoryAccount    = "account"
	AlertRuleCategoryChannel    = "channel"
)

type AlertRuleConstraints struct {
	ValueKind         string  `json:"value_kind"`
	ThresholdEditable bool    `json:"threshold_editable"`
	ThresholdMin      *string `json:"threshold_min"`
	ThresholdMax      *string `json:"threshold_max"`
	ThresholdStep     *string `json:"threshold_step"`
	ForTimesEditable  bool    `json:"for_times_editable"`
	ForTimesMin       int     `json:"for_times_min"`
	ForTimesMax       int     `json:"for_times_max"`
	PairedRuleID      *string `json:"paired_rule_id"`
	Relation          *string `json:"relation"`
}

type AlertRuleItem struct {
	ID              string               `json:"id"`
	EffectiveRuleID string               `json:"effective_rule_id"`
	BaseRuleID      string               `json:"base_rule_id"`
	OverrideRuleID  *string              `json:"override_rule_id"`
	RuleKey         string               `json:"rule_key"`
	Category        string               `json:"category"`
	Name            string               `json:"name"`
	Enabled         bool                 `json:"enabled"`
	Level           string               `json:"level"`
	Metric          string               `json:"metric"`
	CompareOperator string               `json:"compare_operator"`
	ThresholdValue  *string              `json:"threshold_value"`
	ForTimes        int                  `json:"for_times"`
	ScopeType       string               `json:"scope_type"`
	ScopeID         string               `json:"scope_id"`
	Inherited       bool                 `json:"inherited"`
	EditableFields  []string             `json:"editable_fields"`
	Constraints     AlertRuleConstraints `json:"constraints"`
	UpdatedAt       int64                `json:"updated_at"`
}

type AlertRuleListQuery struct {
	ScopeType  string
	ScopeID    int64
	Page       int
	PageSize   int
	Categories []string
	Levels     []string
	Enabled    *bool
	Inherited  *bool
	SortBy     string
	SortOrder  string
}

func (query *AlertRuleListQuery) Normalize() {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	query.ScopeType = strings.ToLower(strings.TrimSpace(query.ScopeType))
	if query.ScopeType == "" {
		query.ScopeType = AlertScopeGlobal
	}
	query.Categories = normalizeEnumList(query.Categories)
	query.Levels = normalizeEnumList(query.Levels)
	query.SortBy = strings.ToLower(strings.TrimSpace(query.SortBy))
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	if query.SortOrder == "" {
		query.SortOrder = "asc"
	}
}

func (query AlertRuleListQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.ScopeType == AlertScopeGlobal {
		if query.ScopeID != 0 {
			errors["scope_id"] = "must be 0 for global scope"
		}
	} else if query.ScopeType == AlertScopeSite {
		if query.ScopeID <= 0 {
			errors["scope_id"] = "must be positive for site scope"
		}
	} else {
		errors["scope_type"] = "must be global or site"
	}
	if query.Page < 1 {
		errors["page"] = "must be at least 1"
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		errors["page_size"] = "must be between 1 and 100"
	}
	if !validEnumList(query.Categories, AlertRuleCategorySite, AlertRuleCategoryCollection, AlertRuleCategoryInstance, AlertRuleCategoryAccount, AlertRuleCategoryChannel) {
		errors["category"] = "contains an invalid value"
	}
	if !validEnumList(query.Levels, AlertLevelInfo, AlertLevelWarning, AlertLevelCritical) {
		errors["level"] = "contains an invalid value"
	}
	if query.SortBy != "" && !alertOneOf(query.SortBy, "category", "rule_key", "level", "metric", "enabled", "updated_at") {
		errors["sort_by"] = "is invalid"
	}
	if !alertOneOf(query.SortOrder, "asc", "desc") {
		errors["sort_order"] = "is invalid"
	}
	return nilIfNoAlertErrors(errors)
}

func (query AlertRuleListQuery) Offset() int { return (query.Page - 1) * query.PageSize }

type AlertRuleUpdateRequest struct {
	Enabled        *bool   `json:"enabled"`
	ThresholdValue *string `json:"threshold_value"`
	ForTimes       *int    `json:"for_times"`
}

func (request AlertRuleUpdateRequest) Validate() map[string]string {
	errors := map[string]string{}
	if request.Enabled == nil && request.ThresholdValue == nil && request.ForTimes == nil {
		errors["body"] = "at least one editable field is required"
	}
	if request.ThresholdValue != nil && strings.TrimSpace(*request.ThresholdValue) == "" {
		errors["threshold_value"] = "must not be empty"
	}
	if request.ForTimes != nil && (*request.ForTimes < 1 || *request.ForTimes > 60) {
		errors["for_times"] = "must be between 1 and 60"
	}
	return nilIfNoAlertErrors(errors)
}

type AlertRuleOverrideRequest struct {
	BaseRuleID     string  `json:"base_rule_id"`
	SiteID         string  `json:"site_id"`
	Enabled        *bool   `json:"enabled"`
	ThresholdValue *string `json:"threshold_value"`
	ForTimes       *int    `json:"for_times"`
}

func (request AlertRuleOverrideRequest) Validate() map[string]string {
	errors := map[string]string{}
	if !validAlertID(request.BaseRuleID) {
		errors["base_rule_id"] = "must be a positive decimal int64 string"
	}
	if !validAlertID(request.SiteID) {
		errors["site_id"] = "must be a positive decimal int64 string"
	}
	if request.ThresholdValue != nil && strings.TrimSpace(*request.ThresholdValue) == "" {
		errors["threshold_value"] = "must not be empty"
	}
	if request.ForTimes != nil && (*request.ForTimes < 1 || *request.ForTimes > 60) {
		errors["for_times"] = "must be between 1 and 60"
	}
	return nilIfNoAlertErrors(errors)
}

type AlertEventItem struct {
	ID               string     `json:"id"`
	RuleID           string     `json:"rule_id"`
	RuleKey          string     `json:"rule_key"`
	SiteID           *string    `json:"site_id"`
	SiteName         string     `json:"site_name"`
	TargetType       string     `json:"target_type"`
	TargetKey        string     `json:"target_key"`
	TargetName       string     `json:"target_name"`
	Level            string     `json:"level"`
	Status           string     `json:"status"`
	CurrentValue     *string    `json:"current_value"`
	ThresholdValue   *string    `json:"threshold_value"`
	Message          MessageRef `json:"message"`
	FirstObservedAt  int64      `json:"first_observed_at"`
	FirstFiredAt     *int64     `json:"first_fired_at"`
	LastFiredAt      *int64     `json:"last_fired_at"`
	ResolvedAt       *int64     `json:"resolved_at"`
	ResolutionReason *string    `json:"resolution_reason"`
}

type AlertDeliveryItem struct {
	ID              string `json:"id"`
	EventType       string `json:"event_type"`
	Status          string `json:"status"`
	AttemptCount    int    `json:"attempt_count"`
	ErrorCode       string `json:"error_code"`
	ResponseCode    *int   `json:"response_code"`
	ResponseMessage string `json:"response_message"`
	NextRetryAt     *int64 `json:"next_retry_at"`
	SentAt          *int64 `json:"sent_at"`
}

type AlertEventDetail struct {
	AlertEventItem
	ConsecutiveCount int                 `json:"consecutive_count"`
	Deliveries       []AlertDeliveryItem `json:"deliveries"`
}

type AlertSummary struct {
	FiringCount        int64 `json:"firing_count"`
	CriticalCount      int64 `json:"critical_count"`
	WarningCount       int64 `json:"warning_count"`
	ResolvedTodayCount int64 `json:"resolved_today_count"`
	UpdatedAt          int64 `json:"updated_at"`
}

type NotificationTestResult struct {
	DeliveryID   *string    `json:"delivery_id"`
	Status       string     `json:"status"`
	ResponseCode *int       `json:"response_code"`
	Message      MessageRef `json:"message"`
}

type AlertListQuery struct {
	Page           int
	PageSize       int
	Statuses       []string
	Levels         []string
	TargetTypes    []string
	SiteID         *int64
	StartTimestamp *int64
	EndTimestamp   *int64
	SortBy         string
	SortOrder      string
}

func (query *AlertListQuery) Normalize() {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	query.Statuses = normalizeEnumList(query.Statuses)
	query.Levels = normalizeEnumList(query.Levels)
	query.TargetTypes = normalizeEnumList(query.TargetTypes)
	query.SortBy = strings.ToLower(strings.TrimSpace(query.SortBy))
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
}

func (query AlertListQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Page < 1 {
		errors["page"] = "must be at least 1"
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		errors["page_size"] = "must be between 1 and 100"
	}
	if !validEnumList(query.Statuses, AlertStatusPending, AlertStatusFiring, AlertStatusResolved) {
		errors["status"] = "contains an invalid value"
	}
	if !validEnumList(query.Levels, AlertLevelInfo, AlertLevelWarning, AlertLevelCritical) {
		errors["level"] = "contains an invalid value"
	}
	if !validEnumList(query.TargetTypes, "site", "instance", "account", "collection") {
		errors["target_type"] = "contains an invalid value"
	}
	if query.SiteID != nil && *query.SiteID <= 0 {
		errors["site_id"] = "must be positive"
	}
	if query.StartTimestamp != nil && *query.StartTimestamp < 0 {
		errors["start_timestamp"] = "must not be negative"
	}
	if query.EndTimestamp != nil && *query.EndTimestamp < 0 {
		errors["end_timestamp"] = "must not be negative"
	}
	if query.StartTimestamp != nil && query.EndTimestamp != nil && *query.StartTimestamp >= *query.EndTimestamp {
		errors["end_timestamp"] = "must be greater than start_timestamp"
	}
	if query.SortBy != "" && !alertOneOf(query.SortBy, "rule_key", "status", "level", "site_name", "first_fired_at", "last_fired_at", "resolved_at") {
		errors["sort_by"] = "is invalid"
	}
	if !alertOneOf(query.SortOrder, "asc", "desc") {
		errors["sort_order"] = "is invalid"
	}
	return nilIfNoAlertErrors(errors)
}

func (query AlertListQuery) Offset() int { return (query.Page - 1) * query.PageSize }

func validAlertID(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func alertOneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

const QueryEnumListMaximumValues = 20

func normalizeEnumList(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validEnumList(values []string, choices ...string) bool {
	if len(values) > QueryEnumListMaximumValues {
		return false
	}
	for _, value := range values {
		if value == "" || !alertOneOf(value, choices...) {
			return false
		}
	}
	return true
}

func nilIfNoAlertErrors(errors map[string]string) map[string]string {
	if len(errors) == 0 {
		return nil
	}
	return errors
}
