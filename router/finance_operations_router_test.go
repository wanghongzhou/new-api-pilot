package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

type financeRouteResolver struct{}

func (financeRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}

func TestFinanceOperationsRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterFinanceOperationsRoutes(engine, controller.NewFinanceOperationsController(nil), financeRouteResolver{})
	wanted := map[string]bool{"GET /api/topups": false, "GET /api/topups/statistics": false, "GET /api/sites/:id/topups": false, "GET /api/sites/:id/topups/statistics": false, "GET /api/redemptions": false, "GET /api/redemptions/statistics": false, "GET /api/sites/:id/redemptions": false, "GET /api/sites/:id/redemptions/statistics": false}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := wanted[key]; ok {
			wanted[key] = true
		}
	}
	for route, found := range wanted {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
}
