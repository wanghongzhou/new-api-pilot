package dto

import (
	"encoding/json"
	"testing"
	"time"

	"new-api-pilot/common"
)

func TestStatisticsBreakdownItemMarshalsConcreteTypedFields(t *testing.T) {
	siteID := "7"
	siteName := "Primary"
	requestCount := "9007199254740993"
	items := []StatisticsBreakdownItem{
		AccountStatisticsBreakdown{
			StatisticsBreakdownBase: StatisticsBreakdownBase{
				DimensionID: "88", DimensionName: "managed-user", SiteID: &siteID, SiteName: &siteName,
				BucketStart: 1710000000, BucketEnd: 1710003600, RequestCount: &requestCount,
				DataStatus: "complete", SiteBreakdown: []SiteQuotaBreakdown{}, CompletenessRate: 1,
			},
			DimensionType: "account", CustomerID: "9", CustomerName: "Customer", RemoteUserID: "100",
		},
	}
	response := StatisticsResponse{
		Scope: StatisticsScopeAccount, Granularity: StatisticsGranularityHour,
		Trend: []TrendPoint{}, Breakdown: common.NewPageData(1, 20, 1, items),
		SiteBreakdown: []SiteQuotaBreakdown{}, Completeness: Completeness{MissingSiteIDs: []string{}, MissingRanges: []MissingRange{}},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal statistics response: %v", err)
	}
	var decoded struct {
		Breakdown struct {
			Items []map[string]any `json:"items"`
		} `json:"breakdown"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode statistics response: %v", err)
	}
	if len(decoded.Breakdown.Items) != 1 {
		t.Fatalf("breakdown items = %#v", decoded.Breakdown.Items)
	}
	item := decoded.Breakdown.Items[0]
	if item["dimension_type"] != "account" || item["site_id"] != "7" || item["customer_id"] != "9" ||
		item["remote_user_id"] != "100" || item["request_count"] != requestCount {
		t.Fatalf("typed breakdown JSON = %#v", item)
	}
}

func TestStatisticsQueryValidationUsesBeijingBucketsAndScopeFilters(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix()
	query := StatisticsQuery{
		StartTimestamp: start, EndTimestamp: start + 24*3600, Granularity: StatisticsGranularityHour,
		SiteIDs: []int64{2, 2}, ModelNames: []string{"Model-A", "Model-A", "model-a"},
		Page: 1, PageSize: 100, SortBy: "active_users", SortOrder: "desc",
	}
	query.Normalize()
	if errors := query.Validate(StatisticsScopeModel); errors != nil {
		t.Fatalf("valid model statistics query errors = %#v", errors)
	}
	if len(query.SiteIDs) != 1 || len(query.ModelNames) != 2 || query.ModelNames[0] == query.ModelNames[1] {
		t.Fatalf("normalized statistics filters = %#v", query)
	}

	misaligned := query
	misaligned.StartTimestamp++
	if errors := misaligned.Validate(StatisticsScopeModel); errors == nil || errors["range"] == "" {
		t.Fatalf("misaligned statistics errors = %#v", errors)
	}
	overflow := query
	overflow.EndTimestamp = overflow.StartTimestamp + 31*24*3600 + 3600
	if errors := overflow.Validate(StatisticsScopeModel); errors == nil || errors["range"] == "" {
		t.Fatalf("overlong statistics errors = %#v", errors)
	}
	invalidChannel := query
	invalidChannel.ModelNames = nil
	invalidChannel.ChannelKeys = []string{"2:01"}
	if errors := invalidChannel.Validate(StatisticsScopeChannel); errors == nil || errors["channel_keys"] == "" {
		t.Fatalf("invalid channel key errors = %#v", errors)
	}
	wrongScope := query
	wrongScope.CustomerIDs = []int64{7}
	if errors := wrongScope.Validate(StatisticsScopeModel); errors == nil || errors["customer_ids"] == "" {
		t.Fatalf("wrong-scope statistics errors = %#v", errors)
	}

	overflowPage := query
	overflowPage.Page = int(^uint(0) >> 1)
	overflowPage.PageSize = 100
	if errors := overflowPage.Validate(StatisticsScopeModel); errors == nil || errors["p"] == "" || overflowPage.Offset() != 0 {
		t.Fatalf("overflow page errors = %#v offset=%d", errors, overflowPage.Offset())
	}
}

func TestStatisticsQueryExportValidationAllowsRetainedHistory(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	cases := []struct {
		name        string
		granularity string
		start       time.Time
		end         time.Time
		pageRejects bool
	}{
		{name: "hour", granularity: StatisticsGranularityHour, start: time.Date(2025, 1, 1, 0, 0, 0, 0, location), end: time.Date(2025, 2, 2, 0, 0, 0, 0, location), pageRejects: true},
		{name: "day", granularity: StatisticsGranularityDay, start: time.Date(2020, 1, 1, 0, 0, 0, 0, location), end: time.Date(2023, 1, 2, 0, 0, 0, 0, location), pageRejects: true},
		{name: "month", granularity: StatisticsGranularityMonth, start: time.Date(2000, 1, 1, 0, 0, 0, 0, location), end: time.Date(2021, 2, 1, 0, 0, 0, 0, location), pageRejects: true},
		{name: "year", granularity: StatisticsGranularityYear, start: time.Date(1971, 1, 1, 0, 0, 0, 0, location), end: time.Date(2026, 1, 1, 0, 0, 0, 0, location)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			query := StatisticsQuery{
				StartTimestamp: testCase.start.Unix(), EndTimestamp: testCase.end.Unix(),
				Granularity: testCase.granularity, Page: 1, PageSize: 100,
				SortBy: "bucket_start", SortOrder: "asc",
			}
			if fields := query.ValidateForExport(StatisticsScopeGlobal); fields != nil {
				t.Fatalf("export range rejected: %#v", fields)
			}
			if fields := query.Validate(StatisticsScopeGlobal); testCase.pageRejects && (fields == nil || fields["range"] == "") {
				t.Fatalf("interactive range unexpectedly accepted: %#v", fields)
			}
		})
	}
	misaligned := StatisticsQuery{
		StartTimestamp: cases[0].start.Unix() + 1, EndTimestamp: cases[0].end.Unix(),
		Granularity: StatisticsGranularityHour, Page: 1, PageSize: 100,
		SortBy: "bucket_start", SortOrder: "asc",
	}
	if fields := misaligned.ValidateForExport(StatisticsScopeGlobal); fields == nil || fields["range"] == "" {
		t.Fatalf("misaligned export range accepted: %#v", fields)
	}
	tooMany := StatisticsQuery{
		StartTimestamp: cases[0].start.Unix(),
		EndTimestamp:   cases[0].start.Unix() + (statisticsMaximumExportBuckets+1)*3600,
		Granularity:    StatisticsGranularityHour, Page: 1, PageSize: 100,
		SortBy: "bucket_start", SortOrder: "asc",
	}
	if fields := tooMany.ValidateForExport(StatisticsScopeGlobal); fields == nil || fields["range"] == "" {
		t.Fatalf("unsafe export bucket count accepted: %#v", fields)
	}
}

func TestStatisticsOptionQueryNormalizesStableSiteIDsAndBoundsOffset(t *testing.T) {
	query := StatisticsOptionQuery{Keyword: " Model ", SiteIDs: []int64{2, 2, 1}, Page: 2, PageSize: 20}
	query.Normalize()
	if errors := query.Validate(); errors != nil || query.Keyword != "Model" ||
		len(query.SiteIDs) != 2 || query.SiteIDs[0] != 2 || query.SiteIDs[1] != 1 || query.Offset() != 20 {
		t.Fatalf("normalized options query = %#v errors=%#v", query, errors)
	}
	query.Page = int(^uint(0) >> 1)
	query.PageSize = 100
	if errors := query.Validate(); errors == nil || errors["p"] == "" || query.Offset() != 0 {
		t.Fatalf("overflow options query errors = %#v offset=%d", errors, query.Offset())
	}
	for _, siteID := range []int64{0, -1} {
		invalid := StatisticsOptionQuery{SiteIDs: []int64{siteID}, Page: 1, PageSize: 20}
		invalid.Normalize()
		if errors := invalid.Validate(); errors == nil || errors["site_ids"] == "" {
			t.Fatalf("invalid option site ID %d errors = %#v", siteID, errors)
		}
	}
}

func TestFlowStatisticsScopeFilters(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	start := time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix()
	base := StatisticsQuery{
		StartTimestamp: start, EndTimestamp: start + 3600,
		Granularity: StatisticsGranularityHour, Page: 1, PageSize: 20,
		SortBy: "quota", SortOrder: "desc",
	}
	tests := []struct {
		scope string
		apply func(*StatisticsQuery)
	}{
		{scope: StatisticsScopeGroup, apply: func(query *StatisticsQuery) { query.UseGroups = []string{""} }},
		{scope: StatisticsScopeToken, apply: func(query *StatisticsQuery) { query.TokenKeys = []string{"2:0"} }},
		{scope: StatisticsScopeNode, apply: func(query *StatisticsQuery) { query.NodeNames = []string{""} }},
	}
	for _, test := range tests {
		query := base
		test.apply(&query)
		if fields := query.Validate(test.scope); fields != nil {
			t.Fatalf("valid %s flow filter = %#v", test.scope, fields)
		}
		if fields := query.Validate(StatisticsScopeGlobal); fields == nil {
			t.Fatalf("%s flow filter was accepted by global scope", test.scope)
		}
	}
}

func TestLogQueryValidation(t *testing.T) {
	channel := int64(0)
	logType := 2
	query := LogQuery{Page: 1, PageSize: 100, StartTimestamp: 100, EndTimestamp: 200,
		SiteIDs: []int64{2, 2}, Type: &logType, ChannelID: &channel, RequestID: "req"}
	query.Normalize()
	if fields := query.Validate(); fields != nil || len(query.SiteIDs) != 1 {
		t.Fatalf("valid log query = %#v %#v", query, fields)
	}
	query.EndTimestamp = query.StartTimestamp + 31*24*3600 + 1
	if fields := query.Validate(); fields == nil || fields["range"] == "" {
		t.Fatalf("overlong log query = %#v", fields)
	}
}
