package model

import (
	"strings"
	"testing"
)

func TestStatisticsExportRowsStatementAggregatesCompleteLogicalItemsBeforeFinalKeyset(t *testing.T) {
	statement, args, err := statisticsExportRowsStatement(StatisticsExportRowQuery{
		Request: StatisticsReadRequest{
			Scope: "model", Granularity: "hour",
			StartTimestamp: 1_767_196_800, EndTimestamp: 1_767_556_800,
			SiteIDs: []int64{1, 2}, ModelNames: []string{"Model-A", "model-a"},
		},
		SortBy: "quota", SortOrder: "desc", Now: 1_767_600_000,
		UsageDelayMinutes: 5, Limit: StatisticsExportMaximumPageSize,
		Cursor: StatisticsExportRowCursor{
			Initialized: true, DimensionSiteID: 2, DimensionValue: "Model-A",
			DimensionKey: "2\x00Model-A", BucketStart: 1_767_200_400,
			SortKnown: 1, SortNumber: 42, SortText: "Model-A", BreakdownSiteID: 2,
		},
	})
	if err != nil {
		t.Fatalf("build final-row SQL: %v", err)
	}
	lower := strings.ToLower(statement)
	if strings.Contains(lower, "recursive") || strings.Contains(lower, " offset ") {
		t.Fatalf("final-row SQL uses an unbounded paging construct:\n%s", statement)
	}
	candidatesStart := strings.Index(statement, "candidates AS (")
	candidatesEnd := strings.Index(statement, "candidate_bounds AS (")
	windowedStart := strings.Index(statement, "windowed_rows AS (")
	finalStart := strings.Index(statement, "final_rows AS (")
	if candidatesStart < 0 || candidatesEnd <= candidatesStart ||
		windowedStart <= candidatesEnd || finalStart <= windowedStart {
		t.Fatalf("final-row SQL CTE order is invalid")
	}
	candidates := statement[candidatesStart:candidatesEnd]
	for _, forbidden := range []string{"ORDER BY", "LIMIT ?", "breakdown_site_id > ?"} {
		if strings.Contains(candidates, forbidden) {
			t.Fatalf("candidate CTE truncates a logical item with %q:\n%s", forbidden, candidates)
		}
	}
	if !strings.Contains(statement[windowedStart:finalStart],
		"PARTITION BY s.entity_id, s.dimension_site_id, s.dimension_value, s.bucket_start") {
		t.Fatalf("logical totals are not windowed across the complete site breakdown")
	}
	if strings.Count(statement, "LIMIT ?") != 1 {
		t.Fatalf("LIMIT placeholder count = %d, want final limit only", strings.Count(statement, "LIMIT ?"))
	}
	limitValues := 0
	for _, arg := range args {
		if value, ok := arg.(int); ok && value == StatisticsExportMaximumPageSize+1 {
			limitValues++
		}
	}
	if limitValues != 1 {
		t.Fatalf("limit+1 argument count = %d, want 1", limitValues)
	}
	wantOrder := "ORDER BY sort_known DESC, sort_number DESC, sort_text COLLATE utf8mb4_bin DESC, " +
		"bucket_start DESC, dimension_key COLLATE utf8mb4_bin ASC, breakdown_site_id ASC"
	if !strings.Contains(statement, wantOrder) {
		t.Fatalf("metric order tuple does not match HTTP order:\n%s", statement)
	}
}

