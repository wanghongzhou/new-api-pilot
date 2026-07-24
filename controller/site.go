package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type SiteController struct {
	sites *service.SiteService
}

func NewSiteController(sites *service.SiteService) *SiteController {
	return &SiteController{sites: sites}
}

func (controller *SiteController) List(c *gin.Context) {
	query, fieldErrors := parseSiteListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site list query", fieldErrors)
		return
	}
	page, err := controller.sites.List(c.Request.Context(), query)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *SiteController) Create(c *gin.Context) {
	var request dto.SiteCreateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site", fieldErrors)
		return
	}
	detail, err := controller.sites.Create(c.Request.Context(), request)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) Get(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	detail, err := controller.sites.Get(c.Request.Context(), siteID)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) Performance(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours < 1 || hours > 720 {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid performance range", map[string]string{"hours": "must be between 1 and 720"})
		return
	}
	data, err := controller.sites.PerformanceSummary(c.Request.Context(), siteID, hours, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, data)
}

func (controller *SiteController) PreflightBaseURL(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	var request dto.SiteBaseURLPreflightRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site base URL", fieldErrors)
		return
	}
	result, err := controller.sites.PreflightBaseURL(c.Request.Context(), siteID, request.BaseURL, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *SiteController) Update(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	var request dto.SiteUpdateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site", fieldErrors)
		return
	}
	detail, err := controller.sites.Update(c.Request.Context(), siteID, request)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) Delete(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	if err := controller.sites.Delete(c.Request.Context(), siteID); err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func (controller *SiteController) Authorize(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	var request dto.SiteAuthorizeRequest
	if !decodeJSON(c, &request, common.CredentialBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site authorization", fieldErrors)
		return
	}
	result, err := controller.sites.Authorize(c.Request.Context(), siteID, request, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *SiteController) RecheckCapabilities(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	result, err := controller.sites.RecheckCapabilities(c.Request.Context(), siteID, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *SiteController) Probe(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	result, err := controller.sites.Probe(c.Request.Context(), siteID, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *SiteController) RefreshBatch(c *gin.Context) {
	var request dto.SiteBatchRefreshRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site refresh request", fieldErrors)
		return
	}
	runs, err := controller.sites.QueueRefresh(c.Request.Context(), request.ParsedSiteIDs(), common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, runs)
}

func (controller *SiteController) Refresh(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	runs, err := controller.sites.QueueRefresh(c.Request.Context(), []int64{siteID}, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, runs)
}

func (controller *SiteController) Backfill(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	var request dto.SiteBackfillRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site backfill request", fieldErrors)
		return
	}
	run, err := controller.sites.Backfill(c.Request.Context(), siteID, request, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, run)
}

func (controller *SiteController) Disable(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	detail, err := controller.sites.Disable(c.Request.Context(), siteID)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) Enable(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	run, err := controller.sites.Enable(c.Request.Context(), siteID, common.RequestID(c))
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, run)
}

func (controller *SiteController) EndStatistics(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	var request dto.SiteStatisticsEndRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics end", fieldErrors)
		return
	}
	detail, err := controller.sites.EndStatistics(c.Request.Context(), siteID, request.StatisticsEndAt)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) ClearStatisticsEnd(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok || !requireEmptyBody(c) {
		return
	}
	detail, err := controller.sites.ClearStatisticsEnd(c.Request.Context(), siteID)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *SiteController) ListInstances(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	items, err := controller.sites.ListInstances(c.Request.Context(), siteID)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, items)
}

func (controller *SiteController) ListCollectionRuns(c *gin.Context) {
	siteID, ok := parseSiteID(c)
	if !ok {
		return
	}
	query, fieldErrors := parseCollectionRunListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid collection run list query", fieldErrors)
		return
	}
	page, err := controller.sites.ListCollectionRuns(c.Request.Context(), siteID, query)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *SiteController) GetCollectionRun(c *gin.Context) {
	runID, ok := parsePositivePathID(c, "id", "collection run")
	if !ok {
		return
	}
	run, err := controller.sites.GetCollectionRun(c.Request.Context(), runID)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, run)
}

