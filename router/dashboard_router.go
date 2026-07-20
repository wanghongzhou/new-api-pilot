package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterDashboardRoutes(
	engine *gin.Engine,
	dashboard *controller.DashboardController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || dashboard == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	routes := engine.Group("/api/dashboard", authenticated...)
	routes.GET("/summary", dashboard.Summary)
	routes.GET("/trend", dashboard.Trend)
	routes.GET("/top", dashboard.Top)
	routes.GET("/health", dashboard.Health)
}
