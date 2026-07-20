package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomerWriteRequestsNormalizeAndValidate(t *testing.T) {
	request := CustomerCreateRequest{
		Name: "  示例客户  ", Contact: "  contact@example.test ", Remark: "  remark  ", Status: CustomerStatusUsing,
	}
	request.Normalize()
	if request.Name != "示例客户" || request.Contact != "contact@example.test" || request.Remark != "remark" {
		t.Fatalf("normalized request = %#v", request)
	}
	if errors := request.Validate(); errors != nil {
		t.Fatalf("valid request errors = %#v", errors)
	}

	tests := []struct {
		name    string
		request CustomerCreateRequest
		field   string
	}{
		{name: "empty name", request: CustomerCreateRequest{Status: CustomerStatusUsing}, field: "name"},
		{name: "long Unicode name", request: CustomerCreateRequest{Name: strings.Repeat("客", 129), Status: CustomerStatusUsing}, field: "name"},
		{name: "long contact", request: CustomerCreateRequest{Name: "ok", Contact: strings.Repeat("联", 256), Status: CustomerStatusUsing}, field: "contact"},
		{name: "long remark", request: CustomerCreateRequest{Name: "ok", Remark: strings.Repeat("注", 501), Status: CustomerStatusUsing}, field: "remark"},
		{name: "disabled bypass", request: CustomerCreateRequest{Name: "ok", Status: CustomerStatusDisabled}, field: "status"},
		{name: "unknown status", request: CustomerCreateRequest{Name: "ok", Status: "active"}, field: "status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if errors := test.request.Validate(); errors == nil || errors[test.field] == "" {
				t.Fatalf("expected %s error, got %#v", test.field, errors)
			}
		})
	}

	update := CustomerUpdateRequest{Name: "name", Status: CustomerStatusDisabled}
	if errors := update.Validate(); errors == nil || errors["status"] == "" {
		t.Fatalf("generic update accepted disabled: %#v", errors)
	}
}

func TestCustomerListQueryDefaultsAndWhitelist(t *testing.T) {
	query := CustomerListQuery{}
	query.Normalize()
	if query.Page != 1 || query.PageSize != 20 || query.SortBy != "updated_at" || query.SortOrder != "desc" || query.Offset() != 0 {
		t.Fatalf("defaults = %#v", query)
	}
	if errors := query.Validate(); errors != nil {
		t.Fatalf("default query errors = %#v", errors)
	}

	query = CustomerListQuery{Page: 2, PageSize: 100, Status: CustomerStatusDisabled, SortBy: "account_count", SortOrder: "asc"}
	if errors := query.Validate(); errors != nil || query.Offset() != 100 {
		t.Fatalf("valid filtered query errors=%#v offset=%d", errors, query.Offset())
	}

	query = CustomerListQuery{Page: -1, PageSize: 101, Keyword: strings.Repeat("k", 129), Status: "bad", SortBy: "name; DROP TABLE customer", SortOrder: "sideways"}
	errors := query.Validate()
	for _, field := range []string{"p", "page_size", "keyword", "status", "sort_by", "sort_order"} {
		if errors[field] == "" {
			t.Fatalf("missing %s error in %#v", field, errors)
		}
	}
}

func TestCustomerResponseBigintsMarshalAsStrings(t *testing.T) {
	metric := "9223372036854775807"
	item := CustomerListItem{
		ID: metric,
		Today: CustomerUsageSummary{
			UsageSummary:  UsageSummary{RequestCount: &metric, Quota: &metric, TokenUsed: &metric, ActiveUsers: &metric},
			SiteBreakdown: []SiteQuotaBreakdown{{SiteID: metric, Quota: &metric}},
		},
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal customer: %v", err)
	}
	text := string(payload)
	for _, fragment := range []string{`"id":"9223372036854775807"`, `"request_count":"9223372036854775807"`, `"site_id":"9223372036854775807"`} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("response does not preserve bigint string %s: %s", fragment, text)
		}
	}
}
