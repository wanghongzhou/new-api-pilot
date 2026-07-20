package service

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"math/big"
	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"sort"
	"strconv"
	"time"
)

type LocalRankingService struct {
	db    *gorm.DB
	clock common.Clock
}

func NewLocalRankingService(db *gorm.DB, clock common.Clock) (*LocalRankingService, error) {
	if db == nil || clock == nil {
		return nil, errors.New("ranking dependencies required")
	}
	return &LocalRankingService{db: db, clock: clock}, nil
}

func (r *LocalRankingService) readSnapshot(ctx context.Context, read func(*model.LocalRankingRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(model.NewLocalRankingRepository(tx))
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func rankingWindow(now time.Time, period string) (int64, int64, int64, error) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	n := now.In(loc)
	var start time.Time
	switch period {
	case "today":
		start = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	case "week":
		d := (int(n.Weekday()) + 6) % 7
		start = time.Date(n.Year(), n.Month(), n.Day()-d, 0, 0, 0, 0, loc)
	case "month":
		start = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
	case "year":
		start = time.Date(n.Year(), 1, 1, 0, 0, 0, 0, loc)
	default:
		return 0, 0, 0, ErrStatisticsInvalid
	}
	end := n.Unix()
	duration := end - start.Unix()
	return start.Unix(), end, start.Unix() - duration, nil
}
func decimalRatio(n, d int64) *string {
	if d == 0 {
		return nil
	}
	v := new(big.Rat).SetFrac(big.NewInt(n), big.NewInt(d)).FloatString(10)
	for len(v) > 1 && v[len(v)-1] == '0' {
		v = v[:len(v)-1]
	}
	if v[len(v)-1] == '.' {
		v = v[:len(v)-1]
	}
	return &v
}
func rankingCompletenessStatus(c model.RankingCompletenessRow, expected int64) string {
	if expected <= 0 {
		return "pending"
	}
	if c.CompleteCount >= expected {
		return "complete"
	}
	if c.CompleteCount == 0 && c.UnavailableCount > 0 {
		return "unavailable"
	}
	if c.CompleteCount == 0 && c.MissingCount > 0 {
		return "missing"
	}
	if c.CompleteCount == 0 {
		return "pending"
	}
	return "partial"
}

func rankingMoversDroppers(items []dto.LocalRankingItem) ([]dto.LocalRankingItem, []dto.LocalRankingItem) {
	movers := make([]dto.LocalRankingItem, 0, len(items))
	droppers := make([]dto.LocalRankingItem, 0, len(items))
	for _, item := range items {
		if item.Growth == nil {
			continue
		}
		growth, ok := new(big.Rat).SetString(*item.Growth)
		if !ok {
			continue
		}
		switch growth.Sign() {
		case 1:
			movers = append(movers, item)
		case -1:
			droppers = append(droppers, item)
		}
	}
	compareGrowth := func(left, right dto.LocalRankingItem) int {
		a, _ := new(big.Rat).SetString(*left.Growth)
		b, _ := new(big.Rat).SetString(*right.Growth)
		return a.Cmp(b)
	}
	sort.SliceStable(movers, func(i, j int) bool { return compareGrowth(movers[i], movers[j]) > 0 })
	sort.SliceStable(droppers, func(i, j int) bool { return compareGrowth(droppers[i], droppers[j]) < 0 })
	return movers, droppers
}

func (r *LocalRankingService) Query(ctx context.Context, q dto.LocalRankingQuery, kind string) (dto.LocalRankingResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.LocalRankingResponse{}, ErrStatisticsInvalid
	}
	start, end, prior, err := rankingWindow(r.clock.Now(), q.Period)
	if err != nil {
		return dto.LocalRankingResponse{}, err
	}
	var rows, siteRows []model.LocalRankingRow
	var history []model.LocalRankingHistoryRow
	var completeness []model.RankingCompletenessRow
	if err := r.readSnapshot(ctx, func(repo *model.LocalRankingRepository) error {
		var err error
		rows, err = repo.Rows(ctx, q, kind, start, end, prior, false)
		if err != nil {
			return err
		}
		siteRows, err = repo.Rows(ctx, q, kind, start, end, prior, true)
		if err != nil {
			return err
		}
		history, err = repo.History(ctx, q, kind, start, end)
		if err != nil {
			return err
		}
		completeness, err = repo.Completeness(ctx, q, start, end)
		return err
	}); err != nil {
		return dto.LocalRankingResponse{}, err
	}
	total := int64(0)
	for _, x := range rows {
		total += x.TokenUsed
	}
	items := make([]dto.LocalRankingItem, 0, len(rows))
	for i, x := range rows {
		share := decimalRatio(x.TokenUsed, total)
		growth := func() *string {
			if x.PreviousTokenUsed == 0 {
				return nil
			}
			return decimalRatio(x.TokenUsed-x.PreviousTokenUsed, x.PreviousTokenUsed)
		}()
		s := "0"
		if share != nil {
			s = *share
		}
		items = append(items, dto.LocalRankingItem{DimensionID: x.DimensionID, DimensionName: x.DimensionName, TokenUsed: strconv.FormatInt(x.TokenUsed, 10), RequestCount: strconv.FormatInt(x.RequestCount, 10), Quota: strconv.FormatInt(x.Quota, 10), Share: s, Growth: growth, Rank: i + 1})
	}
	movers, droppers := rankingMoversDroppers(items)
	statusBy := map[int64]string{}
	asOfBy := map[int64]*int64{}
	overall := "pending"
	statusCounts := map[string]int{}
	expected := (end - end%3600 - start) / 3600
	if expected < 0 {
		expected = 0
	}
	var asOf *int64
	for _, c := range completeness {
		status := rankingCompletenessStatus(c, expected)
		statusCounts[status]++
		if status != "complete" {
			overall = "partial"
		}
		statusBy[c.SiteID] = status
		asOfBy[c.SiteID] = c.AsOf
		if c.AsOf != nil && (asOf == nil || *c.AsOf > *asOf) {
			v := *c.AsOf
			asOf = &v
		}
	}
	if len(completeness) > 0 {
		overall = "complete"
		for status, count := range statusCounts {
			if status != "complete" && count > 0 {
				overall = "partial"
				break
			}
		}
	}
	if len(completeness) > 0 && statusCounts["complete"] != len(completeness) {
		overall = "partial"
		for _, candidate := range []string{"unavailable", "missing", "pending"} {
			if statusCounts[candidate] == len(completeness) {
				overall = candidate
			}
		}
	}
	breakdown := make([]dto.RankingSiteBreakdown, 0, len(siteRows))
	for _, x := range siteRows {
		breakdown = append(breakdown, dto.RankingSiteBreakdown{DimensionID: x.DimensionID, SiteID: strconv.FormatInt(x.SiteID, 10), SiteName: x.SiteName, TokenUsed: strconv.FormatInt(x.TokenUsed, 10), DataStatus: statusBy[x.SiteID], AsOf: asOfBy[x.SiteID]})
	}
	points := make([]dto.RankingHistoryPoint, 0, len(history))
	for _, x := range history {
		points = append(points, dto.RankingHistoryPoint{DimensionID: x.DimensionID, BucketStart: x.BucketStart, TokenUsed: strconv.FormatInt(x.TokenUsed, 10)})
	}
	return dto.LocalRankingResponse{Period: q.Period, StartTimestamp: start, EndTimestamp: end, Items: items, Movers: movers, Droppers: droppers, History: points, SiteBreakdown: breakdown, DataStatus: overall, AsOf: asOf}, nil
}
