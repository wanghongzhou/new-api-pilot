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

type accountApplication interface {
	List(context.Context, dto.AccountListQuery) (common.PageData[dto.AccountListItem], error)
	SearchRemoteUsers(context.Context, int64, dto.RemoteUserListQuery, string) (common.PageData[dto.RemoteUserItem], error)
	Create(context.Context, dto.AccountCreateRequest, string) (dto.AccountDetail, error)
	Get(context.Context, int64) (dto.AccountDetail, error)
	Update(context.Context, int64, dto.AccountUpdateRequest) (dto.AccountDetail, error)
	Delete(context.Context, int64) error
	Archive(context.Context, int64) (dto.AccountDetail, error)
	Restore(context.Context, int64, string) (dto.CollectionRunItem, error)
	Refresh(context.Context, int64, string) (dto.AccountDetail, error)
	Statistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error)
}

type AccountController struct {
	accounts accountApplication
}

func NewAccountController(accounts accountApplication) *AccountController {
	return &AccountController{accounts: accounts}
}

func (controller *AccountController) List(c *gin.Context) {
	query, fieldErrors := parseAccountListQuery(c, false)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid account list query", fieldErrors)
		return
	}
	page, err := controller.accounts.List(c.Request.Context(), query)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *AccountController) SearchRemoteUsers(c *gin.Context) {
	siteID, ok := parsePositivePathID(c, "siteId", "site")
	if !ok {
		return
	}
	query, fieldErrors := parseRemoteUserListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid remote user query", fieldErrors)
		return
	}
	page, err := controller.accounts.SearchRemoteUsers(c.Request.Context(), siteID, query, common.RequestID(c))
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *AccountController) Create(c *gin.Context) {
	if !requireNoQuery(c) {
		return
	}
	var request dto.AccountCreateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid account", fieldErrors)
		return
	}
	detail, err := controller.accounts.Create(c.Request.Context(), request, common.RequestID(c))
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *AccountController) Get(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) {
		return
	}
	detail, err := controller.accounts.Get(c.Request.Context(), id)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *AccountController) Update(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) {
		return
	}
	var request dto.AccountUpdateRequest
	if !decodeJSON(c, &request, common.DefaultJSONBodyLimit) {
		return
	}
	request.Normalize()
	if fieldErrors := request.Validate(); fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid account", fieldErrors)
		return
	}
	detail, err := controller.accounts.Update(c.Request.Context(), id, request)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *AccountController) Delete(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	if err := controller.accounts.Delete(c.Request.Context(), id); err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, nil)
}

func (controller *AccountController) Archive(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	detail, err := controller.accounts.Archive(c.Request.Context(), id)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *AccountController) Restore(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	run, err := controller.accounts.Restore(c.Request.Context(), id, common.RequestID(c))
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, run)
}

func (controller *AccountController) Refresh(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	detail, err := controller.accounts.Refresh(c.Request.Context(), id, common.RequestID(c))
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, detail)
}

func (controller *AccountController) Statistics(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "account")
	if !ok {
		return
	}
	query, fieldErrors := parseEntityStatisticsQuery(c, dto.StatisticsScopeAccount, false)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query", fieldErrors)
		return
	}
	result, err := controller.accounts.Statistics(c.Request.Context(), id, query)
	if err != nil {
		writeAccountServiceError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func parseAccountListQuery(c *gin.Context, customerScoped bool) (dto.AccountListQuery, map[string]string) {
	allowed := map[string]struct{}{
		"p": {}, "page_size": {}, "keyword": {}, "remote_status": {}, "remote_state": {},
		"managed_status": {}, "sort_by": {}, "sort_order": {},
	}
	if !customerScoped {
		allowed["site_id"] = struct{}{}
		allowed["customer_id"] = struct{}{}
	}
	fieldErrors := validateQueryKeys(c, allowed)
	query := dto.AccountListQuery{Page: 1, PageSize: 20, SortBy: "updated_at", SortOrder: "desc"}
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.Keyword, _ = singletonQueryValue(c, "keyword", fieldErrors)
	if !customerScoped {
		query.SiteID, _ = singletonQueryValue(c, "site_id", fieldErrors)
		query.CustomerID, _ = singletonQueryValue(c, "customer_id", fieldErrors)
	}
	if raw, exists := singletonQueryValue(c, "remote_status", fieldErrors); exists {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || strconv.FormatInt(value, 10) != raw {
			fieldErrors["remote_status"] = "must be a canonical 32-bit integer"
		} else {
			status := int(value)
			query.RemoteStatus = &status
		}
	}
	query.RemoteState, _ = singletonQueryValue(c, "remote_state", fieldErrors)
	query.ManagedStatus, _ = singletonQueryValue(c, "managed_status", fieldErrors)
	if value, exists := singletonQueryValue(c, "sort_by", fieldErrors); exists && value != "" {
		query.SortBy = value
	}
	if value, exists := singletonQueryValue(c, "sort_order", fieldErrors); exists && value != "" {
		query.SortOrder = strings.ToLower(value)
	}
	query.Normalize()
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.AccountListQuery{}, fieldErrors
	}
	return query, nil
}

