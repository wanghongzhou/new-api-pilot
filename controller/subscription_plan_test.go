package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

type fakeSubscriptionPlanApplication struct{ query dto.SubscriptionPlanQuery }

func (application *fakeSubscriptionPlanApplication) List(_ context.Context, query dto.SubscriptionPlanQuery) (dto.SubscriptionPlanPageResponse, error) {
	application.query = query
	return dto.SubscriptionPlanPageResponse{Items: []dto.SubscriptionPlanItem{}, Page: query.Page, PageSize: query.PageSize, DataStatus: "pending"}, nil
}
func (application *fakeSubscriptionPlanApplication) Statistics(_ context.Context, query dto.SubscriptionPlanQuery) (dto.SubscriptionPlanStatistics, error) {
	application.query = query
	return dto.SubscriptionPlanStatistics{Total: "0", Enabled: "0", Disabled: "0", Missing: "0", DataStatus: "pending", SiteBreakdown: []dto.SubscriptionPlanBreakdown{}}, nil
}

func TestSubscriptionPlanQueryRejectsUnknownKeysAndParsesEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeSubscriptionPlanApplication{}
	engine := gin.New()
	engine.GET("/plans", NewSubscriptionPlanController(application).Global)

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
}
