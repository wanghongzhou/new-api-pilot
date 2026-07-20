package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterPricingCatalogRoutes(e *gin.Engine, c *controller.PricingCatalogController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/pricing-catalog", c.Global)
	g.GET("/pricing-catalog/statistics", c.GlobalStatistics)
	g.GET("/group-catalog", c.GlobalGroups)
	g.GET("/sites/:id/pricing-catalog", c.Site)
	g.GET("/sites/:id/pricing-catalog/statistics", c.SiteStatistics)
	g.GET("/sites/:id/group-catalog", c.SiteGroups)
}
