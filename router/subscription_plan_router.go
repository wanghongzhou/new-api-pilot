package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterSubscriptionPlanRoutes(e *gin.Engine, c *controller.SubscriptionPlanController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/subscription-plans", c.Global)
	g.GET("/subscription-plans/statistics", c.GlobalStatistics)
	g.GET("/sites/:id/subscription-plans", c.Site)
	g.GET("/sites/:id/subscription-plans/statistics", c.SiteStatistics)
}
