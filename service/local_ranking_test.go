package service

import (
	"context"
	"fmt"
	"strings"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
	"testing"
	"time"
)

func TestRankingWindowBeijingBoundaries(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 7, 15, 12, 34, 0, 0, loc)
	want := map[string]time.Time{"today": time.Date(2026, 7, 15, 0, 0, 0, 0, loc), "week": time.Date(2026, 7, 13, 0, 0, 0, 0, loc), "month": time.Date(2026, 7, 1, 0, 0, 0, 0, loc), "year": time.Date(2026, 1, 1, 0, 0, 0, 0, loc)}
	for period, startWant := range want {
		start, end, prior, err := rankingWindow(now, period)
		if err != nil || start != startWant.Unix() || end != now.Unix() || prior != start-(end-start) {
			t.Fatalf("period=%s start=%d end=%d prior=%d err=%v", period, start, end, prior, err)
		}
	}
}

func TestVendorRankingTreatsConflictingExactMetadataAsUnknown(t *testing.T) {
	database := openUpstreamLogExportDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, location)
	timestamp := now.Unix()
	site := model.Site{Name: "Ranking Conflict", BaseURL: fmt.Sprintf("https://ranking-conflict-%d.example", time.Now().UnixNano()), ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, CreatedAt: timestamp, UpdatedAt: timestamp}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	metas := []model.SiteModelMeta{
		{SiteID: site.ID, RemoteID: 1, ModelName: "gpt", VendorID: 7, RemoteStatus: 1, SyncOfficial: 1, NameRule: 0, SourceHash: strings.Repeat("a", 64), ConfigVersion: 1, CollectedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp},
		{SiteID: site.ID, RemoteID: 2, ModelName: "gpt", VendorID: 8, RemoteStatus: 1, SyncOfficial: 1, NameRule: 0, SourceHash: strings.Repeat("b", 64), ConfigVersion: 1, CollectedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp},
	}
	if err := database.GORM.Create(&metas).Error; err != nil {
		t.Fatal(err)
	}
	fact := model.UsageFactHourly{SiteID: site.ID, RemoteUserID: 1, UsernameSnapshot: "user", ModelName: "gpt", UseGroup: "default", NodeName: "node", HourTS: timestamp - 3600, RequestCount: 1, Quota: 1, TokenUsed: 100, CollectedAt: timestamp}
	if err := database.GORM.Create(&fact).Error; err != nil {
		t.Fatal(err)
	}
	svc, err := NewLocalRankingService(database.GORM, testsupport.NewFakeClock(now))
	if err != nil {
		t.Fatal(err)
	}
	response, err := svc.Query(context.Background(), dto.LocalRankingQuery{Period: "today", SiteIDs: []int64{site.ID}}, "vendor")
	if err != nil || len(response.Items) != 1 || response.Items[0].DimensionID != "0" || response.Items[0].DimensionName != "unknown" {
		t.Fatalf("ranking=%#v err=%v", response, err)
	}
}
func TestRankingCompletenessStatuses(t *testing.T) {
	cases := []struct {
		name string
		row  model.RankingCompletenessRow
		want string
	}{{"complete", model.RankingCompletenessRow{CompleteCount: 12}, "complete"}, {"partial", model.RankingCompletenessRow{CompleteCount: 11}, "partial"}, {"missing", model.RankingCompletenessRow{MissingCount: 1}, "missing"}, {"unavailable", model.RankingCompletenessRow{UnavailableCount: 1}, "unavailable"}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rankingCompletenessStatus(c.row, 12); got != c.want {
				t.Fatalf("got=%s want=%s", got, c.want)
			}
		})
	}
}

func TestRankingCompletenessZeroExpectedIsPending(t *testing.T) {
	if got := rankingCompletenessStatus(model.RankingCompletenessRow{}, 0); got != "pending" {
		t.Fatalf("got=%s want=pending", got)
	}
}

func TestRankingMoversDroppersOnlyContainDirectionalGrowth(t *testing.T) {
	positiveHigh, positiveLow := "1.5", "0.25"
	negativeHigh, negativeLow := "-0.1", "-0.75"
	zero := "0"
	items := []dto.LocalRankingItem{
		{DimensionID: "stable", Growth: &zero, MovementType: "stable"},
		{DimensionID: "new", MovementType: "new"},
		{DimensionID: "up-low", Growth: &positiveLow, MovementType: "up"},
		{DimensionID: "unknown", Growth: nil},
		{DimensionID: "removed", MovementType: "removed"},
		{DimensionID: "down-low", Growth: &negativeLow, MovementType: "down"},
		{DimensionID: "up-high", Growth: &positiveHigh, MovementType: "up"},
		{DimensionID: "down-high", Growth: &negativeHigh, MovementType: "down"},
	}
	movers, droppers := rankingMoversDroppers(items)
	if len(movers) != 3 || movers[0].DimensionID != "new" || movers[1].DimensionID != "up-high" || movers[2].DimensionID != "up-low" {
		t.Fatalf("movers=%#v", movers)
	}
	if len(droppers) != 3 || droppers[0].DimensionID != "removed" || droppers[1].DimensionID != "down-low" || droppers[2].DimensionID != "down-high" {
		t.Fatalf("droppers=%#v", droppers)
	}
}
