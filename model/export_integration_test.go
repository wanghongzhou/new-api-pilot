package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestMySQLExportClaimTakeoverLeaseAndStaleTokenCAS(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()

	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	ownerID := exportIntegrationOwnerID(t, tx)
	repository := NewExportRepository(tx)
	now := int64(1_752_400_800)
	job := exportIntegrationPendingJob(ownerID, "claim-lifecycle", now)
	if err := repository.Create(ctx, &job); err != nil {
		t.Fatalf("create export job: %v", err)
	}

	first, err := repository.Claim(ctx, now, "first-claim-token", now+300)
	if err != nil || first == nil {
		t.Fatalf("first Claim = %#v, %v", first, err)
	}
	if first.Job.ID != job.ID || first.Job.AttemptCount != 1 || first.Job.Status != "running" {
		t.Fatalf("first claim job = %#v", first.Job)
	}
	rateSnapshot := json.RawMessage(`{"sites":[{"site_id":"1","site_name":"A","quota_per_unit":"500000","usd_exchange_rate":"7.2","rate_source":"site","rate_updated_at":1}]}`)
	if err := repository.SetRateSnapshot(ctx, job.ID, "first-claim-token", rateSnapshot, now+1); err != nil {
		t.Fatalf("SetRateSnapshot: %v", err)
	}
	if err := repository.SetTemporaryPath(ctx, job.ID, "first-claim-token", ".stale.tmp", now+1); err != nil {
		t.Fatalf("SetTemporaryPath: %v", err)
	}
	recovered, err := repository.RecoverRunning(ctx, now+2, true)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if len(recovered) != 1 || recovered[0].JobID != job.ID || recovered[0].FilePath != ".stale.tmp" || recovered[0].Failed {
		t.Fatalf("recovered = %#v", recovered)
	}
	var afterTakeover ExportJob
	if err := tx.First(&afterTakeover, job.ID).Error; err != nil {
		t.Fatalf("read takeover job: %v", err)
	}
	if afterTakeover.Status != "pending" || afterTakeover.AttemptCount != 1 || afterTakeover.ClaimToken != nil ||
		afterTakeover.LeaseExpiresAt != nil || afterTakeover.FilePath == nil {
		t.Fatalf("takeover state = %#v", afterTakeover)
	}
	if err := repository.Heartbeat(ctx, job.ID, "first-claim-token", now+3, now+303, 10); !errors.Is(err, ErrExportClaimLost) {
		t.Fatalf("stale heartbeat error = %v", err)
	}
	if err := repository.ClearArtifactPath(ctx, job.ID, ".stale.tmp", now+3); err != nil {
		t.Fatalf("ClearArtifactPath: %v", err)
	}

	second, err := repository.Claim(ctx, now+3, "second-claim-token", now+303)
	if err != nil || second == nil {
		t.Fatalf("second Claim = %#v, %v", second, err)
	}
	if second.Job.AttemptCount != 2 {
		t.Fatalf("second claim attempt_count = %d, want 2", second.Job.AttemptCount)
	}
	if canonicalExportIntegrationJSON(t, second.Job.RateSnapshot) != canonicalExportIntegrationJSON(t, rateSnapshot) {
		t.Fatalf("retry rate snapshot = %s, want %s", second.Job.RateSnapshot, rateSnapshot)
	}
	if err := repository.SetRateSnapshot(ctx, job.ID, "second-claim-token", json.RawMessage(`{"sites":[]}`), now+3); !errors.Is(err, ErrExportClaimLost) {
		t.Fatalf("retry overwrote frozen rate snapshot: %v", err)
	}
	if err := repository.Heartbeat(ctx, job.ID, "first-claim-token", now+4, now+304, 20); !errors.Is(err, ErrExportClaimLost) {
		t.Fatalf("old token heartbeat error = %v", err)
	}
	if err := repository.Complete(ctx, job.ID, "first-claim-token", "old.csv", "old.csv", 1, 1, now+4, now+3604); !errors.Is(err, ErrExportClaimLost) {
		t.Fatalf("old token completion error = %v", err)
	}
	if err := repository.Heartbeat(ctx, job.ID, "second-claim-token", now+4, now+604, 42); err != nil {
		t.Fatalf("current heartbeat: %v", err)
	}
	if err := repository.Complete(ctx, job.ID, "second-claim-token", "final.csv", "final.csv", 123, 456, now+5, now+3605); err != nil {
		t.Fatalf("current completion: %v", err)
	}
	var completed ExportJob
	if err := tx.First(&completed, job.ID).Error; err != nil {
		t.Fatalf("read completed job: %v", err)
	}
	if completed.Status != "success" || completed.Progress != 100 || completed.AttemptCount != 2 ||
		completed.ActiveKey != nil || completed.ClaimToken != nil || completed.LeaseExpiresAt != nil ||
		completed.FilePath == nil || *completed.FilePath != "final.csv" || completed.FileSize != 123 || completed.RowCount != 456 {
		t.Fatalf("completed state = %#v", completed)
	}
}

