package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func registerSiteRoutes(engine *gin.Engine, siteController *controller.SiteController, resolver middleware.IdentityResolver) {
	if siteController == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}

	sites := engine.Group("/api/sites", authenticated...)
	sites.GET("", siteController.List)
	sites.POST("", middleware.AdminAuth(), siteController.Create)
	sites.POST("/refresh", middleware.AdminAuth(), siteController.RefreshBatch)
	sites.GET("/:id", siteController.Get)
	sites.GET("/:id/performance", siteController.Performance)
	sites.POST("/:id/base-url-preflight", middleware.AdminAuth(), siteController.PreflightBaseURL)
	sites.PUT("/:id", middleware.AdminAuth(), siteController.Update)
	sites.DELETE("/:id", middleware.AdminAuth(), siteController.Delete)
	sites.POST("/:id/authorize", middleware.AdminAuth(), siteController.Authorize)
	sites.POST("/:id/recheck-capabilities", middleware.AdminAuth(), siteController.RecheckCapabilities)
	sites.POST("/:id/probe", middleware.AdminAuth(), siteController.Probe)
	sites.POST("/:id/refresh", middleware.AdminAuth(), siteController.Refresh)
	sites.POST("/:id/backfill", middleware.AdminAuth(), siteController.Backfill)
	sites.POST("/:id/disable", middleware.AdminAuth(), siteController.Disable)
	sites.POST("/:id/enable", middleware.AdminAuth(), siteController.Enable)
	sites.POST("/:id/end-statistics", middleware.AdminAuth(), siteController.EndStatistics)
	sites.DELETE("/:id/statistics-end", middleware.AdminAuth(), siteController.ClearStatisticsEnd)
	sites.GET("/:id/status", siteController.ResourceStatus)
	sites.GET("/:id/instances", siteController.ListInstances)
	sites.GET("/:id/collection-runs", siteController.ListCollectionRuns)

	runs := engine.Group("/api/collection-runs", authenticated...)
	runs.GET("/:id", siteController.GetCollectionRun)
	runs.GET("/:id/windows", siteController.ListCollectionRunWindows)
}
