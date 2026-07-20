package service

import (
	"new-api-pilot/dto"
	"new-api-pilot/model"
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
		{DimensionID: "stable", Growth: &zero},
		{DimensionID: "up-low", Growth: &positiveLow},
		{DimensionID: "unknown", Growth: nil},
		{DimensionID: "down-low", Growth: &negativeLow},
		{DimensionID: "up-high", Growth: &positiveHigh},
		{DimensionID: "down-high", Growth: &negativeHigh},
	}
	movers, droppers := rankingMoversDroppers(items)
	if len(movers) != 2 || movers[0].DimensionID != "up-high" || movers[1].DimensionID != "up-low" {
		t.Fatalf("movers=%#v", movers)
	}
	if len(droppers) != 2 || droppers[0].DimensionID != "down-low" || droppers[1].DimensionID != "down-high" {
		t.Fatalf("droppers=%#v", droppers)
	}
}
