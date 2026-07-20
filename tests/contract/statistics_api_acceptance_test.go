package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/router"
)

type coreContractStatisticsApplication struct {
	last dto.StatisticsQuery
}

func (application *coreContractStatisticsApplication) Global(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeGlobal, query), nil
}

func (application *coreContractStatisticsApplication) Sites(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeSite, query), nil
}

func (application *coreContractStatisticsApplication) SiteStatistics(_ context.Context, id int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	query.SiteIDs = []int64{id}
	return application.Sites(context.Background(), query)
}

func (application *coreContractStatisticsApplication) Customers(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeCustomer, query), nil
}

func (application *coreContractStatisticsApplication) Accounts(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeAccount, query), nil
}

func (application *coreContractStatisticsApplication) Models(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeModel, query), nil
}

func (application *coreContractStatisticsApplication) Channels(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeChannel, query), nil
}

func (application *coreContractStatisticsApplication) Groups(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeGroup, query), nil
}

func (application *coreContractStatisticsApplication) Tokens(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeToken, query), nil
}

func (application *coreContractStatisticsApplication) Nodes(_ context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.last = query
	return coreContractStatisticsResponse(dto.StatisticsScopeNode, query), nil
}

func (application *coreContractStatisticsApplication) ModelOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.ModelOption], error) {
	return common.NewPageData(1, 20, 1, []dto.ModelOption{{Key: "9007199254740993:Model-A", SiteID: "9007199254740993", ModelName: "Model-A"}}), nil
}

func (application *coreContractStatisticsApplication) ChannelOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.ChannelOption], error) {
	return common.NewPageData(1, 20, 1, []dto.ChannelOption{{
		Key: "9007199254740993:0", SiteID: "9007199254740993", RemoteChannelID: "0", Name: "unknown",
	}}), nil
}

func (application *coreContractStatisticsApplication) GroupOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.GroupOption], error) {
	return common.NewPageData(1, 20, 1, []dto.GroupOption{{
		Key: "9007199254740993:default", SiteID: "9007199254740993", UseGroup: "default",
	}}), nil
}

func (application *coreContractStatisticsApplication) TokenOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.TokenOption], error) {
	return common.NewPageData(1, 20, 1, []dto.TokenOption{{
		Key: "9007199254740993:9007199254740995", SiteID: "9007199254740993",
		TokenID: "9007199254740995", TokenName: "fixture-token",
	}}), nil
}

func (application *coreContractStatisticsApplication) NodeOptions(context.Context, dto.StatisticsOptionQuery) (common.PageData[dto.NodeOption], error) {
	return common.NewPageData(1, 20, 1, []dto.NodeOption{{
		Key: "9007199254740993:node-a", SiteID: "9007199254740993", NodeName: "node-a",
	}}), nil
}

func newCoreContractStatisticsEngine(t *testing.T, application *coreContractStatisticsApplication) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine, err := router.New(router.Options{
		Config:               config.Config{AppEnv: config.EnvironmentTest},
		StatisticsController: controller.NewStatisticsController(application),
		IdentityResolver:     coreContractIdentityResolver{role: constant.RoleViewer},
	})
	if err != nil {
		t.Fatalf("create statistics contract router: %v", err)
	}
	return engine
}

func coreContractStatisticsResponse(scope string, query dto.StatisticsQuery) dto.StatisticsResponse {
	requestCount := "9007199254740993"
	quota := "9007199254740994"
	tokens := "9007199254740995"
	active := "9007199254740996"
	firstSiteID, secondSiteID := "9007199254740993", "9007199254740994"
	rate := "1.2500000000"
	firstQuota, secondQuota := "100", "200"
	siteBreakdown := []dto.SiteQuotaBreakdown{
		{SiteID: firstSiteID, SiteName: "First", Quota: &firstQuota, QuotaPerUnit: &rate, USDExchangeRate: &rate, RateSource: "site", DataStatus: "complete"},
		{SiteID: secondSiteID, SiteName: "Second", Quota: &secondQuota, QuotaPerUnit: &rate, USDExchangeRate: &rate, RateSource: "site", DataStatus: "complete"},
	}
	first := dto.TrendPoint{
		BucketStart: query.StartTimestamp, BucketEnd: query.StartTimestamp + 3600,
		RequestCount: &requestCount, Quota: &quota, TokenUsed: &tokens, ActiveUsers: &active,
		DataStatus: "complete", IsFinal: true, CompleteSiteCount: 2, ExpectedSiteCount: 2, SiteBreakdown: siteBreakdown,
	}
	second := first
	second.BucketStart = first.BucketEnd
	second.BucketEnd = query.EndTimestamp
	thirdQuota := "300"
	second.SiteBreakdown = append([]dto.SiteQuotaBreakdown(nil), siteBreakdown...)
	second.SiteBreakdown[0].Quota = &thirdQuota
	breakdown := []dto.StatisticsBreakdownItem{dto.GlobalStatisticsBreakdown{
		StatisticsBreakdownBase: dto.StatisticsBreakdownBase{
			DimensionID: "global", DimensionName: "Global", BucketStart: first.BucketStart, BucketEnd: first.BucketEnd,
			RequestCount: &requestCount, Quota: &quota, TokenUsed: &tokens, ActiveUsers: &active,
			DataStatus: "complete", IsFinal: true, SiteBreakdown: siteBreakdown, CompletenessRate: 1,
		},
		DimensionType: dto.StatisticsScopeGlobal, CompleteSiteCount: 2, ExpectedSiteCount: 2,
	}}
	return dto.StatisticsResponse{
		Scope: scope, Granularity: query.Granularity,
		Range:     dto.StatisticsRange{StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp, Timezone: "Asia/Shanghai", AsOf: query.EndTimestamp},
		Summary:   dto.StatisticsSummary{RequestCount: &requestCount, Quota: &quota, TokenUsed: &tokens, ActiveUsers: &active, DataStatus: "complete"},
		Trend:     []dto.TrendPoint{first, second},
		Breakdown: common.NewPageData(1, 20, 1, breakdown), SiteBreakdown: siteBreakdown,
		Completeness: dto.Completeness{DataStatus: "complete", CompleteSiteCount: 2, ExpectedSiteCount: 2, UnitType: "site", CompleteUnitCount: 2, ExpectedUnitCount: 2, CompletenessRate: 1, MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{}},
	}
}

