package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

type fakeDashboardApplication struct {
	trendQuery dto.DashboardTrendQuery
	topQuery   dto.DashboardTopQuery
	fail       string
}

func (application *fakeDashboardApplication) Summary(context.Context) (dto.DashboardSummary, error) {
	if application.fail == "summary" {
		return dto.DashboardSummary{}, errors.New("summary failed")
	}
	return dto.DashboardSummary{
		Today:                dto.DashboardUsageSummary{SiteBreakdown: []dto.SiteQuotaBreakdown{}},
		ResourceStaleSiteIDs: []string{}, StaleSiteIDs: []string{},
	}, nil
}

func (application *fakeDashboardApplication) Trend(
	_ context.Context,
	query dto.DashboardTrendQuery,
) ([]dto.TrendPoint, error) {
	application.trendQuery = query
	if application.fail == "trend" {
		return nil, errors.New("trend failed")
	}
	return []dto.TrendPoint{}, nil
}

func (application *fakeDashboardApplication) Top(
	_ context.Context,
	query dto.DashboardTopQuery,
) ([]dto.DashboardRankingItem, error) {
	application.topQuery = query
	if application.fail == "top" {
		return nil, errors.New("top failed")
	}
	return []dto.DashboardRankingItem{}, nil
}

func (application *fakeDashboardApplication) Health(context.Context) (dto.DashboardHealth, error) {
	if application.fail == "health" {
		return dto.DashboardHealth{}, errors.New("health failed")
	}
	return dto.DashboardHealth{
		AuthExpiredSiteIDs: []string{}, StatisticsNotReadySiteIDs: []string{},
		LatestAlerts: []dto.AlertEventItem{}, Sites: []dto.DashboardSiteHealthItem{},
		Completeness: dto.Completeness{MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{}},
	}, nil
}

func TestDashboardControllerStrictQueriesAndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeDashboardApplication{}
	controller := NewDashboardController(application)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(constant.ContextRequestID, "req_dashboard_controller")
		c.Next()
	})
	engine.GET("/summary", controller.Summary)
	engine.GET("/trend", controller.Trend)
	engine.GET("/top", controller.Top)
	engine.GET("/health", controller.Health)

	valid := []string{
		"/summary",
		"/trend?days=30",
		"/top?type=site&metric=quota&limit=5",
		"/top?type=customer&metric=request_count&limit=5",
		"/top?type=model&metric=quota&limit=5",
		"/top?type=channel&metric=request_count&limit=20",
		"/health",
	}
	for _, target := range valid {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("valid dashboard request %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
		var envelope common.APIResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || !envelope.Success ||
			envelope.RequestID != "req_dashboard_controller" {
			t.Fatalf("dashboard envelope %s = %#v, %v", target, envelope, err)
		}
	}
	if application.trendQuery.Days != 30 || application.topQuery.Type != dto.DashboardTopTypeChannel ||
		application.topQuery.Metric != dto.DashboardTopMetricRequestCount || application.topQuery.Limit != 20 {
		t.Fatalf("parsed dashboard queries = trend:%#v top:%#v", application.trendQuery, application.topQuery)
	}

	invalid := []string{
		"/summary?extra=1",
		"/trend?days=0",
		"/trend?days=91",
		"/trend?days=30&days=31",
		"/top?type=account&metric=quota&limit=5",
		"/top?type=customer&metric=token_used&limit=5",
		"/top?type=customer&metric=quota&limit=0",
		"/top?type=customer&metric=quota&limit=21",
		"/top?type=customer&type=site&metric=quota",
		"/health?extra=1",
	}
	for _, target := range invalid {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), constant.CodeValidationError) {
			t.Fatalf("invalid dashboard request %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDashboardControllerEndpointFailureDoesNotChangeOtherHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeDashboardApplication{fail: "top"}
	controller := NewDashboardController(application)
	engine := gin.New()
	engine.GET("/summary", controller.Summary)
	engine.GET("/top", controller.Top)
	engine.GET("/health", controller.Health)

	for target, expected := range map[string]int{
		"/summary": http.StatusOK,
		"/top?type=customer&metric=quota&limit=5": http.StatusInternalServerError,
		"/health": http.StatusOK,
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != expected {
			t.Fatalf("isolated dashboard endpoint %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDashboardControllerMissingApplicationDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, controller := range map[string]*DashboardController{
		"nil controller":  nil,
		"nil application": NewDashboardController(nil),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/summary", nil)
			controller.Summary(c)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("missing dashboard application = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
