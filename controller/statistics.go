package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type statisticsApplication interface {
	Global(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Sites(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	SiteStatistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Customers(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Accounts(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Models(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Channels(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Groups(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Tokens(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Nodes(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	ModelOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.ModelOption], error)
	ChannelOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.ChannelOption], error)
	GroupOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.GroupOption], error)
	TokenOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.TokenOption], error)
	NodeOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.NodeOption], error)
}

type StatisticsController struct {
	statistics statisticsApplication
}

func NewStatisticsController(statistics statisticsApplication) *StatisticsController {
	return &StatisticsController{statistics: statistics}
}

func (controller *StatisticsController) Global(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeGlobal)
}

func (controller *StatisticsController) Sites(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeSite)
}

func (controller *StatisticsController) Customers(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeCustomer)
}

func (controller *StatisticsController) Accounts(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeAccount)
}

func (controller *StatisticsController) Models(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeModel)
}

func (controller *StatisticsController) Channels(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeChannel)
}
func (controller *StatisticsController) Groups(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeGroup)
}
func (controller *StatisticsController) Tokens(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeToken)
}
func (controller *StatisticsController) Nodes(c *gin.Context) {
	controller.query(c, dto.StatisticsScopeNode)
}

func (controller *StatisticsController) Site(c *gin.Context) {
	if controller == nil || controller.statistics == nil {
		common.AbortInternalError(c)
		return
	}
	id, ok := parsePositivePathID(c, "id", "site")
	if !ok {
		return
	}
	query, fieldErrors := parseEntityStatisticsQuery(c, dto.StatisticsScopeSite, false)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query", fieldErrors)
		return
	}
	response, err := controller.statistics.SiteStatistics(c.Request.Context(), id, query)
	if err != nil {
		writeStatisticsError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func (controller *StatisticsController) ModelOptions(c *gin.Context) {
	if controller == nil || controller.statistics == nil {
		common.AbortInternalError(c)
		return
	}
	query, fieldErrors := parseStatisticsOptionQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics options query", fieldErrors)
		return
	}
	page, err := controller.statistics.ModelOptions(c.Request.Context(), query)
	if err != nil {
		writeStatisticsError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *StatisticsController) ChannelOptions(c *gin.Context) {
	if controller == nil || controller.statistics == nil {
		common.AbortInternalError(c)
		return
	}
	query, fieldErrors := parseStatisticsOptionQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics options query", fieldErrors)
		return
	}
	page, err := controller.statistics.ChannelOptions(c.Request.Context(), query)
	if err != nil {
		writeStatisticsError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, page)
}

func (controller *StatisticsController) GroupOptions(c *gin.Context) { controller.options(c, "group") }
func (controller *StatisticsController) TokenOptions(c *gin.Context) { controller.options(c, "token") }
func (controller *StatisticsController) NodeOptions(c *gin.Context)  { controller.options(c, "node") }

func (controller *StatisticsController) options(c *gin.Context, kind string) {
	if controller == nil || controller.statistics == nil {
		common.AbortInternalError(c)
		return
	}
	query, fields := parseStatisticsOptionQuery(c)
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics options query", fields)
		return
	}
	var value any
	var err error
	switch kind {
	case "group":
		value, err = controller.statistics.GroupOptions(c.Request.Context(), query)
	case "token":
		value, err = controller.statistics.TokenOptions(c.Request.Context(), query)
	case "node":
		value, err = controller.statistics.NodeOptions(c.Request.Context(), query)
	default:
		common.AbortInternalError(c)
		return
	}
	if err != nil {
		writeStatisticsError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, value)
}

