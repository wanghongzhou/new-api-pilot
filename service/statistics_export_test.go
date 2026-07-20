package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type fakeExportRowReader struct {
	rows    []model.StatisticsExportRow
	queries []model.StatisticsExportRowQuery
	sites   []model.StatisticsSite
	err     error
}

func (reader *fakeExportRowReader) LoadExportRows(
	_ context.Context,
	query model.StatisticsExportRowQuery,
) ([]model.StatisticsExportRow, error) {
	reader.queries = append(reader.queries, query)
	if reader.err != nil {
		return nil, reader.err
	}
	start := 0
	if query.Cursor.Initialized {
		start = -1
		for index, row := range reader.rows {
			if exportRowCursorEqual(row.Cursor(), query.Cursor) {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, fmt.Errorf("cursor did not reference an emitted row: %#v", query.Cursor)
		}
	}
	end := start + query.Limit + 1
	if end > len(reader.rows) {
		end = len(reader.rows)
	}
	return append([]model.StatisticsExportRow(nil), reader.rows[start:end]...), nil
}

func (reader *fakeExportRowReader) LoadSites(context.Context, []int64) ([]model.StatisticsSite, error) {
	return append([]model.StatisticsSite(nil), reader.sites...), nil
}

func (reader *fakeExportRowReader) LoadFallbackRates(context.Context) (model.StatisticsFallbackRates, error) {
	return model.StatisticsFallbackRates{}, nil
}

func TestStatisticsExportIteratorPagesFinalRowsAtFixedBound(t *testing.T) {
	const totalRows = 12_003
	rows := make([]model.StatisticsExportRow, 0, totalRows)
	start := exportTestQuery(t, 1).StartTimestamp
	for siteID := int64(1); siteID <= totalRows; siteID++ {
		value := strconv.FormatInt(siteID, 10)
		rows = append(rows, model.StatisticsExportRow{
			DimensionID: "global", DimensionName: "全局",
			BreakdownSiteID: siteID, BreakdownSiteName: "site-" + value,
			BucketStart: start, BucketEnd: start + 3600,
			RequestCount: &value, Quota: &value, TokenUsed: &value, ActiveUsers: &value, SiteQuota: &value,
			DataStatus: "complete", IsFinal: true,
			SortKnown: 1, SortNumber: siteID,
		})
	}
	repository := &fakeExportRowReader{rows: rows}
	query := exportTestQuery(t, 1)
	request, err := statisticsReadRequest(dto.StatisticsScopeGlobal, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	iterator := &StatisticsExportIterator{
		repository: repository, clock: testsupport.NewFakeClock(time.Unix(query.EndTimestamp+3600, 0)),
		scope: dto.StatisticsScopeGlobal, query: query, request: request, pageSize: StatisticsExportPageSize,
	}
	seen := make(map[int64]struct{}, totalRows)
	pageSizes := []int{}
	for {
		response, done, nextErr := iterator.Next(context.Background())
		if nextErr != nil {
			t.Fatalf("Next: %v", nextErr)
		}
		if done {
			break
		}
		if len(response.Breakdown.Items) > StatisticsExportPageSize {
			t.Fatalf("page held %d rows", len(response.Breakdown.Items))
		}
		pageSizes = append(pageSizes, len(response.Breakdown.Items))
		for _, item := range response.Breakdown.Items {
			base := item.(dto.GlobalStatisticsBreakdown).StatisticsBreakdownBase
			if len(base.SiteBreakdown) != 1 {
				t.Fatalf("final row expanded to %d site rows", len(base.SiteBreakdown))
			}
			siteID, parseErr := strconv.ParseInt(base.SiteBreakdown[0].SiteID, 10, 64)
			if parseErr != nil {
				t.Fatalf("site ID: %v", parseErr)
			}
			if _, duplicate := seen[siteID]; duplicate {
				t.Fatalf("duplicate final row %d", siteID)
			}
			seen[siteID] = struct{}{}
		}
	}
	if got := fmt.Sprint(pageSizes); got != "[5000 5000 2003]" || len(seen) != totalRows {
		t.Fatalf("pages=%s rows=%d", got, len(seen))
	}
	if len(repository.queries) != 3 {
		t.Fatalf("DB page queries = %d, want 3", len(repository.queries))
	}
	for index, pageQuery := range repository.queries {
		if pageQuery.Limit != StatisticsExportPageSize {
			t.Fatalf("query %d limit=%d", index, pageQuery.Limit)
		}
		if index > 0 && !pageQuery.Cursor.Initialized {
			t.Fatalf("query %d did not carry cursor", index)
		}
	}
	if repository.queries[1].Cursor.BreakdownSiteID != 5000 ||
		repository.queries[2].Cursor.BreakdownSiteID != 10_000 {
		t.Fatalf("cursors used lookahead rather than last emitted row: %#v", repository.queries)
	}
}

func TestStatisticsExportIteratorPreservesBinaryModelNames(t *testing.T) {
	query := exportTestQuery(t, 2)
	request, err := statisticsReadRequest(dto.StatisticsScopeModel, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	names := []string{"Model-A", "model-a", "模型"}
	rows := make([]model.StatisticsExportRow, 0, len(names)*2)
	for _, name := range names {
		for bucket := 0; bucket < 2; bucket++ {
			rows = append(rows, model.StatisticsExportRow{
				DimensionSiteID: 7, DimensionValue: name, DimensionID: name, DimensionName: name,
				BreakdownSiteID: 7, BreakdownSiteName: "site-7",
				BucketStart: query.StartTimestamp + int64(bucket)*3600,
				BucketEnd:   query.StartTimestamp + int64(bucket+1)*3600,
				DataStatus:  "missing", SortKnown: 0,
			})
		}
	}
	repository := &fakeExportRowReader{rows: rows}
	iterator := &StatisticsExportIterator{
		repository: repository, clock: testsupport.NewFakeClock(time.Unix(query.EndTimestamp+3600, 0)),
		scope: dto.StatisticsScopeModel, query: query, request: request, pageSize: 2,
	}
	seen := map[string]int{}
	for {
		response, done, nextErr := iterator.Next(context.Background())
		if nextErr != nil {
			t.Fatalf("Next: %v", nextErr)
		}
		if done {
			break
		}
		for _, item := range response.Breakdown.Items {
			modelItem := item.(dto.ModelStatisticsBreakdown)
			seen[modelItem.ModelName]++
		}
	}
	for _, name := range names {
		if seen[name] != 2 {
			t.Fatalf("model %q rows=%d", name, seen[name])
		}
	}
}

func TestStatisticsExportIteratorEmptySnapshotFinishesWithoutRows(t *testing.T) {
	query := exportTestQuery(t, 24)
	request, err := statisticsReadRequest(dto.StatisticsScopeModel, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	repository := &fakeExportRowReader{}
	iterator := &StatisticsExportIterator{
		repository: repository, clock: testsupport.NewFakeClock(time.Unix(query.EndTimestamp+3600, 0)),
		scope: dto.StatisticsScopeModel, query: query, request: request, pageSize: 5,
	}
	response, done, err := iterator.Next(context.Background())
	if err != nil || !done || len(response.Breakdown.Items) != 0 || len(repository.queries) != 1 {
		t.Fatalf("empty Next response=%#v done=%t queries=%d err=%v", response, done, len(repository.queries), err)
	}
}

func TestStatisticsExportIteratorMapsCapacityToStableFileTooLargeFailure(t *testing.T) {
	query := exportTestQuery(t, 1)
	request, err := statisticsReadRequest(dto.StatisticsScopeGlobal, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	iterator := &StatisticsExportIterator{
		repository: &fakeExportRowReader{err: model.ErrStatisticsExportCapacity},
		clock:      testsupport.NewFakeClock(time.Unix(query.EndTimestamp+3600, 0)),
		scope:      dto.StatisticsScopeGlobal, query: query, request: request, pageSize: StatisticsExportPageSize,
	}
	_, done, err := iterator.Next(context.Background())
	if done || !errors.Is(err, ErrExportFileTooLarge) {
		t.Fatalf("capacity mapping done=%t err=%v", done, err)
	}
}

func exportRowCursorEqual(left, right model.StatisticsExportRowCursor) bool {
	return left.Initialized == right.Initialized && left.EntityID == right.EntityID &&
		left.DimensionSiteID == right.DimensionSiteID && left.DimensionValue == right.DimensionValue &&
		left.DimensionKey == right.DimensionKey &&
		left.BucketStart == right.BucketStart && left.SortKnown == right.SortKnown &&
		left.SortNumber == right.SortNumber && left.SortText == right.SortText &&
		left.BreakdownSiteID == right.BreakdownSiteID
}

func exportTestQuery(t *testing.T, hours int) dto.StatisticsQuery {
	t.Helper()
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, location).Unix()
	return dto.StatisticsQuery{
		StartTimestamp: start, EndTimestamp: start + int64(hours)*3600,
		Granularity: "hour", Page: 1, PageSize: 100, SortBy: "bucket_start", SortOrder: "asc",
	}
}
