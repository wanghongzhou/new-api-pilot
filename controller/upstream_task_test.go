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
	query, fields := parseUpstreamTaskQuery(context)
	if fields != nil || query.StartTimestamp != 0 || query.EndTimestamp != 0 {
		t.Fatalf("query=%#v fields=%v", query, fields)
	}
}