func parseRemoteUserListQuery(c *gin.Context) (dto.RemoteUserListQuery, map[string]string) {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{"p": {}, "page_size": {}, "keyword": {}})
	query := dto.RemoteUserListQuery{Page: 1, PageSize: 20}
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.Keyword, _ = singletonQueryValue(c, "keyword", fieldErrors)
	query.Normalize()
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.RemoteUserListQuery{}, fieldErrors
	}
	return query, nil
}

func writeAccountServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAccountNotFound), errors.Is(err, service.ErrCustomerNotFound):
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Resource not found", nil)
	case errors.Is(err, service.ErrAccountInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid account", nil)
	case errors.Is(err, service.ErrStatisticsInvalid):
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query",
			map[string]string{"statistics": "query is invalid"})
	case errors.Is(err, service.ErrAccountAlreadyManaged):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Remote user is already managed", nil)
	case errors.Is(err, service.ErrAccountRemoteIdentityConflict):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Remote user identity changed", nil)
	case errors.Is(err, service.ErrAccountRemoteUserNotFound):
		common.AbortError(c, http.StatusUnprocessableEntity, constant.CodeUpstreamUserNotFound, "Upstream user was not found", nil)
	case errors.Is(err, service.ErrAccountDeleteRestricted):
		common.AbortError(c, http.StatusConflict, constant.CodeDeleteRestricted, "Account has associated data", nil)
	case errors.Is(err, service.ErrAccountInvalidState), errors.Is(err, service.ErrCustomerInvalidState):
		common.AbortError(c, http.StatusConflict, constant.CodeConflict, "Account state does not allow this operation", nil)
	case errors.Is(err, service.ErrEntityBackfillRunning):
		common.AbortError(c, http.StatusConflict, constant.CodeBackfillRunning, "A different backfill range is already active", nil)
	case errors.Is(err, service.ErrSiteNotFound), errors.Is(err, service.ErrSiteConfigChanged),
		errors.Is(err, service.ErrSiteInvalidState), errors.Is(err, service.ErrSiteExportDisabled),
		errors.Is(err, service.ErrSiteCapabilitiesPending),
		errors.Is(err, service.ErrSiteIncompatible), errors.Is(err, service.ErrUpstreamAuthExpired),
		errors.Is(err, service.ErrUpstreamPermissionDenied), errors.Is(err, service.ErrUpstreamCredentialOriginMismatch),
		errors.Is(err, service.ErrUpstreamResponseInvalid), errors.Is(err, service.ErrUpstreamEnvelopeInvalid),
		errors.Is(err, service.ErrUpstreamResponseTooLarge),
		errors.Is(err, service.ErrUpstreamExportDisabled), errors.Is(err, service.ErrUpstreamAddressForbidden),
		errors.Is(err, service.ErrUpstreamUnavailable), errors.Is(err, service.ErrUpstreamRateLimited),
		errors.Is(err, service.ErrUpstreamRemote):
		writeSiteServiceError(c, err)
	default:
		common.AbortInternalError(c)
	}
}
