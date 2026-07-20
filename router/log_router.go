package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterLogRoutes(engine *gin.Engine, logs *controller.LogController, resolver middleware.IdentityResolver) {
	if engine == nil || logs == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	routes := engine.Group("/api", authenticated...)
	routes.GET("/logs", logs.Global)
	routes.GET("/sites/:id/logs", logs.Site)
}
