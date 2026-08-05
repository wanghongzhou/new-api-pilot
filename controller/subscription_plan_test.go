package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

type fakeSubscriptionPlanApplication struct {
	err    error
	query  dto.SubscriptionPlanQuery
	called bool
}

func (application *fakeSubscriptionPlanApplication) List(_ context.Context, query dto.SubscriptionPlanQuery) (dto.SubscriptionPlanPageResponse, error) {
	application.called = true
	application.query = query
	return dto.SubscriptionPlanPageResponse{Items: []dto.SubscriptionPlanItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: "pending"}, application.err
}
func (application *fakeSubscriptionPlanApplication) Statistics(_ context.Context, query dto.SubscriptionPlanQuery) (dto.SubscriptionPlanStatistics, error) {
	application.called = true
	application.query = query
	return dto.SubscriptionPlanStatistics{Total: "0", Enabled: "0", Disabled: "0", Missing: "0", DataStatus: "pending", SiteBreakdown: []dto.SubscriptionPlanBreakdown{}}, application.err
}

func TestSubscriptionPlanQueryValidatesBeforeCallingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeSubscriptionPlanApplication{}
	engine := gin.New()
	controller := NewSubscriptionPlanController(application)
	engine.GET("/plans", controller.Global)
	engine.GET("/statistics", controller.GlobalStatistics)
	engine.GET("/sites/:id/plans", controller.Site)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plans?keyword="+strings.Repeat("a", 129), nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || application.query.Page != 0 {
		t.Fatalf("validation status=%d query=%#v body=%s", response.Code, application.query, response.Body.String())
	}
}

func TestSubscriptionPlanServiceFailureIsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeSubscriptionPlanApplication{err: errors.New("database unavailable")}
	engine := gin.New()
	engine.GET("/plans", NewSubscriptionPlanController(application).Global)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plans", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("service error status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSubscriptionPlanQueryRejectsUnknownKeysAndParsesEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeSubscriptionPlanApplication{}
	engine := gin.New()
	controller := NewSubscriptionPlanController(application)
	engine.GET("/plans", controller.Global)
	engine.GET("/statistics", controller.GlobalStatistics)
	engine.GET("/sites/:id/plans", controller.Site)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plans?enabled=false&unknown=1", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/plans?enabled=false", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.query.Enabled == nil || *application.query.Enabled {
		t.Fatalf("enabled status=%d query=%#v body=%s", response.Code, application.query, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/plans?enabled=0", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid enabled status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/plans?site_ids=not-an-id", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid site_ids status=%d body=%s", response.Code, response.Body.String())
	}

	for _, target := range []string{"/plans?p=1&p=2", "/plans?site_ids=-1", "/sites/2/plans?site_ids=2", "/statistics?p=1", "/statistics?keyword=pro", "/statistics?enabled=true", "/statistics?states=normal"} {
		application.called = false
		response = httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusBadRequest || application.called {
			t.Fatalf("target=%s status=%d called=%v body=%s", target, response.Code, application.called, response.Body.String())
		}
	}
	application.called = false
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/plans", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest || application.called {
		t.Fatalf("non-empty body status=%d called=%v body=%s", response.Code, application.called, response.Body.String())
	}
}
