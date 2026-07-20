package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
)

type routerLogApplication struct{}

func (routerLogApplication) Query(_ context.Context, query dto.LogQuery) (dto.LogResponse, error) {
	return dto.LogResponse{Items: []dto.LogItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: dto.LogCollectionComplete}, nil
}

type logRouteResolver struct{}

func (logRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}

func TestLogRoutesRequireAuthenticationAndAllowViewerReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterLogRoutes(engine, controller.NewLogController(routerLogApplication{}), logRouteResolver{})
	query := "?start_timestamp=100&end_timestamp=200&p=1&page_size=20"
	for _, path := range []string{"/api/logs", "/api/sites/2/logs"} {
		unauthorized := httptest.NewRecorder()
		engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path+query, nil))
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s = %d", path, unauthorized.Code)
		}
		request := httptest.NewRequest(http.MethodGet, path+query, nil)
		request.Header.Set("New-Api-User", "1")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("viewer %s = %d %s", path, response.Code, response.Body.String())
		}
	}
}
