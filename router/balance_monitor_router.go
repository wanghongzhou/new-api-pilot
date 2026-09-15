package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterBalanceMonitorRoutes(e *gin.Engine, c *controller.BalanceMonitorController, r middleware.IdentityResolver) {
	if c == nil {
		return
	}
	e.POST("/api/balance-monitor/ingest", c.Ingest)
	if r == nil {
		return
	}
	group := e.Group("/api/balance-monitor", middleware.UserAuth(r), middleware.ForcePasswordChange())
	group.GET("/records", c.List)
	group.GET("/accounts", c.Accounts)
}
