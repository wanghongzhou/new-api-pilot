package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func registerAlertRoutes(engine *gin.Engine, alertController *controller.AlertController, resolver middleware.IdentityResolver) {
	RegisterAlertRoutes(engine, alertController, resolver)
}

func RegisterAlertRoutes(engine *gin.Engine, alertController *controller.AlertController, resolver middleware.IdentityResolver) {
	if engine == nil || alertController == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}

	alerts := engine.Group("/api/alerts", authenticated...)
	alerts.GET("/summary", alertController.Summary)
	alerts.GET("", alertController.List)
	alerts.GET("/:id", alertController.Get)

	rules := engine.Group("/api/alert-rules", authenticated...)
	rules.GET("", alertController.ListRules)
	rules.PUT("/:id", middleware.AdminAuth(), alertController.UpdateRule)
	rules.POST("/overrides", middleware.AdminAuth(), alertController.CreateOverride)
	rules.DELETE("/:id", middleware.AdminAuth(), alertController.DeleteOverride)
}
