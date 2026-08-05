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

type fakeModelCatalogApplication struct {
	err    error
	query  dto.ModelCatalogQuery
	called bool
}

func (application *fakeModelCatalogApplication) List(_ context.Context, query dto.ModelCatalogQuery) (dto.ModelCatalogPageResponse, error) {
	application.called, application.query = true, query
	return dto.ModelCatalogPageResponse{}, application.err
}
func (application *fakeModelCatalogApplication) Missing(_ context.Context, query dto.ModelCatalogQuery) (dto.MissingModelPageResponse, error) {
	application.called, application.query = true, query
	return dto.MissingModelPageResponse{}, application.err
}
func (application *fakeModelCatalogApplication) Coverage(_ context.Context, query dto.ModelCatalogQuery) (dto.ModelCoverageResponse, error) {
	application.called, application.query = true, query
	return dto.ModelCoverageResponse{}, application.err
}

func TestModelCatalogRejectsUnsupportedViewFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeModelCatalogApplication{}
	engine := gin.New()
	controller := NewModelCatalogController(application)
	engine.GET("/catalog", controller.Global)
	engine.GET("/missing", controller.GlobalMissing)
	engine.GET("/coverage", controller.GlobalCoverage)
	engine.GET("/sites/:id/catalog", controller.Site)

	for _, path := range []string{"/missing?statuses=1", "/coverage?site_ids=1", "/coverage?keyword=model", "/coverage?p=1", "/catalog?p=1&p=2", "/catalog?site_ids=-1", "/sites/2/catalog?site_ids=2"} {
		response := httptest.NewRecorder()
		application.called = false
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || application.called {
			t.Fatalf("path=%s status=%d called=%v body=%s", path, response.Code, application.called, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	application.called = false
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/catalog", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest || application.called {
		t.Fatalf("non-empty body status=%d called=%v body=%s", response.Code, application.called, response.Body.String())
	}
}

func TestModelCatalogServiceFailureIsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeModelCatalogApplication{err: errors.New("database unavailable")}
	engine := gin.New()
	engine.GET("/catalog", NewModelCatalogController(application).Global)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
