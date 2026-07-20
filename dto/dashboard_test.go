package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDashboardQueriesNormalizeAndValidateAuthoritativeLimits(t *testing.T) {
	trend := DashboardTrendQuery{}
	trend.Normalize()
	if trend.Days != 30 || trend.Validate() != nil {
		t.Fatalf("default dashboard trend query = %#v errors=%#v", trend, trend.Validate())
	}
	trend.Days = 91
	if errors := trend.Validate(); errors == nil || errors["days"] == "" {
		t.Fatalf("overlong dashboard trend errors = %#v", errors)
	}

	top := DashboardTopQuery{Type: " CUSTOMER ", Metric: "QUOTA"}
	top.Normalize()
	if top.Type != DashboardTopTypeCustomer || top.Metric != DashboardTopMetricQuota || top.Limit != 5 ||
		top.Validate() != nil {
		t.Fatalf("normalized dashboard top query = %#v errors=%#v", top, top.Validate())
	}
	for name, query := range map[string]DashboardTopQuery{
		"unsupported type":   {Type: "account", Metric: DashboardTopMetricQuota, Limit: 5},
		"unsupported metric": {Type: DashboardTopTypeSite, Metric: "token_used", Limit: 5},
		"oversized limit":    {Type: DashboardTopTypeCustomer, Metric: DashboardTopMetricRequestCount, Limit: 21},
	} {
		t.Run(name, func(t *testing.T) {
			if errors := query.Validate(); errors == nil {
				t.Fatalf("invalid dashboard top query accepted: %#v", query)
			}
		})
	}
}

func TestRankingItemMarshalsBigintValueAsString(t *testing.T) {
	large := "9007199254740993"
	payload, err := json.Marshal(RankingItem{
		DimensionType: DashboardTopTypeSite,
		DimensionID:   "1",
		DimensionName: "site",
		SiteID:        func() *string { value := "1"; return &value }(),
		Value:         &large,
		DataStatus:    "complete",
		SiteBreakdown: []SiteQuotaBreakdown{},
	})
	if err != nil || !strings.Contains(string(payload), `"value":"9007199254740993"`) ||
		strings.Contains(string(payload), `"value":9007199254740993`) {
		t.Fatalf("ranking bigint JSON = %s, %v", payload, err)
	}
}

func TestDashboardSummaryMarshalsBigintsAsStrings(t *testing.T) {
	large := "9007199254740993"
	response := DashboardSummary{
		Today: DashboardUsageSummary{
			UsageSummary:  UsageSummary{RequestCount: &large, Quota: &large, DataStatus: "complete"},
			SiteBreakdown: []SiteQuotaBreakdown{},
		},
		ActiveAccountsToday:  &large,
		RPM:                  &large,
		TPM:                  &large,
		ResourceStaleSiteIDs: []string{},
		StaleSiteIDs:         []string{},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal dashboard summary: %v", err)
	}
	text := string(payload)
	if strings.Count(text, `"9007199254740993"`) < 5 || strings.Contains(text, `:9007199254740993`) {
		t.Fatalf("dashboard bigint JSON = %s", text)
	}
}