func (controller *SiteController) ListCollectionRunWindows(c *gin.Context) {
	runID, ok := parsePositivePathID(c, "id", "collection run")
	if !ok {
		return
	}
	query, fieldErrors := parseCollectionRunWindowListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid collection run window query", fieldErrors)
		return
	}
	page, err := controller.sites.ListCollectionRunWindows(c.Request.Context(), runID, query)
	if err != nil {
		writeSiteServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func parseSiteID(c *gin.Context) (int64, bool) {
	return parsePositivePathID(c, "id", "site")
}

func parsePositivePathID(c *gin.Context, parameter, resource string) (int64, bool) {
	id, ok := parsePositiveID(c.Param(parameter))
	if !ok {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid "+resource+" ID", map[string]string{parameter: "must be a positive decimal integer"})
	}
	return id, ok
}

func requireEmptyBody(c *gin.Context) bool {
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, common.DefaultJSONBodyLimit+1))
	if err != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid request body", map[string]string{"body": "could not be read"})
		return false
	}
	if int64(len(data)) > common.DefaultJSONBodyLimit {
		common.AbortError(c, http.StatusRequestEntityTooLarge, constant.CodePayloadTooLarge, "Request body is too large", nil)
		return false
	}
	if len(bytes.TrimSpace(data)) != 0 {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid request body", map[string]string{"body": "must be empty"})
		return false
	}
	return true
}

func parseSiteListQuery(c *gin.Context) (dto.SiteListQuery, map[string]string) {
	query := dto.SiteListQuery{Page: 1, PageSize: 20, SortBy: "priority", SortOrder: "desc"}
	errors := validateQueryKeys(c, map[string]struct{}{
		"p": {}, "page_size": {}, "keyword": {}, "sort_by": {}, "sort_order": {},
		"management_status": {}, "online_status": {}, "auth_status": {}, "statistics_status": {}, "health_status": {},
	})
	parsePageQuery(c, &query.Page, &query.PageSize, errors)
	query.Keyword = strings.TrimSpace(c.Query("keyword"))
	if !utf8.ValidString(query.Keyword) || utf8.RuneCountInString(query.Keyword) > 128 {
		errors["keyword"] = "must not exceed 128 Unicode characters"
	}
	if raw := c.Query("sort_by"); raw != "" {
		query.SortBy = raw
	}
	if !oneOf(query.SortBy, "priority", "name", "today_quota", "updated_at") {
		errors["sort_by"] = "must be priority, name, today_quota, or updated_at"
	}
	if raw := c.Query("sort_order"); raw != "" {
		query.SortOrder = strings.ToLower(raw)
	}
	if !oneOf(query.SortOrder, "asc", "desc") {
		errors["sort_order"] = "must be asc or desc"
	}
	query.ManagementStatuses = parseEnumArray(c, "management_status", errors, dto.ValidSiteManagementStatus)
	query.OnlineStatuses = parseEnumArray(c, "online_status", errors, dto.ValidSiteOnlineStatus)
	query.AuthStatuses = parseEnumArray(c, "auth_status", errors, dto.ValidSiteAuthStatus)
	query.StatisticsStatuses = parseEnumArray(c, "statistics_status", errors, dto.ValidSiteStatisticsStatus)
	query.HealthStatuses = parseEnumArray(c, "health_status", errors, dto.ValidSiteHealthStatus)
	if len(errors) > 0 {
		return dto.SiteListQuery{}, errors
	}
	return query, nil
}

func parseCollectionRunListQuery(c *gin.Context) (dto.CollectionRunListQuery, map[string]string) {
	query := dto.CollectionRunListQuery{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}
	errors := validateQueryKeys(c, map[string]struct{}{
		"p": {}, "page_size": {}, "task_type": {}, "status": {}, "sort_by": {}, "sort_order": {},
	})
	parsePageQuery(c, &query.Page, &query.PageSize, errors)
	query.TaskType = c.Query("task_type")
	if query.TaskType != "" && !oneOf(query.TaskType,
		constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot,
		constant.TaskTypeUserSync, constant.TaskTypeChannelSync, constant.TaskTypeUsageHour,
		constant.TaskTypeUsageBackfill, constant.TaskTypeUsageValidation,
		constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild) {
		errors["task_type"] = "is not a supported collection task type"
	}
	query.Status = c.Query("status")
	if query.Status != "" && !oneOf(query.Status, "pending", "running", "success", "failed") {
		errors["status"] = "must be pending, running, success, or failed"
	}
	if raw := c.Query("sort_by"); raw != "" {
		query.SortBy = raw
	}
	if !oneOf(query.SortBy, "created_at", "started_at", "priority", "status") {
		errors["sort_by"] = "must be created_at, started_at, priority, or status"
	}
	if raw := c.Query("sort_order"); raw != "" {
		query.SortOrder = strings.ToLower(raw)
	}
	if !oneOf(query.SortOrder, "asc", "desc") {
		errors["sort_order"] = "must be asc or desc"
	}
	if len(errors) > 0 {
		return dto.CollectionRunListQuery{}, errors
	}
	return query, nil
}

