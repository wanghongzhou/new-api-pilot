package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func RegisterModelCatalogRoutes(e *gin.Engine, c *controller.ModelCatalogController, r middleware.IdentityResolver) {
	if e == nil || c == nil || r == nil {
		return
	}
	g := e.Group("/api", middleware.UserAuth(r), middleware.ForcePasswordChange())
	g.GET("/model-catalog", c.Global)
	g.GET("/model-catalog/coverage", c.GlobalCoverage)
	g.GET("/model-catalog/missing", c.GlobalMissing)
	g.GET("/sites/:id/model-catalog", c.Site)
	g.GET("/sites/:id/model-catalog/coverage", c.SiteCoverage)
	g.GET("/sites/:id/model-catalog/missing", c.SiteMissing)
}
