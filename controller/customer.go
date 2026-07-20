package controller

import (
	"context"
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

type customerApplication interface {
	List(context.Context, dto.CustomerListQuery) (common.PageData[dto.CustomerListItem], error)
	Create(context.Context, dto.CustomerCreateRequest) (dto.CustomerDetail, error)
	Get(context.Context, int64) (dto.CustomerDetail, error)
	Update(context.Context, int64, dto.CustomerUpdateRequest) (dto.CustomerDetail, error)
	Delete(context.Context, int64) error
	Disable(context.Context, int64) (dto.CustomerDetail, error)
	Enable(context.Context, int64, string) (dto.CollectionRunItem, error)
	Statistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error)
}

type accountListApplication interface {
	List(context.Context, dto.AccountListQuery) (common.PageData[dto.AccountListItem], error)
}

type CustomerController struct {
	customers customerApplication
	accounts  accountListApplication
}

func NewCustomerController(customers customerApplication, accounts accountListApplication) *CustomerController {
	return &CustomerController{customers: customers, accounts: accounts}
}

func (controller *CustomerController) List(c *gin.Context) {
	query, fieldErrors := parseCustomerListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid customer list query", fieldErrors)
		return
	}
	page, err := controller.customers.List(c.Request.Context(), query)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *CustomerController) Create(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	var request dto.CustomerCreateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid customer", fieldErrors)
		return
	}
	detail, err := controller.customers.Create(c.Request.Context(), request)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *CustomerController) Get(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok || !requireNoQuery(c) {
		return
	}
	detail, err := controller.customers.Get(c.Request.Context(), id)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *CustomerController) Update(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok || !requireNoQuery(c) {
		return
	}
	var request dto.CustomerUpdateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid customer", fieldErrors)
		return
	}
	detail, err := controller.customers.Update(c.Request.Context(), id, request)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *CustomerController) Delete(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	if err := controller.customers.Delete(c.Request.Context(), id); err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func (controller *CustomerController) Disable(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	detail, err := controller.customers.Disable(c.Request.Context(), id)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *CustomerController) Enable(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	run, err := controller.customers.Enable(c.Request.Context(), id, common.RequestID(c))
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, run)
}

func (controller *CustomerController) ListAccounts(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok {
		return
	}
	query, fieldErrors := parseAccountListQuery(c, true)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid account list query", fieldErrors)
		return
	}
	query.CustomerID = strconv.FormatInt(id, 10)
	if _, err := controller.customers.Get(c.Request.Context(), id); err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	page, err := controller.accounts.List(c.Request.Context(), query)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *CustomerController) Statistics(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "customer")
	if !ok {
		return
	}
	query, fieldErrors := parseEntityStatisticsQuery(c, dto.StatisticsScopeCustomer, true)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query", fieldErrors)
		return
	}
	result, err := controller.customers.Statistics(c.Request.Context(), id, query)
	if err != nil {
		writeCustomerServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func parseCustomerListQuery(c *gin.Context) (dto.CustomerListQuery, map[string]string) {
	query := dto.CustomerListQuery{Page: 1, PageSize: 20, SortBy: "updated_at", SortOrder: "desc"}
	fieldErrors := validateQueryKeys(c, map[string]struct{}{
		"p": {}, "page_size": {}, "keyword": {}, "status": {}, "sort_by": {}, "sort_order": {},
	})
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.Keyword, _ = singletonQueryValue(c, "keyword", fieldErrors)
	query.Status, _ = singletonQueryValue(c, "status", fieldErrors)
	if value, exists := singletonQueryValue(c, "sort_by", fieldErrors); exists && value != "" {
		query.SortBy = value
	}
	if value, exists := singletonQueryValue(c, "sort_order", fieldErrors); exists && value != "" {
		query.SortOrder = strings.ToLower(value)
	}
	query.Normalize()
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.CustomerListQuery{}, fieldErrors
	}
	return query, nil
}

