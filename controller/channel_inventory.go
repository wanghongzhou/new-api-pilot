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
)

type channelInventoryApplication interface {
	List(context.Context, dto.ChannelInventoryQuery) (dto.ChannelInventoryPage, error)
	Statistics(context.Context, dto.ChannelInventoryStatisticsQuery) (dto.ChannelInventoryStatisticsResponse, error)
}
type ChannelInventoryController struct{ inventory channelInventoryApplication }

func NewChannelInventoryController(a channelInventoryApplication) *ChannelInventoryController {
	return &ChannelInventoryController{inventory: a}
}
func (c *ChannelInventoryController) Global(g *gin.Context) { c.list(g, nil) }
func (c *ChannelInventoryController) Site(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.list(g, []int64{id})
	}
}
func (c *ChannelInventoryController) GlobalStatistics(g *gin.Context) { c.statistics(g, nil) }
func (c *ChannelInventoryController) SiteStatistics(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.statistics(g, []int64{id})
	}
}
func (c *ChannelInventoryController) list(g *gin.Context, forced []int64) {
	if c == nil || c.inventory == nil {
		common.AbortInternalError(g)
		return
	}
	q, e := parseChannelInventoryQuery(g)
	if len(forced) > 0 {
		q.SiteIDs = forced
		e = q.Validate()
	}
	if e != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid channel inventory query", e)
		return
	}
	out, err := c.inventory.List(g.Request.Context(), q)
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func (c *ChannelInventoryController) statistics(g *gin.Context, forced []int64) {
	if c == nil || c.inventory == nil {
		common.AbortInternalError(g)
		return
	}
	q, e := parseChannelInventoryStatisticsQuery(g)
	if len(forced) > 0 {
		q.SiteIDs = forced
		e = q.Validate()
	}
	if e != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid channel inventory statistics query", e)
		return
	}
	out, err := c.inventory.Statistics(g.Request.Context(), q)
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func parseChannelInventoryQuery(g *gin.Context) (dto.ChannelInventoryQuery, map[string]string) {
	q := dto.ChannelInventoryQuery{Page: 1, PageSize: 20, Keyword: g.Query("keyword"), Groups: inventoryQueryValues(g, "groups"), Tags: inventoryQueryValues(g, "tags"), States: inventoryQueryValues(g, "states")}
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
	q.Types, bad = parseInventoryInts(inventoryQueryValues(g, "types"))
	if bad {
		return q, map[string]string{"types": "invalid"}
	}
	q.Statuses, bad = parseInventoryInts(inventoryQueryValues(g, "statuses"))
	if bad {
		return q, map[string]string{"statuses": "invalid"}
	}
	q.MinBalance = optionalString(g.Query("min_balance"))
	q.MaxBalance = optionalString(g.Query("max_balance"))
	var e map[string]string
	q.MinResponseTimeMS, e = parseOptionalCanonicalInt64(g.Query("min_response_time_ms"), "min_response_time_ms")
	if e != nil {
		return q, e
	}
	q.MaxResponseTimeMS, e = parseOptionalCanonicalInt64(g.Query("max_response_time_ms"), "max_response_time_ms")
	if e != nil {
		return q, e
	}
	q.Normalize()
	return q, q.Validate()
}
func parseChannelInventoryStatisticsQuery(g *gin.Context) (dto.ChannelInventoryStatisticsQuery, map[string]string) {
	q := dto.ChannelInventoryStatisticsQuery{Groups: inventoryQueryValues(g, "groups"), Tags: inventoryQueryValues(g, "tags")}
	q.StartTimestamp, _ = strconv.ParseInt(g.Query("start_timestamp"), 10, 64)
	q.EndTimestamp, _ = strconv.ParseInt(g.Query("end_timestamp"), 10, 64)
	q.SiteIDs, _ = parseLogIDList(g.QueryArray("site_ids"))
	q.Types, _ = parseInventoryInts(inventoryQueryValues(g, "types"))
	q.Statuses, _ = parseInventoryInts(inventoryQueryValues(g, "statuses"))
	q.Normalize()
	return q, q.Validate()
}
func optionalString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
func parseOptionalCanonicalInt64(v, field string) (*int64, map[string]string) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != v {
		return nil, map[string]string{field: "must be a canonical non-negative int64"}
	}
	return &n, nil
}