func TestMySQLExportRecoveryRetriesStaleArtifactCleanupWithoutChangingAttempt(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	ownerID := exportIntegrationOwnerID(t, tx)
	repository := NewExportRepository(tx)
	now := int64(1_752_400_800)
	job := exportIntegrationPendingJob(ownerID, "stale-cleanup", now)
	job.Status = "pending"
	job.AttemptCount = 1
	job.FilePath = pointerString(".stale-retry.tmp")
	if err := repository.Create(ctx, &job); err != nil {
		t.Fatalf("create stale export: %v", err)
	}

	for pass := 0; pass < 2; pass++ {
		recovered, err := repository.RecoverRunning(ctx, now+int64(pass), pass == 0)
		if err != nil {
			t.Fatalf("recovery pass %d: %v", pass, err)
		}
		if len(recovered) != 1 || recovered[0].FilePath != ".stale-retry.tmp" {
			t.Fatalf("recovery pass %d = %#v", pass, recovered)
		}
		var current ExportJob
		if err := tx.First(&current, job.ID).Error; err != nil {
			t.Fatalf("read recovery pass %d: %v", pass, err)
		}
		if current.Status != "pending" || current.AttemptCount != 1 || current.FilePath == nil {
			t.Fatalf("recovery pass %d changed state = %#v", pass, current)
		}
	}
}

func TestMySQLExportListForUserFiltersMultipleStatusesWithIN(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	ownerID := exportIntegrationOwnerID(t, tx)
	repository := NewExportRepository(tx)
	now := int64(1_752_400_800)
	statuses := []string{"pending", "success", "failed"}
	for index, status := range statuses {
		job := exportIntegrationPendingJob(ownerID, fmt.Sprintf("list-status-%d", index), now+int64(index))
		job.Status = status
		if status != "pending" {
			job.ActiveKey = nil
			finished := now + int64(index)
			job.FinishedAt = &finished
		}
		if err := repository.Create(ctx, &job); err != nil {
			t.Fatalf("create %s export job: %v", status, err)
		}
	}
	jobs, total, err := repository.ListForUser(
		ctx, ownerID, []string{"pending", "failed"}, "", "", "created_at", "asc", 20, 0,
	)
	if err != nil || total != 2 || len(jobs) != 2 || jobs[0].Status != "pending" || jobs[1].Status != "failed" {
		t.Fatalf("multi-status export jobs = %#v total=%d err=%v", jobs, total, err)
	}
}

