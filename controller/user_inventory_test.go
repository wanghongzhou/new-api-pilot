package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

type fakeUserInventoryApplication struct {
	listQuery  dto.UserInventoryQuery
	statsQuery dto.UserInventoryStatisticsQuery
}

func (application *fakeUserInventoryApplication) List(_ context.Context, query dto.UserInventoryQuery) (dto.UserInventoryPage, error) {
	application.listQuery = query
	return dto.UserInventoryPage{Items: []dto.UserInventoryItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: "complete"}, nil
}

func (application *fakeUserInventoryApplication) Statistics(_ context.Context, query dto.UserInventoryStatisticsQuery) (dto.UserInventoryStatisticsResponse, error) {
	application.statsQuery = query
	return dto.UserInventoryStatisticsResponse{Trend: []dto.UserInventoryTrendPoint{}, RoleBreakdown: []dto.UserInventoryBreakdown{}, StatusBreakdown: []dto.UserInventoryBreakdown{}, GroupBreakdown: []dto.UserInventoryBreakdown{}, SiteBreakdown: []dto.UserInventorySiteBreakdown{}, DataStatus: "complete"}, nil
}

func TestUserInventoryControllerParsesGlobalAndSiteQueriesStrictly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeUserInventoryApplication{}
	controller := NewUserInventoryController(application)
	engine := gin.New()
	engine.GET("/inventory", controller.Global)
	engine.GET("/inventory/statistics", controller.GlobalStatistics)
	engine.GET("/sites/:id/inventory", controller.Site)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/inventory?p=2&page_size=10&site_ids=3,4&remote_user_id=9007199254740993&roles=1,2&statuses=1&groups=vip&states=normal&keyword=alice&min_balance=-5&max_balance=9", nil))
	if response.Code != http.StatusOK || application.listQuery.Page != 2 || len(application.listQuery.SiteIDs) != 2 || application.listQuery.RemoteUserID == nil || *application.listQuery.RemoteUserID != 9007199254740993 || application.listQuery.MinBalance == nil || *application.listQuery.MinBalance != -5 {
		t.Fatalf("global inventory = %d %#v %s", response.Code, application.listQuery, response.Body.String())
	}

	site := httptest.NewRecorder()
	engine.ServeHTTP(site, httptest.NewRequest(http.MethodGet, "/sites/9/inventory?site_ids=3&p=1&page_size=20", nil))
	if site.Code != http.StatusOK || len(application.listQuery.SiteIDs) != 1 || application.listQuery.SiteIDs[0] != 9 {
		t.Fatalf("site inventory = %d %#v %s", site.Code, application.listQuery, site.Body.String())
	}

	stats := httptest.NewRecorder()
	engine.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/inventory/statistics?start_timestamp=3600&end_timestamp=7200&site_ids=3&roles=1&statuses=1&groups=vip", nil))
	if stats.Code != http.StatusOK || application.statsQuery.StartTimestamp != 3600 || len(application.statsQuery.SiteIDs) != 1 {
		t.Fatalf("inventory statistics = %d %#v %s", stats.Code, application.statsQuery, stats.Body.String())
	}

	for _, target := range []string{
		"/inventory?site_ids=bad&p=1&page_size=20",
		"/inventory?site_ids=-1&p=1&page_size=20",
		"/inventory?roles=admin&p=1&page_size=20",
		"/inventory?remote_user_id=01&p=1&page_size=20",
		"/inventory?states=unknown&p=1&page_size=20",
		"/inventory?min_balance=01&p=1&page_size=20",
		"/inventory/statistics?start_timestamp=3601&end_timestamp=7200",
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid inventory query %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
}
