package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func registerSettingRoutes(
	engine *gin.Engine,
	settingController *controller.SettingController,
	resolver middleware.IdentityResolver,
) {
	RegisterSettingRoutes(engine, settingController, resolver)
}

func RegisterSettingRoutes(
	engine *gin.Engine,
	settingController *controller.SettingController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || settingController == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	settings := engine.Group("/api/settings", authenticated...)
	settings.GET("", settingController.Get)
	settings.PUT("", middleware.AdminAuth(), settingController.Update)
}
