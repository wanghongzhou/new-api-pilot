package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterUpstreamTaskRoutes(e *gin.Engine, c *controller.UpstreamTaskController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/upstream-tasks", c.Global)
	g.GET("/upstream-tasks/statistics", c.GlobalStatistics)
	g.GET("/sites/:id/upstream-tasks", c.Site)
	g.GET("/sites/:id/upstream-tasks/statistics", c.SiteStatistics)
}