func TestA13A40A64A82StatisticsAPIContract(t *testing.T) {
	application := &coreContractStatisticsApplication{}
	handler := newCoreContractStatisticsEngine(t, application)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, location).Unix()
	end := start + 2*3600
	target := "/api/statistics/global?start_timestamp=" + strconv.FormatInt(start, 10) +
		"&end_timestamp=" + strconv.FormatInt(end, 10) + "&granularity=hour&p=1&page_size=20&sort_by=bucket_start&sort_order=asc"
	response := coreContractRequest(handler, http.MethodGet, target, "")
	envelope := decodeCoreContractEnvelope(t, response)
	if response.Code != http.StatusOK || !envelope.Success || application.last.StartTimestamp != start || application.last.EndTimestamp != end {
		t.Fatalf("statistics API response=%d %#v query=%#v", response.Code, envelope, application.last)
	}
	var data struct {
		Summary map[string]any `json:"summary"`
		Trend   []struct {
			RequestCount  any `json:"request_count"`
			Quota         any `json:"quota"`
			TokenUsed     any `json:"token_used"`
			ActiveUsers   any `json:"active_users"`
			SiteBreakdown []struct {
				SiteID string `json:"site_id"`
				Quota  string `json:"quota"`
			} `json:"site_breakdown"`
		} `json:"trend"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode statistics API response: %v", err)
	}
	for _, field := range []string{"request_count", "quota", "token_used", "active_users"} {
		if _, ok := data.Summary[field].(string); !ok {
			t.Fatalf("A13 bigint statistics summary field %s is not a string: %#v", field, data.Summary)
		}
	}
	if len(data.Trend) != 2 || len(data.Trend[0].SiteBreakdown) != 2 || len(data.Trend[1].SiteBreakdown) != 2 ||
		data.Trend[0].SiteBreakdown[0].Quota == data.Trend[1].SiteBreakdown[0].Quota {
		t.Fatalf("A64 per-bucket site breakdown contract = %#v", data.Trend)
	}
	for _, point := range data.Trend {
		for _, value := range []any{point.RequestCount, point.Quota, point.TokenUsed, point.ActiveUsers} {
			if _, ok := value.(string); !ok {
				t.Fatalf("A13 bigint trend metric is not a string: %#v", point)
			}
		}
	}

	for _, invalid := range []string{
		"/api/statistics/global?start_timestamp=" + strconv.FormatInt(start+1, 10) + "&end_timestamp=" + strconv.FormatInt(end, 10) + "&granularity=hour",
		"/api/statistics/global?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(start+31*24*3600+3600, 10) + "&granularity=hour",
		"/api/statistics/channels?start_timestamp=" + strconv.FormatInt(start, 10) + "&end_timestamp=" + strconv.FormatInt(end, 10) + "&granularity=hour&channel_keys=7:01",
	} {
		invalidResponse := coreContractRequest(handler, http.MethodGet, invalid, "")
		invalidEnvelope := decodeCoreContractEnvelope(t, invalidResponse)
		if invalidResponse.Code != http.StatusBadRequest || invalidEnvelope.Code != constant.CodeValidationError || len(invalidEnvelope.FieldErrors) == 0 {
			t.Fatalf("A82 invalid statistics query response=%d %#v", invalidResponse.Code, invalidEnvelope)
		}
	}
}
