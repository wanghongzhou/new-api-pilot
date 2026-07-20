package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/controller"
)

func TestSystemTaskCatalogRoutesAreReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterSystemTaskCatalogRoutes(engine, controller.NewSystemTaskCatalogController(nil), upstreamTaskRouteResolver{})
	allowed := map[string]bool{"GET /api/system-tasks": true, "GET /api/system-tasks/statistics": true, "GET /api/sites/:id/system-tasks": true, "GET /api/sites/:id/system-tasks/statistics": true}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if allowed[key] {
			delete(allowed, key)
		}
		if route.Path == "/api/system-tasks" && route.Method != http.MethodGet {
			t.Fatalf("mutation route registered: %s", key)
		}
	}
	if len(allowed) > 0 {
		t.Fatalf("missing routes=%v", allowed)
	}
	for _, path := range []string{"/api/system-tasks/1", "/api/system-tasks/current", "/api/system-tasks/run", "/api/system-tasks/create"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("mutation %s status=%d", path, response.Code)
		}
	}
}
