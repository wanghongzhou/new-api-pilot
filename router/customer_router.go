package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func registerCustomerRoutes(
	engine *gin.Engine,
	customerController *controller.CustomerController,
	accountController *controller.AccountController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || customerController == nil || accountController == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}

	customers := engine.Group("/api/customers", authenticated...)
	customers.GET("", customerController.List)
	customers.POST("", middleware.AdminAuth(), customerController.Create)
	customers.GET("/:id", customerController.Get)
	customers.PUT("/:id", middleware.AdminAuth(), customerController.Update)
	customers.DELETE("/:id", middleware.AdminAuth(), customerController.Delete)
	customers.POST("/:id/disable", middleware.AdminAuth(), customerController.Disable)
	customers.POST("/:id/enable", middleware.AdminAuth(), customerController.Enable)
	customers.GET("/:id/accounts", customerController.ListAccounts)
	customers.GET("/:id/stats", customerController.Statistics)

	accounts := engine.Group("/api/accounts", authenticated...)
	accounts.GET("", accountController.List)
	accounts.GET("/site/:siteId/remote-users", middleware.AdminAuth(), accountController.SearchRemoteUsers)
	accounts.POST("", middleware.AdminAuth(), accountController.Create)
	accounts.GET("/:id", accountController.Get)
	accounts.PUT("/:id", middleware.AdminAuth(), accountController.Update)
	accounts.DELETE("/:id", middleware.AdminAuth(), accountController.Delete)
	accounts.POST("/:id/archive", middleware.AdminAuth(), accountController.Archive)
	accounts.POST("/:id/restore", middleware.AdminAuth(), accountController.Restore)
	accounts.POST("/:id/refresh", middleware.AdminAuth(), accountController.Refresh)
	accounts.GET("/:id/stats", accountController.Statistics)
}
