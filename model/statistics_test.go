package model

import (
	"strings"
	"testing"
)

func TestStatisticsQuerySourcesPreserveDailyIdentityAndExactChannelPairs(t *testing.T) {
	table, dimension, site, joins, err := statisticsMetricSource("model", "month")
	if err != nil || table != "model_stat_daily" || dimension != "st.model_name" || site != "st.site_id" || joins != "" {
		t.Fatalf("monthly model metric source = %q %q %q %q, %v", table, dimension, site, joins, err)
	}
	request := StatisticsReadRequest{
		Scope: "channel", Granularity: "month", StartTimestamp: 1, EndTimestamp: 2,
		StartDateKey: 20320101, EndDateKey: 20320201, SiteIDs: []int64{5, 6},
		ChannelKeys: []StatisticsChannelKey{{SiteID: 5, ChannelID: 1}, {SiteID: 6, ChannelID: 0}},
	}
	from, _, _, _, bucket, where, args, err := statisticsActiveSource(request)
	if err != nil || !strings.Contains(from, "usage_fact_daily") || bucket != "f.date_key DIV 100" {
		t.Fatalf("monthly channel active source = from %q bucket %q, %v", from, bucket, err)
	}
	if !strings.Contains(where, "(f.site_id = ? AND f.channel_id = ?) OR (f.site_id = ? AND f.channel_id = ?)") ||
		len(args) != 7 {
		t.Fatalf("exact channel active filter = %q args=%#v", where, args)
	}

	customer := request
	customer.Scope = "customer"
	customer.ChannelKeys = nil
	from, _, identity, totalIdentity, _, where, _, err := statisticsActiveSource(customer)
	if err != nil || !strings.Contains(from, "usage_fact_hourly") || identity != "a.id" || totalIdentity != "a.id" ||
		!strings.Contains(where, "a.remote_created_at < f.hour_ts + 3600") ||
		!strings.Contains(where, "c.statistics_paused_at") {
		t.Fatalf("customer fixed-identity active source = from %q where %q identity=%q/%q, %v",
			from, where, identity, totalIdentity, err)
	}

	account := customer
	account.Scope = "account"
	account.CustomerIDs = []int64{7}
	account.AccountIDs = []int64{9}
	from, dimension, identity, totalIdentity, _, where, _, err = statisticsActiveSource(account)
	if err != nil || !strings.Contains(from, "usage_fact_hourly") || !strings.Contains(from, "JOIN account AS a") ||
		dimension != "CAST(a.id AS CHAR)" || identity != "a.id" || totalIdentity != "a.id" ||
		!strings.Contains(where, "a.id IN ?") || !strings.Contains(where, "a.statistics_paused_at") {
		t.Fatalf("account fact-derived active source = from %q where %q dimension=%q identity=%q/%q, %v",
			from, where, dimension, identity, totalIdentity, err)
	}
}

func TestStatisticsActiveQueryUsesAggregateRowsAndOneFactSummaryScan(t *testing.T) {
	request := StatisticsReadRequest{
		Scope: "global", Granularity: "hour", StartTimestamp: 1_752_000_000, EndTimestamp: 1_754_678_400,
		SiteIDs: []int64{1, 2, 3},
	}
	query, args, err := statisticsActiveQuery(request)
	if err != nil {
		t.Fatalf("build global active query: %v", err)
	}
	if !strings.Contains(query, "WITH site_active AS") || !strings.Contains(query, "FROM site_stat_hourly AS st") {
		t.Fatalf("global active query did not use hourly aggregate rows:\n%s", query)
	}
	if count := strings.Count(query, "usage_fact_hourly AS f"); count != 1 {
		t.Fatalf("global active query fact scan count = %d, want 1:\n%s", count, query)
	}
	if !strings.Contains(query, "COUNT(DISTINCT CAST(CONCAT_WS(':', f.site_id, f.remote_user_id) AS BINARY))") {
		t.Fatalf("global summary lost site-scoped distinct identity:\n%s", query)
	}
	if len(args) != 6 {
		t.Fatalf("global active query args = %#v, want aggregate and fact filters once each", args)
	}

	request.Granularity = "day"
	request.StartDateKey = 20250701
	request.EndDateKey = 20250801
	query, _, err = statisticsActiveQuery(request)
	if err != nil || !strings.Contains(query, "FROM site_stat_daily AS st") ||
		strings.Count(query, "usage_fact_daily AS f") != 1 {
		t.Fatalf("daily global active query = %v\n%s", err, query)
	}
}

func TestStatisticsActiveQueryMaterializesFilteredIdentitiesOnceForAllOtherScopes(t *testing.T) {
	request := StatisticsReadRequest{
		Scope: "model", Granularity: "hour", StartTimestamp: 1_752_000_000, EndTimestamp: 1_754_678_400,
		SiteIDs: []int64{1, 2}, ModelNames: []string{"gpt-5", "gpt-5-mini"},
	}
	query, args, err := statisticsActiveQuery(request)
	if err != nil {
		t.Fatalf("build model active query: %v", err)
	}
	if !strings.Contains(query, "WITH active_base AS") || strings.Contains(query, "site_stat_hourly") {
		t.Fatalf("model active query did not use the identity base:\n%s", query)
	}
	if count := strings.Count(query, "usage_fact_hourly AS f"); count != 1 {
		t.Fatalf("model active query fact scan count = %d, want 1:\n%s", count, query)
	}
	if !strings.Contains(query, "GROUP BY dimension_id, site_id, bucket_key, dimension_identity, total_identity") ||
		!strings.Contains(query, "COUNT(DISTINCT dimension_identity)") ||
		!strings.Contains(query, "COUNT(DISTINCT total_identity)") {
		t.Fatalf("model active query lost distinct aggregation levels:\n%s", query)
	}
	if len(args) != 4 {
		t.Fatalf("model active query args = %#v, want one set of filters", args)
	}

	request.Scope = "customer"
	request.Granularity = "day"
	request.ModelNames = nil
	request.CustomerIDs = []int64{7}
	query, _, err = statisticsActiveQuery(request)
	if err != nil || strings.Count(query, "usage_fact_hourly AS f") != 1 ||
		!strings.Contains(query, "JOIN account AS a") || !strings.Contains(query, "JOIN customer AS c") ||
		!strings.Contains(query, "a.statistics_paused_at") || !strings.Contains(query, "c.statistics_paused_at") {
		t.Fatalf("customer active query lost lifecycle filters: %v\n%s", err, query)
	}
}