func (controller *StatisticsController) query(
	c *gin.Context,
	scope string,
) {
	if controller == nil || controller.statistics == nil {
		common.AbortInternalError(c)
		return
	}
	var read func(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	switch scope {
	case dto.StatisticsScopeGlobal:
		read = controller.statistics.Global
	case dto.StatisticsScopeSite:
		read = controller.statistics.Sites
	case dto.StatisticsScopeCustomer:
		read = controller.statistics.Customers
	case dto.StatisticsScopeAccount:
		read = controller.statistics.Accounts
	case dto.StatisticsScopeModel:
		read = controller.statistics.Models
	case dto.StatisticsScopeChannel:
		read = controller.statistics.Channels
	case dto.StatisticsScopeGroup:
		read = controller.statistics.Groups
	case dto.StatisticsScopeToken:
		read = controller.statistics.Tokens
	case dto.StatisticsScopeNode:
		read = controller.statistics.Nodes
	default:
		common.AbortInternalError(c)
		return
	}
	query, fieldErrors := parseStatisticsQuery(c, scope)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query", fieldErrors)
		return
	}
	response, err := read(c.Request.Context(), query)
	if err != nil {
		writeStatisticsError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func parseStatisticsQuery(c *gin.Context, scope string) (dto.StatisticsQuery, map[string]string) {
	allowed := map[string]struct{}{
		"start_timestamp": {}, "end_timestamp": {}, "granularity": {},
		"site_ids": {}, "customer_ids": {}, "account_ids": {}, "model_names": {}, "channel_keys": {},
		"use_groups": {}, "token_keys": {}, "node_names": {},
		"p": {}, "page_size": {}, "sort_by": {}, "sort_order": {},
	}
	fieldErrors := validateQueryKeys(c, allowed)
	query := dto.StatisticsQuery{Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "desc"}
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.StartTimestamp = parseRequiredTimestampQuery(c, "start_timestamp", fieldErrors)
	query.EndTimestamp = parseRequiredTimestampQuery(c, "end_timestamp", fieldErrors)
	query.Granularity, _ = singletonQueryValue(c, "granularity", fieldErrors)
	query.SiteIDs = parseCommaSeparatedIDs(c, "site_ids", fieldErrors)
	query.CustomerIDs = parseCommaSeparatedIDs(c, "customer_ids", fieldErrors)
	query.AccountIDs = parseCommaSeparatedIDs(c, "account_ids", fieldErrors)
	query.ModelNames = repeatedStatisticsValues(c, "model_names", fieldErrors)
	query.ChannelKeys = repeatedStatisticsValues(c, "channel_keys", fieldErrors)
	query.UseGroups = repeatedStatisticsValuesAllowEmpty(c, "use_groups", fieldErrors)
	query.TokenKeys = repeatedStatisticsValues(c, "token_keys", fieldErrors)
	query.NodeNames = repeatedStatisticsValuesAllowEmpty(c, "node_names", fieldErrors)
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

func repeatedStatisticsValuesAllowEmpty(c *gin.Context, key string, fieldErrors map[string]string) []string {
	values, exists := c.Request.URL.Query()[key]
	if !exists {
		return nil
	}
	if len(values) > 100 {
		fieldErrors[key] = "must contain at most 100 values"
		return nil
	}
	return append([]string(nil), values...)
}

func repeatedStatisticsValues(c *gin.Context, key string, fieldErrors map[string]string) []string {
	values, exists := c.Request.URL.Query()[key]
	if !exists {
		return nil
	}
	if len(values) > 100 {
		fieldErrors[key] = "must contain at most 100 values"
		return nil
	}
	for _, value := range values {
		if value == "" {
			fieldErrors[key] = "must not contain empty values"
			return nil
		}
	}
	return append([]string(nil), values...)
}

func parseStatisticsOptionQuery(c *gin.Context) (dto.StatisticsOptionQuery, map[string]string) {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{
		"keyword": {}, "site_ids": {}, "p": {}, "page_size": {},
	})
	query := dto.StatisticsOptionQuery{Page: 1, PageSize: 20}
	parsePageQuery(c, &query.Page, &query.PageSize, fieldErrors)
	query.Keyword, _ = singletonQueryValue(c, "keyword", fieldErrors)
	query.SiteIDs = parseCommaSeparatedIDs(c, "site_ids", fieldErrors)
	query.Normalize()
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.StatisticsOptionQuery{}, fieldErrors
	}
	return query, nil
}

func writeStatisticsError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrStatisticsInvalid) {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid statistics query",
			map[string]string{"statistics": "query is invalid"})
		return
	}
	if errors.Is(err, service.ErrStatisticsNotFound) {
		common.AbortError(c, http.StatusNotFound, constant.CodeNotFound, "Statistics scope not found", nil)
		return
	}
	common.AbortInternalError(c)
}