func parseCollectionRunWindowListQuery(c *gin.Context) (dto.CollectionRunWindowListQuery, map[string]string) {
	query := dto.CollectionRunWindowListQuery{Page: 1, PageSize: 20}
	errors := validateQueryKeys(c, map[string]struct{}{"p": {}, "page_size": {}, "status": {}})
	parsePageQuery(c, &query.Page, &query.PageSize, errors)
	query.Status = c.Query("status")
	if query.Status != "" && !oneOf(query.Status, "pending", "running", "success", "failed", "unavailable") {
		errors["status"] = "must be pending, running, success, failed, or unavailable"
	}
	if len(errors) > 0 {
		return dto.CollectionRunWindowListQuery{}, errors
	}
	return query, nil
}

func parsePageQuery(c *gin.Context, page, pageSize *int, fieldErrors map[string]string) {
	if values, exists := c.Request.URL.Query()["p"]; exists {
		if len(values) != 1 {
			fieldErrors["p"] = "must be specified once"
		} else if value, err := strconv.Atoi(values[0]); err != nil || value < 1 {
			fieldErrors["p"] = "must be at least 1"
		} else {
			*page = value
		}
	}
	if values, exists := c.Request.URL.Query()["page_size"]; exists {
		if len(values) != 1 {
			fieldErrors["page_size"] = "must be specified once"
		} else if value, err := strconv.Atoi(values[0]); err != nil || value < 1 || value > 100 {
			fieldErrors["page_size"] = "must be between 1 and 100"
		} else {
			*pageSize = value
		}
	}
}

func validateQueryKeys(c *gin.Context, allowed map[string]struct{}) map[string]string {
	return validateQueryKeysAllowRepeated(c, allowed, nil)
}

func validateQueryKeysAllowRepeated(
	c *gin.Context,
	allowed map[string]struct{},
	repeated map[string]struct{},
) map[string]string {
	fieldErrors := map[string]string{}
	for key := range c.Request.URL.Query() {
		if _, exists := allowed[key]; !exists {
			fieldErrors[key] = "is not a supported query parameter"
		}
	}
	for _, key := range []string{"keyword", "sort_by", "sort_order", "task_type", "status"} {
		if _, allowedToRepeat := repeated[key]; allowedToRepeat {
			continue
		}
		if values, exists := c.Request.URL.Query()[key]; exists && len(values) != 1 {
			fieldErrors[key] = "must be specified once"
		}
	}
	return fieldErrors
}

