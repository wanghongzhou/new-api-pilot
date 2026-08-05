package dto

import (
	"strconv"
	"strings"
	"testing"
)

func catalogTestSiteIDs(count int) []int64 {
	ids := make([]int64, count)
	for index := range ids {
		ids[index] = int64(index + 1)
	}
	return ids
}

func TestPricingCatalogQueryRejectsOversizedFilters(t *testing.T) {
	query := PricingCatalogQuery{Page: 1, PageSize: 20, SiteIDs: catalogTestSiteIDs(101)}
	query.Normalize()
	if fields := query.Validate(); fields == nil || fields["filters"] == "" {
		t.Fatalf("site filter validation=%v", fields)
	}

	query = PricingCatalogQuery{Page: 1, PageSize: 20, States: []string{"normal", "missing", "unexpected"}}
	query.Normalize()
	if fields := query.Validate(); fields == nil || fields["filters"] == "" || fields["states"] == "" {
		t.Fatalf("state filter validation=%v", fields)
	}
}

func TestSubscriptionPlanQueryRejectsOversizedFilters(t *testing.T) {
	tests := []struct {
		field string
		query SubscriptionPlanQuery
	}{
		{field: "filters", query: SubscriptionPlanQuery{Page: 1, PageSize: 20, SiteIDs: catalogTestSiteIDs(101)}},
		{field: "filters", query: SubscriptionPlanQuery{Page: 1, PageSize: 20, States: []string{"normal", "missing", "unexpected"}}},
		{field: "keyword", query: SubscriptionPlanQuery{Page: 1, PageSize: 20, Keyword: strings.Repeat("界", 43)}},
		{field: "keyword", query: SubscriptionPlanQuery{Page: 1, PageSize: 20, Keyword: string([]byte{0xff})}},
	}
	for index, test := range tests {
		test.query.Normalize()
		if fields := test.query.Validate(); fields == nil || fields[test.field] == "" {
			t.Fatalf("case %s validation=%v", strconv.Itoa(index), fields)
		}
	}
}

func TestCatalogQueriesRejectInvalidUTF8AndByteOverflow(t *testing.T) {
	modelQueries := []ModelCatalogQuery{
		{Page: 1, PageSize: 20, Keyword: strings.Repeat("界", 43)},
		{Page: 1, PageSize: 20, Keyword: string([]byte{0xff})},
	}
	for _, query := range modelQueries {
		query.Normalize()
		if fields := query.Validate(); fields == nil || fields["keyword"] == "" {
			t.Fatalf("model query validation=%v", fields)
		}
	}

	pricingQueries := []struct {
		field string
		query PricingCatalogQuery
	}{
		{field: "keyword", query: PricingCatalogQuery{Page: 1, PageSize: 20, Keyword: strings.Repeat("界", 86)}},
		{field: "keyword", query: PricingCatalogQuery{Page: 1, PageSize: 20, Keyword: string([]byte{0xff})}},
		{field: "group", query: PricingCatalogQuery{Page: 1, PageSize: 20, Group: strings.Repeat("界", 43)}},
	}
	for _, test := range pricingQueries {
		test.query.Normalize()
		if fields := test.query.Validate(); fields == nil || fields[test.field] == "" {
			t.Fatalf("pricing query field=%s validation=%v", test.field, fields)
		}
	}
}
