package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"strconv"
)

type upstreamTaskApplication interface {
	List(context.Context, dto.UpstreamTaskQuery) (dto.UpstreamTaskPageResponse, error)
	Statistics(context.Context, dto.UpstreamTaskQuery) (dto.UpstreamTaskStatisticsResponse, error)
}
type UpstreamTaskController struct{ service upstreamTaskApplication }

func NewUpstreamTaskController(s upstreamTaskApplication) *UpstreamTaskController {
	return &UpstreamTaskController{service: s}
}
func (c *UpstreamTaskController) Global(g *gin.Context) { c.list(g, nil) }
func (c *UpstreamTaskController) Site(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.list(g, []int64{id})
	}
}
func (c *UpstreamTaskController) GlobalStatistics(g *gin.Context) { c.statistics(g, nil) }
func (c *UpstreamTaskController) SiteStatistics(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.statistics(g, []int64{id})
	}
}
func (c *UpstreamTaskController) list(g *gin.Context, forced []int64) {
	if c == nil || c.service == nil {
		common.AbortInternalError(g)
		return
	}
	q, fields := parseUpstreamTaskQuery(g, forced == nil)
	if len(forced) > 0 {
		q.SiteIDs = forced
		fields = q.Validate()
	}
	if fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid upstream task query", fields)
		return
	}
	out, err := c.service.List(g.Request.Context(), q)
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func (c *UpstreamTaskController) statistics(g *gin.Context, forced []int64) {
	if c == nil || c.service == nil {
		common.AbortInternalError(g)
		return
	}
	q, fields := parseUpstreamTaskQuery(g, forced == nil)
	if len(forced) > 0 {
		q.SiteIDs = forced
		fields = q.Validate()
	}
	if fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid upstream task statistics query", fields)
		return
	}
	out, err := c.service.Statistics(g.Request.Context(), q)
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
func parseUpstreamTaskQuery(g *gin.Context, allowSites bool) (dto.UpstreamTaskQuery, map[string]string) {
	q := dto.UpstreamTaskQuery{Page: 1, PageSize: 20, TaskID: g.Query("task_id"), Platforms: inventoryQueryValues(g, "platforms"), Groups: inventoryQueryValues(g, "groups"), Actions: inventoryQueryValues(g, "actions"), Statuses: inventoryQueryValues(g, "statuses"), Models: inventoryQueryValues(g, "models")}
	allowed := map[string]bool{"p": true, "page_size": true, "remote_id": true, "remote_user_id": true, "remote_channel_id": true, "task_id": true, "platforms": true, "groups": true, "actions": true, "statuses": true, "models": true, "start_timestamp": true, "end_timestamp": true}
	if allowSites {
		allowed["site_ids"] = true
	}
	if fields := strictQueryFields(g, allowed, "p", "page_size", "remote_id", "remote_user_id", "remote_channel_id", "task_id", "start_timestamp", "end_timestamp"); fields != nil {
		return q, fields
	}
	if v := g.Query("p"); v != "" {
		q.Page, _ = strconv.Atoi(v)
	}
	if v := g.Query("page_size"); v != "" {
		q.PageSize, _ = strconv.Atoi(v)
	}
	var bad bool
	if allowSites {
		q.SiteIDs, bad = parseLogIDList(g.QueryArray("site_ids"))
		if bad || containsNonPositiveInventoryID(q.SiteIDs) {
			return q, map[string]string{"site_ids": "invalid"}
		}
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
	q.RemoteChannelID, fields = parseOptionalCanonicalInt64(g.Query("remote_channel_id"), "remote_channel_id")
	if fields != nil {
		return q, fields
	}
	if v := g.Query("start_timestamp"); v != "" {
		q.StartTimestamp, fields = parseCanonicalTaskTimestamp(v, "start_timestamp")
		if fields != nil {
			return q, fields
		}
	}
	if v := g.Query("end_timestamp"); v != "" {
		q.EndTimestamp, fields = parseCanonicalTaskTimestamp(v, "end_timestamp")
		if fields != nil {
			return q, fields
		}
	}
	q.Normalize()
	return q, q.Validate()
}

func parseCanonicalTaskTimestamp(raw string, field string) (int64, map[string]string) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
		return 0, map[string]string{field: "must be a canonical positive Unix timestamp"}
	}
	return value, nil
}
