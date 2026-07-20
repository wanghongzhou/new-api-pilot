package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

type settingRouteIdentityResolver struct{ role string }

func (resolver settingRouteIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: resolver.role, Status: constant.UserStatusEnabled}, nil
}

func TestSettingRoutesAllowViewerReadAndRequireAdminWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	viewer := gin.New()
	RegisterSettingRoutes(viewer, controller.NewSettingController(nil), settingRouteIdentityResolver{role: constant.RoleViewer})

	read := httptest.NewRequest(http.MethodGet, "/api/settings?unsupported=true", nil)
	read.Header.Set("New-Api-User", "1")
	readResponse := httptest.NewRecorder()
	viewer.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusBadRequest {
		t.Fatalf("viewer GET /api/settings = %d, want controller-level 400", readResponse.Code)
	}

	write := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"items":[]}`))
	write.Header.Set("New-Api-User", "1")
	writeResponse := httptest.NewRecorder()
	viewer.ServeHTTP(writeResponse, write)
	if writeResponse.Code != http.StatusForbidden {
		t.Fatalf("viewer PUT /api/settings = %d, want 403", writeResponse.Code)
	}
}
