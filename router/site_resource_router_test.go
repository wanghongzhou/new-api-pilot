package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"new-api-pilot/service"
)

type resourceRouteIdentityResolver struct{}

func (resourceRouteIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}

func TestSiteResourceStatusRouteIsViewerReadable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerSiteRoutes(engine, controller.NewSiteController(&service.SiteService{}), resourceRouteIdentityResolver{})
	found := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/sites/:id/status" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("GET /api/sites/:id/status was not registered")
	}

	request := httptest.NewRequest(http.MethodGet, "/api/sites/1/status", nil)
	request.Header.Set("New-Api-User", "1")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("viewer status without required query = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}
