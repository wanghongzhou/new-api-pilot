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
}

func (application *fakePricingCatalogApplication) List(context.Context, dto.PricingCatalogQuery) (dto.PricingCatalogPageResponse, error) {
	application.called = true
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
