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

type subscriptionPlanApplication interface {
	List(context.Context, dto.SubscriptionPlanQuery) (dto.SubscriptionPlanPageResponse, error)
	Statistics(context.Context, dto.SubscriptionPlanQuery) (dto.SubscriptionPlanStatistics, error)
}
type SubscriptionPlanController struct{ s subscriptionPlanApplication }

func NewSubscriptionPlanController(s subscriptionPlanApplication) *SubscriptionPlanController {
	return &SubscriptionPlanController{s: s}
}
func (c *SubscriptionPlanController) Global(g *gin.Context)           { c.run(g, nil, false) }
func (c *SubscriptionPlanController) GlobalStatistics(g *gin.Context) { c.run(g, nil, true) }
func (c *SubscriptionPlanController) Site(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, false)
	}
}
func (c *SubscriptionPlanController) SiteStatistics(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, true)
	}
}
func (c *SubscriptionPlanController) run(g *gin.Context, sites []int64, stats bool) {
	allowed := map[string]bool{"p": true, "page_size": true, "states": true, "enabled": true, "keyword": true}
	if sites == nil {
		allowed["site_ids"] = true
	}
	for key := range g.Request.URL.Query() {
		if !allowed[key] {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid subscription plan query", nil)
			return
		}
	}
	p, _ := strconv.Atoi(g.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(g.DefaultQuery("page_size", "20"))
	q := dto.SubscriptionPlanQuery{Page: p, PageSize: size, SiteIDs: sites, States: inventoryQueryValues(g, "states"), Keyword: g.Query("keyword")}
	if raw, exists := g.GetQuery("enabled"); exists {
		if raw != "true" && raw != "false" {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid subscription plan query", nil)
			return
		}
		value, _ := strconv.ParseBool(raw)
		q.Enabled = &value
	}
	if sites == nil {
		var invalid bool
		q.SiteIDs, invalid = parseLogIDList(g.QueryArray("site_ids"))
		if invalid {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid subscription plan query", nil)
			return
		}
	}
	var v any
	var err error
	if stats {
		v, err = c.s.Statistics(g.Request.Context(), q)
	} else {
		v, err = c.s.List(g.Request.Context(), q)
	}
	if err != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid subscription plan query", nil)
		return
	}
	common.WriteSuccess(g, http.StatusOK, v)
}
