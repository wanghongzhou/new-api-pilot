package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type localRankingApplication interface {
	Query(context.Context, dto.LocalRankingQuery, string) (dto.LocalRankingResponse, error)
}
type LocalRankingController struct{ service localRankingApplication }

func NewLocalRankingController(service localRankingApplication) *LocalRankingController {
	return &LocalRankingController{service: service}
}
func (c *LocalRankingController) GlobalModels(g *gin.Context)  { c.run(g, nil, "model") }
func (c *LocalRankingController) GlobalVendors(g *gin.Context) { c.run(g, nil, "vendor") }
func (c *LocalRankingController) SiteModels(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, "model")
	}
}
func (c *LocalRankingController) SiteVendors(g *gin.Context) {
	if id, ok := parsePositivePathID(g, "id", "site"); ok {
		c.run(g, []int64{id}, "vendor")
	}
}
func (c *LocalRankingController) run(g *gin.Context, sites []int64, kind string) {
	if c == nil || c.service == nil {
		common.AbortInternalError(g)
		return
	}
	q := dto.LocalRankingQuery{Period: g.DefaultQuery("period", "today"), SiteIDs: sites}
	if sites == nil {
		var invalid bool
		q.SiteIDs, invalid = parseLogIDList(g.QueryArray("site_ids"))
		if invalid {
			common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid ranking query", map[string]string{"site_ids": "invalid"})
			return
		}
	}
	out, err := c.service.Query(g.Request.Context(), q, kind)
	if err != nil {
		common.AbortError(g, http.StatusBadRequest, constant.CodeValidationError, "Invalid ranking query", nil)
		return
	}
	common.WriteSuccess(g, http.StatusOK, out)
}
