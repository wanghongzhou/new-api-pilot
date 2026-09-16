package model

import (
	"strings"
	"testing"
)

func TestSiteUsageOverviewQueryUsesBoundedHourlyIdentityIndex(t *testing.T) {
	lower := strings.ToLower(siteUsageOverviewQuery)
	for _, required := range []string{
		"count(distinct f.remote_user_id)",
		"force index (idx_usage_fact_hourly_site_time)",
		"f.site_id in ?",
		"f.hour_ts >= ? and f.hour_ts < ?",
		"w.status = 'complete'",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("site usage overview query missing %q:\n%s", required, siteUsageOverviewQuery)
		}
	}
}