func TestMySQLStatisticsExportFinalRowsUseBoundedCursor(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	now := int64(1_752_400_800)
	start := int64(1_752_390_000)
	statisticsStart := start
	site := Site{
		Name: "Final Row Site", BaseURL: fmt.Sprintf("https://final-row-%d.example.com", time.Now().UnixNano()),
		ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "healthy", StatisticsStartAt: &statisticsStart,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create final row site: %v", err)
	}
	for bucket := 0; bucket < 6; bucket++ {
		hour := start + int64(bucket)*3600
		if err := tx.Exec(`INSERT INTO site_stat_hourly
  (site_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'complete', ?, ?, ?)`,
			site.ID, hour, bucket+1, (bucket+1)*10, (bucket+1)*100, bucket+1, now, now, now).Error; err != nil {
			t.Fatalf("insert final row metric %d: %v", bucket, err)
		}
	}
	repository := NewStatisticsRepository(tx)
	request := StatisticsReadRequest{
		Scope: "global", Granularity: "hour", StartTimestamp: start, EndTimestamp: start + 6*3600,
		SiteIDs: []int64{site.ID},
	}
	first, err := repository.LoadExportRows(ctx, StatisticsExportRowQuery{
		Request: request, SortBy: "bucket_start", SortOrder: "asc", Now: now, Limit: 2,
	})
	if err != nil {
		t.Fatalf("first final row page: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first query rows = %d, want limit+1=3", len(first))
	}
	cursor := first[1].Cursor()
	second, err := repository.LoadExportRows(ctx, StatisticsExportRowQuery{
		Request: request, SortBy: "bucket_start", SortOrder: "asc", Now: now, Limit: 2, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("second final row page: %v", err)
	}
	if len(second) != 3 || second[0].BucketStart != start+2*3600 || second[0].BreakdownSiteID != site.ID {
		t.Fatalf("second final row page = %#v", second)
	}
	third, err := repository.LoadExportRows(ctx, StatisticsExportRowQuery{
		Request: request, SortBy: "bucket_start", SortOrder: "asc", Now: now, Limit: 2,
		Cursor: second[1].Cursor(),
	})
	if err != nil {
		t.Fatalf("third final row page: %v", err)
	}
	if len(third) != 2 || third[0].BucketStart != start+4*3600 || third[1].BucketStart != start+5*3600 {
		t.Fatalf("third final row page = %#v", third)
	}
}

func TestMySQLStatisticsExportFinalRowsAllowLongRanges(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin long range transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	now := int64(1_752_400_800)
	statisticsStart := int64(31_507_200)
	site := Site{
		Name: "Long Range Site", BaseURL: fmt.Sprintf("https://long-range-%d.example.com", time.Now().UnixNano()),
		ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "healthy", StatisticsStartAt: &statisticsStart,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create long range site: %v", err)
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	cases := []struct {
		name        string
		granularity string
		start       time.Time
		end         time.Time
	}{
		{name: "hour_over_31_days", granularity: "hour", start: time.Date(2025, 1, 1, 0, 0, 0, 0, location), end: time.Date(2025, 2, 2, 0, 0, 0, 0, location)},
		{name: "day_over_2_years", granularity: "day", start: time.Date(2020, 1, 1, 0, 0, 0, 0, location), end: time.Date(2023, 1, 2, 0, 0, 0, 0, location)},
		{name: "month_over_20_years", granularity: "month", start: time.Date(2000, 1, 1, 0, 0, 0, 0, location), end: time.Date(2021, 2, 1, 0, 0, 0, 0, location)},
		{name: "year_full_history", granularity: "year", start: time.Date(1971, 1, 1, 0, 0, 0, 0, location), end: time.Date(2026, 1, 1, 0, 0, 0, 0, location)},
	}
	repository := NewStatisticsRepository(tx)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := StatisticsReadRequest{
				Scope: "global", Granularity: testCase.granularity,
				StartTimestamp: testCase.start.Unix(), EndTimestamp: testCase.end.Unix(),
				StartDateKey: testCase.start.Year()*10000 + int(testCase.start.Month())*100 + testCase.start.Day(),
				EndDateKey:   testCase.end.Year()*10000 + int(testCase.end.Month())*100 + testCase.end.Day(),
				SiteIDs:      []int64{site.ID},
			}
			rows, err := repository.LoadExportRows(ctx, StatisticsExportRowQuery{
				Request: request, SortBy: "bucket_start", SortOrder: "asc", Now: now, Limit: 2,
			})
			if err != nil {
				t.Fatalf("long range rows: %v", err)
			}
			if len(rows) != 3 || rows[0].BucketStart != testCase.start.Unix() {
				t.Fatalf("long range first page = %#v", rows)
			}
		})
	}
}

