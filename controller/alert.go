package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type AlertController struct{ alerts *service.AlertService }

func NewAlertController(alerts *service.AlertService) *AlertController {
	return &AlertController{alerts: alerts}
}

func (controller *AlertController) Summary(c *gin.Context) {
	if !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	result, err := controller.alerts.Summary(c.Request.Context())
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) List(c *gin.Context) {
	if !requireEmptyBody(c) {
		return
	}
	query, fields := parseAlertListQuery(c)
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert list query", fields)
		return
	}
	result, err := controller.alerts.List(c.Request.Context(), query)
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) Get(c *gin.Context) {
	if !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	id, ok := parseAlertID(c)
	if !ok {
		return
	}
	result, err := controller.alerts.Get(c.Request.Context(), id)
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) ListRules(c *gin.Context) {
	if !requireEmptyBody(c) {
		return
	}
	scopeType, scopeID, fields := parseAlertRuleScope(c)
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert rule scope", fields)
		return
	}
	result, err := controller.alerts.ListRules(c.Request.Context(), scopeType, scopeID)
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) UpdateRule(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	id, ok := parseAlertID(c)
	if !ok {
		return
	}
	var request dto.AlertRuleUpdateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	if fields := request.Validate(); fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert rule", fields)
		return
	}
	result, err := controller.alerts.UpdateRule(c.Request.Context(), id, request)
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) CreateOverride(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	var request dto.AlertRuleOverrideRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	if fields := request.Validate(); fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert rule override", fields)
		return
	}
	result, err := controller.alerts.CreateOverride(c.Request.Context(), request)
	if err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *AlertController) DeleteOverride(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	id, ok := parseAlertID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	if err := controller.alerts.DeleteOverride(c.Request.Context(), id); err != nil {
		writeAlertServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func parseAlertID(c *gin.Context) (int64, bool) {
	raw := c.Param("id")
	id, ok := parsePositiveID(raw)
	if !ok || strconv.FormatInt(id, 10) != raw {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert ID", map[string]string{"id": "must be a canonical positive decimal int64"})
		return 0, false
	}
	return id, true
}

func parseAlertListQuery(c *gin.Context) (dto.AlertListQuery, map[string]string) {
	allowed := map[string]struct{}{"p": {}, "page_size": {}, "status": {}, "level": {}, "target_type": {}, "site_id": {}, "start_timestamp": {}, "end_timestamp": {}, "sort_by": {}, "sort_order": {}}
	fields := validateQueryKeysAllowRepeated(c, allowed, map[string]struct{}{
		"status": {}, "level": {}, "target_type": {},
	})
	query := dto.AlertListQuery{Page: 1, PageSize: 20, SortOrder: "desc"}
	parsePageQuery(c, &query.Page, &query.PageSize, fields)
	query.Statuses = parseRepeatedEnumQuery(c, "status", fields, func(value string) bool {
		return value == dto.AlertStatusPending || value == dto.AlertStatusFiring || value == dto.AlertStatusResolved
	})
	query.Levels = parseRepeatedEnumQuery(c, "level", fields, func(value string) bool {
		return value == dto.AlertLevelInfo || value == dto.AlertLevelWarning || value == dto.AlertLevelCritical
	})
	query.TargetTypes = parseRepeatedEnumQuery(c, "target_type", fields, func(value string) bool {
		return value == "site" || value == "instance" || value == "account" || value == "collection"
	})
	query.SortBy, _ = singletonQueryValue(c, "sort_by", fields)
	if value, exists := singletonQueryValue(c, "sort_order", fields); exists {
		query.SortOrder = value
	}
	query.SiteID = parseOptionalAlertInt64(c, "site_id", true, fields)
	query.StartTimestamp = parseOptionalAlertInt64(c, "start_timestamp", false, fields)
	query.EndTimestamp = parseOptionalAlertInt64(c, "end_timestamp", false, fields)
	query.Normalize()
	mergeFieldErrors(fields, query.Validate())
	if len(fields) > 0 {
		return dto.AlertListQuery{}, fields
	}
	return query, nil
}

func parseAlertRuleScope(c *gin.Context) (string, int64, map[string]string) {
	fields := validateQueryKeys(c, map[string]struct{}{"scope_type": {}, "scope_id": {}})
	scopeType := dto.AlertScopeGlobal
	if value, exists := singletonQueryValue(c, "scope_type", fields); exists {
		scopeType = strings.ToLower(value)
	}
	rawScopeID, hasScopeID := singletonQueryValue(c, "scope_id", fields)
	var scopeID int64
	if scopeType == dto.AlertScopeGlobal {
		if hasScopeID && rawScopeID != "0" {
			fields["scope_id"] = "must be 0 for global scope"
		}
	} else if scopeType == dto.AlertScopeSite {
		parsed, valid := parsePositiveID(rawScopeID)
		if !hasScopeID || !valid || strconv.FormatInt(parsed, 10) != rawScopeID {
			fields["scope_id"] = "must be a canonical positive decimal int64 for site scope"
		} else {
			scopeID = parsed
		}
	} else {
		fields["scope_type"] = "must be global or site"
	}
	if len(fields) > 0 {
		return "", 0, fields
	}
	return scopeType, scopeID, nil
}

func parseOptionalAlertInt64(c *gin.Context, key string, positive bool, fields map[string]string) *int64 {
	raw, exists := singletonQueryValue(c, key, fields)
	if !exists {
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || strconv.FormatInt(value, 10) != raw || (positive && value <= 0) || (!positive && value < 0) {
		fields[key] = "must be a canonical non-negative integer"
		return nil
	}
	return &value
}

func writeAlertServiceError(c *gin.Context, err error) {
	var validation *service.AlertValidationError
	switch {
	case errors.As(err, &validation):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert rule", validation.Fields)
	case errors.Is(err, service.ErrAlertNotFound), errors.Is(err, service.ErrAlertRuleNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Alert resource not found", nil)
	case errors.Is(err, service.ErrAlertRuleConflict):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Alert rule override already exists", nil)
	case errors.Is(err, service.ErrAlertRuleInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid alert request", nil)
	default:
		common.AbortInternalError(c)
	}
}
