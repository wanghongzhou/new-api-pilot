package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterChannelInventoryRoutes(e *gin.Engine, c *controller.ChannelInventoryController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/channel-inventory", c.Global)
	g.GET("/channel-inventory/statistics", c.GlobalStatistics)
	g.GET("/sites/:id/channel-inventory", c.Site)
	g.GET("/sites/:id/channel-inventory/statistics", c.SiteStatistics)
}
