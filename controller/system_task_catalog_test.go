package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseSystemTaskQueryFrozenFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := gin.CreateTestContext(httptest.NewRecorder())
	g.Request = httptest.NewRequest("GET", "/api/system-tasks?types=channel_test&statuses=running&error_present=true&created_start=10&created_end=20", nil)
	q, fields := parseSystemTaskQuery(g, true)
	if fields != nil || len(q.Types) != 1 || q.CreatedStart != 10 || q.CreatedEnd != 20 || q.ErrorPresent == nil || !*q.ErrorPresent {
		t.Fatalf("q=%+v fields=%v", q, fields)
	}
}
func TestParseSystemTaskQueryRejectsRawAndMutationFields(t *testing.T) {
	for _, key := range []string{"payload", "state", "result", "error", "active_key", "locked_by", "task_id", "action"} {
		g, _ := gin.CreateTestContext(httptest.NewRecorder())
		g.Request = httptest.NewRequest("GET", "/api/system-tasks?"+key+"=secret", nil)
		_, fields := parseSystemTaskQuery(g, true)
		if fields == nil {
			t.Fatalf("field %s accepted", key)
		}
	}
}
