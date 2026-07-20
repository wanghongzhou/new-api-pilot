package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func registerUserRoutes(
	engine *gin.Engine,
	authController *controller.AuthController,
	userController *controller.PlatformUserController,
	resolver middleware.IdentityResolver,
) {
	if authController == nil || userController == nil || resolver == nil {
		return
	}
	users := engine.Group("/api/user")
	users.POST("/login", authController.Login)

	authenticated := users.Group("")
	authenticated.Use(middleware.UserAuth(resolver), middleware.ForcePasswordChange())
	authenticated.POST("/logout", authController.Logout)
	authenticated.GET("/self", authController.Self)
	authenticated.PUT("/password", authController.ChangePassword)
	authenticated.GET("/", userController.List)
	authenticated.POST("/", middleware.AdminAuth(), userController.Create)
	authenticated.PUT("/:id", middleware.AdminAuth(), userController.Update)
	authenticated.POST("/:id/enable", middleware.AdminAuth(), userController.Enable)
	authenticated.POST("/:id/disable", middleware.AdminAuth(), userController.Disable)
	authenticated.POST("/:id/reset-password", middleware.AdminAuth(), userController.ResetPassword)
}
