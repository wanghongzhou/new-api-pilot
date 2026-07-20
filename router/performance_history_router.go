package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterPerformanceHistoryRoutes(e *gin.Engine, c *controller.PerformanceHistoryController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/performance-history", c.Global)
	g.GET("/performance-history/statistics", c.GlobalStatistics)
	g.GET("/sites/:id/performance-history", c.Site)
	g.GET("/sites/:id/performance-history/statistics", c.SiteStatistics)
}
