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
	queries := []SubscriptionPlanQuery{
		{Page: 1, PageSize: 20, SiteIDs: catalogTestSiteIDs(101)},
		{Page: 1, PageSize: 20, States: []string{"normal", "missing", "unexpected"}},
		{Page: 1, PageSize: 20, Keyword: strings.Repeat("界", 43)},
	}
	for index, query := range queries {
		query.Normalize()
		if fields := query.Validate(); fields == nil || fields["filters"] == "" {
			t.Fatalf("case %s validation=%v", strconv.Itoa(index), fields)
		}
	}
}
