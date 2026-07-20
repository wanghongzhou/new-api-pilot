package service

import "testing"

func TestChannelInventoryMetricKeysAreStableAndContainNoKeyLevelMetrics(t *testing.T) {
	values := []string{ChannelMetricAvailableCount, ChannelMetricUnavailableCount, ChannelMetricAvailabilityRate, ChannelMetricBalanceTotal, ChannelMetricResponseTimeAvgMS, ChannelMetricResponseTimeMaxMS}
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || seen[v] {
			t.Fatalf("invalid channel metric key %q", v)
		}
		seen[v] = true
	}
	for _, forbidden := range []string{"channel.key_count", "channel.multi_key_health", "channel.key_balance"} {
		if seen[forbidden] {
			t.Fatalf("forbidden key-level metric %q", forbidden)
		}
	}
	if len(ChannelAlertRuleContracts) != 3 || ChannelAlertRuleContracts[0].Metric != ChannelMetricBalanceTotal ||
		ChannelAlertRuleContracts[1].Metric != ChannelMetricResponseTimeAvgMS ||
		ChannelAlertRuleContracts[2].Metric != ChannelMetricAvailabilityRate {
		t.Fatalf("channel alert rule contracts=%#v", ChannelAlertRuleContracts)
	}
}
