package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type fakeStatisticsApplication struct {
	scope       string
	query       dto.StatisticsQuery
	optionQuery dto.StatisticsOptionQuery
	siteError   error
}

func (application *fakeStatisticsApplication) response(scope string, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.scope = scope
	application.query = query
	return dto.StatisticsResponse{
		Scope: scope, Granularity: query.Granularity, Trend: []dto.TrendPoint{},
		Breakdown:     common.NewPageData(query.Page, query.PageSize, 0, []dto.StatisticsBreakdownItem{}),
		SiteBreakdown: []dto.SiteQuotaBreakdown{},
		Completeness:  dto.Completeness{MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{}},
	}, nil
}

func (application *fakeStatisticsApplication) Global(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeGlobal, query)
}
func (application *fakeStatisticsApplication) Sites(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return application.response(dto.StatisticsScopeSite, query)
}
func (application *fakeStatisticsApplication) SiteStatistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	if application.siteError != nil {
		return dto.StatisticsResponse{}, application.siteError
	}
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
	application.optionQuery = query
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.ModelOption{}), nil
}
func (application *fakeStatisticsApplication) ChannelOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.ChannelOption], error) {
	application.optionQuery = query
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.ChannelOption{}), nil
}
func (application *fakeStatisticsApplication) GroupOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.GroupOption], error) {
	application.optionQuery = query
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.GroupOption{}), nil
}
func (application *fakeStatisticsApplication) TokenOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.TokenOption], error) {
	application.optionQuery = query
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.TokenOption{}), nil
}
func (application *fakeStatisticsApplication) NodeOptions(_ context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.NodeOption], error) {
	application.optionQuery = query
	return common.NewPageData(query.Page, query.PageSize, 0, []dto.NodeOption{}), nil
}

func TestStatisticsControllerStrictQueryAndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &fakeStatisticsApplication{}
	controller := NewStatisticsController(application)
	engine := gin.New()
	engine.Use(middlewareRequestIDForStatisticsTest())
	engine.GET("/models", controller.Models)
	engine.GET("/customers", controller.Customers)
	engine.GET("/channels", controller.Channels)
	engine.GET("/groups", controller.Groups)
	engine.GET("/tokens", controller.Tokens)
	engine.GET("/nodes", controller.Nodes)
	engine.GET("/options/models", controller.ModelOptions)
	engine.GET("/options/groups", controller.GroupOptions)
	engine.GET("/options/tokens", controller.TokenOptions)
	engine.GET("/options/nodes", controller.NodeOptions)
	engine.GET("/sites/:id/stats", controller.Site)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix()

	valid := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/models?start_timestamp="+strconv.FormatInt(start, 10)+"&end_timestamp="+strconv.FormatInt(start+3600, 10)+
			"&granularity=hour&site_ids=2,3&model_names=Model-A&model_names=model-a&p=1&page_size=50&sort_by=quota&sort_order=asc", nil)
	engine.ServeHTTP(valid, request)
	if valid.Code != http.StatusOK || application.scope != dto.StatisticsScopeModel ||
		len(application.query.ModelNames) != 2 || application.query.SortBy != "quota" {
		t.Fatalf("valid statistics controller = %d scope=%s query=%#v body=%s",
			valid.Code, application.scope, application.query, valid.Body.String())
	}
	for _, test := range []struct {
		target string
		scope  string
	}{
		{target: "/groups?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour&use_groups=default", scope: dto.StatisticsScopeGroup},
		{target: "/tokens?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour&token_keys=2:9", scope: dto.StatisticsScopeToken},
		{target: "/nodes?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour&node_names=node-a", scope: dto.StatisticsScopeNode},
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.target, nil))
		if recorder.Code != http.StatusOK || application.scope != test.scope {
			t.Fatalf("flow statistics %s = %d scope=%s body=%s", test.scope, recorder.Code, application.scope, recorder.Body.String())
		}
	}
	var envelope common.APIResponse
	if err := json.Unmarshal(valid.Body.Bytes(), &envelope); err != nil || !envelope.Success {
		t.Fatalf("statistics envelope = %#v, %v", envelope, err)
	}
	options := httptest.NewRecorder()
	engine.ServeHTTP(options, httptest.NewRequest(http.MethodGet,
		"/options/models?keyword=Model&site_ids=2,2,3&p=1&page_size=50", nil))
	if options.Code != http.StatusOK || len(application.optionQuery.SiteIDs) != 2 ||
		application.optionQuery.SiteIDs[0] != 2 || application.optionQuery.SiteIDs[1] != 3 {
		t.Fatalf("statistics options = %d query=%#v body=%s", options.Code, application.optionQuery, options.Body.String())
	}
	for _, target := range []string{
		"/options/groups?keyword=default&site_ids=2&p=2&page_size=10",
		"/options/tokens?keyword=key&site_ids=2&p=2&page_size=10",
		"/options/nodes?keyword=node&site_ids=2&p=2&page_size=10",
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK || application.optionQuery.Page != 2 || application.optionQuery.PageSize != 10 || application.optionQuery.SiteIDs[0] != 2 {
			t.Fatalf("flow options %s = %d query=%#v body=%s", target, recorder.Code, application.optionQuery, recorder.Body.String())
		}
	}
	application.siteError = service.ErrStatisticsNotFound
	notFound := httptest.NewRecorder()
	engine.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/sites/99/stats?start_timestamp="+
		strconv.FormatInt(start, 10)+"&end_timestamp="+strconv.FormatInt(start+3600, 10)+"&granularity=hour", nil))
	if notFound.Code != http.StatusNotFound || !strings.Contains(notFound.Body.String(), constant.CodeNotFound) {
		t.Fatalf("missing site statistics = %d %s", notFound.Code, notFound.Body.String())
	}
	application.siteError = nil

	invalid := []string{
		"/models?start_timestamp=" + strconv.FormatInt(start+1, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour",
		"/customers?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour&account_ids=1",
		"/channels?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour&channel_keys=1:01",
		"/models?start_timestamp=" + strconv.FormatInt(start, 10) + "&start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+3600, 10) + "&granularity=hour",
		"/options/models?p=9223372036854775807&page_size=100",
	}
	for _, target := range invalid {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), constant.CodeValidationError) {
			t.Fatalf("invalid statistics query %s = %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestStatisticsControllerRejectsMissingApplicationWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, controller := range map[string]*StatisticsController{
		"nil controller":  nil,
		"nil application": NewStatisticsController(nil),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/statistics/global", nil)
			controller.Global(c)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("missing statistics application = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func middlewareRequestIDForStatisticsTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(constant.ContextRequestID, "req_statistics_controller")
		c.Next()
	}
}
