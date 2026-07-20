package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterNotificationRoutes(
	engine *gin.Engine,
	notificationController *controller.NotificationController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || notificationController == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	notifications := engine.Group("/api/notifications", authenticated...)
	notifications.POST("/dingtalk/test", middleware.AdminAuth(), notificationController.TestDingTalk)
}
