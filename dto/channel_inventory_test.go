package dto

import (
	"strings"
	"testing"
)

func TestChannelInventoryStatisticsQueryRejectsOversizedFilters(t *testing.T) {
	query := ChannelInventoryStatisticsQuery{StartTimestamp: 3600, EndTimestamp: 7200, SiteIDs: make([]int64, 101)}
	for index := range query.SiteIDs {
		query.SiteIDs[index] = int64(index + 1)
	}
	if fields := query.Validate(); fields == nil || fields["filters"] == "" {
		t.Fatalf("oversized statistics filters accepted: %#v", fields)
	}
	query = ChannelInventoryStatisticsQuery{StartTimestamp: 3600, EndTimestamp: 7200, Groups: []string{strings.Repeat("组", 129)}}
	if fields := query.Validate(); fields == nil || fields["groups"] == "" {
		t.Fatalf("oversized group accepted: %#v", fields)
	}
}
