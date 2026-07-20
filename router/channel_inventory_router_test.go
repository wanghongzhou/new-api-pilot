package router

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"testing"
)

type routeChannelInventoryApp struct{}

func (routeChannelInventoryApp) List(_ context.Context, q dto.ChannelInventoryQuery) (dto.ChannelInventoryPage, error) {
	return dto.ChannelInventoryPage{Items: []dto.ChannelInventoryItem{}, Page: q.Page, PageSize: q.PageSize, DataStatus: "complete"}, nil
}
func (routeChannelInventoryApp) Statistics(context.Context, dto.ChannelInventoryStatisticsQuery) (dto.ChannelInventoryStatisticsResponse, error) {
	return dto.ChannelInventoryStatisticsResponse{Trend: []dto.ChannelInventoryTrendPoint{}, DataStatus: "complete"}, nil
}

type routeChannelInventoryResolver struct{}

func (routeChannelInventoryResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}
func TestChannelInventoryRoutesViewerAndAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	RegisterChannelInventoryRoutes(e, controller.NewChannelInventoryController(routeChannelInventoryApp{}), routeChannelInventoryResolver{})
	for _, target := range []string{"/api/channel-inventory?p=1&page_size=20", "/api/sites/2/channel-inventory?p=1&page_size=20", "/api/channel-inventory/statistics?start_timestamp=3600&end_timestamp=7200", "/api/sites/2/channel-inventory/statistics?start_timestamp=3600&end_timestamp=7200"} {
		r := httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, target, nil))
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s=%d", target, r.Code)
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("New-Api-User", "1")
		r = httptest.NewRecorder()
		e.ServeHTTP(r, req)
		if r.Code != http.StatusOK {
			t.Fatalf("viewer %s=%d %s", target, r.Code, r.Body.String())
		}
	}
}
