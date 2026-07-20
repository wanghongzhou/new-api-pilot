package router

import (
	"github.com/gin-gonic/gin"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"testing"
)

type upstreamTaskRouteResolver struct{}

func (upstreamTaskRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}

func TestUpstreamTaskRoutesAreReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterUpstreamTaskRoutes(engine, controller.NewUpstreamTaskController(nil), upstreamTaskRouteResolver{})
	wanted := map[string]bool{
		"GET /api/upstream-tasks":                      false,
		"GET /api/upstream-tasks/statistics":           false,
		"GET /api/sites/:id/upstream-tasks":            false,
		"GET /api/sites/:id/upstream-tasks/statistics": false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if route.Path == "/api/upstream-tasks" || route.Path == "/api/upstream-tasks/statistics" || route.Path == "/api/sites/:id/upstream-tasks" || route.Path == "/api/sites/:id/upstream-tasks/statistics" {
			if route.Method != "GET" {
				t.Fatalf("mutation route registered: %s", key)
			}
			wanted[key] = true
		}
	}
	for route, found := range wanted {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
}
