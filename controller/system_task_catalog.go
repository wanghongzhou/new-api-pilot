package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type systemTaskCatalogApplication interface {
	List(context.Context, dto.SystemTaskQuery) (dto.SystemTaskPageResponse, error)
	Statistics(context.Context, dto.SystemTaskQuery) (dto.SystemTaskStatisticsResponse, error)
}
type SystemTaskCatalogController struct{ s systemTaskCatalogApplication }

func NewSystemTaskCatalogController(s systemTaskCatalogApplication) *SystemTaskCatalogController {
	return &SystemTaskCatalogController{s: s}
}
func (c *SystemTaskCatalogController) Global(g *gin.Context)           { c.run(g, nil, false) }
func (c *SystemTaskCatalogController) GlobalStatistics(g *gin.Context) { c.run(g, nil, true) }
func (c *SystemTaskCatalogController) Site(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, false)
	}
}
func (c *SystemTaskCatalogController) SiteStatistics(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, true)
	}
}
func (c *SystemTaskCatalogController) run(g *gin.Context, sites []int64, statistics bool) {
	if c == nil || c.s == nil {
		common.AbortInternalError(g)
		return
	}
	q, fields := parseSystemTaskQuery(g, sites == nil)
	if sites != nil {
		q.SiteIDs = sites
	}
	if fields == nil {
		fields = q.Validate()
	}
	if fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid system task query", fields)
		return
	}
	var out any
	var err error
	if statistics {
		out, err = c.s.Statistics(g.Request.Context(), q)
	} else {
		out, err = c.s.List(g.Request.Context(), q)
	}
	if err != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid system task query", nil)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func parseSystemTaskQuery(g *gin.Context, allowSites bool) (dto.SystemTaskQuery, map[string]string) {
	q := dto.SystemTaskQuery{Page: 1, PageSize: 20, Types: inventoryQueryValues(g, "types"), Statuses: inventoryQueryValues(g, "statuses")}
	allowed := map[string]bool{"p": true, "page_size": true, "types": true, "statuses": true, "error_present": true, "created_start": true, "created_end": true}
	if allowSites {
		allowed["site_ids"] = true
	}
	for key := range g.Request.URL.Query() {
		if !allowed[key] {
			return q, map[string]string{key: "invalid"}
		}
	}
	if raw := g.Query("p"); raw != "" {
		q.Page, _ = strconv.Atoi(raw)
	}
	if raw := g.Query("page_size"); raw != "" {
		q.PageSize, _ = strconv.Atoi(raw)
	}
	if allowSites {
		var invalid bool
		q.SiteIDs, invalid = parseLogIDList(g.QueryArray("site_ids"))
		if invalid || containsNonPositiveInventoryID(q.SiteIDs) {
			return q, map[string]string{"site_ids": "invalid"}
		}
	}
	if raw, exists := g.GetQuery("error_present"); exists {
		value, err := strconv.ParseBool(raw)
		if err != nil || strconv.FormatBool(value) != raw {
			return q, map[string]string{"error_present": "invalid"}
		}
		q.ErrorPresent = &value
	}
	if raw := g.Query("created_start"); raw != "" {
		q.CreatedStart, _ = strconv.ParseInt(raw, 10, 64)
	}
	if raw := g.Query("created_end"); raw != "" {
		q.CreatedEnd, _ = strconv.ParseInt(raw, 10, 64)
	}
	q.Normalize()
	return q, q.Validate()
}
