package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
)

type dashboardRouteApplication struct{}

func (dashboardRouteApplication) Summary(context.Context) (dto.DashboardSummary, error) {
	return dto.DashboardSummary{
		Today:                dto.DashboardUsageSummary{SiteBreakdown: []dto.SiteQuotaBreakdown{}},
		ResourceStaleSiteIDs: []string{}, StaleSiteIDs: []string{},
	}, nil
}

func (dashboardRouteApplication) Trend(context.Context, dto.DashboardTrendQuery) ([]dto.TrendPoint, error) {
	return []dto.TrendPoint{}, nil
}

func (dashboardRouteApplication) Top(context.Context, dto.DashboardTopQuery) ([]dto.DashboardRankingItem, error) {
	return []dto.DashboardRankingItem{}, nil
}

func (dashboardRouteApplication) Health(context.Context) (dto.DashboardHealth, error) {
	return dto.DashboardHealth{
		AuthExpiredSiteIDs: []string{}, StatisticsNotReadySiteIDs: []string{},
		LatestAlerts: []dto.AlertEventItem{}, Sites: []dto.DashboardSiteHealthItem{},
		Completeness: dto.Completeness{MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{}},
	}, nil
}

type dashboardRouteResolver struct {
	mustChange bool
}

func (resolver dashboardRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{
		ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled,
		MustChangePassword: resolver.mustChange,
	}, nil
}

func TestDashboardFeatureRoutesAllowViewerReadsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterDashboardRoutes(
		engine,
		controller.NewDashboardController(dashboardRouteApplication{}),
		dashboardRouteResolver{},
	)
	targets := []string{
		"/api/dashboard/summary",
		"/api/dashboard/trend?days=30",
		"/api/dashboard/top?type=site&metric=request_count&limit=5",
		"/api/dashboard/health",
	}
	for _, target := range targets {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("New-Api-User", "1")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("viewer dashboard %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}

	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, targets[0], nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized dashboard = %d %s", unauthorized.Code, unauthorized.Body.String())
	}

	write := httptest.NewRecorder()
	writeRequest := httptest.NewRequest(http.MethodPost, "/api/dashboard/summary", nil)
	writeRequest.Header.Set("New-Api-User", "1")
	engine.ServeHTTP(write, writeRequest)
	if write.Code != http.StatusNotFound {
		t.Fatalf("dashboard write route = %d %s", write.Code, write.Body.String())
	}

	passwordEngine := gin.New()
	RegisterDashboardRoutes(
		passwordEngine,
		controller.NewDashboardController(dashboardRouteApplication{}),
		dashboardRouteResolver{mustChange: true},
	)
	blockedRequest := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	blockedRequest.Header.Set("New-Api-User", "1")
	blocked := httptest.NewRecorder()
	passwordEngine.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("password-gated dashboard = %d %s", blocked.Code, blocked.Body.String())
	}
}
