package router

import (
	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterStatisticsRoutes(
	engine *gin.Engine,
	statistics *controller.StatisticsController,
	resolver middleware.IdentityResolver,
) {
	if engine == nil || statistics == nil || resolver == nil {
		return
	}
	authenticated := []gin.HandlerFunc{middleware.UserAuth(resolver), middleware.ForcePasswordChange()}
	routes := engine.Group("/api/statistics", authenticated...)
	routes.GET("/global", statistics.Global)
	routes.GET("/sites", statistics.Sites)
	routes.GET("/customers", statistics.Customers)
	routes.GET("/accounts", statistics.Accounts)
	routes.GET("/models", statistics.Models)
	routes.GET("/channels", statistics.Channels)
	routes.GET("/groups", statistics.Groups)
	routes.GET("/tokens", statistics.Tokens)
	routes.GET("/nodes", statistics.Nodes)
	routes.GET("/options/models", statistics.ModelOptions)
	routes.GET("/options/channels", statistics.ChannelOptions)
	routes.GET("/options/groups", statistics.GroupOptions)
	routes.GET("/options/tokens", statistics.TokenOptions)
	routes.GET("/options/nodes", statistics.NodeOptions)

	sites := engine.Group("/api/sites", authenticated...)
	sites.GET("/:id/stats", statistics.Site)
}
