package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
)

type fakeStatisticsApplication struct{}

func (application *fakeStatisticsApplication) response(scope string, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return dto.StatisticsResponse{
		Scope:         scope,
		Granularity:   query.Granularity,
		Trend:         []dto.TrendPoint{},
		Breakdown:     common.NewPageData(query.Page, query.PageSize, 0, []dto.StatisticsBreakdownItem{}),
		SiteBreakdown: []dto.SiteQuotaBreakdown{},
		Completeness: dto.Completeness{
			MissingSiteIDs: []string{},
			MissingRanges:  []dto.MissingRange{},
		},
	}, nil
}

func (application *fakeStatisticsApplication) Global(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeGlobal, query)
}

func (application *fakeStatisticsApplication) Sites(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeSite, query)
}

func (application *fakeStatisticsApplication) SiteStatistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeSite, query)
}

func (application *fakeStatisticsApplication) Customers(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeCustomer, query)
}

func (application *fakeStatisticsApplication) Accounts(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeAccount, query)
}

func (application *fakeStatisticsApplication) Models(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeModel, query)
}

func (application *fakeStatisticsApplication) Channels(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeChannel, query)
}
func (application *fakeStatisticsApplication) Groups(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeGroup, query)
}
func (application *fakeStatisticsApplication) Tokens(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeToken, query)
}
func (application *fakeStatisticsApplication) Nodes(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeNode, query)
}

func (application *fakeStatisticsApplication) ModelOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.ModelOption], error) {
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.ModelOption{}), nil
}

func (application *fakeStatisticsApplication) ChannelOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.ChannelOption], error) {
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.ChannelOption{}), nil
}
func (application *fakeStatisticsApplication) GroupOptions(_ context.Context, _ dto.StatisticsOptionQuery) (common.PageData[dto.GroupOption], error) {
	return common.NewPageData(1, 20, 0, []dto.GroupOption{}), nil
}
func (application *fakeStatisticsApplication) TokenOptions(_ context.Context, _ dto.StatisticsOptionQuery) (common.PageData[dto.TokenOption], error) {
	return common.NewPageData(1, 20, 0, []dto.TokenOption{}), nil
}
func (application *fakeStatisticsApplication) NodeOptions(_ context.Context, _ dto.StatisticsOptionQuery) (common.PageData[dto.NodeOption], error) {
	return common.NewPageData(1, 20, 0, []dto.NodeOption{}), nil
}

type statisticsRouteResolver struct {
	mustChange bool
}

func (resolver statisticsRouteResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{
		ID: "1", Role: constant.RoleViewer, Status: constant.UserStatusEnabled,
		MustChangePassword: resolver.mustChange,
	}, nil
}

func TestStatisticsFeatureRoutesAllowViewerReadsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeStatisticsApplication{}
	engine := gin.New()
	engine.Use(middleware.RequestID())
	RegisterStatisticsRoutes(engine, controller.NewStatisticsController(application), statisticsRouteResolver{})
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix()
	query := "?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" +
		strconv.FormatInt(start+3600, 10) + "&granularity=hour&p=1&page_size=20&sort_by=bucket_start&sort_order=asc"
	for _, scope := range []string{"global", "sites", "customers", "accounts", "models", "channels", "groups", "tokens", "nodes"} {
		request := httptest.NewRequest(http.MethodGet, "/api/statistics/"+scope+query, nil)
		request.Header.Set("New-Api-User", "1")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("viewer statistics %s = %d %s", scope, recorder.Code, recorder.Body.String())
		}
	}
	for _, target := range []string{
		"/api/statistics/options/models?site_ids=2,2&p=1&page_size=20",
		"/api/statistics/options/channels?keyword=Primary&p=1&page_size=20",
		"/api/statistics/options/groups?keyword=default&p=1&page_size=20",
		"/api/statistics/options/tokens?keyword=key&p=1&page_size=20",
		"/api/statistics/options/nodes?keyword=node&p=1&page_size=20",
		"/api/sites/7/stats" + query,
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("New-Api-User", "1")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("viewer statistics route %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
	overflow := httptest.NewRecorder()
	overflowRequest := httptest.NewRequest(http.MethodGet, "/api/statistics/global?start_timestamp="+
		strconv.FormatInt(start, 10)+"&end_timestamp="+strconv.FormatInt(start+3600, 10)+
		"&granularity=hour&p=9223372036854775807&page_size=100", nil)
	overflowRequest.Header.Set("New-Api-User", "1")
	engine.ServeHTTP(overflow, overflowRequest)
	if overflow.Code != http.StatusBadRequest || !strings.Contains(overflow.Body.String(), `"p"`) ||
		!strings.Contains(overflow.Body.String(), `"request_id":"`) {
		t.Fatalf("overflow statistics page = %d %s", overflow.Code, overflow.Body.String())
	}
	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/statistics/global"+query, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized statistics = %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	write := httptest.NewRecorder()
	writeRequest := httptest.NewRequest(http.MethodPost, "/api/statistics/global"+query, nil)
	writeRequest.Header.Set("New-Api-User", "1")
	engine.ServeHTTP(write, writeRequest)
	if write.Code != http.StatusNotFound {
		t.Fatalf("statistics write route = %d %s", write.Code, write.Body.String())
	}

	passwordEngine := gin.New()
	passwordEngine.Use(middleware.RequestID())
	RegisterStatisticsRoutes(passwordEngine, controller.NewStatisticsController(application), statisticsRouteResolver{mustChange: true})
	blockedRequest := httptest.NewRequest(http.MethodGet, "/api/statistics/global"+query, nil)
	blockedRequest.Header.Set("New-Api-User", "1")
	blocked := httptest.NewRecorder()
	passwordEngine.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("password-gated statistics = %d %s", blocked.Code, blocked.Body.String())
	}
}
