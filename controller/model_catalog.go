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

type modelCatalogApplication interface {
	List(context.Context, dto.ModelCatalogQuery) (dto.ModelCatalogPageResponse, error)
	Missing(context.Context, dto.ModelCatalogQuery) (dto.MissingModelPageResponse, error)
	Coverage(context.Context, dto.ModelCatalogQuery) (dto.ModelCoverageResponse, error)
}
type ModelCatalogController struct{ s modelCatalogApplication }

func NewModelCatalogController(s modelCatalogApplication) *ModelCatalogController {
	return &ModelCatalogController{s: s}
}
func (c *ModelCatalogController) Global(g *gin.Context) { c.run(g, nil, "list") }
func (c *ModelCatalogController) Site(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.run(g, []int64{id}, "list")
	}
}
func (c *ModelCatalogController) GlobalMissing(g *gin.Context) { c.run(g, nil, "missing") }
func (c *ModelCatalogController) SiteMissing(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.run(g, []int64{id}, "missing")
	}
}
func (c *ModelCatalogController) GlobalCoverage(g *gin.Context) { c.run(g, nil, "coverage") }
func (c *ModelCatalogController) SiteCoverage(g *gin.Context) {
	id, ok := parsePositivePathID(g, "id", "site")
	if ok {
		c.run(g, []int64{id}, "coverage")
	}
}
func (c *ModelCatalogController) run(g *gin.Context, sites []int64, kind string) {
	if c == nil || c.s == nil {
		common.AbortInternalError(g)
		return
	}
	p, _ := strconv.Atoi(g.DefaultQuery("p", "1"))
	size, _ := strconv.Atoi(g.DefaultQuery("page_size", "20"))
	q := dto.ModelCatalogQuery{Page: p, PageSize: size, SiteIDs: sites, Keyword: g.Query("keyword")}
	if sites == nil {
		var bad bool
		q.SiteIDs, bad = parseLogIDList(g.QueryArray("site_ids"))
		if bad {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid model catalog query", map[string]string{"site_ids": "invalid"})
			return
		}
	}
	var fields map[string]string
	q.VendorID, fields = parseOptionalCanonicalInt64(g.Query("vendor_id"), "vendor_id")
	if fields != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid model catalog query", fields)
		return
	}
	var bad bool
	q.Statuses, bad = parseInventoryInts(inventoryQueryValues(g, "statuses"))
	if bad {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid model catalog query", map[string]string{"statuses": "invalid"})
		return
	}
	q.SyncOfficial, bad = parseInventoryInts(inventoryQueryValues(g, "sync_official"))
	if bad {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid model catalog query", map[string]string{"sync_official": "invalid"})
		return
	}
	var v any
	var err error
	if kind == "list" {
		v, err = c.s.List(g.Request.Context(), q)
	} else if kind == "missing" {
		v, err = c.s.Missing(g.Request.Context(), q)
	} else {
		v, err = c.s.Coverage(g.Request.Context(), q)
	}
	if err != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid model catalog query", nil)
		return
	}
	common.WriteSuccess(g, http.StatusOK, v)
}
