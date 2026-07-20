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

type logApplication interface {
	Query(context.Context, dto.LogQuery) (dto.LogResponse, error)
}

type LogController struct{ logs logApplication }

func NewLogController(logs logApplication) *LogController { return &LogController{logs: logs} }

func (controller *LogController) Global(c *gin.Context) { controller.query(c, nil) }

func (controller *LogController) Site(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid site id", map[string]string{"id": "must be positive"})
		return
	}
	controller.query(c, []int64{id})
}

func (controller *LogController) query(c *gin.Context, forcedSiteIDs []int64) {
	if controller == nil || controller.logs == nil {
		common.AbortInternalError(c)
		return
	}
	query, fieldErrors := parseLogQuery(c)
	if len(forcedSiteIDs) > 0 {
		query.SiteIDs = forcedSiteIDs
	}
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid log query", fieldErrors)
		return
	}
	response, err := controller.logs.Query(c.Request.Context(), query)
	if err != nil {
		if err == service.ErrStatisticsInvalid {
			common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid log query", nil)
			return
		}
		common.AbortInternalError(c)
		return
	}
	common.WriteSuccess(c, http.StatusOK, response)
}

func parseLogQuery(c *gin.Context) (dto.LogQuery, map[string]string) {
	query := dto.LogQuery{Page: 1, PageSize: 20}
	if raw := c.Query("p"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return query, map[string]string{"p": "must be an integer"}
		}
		query.Page = value
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return query, map[string]string{"page_size": "must be an integer"}
		}
		query.PageSize = value
	}
	query.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	query.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if raw := c.Query("type"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return query, map[string]string{"type": "must be an integer"}
		}
		query.Type = &value
	}
	if raw := c.Query("channel_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return query, map[string]string{"channel_id": "must be an integer"}
		}
		query.ChannelID = &value
	}
	var siteError bool
	query.SiteIDs, siteError = parseLogIDList(c.QueryArray("site_ids"))
	query.Username, query.ModelName, query.TokenName, query.UseGroup = c.Query("username"), c.Query("model_name"), c.Query("token_name"), c.Query("group")
	query.RequestID, query.UpstreamRequestID = c.Query("request_id"), c.Query("upstream_request_id")
	query.Normalize()
	fieldErrors := query.Validate()
	if siteError {
		if fieldErrors == nil {
			fieldErrors = map[string]string{}
		}
		fieldErrors["site_ids"] = "must contain positive integer IDs"
	}
	return query, fieldErrors
}

func parseLogIDList(repeated []string) ([]int64, bool) {
	values := make([]string, 0, len(repeated))
	for _, value := range repeated {
		values = append(values, strings.Split(value, ",")...)
	}
	result := make([]int64, 0, len(values))
	invalid := false
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		value, err := strconv.ParseInt(trimmed, 10, 64)
		if err == nil && value > 0 && strconv.FormatInt(value, 10) == trimmed {
			result = append(result, value)
		} else {
			invalid = true
		}
	}
	return result, invalid
}