func parseEntityStatisticsQuery(c *gin.Context, scope string, allowSiteIDs bool) (dto.StatisticsQuery, map[string]string) {
	allowed := map[string]struct{}{
		"start_timestamp": {}, "end_timestamp": {}, "granularity": {}, "p": {}, "page_size": {},
		"sort_by": {}, "sort_order": {},
	}
	if allowSiteIDs {
		allowed["site_ids"] = struct{}{}
	}
	fieldErrors := validateQueryKeys(c, allowed)
	query := dto.StatisticsQuery{Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "desc"}
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.StartTimestamp = parseRequiredTimestampQuery(c, "start_timestamp", fieldErrors)
	query.EndTimestamp = parseRequiredTimestampQuery(c, "end_timestamp", fieldErrors)
	query.Granularity, _ = singletonQueryValue(c, "granularity", fieldErrors)
	if allowSiteIDs {
		query.SiteIDs = parseCommaSeparatedIDs(c, "site_ids", fieldErrors)
	}
	if value, exists := singletonQueryValue(c, "sort_by", fieldErrors); exists && value != "" {
		query.SortBy = value
	}
	if value, exists := singletonQueryValue(c, "sort_order", fieldErrors); exists && value != "" {
		query.SortOrder = strings.ToLower(value)
	}
	query.Normalize()
	mergeFieldErrors(fieldErrors, query.Validate(scope))
	if len(fieldErrors) > 0 {
		return dto.StatisticsQuery{}, fieldErrors
	}
	return query, nil
}

func parseRequiredTimestampQuery(c *gin.Context, key string, fieldErrors map[string]string) int64 {
	raw, exists := singletonQueryValue(c, key, fieldErrors)
	if !exists || raw == "" {
		fieldErrors[key] = "is required"
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
		fieldErrors[key] = "must be a canonical positive Unix timestamp"
		return 0
	}
	return value
}

func parseCommaSeparatedIDs(c *gin.Context, key string, fieldErrors map[string]string) []int64 {
	raw, exists := singletonQueryValue(c, key, fieldErrors)
	if !exists {
		return nil
	}
	parts := strings.Split(raw, ",")
	if raw == "" {
		fieldErrors[key] = "must contain between 1 and 100 IDs"
		return nil
	}
	result := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != part {
			fieldErrors[key] = "must contain canonical positive decimal IDs"
			return nil
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) > 100 {
			fieldErrors[key] = "must contain at most 100 unique IDs"
			return nil
		}
	}
	return result
}

func singletonQueryValue(c *gin.Context, key string, fieldErrors map[string]string) (string, bool) {
	values, exists := c.Request.URL.Query()[key]
	if !exists {
		return "", false
	}
	if len(values) != 1 {
		fieldErrors[key] = "must be specified once"
		return "", true
	}
	return values[0], true
}

func mergeFieldErrors(destination, source map[string]string) {
	for key, value := range source {
		if _, exists := destination[key]; !exists {
			destination[key] = value
		}
	}
}

func requireNoQuery(c *gin.Context) bool {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{})
	if len(fieldErrors) == 0 {
		return true
	}
	common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Query parameters are not allowed", fieldErrors)
	return false
}

func writeCustomerServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCustomerNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Customer not found", nil)
	case errors.Is(err, service.ErrCustomerInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid customer", nil)
	case errors.Is(err, service.ErrStatisticsInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query",
			map[string]string{"statistics": "query is invalid"})
	case errors.Is(err, service.ErrCustomerDeleteRestricted):
		common.AbortError(c, http.StatusConflict, constant.CodeDeleteRestricted, "Customer has associated data", nil)
	case errors.Is(err, service.ErrCustomerInvalidState):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Customer state does not allow this operation", nil)
	case errors.Is(err, service.ErrEntityBackfillRunning):
		common.AbortError(c, http.StatusConflict, constant.CodeBackfillRunning, "A different backfill range is already active", nil)
	default:
		common.AbortInternalError(c)
	}
}
