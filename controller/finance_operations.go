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

type financeOperationsApplication interface {
	Topups(context.Context, dto.FinanceInventoryQuery) (dto.FinanceInventoryPage[dto.TopupInventoryItem], error)
	Redemptions(context.Context, dto.FinanceInventoryQuery) (dto.FinanceInventoryPage[dto.RedemptionInventoryItem], error)
	TopupStatistics(context.Context, dto.FinanceInventoryQuery) (dto.FinanceStatisticsResponse, error)
	RedemptionStatistics(context.Context, dto.FinanceInventoryQuery) (dto.FinanceStatisticsResponse, error)
}
type FinanceOperationsController struct{ service financeOperationsApplication }

func NewFinanceOperationsController(s financeOperationsApplication) *FinanceOperationsController {
	return &FinanceOperationsController{service: s}
}
func (c *FinanceOperationsController) GlobalTopups(g *gin.Context) { c.handle(g, nil, "topup", false) }
func (c *FinanceOperationsController) SiteTopups(g *gin.Context)   { c.handleSite(g, "topup", false) }
func (c *FinanceOperationsController) GlobalTopupStatistics(g *gin.Context) {
	c.handle(g, nil, "topup", true)
}
func (c *FinanceOperationsController) SiteTopupStatistics(g *gin.Context) {
	c.handleSite(g, "topup", true)
}
func (c *FinanceOperationsController) GlobalRedemptions(g *gin.Context) {
	c.handle(g, nil, "redemption", false)
}
func (c *FinanceOperationsController) SiteRedemptions(g *gin.Context) {
	c.handleSite(g, "redemption", false)
}
func (c *FinanceOperationsController) GlobalRedemptionStatistics(g *gin.Context) {
	c.handle(g, nil, "redemption", true)
}
func (c *FinanceOperationsController) SiteRedemptionStatistics(g *gin.Context) {
	c.handleSite(g, "redemption", true)
}
func (c *FinanceOperationsController) handleSite(g *gin.Context, kind string, statistics bool) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.handle(g, []int64{id}, kind, statistics)
	}
}
func (c *FinanceOperationsController) handle(g *gin.Context, forced []int64, kind string, statistics bool) {
	if c == nil || c.service == nil {
		common.AbortInternalError(g)
		return
	}
	q, fields := parseFinanceInventoryQuery(g)
	if len(forced) > 0 {
		q.SiteIDs = forced
		fields = q.Validate()
	}
	if fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid finance inventory query", fields)
		return
	}
	var out any
	var err error
	if kind == "topup" {
		if statistics {
			out, err = c.service.TopupStatistics(g.Request.Context(), q)
		} else {
			out, err = c.service.Topups(g.Request.Context(), q)
		}
	} else {
		if statistics {
			out, err = c.service.RedemptionStatistics(g.Request.Context(), q)
		} else {
			out, err = c.service.Redemptions(g.Request.Context(), q)
		}
	}
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func parseFinanceInventoryQuery(g *gin.Context) (dto.FinanceInventoryQuery, map[string]string) {
	q := dto.FinanceInventoryQuery{Page: 1, PageSize: 20, Statuses: inventoryQueryValues(g, "statuses"), Providers: inventoryQueryValues(g, "providers"), Methods: inventoryQueryValues(g, "methods"), States: inventoryQueryValues(g, "states"), Keyword: g.Query("keyword")}
	if v := g.Query("p"); v != "" {
		q.Page, _ = strconv.Atoi(v)
	}
	if v := g.Query("page_size"); v != "" {
		q.PageSize, _ = strconv.Atoi(v)
	}
	var bad bool
	q.SiteIDs, bad = parseLogIDList(g.QueryArray("site_ids"))
	if bad || containsNonPositiveInventoryID(q.SiteIDs) {
		return q, map[string]string{"site_ids": "invalid"}
	}
	var fields map[string]string
	q.RemoteID, fields = parseOptionalCanonicalInt64(g.Query("remote_id"), "remote_id")
	if fields != nil {
		return q, fields
	}
	q.RemoteUserID, fields = parseOptionalCanonicalInt64(g.Query("remote_user_id"), "remote_user_id")
	if fields != nil {
		return q, fields
	}
	if v := g.Query("start_timestamp"); v != "" {
		q.StartTimestamp, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := g.Query("end_timestamp"); v != "" {
		q.EndTimestamp, _ = strconv.ParseInt(v, 10, 64)
	}
	q.Normalize()
	return q, q.Validate()
}
