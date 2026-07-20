package controller

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
	"strconv"
)

type performanceHistoryApplication interface {
	List(context.Context, dto.PerformanceHistoryQuery) (dto.PerformanceHistoryPage, error)
	Statistics(context.Context, dto.PerformanceHistoryQuery) (dto.PerformanceHistoryStatisticsResponse, error)
}
type PerformanceHistoryController struct{ history performanceHistoryApplication }

func NewPerformanceHistoryController(h performanceHistoryApplication) *PerformanceHistoryController {
	return &PerformanceHistoryController{history: h}
}
func (c *PerformanceHistoryController) Global(g *gin.Context) { c.list(g, nil) }
func (c *PerformanceHistoryController) Site(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.list(g, []int64{id})
	}
}
func (c *PerformanceHistoryController) GlobalStatistics(g *gin.Context) { c.statistics(g, nil) }
func (c *PerformanceHistoryController) SiteStatistics(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.statistics(g, []int64{id})
	}
}
func (c *PerformanceHistoryController) list(g *gin.Context, forced []int64) {
	if c == nil || c.history == nil {
		common.AbortInternalError(g)
		return
	}
	q, e := parsePerformanceHistoryQuery(g)
	if len(forced) > 0 {
		q.SiteIDs = forced
		e = q.Validate()
	}
	if e != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid performance history query", e)
		return
	}
	out, err := c.history.List(g.Request.Context(), q)
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func (c *PerformanceHistoryController) statistics(g *gin.Context, forced []int64) {
	if c == nil || c.history == nil {
		common.AbortInternalError(g)
		return
	}
	q, e := parsePerformanceHistoryQuery(g)
	q.Page, q.PageSize = 1, 100
	if len(forced) > 0 {
		q.SiteIDs = forced
		e = q.Validate()
	}
	if e != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid performance history query", e)
		return
	}
	out, err := c.history.Statistics(g.Request.Context(), q)
	if err != nil {
		if errors.Is(err, service.ErrPerformanceHistoryTooLarge) {
			common.AbortError(g, http.StatusRequestEntityTooLarge, constant.CodePayloadTooLarge, "Performance history result set is too large", nil)
			return
		}
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func parsePerformanceHistoryQuery(g *gin.Context) (dto.PerformanceHistoryQuery, map[string]string) {
	q := dto.PerformanceHistoryQuery{Page: 1, PageSize: 20, ModelNames: inventoryQueryValues(g, "model_names"), Groups: inventoryQueryValues(g, "groups")}
	if v := g.Query("p"); v != "" {
		q.Page, _ = strconv.Atoi(v)
	}
	if v := g.Query("page_size"); v != "" {
		q.PageSize, _ = strconv.Atoi(v)
	}
	q.StartTimestamp, _ = strconv.ParseInt(g.Query("start_timestamp"), 10, 64)
	q.EndTimestamp, _ = strconv.ParseInt(g.Query("end_timestamp"), 10, 64)
	var bad bool
	q.SiteIDs, bad = parseLogIDList(g.QueryArray("site_ids"))
	if bad || containsNonPositiveInventoryID(q.SiteIDs) {
		return q, map[string]string{"site_ids": "invalid"}
	}
	q.Normalize()
	return q, q.Validate()
}
