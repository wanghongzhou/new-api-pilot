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

type routerUserInventoryApplication struct{}

func (routerUserInventoryApplication) List(_ context.Context, query dto.UserInventoryQuery) (dto.UserInventoryPage, error) {
	return dto.UserInventoryPage{Items: []dto.UserInventoryItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: "complete"}, nil
}

func (routerUserInventoryApplication) Statistics(context.Context, dto.UserInventoryStatisticsQuery) (dto.UserInventoryStatisticsResponse, error) {
	return dto.UserInventoryStatisticsResponse{Trend: []dto.UserInventoryTrendPoint{}, RoleBreakdown: []dto.UserInventoryBreakdown{}, StatusBreakdown: []dto.UserInventoryBreakdown{}, GroupBreakdown: []dto.UserInventoryBreakdown{}, SiteBreakdown: []dto.UserInventorySiteBreakdown{}, DataStatus: "complete"}, nil
}

type userInventoryRouteResolver struct{ mustChange bool }

func (resolver userInventoryRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled, MustChangePassword: resolver.mustChange}, nil
}

func TestUserInventoryRoutesRequireAuthenticationAndAllowViewerReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterUserInventoryRoutes(engine, controller.NewUserInventoryController(routerUserInventoryApplication{}), userInventoryRouteResolver{})
	for _, target := range []string{
		"/api/user-inventory?p=1&page_size=20",
		"/api/sites/2/user-inventory?p=1&page_size=20",
		"/api/user-inventory/statistics?start_timestamp=3600&end_timestamp=7200",
		"/api/sites/2/user-inventory/statistics?start_timestamp=3600&end_timestamp=7200",
	} {
		unauthorized := httptest.NewRecorder()
		engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, target, nil))
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s = %d", target, unauthorized.Code)
		}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("New-Api-User", "1")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("viewer %s = %d %s", target, response.Code, response.Body.String())
		}
	}
}
