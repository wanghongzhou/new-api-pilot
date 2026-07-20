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

type routePerformanceHistoryApp struct{}

func (routePerformanceHistoryApp) List(_ context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryPage, error) {
	return dto.PerformanceHistoryPage{Items: []dto.PerformanceHistoryItem{}, Page: q.Page, PageSize: q.PageSize, DataStatus: "complete"}, nil
}
func (routePerformanceHistoryApp) Statistics(context.Context, dto.PerformanceHistoryQuery) (dto.PerformanceHistoryStatisticsResponse, error) {
	return dto.PerformanceHistoryStatisticsResponse{Trend: []dto.PerformanceHistoryItem{}, SiteBreakdown: []dto.PerformanceHistoryItem{}, DataStatus: "complete", AggregationStatus: "unavailable"}, nil
}

type routePerformanceResolver struct{}

func (routePerformanceResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled}, nil
}
func TestPerformanceHistoryRoutesAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	RegisterPerformanceHistoryRoutes(e, controller.NewPerformanceHistoryController(routePerformanceHistoryApp{}), routePerformanceResolver{})
	for _, target := range []string{"/api/performance-history?start_timestamp=100&end_timestamp=200&p=1&page_size=20", "/api/performance-history/statistics?start_timestamp=100&end_timestamp=200", "/api/sites/2/performance-history?start_timestamp=100&end_timestamp=200&p=1&page_size=20", "/api/sites/2/performance-history/statistics?start_timestamp=100&end_timestamp=200"} {
		r := httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, target, nil))
		if r.Code != 401 {
			t.Fatalf("unauthorized %s=%d", target, r.Code)
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("New-Api-User", "1")
		r = httptest.NewRecorder()
		e.ServeHTTP(r, req)
		if r.Code != 200 {
			t.Fatalf("viewer %s=%d %s", target, r.Code, r.Body.String())
		}
	}
}
