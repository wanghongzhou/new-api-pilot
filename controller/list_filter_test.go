package controller

import (
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAlertListQueryAcceptsSingleRepeatedCommaAndJSONEnumArrays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jsonLevels := url.QueryEscape(`["critical","warning"]`)
	target := "/alerts?status=firing&status=pending,resolved&status=firing&level=" + jsonLevels +
		"&target_type=site&target_type=account&p=1&page_size=20"
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", target, nil)
	query, fields := parseAlertListQuery(context)
	if fields != nil || !reflect.DeepEqual(query.Statuses, []string{"firing", "pending", "resolved"}) ||
		!reflect.DeepEqual(query.Levels, []string{"critical", "warning"}) ||
		!reflect.DeepEqual(query.TargetTypes, []string{"site", "account"}) {
		t.Fatalf("parsed alert filters = %#v fields=%#v", query, fields)
	}

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/alerts?status=firing", nil)
	query, fields = parseAlertListQuery(context)
	if fields != nil || !reflect.DeepEqual(query.Statuses, []string{"firing"}) {
		t.Fatalf("single alert status = %#v fields=%#v", query, fields)
	}
}

func TestAlertRuleListQueryParsesFiltersPaginationAndSorting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := "/alert-rules?scope_type=site&scope_id=42&p=2&page_size=10" +
		"&category=instance&category=channel&level=warning&level=critical" +
		"&enabled=true&inherited=false&sort_by=metric&sort_order=desc"
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", target, nil)
	query, fields := parseAlertRuleListQuery(context)
	if fields != nil || query.ScopeType != "site" || query.ScopeID != 42 || query.Page != 2 || query.PageSize != 10 ||
		!reflect.DeepEqual(query.Categories, []string{"instance", "channel"}) ||
		!reflect.DeepEqual(query.Levels, []string{"warning", "critical"}) ||
		query.Enabled == nil || !*query.Enabled || query.Inherited == nil || *query.Inherited ||
		query.SortBy != "metric" || query.SortOrder != "desc" {
		t.Fatalf("parsed alert rule query = %#v fields=%#v", query, fields)
	}

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/alert-rules?enabled=1&sort_by=for_times", nil)
	_, fields = parseAlertRuleListQuery(context)
	if fields["enabled"] == "" || fields["sort_by"] == "" {
		t.Fatalf("invalid alert rule query fields = %#v", fields)
	}
}

func TestExportListQueryAcceptsCompatibleEnumArrayEncodings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	target := "/exports?status=pending&status=running,failed&status=" +
		url.QueryEscape(`["success","pending"]`)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", target, nil)
	query, fields := parseExportListQuery(context)
	if fields != nil || !reflect.DeepEqual(query.Statuses, []string{"pending", "running", "failed", "success"}) {
		t.Fatalf("parsed export filters = %#v fields=%#v", query, fields)
	}
}

func TestEnumArrayQueriesRejectMalformedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, target := range []string{
		"/alerts?status=",
		"/alerts?status=firing,,pending",
		"/alerts?status=" + url.QueryEscape(`["firing",1]`),
		"/alerts?status[]=firing",
		"/exports?status=unknown",
	} {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest("GET", target, nil)
		var fields map[string]string
		if strings.HasPrefix(target, "/alerts") {
			_, fields = parseAlertListQuery(context)
		} else {
			_, fields = parseExportListQuery(context)
		}
		if len(fields) == 0 {
			t.Fatalf("malformed enum query accepted: %s", target)
		}
	}
}
