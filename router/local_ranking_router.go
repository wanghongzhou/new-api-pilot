package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterLocalRankingRoutes(e *gin.Engine, c *controller.LocalRankingController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/rankings/models", c.GlobalModels)
	g.GET("/rankings/vendors", c.GlobalVendors)
	g.GET("/sites/:id/rankings/models", c.SiteModels)
	g.GET("/sites/:id/rankings/vendors", c.SiteVendors)
}
