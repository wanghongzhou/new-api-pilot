package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestCollectionWindowCompletenessCountsUnmaterializedExpectedHours(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_100_240_000)
	now -= now % 3600
	start := now - 3*3600
	site := createRunnableSite(t, database, fmt.Sprintf("run-overview-%d", time.Now().UnixNano()), now)
	site.StatisticsStartAt = &start
	if err := database.GORM.Model(&Site{}).Where("id = ?", site.ID).
		Update("statistics_start_at", start).Error; err != nil {
		t.Fatalf("set statistics start: %v", err)
	}

	windows := []CollectionWindow{
		{SiteID: site.ID, HourTS: start - 3600, Status: CollectionWindowStatusComplete, UpdatedAt: now},
		{SiteID: site.ID, HourTS: start, Status: CollectionWindowStatusComplete, UpdatedAt: now},
		{SiteID: site.ID, HourTS: start + 3600, Status: CollectionWindowStatusComplete, UpdatedAt: now},
		{SiteID: site.ID, HourTS: now, Status: CollectionWindowStatusComplete, UpdatedAt: now},
	}
	if err := database.GORM.Create(&windows).Error; err != nil {
		t.Fatalf("create collection windows: %v", err)
	}
	repository := NewSiteRepository(database.GORM)
	rates, err := repository.ListCollectionWindowCompleteness(context.Background(), []Site{site}, now)
	if err != nil {
		t.Fatalf("read incomplete collection range: %v", err)
	}
	if got, want := rates[site.ID], 2.0/3.0; got != want {
		t.Fatalf("completeness with one unmaterialized expected hour = %v, want %v", got, want)
	}

	if err := database.GORM.Create(&CollectionWindow{
		SiteID: site.ID, HourTS: start + 2*3600, Status: CollectionWindowStatusComplete, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("materialize final expected window: %v", err)
	}
	rates, err = repository.ListCollectionWindowCompleteness(context.Background(), []Site{site}, now)
	if err != nil || rates[site.ID] != 1 {
		t.Fatalf("complete collection range = %v, err %v", rates[site.ID], err)
	}

	statisticsEnd := start + 2*3600
	site.StatisticsEndAt = &statisticsEnd
	rates, err = repository.ListCollectionWindowCompleteness(context.Background(), []Site{site}, now)
	if err != nil || rates[site.ID] != 1 {
		t.Fatalf("closed collection range = %v, err %v", rates[site.ID], err)
	}
}
