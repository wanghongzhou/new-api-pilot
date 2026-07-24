package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

var (
	ErrAlertNotFound       = errors.New("alert event not found")
	ErrAlertRuleNotFound   = errors.New("alert rule not found")
	ErrAlertRuleConflict   = errors.New("alert rule override already exists")
	ErrAlertRuleInvalid    = errors.New("alert rule change is invalid")
	ErrAlertRead           = errors.New("alert read failed")
	ErrAlertEvaluation     = errors.New("alert evaluation failed")
	ErrAlertSampleConflict = errors.New("alert sample identity conflict")
)

type AlertValidationError struct{ Fields map[string]string }

func (err *AlertValidationError) Error() string { return ErrAlertRuleInvalid.Error() }
func (err *AlertValidationError) Unwrap() error { return ErrAlertRuleInvalid }

type AlertService struct {
	repository *model.AlertRepository
	clock      common.Clock
	metrics    AlertTransitionMetricsRecorder
}

type AlertServiceOptions struct {
	Database *gorm.DB
	Clock    common.Clock
	Metrics  AlertTransitionMetricsRecorder
}

func NewAlertService(options AlertServiceOptions) (*AlertService, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("alert service dependencies are required")
	}
	return &AlertService{
		repository: model.NewAlertRepository(options.Database), clock: options.Clock, metrics: options.Metrics,
	}, nil
}

func (service *AlertService) Summary(ctx context.Context) (dto.AlertSummary, error) {
	now := service.clock.Now()
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(location)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).Unix()
	summary, err := service.repository.Summary(ctx, todayStart)
	if err != nil {
		return dto.AlertSummary{}, errors.Join(ErrAlertRead, err)
	}
	return dto.AlertSummary{
		FiringCount: summary.FiringCount, CriticalCount: summary.CriticalCount,
		WarningCount: summary.WarningCount, ResolvedTodayCount: summary.ResolvedTodayCount,
		UpdatedAt: now.Unix(),
	}, nil
}

