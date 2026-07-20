package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterSystemTaskCatalogRoutes(e *gin.Engine, c *controller.SystemTaskCatalogController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/system-tasks", c.Global)
	g.GET("/system-tasks/statistics", c.GlobalStatistics)
	g.GET("/sites/:id/system-tasks", c.Site)
	g.GET("/sites/:id/system-tasks/statistics", c.SiteStatistics)
}