func TestStatisticsExportOrderTermsPutUserSortBeforeStableIdentity(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      []string
	}{
		{
			name: "metric asc keeps unknown last", sortBy: "request_count", sortOrder: "asc",
			want: []string{"sort_known DESC", "sort_number ASC", "sort_text COLLATE utf8mb4_bin ASC",
				"bucket_start ASC", "dimension_key COLLATE utf8mb4_bin ASC", "breakdown_site_id ASC"},
		},
		{
			name: "metric desc keeps unknown last", sortBy: "active_users", sortOrder: "desc",
			want: []string{"sort_known DESC", "sort_number DESC", "sort_text COLLATE utf8mb4_bin DESC",
				"bucket_start DESC", "dimension_key COLLATE utf8mb4_bin ASC", "breakdown_site_id ASC"},
		},
		{
			name: "name", sortBy: "name", sortOrder: "desc",
			want: []string{"sort_text COLLATE utf8mb4_bin DESC", "bucket_start DESC",
				"dimension_key COLLATE utf8mb4_bin ASC", "breakdown_site_id ASC"},
		},
		{
			name: "bucket", sortBy: "bucket_start", sortOrder: "asc",
			want: []string{"sort_number ASC", "sort_text COLLATE utf8mb4_bin ASC",
				"dimension_key COLLATE utf8mb4_bin ASC", "breakdown_site_id ASC"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			terms, err := statisticsExportOrderTerms("customer", testCase.sortBy, testCase.sortOrder,
				StatisticsExportRowCursor{EntityID: 12})
			if err != nil {
				t.Fatalf("order terms: %v", err)
			}
			got := make([]string, 0, len(terms))
			for _, term := range terms {
				got = append(got, term.Expression+" "+term.Direction)
			}
			if strings.Join(got, "|") != strings.Join(testCase.want, "|") {
				t.Fatalf("order terms = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestStatisticsExportSelectedItemsPushesNameAndBucketCursorBeforeExpansion(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		fragments []string
	}{
		{
			name: "name desc", sortBy: "name", sortOrder: "desc",
			fragments: []string{
				"r.dimension_name COLLATE utf8mb4_bin < ?", "b.bucket_start < ?",
				"ORDER BY r.dimension_name COLLATE utf8mb4_bin DESC, b.bucket_start DESC",
			},
		},
		{
			name: "bucket asc", sortBy: "bucket_start", sortOrder: "asc",
			fragments: []string{
				"b.bucket_start > ?", "r.dimension_name COLLATE utf8mb4_bin > ?",
				"ORDER BY b.bucket_start ASC, r.dimension_name COLLATE utf8mb4_bin ASC",
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			statement, args, err := statisticsExportSelectedItemsSQL(
				"customer", testCase.sortBy, testCase.sortOrder,
				StatisticsExportRowCursor{
					Initialized: true, EntityID: 12, DimensionKey: "12", SortText: "same-name",
					BucketStart: 1_767_200_400,
				},
				StatisticsExportMaximumPageSize,
			)
			if err != nil {
				t.Fatalf("selected items SQL: %v", err)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(statement, fragment) {
					t.Fatalf("selected-item keyset is missing %q:\n%s", fragment, statement)
				}
			}
			if !strings.Contains(statement, "LIMIT ?") ||
				len(args) == 0 || args[len(args)-1] != StatisticsExportMaximumPageSize+2 {
				t.Fatalf("selected-item limit args = %#v", args)
			}
		})
	}
}

func TestCheckedStatisticsExportCandidateRowsRejectsOverflow(t *testing.T) {
	const maximumInt64 = int64(^uint64(0) >> 1)
	if rows, ok := checkedStatisticsExportCandidateRows(49, 103); !ok || rows != 5047 {
		t.Fatalf("checked product = %d, %t", rows, ok)
	}
	if rows, ok := checkedStatisticsExportCandidateRows(100, 10_000); !ok ||
		rows != StatisticsExportMaximumCandidateRows {
		t.Fatalf("capacity boundary = %d, %t", rows, ok)
	}
	if rows, ok := checkedStatisticsExportCandidateRows(100, 10_001); !ok ||
		rows <= StatisticsExportMaximumCandidateRows {
		t.Fatalf("over-capacity estimate = %d, %t", rows, ok)
	}
	if _, ok := checkedStatisticsExportCandidateRows(maximumInt64, 2); ok {
		t.Fatal("overflowing candidate estimate was accepted")
	}
	if _, ok := checkedStatisticsExportCandidateRows(-1, 1); ok {
		t.Fatal("negative candidate estimate was accepted")
	}
}

func TestStatisticsExportCalendarBucketsDoNotDependOnMySQLTimezone(t *testing.T) {
	statement, args := statisticsExportBucketsSQL(StatisticsReadRequest{
		Granularity: "month", StartTimestamp: 1_972_224_000, EndTimestamp: 1_974_902_400,
	}, 1)
	lower := strings.ToLower(statement)
	if strings.Contains(lower, "from_unixtime") || strings.Contains(lower, "unix_timestamp") {
		t.Fatalf("calendar bucket SQL depends on MySQL session timezone:\n%s", statement)
	}
	for _, fragment := range []string{
		"calendar_origin(local_start)", "TIMESTAMPADD(SECOND, ? + 28800",
		"TIMESTAMPDIFF(SECOND", "- 28800 AS bucket_start", "- 28800 AS bucket_end",
	} {
		if !strings.Contains(statement, fragment) {
			t.Fatalf("calendar bucket SQL is missing %q:\n%s", fragment, statement)
		}
	}
	if len(args) != 2 || args[0] != int64(1_972_224_000) || args[1] != int64(1) {
		t.Fatalf("calendar bucket args = %#v", args)
	}
}

func TestStatisticsExportNonHourlyVerificationCutoffUsesShanghaiEpochArithmetic(t *testing.T) {
	statement, _, err := statisticsExportRowsStatement(StatisticsExportRowQuery{
		Request: StatisticsReadRequest{
			Scope: "account", Granularity: "month",
			StartTimestamp: 1_972_224_000, EndTimestamp: 1_974_902_400,
			StartDateKey: 20320701, EndDateKey: 20320801,
			SiteIDs: []int64{1}, AccountIDs: []int64{2},
		},
		SortBy: "bucket_start", SortOrder: "asc", Now: 1_975_000_000,
		UsageDelayMinutes: 5, Limit: 100,
	})
	if err != nil {
		t.Fatalf("build non-hourly export SQL: %v", err)
	}
	lower := strings.ToLower(statement)
	if strings.Contains(lower, "from_unixtime") || strings.Contains(lower, "unix_timestamp") {
		t.Fatalf("non-hourly export SQL depends on MySQL session timezone:\n%s", statement)
	}
	for _, fragment := range []string{
		"cw.hour_ts + 28800", "TIMESTAMPDIFF(SECOND", "INTERVAL 1 DAY", ") - 28800",
	} {
		if !strings.Contains(statement, fragment) {
			t.Fatalf("non-hourly verification cutoff is missing %q:\n%s", fragment, statement)
		}
	}
}
