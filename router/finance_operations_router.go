package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterFinanceOperationsRoutes(e *gin.Engine, c *controller.FinanceOperationsController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/topups", c.GlobalTopups)
	g.GET("/topups/statistics", c.GlobalTopupStatistics)
	g.GET("/sites/:id/topups", c.SiteTopups)
	g.GET("/sites/:id/topups/statistics", c.SiteTopupStatistics)
	g.GET("/redemptions", c.GlobalRedemptions)
	g.GET("/redemptions/statistics", c.GlobalRedemptionStatistics)
	g.GET("/sites/:id/redemptions", c.SiteRedemptions)
	g.GET("/sites/:id/redemptions/statistics", c.SiteRedemptionStatistics)
}
