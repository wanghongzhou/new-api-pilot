package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

type fakeLogApplication struct{ query dto.LogQuery }

func (application *fakeLogApplication) Query(_ context.Context, query dto.LogQuery) (dto.LogResponse, error) {
	application.query = query
	return dto.LogResponse{Items: []dto.LogItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: dto.LogCollectionComplete}, nil
}

func TestLogControllerGlobalAndSiteQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeLogApplication{}
	controller := NewLogController(application)
	engine := gin.New()
	engine.GET("/logs", controller.Global)
	engine.GET("/sites/:id/logs", controller.Site)
	base := "?start_timestamp=100&end_timestamp=200&type=2&channel_id=0&username=alice&model_name=gpt&token_name=key&group=vip&request_id=req&upstream_request_id=up&p=2&page_size=10"

	global := httptest.NewRecorder()
	engine.ServeHTTP(global, httptest.NewRequest(http.MethodGet, "/logs"+base+"&site_ids=3,4", nil))
	if global.Code != http.StatusOK || application.query.Page != 2 || len(application.query.SiteIDs) != 2 || application.query.Type == nil || *application.query.Type != 2 {
		t.Fatalf("global log query = %d %#v %s", global.Code, application.query, global.Body.String())
	}

	site := httptest.NewRecorder()
	engine.ServeHTTP(site, httptest.NewRequest(http.MethodGet, "/sites/9/logs"+base, nil))
	if site.Code != http.StatusOK || len(application.query.SiteIDs) != 1 || application.query.SiteIDs[0] != 9 {
		t.Fatalf("site log query = %d %#v %s", site.Code, application.query, site.Body.String())
	}

	invalid := httptest.NewRecorder()
	engine.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/logs?start_timestamp=100&end_timestamp="+strconv.FormatInt(100+32*24*3600, 10), nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid log query = %d %s", invalid.Code, invalid.Body.String())
	}
	for _, target := range []string{
		"/logs?start_timestamp=100&end_timestamp=200&p=bad&page_size=20",
		"/logs?start_timestamp=100&end_timestamp=200&p=1&page_size=bad",
		"/logs?start_timestamp=100&end_timestamp=200&p=1&page_size=20&site_ids=0",
		"/logs?start_timestamp=100&end_timestamp=200&p=1&page_size=20&site_ids=01",
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid log query %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
}
