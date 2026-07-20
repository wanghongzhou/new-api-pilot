package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

func TestParseResourceQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		target     string
		valid      bool
		wantNode   string
		wantNodeOK bool
	}{
		{
			name: "site minute query", target: "/?start_timestamp=1752400800&end_timestamp=1752400860&granularity=minute",
			valid: true,
		},
		{
			name: "case sensitive node", target: "/?start_timestamp=1752400800&end_timestamp=1752404400&granularity=hour&node_name=Node-A",
			valid: true, wantNode: "Node-A", wantNodeOK: true,
		},
		{name: "missing start", target: "/?end_timestamp=1752404400&granularity=hour"},
		{name: "duplicate node", target: "/?start_timestamp=1752400800&end_timestamp=1752404400&granularity=hour&node_name=a&node_name=b"},
		{name: "unknown query", target: "/?start_timestamp=1752400800&end_timestamp=1752404400&granularity=hour&aggregation=max"},
		{name: "unaligned hour", target: "/?start_timestamp=1752400801&end_timestamp=1752404400&granularity=hour"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("GET", test.target, nil)
			query, fieldErrors := parseResourceQuery(context)
			if test.valid && fieldErrors != nil {
				t.Fatalf("parseResourceQuery() errors = %#v", fieldErrors)
			}
			if !test.valid && fieldErrors == nil {
				t.Fatalf("parseResourceQuery() accepted query %#v", query)
			}
			if test.valid && query.Granularity == "" {
				t.Fatal("parseResourceQuery() dropped granularity")
			}
			if test.wantNodeOK && (query.NodeName == nil || *query.NodeName != test.wantNode) {
				t.Fatalf("node_name = %#v, want %q", query.NodeName, test.wantNode)
			}
			if !test.wantNodeOK && test.valid && query.NodeName != nil {
				t.Fatalf("node_name = %#v, want nil", query.NodeName)
			}
		})
	}
}

func TestResourceQueryDTOUsesDocumentedGranularities(t *testing.T) {
	for _, granularity := range []string{dto.ResourceGranularityMinute, dto.ResourceGranularityHour, dto.ResourceGranularityDay} {
		if granularity == "" {
			t.Fatal("empty resource granularity")
		}
	}
}
