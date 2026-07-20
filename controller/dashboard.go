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

type dashboardApplication interface {
	Summary(context.Context) (dto.DashboardSummary, error)
	Trend(context.Context, dto.DashboardTrendQuery) ([]dto.TrendPoint, error)
	Top(context.Context, dto.DashboardTopQuery) ([]dto.DashboardRankingItem, error)
	Health(context.Context) (dto.DashboardHealth, error)
}

type DashboardController struct {
	dashboard dashboardApplication
}

func NewDashboardController(dashboard dashboardApplication) *DashboardController {
	return &DashboardController{dashboard: dashboard}
}

func (controller *DashboardController) Summary(c *gin.Context) {
	if !controller.available(c) || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	result, err := controller.dashboard.Summary(c.Request.Context())
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *DashboardController) Trend(c *gin.Context) {
	if !controller.available(c) || !requireEmptyBody(c) {
		return
	}
	query, fieldErrors := parseDashboardTrendQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid dashboard trend query", fieldErrors)
		return
	}
	result, err := controller.dashboard.Trend(c.Request.Context(), query)
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *DashboardController) Top(c *gin.Context) {
	if !controller.available(c) || !requireEmptyBody(c) {
		return
	}
	query, fieldErrors := parseDashboardTopQuery(c)
	if fieldErrors != nil {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid dashboard top query", fieldErrors)
		return
	}
	result, err := controller.dashboard.Top(c.Request.Context(), query)
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *DashboardController) Health(c *gin.Context) {
	if !controller.available(c) || !requireNoQuery(c) || !requireEmptyBody(c) {
		return
	}
	result, err := controller.dashboard.Health(c.Request.Context())
	if err != nil {
		writeDashboardError(c, err)
		return
	}
	common.WriteSuccess(c, http.StatusOK, result)
}

func (controller *DashboardController) available(c *gin.Context) bool {
	if controller == nil || controller.dashboard == nil {
		common.AbortInternalError(c)
		return false
	}
	return true
}

func parseDashboardTrendQuery(c *gin.Context) (dto.DashboardTrendQuery, map[string]string) {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{"days": {}})
	query := dto.DashboardTrendQuery{}
	query.Normalize()
	if raw, exists := singletonQueryValue(c, "days", fieldErrors); exists {
		value, err := strconv.Atoi(raw)
		if err != nil {
			fieldErrors["days"] = "must be an integer"
		} else {
			query.Days = value
		}
	}
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.DashboardTrendQuery{}, fieldErrors
	}
	return query, nil
}

func parseDashboardTopQuery(c *gin.Context) (dto.DashboardTopQuery, map[string]string) {
	fieldErrors := validateQueryKeys(c, map[string]struct{}{"type": {}, "metric": {}, "limit": {}})
	query := dto.DashboardTopQuery{}
	query.Normalize()
	query.Type, _ = singletonQueryValue(c, "type", fieldErrors)
	query.Metric, _ = singletonQueryValue(c, "metric", fieldErrors)
	if raw, exists := singletonQueryValue(c, "limit", fieldErrors); exists {
		value, err := strconv.Atoi(raw)
		if err != nil {
			fieldErrors["limit"] = "must be an integer"
		} else {
			query.Limit = value
		}
	}
	query.Type = strings.ToLower(strings.TrimSpace(query.Type))
	query.Metric = strings.ToLower(strings.TrimSpace(query.Metric))
	mergeFieldErrors(fieldErrors, query.Validate())
	if len(fieldErrors) > 0 {
		return dto.DashboardTopQuery{}, fieldErrors
	}
	return query, nil
}

func writeDashboardError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrDashboardInvalid) {
		common.AbortError(c, http.StatusBadRequest, constant.CodeValidationError, "Invalid dashboard query", nil)
		return
	}
	common.AbortInternalError(c)
}