func parseRepeatedEnumQuery(
	c *gin.Context,
	key string,
	fieldErrors map[string]string,
	valid func(string) bool,
) []string {
	values, exists := c.Request.URL.Query()[key]
	if !exists {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		candidates := strings.Split(raw, ",")
		if strings.HasPrefix(strings.TrimSpace(raw), "[") {
			if err := json.Unmarshal([]byte(raw), &candidates); err != nil || len(candidates) == 0 {
				fieldErrors[key] = "must be repeated values, a comma-separated list, or a JSON string array"
				return nil
			}
		}
		for _, candidate := range candidates {
			value := strings.ToLower(strings.TrimSpace(candidate))
			if value == "" || !valid(value) {
				fieldErrors[key] = "contains an unsupported value"
				return nil
			}
			if _, duplicate := seen[value]; duplicate {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
			if len(result) > dto.QueryEnumListMaximumValues {
				fieldErrors[key] = "must contain at most 20 unique values"
				return nil
			}
		}
	}
	return result
}

func parseEnumArray(c *gin.Context, key string, fieldErrors map[string]string, valid func(string) bool) []string {
	values, exists := c.Request.URL.Query()[key]
	if !exists {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || !valid(value) {
			fieldErrors[key] = "contains an unsupported status"
			return nil
		}
		if _, duplicate := seen[value]; duplicate {
			fieldErrors[key] = "must not contain duplicate values"
			return nil
		}
		seen[value] = struct{}{}
	}
	return values
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func writeSiteServiceError(c *gin.Context, err error) {
	var deleteRestricted *service.SiteDeleteRestrictedError
	var configChanged *service.SiteConfigChangedError
	switch {
	case errors.Is(err, service.ErrSiteNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Resource not found", nil)
	case errors.Is(err, service.ErrSiteInvalidBaseURL):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site base URL", map[string]string{"base_url": "must be an allowed absolute HTTP or HTTPS URL"})
	case errors.Is(err, service.ErrSiteConflict):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Site base URL already exists", map[string]string{"base_url": "already exists"})
	case errors.As(err, &deleteRestricted):
		common.AbortErrorWithParams(c, http.StatusConflict, constant.CodeDeleteRestricted, "Site has associated data", map[string]any{
			"dependency_types": deleteRestricted.DependencyTypes,
		}, nil)
	case errors.Is(err, service.ErrSiteDeleteRestricted):
		common.AbortErrorWithParams(c, http.StatusConflict, constant.CodeDeleteRestricted, "Site has associated data", map[string]any{
			"dependency_types": []string{},
		}, nil)
	case errors.As(err, &configChanged):
		common.AbortErrorWithParams(c, http.StatusConflict, constant.CodeSiteConfigChanged, "Site configuration changed", map[string]any{
			"site_id":                 strconv.FormatInt(configChanged.SiteID, 10),
			"expected_config_version": configChanged.ExpectedConfigVersion,
			"actual_config_version":   configChanged.ActualConfigVersion,
		}, nil)
	case errors.Is(err, service.ErrSiteConfigChanged):
		common.AbortError(c, http.StatusConflict, constant.CodeSiteConfigChanged, "Site configuration changed", nil)
	case errors.Is(err, service.ErrBaseURLPreflightRequired):
		common.AbortError(c, http.StatusConflict, constant.CodeBaseURLPreflightRequired, "A current base URL preflight is required", nil)
	case errors.Is(err, service.ErrSiteTaskOverlap):
		common.AbortError(c, http.StatusConflict, constant.CodeTaskOverlap, "A collection task overlaps this range", nil)
	case errors.Is(err, service.ErrSiteInvalidBackfillRange):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site backfill range", map[string]string{"range": "must be within the site statistics range and configured maximum duration"})
	case errors.Is(err, service.ErrSiteInvalidStatisticsEnd):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics end", map[string]string{"statistics_end_at": "must be between the next incomplete hour and disabled_at"})
	case errors.Is(err, service.ErrSiteResourceRange):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site resource range", map[string]string{"range": "must be within the configured retention and maximum duration"})
	case errors.Is(err, service.ErrSiteInvalidState):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Site state does not allow this operation", nil)
	case errors.Is(err, service.ErrSiteExportDisabled):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeSiteExportDisabled, "Site data export is disabled", nil)
	case errors.Is(err, service.ErrUpstreamLoginRejected):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeUpstreamLoginRejected, "Upstream login was rejected", nil)
	case errors.Is(err, service.ErrSiteCapabilitiesPending), errors.Is(err, service.ErrSiteIncompatible),
		errors.Is(err, service.ErrUpstreamAuthExpired), errors.Is(err, service.ErrUpstreamPermissionDenied),
		errors.Is(err, service.ErrUpstreamCredentialOriginMismatch), errors.Is(err, service.ErrUpstreamResponseInvalid),
		errors.Is(err, service.ErrUpstreamEnvelopeInvalid), errors.Is(err, service.ErrUpstreamResponseTooLarge),
		errors.Is(err, service.ErrUpstreamDataMismatch):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeSiteIncompatible, "Site identity or API contract is incompatible", nil)
	case errors.Is(err, service.ErrUpstreamExportDisabled):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeSiteExportDisabled, "Site data export is disabled", nil)
	case errors.Is(err, service.ErrUpstreamAddressForbidden):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeUpstreamAddressForbidden, "Upstream address is not allowed", nil)
	case errors.Is(err, service.ErrUpstreamTokenRotationResultUnknown):
		common.AbortError(c, http.StatusBadGateway, constant.CodeTokenRotationResultUnknown, "Token rotation result is unknown", nil)
	case errors.Is(err, service.ErrUpstreamUnavailable), errors.Is(err, service.ErrUpstreamRateLimited):
		common.AbortError(c, http.StatusServiceUnavailable, constant.CodeUpstreamUnavailable, "Upstream service is unavailable", nil)
	case errors.Is(err, service.ErrUpstreamRemote):
		common.AbortError(c, http.StatusBadGateway, constant.CodeUpstreamError, "Upstream service returned an error", nil)
	default:
		common.AbortInternalError(c)
	}
}
