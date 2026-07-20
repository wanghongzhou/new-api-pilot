package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type userInventoryApplication interface {
	List(context.Context, dto.UserInventoryQuery) (dto.UserInventoryPage, error)
	Statistics(context.Context, dto.UserInventoryStatisticsQuery) (dto.UserInventoryStatisticsResponse, error)
}

type UserInventoryController struct{ inventory userInventoryApplication }

func NewUserInventoryController(inventory userInventoryApplication) *UserInventoryController {
	return &UserInventoryController{inventory: inventory}
}
func (controller *UserInventoryController) Global(c *gin.Context) { controller.list(c, nil) }
func (controller *UserInventoryController) Site(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "site")
	if ok {
		controller.list(c, []int64{id})
	}
}
func (controller *UserInventoryController) GlobalStatistics(c *gin.Context) {
	controller.statistics(c, nil)
}
func (controller *UserInventoryController) SiteStatistics(c *gin.Context) {
	id, ok := parsePositivePathID(c, "id", "site")
	if ok {
		controller.statistics(c, []int64{id})
	}
}

func (controller *UserInventoryController) list(c *gin.Context, forced []int64) {
	if controller == nil || controller.inventory == nil {
		common.AbortInternalError(c)
		return
	}
	query, fields := parseUserInventoryQuery(c)
	if len(forced) > 0 {
		query.SiteIDs = forced
		fields = query.Validate()
	}
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid user inventory query", fields)
		return
	}
	response, err := controller.inventory.List(c.Request.Context(), query)
	if err != nil {
		writeUserInventoryError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func (controller *UserInventoryController) statistics(c *gin.Context, forced []int64) {
	if controller == nil || controller.inventory == nil {
		common.AbortInternalError(c)
		return
	}
	query, fields := parseUserInventoryStatisticsQuery(c)
	if len(forced) > 0 {
		query.SiteIDs = forced
		fields = query.Validate()
	}
	if fields != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid user inventory statistics query", fields)
		return
	}
	response, err := controller.inventory.Statistics(c.Request.Context(), query)
	if err != nil {
		writeUserInventoryError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func parseUserInventoryQuery(c *gin.Context) (dto.UserInventoryQuery, map[string]string) {
	query := dto.UserInventoryQuery{Page: 1, PageSize: 20, Keyword: c.Query("keyword"), Groups: inventoryQueryValues(c, "groups"), States: inventoryQueryValues(c, "states")}
	if raw := c.Query("p"); raw != "" {
		query.Page, _ = strconv.Atoi(raw)
	}
	if raw := c.Query("page_size"); raw != "" {
		query.PageSize, _ = strconv.Atoi(raw)
	}
	var invalid bool
	query.SiteIDs, invalid = parseLogIDList(c.QueryArray("site_ids"))
	invalid = invalid || containsNonPositiveInventoryID(query.SiteIDs)
	if invalid {
		return query, map[string]string{"site_ids": "must contain positive integer IDs"}
	}
	if raw := c.Query("remote_user_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
			return query, map[string]string{"remote_user_id": "must be a canonical positive bigint string"}
		}
		query.RemoteUserID = &value
	}
	query.Roles, invalid = parseInventoryInts(inventoryQueryValues(c, "roles"))
	if invalid {
		return query, map[string]string{"roles": "must contain integers"}
	}
	query.Statuses, invalid = parseInventoryInts(inventoryQueryValues(c, "statuses"))
	if invalid {
		return query, map[string]string{"statuses": "must contain integers"}
	}
	if raw := c.Query("min_balance"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || strconv.FormatInt(value, 10) != raw {
			return query, map[string]string{"min_balance": "must be a canonical int64"}
		}
		query.MinBalance = &value
	}
	if raw := c.Query("max_balance"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || strconv.FormatInt(value, 10) != raw {
			return query, map[string]string{"max_balance": "must be a canonical int64"}
		}
		query.MaxBalance = &value
	}
	query.Normalize()
	return query, query.Validate()
}

func parseUserInventoryStatisticsQuery(c *gin.Context) (dto.UserInventoryStatisticsQuery, map[string]string) {
	query := dto.UserInventoryStatisticsQuery{Groups: inventoryQueryValues(c, "groups")}
	query.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	query.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	var invalid bool
	query.SiteIDs, invalid = parseLogIDList(c.QueryArray("site_ids"))
	invalid = invalid || containsNonPositiveInventoryID(query.SiteIDs)
	if invalid {
		return query, map[string]string{"site_ids": "must contain positive integer IDs"}
	}
	query.Roles, invalid = parseInventoryInts(inventoryQueryValues(c, "roles"))
	if invalid {
		return query, map[string]string{"roles": "must contain integers"}
	}
	query.Statuses, invalid = parseInventoryInts(inventoryQueryValues(c, "statuses"))
	if invalid {
		return query, map[string]string{"statuses": "must contain integers"}
	}
	query.Normalize()
	return query, query.Validate()
}

func parseInventoryInts(values []string) ([]int, bool) {
	result := make([]int, 0, len(values))
	invalid := false
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			value, err := strconv.Atoi(strings.TrimSpace(part))
			if err == nil {
				result = append(result, value)
			} else {
				invalid = true
			}
		}
	}
	return result, invalid
}

func inventoryQueryValues(c *gin.Context, key string) []string {
	result := make([]string, 0)
	for _, raw := range c.QueryArray(key) {
		for _, part := range strings.Split(raw, ",") {
			if value := strings.TrimSpace(part); value != "" {
				result = append(result, value)
			}
		}
	}
	return result
}

func containsNonPositiveInventoryID(values []int64) bool {
	for _, value := range values {
		if value <= 0 {
			return true
		}
	}
	return false
}

func writeUserInventoryError(c *gin.Context, err error) {
	if err == service.ErrStatisticsInvalid {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid user inventory query", nil)
		return
	}
	common.AbortInternalError(c)
}
