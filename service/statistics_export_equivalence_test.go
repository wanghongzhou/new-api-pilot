package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type statisticsExportComparableRow struct {
	DimensionID       string
	DimensionName     string
	DimensionSiteID   int64
	DimensionValue    string
	BreakdownSiteID   int64
	BreakdownSiteName string
	BucketStart       int64
	BucketEnd         int64
	RequestCount      *string
	Quota             *string
	TokenUsed         *string
	ActiveUsers       *string
	SiteQuota         *string
	DataStatus        string
	IsFinal           bool
	AsOf              *int64
}

func TestMySQLStatisticsExportRowsMatchHTTPStatisticsSnapshot(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	ctx := context.Background()
	start := fixture.start
	now := fixture.service.clock.Now().Unix()

	if err := fixture.database.Model(&model.CollectionWindow{}).
		Where("hour_ts = ?", start+2*3600).
		Updates(map[string]any{"status": model.CollectionWindowStatusUnavailable, "verified_at": nil}).Error; err != nil {
		t.Fatalf("mark unavailable windows: %v", err)
	}
	if err := fixture.database.Where(
		"(site_id = ? AND hour_ts IN ?) OR (site_id = ? AND hour_ts IN ?)",
		fixture.sites[0].ID, []int64{start + 4*3600, start + 5*3600},
		fixture.sites[1].ID, []int64{start + 3*3600, start + 4*3600, start + 5*3600, start + 6*3600},
	).Delete(&model.CollectionWindow{}).Error; err != nil {
		t.Fatalf("delete state-matrix windows: %v", err)
	}
	if err := fixture.database.Model(&model.Site{}).Where("id = ?", fixture.sites[1].ID).
		Updates(map[string]any{
			"management_status": constant.SiteManagementDisabled,
			"statistics_end_at": nil,
			"disabled_at":       start + 3*3600,
		}).Error; err != nil {
		t.Fatalf("pause state-matrix site: %v", err)
	}
	createStatisticsActiveRun(
		t, fixture.database, fixture.sites[0], constant.TaskTypeUsageBackfill,
		model.CollectionTaskStatusRunning, []int64{start + 4*3600}, model.CollectionTaskStatusRunning,
		now, "export-equivalence-backfill",
	)
	createStatisticsActiveRun(
		t, fixture.database, fixture.sites[0], constant.TaskTypeUsageHour,
		model.CollectionTaskStatusRunning, []int64{start + 5*3600}, model.CollectionTaskStatusRunning,
		now, "export-equivalence-pending",
	)
	if err := fixture.database.Model(&model.Customer{}).Where("id = ?", fixture.customers[0].ID).
		Update("statistics_paused_at", start+3*3600).Error; err != nil {
		t.Fatalf("pause state-matrix customer: %v", err)
	}
	if err := fixture.database.Model(&model.Account{}).Where("id = ?", fixture.accounts[0].ID).
		Update("statistics_paused_at", start+3*3600).Error; err != nil {
		t.Fatalf("pause state-matrix account: %v", err)
	}
	if err := fixture.database.Model(&model.Account{}).Where("id = ?", fixture.accounts[2].ID).
		Update("statistics_paused_at", start+3*3600).Error; err != nil {
		t.Fatalf("pause resumable customer account: %v", err)
	}
	resumedAccount := statisticsTestAccount(
		fixture.sites[0].ID, fixture.customers[1].ID, 30, "alpha-resumed", start+6*3600,
	)
	if err := fixture.database.Create(&resumedAccount).Error; err != nil {
		t.Fatalf("create resumed customer account: %v", err)
	}

	tests := []struct {
		scope string
		apply func(*dto.StatisticsQuery)
	}{
		{scope: dto.StatisticsScopeGlobal},
		{scope: dto.StatisticsScopeSite},
		{scope: dto.StatisticsScopeCustomer, apply: func(query *dto.StatisticsQuery) {
			query.CustomerIDs = []int64{fixture.customers[0].ID, fixture.customers[1].ID}
		}},
		{scope: dto.StatisticsScopeAccount, apply: func(query *dto.StatisticsQuery) {
			query.AccountIDs = []int64{
				fixture.accounts[0].ID, fixture.accounts[1].ID,
				fixture.accounts[2].ID, fixture.accounts[3].ID,
			}
		}},
		{scope: dto.StatisticsScopeModel, apply: func(query *dto.StatisticsQuery) {
			query.ModelNames = []string{"Model-A", "model-a"}
		}},
		{scope: dto.StatisticsScopeChannel, apply: func(query *dto.StatisticsQuery) {
			query.ChannelKeys = []string{
				strconv.FormatInt(fixture.sites[0].ID, 10) + ":1",
				strconv.FormatInt(fixture.sites[1].ID, 10) + ":1",
			}
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.scope, func(t *testing.T) {
			base := dto.StatisticsQuery{
				SiteIDs: []int64{fixture.sites[0].ID, fixture.sites[1].ID},
				Page:    1, PageSize: 100,
			}
			if testCase.apply != nil {
				testCase.apply(&base)
			}
			ranges := []struct {
				name        string
				granularity string
				start       int64
				end         int64
			}{
				{name: "hour", granularity: dto.StatisticsGranularityHour, start: start - 3600, end: start + 7*3600},
				{name: "month", granularity: dto.StatisticsGranularityMonth, start: start, end: fixture.monthEnd},
			}
			sorts := []struct {
				by    string
				order string
			}{
				{by: "bucket_start", order: "asc"},
				{by: "request_count", order: "asc"},
				{by: "quota", order: "desc"},
				{by: "active_users", order: "desc"},
			}
			if testCase.scope != dto.StatisticsScopeGlobal {
				sorts = append(sorts, struct {
					by    string
					order string
				}{by: "name", order: "desc"})
			}
			for _, rangeCase := range ranges {
				t.Run(rangeCase.name, func(t *testing.T) {
					for _, sortCase := range sorts {
						t.Run(sortCase.by+"_"+sortCase.order, func(t *testing.T) {
							query := base
							query.StartTimestamp = rangeCase.start
							query.EndTimestamp = rangeCase.end
							query.Granularity = rangeCase.granularity
							query.SortBy = sortCase.by
							query.SortOrder = sortCase.order
							assertStatisticsExportRowsEquivalent(t, ctx, fixture, testCase.scope, query, now)
						})
					}
				})
			}
		})
	}
}

func TestMySQLAccountStatisticsExportActiveUsersCompletenessContract(t *testing.T) {
	fixture := newStatisticsServiceFixture(t)
	ctx := context.Background()
	now := fixture.service.clock.Now().Unix()
	repository := model.NewStatisticsRepository(fixture.database)
	settings, err := repository.LoadFallbackRates(ctx)
	if err != nil {
		t.Fatalf("load export settings: %v", err)
	}
	accountIDs := make([]int64, len(fixture.accounts))
	for index := range fixture.accounts {
		accountIDs[index] = fixture.accounts[index].ID
	}
	load := func(granularity string, start, end int64) []model.StatisticsExportRow {
		t.Helper()
		query := dto.StatisticsQuery{
			StartTimestamp: start, EndTimestamp: end, Granularity: granularity,
			AccountIDs: accountIDs, Page: 1, PageSize: 100,
			SortBy: "bucket_start", SortOrder: "asc",
		}
		request, requestErr := statisticsReadRequest(dto.StatisticsScopeAccount, query)
		if requestErr != nil {
			t.Fatalf("build account export request: %v", requestErr)
		}
		rows, loadErr := repository.LoadExportRows(ctx, model.StatisticsExportRowQuery{
			Request: request, SortBy: query.SortBy, SortOrder: query.SortOrder,
			Now: now, UsageDelayMinutes: settings.UsageDelayMinutes, Limit: 100,
		})
		if loadErr != nil {
			t.Fatalf("load account export rows: %v", loadErr)
		}
		assertStatisticsExportRowsEquivalent(t, ctx, fixture, dto.StatisticsScopeAccount, query, now)
		return rows
	}
	find := func(rows []model.StatisticsExportRow, accountID, bucketStart int64) model.StatisticsExportRow {
		t.Helper()
		dimensionID := strconv.FormatInt(accountID, 10)
		for _, row := range rows {
			if row.DimensionID == dimensionID && row.BucketStart == bucketStart {
				return row
			}
		}
		t.Fatalf("account %s bucket %d is missing from export rows", dimensionID, bucketStart)
		return model.StatisticsExportRow{}
	}

	hourRows := load(dto.StatisticsGranularityHour, fixture.start, fixture.start+2*3600)
	hourActive := find(hourRows, fixture.accounts[0].ID, fixture.start)
	hourZero := find(hourRows, fixture.accounts[0].ID, fixture.start+3600)
	hourMissing := find(hourRows, fixture.accounts[1].ID, fixture.start+3600)
	if stringValue(hourActive.ActiveUsers) != "1" || stringValue(hourZero.ActiveUsers) != "0" ||
		hourMissing.ActiveUsers != nil || hourMissing.DataStatus != model.CollectionWindowStatusMissing {
		t.Fatalf("hour account active users = active:%#v zero:%#v missing:%#v", hourActive, hourZero, hourMissing)
	}

	monthRows := load(dto.StatisticsGranularityMonth, fixture.start, fixture.monthEnd)
	monthActive := find(monthRows, fixture.accounts[0].ID, fixture.start)
	monthZero := find(monthRows, fixture.accounts[2].ID, fixture.start)
	monthPartial := find(monthRows, fixture.accounts[1].ID, fixture.start)
	if stringValue(monthActive.ActiveUsers) != "1" || stringValue(monthZero.ActiveUsers) != "0" ||
		monthPartial.ActiveUsers != nil || monthPartial.DataStatus != model.UsageAggregationStatusPartial ||
		stringValue(monthPartial.RequestCount) != "6" || stringValue(monthPartial.Quota) != "60" ||
		stringValue(monthPartial.TokenUsed) != "600" {
		t.Fatalf("month account active users = active:%#v zero:%#v partial:%#v", monthActive, monthZero, monthPartial)
	}
}

func assertStatisticsExportRowsEquivalent(
	t *testing.T,
	ctx context.Context,
	fixture statisticsServiceFixture,
	scope string,
	query dto.StatisticsQuery,
	now int64,
) {
	t.Helper()
	query.Normalize()
	httpSnapshot, err := fixture.service.ExportSnapshot(ctx, scope, query)
	if err != nil {
		t.Fatalf("load HTTP statistics snapshot: %v", err)
	}
	request, err := statisticsReadRequest(scope, query)
	if err != nil {
		t.Fatalf("build export read request: %v", err)
	}
	repository := model.NewStatisticsRepository(fixture.database)
	settings, err := repository.LoadFallbackRates(ctx)
	if err != nil {
		t.Fatalf("load export settings: %v", err)
	}
	exportRows, err := repository.LoadExportRows(ctx, model.StatisticsExportRowQuery{
		Request: request, SortBy: query.SortBy, SortOrder: query.SortOrder,
		Now: now, UsageDelayMinutes: settings.UsageDelayMinutes, Limit: 5000,
	})
	if err != nil {
		t.Fatalf("load final export rows: %v", err)
	}

	want := flattenHTTPStatisticsRows(t, scope, httpSnapshot)
	got := comparableExportRows(t, scope, exportRows)
	if !reflect.DeepEqual(got, want) {
		maximum := len(got)
		if len(want) > maximum {
			maximum = len(want)
		}
		for index := 0; index < maximum; index++ {
			if index >= len(got) {
				t.Fatalf("row %d missing from export; want %s", index, statisticsComparableRowJSON(want[index]))
			}
			if index >= len(want) {
				t.Fatalf("unexpected export row %d: %s", index, statisticsComparableRowJSON(got[index]))
			}
			if !reflect.DeepEqual(got[index], want[index]) {
				t.Fatalf("row %d (%s) mismatch\n got: %s\nwant: %s", index,
					statisticsComparableRowKey(scope, want[index]),
					statisticsComparableRowJSON(got[index]), statisticsComparableRowJSON(want[index]))
			}
		}
	}
}

func statisticsComparableRowJSON(row statisticsExportComparableRow) string {
	data, _ := json.Marshal(row)
	return string(data)
}

func flattenHTTPStatisticsRows(
	t *testing.T,
	scope string,
	response dto.StatisticsResponse,
) []statisticsExportComparableRow {
	t.Helper()
	result := make([]statisticsExportComparableRow, 0)
	seen := make(map[string]struct{})
	for _, item := range response.Breakdown.Items {
		base, itemScope, err := exportBreakdownBase(item)
		if err != nil || itemScope != scope {
			t.Fatalf("read HTTP breakdown base = %q, %v", itemScope, err)
		}
		dimensionSiteID := int64(0)
		if base.SiteID != nil {
			dimensionSiteID, err = strconv.ParseInt(*base.SiteID, 10, 64)
			if err != nil {
				t.Fatalf("parse dimension site ID %q: %v", *base.SiteID, err)
			}
		}
		dimensionValue := ""
		if scope == dto.StatisticsScopeGlobal {
			dimensionValue = dto.StatisticsScopeGlobal
		}
		switch value := item.(type) {
		case dto.ModelStatisticsBreakdown:
			dimensionValue = value.ModelName
		}
		sites := base.SiteBreakdown
		if len(sites) == 0 {
			site := dto.SiteQuotaBreakdown{Quota: base.Quota, DataStatus: base.DataStatus}
			if base.SiteID != nil {
				site.SiteID = *base.SiteID
			}
			if base.SiteName != nil {
				site.SiteName = *base.SiteName
			}
			sites = []dto.SiteQuotaBreakdown{site}
		}
		for _, site := range sites {
			siteID := int64(0)
			if site.SiteID != "" {
				siteID, err = strconv.ParseInt(site.SiteID, 10, 64)
				if err != nil {
					t.Fatalf("parse breakdown site ID %q: %v", site.SiteID, err)
				}
			}
			row := statisticsExportComparableRow{
				DimensionID: base.DimensionID, DimensionName: base.DimensionName,
				DimensionSiteID: dimensionSiteID, DimensionValue: dimensionValue,
				BreakdownSiteID: siteID, BreakdownSiteName: site.SiteName,
				BucketStart: base.BucketStart, BucketEnd: base.BucketEnd,
				RequestCount: base.RequestCount, Quota: base.Quota, TokenUsed: base.TokenUsed,
				ActiveUsers: base.ActiveUsers, SiteQuota: site.Quota,
				DataStatus: base.DataStatus, IsFinal: base.IsFinal, AsOf: base.AsOf,
			}
			key := statisticsComparableRowKey(scope, row)
			if _, duplicate := seen[key]; duplicate {
				t.Fatalf("duplicate HTTP row %s", key)
			}
			seen[key] = struct{}{}
			result = append(result, row)
		}
	}
	return result
}

func comparableExportRows(
	t *testing.T,
	scope string,
	rows []model.StatisticsExportRow,
) []statisticsExportComparableRow {
	t.Helper()
	result := make([]statisticsExportComparableRow, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, item := range rows {
		row := statisticsExportComparableRow{
			DimensionID: item.DimensionID, DimensionName: item.DimensionName,
			DimensionSiteID: item.DimensionSiteID, DimensionValue: item.DimensionValue,
			BreakdownSiteID: item.BreakdownSiteID, BreakdownSiteName: item.BreakdownSiteName,
			BucketStart: item.BucketStart, BucketEnd: item.BucketEnd,
			RequestCount: item.RequestCount, Quota: item.Quota, TokenUsed: item.TokenUsed,
			ActiveUsers: item.ActiveUsers, SiteQuota: item.SiteQuota,
			DataStatus: item.DataStatus, IsFinal: item.IsFinal, AsOf: item.AsOf,
		}
		key := statisticsComparableRowKey(scope, row)
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate export row %s", key)
		}
		seen[key] = struct{}{}
		result = append(result, row)
	}
	return result
}

func statisticsComparableRowKey(scope string, row statisticsExportComparableRow) string {
	return fmt.Sprintf(
		"%s:%s:%d:%s:%d:%d",
		scope, row.DimensionID, row.DimensionSiteID, row.DimensionValue,
		row.BucketStart, row.BreakdownSiteID,
	)
}
