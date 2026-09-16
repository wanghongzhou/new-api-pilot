package model

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type statisticsCalendarBucket struct {
	Start       int64
	End         int64
	MetricStart int
	MetricEnd   int
}

func TestStatisticsExportCalendarBucketsStayOnShanghaiBoundariesAcrossSessionTimezones(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 1, MaxOpen: 2, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open statistics calendar database: %v", err)
	}
	defer func() { _ = database.Close() }()
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve statistics calendar connection: %v", err)
	}
	defer func() {
		_, _ = connection.ExecContext(context.Background(), "SET time_zone = '+00:00'")
		_ = connection.Close()
	}()
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	tests := []struct {
		name        string
		granularity string
		start       time.Time
		end         time.Time
		want        []statisticsCalendarBucket
	}{
		{
			name: "day boundary", granularity: "day",
			start: time.Date(2032, 7, 1, 0, 0, 0, 0, location),
			end:   time.Date(2032, 7, 3, 0, 0, 0, 0, location),
			want: []statisticsCalendarBucket{
				{Start: time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix(), End: time.Date(2032, 7, 2, 0, 0, 0, 0, location).Unix(), MetricStart: 20320701, MetricEnd: 20320702},
				{Start: time.Date(2032, 7, 2, 0, 0, 0, 0, location).Unix(), End: time.Date(2032, 7, 3, 0, 0, 0, 0, location).Unix(), MetricStart: 20320702, MetricEnd: 20320703},
			},
		},
		{
			name: "month end", granularity: "month",
			start: time.Date(2032, 7, 1, 0, 0, 0, 0, location),
			end:   time.Date(2032, 8, 1, 0, 0, 0, 0, location),
			want: []statisticsCalendarBucket{
				{Start: time.Date(2032, 7, 1, 0, 0, 0, 0, location).Unix(), End: time.Date(2032, 8, 1, 0, 0, 0, 0, location).Unix(), MetricStart: 20320701, MetricEnd: 20320801},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := StatisticsReadRequest{
				Granularity: test.granularity, StartTimestamp: test.start.Unix(), EndTimestamp: test.end.Unix(),
			}
			count, err := statisticsExportBucketCount(request)
			if err != nil {
				t.Fatalf("count calendar buckets: %v", err)
			}
			statement, args := statisticsExportBucketsSQL(request, count)
			query := "WITH " + statement + " SELECT bucket_start, bucket_end, metric_start, metric_end FROM buckets ORDER BY bucket_start"
			for _, timezone := range []string{"+00:00", "-05:00", "+09:30"} {
				if _, err := connection.ExecContext(ctx, "SET time_zone = ?", timezone); err != nil {
					t.Fatalf("set MySQL session timezone %s: %v", timezone, err)
				}
				rows, err := connection.QueryContext(ctx, query, args...)
				if err != nil {
					t.Fatalf("load %s buckets in %s: %v", test.granularity, timezone, err)
				}
				got := make([]statisticsCalendarBucket, 0, count)
				for rows.Next() {
					var bucket statisticsCalendarBucket
					if err := rows.Scan(&bucket.Start, &bucket.End, &bucket.MetricStart, &bucket.MetricEnd); err != nil {
						_ = rows.Close()
						t.Fatalf("scan %s bucket in %s: %v", test.granularity, timezone, err)
					}
					got = append(got, bucket)
				}
				if err := rows.Close(); err != nil {
					t.Fatalf("close %s buckets in %s: %v", test.granularity, timezone, err)
				}
				if !reflect.DeepEqual(got, test.want) {
					t.Fatalf("%s buckets in %s = %#v, want %#v", test.granularity, timezone, got, test.want)
				}
			}
		})
	}
}