func (service *AlertService) List(ctx context.Context, query dto.AlertListQuery) (common.PageData[dto.AlertEventItem], error) {
	query.Normalize()
	if query.Validate() != nil {
		return common.PageData[dto.AlertEventItem]{}, ErrAlertRuleInvalid
	}
	events, total, err := service.repository.ListEvents(ctx, model.AlertEventFilter{
		Offset: query.Offset(), Limit: query.PageSize, Statuses: query.Statuses, Levels: query.Levels,
		TargetTypes: query.TargetTypes, SiteID: query.SiteID, StartTimestamp: query.StartTimestamp,
		EndTimestamp: query.EndTimestamp, SortBy: query.SortBy, SortOrder: query.SortOrder,
	})
	if err != nil {
		return common.PageData[dto.AlertEventItem]{}, errors.Join(ErrAlertRead, err)
	}
	items := make([]dto.AlertEventItem, 0, len(events))
	for _, event := range events {
		items = append(items, alertEventItem(event))
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *AlertService) Get(ctx context.Context, id int64) (dto.AlertEventDetail, error) {
	event, err := service.repository.FindEvent(ctx, id)
	if errors.Is(err, model.ErrAlertRecordNotFound) {
		return dto.AlertEventDetail{}, ErrAlertNotFound
	}
	if err != nil {
		return dto.AlertEventDetail{}, errors.Join(ErrAlertRead, err)
	}
	deliveries, err := service.repository.ListDeliveries(ctx, id)
	if err != nil {
		return dto.AlertEventDetail{}, errors.Join(ErrAlertRead, err)
	}
	items := make([]dto.AlertDeliveryItem, 0, len(deliveries))
	for _, delivery := range deliveries {
		responseMessage := ""
		if delivery.ResponseMessage != nil {
			responseMessage = *delivery.ResponseMessage
		}
		items = append(items, dto.AlertDeliveryItem{
			ID: strconv.FormatInt(delivery.ID, 10), EventType: delivery.EventType, Status: delivery.Status,
			AttemptCount: delivery.AttemptCount, ErrorCode: delivery.ErrorCode, ResponseCode: delivery.ResponseCode,
			ResponseMessage: responseMessage, NextRetryAt: delivery.NextRetryAt, SentAt: delivery.SentAt,
		})
	}
	return dto.AlertEventDetail{AlertEventItem: alertEventItem(event), ConsecutiveCount: event.ConsecutiveCount, Deliveries: items}, nil
}

func (service *AlertService) ListRules(ctx context.Context, query dto.AlertRuleListQuery) (common.PageData[dto.AlertRuleItem], error) {
	query.Normalize()
	if fields := query.Validate(); fields != nil {
		return common.PageData[dto.AlertRuleItem]{}, &AlertValidationError{Fields: fields}
	}
	items, err := service.listRuleItems(ctx, query.ScopeType, query.ScopeID)
	if err != nil {
		return common.PageData[dto.AlertRuleItem]{}, err
	}
	items = filterAlertRuleItems(items, query)
	sortAlertRuleItems(items, query.SortBy, query.SortOrder)
	total := int64(len(items))
	start := min(query.Offset(), len(items))
	end := min(start+query.PageSize, len(items))
	return common.NewPageData(query.Page, query.PageSize, total, items[start:end]), nil
}

func (service *AlertService) listRuleItems(ctx context.Context, scopeType string, siteID int64) ([]dto.AlertRuleItem, error) {
	if scopeType == dto.AlertScopeSite {
		exists, err := service.repository.SiteExists(ctx, siteID)
		if err != nil {
			return nil, errors.Join(ErrAlertRead, err)
		}
		if !exists {
			return nil, ErrAlertRuleNotFound
		}
	}
	rules, err := service.repository.ListRules(ctx, scopeType, siteID)
	if err != nil {
		return nil, errors.Join(ErrAlertRead, err)
	}
	return alertRuleItems(rules), nil
}

func (service *AlertService) UpdateRule(ctx context.Context, id int64, request dto.AlertRuleUpdateRequest) (dto.AlertRuleItem, error) {
	if fields := request.Validate(); fields != nil {
		return dto.AlertRuleItem{}, &AlertValidationError{Fields: fields}
	}
	var updated model.AlertRule
	err := service.repository.Transaction(ctx, func(repository *model.AlertRepository) error {
		unlocked, err := repository.FindRule(ctx, id)
		if err != nil {
			return err
		}
		rules, err := repository.LockAllRulesByKey(ctx, unlocked.RuleKey)
		if err != nil {
			return err
		}
		target, found := alertRuleByID(rules, id)
		if !found {
			return model.ErrAlertRecordNotFound
		}
		patch, fields := alertRulePatch(target, request.Enabled, request.ThresholdValue, request.ForTimes, service.clock.Now().Unix())
		if fields != nil {
			return &AlertValidationError{Fields: fields}
		}
		if fields := validateAlertPairs(rules, target, patch); fields != nil {
			return &AlertValidationError{Fields: fields}
		}
		if err := repository.UpdateRule(ctx, id, patch); err != nil {
			return err
		}
		updated = applyAlertPatch(target, patch)
		return nil
	})
	if errors.Is(err, model.ErrAlertRecordNotFound) {
		return dto.AlertRuleItem{}, ErrAlertRuleNotFound
	}
	if err != nil {
		return dto.AlertRuleItem{}, err
	}
	items, err := service.listRuleItems(ctx, updated.ScopeType, updated.ScopeID)
	if err != nil {
		return dto.AlertRuleItem{}, err
	}
	for _, item := range items {
		if item.EffectiveRuleID == strconv.FormatInt(id, 10) {
			return item, nil
		}
	}
	return dto.AlertRuleItem{}, ErrAlertRuleNotFound
}

func (service *AlertService) CreateOverride(ctx context.Context, request dto.AlertRuleOverrideRequest) (dto.AlertRuleItem, error) {
	if fields := request.Validate(); fields != nil {
		return dto.AlertRuleItem{}, &AlertValidationError{Fields: fields}
	}
	baseID, _ := strconv.ParseInt(request.BaseRuleID, 10, 64)
	siteID, _ := strconv.ParseInt(request.SiteID, 10, 64)
	var overrideID int64
	err := service.repository.Transaction(ctx, func(repository *model.AlertRepository) error {
		base, err := repository.FindRule(ctx, baseID)
		if err != nil {
			return err
		}
		if base.ScopeType != dto.AlertScopeGlobal {
			return &AlertValidationError{Fields: map[string]string{"base_rule_id": "must identify a global rule"}}
		}
		rules, err := repository.LockAllRulesByKey(ctx, base.RuleKey)
		if err != nil {
			return err
		}
		lockedBase, found := alertRuleByID(rules, baseID)
		if !found {
			return model.ErrAlertRecordNotFound
		}
		exists, err := repository.OverrideExists(ctx, lockedBase, siteID)
		if err != nil {
			return err
		}
		if exists {
			return ErrAlertRuleConflict
		}
		siteExists, err := repository.SiteExists(ctx, siteID)
		if err != nil {
			return err
		}
		if !siteExists {
			return &AlertValidationError{Fields: map[string]string{"site_id": "site does not exist"}}
		}
		patch, fields := alertRulePatch(lockedBase, request.Enabled, request.ThresholdValue, request.ForTimes, service.clock.Now().Unix())
		if fields != nil {
			return &AlertValidationError{Fields: fields}
		}
		hypothetical := lockedBase
		hypothetical.ScopeType, hypothetical.ScopeID = dto.AlertScopeSite, siteID
		if fields := validateAlertPairs(rules, hypothetical, patch); fields != nil {
			return &AlertValidationError{Fields: fields}
		}
		overrideID, err = repository.CreateOverride(ctx, lockedBase, siteID, service.clock.Now().Unix(), patch)
		return err
	})
	if errors.Is(err, model.ErrAlertRecordNotFound) {
		return dto.AlertRuleItem{}, ErrAlertRuleNotFound
	}
	if err != nil {
		return dto.AlertRuleItem{}, err
	}
	items, err := service.listRuleItems(ctx, dto.AlertScopeSite, siteID)
	if err != nil {
		return dto.AlertRuleItem{}, err
	}
	for _, item := range items {
		if item.EffectiveRuleID == strconv.FormatInt(overrideID, 10) {
			return item, nil
		}
	}
	return dto.AlertRuleItem{}, ErrAlertRuleNotFound
}

func (service *AlertService) DeleteOverride(ctx context.Context, id int64) error {
	return service.repository.Transaction(ctx, func(repository *model.AlertRepository) error {
		rule, err := repository.LockRule(ctx, id)
		if errors.Is(err, model.ErrAlertRecordNotFound) {
			return ErrAlertRuleNotFound
		}
		if err != nil {
			return err
		}
		if rule.ScopeType != dto.AlertScopeSite {
			return &AlertValidationError{Fields: map[string]string{"id": "only site overrides can be deleted"}}
		}
		return repository.DeleteOverride(ctx, id)
	})
}

type AlertSampleState string

const (
	AlertSampleKnown         AlertSampleState = "known"
	AlertSampleUnknown       AlertSampleState = "unknown"
	AlertSampleScopeInactive AlertSampleState = "scope_inactive"
)

type AlertEvaluation struct {
	RuleKey          string
	SiteID           *int64
	TargetType       string
	TargetKey        string
	TargetName       string
	State            AlertSampleState
	CurrentValue     *string
	Message          dto.MessageRef
	ScopeType        string
	ScopeID          int64
	ScopeName        string
	Source           string
	RequestID        string
	ObservedAt       int64
	SampleKey        string
	PriorSampleKeys  []string
	ResolutionReason string
}

type AlertEvaluationResult struct {
	EventID    int64
	Status     string
	Level      string
	Transition string
}

type alertCollectionTargetKind string

const (
	alertCollectionTargetNone alertCollectionTargetKind = ""
	alertCollectionTargetHour alertCollectionTargetKind = "hour"
	alertCollectionTargetRun  alertCollectionTargetKind = "run"
)

type alertRuleContract struct {
	TargetType       string
	MessageCode      constant.MessageCode
	CollectionTarget alertCollectionTargetKind
}

var alertRuleContracts = map[string]alertRuleContract{
	"site_offline":               {TargetType: "site", MessageCode: constant.MessageAlertSiteOffline},
	"site_auth_expired":          {TargetType: "site", MessageCode: constant.MessageAlertAuthExpired},
	"site_export_disabled":       {TargetType: "site", MessageCode: constant.MessageAlertExportDisabled},
	"collection_missing":         {TargetType: "collection", MessageCode: constant.MessageAlertCollectionMissing, CollectionTarget: alertCollectionTargetHour},
	"backfill_failed":            {TargetType: "collection", MessageCode: constant.MessageAlertBackfillFailed, CollectionTarget: alertCollectionTargetRun},
	"validation_failed":          {TargetType: "collection", MessageCode: constant.MessageAlertValidationFailed, CollectionTarget: alertCollectionTargetHour},
	"instance_stale":             {TargetType: "instance", MessageCode: constant.MessageAlertInstanceStale},
	"instance_offline":           {TargetType: "instance", MessageCode: constant.MessageAlertInstanceOffline},
	"site_no_instance":           {TargetType: "site", MessageCode: constant.MessageAlertNoInstance},
	"cpu_high":                   {TargetType: "instance", MessageCode: constant.MessageAlertCPUHigh},
	"memory_high":                {TargetType: "instance", MessageCode: constant.MessageAlertMemoryHigh},
	"disk_high":                  {TargetType: "instance", MessageCode: constant.MessageAlertDiskHigh},
	"account_missing":            {TargetType: "account", MessageCode: constant.MessageAlertAccountMissing},
	"account_identity_mismatch":  {TargetType: "account", MessageCode: constant.MessageAlertAccountIdentityMismatch},
	"account_disabled":           {TargetType: "account", MessageCode: constant.MessageAlertAccountDisabled},
	"account_quota_empty":        {TargetType: "account", MessageCode: constant.MessageAlertAccountQuotaEmpty},
	"channel_balance_low":        {TargetType: "site", MessageCode: constant.MessageAlertChannelBalanceLow},
	"channel_response_time_high": {TargetType: "site", MessageCode: constant.MessageAlertChannelResponseTimeHigh},
	"channel_availability_low":   {TargetType: "site", MessageCode: constant.MessageAlertChannelAvailabilityLow},
}

type canonicalAlertTarget struct {
	Key      string
	EntityID int64
}

type AlertEvaluator interface {
	Evaluate(context.Context, AlertEvaluation) (AlertEvaluationResult, error)
}

func (service *AlertService) Evaluate(ctx context.Context, evaluation AlertEvaluation) (AlertEvaluationResult, error) {
	evaluation.RuleKey = strings.TrimSpace(evaluation.RuleKey)
	evaluation.TargetType = strings.ToLower(strings.TrimSpace(evaluation.TargetType))
	evaluation.TargetKey = strings.TrimSpace(evaluation.TargetKey)
	if fields := validateAlertEvaluation(evaluation); fields != nil {
		return AlertEvaluationResult{}, &AlertValidationError{Fields: fields}
	}
	contract := alertRuleContracts[evaluation.RuleKey]
	target, _ := canonicalAlertTargetKey(evaluation, contract)
	targetKey := target.Key
	activeKey := alertActiveKey(evaluation.RuleKey, evaluation.TargetType, targetKey)
	var result AlertEvaluationResult
	err := service.repository.Transaction(ctx, func(repository *model.AlertRepository) error {
		rules, err := repository.LockRulesByKey(ctx, evaluation.RuleKey, alertSiteID(evaluation.SiteID))
		if err != nil {
			return err
		}
		if len(rules) == 0 {
			return ErrAlertRuleNotFound
		}
		cursor, hasCursor, err := repository.LockEvaluationCursor(ctx, activeKey)
		if err != nil {
			return err
		}
		active, activeErr := repository.LockActiveEvent(ctx, activeKey)
		if activeErr != nil && !errors.Is(activeErr, model.ErrAlertRecordNotFound) {
			return activeErr
		}
		hasActive := activeErr == nil
		if alertEvaluationIdentityUpgrade(cursor, hasCursor, evaluation) {
			processedAt := service.clock.Now().Unix()
			if processedAt <= 0 {
				return errors.New("alert evaluation clock is invalid")
			}
			cursor.LastSampleKey, cursor.UpdatedAt = evaluation.SampleKey, processedAt
			if err := repository.AdvanceEvaluationCursor(ctx, &cursor); err != nil {
				return err
			}
			if hasActive {
				result = evaluationResult(active, "duplicate")
			} else {
				result.Transition = "duplicate"
			}
			return nil
		}
		duplicate, err := alertEvaluationAlreadyApplied(cursor, hasCursor, evaluation)
		if err != nil {
			return err
		}
		if duplicate {
			if hasActive {
				result = evaluationResult(active, "duplicate")
			} else {
				result.Transition = "duplicate"
			}
			return nil
		}
		mutationErr := func() error {
			if evaluation.State == AlertSampleUnknown {
				if hasActive {
					result = evaluationResult(active, "unknown")
				} else {
					result.Transition = "unknown"
				}
				return nil
			}
			now := evaluation.ObservedAt
			if evaluation.State == AlertSampleScopeInactive {
				if !hasActive {
					result.Transition = "unchanged"
					return nil
				}
				wasFiring := active.Status == dto.AlertStatusFiring
				message, params, err := scopeInactiveMessage(evaluation)
				if err != nil {
					return err
				}
				resolveAlertEvent(&active, now, alertResolutionRetired)
				active.MessageCode, active.MessageParams, active.Message = string(message.Code), params, message.TechnicalDetail
				if err := repository.SaveEvent(ctx, &active); err != nil {
					return err
				}
				if wasFiring && active.ResolutionReason != nil && *active.ResolutionReason == alertResolutionRecovered {
					if err := enqueueAlertTransition(ctx, repository, active, model.AlertDeliveryEventResolved, now); err != nil {
						return err
					}
				}
				result = evaluationResult(active, "resolved")
				return nil
			}
			value, _, valid := parseAlertDecimal(*evaluation.CurrentValue, true)
			if !valid {
				return &AlertValidationError{Fields: map[string]string{"current_value": "must be a valid decimal"}}
			}
			canonical := value.FloatString(10)
			effective := effectiveAlertRules(rules)
			candidate := matchingAlertRule(effective, value)
			if candidate == nil {
				if !hasActive {
					result.Transition = "unchanged"
					return nil
				}
				wasFiring := active.Status == dto.AlertStatusFiring
				if err := updateResolvedAlertEvidence(evaluation, &active, canonical); err != nil {
					return err
				}
				resolveAlertEvent(&active, now, alertResolutionReason(evaluation))
				if err := repository.SaveEvent(ctx, &active); err != nil {
					return err
				}
				if wasFiring && active.ResolutionReason != nil && *active.ResolutionReason == alertResolutionRecovered {
					if err := enqueueAlertTransition(ctx, repository, active, model.AlertDeliveryEventResolved, now); err != nil {
						return err
					}
				}
				result = evaluationResult(active, "resolved")
				return nil
			}
			threshold, _, valid := parseAlertThreshold(*candidate.ThresholdValue, true)
			if !valid {
				return errors.New("effective alert threshold is invalid")
			}
			canonicalThreshold := threshold.FloatString(2)
			candidate.ThresholdValue = &canonicalThreshold
			message, params, err := buildAlertMessage(evaluation, contract, target, *candidate, canonical)
			if err != nil {
				return err
			}
			if hasActive && active.Level == candidate.Level {
				active.CurrentValue, active.ThresholdValue, active.UpdatedAt = &canonical, candidate.ThresholdValue, now
				active.MessageCode, active.MessageParams, active.Message = string(message.Code), params, message.TechnicalDetail
				transition := "unchanged"
				if active.Status == dto.AlertStatusPending {
					active.ConsecutiveCount++
					if active.ConsecutiveCount >= candidate.ForTimes {
						active.Status, active.FirstFiredAt, active.LastFiredAt = dto.AlertStatusFiring, &now, &now
						transition = "firing"
					} else {
						transition = "pending"
					}
				} else if active.Status == dto.AlertStatusFiring {
					active.LastFiredAt = &now
				}
				if err := repository.SaveEvent(ctx, &active); err != nil {
					return err
				}
				if transition == "firing" {
					if err := enqueueAlertTransition(ctx, repository, active, model.AlertDeliveryEventFiring, now); err != nil {
						return err
					}
				}
				result = evaluationResult(active, transition)
				return nil
			}
			if hasActive {
				wasFiring := active.Status == dto.AlertStatusFiring
				if err := updateResolvedAlertEvidence(evaluation, &active, canonical); err != nil {
					return err
				}
				resolveAlertEvent(&active, now, alertResolutionReason(evaluation))
				if err := repository.SaveEvent(ctx, &active); err != nil {
					return err
				}
				if wasFiring && active.ResolutionReason != nil && *active.ResolutionReason == alertResolutionRecovered {
					if err := enqueueAlertTransition(ctx, repository, active, model.AlertDeliveryEventResolved, now); err != nil {
						return err
					}
				}
			}
			status, firstFiredAt := dto.AlertStatusPending, (*int64)(nil)
			transition := "pending"
			if candidate.ForTimes <= 1 {
				status, firstFiredAt, transition = dto.AlertStatusFiring, &now, "firing"
			}
			event := model.AlertEvent{
				RuleID: candidate.ID, RuleKey: candidate.RuleKey, SiteID: evaluation.SiteID,
				TargetType: evaluation.TargetType, TargetKey: targetKey, ActiveKey: &activeKey,
				Level: candidate.Level, Status: status, ConsecutiveCount: 1, CurrentValue: &canonical,
				ThresholdValue: candidate.ThresholdValue, MessageCode: string(message.Code),
				MessageParams: params, Message: message.TechnicalDetail, FirstObservedAt: now,
				FirstFiredAt: firstFiredAt, LastFiredAt: firstFiredAt, CreatedAt: now, UpdatedAt: now,
			}
			if err := repository.CreateEvent(ctx, &event); err != nil {
				return err
			}
			if transition == "firing" {
				if err := enqueueAlertTransition(ctx, repository, event, model.AlertDeliveryEventFiring, now); err != nil {
					return err
				}
			}
			result = evaluationResult(event, transition)
			return nil
		}()
		if mutationErr != nil {
			return mutationErr
		}
		if evaluation.State != AlertSampleUnknown {
			processedAt := service.clock.Now().Unix()
			if processedAt <= 0 {
				return errors.New("alert evaluation clock is invalid")
			}
			if hasCursor {
				cursor.LastSampleAt, cursor.LastSampleKey, cursor.UpdatedAt = evaluation.ObservedAt, evaluation.SampleKey, processedAt
				return repository.AdvanceEvaluationCursor(ctx, &cursor)
			}
			return repository.CreateEvaluationCursor(ctx, &model.AlertEvaluationCursor{
				ActiveKey: activeKey, LastSampleAt: evaluation.ObservedAt, LastSampleKey: evaluation.SampleKey,
				CreatedAt: processedAt, UpdatedAt: processedAt,
			})
		}
		return nil
	})
	if err != nil {
		return AlertEvaluationResult{}, errors.Join(ErrAlertEvaluation, err)
	}
	if service.metrics != nil {
		recordServiceMetric(func() {
			service.metrics.IncrementAlertTransition(result.Level, result.Transition)
		})
	}
	return result, nil
}

func alertEventItem(event model.AlertEventView) dto.AlertEventItem {
	var siteID *string
	if event.SiteID != nil {
		value := strconv.FormatInt(*event.SiteID, 10)
		siteID = &value
	}
	return dto.AlertEventItem{
		ID: strconv.FormatInt(event.ID, 10), RuleID: strconv.FormatInt(event.RuleID, 10), RuleKey: event.RuleKey,
		SiteID: siteID, SiteName: event.SiteName, TargetType: event.TargetType, TargetKey: event.TargetKey,
		TargetName: event.TargetName, Level: event.Level, Status: event.Status, CurrentValue: event.CurrentValue,
		ThresholdValue: event.ThresholdValue, Message: alertMessageRef(event), FirstObservedAt: event.FirstObservedAt,
		FirstFiredAt: event.FirstFiredAt, LastFiredAt: event.LastFiredAt, ResolvedAt: event.ResolvedAt, ResolutionReason: event.ResolutionReason,
	}
}

func alertMessageRef(event model.AlertEventView) dto.MessageRef {
	params := map[string]any{}
	if event.MessageParams != nil {
		_ = json.Unmarshal([]byte(*event.MessageParams), &params)
	}
	message, err := dto.NewMessageRef(constant.MessageCode(event.MessageCode), params, event.Message)
	if err == nil {
		return message
	}
	fallback, _ := dto.NewMessageRef(constant.MessageInternalContractError, map[string]any{"component": "alert_event", "value": event.MessageCode}, event.Message)
	return fallback
}

func alertRuleItems(rules []model.EffectiveAlertRule) []dto.AlertRuleItem {
	items := make([]dto.AlertRuleItem, len(rules))
	paired := map[string]map[string]int64{}
	for _, rule := range rules {
		if paired[rule.RuleKey] == nil {
			paired[rule.RuleKey] = map[string]int64{}
		}
		paired[rule.RuleKey][rule.Level] = rule.ID
	}
	for index, rule := range rules {
		id, baseID := strconv.FormatInt(rule.ID, 10), strconv.FormatInt(rule.BaseRuleID, 10)
		var overrideID *string
		if rule.OverrideRuleID != nil {
			value := strconv.FormatInt(*rule.OverrideRuleID, 10)
			overrideID = &value
		}
		constraints, editable := alertRuleConstraints(rule.AlertRule)
		other := ""
		if rule.Level == dto.AlertLevelWarning {
			other = dto.AlertLevelCritical
		} else if rule.Level == dto.AlertLevelCritical {
			other = dto.AlertLevelWarning
		}
		if pairID := paired[rule.RuleKey][other]; pairID > 0 {
			value, relation := strconv.FormatInt(pairID, 10), "warning_lt_critical"
			constraints.PairedRuleID, constraints.Relation = &value, &relation
		}
		items[index] = dto.AlertRuleItem{
			ID: id, EffectiveRuleID: id, BaseRuleID: baseID, OverrideRuleID: overrideID,
			RuleKey: rule.RuleKey, Category: alertRuleCategory(rule.AlertRule), Name: rule.Name, Enabled: rule.Enabled, Level: rule.Level,
			Metric: rule.Metric, CompareOperator: rule.CompareOperator, ThresholdValue: rule.ThresholdValue,
			ForTimes: rule.ForTimes, ScopeType: rule.ScopeType, ScopeID: strconv.FormatInt(rule.ScopeID, 10),
			Inherited: rule.Inherited, EditableFields: editable, Constraints: constraints, UpdatedAt: rule.UpdatedAt,
		}
	}
	return items
}

func alertRuleCategory(rule model.AlertRule) string {
	if strings.HasPrefix(rule.RuleKey, "channel_") {
		return dto.AlertRuleCategoryChannel
	}
	if contract, exists := alertRuleContracts[rule.RuleKey]; exists {
		switch contract.TargetType {
		case dto.AlertRuleCategoryCollection, dto.AlertRuleCategoryInstance, dto.AlertRuleCategoryAccount:
			return contract.TargetType
		default:
			return dto.AlertRuleCategorySite
		}
	}
	prefix, _, _ := strings.Cut(rule.Metric, ".")
	switch prefix {
	case dto.AlertRuleCategoryCollection, dto.AlertRuleCategoryInstance, dto.AlertRuleCategoryAccount, dto.AlertRuleCategoryChannel:
		return prefix
	default:
		return dto.AlertRuleCategorySite
	}
}

func filterAlertRuleItems(items []dto.AlertRuleItem, query dto.AlertRuleListQuery) []dto.AlertRuleItem {
	filtered := make([]dto.AlertRuleItem, 0, len(items))
	for _, item := range items {
		if len(query.Categories) > 0 && !alertRuleStringIn(item.Category, query.Categories) {
			continue
		}
		if len(query.Levels) > 0 && !alertRuleStringIn(item.Level, query.Levels) {
			continue
		}
		if query.Enabled != nil && item.Enabled != *query.Enabled {
			continue
		}
		if query.Inherited != nil && item.Inherited != *query.Inherited {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func alertRuleStringIn(value string, choices []string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func sortAlertRuleItems(items []dto.AlertRuleItem, sortBy, sortOrder string) {
	descending := sortOrder == "desc"
	sort.SliceStable(items, func(left, right int) bool {
		a, b := items[left], items[right]
		comparison := compareAlertRuleField(a, b, sortBy)
		if comparison != 0 {
			if descending {
				return comparison > 0
			}
			return comparison < 0
		}
		if sortBy == "" {
			if comparison = strings.Compare(a.Category, b.Category); comparison != 0 {
				return comparison < 0
			}
		}
		if comparison = strings.Compare(a.RuleKey, b.RuleKey); comparison != 0 {
			return comparison < 0
		}
		if comparison = compareAlertRuleLevelDefault(a.Level, b.Level); comparison != 0 {
			return comparison < 0
		}
		return a.ID < b.ID
	})
}

func compareAlertRuleField(left, right dto.AlertRuleItem, sortBy string) int {
	switch sortBy {
	case "category":
		return strings.Compare(left.Category, right.Category)
	case "rule_key":
		return strings.Compare(left.RuleKey, right.RuleKey)
	case "level":
		return alertRuleLevelRank(left.Level) - alertRuleLevelRank(right.Level)
	case "metric":
		return strings.Compare(left.Metric, right.Metric)
	case "enabled":
		return compareAlertRuleBool(left.Enabled, right.Enabled)
	case "updated_at":
		return compareAlertRuleInt64(left.UpdatedAt, right.UpdatedAt)
	default:
		return 0
	}
}

func alertRuleLevelRank(level string) int {
	switch level {
	case dto.AlertLevelInfo:
		return 0
	case dto.AlertLevelWarning:
		return 1
	case dto.AlertLevelCritical:
		return 2
	default:
		return 3
	}
}

func compareAlertRuleLevelDefault(left, right string) int {
	return alertRuleLevelRank(right) - alertRuleLevelRank(left)
}

func compareAlertRuleBool(left, right bool) int {
	if left == right {
		return 0
	}
	if left {
		return 1
	}
	return -1
}

func compareAlertRuleInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func alertRuleConstraints(rule model.AlertRule) (dto.AlertRuleConstraints, []string) {
	kind := alertValueKind(rule.Metric)
	constraints := dto.AlertRuleConstraints{ValueKind: kind, ForTimesMin: 1, ForTimesMax: 60}
	fields := []string{"enabled"}
	if kind == "boolean" {
		return constraints, fields
	}
	constraints.ThresholdEditable, constraints.ForTimesEditable = true, true
	fields = append(fields, "threshold_value", "for_times")
	min, step := "0", "1"
	constraints.ThresholdMin, constraints.ThresholdStep = &min, &step
	if kind == "percentage" {
		min, max, decimalStep := "1", "100", "0.1"
		constraints.ThresholdMin, constraints.ThresholdMax, constraints.ThresholdStep = &min, &max, &decimalStep
	}
	if kind == "seconds" {
		min, max := "1", "86400"
		constraints.ThresholdMin, constraints.ThresholdMax = &min, &max
	}
	if kind == "ratio" {
		min, max, decimalStep := "0", "1", "0.01"
		constraints.ThresholdMin, constraints.ThresholdMax, constraints.ThresholdStep = &min, &max, &decimalStep
	}
	if kind == "decimal" {
		decimalStep := "0.0000000001"
		constraints.ThresholdStep = &decimalStep
	}
	return constraints, fields
}

func alertValueKind(metric string) string {
	if strings.Contains(metric, "percent") {
		return "percentage"
	}
	if strings.Contains(metric, "stale_seconds") {
		return "seconds"
	}
	if metric == "account.quota" {
		return "quota"
	}
	if metric == ChannelMetricAvailabilityRate {
		return "ratio"
	}
	if metric == ChannelMetricBalanceTotal {
		return "decimal"
	}
	if metric == ChannelMetricResponseTimeAvgMS || metric == ChannelMetricResponseTimeMaxMS {
		return "milliseconds"
	}
	boolean := map[string]bool{"site.auth_expired": true, "site.data_export_enabled": true, "instance.online": true, "account.remote_exists": true, "account.identity_match": true, "account.remote_enabled": true}
	if boolean[metric] {
		return "boolean"
	}
	return "count"
}

func alertRulePatch(rule model.AlertRule, enabled *bool, threshold *string, forTimes *int, now int64) (model.AlertRulePatch, map[string]string) {
	patch := model.AlertRulePatch{Enabled: enabled, ForTimes: forTimes, UpdatedAt: now}
	errors := map[string]string{}
	kind := alertValueKind(rule.Metric)
	if kind == "boolean" {
		if threshold != nil {
			errors["threshold_value"] = "is fixed for boolean rules"
		}
		if forTimes != nil {
			errors["for_times"] = "is fixed for boolean rules"
		}
		return patch, nilIfNoAlertServiceErrors(errors)
	}
	if threshold != nil {
		value, canonical, valid := parseAlertThreshold(*threshold, false)
		if !valid {
			errors["threshold_value"] = "must be a non-negative decimal with at most 2 fractional digits"
		} else {
			if kind == "percentage" && (value.Cmp(big.NewRat(1, 1)) < 0 || value.Cmp(big.NewRat(100, 1)) > 0) {
				errors["threshold_value"] = "must be between 1 and 100"
			}
			if kind == "seconds" && (value.Cmp(big.NewRat(1, 1)) < 0 || value.Cmp(big.NewRat(86400, 1)) > 0) {
				errors["threshold_value"] = "must be between 1 and 86400"
			}
			if kind == "ratio" && (value.Sign() < 0 || value.Cmp(big.NewRat(1, 1)) > 0) {
				errors["threshold_value"] = "must be between 0 and 1"
			}
			patch.ThresholdValue = &canonical
		}
	}
	if forTimes != nil && (*forTimes < 1 || *forTimes > 60) {
		errors["for_times"] = "must be between 1 and 60"
	}
	return patch, nilIfNoAlertServiceErrors(errors)
}

func validateAlertPairs(rules []model.AlertRule, target model.AlertRule, patch model.AlertRulePatch) map[string]string {
	replacement := applyAlertPatch(target, patch)
	updated := append([]model.AlertRule(nil), rules...)
	replaced := false
	for index := range updated {
		if updated[index].ID == target.ID && updated[index].ScopeType == target.ScopeType && updated[index].ScopeID == target.ScopeID {
			updated[index] = replacement
			replaced = true
			break
		}
	}
	if !replaced {
		updated = append(updated, replacement)
	}
	siteIDs := map[int64]struct{}{0: {}}
	for _, rule := range updated {
		if rule.ScopeType == dto.AlertScopeSite && rule.ScopeID > 0 {
			siteIDs[rule.ScopeID] = struct{}{}
		}
	}
	for siteID := range siteIDs {
		if !validEffectiveAlertPair(effectiveAlertRulesForSite(updated, siteID)) {
			return map[string]string{"threshold_value": "warning and critical thresholds are inconsistent"}
		}
	}
	return nil
}

func validEffectiveAlertPair(effective map[string]model.AlertRule) bool {
	warning, warningOK := effective[dto.AlertLevelWarning]
	critical, criticalOK := effective[dto.AlertLevelCritical]
	if !warningOK || !criticalOK || warning.ThresholdValue == nil || critical.ThresholdValue == nil {
		return true
	}
	warningValue, _, warningValid := parseAlertThreshold(*warning.ThresholdValue, true)
	criticalValue, _, criticalValid := parseAlertThreshold(*critical.ThresholdValue, true)
	if !warningValid || !criticalValid || warning.CompareOperator != critical.CompareOperator {
		return false
	}
	switch warning.CompareOperator {
	case ">=":
		return warningValue.Cmp(criticalValue) < 0
	case "<=":
		return warningValue.Cmp(criticalValue) > 0
	case "==":
		return warningValue.Cmp(criticalValue) == 0
	default:
		return false
	}
}

func effectiveAlertRulesForSite(rules []model.AlertRule, siteID int64) map[string]model.AlertRule {
	result := map[string]model.AlertRule{}
	for _, rule := range rules {
		if rule.ScopeType == dto.AlertScopeGlobal && rule.ScopeID == 0 {
			result[rule.Level] = rule
		}
	}
	if siteID <= 0 {
		return result
	}
	for _, rule := range rules {
		if rule.ScopeType == dto.AlertScopeSite && rule.ScopeID == siteID {
			result[rule.Level] = rule
		}
	}
	return result
}

func effectiveAlertRules(rules []model.AlertRule) map[string]model.AlertRule {
	result := map[string]model.AlertRule{}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	for _, rule := range rules {
		current, exists := result[rule.Level]
		if !exists || (current.ScopeType == dto.AlertScopeGlobal && rule.ScopeType == dto.AlertScopeSite) {
			result[rule.Level] = rule
		}
	}
	return result
}

func matchingAlertRule(rules map[string]model.AlertRule, value *big.Rat) *model.AlertRule {
	for _, level := range []string{dto.AlertLevelCritical, dto.AlertLevelWarning, dto.AlertLevelInfo} {
		rule, exists := rules[level]
		if !exists || !rule.Enabled || rule.ThresholdValue == nil {
			continue
		}
		threshold, _, valid := parseAlertThreshold(*rule.ThresholdValue, true)
		if !valid {
			continue
		}
		comparison := value.Cmp(threshold)
		matches := (rule.CompareOperator == ">=" && comparison >= 0) || (rule.CompareOperator == "<=" && comparison <= 0) || (rule.CompareOperator == "==" && comparison == 0)
		if matches {
			selected := rule
			return &selected
		}
	}
	return nil
}

var (
	alertDecimalPattern   = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,10})?$`)
	alertThresholdPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,2})?$`)
)

func parseAlertThreshold(raw string, allowNegative bool) (*big.Rat, string, bool) {
	if raw != strings.TrimSpace(raw) || !alertThresholdPattern.MatchString(raw) {
		return nil, "", false
	}
	return parseAlertDecimal(raw, allowNegative)
}

func parseAlertDecimal(raw string, allowNegative bool) (*big.Rat, string, bool) {
	if raw != strings.TrimSpace(raw) || !alertDecimalPattern.MatchString(raw) {
		return nil, "", false
	}
	parts := strings.Split(raw, ".")
	integer := strings.TrimPrefix(parts[0], "-")
	if len(integer) > 20 {
		return nil, "", false
	}
	value, valid := new(big.Rat).SetString(raw)
	if !valid || (!allowNegative && value.Sign() < 0) {
		return nil, "", false
	}
	canonical := raw
	if len(parts) == 2 {
		canonical = parts[0] + "." + strings.TrimRight(parts[1], "0")
		canonical = strings.TrimSuffix(canonical, ".")
	}
	if value.Sign() == 0 {
		canonical = "0"
	}
	return value, canonical, true
}

func validateAlertEvaluation(evaluation AlertEvaluation) map[string]string {
	errors := map[string]string{}
	if evaluation.RuleKey == "" || len(evaluation.RuleKey) > 64 {
		errors["rule_key"] = "must be between 1 and 64 characters"
	}
	contract, exists := alertRuleContracts[evaluation.RuleKey]
	if !exists {
		errors["rule_key"] = "is not a built-in alert rule"
	} else {
		if evaluation.TargetType != contract.TargetType {
			errors["target_type"] = "does not apply to this alert rule"
		}
		if evaluation.SiteID == nil || *evaluation.SiteID <= 0 {
			errors["site_id"] = "is required and must be positive"
		}
		target, valid := canonicalAlertTargetKey(evaluation, contract)
		if !valid || len(target.Key) > 255 || strings.ContainsAny(target.Key, "\x00\r\n") {
			errors["target_key"] = "is not canonical for this alert rule"
		}
	}
	if !alertOneOfService(string(evaluation.State), string(AlertSampleKnown), string(AlertSampleUnknown), string(AlertSampleScopeInactive)) {
		errors["state"] = "is invalid"
	}
	if evaluation.State == AlertSampleKnown && evaluation.CurrentValue == nil {
		errors["current_value"] = "is required for known samples"
	}
	if evaluation.State == AlertSampleScopeInactive && evaluation.ScopeID <= 0 {
		errors["scope_id"] = "is required for an inactive scope"
	}
	if evaluation.ResolutionReason != "" && !alertOneOfService(evaluation.ResolutionReason, alertResolutionRecovered, alertResolutionRemediated, alertResolutionRetired, alertResolutionSuperseded) {
		errors["resolution_reason"] = "is invalid"
	}
	if evaluation.ObservedAt <= 0 {
		errors["observed_at"] = "is required and must be positive"
	}
	if evaluation.SampleKey == "" || len(evaluation.SampleKey) > 255 ||
		strings.TrimSpace(evaluation.SampleKey) != evaluation.SampleKey || strings.ContainsAny(evaluation.SampleKey, "\x00\r\n") {
		errors["sample_key"] = "must be a canonical sample identity"
	}
	return nilIfNoAlertServiceErrors(errors)
}

func alertEvaluationAlreadyApplied(
	cursor model.AlertEvaluationCursor,
	hasCursor bool,
	evaluation AlertEvaluation,
) (bool, error) {
	if !hasCursor || evaluation.ObservedAt > cursor.LastSampleAt {
		return false, nil
	}
	if evaluation.ObservedAt < cursor.LastSampleAt || evaluation.SampleKey == cursor.LastSampleKey {
		return true, nil
	}
	return false, fmt.Errorf("%w: active_key=%s observed_at=%d", ErrAlertSampleConflict, cursor.ActiveKey, evaluation.ObservedAt)
}

func alertEvaluationIdentityUpgrade(
	cursor model.AlertEvaluationCursor,
	hasCursor bool,
	evaluation AlertEvaluation,
) bool {
	if !hasCursor || evaluation.ObservedAt != cursor.LastSampleAt || evaluation.SampleKey == cursor.LastSampleKey {
		return false
	}
	for _, prior := range evaluation.PriorSampleKeys {
		if prior == cursor.LastSampleKey {
			return true
		}
	}
	return false
}

func buildAlertMessage(
	evaluation AlertEvaluation,
	contract alertRuleContract,
	target canonicalAlertTarget,
	rule model.AlertRule,
	currentValue string,
) (dto.MessageRef, *string, error) {
	siteID := strconv.FormatInt(*evaluation.SiteID, 10)
	params := map[string]any{}
	switch evaluation.RuleKey {
	case "site_offline", "site_auth_expired", "site_export_disabled", "site_no_instance":
		params = map[string]any{"site_id": siteID, "site_name": evaluation.TargetName}
	case "collection_missing":
		params = map[string]any{"site_id": siteID, "start_timestamp": target.EntityID, "end_timestamp": target.EntityID + 3600}
	case "backfill_failed":
		params = map[string]any{"site_id": siteID, "run_id": strconv.FormatInt(target.EntityID, 10)}
	case "validation_failed":
		failureKind := strings.TrimSpace(evaluation.Source)
		if !alertOneOfService(failureKind, "data_mismatch", "execution_failed") {
			return dto.MessageRef{}, nil, &AlertValidationError{Fields: map[string]string{"source": "must identify the validation failure kind"}}
		}
		params = map[string]any{
			"site_id": siteID, "start_timestamp": target.EntityID, "end_timestamp": target.EntityID + 3600,
			"failure_kind": failureKind,
		}
	case "instance_stale", "instance_offline":
		params = map[string]any{"site_id": siteID, "instance_name": evaluation.TargetName}
	case "cpu_high", "memory_high", "disk_high":
		params = map[string]any{
			"site_id": siteID, "target_type": contract.TargetType, "target_name": evaluation.TargetName,
			"value": currentValue, "threshold": *rule.ThresholdValue,
		}
	case "account_missing", "account_identity_mismatch", "account_disabled", "account_quota_empty":
		params = map[string]any{
			"account_id": strconv.FormatInt(target.EntityID, 10), "account_name": evaluation.TargetName, "site_id": siteID,
		}
	case "channel_balance_low", "channel_response_time_high", "channel_availability_low":
		params = map[string]any{
			"site_id": siteID, "site_name": evaluation.TargetName,
			"value": currentValue, "threshold": *rule.ThresholdValue,
		}
	default:
		return dto.MessageRef{}, nil, &AlertValidationError{Fields: map[string]string{"rule_key": "is not a built-in alert rule"}}
	}
	message, err := dto.NewMessageRef(contract.MessageCode, params, alertTechnicalDetail(evaluation))
	if err != nil {
		return dto.MessageRef{}, nil, err
	}
	encoded, err := marshalAlertMessage(message)
	return message, encoded, err
}

func scopeInactiveMessage(evaluation AlertEvaluation) (dto.MessageRef, *string, error) {
	scopeType := strings.TrimSpace(evaluation.ScopeType)
	if scopeType == "" {
		scopeType = evaluation.TargetType
	}
	scopeName := evaluation.ScopeName
	if scopeName == "" {
		scopeName = evaluation.TargetName
	}
	message, err := dto.NewMessageRef(constant.MessageAlertScopeInactive, map[string]any{
		"scope_type": scopeType, "scope_id": strconv.FormatInt(evaluation.ScopeID, 10), "scope_name": scopeName,
	}, alertTechnicalDetail(evaluation))
	if err != nil {
		return dto.MessageRef{}, nil, err
	}
	params, err := marshalAlertMessage(message)
	return message, params, err
}

func marshalAlertMessage(message dto.MessageRef) (*string, error) {
	if err := dto.ValidateMessageParams(message.Code, message.Params); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(message.Params)
	if err != nil {
		return nil, err
	}
	value := string(encoded)
	return &value, nil
}

func alertTechnicalDetail(evaluation AlertEvaluation) string {
	detail := evaluation.Message.TechnicalDetail
	evidence := []string{}
	if evaluation.Source != "" {
		evidence = append(evidence, "source="+evaluation.Source)
	}
	if evaluation.RequestID != "" {
		evidence = append(evidence, "request_id="+evaluation.RequestID)
	}
	if len(evidence) == 0 {
		return detail
	}
	if detail != "" {
		evidence = append([]string{detail}, evidence...)
	}
	return strings.Join(evidence, "; ")
}

func updateResolvedAlertEvidence(evaluation AlertEvaluation, event *model.AlertEvent, currentValue string) error {
	if event == nil || event.MessageCode == "" {
		return errors.New("resolved alert evidence is invalid")
	}
	params := map[string]any{}
	if event.MessageParams != nil {
		if err := json.Unmarshal([]byte(*event.MessageParams), &params); err != nil {
			return err
		}
	}
	if _, exists := params["value"]; exists {
		params["value"] = currentValue
	}
	if _, exists := params["threshold"]; exists {
		if event.ThresholdValue == nil {
			return errors.New("resolved alert threshold evidence is invalid")
		}
		params["threshold"] = *event.ThresholdValue
	}
	safeEvaluation := evaluation
	safeEvaluation.Message.TechnicalDetail = ""
	message, err := dto.NewMessageRef(
		constant.MessageCode(event.MessageCode),
		params,
		alertTechnicalDetail(safeEvaluation),
	)
	if err != nil {
		return err
	}
	encoded, err := marshalAlertMessage(message)
	if err != nil {
		return err
	}
	event.CurrentValue = &currentValue
	event.MessageParams = encoded
	event.Message = message.TechnicalDetail
	return nil
}

const (
	alertResolutionRecovered  = "recovered"
	alertResolutionRemediated = "remediated"
	alertResolutionRetired    = "retired"
	alertResolutionSuperseded = "superseded"
)

func alertResolutionReason(evaluation AlertEvaluation) string {
	if evaluation.ResolutionReason != "" {
		return evaluation.ResolutionReason
	}
	if evaluation.RuleKey == "backfill_failed" {
		return alertResolutionRemediated
	}
	return alertResolutionRecovered
}

func resolveAlertEvent(event *model.AlertEvent, now int64, reason string) {
	event.Status, event.ActiveKey, event.ResolvedAt, event.UpdatedAt = dto.AlertStatusResolved, nil, &now, now
	event.ResolutionReason = &reason
}

func enqueueAlertTransition(
	ctx context.Context,
	repository *model.AlertRepository,
	event model.AlertEvent,
	eventType string,
	now int64,
) error {
	if event.Level != dto.AlertLevelWarning && event.Level != dto.AlertLevelCritical {
		return nil
	}
	enabled, err := repository.LockDingTalkEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	eventView, err := repository.FindEvent(ctx, event.ID)
	if err != nil {
		return err
	}
	payload, err := marshalDingTalkDeliverySnapshot(newDingTalkAlertSnapshot(eventView, eventType))
	if err != nil {
		return err
	}
	_, _, err = repository.EnqueueDelivery(ctx, event.ID, eventType, payload, now)
	return err
}

func evaluationResult(event model.AlertEvent, transition string) AlertEvaluationResult {
	return AlertEvaluationResult{EventID: event.ID, Status: event.Status, Level: event.Level, Transition: transition}
}

func alertActiveKey(ruleKey, targetType, targetKey string) string {
	digest := sha256.Sum256([]byte(ruleKey + "\x00" + targetType + "\x00" + targetKey))
	return "v1:" + hex.EncodeToString(digest[:])
}

func alertSiteID(siteID *int64) int64 {
	if siteID == nil {
		return 0
	}
	return *siteID
}

func canonicalAlertTargetKey(evaluation AlertEvaluation, contract alertRuleContract) (canonicalAlertTarget, bool) {
	if evaluation.SiteID == nil || *evaluation.SiteID <= 0 || evaluation.TargetType != contract.TargetType {
		return canonicalAlertTarget{}, false
	}
	raw := evaluation.TargetKey
	if raw == "" {
		return canonicalAlertTarget{}, false
	}
	canonicalID := func(value string) (string, int64, bool) {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
			return "", 0, false
		}
		return value, parsed, true
	}
	siteKey := strconv.FormatInt(*evaluation.SiteID, 10)
	switch contract.TargetType {
	case "site":
		key, id, valid := canonicalID(raw)
		if !valid || key != siteKey {
			return canonicalAlertTarget{}, false
		}
		return canonicalAlertTarget{Key: key, EntityID: id}, true
	case "account":
		key, id, valid := canonicalID(raw)
		return canonicalAlertTarget{Key: key, EntityID: id}, valid
	case "instance", "collection":
		parts := strings.SplitN(raw, "/", 2)
		if len(parts) != 2 || parts[1] == "" || parts[1] != strings.TrimSpace(parts[1]) {
			return canonicalAlertTarget{}, false
		}
		prefix, _, valid := canonicalID(parts[0])
		if !valid || prefix != siteKey {
			return canonicalAlertTarget{}, false
		}
		if contract.TargetType == "instance" {
			if strings.ContainsAny(parts[1], "\x00\r\n") {
				return canonicalAlertTarget{}, false
			}
			return canonicalAlertTarget{Key: prefix + "/" + parts[1]}, true
		}
		suffix, id, valid := canonicalID(parts[1])
		if !valid || contract.CollectionTarget == alertCollectionTargetNone ||
			(contract.CollectionTarget == alertCollectionTargetHour && id > 1<<63-1-3600) {
			return canonicalAlertTarget{}, false
		}
		return canonicalAlertTarget{Key: prefix + "/" + suffix, EntityID: id}, true
	default:
		return canonicalAlertTarget{}, false
	}
}

func alertRuleByID(rules []model.AlertRule, id int64) (model.AlertRule, bool) {
	for _, rule := range rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return model.AlertRule{}, false
}

func applyAlertPatch(rule model.AlertRule, patch model.AlertRulePatch) model.AlertRule {
	if patch.Enabled != nil {
		rule.Enabled = *patch.Enabled
	}
	if patch.ThresholdValue != nil {
		rule.ThresholdValue = patch.ThresholdValue
	}
	if patch.ForTimes != nil {
		rule.ForTimes = *patch.ForTimes
	}
	rule.UpdatedAt = patch.UpdatedAt
	return rule
}

func nilIfNoAlertServiceErrors(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	return fields
}
func alertOneOfService(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

var _ AlertEvaluator = (*AlertService)(nil)
