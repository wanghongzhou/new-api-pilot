package controller

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestUpstreamTaskDefaultQueryHasNoTimeBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/api/upstream-tasks", nil)
	query, fields := parseUpstreamTaskQuery(context, true)
	if fields != nil || query.StartTimestamp != 0 || query.EndTimestamp != 0 {
		t.Fatalf("query=%#v fields=%v", query, fields)
	}
}

func TestUpstreamTaskQueryRejectsAmbiguousAndSiteOverrideFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, target := range []string{
		"/api/upstream-tasks?unknown=value",
		"/api/upstream-tasks?p=1&p=2",
		"/api/upstream-tasks?start_timestamp=01",
		"/api/upstream-tasks?end_timestamp=0",
	} {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest("GET", target, nil)
		if _, fields := parseUpstreamTaskQuery(context, true); fields == nil {
			t.Fatalf("accepted ambiguous query %s", target)
		}
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/api/sites/1/upstream-tasks?site_ids=2", nil)
	if _, fields := parseUpstreamTaskQuery(context, false); fields == nil {
		t.Fatal("site route accepted site_ids override")
	}
}