func TestMySQLStatisticsExportFinalRowsPageEveryScopeWithoutDuplicates(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin every-scope transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	now := int64(1_752_400_800)
	start := int64(1_752_390_000)
	statisticsStart := start
	sites := []Site{
		{Name: "Scope Site A", BaseURL: fmt.Sprintf("https://scope-a-%d.example.com", time.Now().UnixNano()), ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized", StatisticsStatus: "ready", HealthStatus: "healthy", StatisticsStartAt: &statisticsStart, CreatedAt: now, UpdatedAt: now},
		{Name: "Scope Site B", BaseURL: fmt.Sprintf("https://scope-b-%d.example.com", time.Now().UnixNano()), ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized", StatisticsStatus: "ready", HealthStatus: "healthy", StatisticsStartAt: &statisticsStart, CreatedAt: now, UpdatedAt: now},
	}
	for index := range sites {
		if err := tx.Create(&sites[index]).Error; err != nil {
			t.Fatalf("create scope site %d: %v", index, err)
		}
	}
	customers := []Customer{
		{Name: "Scope Customer A", Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
		{Name: "Scope Customer B", Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
	}
	for index := range customers {
		if err := tx.Create(&customers[index]).Error; err != nil {
			t.Fatalf("create scope customer %d: %v", index, err)
		}
	}
	accounts := []Account{
		{SiteID: sites[0].ID, CustomerID: customers[0].ID, RemoteUserID: 101, RemoteCreatedAt: start - 3600, Username: "a", DisplayName: "A", RemoteState: "normal", ManagedStatus: "active", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, CustomerID: customers[0].ID, RemoteUserID: 102, RemoteCreatedAt: start - 3600, Username: "b", DisplayName: "B", RemoteState: "normal", ManagedStatus: "active", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[0].ID, CustomerID: customers[1].ID, RemoteUserID: 103, RemoteCreatedAt: start - 3600, Username: "c", DisplayName: "C", RemoteState: "normal", ManagedStatus: "active", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
	}
	for index := range accounts {
		if err := tx.Create(&accounts[index]).Error; err != nil {
			t.Fatalf("create scope account %d: %v", index, err)
		}
	}
	for bucket := 0; bucket < 3; bucket++ {
		hour := start + int64(bucket)*3600
		for siteIndex, site := range sites {
			value := (bucket+1)*100 + siteIndex
			if err := tx.Exec(`INSERT INTO site_stat_hourly
  (site_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'complete', ?, ?, ?)`, site.ID, hour, value, value, value, 1, now, now, now).Error; err != nil {
				t.Fatalf("insert site metric: %v", err)
			}
		}
		for _, account := range accounts {
			value := bucket + int(account.ID%10) + 1
			if err := tx.Exec(`INSERT INTO account_stat_hourly
  (account_id, hour_ts, request_count, quota, token_used, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 'complete', ?, ?, ?)`, account.ID, hour, value, value, value, now, now, now).Error; err != nil {
				t.Fatalf("insert account metric: %v", err)
			}
			if err := tx.Exec(`INSERT INTO customer_stat_hourly
  (customer_id, site_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 1, 'complete', ?, ?, ?)`, account.CustomerID, account.SiteID, hour, value, value, value, now, now, now).Error; err != nil {
				t.Fatalf("insert customer metric: %v", err)
			}
		}
		for _, site := range sites {
			for modelIndex, name := range []string{"Model-A", "model-a", "模型"} {
				value := bucket*10 + modelIndex + 1
				if err := tx.Exec(`INSERT INTO model_stat_hourly
  (site_id, model_name, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 1, 'complete', ?, ?, ?)`, site.ID, name, hour, value, value, value, now, now, now).Error; err != nil {
					t.Fatalf("insert model metric: %v", err)
				}
			}
		}
		for _, key := range []StatisticsChannelKey{{SiteID: sites[0].ID, ChannelID: 0}, {SiteID: sites[0].ID, ChannelID: 7}, {SiteID: sites[1].ID, ChannelID: 8}} {
			value := bucket + int(key.ChannelID) + 1
			if err := tx.Exec(`INSERT INTO channel_stat_hourly
  (site_id, channel_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 1, 'complete', ?, ?, ?)`, key.SiteID, key.ChannelID, hour, value, value, value, now, now, now).Error; err != nil {
				t.Fatalf("insert channel metric: %v", err)
			}
		}
	}
	cases := []struct {
		scope string
		rows  int
		apply func(*StatisticsReadRequest)
	}{
		{scope: "global", rows: 6},
		{scope: "site", rows: 6},
		{scope: "customer", rows: 9, apply: func(request *StatisticsReadRequest) { request.CustomerIDs = []int64{customers[0].ID, customers[1].ID} }},
		{scope: "account", rows: 9, apply: func(request *StatisticsReadRequest) {
			request.AccountIDs = []int64{accounts[0].ID, accounts[1].ID, accounts[2].ID}
		}},
		{scope: "model", rows: 18, apply: func(request *StatisticsReadRequest) { request.ModelNames = []string{"Model-A", "model-a", "模型"} }},
		{scope: "channel", rows: 9, apply: func(request *StatisticsReadRequest) {
			request.ChannelKeys = []StatisticsChannelKey{{SiteID: sites[0].ID, ChannelID: 0}, {SiteID: sites[0].ID, ChannelID: 7}, {SiteID: sites[1].ID, ChannelID: 8}}
		}},
	}
	repository := NewStatisticsRepository(tx)
	for _, testCase := range cases {
		t.Run(testCase.scope, func(t *testing.T) {
			request := StatisticsReadRequest{
				Scope: testCase.scope, Granularity: "hour", StartTimestamp: start, EndTimestamp: start + 3*3600,
				SiteIDs: []int64{sites[0].ID, sites[1].ID},
			}
			if testCase.apply != nil {
				testCase.apply(&request)
			}
			sorts := []struct {
				by    string
				order string
			}{
				{by: "quota", order: "desc"},
				{by: "request_count", order: "asc"},
				{by: "bucket_start", order: "desc"},
			}
			if testCase.scope != "global" {
				sorts = append(sorts, struct {
					by    string
					order string
				}{by: "name", order: "asc"})
			}
			for _, sortCase := range sorts {
				t.Run(sortCase.by+"_"+sortCase.order, func(t *testing.T) {
					cursor := StatisticsExportRowCursor{}
					seen := map[string]struct{}{}
					pages := 0
					for {
						rows, err := repository.LoadExportRows(ctx, StatisticsExportRowQuery{
							Request: request, SortBy: sortCase.by, SortOrder: sortCase.order,
							Now: now, Limit: 2, Cursor: cursor,
						})
						if err != nil {
							t.Fatalf("load page %d: %v", pages+1, err)
						}
						if len(rows) > 3 {
							t.Fatalf("query returned %d rows, want <= limit+1", len(rows))
						}
						if len(rows) == 0 {
							break
						}
						emit := len(rows)
						if emit > 2 {
							emit = 2
						}
						pages++
						for _, row := range rows[:emit] {
							key := statisticsExportIntegrationRowKey(testCase.scope, row)
							if _, duplicate := seen[key]; duplicate {
								t.Fatalf("duplicate row %q", key)
							}
							seen[key] = struct{}{}
						}
						cursor = rows[emit-1].Cursor()
						if len(rows) <= 2 {
							break
						}
					}
					if len(seen) != testCase.rows || pages < 3 {
						t.Fatalf("scope rows=%d pages=%d, want %d/>=3", len(seen), pages, testCase.rows)
					}
				})
			}
		})
	}
}

func TestMySQLStatisticsExportKeepsCompleteLogicalTotalsAcrossPhysicalPageBoundary(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()

	tests := []struct {
		name        string
		siteCount   int
		bucketCount int
	}{
		{name: "three_sites", siteCount: 3, bucketCount: 1667},
		{name: "forty_nine_sites", siteCount: 49, bucketCount: 103},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			tx := database.GORM.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
			if tx.Error != nil {
				t.Fatalf("begin logical-page transaction: %v", tx.Error)
			}
			defer func() { _ = tx.Rollback().Error }()

			location := time.FixedZone("Asia/Shanghai", 8*60*60)
			start := time.Date(2036, time.January, 1, 0, 0, 0, 0, location).Unix()
			end := start + int64(testCase.bucketCount)*3600
			now := end + 24*3600
			nonce := time.Now().UnixNano()
			sites := make([]Site, testCase.siteCount)
			for index := range sites {
				sites[index] = Site{
					Name:          fmt.Sprintf("Logical Page Site %02d", index),
					BaseURL:       fmt.Sprintf("https://logical-page-%d-%02d.example.com", nonce, index),
					ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
					StatisticsStatus: "ready", HealthStatus: "healthy",
					StatisticsStartAt: &start, StatisticsEndAt: &end,
					CreatedAt: now, UpdatedAt: now,
				}
			}
			if err := tx.CreateInBatches(&sites, 100).Error; err != nil {
				t.Fatalf("create logical-page sites: %v", err)
			}

			windows := make([]CollectionWindow, 0, testCase.siteCount*testCase.bucketCount)
			metrics := make([]SiteStatHourly, 0, testCase.siteCount*testCase.bucketCount)
			for _, site := range sites {
				for bucket := 0; bucket < testCase.bucketCount; bucket++ {
					hour := start + int64(bucket)*3600
					verifiedAt := hour + 3600
					windows = append(windows, CollectionWindow{
						SiteID: site.ID, HourTS: hour, Status: CollectionWindowStatusComplete,
						FetchedRows: 1, SourceHash: fmt.Sprintf("%064x", site.ID+int64(bucket)),
						VerifiedAt: &verifiedAt, UpdatedAt: now,
					})
					metrics = append(metrics, SiteStatHourly{
						SiteID: site.ID, HourTS: hour, RequestCount: 1, Quota: 1, TokenUsed: 1,
						ActiveUsers: 1, DataStatus: UsageAggregationStatusComplete,
						LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
					})
				}
			}
			if err := tx.CreateInBatches(&windows, 500).Error; err != nil {
				t.Fatalf("create logical-page windows: %v", err)
			}
			if err := tx.CreateInBatches(&metrics, 500).Error; err != nil {
				t.Fatalf("create logical-page metrics: %v", err)
			}

			query := StatisticsExportRowQuery{
				Request: StatisticsReadRequest{
					Scope: "global", Granularity: "hour", StartTimestamp: start, EndTimestamp: end,
					SiteIDs: func() []int64 {
						ids := make([]int64, len(sites))
						for index := range sites {
							ids[index] = sites[index].ID
						}
						return ids
					}(),
				},
				SortBy: "bucket_start", SortOrder: "asc", Now: now,
				Limit: StatisticsExportMaximumPageSize,
			}
			repository := NewStatisticsRepository(tx)
			first, err := repository.LoadExportRows(ctx, query)
			if err != nil {
				t.Fatalf("load first logical-page rows: %v", err)
			}
			if len(first) != StatisticsExportMaximumPageSize+1 {
				t.Fatalf("first query rows = %d, want lookahead %d", len(first), StatisticsExportMaximumPageSize+1)
			}
			query.Cursor = first[StatisticsExportMaximumPageSize-1].Cursor()
			second, err := repository.LoadExportRows(ctx, query)
			if err != nil {
				t.Fatalf("load second logical-page rows: %v", err)
			}
			rows := append(append([]StatisticsExportRow(nil), first[:StatisticsExportMaximumPageSize]...), second...)
			wantRows := testCase.siteCount * testCase.bucketCount
			if len(rows) != wantRows {
				t.Fatalf("physical rows = %d, want %d", len(rows), wantRows)
			}
			boundaryLeft := rows[StatisticsExportMaximumPageSize-1]
			boundaryRight := rows[StatisticsExportMaximumPageSize]
			if boundaryLeft.BucketStart != boundaryRight.BucketStart {
				t.Fatalf("page boundary did not split the intended logical item: %d != %d",
					boundaryLeft.BucketStart, boundaryRight.BucketStart)
			}
			wantTotal := strconv.Itoa(testCase.siteCount)
			for index, row := range rows {
				if row.RequestCount == nil || *row.RequestCount != wantTotal ||
					row.Quota == nil || *row.Quota != wantTotal ||
					row.TokenUsed == nil || *row.TokenUsed != wantTotal ||
					row.SiteQuota == nil || *row.SiteQuota != "1" ||
					row.DataStatus != UsageAggregationStatusComplete {
					t.Fatalf("row %d lost complete logical aggregate: %#v", index, row)
				}
			}
			if !reflectExportLogicalTotalsEqual(boundaryLeft, boundaryRight) {
				t.Fatalf("cross-page logical totals differ: left=%#v right=%#v", boundaryLeft, boundaryRight)
			}
		})
	}
}

func TestMySQLStatisticsExportCapacityGateRunsBeforeLargeCandidateCTE(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin capacity-gate transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()

	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2037, time.January, 1, 0, 0, 0, 0, location).Unix()
	statisticsStart := start
	now := start + 3600
	nonce := time.Now().UnixNano()
	sites := make([]Site, 2)
	for index := range sites {
		sites[index] = Site{
			Name:          fmt.Sprintf("Capacity Gate Site %d", index),
			BaseURL:       fmt.Sprintf("https://capacity-gate-%d-%d.example.com", nonce, index),
			ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
			StatisticsStatus: "ready", HealthStatus: "healthy", StatisticsStartAt: &statisticsStart,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := tx.Create(&sites).Error; err != nil {
		t.Fatalf("create capacity-gate sites: %v", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := NewStatisticsRepository(tx).LoadExportRows(requestContext, StatisticsExportRowQuery{
		Request: StatisticsReadRequest{
			Scope: "site", Granularity: "hour", StartTimestamp: start,
			EndTimestamp: start + 500_001*3600,
			SiteIDs:      []int64{sites[0].ID, sites[1].ID},
		},
		SortBy: "quota", SortOrder: "desc", Now: now, Limit: StatisticsExportMaximumPageSize,
	})
	if !errors.Is(err, ErrStatisticsExportCapacity) {
		t.Fatalf("capacity gate error = %v", err)
	}
	if requestContext.Err() != nil {
		t.Fatalf("capacity gate reached the large candidate CTE: %v", requestContext.Err())
	}
}

func reflectExportLogicalTotalsEqual(left, right StatisticsExportRow) bool {
	return left.BucketStart == right.BucketStart && left.BucketEnd == right.BucketEnd &&
		stringPointerEqual(left.RequestCount, right.RequestCount) &&
		stringPointerEqual(left.Quota, right.Quota) &&
		stringPointerEqual(left.TokenUsed, right.TokenUsed) &&
		stringPointerEqual(left.ActiveUsers, right.ActiveUsers) &&
		left.DataStatus == right.DataStatus && left.IsFinal == right.IsFinal &&
		int64PointerEqual(left.AsOf, right.AsOf)
}

func stringPointerEqual(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func int64PointerEqual(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func TestMySQLStatisticsExportSparseFinalRowsAndExplainBoundedPlan(t *testing.T) {
	database, ctx := openExportIntegrationDatabase(t)
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin sparse-row transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()

	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2035, time.January, 1, 0, 0, 0, 0, location).Unix()
	end := start + 3600
	now := end + 24*3600
	statisticsStart := start
	statisticsEnd := end
	nonce := time.Now().UnixNano()
	sites := make([]Site, 6000)
	for index := range sites {
		sites[index] = Site{
			Name:          fmt.Sprintf("Sparse Export Site %04d", index),
			BaseURL:       fmt.Sprintf("https://sparse-export-%d-%04d.example.com", nonce, index),
			ConfigVersion: 1, ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
			StatisticsStatus: "ready", HealthStatus: "healthy",
			StatisticsStartAt: &statisticsStart, StatisticsEndAt: &statisticsEnd,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := tx.CreateInBatches(&sites, 250).Error; err != nil {
		t.Fatalf("create sparse export sites: %v", err)
	}

	cursorSite := sites[999]
	query := StatisticsExportRowQuery{
		Request: StatisticsReadRequest{
			Scope: "site", Granularity: "hour", StartTimestamp: start, EndTimestamp: end,
		},
		SortBy: "bucket_start", SortOrder: "asc", Now: now,
		Limit: StatisticsExportMaximumPageSize,
		Cursor: StatisticsExportRowCursor{
			Initialized: true, EntityID: cursorSite.ID, DimensionSiteID: cursorSite.ID,
			DimensionKey: strconv.FormatInt(cursorSite.ID, 10), BucketStart: start,
			SortKnown: 1, SortNumber: start, SortText: cursorSite.Name, BreakdownSiteID: cursorSite.ID,
		},
	}
	statement, args, err := statisticsExportRowsStatement(query)
	if err != nil {
		t.Fatalf("build sparse final-row SQL: %v", err)
	}
	var plan string
	if err := tx.Raw("EXPLAIN FORMAT=TREE "+statement, args...).Row().Scan(&plan); err != nil {
		t.Fatalf("explain sparse final-row SQL: %v", err)
	}
	lowerPlan := strings.ToLower(plan)
	if strings.Contains(lowerPlan, " offset ") ||
		!strings.Contains(lowerPlan, "limit: 5001 row(s)") ||
		!strings.Contains(lowerPlan, "limit: 5002 row(s)") ||
		!strings.Contains(lowerPlan, "materialize cte selected_items") ||
		!strings.Contains(lowerPlan, "single-row index lookup on site using primary") {
		t.Fatalf("bounded logical-item cursor plan is missing keyset/limit/index evidence:\n%s", plan)
	}

	rows, err := NewStatisticsRepository(tx).LoadExportRows(ctx, query)
	if err != nil {
		t.Fatalf("load sparse final rows: %v", err)
	}
	if len(rows) != StatisticsExportMaximumPageSize {
		t.Fatalf("sparse final rows = %d, want %d", len(rows), StatisticsExportMaximumPageSize)
	}
	for index, row := range rows {
		want := sites[index+1000]
		if row.EntityID != want.ID || row.DimensionID != strconv.FormatInt(want.ID, 10) ||
			row.BreakdownSiteID != want.ID || row.BucketStart != start {
			t.Fatalf("sparse row %d = %#v, want site %d", index, row, want.ID)
		}
	}
}

func statisticsExportIntegrationRowKey(scope string, row StatisticsExportRow) string {
	switch scope {
	case "global":
		return fmt.Sprintf("%d:%d", row.BucketStart, row.BreakdownSiteID)
	case "site", "customer", "account":
		return fmt.Sprintf("%d:%d:%d", row.EntityID, row.BucketStart, row.BreakdownSiteID)
	case "model":
		return fmt.Sprintf("%d:%s:%d:%d", row.DimensionSiteID, row.DimensionValue, row.BucketStart, row.BreakdownSiteID)
	case "channel":
		return fmt.Sprintf("%d:%d:%d:%d", row.DimensionSiteID, row.EntityID, row.BucketStart, row.BreakdownSiteID)
	default:
		return ""
	}
}

func canonicalExportIntegrationJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode integration JSON %s: %v", raw, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("canonicalize integration JSON: %v", err)
	}
	return string(encoded)
}

func openExportIntegrationDatabase(t *testing.T) (*Database, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("prepare migrations: %v", err)
	}
	if err := NewSeeder(database.SQL).Run(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("prepare settings: %v", err)
	}
	return database, ctx
}

func exportIntegrationOwnerID(t *testing.T, tx *gorm.DB) int64 {
	t.Helper()
	now := time.Now().UnixNano()
	user := PlatformUser{
		Username:     "export-integration-" + strings.ReplaceAll(time.Unix(0, now).Format("150405.000000000"), ".", ""),
		PasswordHash: "integration-only", DisplayName: "Export Integration", Role: "admin", Status: 1,
		SessionVersion: 1, CreatedAt: now / int64(time.Second), UpdatedAt: now / int64(time.Second),
	}
	if err := tx.Create(&user).Error; err != nil || user.ID <= 0 {
		t.Fatalf("create export owner = %d, %v", user.ID, err)
	}
	return user.ID
}

func exportIntegrationPendingJob(ownerID int64, suffix string, now int64) ExportJob {
	activeKey := "integration:" + suffix
	return ExportJob{
		UserID: ownerID, Format: "csv", StatisticsType: "global",
		Filters:    json.RawMessage(`{"start_timestamp":1,"end_timestamp":2}`),
		FilterHash: strings.Repeat("a", 64), ActiveKey: &activeKey,
		Status: "pending", Progress: 0, AttemptCount: 0, NextAttemptAt: now,
		FileSize: 0, RowCount: 0, CreatedAt: now, UpdatedAt: now,
	}
}

func insertExportModelStat(
	t *testing.T,
	ctx context.Context,
	database *Database,
	siteID int64,
	name string,
	hour int64,
	now int64,
) {
	t.Helper()
	if err := database.GORM.WithContext(ctx).Exec(`INSERT INTO model_stat_hourly
  (site_id, model_name, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 1, 1, 1, 1, 'complete', ?, ?, ?)`, siteID, name, hour, now, now, now).Error; err != nil {
		t.Fatalf("insert model stat site=%d name=%q hour=%d: %v", siteID, name, hour, err)
	}
}
