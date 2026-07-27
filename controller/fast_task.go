package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type FastTaskController struct {
	Store *common.RedisStore
}

type fastTaskListQuery struct {
	SiteID   int64
	TaskType string
	Status   string
	Offset   int
	Limit    int
}

func NewFastTaskController(store *common.RedisStore) *FastTaskController {
	return &FastTaskController{Store: store}
}

func (controller *FastTaskController) List(c *gin.Context) {
	if controller == nil || controller.Store == nil {
		common.AbortError(c, http.StatusServiceUnavailable, constant.CodeInternalError, "Fast task history unavailable", nil)
		return
	}
	query, fieldErrors := parseFastTaskListQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid fast task history query", fieldErrors)
		return
	}
	rows, total, hasMore, err := controller.Store.ListFiltered(
		c.Request.Context(), query.SiteID, query.TaskType, query.Status, query.Offset, query.Limit,
	)
	if err != nil {
		common.AbortInternalError(c)
		return
	}
	out := make([]dto.FastTaskHistoryItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.FastTaskHistoryItem{
			SiteID:     strconv.FormatInt(row.SiteID, 10),
			TaskType:   row.TaskType,
			StartedAt:  row.StartedAt,
			FinishedAt: row.FinishedAt,
			Status:     row.Status,
			DurationMS: row.DurationMS,
			Error:      row.Error,
			RequestID:  row.RequestID,
		})
	}
	common.WriteSuccess(c, http.StatusOK, gin.H{
		"items": out, "offset": query.Offset, "limit": query.Limit, "total": total, "has_more": hasMore,
	})
}

func parseFastTaskListQuery(c *gin.Context) (fastTaskListQuery, map[string]string) {
	query := fastTaskListQuery{Offset: 0, Limit: 50}
	errors := validateQueryKeys(c, map[string]struct{}{
		"site_id": {}, "task_type": {}, "status": {}, "offset": {}, "limit": {},
	})
	values := c.Request.URL.Query()

	if raw, ok := singleQueryValue(values, "site_id", errors); ok {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
			errors["site_id"] = "must be a positive decimal ID"
		} else {
			query.SiteID = value
		}
	}
	if raw, ok := singleQueryValue(values, "task_type", errors); ok {
		if !oneOf(raw, constant.TaskTypeSiteProbe, constant.TaskTypeRealtimeStat, constant.TaskTypeResourceSnapshot) {
			errors["task_type"] = "must be site_probe, realtime_stat, or resource_snapshot"
		} else {
			query.TaskType = raw
		}
	}
	if raw, ok := optionalSingleQueryValue(values, "status", errors); ok {
		if raw != "" && !oneOf(raw, "success", "failed") {
			errors["status"] = "must be success or failed"
		} else {
			query.Status = raw
		}
	}
	parseNonNegativeIntQuery(values, "offset", &query.Offset, errors)
	parseBoundedPositiveIntQuery(values, "limit", 100, &query.Limit, errors)
	if len(errors) > 0 {
		return fastTaskListQuery{}, errors
	}
	return query, nil
}

func singleQueryValue(values map[string][]string, key string, errors map[string]string) (string, bool) {
	items, exists := values[key]
	if !exists || len(items) != 1 || items[0] == "" {
		errors[key] = "is required and must be specified once"
		return "", false
	}
	return items[0], true
}

func optionalSingleQueryValue(values map[string][]string, key string, errors map[string]string) (string, bool) {
	items, exists := values[key]
	if !exists {
		return "", false
	}
	if len(items) != 1 {
		errors[key] = "must be specified once"
		return "", false
	}
	return items[0], true
}

func parseNonNegativeIntQuery(values map[string][]string, key string, target *int, errors map[string]string) {
	items, exists := values[key]
	if !exists {
		return
	}
	if len(items) != 1 {
		errors[key] = "must be specified once"
		return
	}
	value, err := strconv.Atoi(items[0])
	if err != nil || value < 0 {
		errors[key] = "must be a non-negative integer"
		return
	}
	*target = value
}

func parseBoundedPositiveIntQuery(values map[string][]string, key string, maximum int, target *int, errors map[string]string) {
	items, exists := values[key]
	if !exists {
		return
	}
	if len(items) != 1 {
		errors[key] = "must be specified once"
		return
	}
	value, err := strconv.Atoi(items[0])
	if err != nil || value < 1 || value > maximum {
		errors[key] = "must be between 1 and " + strconv.Itoa(maximum)
		return
	}
	*target = value
}
