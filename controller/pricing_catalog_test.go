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

type fakePricingCatalogApplication struct {
	err    error
	called bool
	query  dto.PricingCatalogQuery
}

func (application *fakePricingCatalogApplication) List(_ context.Context, query dto.PricingCatalogQuery) (dto.PricingCatalogPageResponse, error) {
	application.called = true
	application.query = query
	return dto.PricingCatalogPageResponse{}, application.err
}
func (application *fakePricingCatalogApplication) ListGroups(context.Context, dto.PricingCatalogQuery) (dto.PricingGroupPageResponse, error) {
	application.called = true
	return dto.PricingGroupPageResponse{}, application.err
}
func (application *fakePricingCatalogApplication) Statistics(context.Context, dto.PricingCatalogQuery) (dto.PricingCatalogStatistics, error) {
	application.called = true
	return dto.PricingCatalogStatistics{}, application.err
}

func TestPricingCatalogValidatesBeforeCallingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakePricingCatalogApplication{}
	engine := gin.New()
	engine.GET("/pricing", NewPricingCatalogController(application).Global)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing?keyword="+strings.Repeat("a", 256), nil))
	if response.Code != http.StatusBadRequest || application.called {
		t.Fatalf("status=%d called=%v body=%s", response.Code, application.called, response.Body.String())
	}
}

func TestPricingCatalogServiceFailureIsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakePricingCatalogApplication{err: errors.New("database unavailable")}
	engine := gin.New()
	engine.GET("/pricing", NewPricingCatalogController(application).Global)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPricingCatalogParsesBillingModeAndRejectsVendorDrilldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakePricingCatalogApplication{}
	engine := gin.New()
	controller := NewPricingCatalogController(application)
	engine.GET("/pricing", controller.Global)
	engine.GET("/statistics", controller.GlobalStatistics)
	engine.GET("/sites/:id/pricing", controller.Site)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing?billing_mode=tiered_expr", nil))
	if response.Code != http.StatusOK || application.query.BillingMode != "tiered_expr" {
		t.Fatalf("status=%d query=%#v body=%s", response.Code, application.query, response.Body.String())
	}

	application.called = false
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing?vendor_id=9", nil))
	if response.Code != http.StatusBadRequest || application.called {
		t.Fatalf("status=%d called=%v body=%s", response.Code, application.called, response.Body.String())
	}

	for _, target := range []string{"/pricing?p=1&p=2", "/pricing?site_ids=-1", "/sites/2/pricing?site_ids=2", "/statistics?p=1", "/statistics?states=normal"} {
		application.called = false
		response = httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusBadRequest || application.called {
			t.Fatalf("target=%s status=%d called=%v body=%s", target, response.Code, application.called, response.Body.String())
		}
	}
	application.called = false
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest || application.called {
		t.Fatalf("non-empty body status=%d called=%v body=%s", response.Code, application.called, response.Body.String())
	}
}
