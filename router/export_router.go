package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterExportRoutes(
	engine *gin.Engine,
	exports *controller.ExportController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || exports == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	routes := engine.Group("/api/statistics", authenticated...)
	routes.POST("/export", exports.Create)
	routes.GET("/exports", exports.List)
	routes.GET("/exports/:id", exports.Detail)
	routes.GET("/exports/:id/download", exports.Download)
}
