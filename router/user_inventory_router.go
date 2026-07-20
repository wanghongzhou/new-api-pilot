package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterUserInventoryRoutes(engine *gin.Engine, inventory *controller.UserInventoryController, resolver middleware.IdentityResolver) {
	if engine == nil || inventory == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	routes := engine.Group("/api", authenticated...)
	routes.GET("/user-inventory", inventory.Global)
	routes.GET("/user-inventory/statistics", inventory.GlobalStatistics)
	routes.GET("/sites/:id/user-inventory", inventory.Site)
	routes.GET("/sites/:id/user-inventory/statistics", inventory.SiteStatistics)
}
