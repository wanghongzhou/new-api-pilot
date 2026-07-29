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

type pricingCatalogApplication interface {
	List(context.Context, dto.PricingCatalogQuery) (dto.PricingCatalogPageResponse, error)
	ListGroups(context.Context, dto.PricingCatalogQuery) (dto.PricingGroupPageResponse, error)
	Statistics(context.Context, dto.PricingCatalogQuery) (dto.PricingCatalogStatistics, error)
}
type PricingCatalogController struct{ s pricingCatalogApplication }

func NewPricingCatalogController(s pricingCatalogApplication) *PricingCatalogController {
	return &PricingCatalogController{s: s}
}
func (c *PricingCatalogController) Global(g *gin.Context)           { c.run(g, nil, "pricing") }
func (c *PricingCatalogController) GlobalStatistics(g *gin.Context) { c.run(g, nil, "statistics") }
func (c *PricingCatalogController) GlobalGroups(g *gin.Context)     { c.run(g, nil, "groups") }
func (c *PricingCatalogController) Site(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, "pricing")
	}
}
func (c *PricingCatalogController) SiteStatistics(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, "statistics")
	}
}
func (c *PricingCatalogController) SiteGroups(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, "groups")
	}
}
func (c *PricingCatalogController) run(g *gin.Context, sites []int64, kind string) {
	if c == nil || c.s == nil {
		common.AbortInternalError(g)
		return
	}
	allowed := map[string]bool{"p": true, "page_size": true}
	if kind != "statistics" {
		allowed["states"] = true
		allowed["keyword"] = true
		allowed["group"] = true
	}
	if kind == "pricing" {
		allowed["billing_mode"] = true
	}
	if sites == nil {
		allowed["site_ids"] = true
	}
	for key := range g.Request.URL.Query() {
		if !allowed[key] {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid pricing catalog query", nil)
			return
		}
	}
	p, _ := strconv.Atoi(g.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(g.DefaultQuery("page_size", "20"))
	q := dto.PricingCatalogQuery{Page: p, PageSize: size, SiteIDs: sites, States: inventoryQueryValues(g, "states"), Keyword: g.Query("keyword"), Group: g.Query("group"), BillingMode: g.Query("billing_mode")}
	if sites == nil {
		var invalid bool
		q.SiteIDs, invalid = parseLogIDList(g.QueryArray("site_ids"))
		if invalid {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid pricing catalog query", nil)
			return
		}
	}
	q.Normalize()
	if fields := q.Validate(); fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid pricing catalog query", fields)
		return
	}
	var result any
	var err error
	switch kind {
	case "statistics":
		result, err = c.s.Statistics(g.Request.Context(), q)
	case "groups":
		result, err = c.s.ListGroups(g.Request.Context(), q)
	default:
		result, err = c.s.List(g.Request.Context(), q)
	}
	if err != nil {
		common.AbortInternalError(g)
		return
	}
	common.WriteSuccess(g, http.StatusOK, result)
}