func TestStatisticsCapacityIndexesSupportAllActiveQueryPlans(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := Open(ctx, Options{DSN: dsn, MaxIdle: 2, MaxOpen: 5, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open statistics capacity database: %v", err)
	}
	defer func() { _ = database.Close() }()
	lockConnection := acquireMySQLIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", mysqlIntegrationLockName)
		_ = lockConnection.Close()
	}()
	if err := NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("run statistics capacity migrations: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		t.Fatalf("begin statistics capacity plan transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	statisticsStart := int64(1751990400)
	site := Site{
		Name: "A49 Plan Site", BaseURL: "https://a49-plan.example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", DataExportEnabled: true,
		StatisticsStartAt: &statisticsStart, CreatedAt: statisticsStart, UpdatedAt: statisticsStart,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create statistics capacity plan site: %v", err)
	}
	customer := Customer{
		Name: "A49 Plan Customer", Status: "using", StatisticsBackfillStatus: "none",
		CreatedAt: statisticsStart, UpdatedAt: statisticsStart,
	}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create statistics capacity plan customer: %v", err)
	}
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 100, RemoteCreatedAt: statisticsStart,
		Username: "a49-plan", RemoteState: AccountRemoteStateNormal, ManagedStatus: AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: statisticsStart, UpdatedAt: statisticsStart,
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create statistics capacity plan account: %v", err)
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	globalRequest := StatisticsReadRequest{
		Scope: "global", Granularity: "hour", SiteIDs: []int64{site.ID},
		StartTimestamp: time.Date(2032, 7, 1, 23, 0, 0, 0, location).Unix(),
		EndTimestamp:   time.Date(2032, 7, 3, 1, 0, 0, 0, location).Unix(),
	}
	globalQuery, globalArgs, err := statisticsActiveQuery(globalRequest)
	if err != nil {
		t.Fatalf("build global hybrid active query: %v", err)
	}
	statement := tx.Session(&gorm.Session{DryRun: true}).Raw(globalQuery, globalArgs...).Statement
	if statement.Error != nil {
		t.Fatalf("bind global hybrid active query: %v", statement.Error)
	}
	var globalPlan string
	if err := tx.WithContext(ctx).Raw("EXPLAIN FORMAT=JSON "+statement.SQL.String(), statement.Vars...).Row().Scan(&globalPlan); err != nil {
		t.Fatalf("explain global hybrid active query: %v", err)
	}
	for _, index := range []string{"idx_usage_fact_hourly_time_user", "idx_usage_fact_daily_date_user"} {
		if !strings.Contains(globalPlan, `"key": "`+index+`"`) {
			t.Fatalf("global hybrid active query plan does not use %s:\n%s", index, globalPlan)
		}
	}

	tests := []struct {
		name  string
		index string
		read  StatisticsReadRequest
	}{
		{
			name: "customer", index: "idx_usage_fact_hourly_time_user",
			read: StatisticsReadRequest{Scope: "customer", Granularity: "hour", StartTimestamp: 1752000000, EndTimestamp: 1752003600, CustomerIDs: []int64{customer.ID}},
		},
		{
			name: "account", index: "idx_usage_fact_hourly_time_user",
			read: StatisticsReadRequest{Scope: "account", Granularity: "hour", StartTimestamp: 1752000000, EndTimestamp: 1752003600, AccountIDs: []int64{account.ID}},
		},
		{
			name: "model", index: "idx_usage_fact_hourly_time_model_user",
			read: StatisticsReadRequest{Scope: "model", Granularity: "hour", StartTimestamp: 1752000000, EndTimestamp: 1752003600, ModelNames: []string{"gpt-5"}},
		},
		{
			name: "channel", index: "idx_usage_fact_hourly_time_channel_user",
			read: StatisticsReadRequest{Scope: "channel", Granularity: "hour", StartTimestamp: 1752000000, EndTimestamp: 1752003600, ChannelKeys: []StatisticsChannelKey{{SiteID: site.ID, ChannelID: 7}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query, args, err := statisticsActiveQuery(test.read)
			if err != nil {
				t.Fatalf("build active query: %v", err)
			}
			forced := strings.Replace(query, "usage_fact_hourly AS f",
				"usage_fact_hourly AS f FORCE INDEX ("+test.index+")", 1)
			if forced == query {
				t.Fatalf("active query does not contain the hourly fact source:\n%s", query)
			}
			statement := tx.Session(&gorm.Session{DryRun: true}).Raw(forced, args...).Statement
			if statement.Error != nil {
				t.Fatalf("bind active query: %v", statement.Error)
			}
			var plan string
			if err := tx.WithContext(ctx).Raw("EXPLAIN FORMAT=JSON "+statement.SQL.String(), statement.Vars...).Row().Scan(&plan); err != nil {
				t.Fatalf("explain active query: %v", err)
			}
			if !strings.Contains(plan, `"key": "`+test.index+`"`) {
				t.Fatalf("active query plan does not use %s:\n%s", test.index, plan)
			}
			if !strings.Contains(plan, `"using_index": true`) {
				t.Fatalf("active query plan is not covering for %s:\n%s", test.index, plan)
			}
		})
	}
	boundaryHours := []int64{globalRequest.StartTimestamp, globalRequest.EndTimestamp - 3600}
	for _, hour := range boundaryHours {
		if err := tx.Create(&CollectionWindow{
			SiteID: site.ID, HourTS: hour, Status: CollectionWindowStatusComplete,
			FetchedRows: 2, FactRows: 2, SourceHash: strings.Repeat("a", 64), UpdatedAt: hour,
		}).Error; err != nil {
			t.Fatalf("create hybrid summary window: %v", err)
		}
	}
	for index, userID := range []int64{101, 102, 101, 104} {
		hour := boundaryHours[index/2]
		if err := tx.Exec(`INSERT INTO usage_fact_hourly
  (site_id,remote_user_id,username_snapshot,model_name,channel_id,hour_ts,request_count,quota,token_used,collected_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`, site.ID, userID, "hybrid", "model-hour", 1, hour, 1, 1, 1, hour).Error; err != nil {
			t.Fatalf("create hybrid hourly identity: %v", err)
		}
	}
	for index, userID := range []int64{101, 101, 103} {
		if err := tx.Exec(`INSERT INTO usage_fact_daily
  (site_id,remote_user_id,username_snapshot,model_name,channel_id,date_key,request_count,quota,token_used,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,1,?,?,?)`, site.ID, userID, "hybrid", "model-daily-"+strconv.Itoa(index), 1,
			20320702, 1, 1, 1, globalRequest.EndTimestamp, globalRequest.EndTimestamp, globalRequest.EndTimestamp).Error; err != nil {
			t.Fatalf("create hybrid daily identity: %v", err)
		}
	}
	rows, err := NewStatisticsRepository(tx).LoadActiveRows(ctx, globalRequest)
	if err != nil {
		t.Fatalf("load hybrid summary identities: %v", err)
	}
	if len(rows) != 1 || rows[0].RowKind != "summary" || rows[0].ActiveUsers != "4" {
		t.Fatalf("hybrid summary rows = %#v, want four exact identities", rows)
	}
}
