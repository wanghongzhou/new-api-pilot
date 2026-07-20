package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type a49SeedReport struct {
	SchemaVersion        int              `json:"schema_version"`
	AcceptanceID         string           `json:"acceptance_id"`
	Status               string           `json:"status"`
	Mode                 string           `json:"mode"`
	EvidenceClass        string           `json:"evidence_class"`
	AcceptanceEligible   bool             `json:"acceptance_eligible"`
	FixturePath          string           `json:"fixture_path"`
	FixtureSHA256        string           `json:"fixture_sha256"`
	FixedNowUnix         int64            `json:"fixed_now_unix"`
	Seed                 int64            `json:"seed"`
	StartedAt            string           `json:"started_at"`
	FinishedAt           string           `json:"finished_at"`
	DurationMilliseconds int64            `json:"duration_milliseconds"`
	ExpectedRows         map[string]int64 `json:"expected_rows"`
	ActualRows           map[string]int64 `json:"actual_rows"`
	MetricTotals         map[string]int64 `json:"metric_totals"`
	Checks               map[string]bool  `json:"checks"`
}

type a49SeedContext struct {
	profile    a49RunProfile
	connection *sql.Conn
	fixedNow   int64
	todayStart int64
	factStart  int64
	minuteNow  int64
}

func runA49Seed(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("a49-seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fixturePath := flags.String("fixture", "testdata/design/f05-ops-capacity.yaml", "F05 fixture path")
	mode := flags.String("mode", a49FullMode, "full or smoke")
	reportPath := flags.String("report", "a49-seed-report.json", "machine-readable seed report")
	timeout := flags.Duration("timeout", 4*time.Hour, "bounded seed timeout")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	profile, err := loadA49RunProfile(*fixturePath, *mode)
	if err != nil {
		fmt.Fprintf(stderr, "load A49 profile: %v\n", err)
		return 2
	}
	if err := validateA49SeedEnvironment(profile); err != nil {
		fmt.Fprintf(stderr, "A49 seed guard: %v\n", err)
		return 2
	}
	dsn := os.Getenv("A49_DATABASE_DSN")
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(stderr, "open isolated A49 database: %v\n", err)
		return 1
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	connection, err := database.Conn(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "reserve A49 seed connection: %v\n", err)
		return 1
	}
	defer connection.Close()
	started := time.Now().UTC()
	seed := &a49SeedContext{
		profile: profile, connection: connection, fixedNow: profile.Fixture.Clock.NowUnix,
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	localNow := time.Unix(seed.fixedNow, 0).In(location)
	seed.todayStart = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location).Unix()
	seed.factStart = seed.todayStart - int64(profile.Capacity.FactDays-1)*86400
	seed.minuteNow = seed.fixedNow - seed.fixedNow%60

	if err := seed.run(ctx, stdout); err != nil {
		fmt.Fprintf(stderr, "seed isolated A49 profile: %v\n", err)
		return 1
	}
	report, err := seed.verify(ctx, started)
	if err != nil {
		fmt.Fprintf(stderr, "verify isolated A49 profile: %v\n", err)
		return 1
	}
	if err := writeJSONAtomic(*reportPath, report); err != nil {
		fmt.Fprintf(stderr, "write A49 seed report: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "A49 %s profile seeded and verified: facts=%d report=%s\n",
		profile.Mode, report.ActualRows["usage_fact_hourly"], *reportPath)
	return 0
}

func validateA49SeedEnvironment(profile a49RunProfile) error {
	if os.Getenv("ACCEPTANCE_ID") != a49AcceptanceID || os.Getenv("A49_ISOLATED_MYSQL") != "true" {
		return errors.New("ACCEPTANCE_ID=A49 and A49_ISOLATED_MYSQL=true are required")
	}
	if os.Getenv("ACCEPTANCE_EVIDENCE_CLASS") != profile.evidenceClass() {
		return errors.New("A49 evidence class does not match the seed profile mode")
	}
	if strings.TrimSpace(os.Getenv("A49_DATABASE_DSN")) == "" || strings.TrimSpace(os.Getenv("A49_DATABASE_NAME")) == "" {
		return errors.New("isolated database DSN and name are required")
	}
	if len(os.Getenv("A49_VIEWER_PASSWORD")) < 16 {
		return errors.New("an ephemeral viewer password of at least 16 bytes is required")
	}
	if raw := os.Getenv("A49_FIXED_NOW_UNIX"); raw != fmt.Sprintf("%d", profile.Fixture.Clock.NowUnix) {
		return errors.New("A49_FIXED_NOW_UNIX must exactly match F05 clock.now_unix")
	}
	return nil
}

func (seed *a49SeedContext) run(ctx context.Context, stdout io.Writer) error {
	if err := seed.assertDatabase(ctx); err != nil {
		return err
	}
	for _, statement := range []string{
		"SET SESSION time_zone = '+08:00'",
		"SET SESSION transaction_isolation = 'READ-COMMITTED'",
		"SET SESSION innodb_lock_wait_timeout = 120",
	} {
		if _, err := seed.connection.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure seed session: %w", err)
		}
	}
	if err := seed.createSequences(ctx); err != nil {
		return err
	}
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{"viewers", seed.seedViewers},
		{"sites and resources", seed.seedSites},
		{"customers and accounts", seed.seedCustomersAndAccounts},
		{"channels", seed.seedChannels},
		{"collection windows", seed.seedWindows},
		{"hourly facts", seed.seedHourlyFacts},
		{"daily facts and summaries", seed.seedDailyReadModels},
	}
	for _, step := range steps {
		fmt.Fprintf(stdout, "A49 seed step: %s\n", step.name)
		if err := step.run(ctx); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func (seed *a49SeedContext) assertDatabase(ctx context.Context) error {
	var databaseName string
	if err := seed.connection.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&databaseName); err != nil {
		return fmt.Errorf("read selected database: %w", err)
	}
	if databaseName != os.Getenv("A49_DATABASE_NAME") || !strings.HasPrefix(databaseName, "pilot_a49") {
		return errors.New("selected database is not the isolated A49 database")
	}
	for _, table := range []string{"platform_user", "site", "customer", "account", "usage_fact_hourly"} {
		var count int64
		if err := seed.connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return fmt.Errorf("A49 schema is not migrated (%s): %w", table, err)
		}
		if count != 0 {
			return fmt.Errorf("A49 requires an empty isolated database; %s has %d rows", table, count)
		}
	}
	return nil
}

func (seed *a49SeedContext) createSequences(ctx context.Context) error {
	maximum := seed.profile.Capacity.RemoteUsersPerSite
	for _, candidate := range []int{
		seed.profile.Capacity.Customers, seed.profile.Capacity.Sites,
		seed.profile.Capacity.ManagedAccountsPerSite, seed.profile.Capacity.ActiveChannelsPerSite,
		seed.profile.Capacity.ActiveModelsPerSite,
		int((seed.profile.Capacity.CollectionWindowEndUnix - seed.profile.Capacity.CollectionWindowStartUnix) / 3600),
	} {
		if candidate > maximum {
			maximum = candidate
		}
	}
	if maximum > 100000 {
		return errors.New("A49 sequence bound exceeds 100000")
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`CREATE TEMPORARY TABLE a49_digit0 (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_digit1 (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_digit2 (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_digit3 (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_digit4 (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`INSERT INTO a49_digit0 (n) VALUES (0),(1),(2),(3),(4),(5),(6),(7),(8),(9)`, nil},
		{`INSERT INTO a49_digit1 (n) SELECT n FROM a49_digit0`, nil},
		{`INSERT INTO a49_digit2 (n) SELECT n FROM a49_digit0`, nil},
		{`INSERT INTO a49_digit3 (n) SELECT n FROM a49_digit0`, nil},
		{`INSERT INTO a49_digit4 (n) SELECT n FROM a49_digit0`, nil},
		{`CREATE TEMPORARY TABLE a49_seq (n INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`, nil},
		{`INSERT INTO a49_seq (n)
SELECT d0.n + 10*d1.n + 100*d2.n + 1000*d3.n + 10000*d4.n
FROM a49_digit0 d0 CROSS JOIN a49_digit1 d1 CROSS JOIN a49_digit2 d2
CROSS JOIN a49_digit3 d3 CROSS JOIN a49_digit4 d4
WHERE d0.n + 10*d1.n + 100*d2.n + 1000*d3.n + 10000*d4.n < ?`, []any{maximum}},
		{`CREATE TEMPORARY TABLE a49_site_seq LIKE a49_seq`, nil},
		{`CREATE TEMPORARY TABLE a49_user_seq LIKE a49_seq`, nil},
		{`CREATE TEMPORARY TABLE a49_aux_seq LIKE a49_seq`, nil},
		{`INSERT INTO a49_site_seq (n) SELECT n FROM a49_seq`, nil},
		{`INSERT INTO a49_user_seq (n) SELECT n FROM a49_seq`, nil},
		{`INSERT INTO a49_aux_seq (n) SELECT n FROM a49_seq`, nil},
		{`CREATE TEMPORARY TABLE a49_fact_hour (slot_index INT NOT NULL PRIMARY KEY, local_hour INT NOT NULL UNIQUE) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_day (
day_index INT NOT NULL PRIMARY KEY, day_start BIGINT NOT NULL UNIQUE, date_key INT NOT NULL UNIQUE, is_final TINYINT NOT NULL
) ENGINE=InnoDB`, nil},
		{`CREATE TEMPORARY TABLE a49_window (
hour_ts BIGINT NOT NULL PRIMARY KEY, date_key INT NOT NULL, local_hour INT NOT NULL,
has_fact TINYINT NOT NULL, status VARCHAR(16) NOT NULL, is_final TINYINT NOT NULL
) ENGINE=InnoDB`, nil},
	}
	for _, statement := range statements {
		if _, err := seed.connection.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("create A49 sequence tables: %w", err)
		}
	}
	if err := seed.populateCalendar(ctx); err != nil {
		return err
	}
	return nil
}

func (seed *a49SeedContext) populateCalendar(ctx context.Context) error {
	hours := append([]int(nil), seed.profile.Capacity.FactHoursLocal...)
	sort.Ints(hours)
	for index, hour := range hours {
		if _, err := seed.connection.ExecContext(ctx,
			"INSERT INTO a49_fact_hour (slot_index, local_hour) VALUES (?, ?)", index, hour); err != nil {
			return fmt.Errorf("seed fact hours: %w", err)
		}
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	for day := 0; day < seed.profile.Capacity.FactDays; day++ {
		start := seed.factStart + int64(day)*86400
		local := time.Unix(start, 0).In(location)
		dateKey := local.Year()*10000 + int(local.Month())*100 + local.Day()
		final := 1
		if start == seed.todayStart {
			final = 0
		}
		if _, err := seed.connection.ExecContext(ctx,
			"INSERT INTO a49_day (day_index, day_start, date_key, is_final) VALUES (?, ?, ?, ?)",
			day, start, dateKey, final); err != nil {
			return fmt.Errorf("seed fact days: %w", err)
		}
	}
	hourSet := make(map[int]struct{}, len(hours))
	for _, hour := range hours {
		hourSet[hour] = struct{}{}
	}
	capacity := seed.profile.Capacity
	for hourTS := capacity.CollectionWindowStartUnix; hourTS < capacity.CollectionWindowEndUnix; hourTS += 3600 {
		local := time.Unix(hourTS, 0).In(location)
		dateStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).Unix()
		dateKey := local.Year()*10000 + int(local.Month())*100 + local.Day()
		hasFact := 0
		if dateStart >= seed.factStart && dateStart <= seed.todayStart {
			if _, exists := hourSet[local.Hour()]; exists {
				hasFact = 1
			}
		}
		status, final := "complete", 1
		if hourTS >= seed.fixedNow-seed.fixedNow%3600 {
			status, final = "pending", 0
		} else if dateStart == seed.todayStart {
			final = 0
		}
		if _, err := seed.connection.ExecContext(ctx,
			"INSERT INTO a49_window (hour_ts, date_key, local_hour, has_fact, status, is_final) VALUES (?, ?, ?, ?, ?, ?)",
			hourTS, dateKey, local.Hour(), hasFact, status, final); err != nil {
			return fmt.Errorf("seed collection calendar: %w", err)
		}
	}
	return nil
}

func (seed *a49SeedContext) seedViewers(ctx context.Context) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(os.Getenv("A49_VIEWER_PASSWORD")), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash ephemeral A49 viewer password: %w", err)
	}
	tx, err := seed.connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO platform_user
(username, password_hash, display_name, role, status, must_change_password, session_version, created_at, updated_at)
VALUES (?, ?, ?, 'viewer', 1, 0, 1, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for index := 1; index <= seed.profile.Capacity.ConcurrentReadUsers; index++ {
		username := fmt.Sprintf("%s%02d", seed.profile.Capacity.ViewerUsernamePrefix, index)
		if _, err := statement.ExecContext(ctx, username, string(hash), fmt.Sprintf("A49 只读用户 %02d", index), seed.fixedNow, seed.fixedNow); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (seed *a49SeedContext) seedSites(ctx context.Context) error {
	capacity := seed.profile.Capacity
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO site
(id, name, base_url, config_version, remark, management_status, online_status, auth_status, statistics_status,
 health_status, root_user_id, root_created_at, version, system_name, quota_per_unit, usd_exchange_rate,
 last_rate_at, data_export_enabled, current_rpm, current_tpm, last_realtime_stat_at,
 last_probe_at, last_probe_success_at, statistics_start_at, statistics_start_source, monitoring_start_at, created_at, updated_at)
SELECT n+1, CONCAT('A49 站点 ', LPAD(n+1, 2, '0')), CONCAT('https://a49-site-', LPAD(n+1, 2, '0'), '.invalid'),
  1, 'A49 隔离容量画像', 'active', 'online', 'authorized', 'ready', 'ok', 1, ?, 'v-a49', 'new-api',
  500000, 7.2, ?, 1, 1000+n, 100000+n, ?, ?, ?, ?, 'manual', ?, ?, ?
FROM a49_seq WHERE n < ?`, []any{seed.fixedNow, seed.fixedNow, seed.fixedNow, seed.fixedNow, seed.fixedNow,
			capacity.CollectionWindowStartUnix, capacity.CollectionWindowStartUnix, seed.fixedNow, seed.fixedNow, capacity.Sites}},
		{`INSERT INTO collection_cursor (site_id, cursor_key, last_complete_hour, updated_at)
SELECT n+1, 'usage', ?, ? FROM a49_seq WHERE n < ?`, []any{capacity.HourlyQueryEndUnix - 3600, seed.fixedNow, capacity.Sites}},
		{`INSERT INTO site_instance
(site_id, node_name, hostname, is_master, runtime_version, goos, goarch, upstream_status, upstream_stale_after_seconds,
 current_status, first_seen_at, started_at, last_seen_at, last_synced_at, created_at, updated_at)
SELECT n+1, 'node-1', CONCAT('a49-node-', LPAD(n+1, 2, '0')), 1, 'go1.25', 'linux', 'amd64', 'online', 120,
 'online', ?, ?, ?, ?, ?, ? FROM a49_seq WHERE n < ?`, []any{capacity.CollectionWindowStartUnix, capacity.CollectionWindowStartUnix,
			seed.fixedNow, seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.Sites}},
		{`INSERT INTO site_status_minutely
(site_id, minute_ts, instance_count, online_instance_count, cpu_max_percent, cpu_avg_percent,
 memory_max_percent, memory_avg_percent, disk_max_used_percent, health_status, created_at)
SELECT n+1, ?, 1, 1, 35.0+(n MOD 5), 25.0+(n MOD 5), 50.0+(n MOD 5), 40.0+(n MOD 5),
 60.0+(n MOD 5), 'ok', ? FROM a49_seq WHERE n < ?`, []any{seed.minuteNow, seed.fixedNow, capacity.Sites}},
	}
	return seed.execTransaction(ctx, queries)
}

func (seed *a49SeedContext) seedCustomersAndAccounts(ctx context.Context) error {
	capacity := seed.profile.Capacity
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO customer (id, name, contact, remark, status, statistics_backfill_status, created_at, updated_at)
SELECT n+1, CONCAT('A49 客户 ', LPAD(n+1, 4, '0')), '', 'A49 隔离容量画像', 'cooperating', 'complete', ?, ?
FROM a49_seq WHERE n < ?`, []any{seed.fixedNow, seed.fixedNow, capacity.Customers}},
		{`INSERT INTO account
(id, site_id, customer_id, remote_user_id, remote_created_at, username, display_name, remote_group, remote_status,
 remote_state, last_remote_seen_at, quota, used_quota, request_count, managed_status, statistics_backfill_status,
 last_synced_at, remark, created_at, updated_at)
SELECT s.n*?+u.n+1, s.n+1, MOD(s.n*?+u.n, ?)+1, (s.n+1)*1000000+u.n+1, ?,
 CONCAT('remote-', LPAD(s.n+1, 2, '0'), '-', LPAD(u.n+1, 4, '0')),
 CONCAT('A49 远端用户 ', LPAD(s.n+1, 2, '0'), '-', LPAD(u.n+1, 4, '0')), 'default', 1,
 'normal', ?, 100000000, 50000000, 150, 'active', 'complete', ?, 'A49 managed', ?, ?
FROM a49_site_seq s CROSS JOIN a49_user_seq u WHERE s.n < ? AND u.n < ?`, []any{
			capacity.ManagedAccountsPerSite, capacity.ManagedAccountsPerSite, capacity.Customers,
			seed.factStart, seed.fixedNow, seed.fixedNow, seed.fixedNow, seed.fixedNow,
			capacity.Sites, capacity.ManagedAccountsPerSite,
		}},
	}
	return seed.execTransaction(ctx, queries)
}

func (seed *a49SeedContext) seedChannels(ctx context.Context) error {
	capacity := seed.profile.Capacity
	return seed.execTransaction(ctx, []struct {
		query string
		args  []any
	}{
		{`INSERT INTO site_channel
(site_id, remote_channel_id, name, last_synced_at, remote_missing, created_at, updated_at)
SELECT s.n+1, c.n+1, CONCAT('A49 通道 ', LPAD(c.n+1, 3, '0')), ?, 0, ?, ?
FROM a49_site_seq s CROSS JOIN a49_aux_seq c WHERE s.n < ? AND c.n < ?`, []any{
			seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.Sites, capacity.ActiveChannelsPerSite,
		}},
	})
}

func (seed *a49SeedContext) seedWindows(ctx context.Context) error {
	capacity := seed.profile.Capacity
	return seed.execTransaction(ctx, []struct {
		query string
		args  []any
	}{
		{`INSERT INTO collection_window
(site_id, hour_ts, status, fetched_rows, source_hash, verified_at, updated_at)
SELECT s.n+1, w.hour_ts, w.status, IF(w.has_fact=1, ?, 0),
 SHA2(CONCAT('A49:', s.n+1, ':', w.hour_ts, ':', ?), 256),
 IF(w.status='complete', ?, NULL), ?
FROM a49_seq s CROSS JOIN a49_window w WHERE s.n < ?`, []any{
			capacity.RemoteUsersPerSite, capacity.Seed, seed.fixedNow, seed.fixedNow, capacity.Sites,
		}},
	})
}

func (seed *a49SeedContext) seedHourlyFacts(ctx context.Context) error {
	capacity := seed.profile.Capacity
	for site := 1; site <= capacity.Sites; site++ {
		query := `INSERT INTO usage_fact_hourly
(site_id, remote_user_id, username_snapshot, model_name, channel_id, hour_ts,
 request_count, quota, token_used, collected_at)
SELECT ?, ?*1000000+u.n+1,
 CONCAT('remote-', LPAD(?, 2, '0'), '-', LPAD(u.n+1, 4, '0')),
 CONCAT('model-', LPAD(MOD(u.n+d.day_index, ?)+1, 3, '0')),
 MOD(u.n+d.day_index, ?)+1, d.day_start+h.local_hour*3600,
 1, 1000, 100, ?
FROM a49_seq u CROSS JOIN a49_day d CROSS JOIN a49_fact_hour h
WHERE u.n < ?`
		if err := seed.execTransaction(ctx, []struct {
			query string
			args  []any
		}{{query, []any{site, site, site, capacity.ActiveModelsPerSite, capacity.ActiveChannelsPerSite,
			seed.fixedNow, capacity.RemoteUsersPerSite}}}); err != nil {
			return fmt.Errorf("site %d facts: %w", site, err)
		}
	}
	return nil
}

func (seed *a49SeedContext) seedDailyReadModels(ctx context.Context) error {
	capacity := seed.profile.Capacity
	slots := len(capacity.FactHoursLocal)
	for site := 1; site <= capacity.Sites; site++ {
		queries := []struct {
			query string
			args  []any
		}{
			{`INSERT INTO usage_fact_daily
(site_id, remote_user_id, username_snapshot, model_name, channel_id, date_key,
 request_count, quota, token_used, is_final, last_calculated_at, created_at, updated_at)
SELECT ?, ?*1000000+u.n+1, CONCAT('remote-', LPAD(?, 2, '0'), '-', LPAD(u.n+1, 4, '0')),
 CONCAT('model-', LPAD(MOD(u.n+d.day_index, ?)+1, 3, '0')), MOD(u.n+d.day_index, ?)+1,
 d.date_key, ?, ?*1000, ?*100, d.is_final, ?, ?, ?
FROM a49_seq u CROSS JOIN a49_day d WHERE u.n < ?`, []any{site, site, site,
				capacity.ActiveModelsPerSite, capacity.ActiveChannelsPerSite, slots, slots, slots,
				seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.RemoteUsersPerSite}},
			{`INSERT INTO account_stat_daily
(account_id, date_key, request_count, quota, token_used, data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT (? - 1)*?+u.n+1, d.date_key, ?, ?*1000, ?*100, 'complete', d.is_final, ?, ?, ?
FROM a49_seq u CROSS JOIN a49_day d WHERE u.n < ?`, []any{site, capacity.ManagedAccountsPerSite,
				slots, slots, slots, seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.ManagedAccountsPerSite}},
			{`INSERT INTO customer_stat_daily
(customer_id, site_id, date_key, request_count, quota, token_used, active_users,
 data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT MOD((? - 1)*?+u.n, ?)+1, ?, d.date_key, ?, ?*1000, ?*100, 1,
 'complete', d.is_final, ?, ?, ?
FROM a49_seq u CROSS JOIN a49_day d WHERE u.n < ?`, []any{site, capacity.ManagedAccountsPerSite,
				capacity.Customers, site, slots, slots, slots, seed.fixedNow, seed.fixedNow, seed.fixedNow,
				capacity.ManagedAccountsPerSite}},
			{`INSERT INTO site_stat_daily
(site_id, date_key, request_count, quota, token_used, active_users,
 data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT ?, d.date_key, ?*?, ?*?*1000, ?*?*100, ?, 'complete', d.is_final, ?, ?, ?
FROM a49_day d`, []any{site, capacity.RemoteUsersPerSite, slots, capacity.RemoteUsersPerSite, slots,
				capacity.RemoteUsersPerSite, slots, capacity.RemoteUsersPerSite, seed.fixedNow, seed.fixedNow, seed.fixedNow}},
			{`INSERT INTO model_stat_daily
(site_id, model_name, date_key, request_count, quota, token_used, active_users,
 data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT ?, CONCAT('model-', LPAD(m.n+1, 3, '0')), d.date_key,
 (? DIV ?)*?, (? DIV ?)*?*1000, (? DIV ?)*?*100, (? DIV ?),
 'complete', d.is_final, ?, ?, ?
FROM a49_seq m CROSS JOIN a49_day d WHERE m.n < ?`, []any{site,
				capacity.RemoteUsersPerSite, capacity.ActiveModelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveModelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveModelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveModelsPerSite,
				seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.ActiveModelsPerSite}},
			{`INSERT INTO channel_stat_daily
(site_id, channel_id, date_key, request_count, quota, token_used, active_users,
 data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT ?, c.n+1, d.date_key,
 (? DIV ?)*?, (? DIV ?)*?*1000, (? DIV ?)*?*100, (? DIV ?),
 'complete', d.is_final, ?, ?, ?
FROM a49_seq c CROSS JOIN a49_day d WHERE c.n < ?`, []any{site,
				capacity.RemoteUsersPerSite, capacity.ActiveChannelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveChannelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveChannelsPerSite, slots,
				capacity.RemoteUsersPerSite, capacity.ActiveChannelsPerSite,
				seed.fixedNow, seed.fixedNow, seed.fixedNow, capacity.ActiveChannelsPerSite}},
			{`INSERT INTO site_stat_hourly
(site_id, hour_ts, request_count, quota, token_used, active_users,
 data_status, last_calculated_at, created_at, updated_at)
SELECT ?, w.hour_ts, IF(w.has_fact=1, ?, 0), IF(w.has_fact=1, ?*1000, 0),
 IF(w.has_fact=1, ?*100, 0), IF(w.has_fact=1, ?, 0), w.status, ?, ?, ?
FROM a49_window w`, []any{site, capacity.RemoteUsersPerSite, capacity.RemoteUsersPerSite,
				capacity.RemoteUsersPerSite, capacity.RemoteUsersPerSite, seed.fixedNow, seed.fixedNow, seed.fixedNow}},
		}
		if err := seed.execTransaction(ctx, queries); err != nil {
			return fmt.Errorf("site %d read models: %w", site, err)
		}
	}
	return seed.execTransaction(ctx, []struct {
		query string
		args  []any
	}{
		{`INSERT INTO global_stat_hourly
(hour_ts, request_count, quota, token_used, active_users, data_status, last_calculated_at, created_at, updated_at)
SELECT w.hour_ts, IF(w.has_fact=1, ?*?, 0), IF(w.has_fact=1, ?*?*1000, 0),
 IF(w.has_fact=1, ?*?*100, 0), IF(w.has_fact=1, ?*?, 0), w.status, ?, ?, ? FROM a49_window w`, []any{
			capacity.Sites, capacity.RemoteUsersPerSite, capacity.Sites, capacity.RemoteUsersPerSite,
			capacity.Sites, capacity.RemoteUsersPerSite, capacity.Sites, capacity.RemoteUsersPerSite,
			seed.fixedNow, seed.fixedNow, seed.fixedNow}},
		{`INSERT INTO global_stat_daily
(date_key, request_count, quota, token_used, active_users, data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT d.date_key, ?*?*?, ?*?*?*1000, ?*?*?*100, ?*?, 'complete', d.is_final, ?, ?, ?
FROM a49_day d`, []any{capacity.Sites, capacity.RemoteUsersPerSite, slots,
			capacity.Sites, capacity.RemoteUsersPerSite, slots, capacity.Sites, capacity.RemoteUsersPerSite, slots,
			capacity.Sites, capacity.RemoteUsersPerSite, seed.fixedNow, seed.fixedNow, seed.fixedNow}},
	})
}

func (seed *a49SeedContext) execTransaction(ctx context.Context, statements []struct {
	query string
	args  []any
}) error {
	tx, err := seed.connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (seed *a49SeedContext) verify(ctx context.Context, started time.Time) (a49SeedReport, error) {
	capacity := seed.profile.Capacity
	windows := (capacity.CollectionWindowEndUnix - capacity.CollectionWindowStartUnix) / 3600
	dailyFacts := int64(capacity.Sites * capacity.RemoteUsersPerSite * capacity.FactDays)
	expected := map[string]int64{
		"platform_user":       int64(capacity.ConcurrentReadUsers),
		"site":                int64(capacity.Sites),
		"customer":            int64(capacity.Customers),
		"account":             int64(capacity.ManagedAccounts),
		"site_channel":        int64(capacity.Sites * capacity.ActiveChannelsPerSite),
		"collection_window":   int64(capacity.Sites) * windows,
		"usage_fact_hourly":   capacity.UsageFactHourlyRows30D,
		"usage_fact_daily":    dailyFacts,
		"account_stat_daily":  int64(capacity.ManagedAccounts * capacity.FactDays),
		"customer_stat_daily": int64(capacity.ManagedAccounts * capacity.FactDays),
		"site_stat_hourly":    int64(capacity.Sites) * windows,
		"site_stat_daily":     int64(capacity.Sites * capacity.FactDays),
		"global_stat_hourly":  windows,
		"global_stat_daily":   int64(capacity.FactDays),
		"model_stat_daily":    int64(capacity.Sites * capacity.ActiveModelsPerSite * capacity.FactDays),
		"channel_stat_daily":  int64(capacity.Sites * capacity.ActiveChannelsPerSite * capacity.FactDays),
	}
	actual := make(map[string]int64, len(expected))
	checks := make(map[string]bool)
	for table, wanted := range expected {
		var count int64
		if err := seed.connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return a49SeedReport{}, fmt.Errorf("count %s: %w", table, err)
		}
		actual[table] = count
		checks["rows."+table] = count == wanted
	}
	var distinctRemote, minimumModels, maximumModels, minimumChannels, maximumChannels int64
	if err := seed.connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
SELECT site_id, remote_user_id FROM usage_fact_hourly GROUP BY site_id, remote_user_id
) remote_users`).Scan(&distinctRemote); err != nil {
		return a49SeedReport{}, fmt.Errorf("count distinct remote users: %w", err)
	}
	if err := seed.connection.QueryRowContext(ctx, `SELECT MIN(model_count), MAX(model_count) FROM (
SELECT site_id, COUNT(DISTINCT model_name) AS model_count FROM usage_fact_hourly GROUP BY site_id
) models`).Scan(&minimumModels, &maximumModels); err != nil {
		return a49SeedReport{}, fmt.Errorf("count models per site: %w", err)
	}
	if err := seed.connection.QueryRowContext(ctx, `SELECT MIN(channel_count), MAX(channel_count) FROM (
SELECT site_id, COUNT(*) AS channel_count FROM site_channel GROUP BY site_id
) channels`).Scan(&minimumChannels, &maximumChannels); err != nil {
		return a49SeedReport{}, fmt.Errorf("count channels per site: %w", err)
	}
	checks["distinct_remote_users"] = distinctRemote == int64(capacity.RemoteUsers)
	checks["models_per_site"] = minimumModels == int64(capacity.ActiveModelsPerSite) && maximumModels == minimumModels
	checks["channels_per_site"] = minimumChannels == int64(capacity.ActiveChannelsPerSite) && maximumChannels == minimumChannels

	metricTotals := map[string]int64{}
	metricQueries := map[string]string{
		"hourly_request_count":      "SELECT COALESCE(SUM(request_count),0) FROM usage_fact_hourly",
		"hourly_quota":              "SELECT COALESCE(SUM(quota),0) FROM usage_fact_hourly",
		"hourly_token_used":         "SELECT COALESCE(SUM(token_used),0) FROM usage_fact_hourly",
		"daily_fact_request_count":  "SELECT COALESCE(SUM(request_count),0) FROM usage_fact_daily",
		"site_daily_request_count":  "SELECT COALESCE(SUM(request_count),0) FROM site_stat_daily",
		"site_hourly_request_count": "SELECT COALESCE(SUM(request_count),0) FROM site_stat_hourly",
	}
	for name, query := range metricQueries {
		var value int64
		if err := seed.connection.QueryRowContext(ctx, query).Scan(&value); err != nil {
			return a49SeedReport{}, fmt.Errorf("verify metric %s: %w", name, err)
		}
		metricTotals[name] = value
	}
	expectedRequests := capacity.UsageFactHourlyRows30D
	checks["hourly_metrics"] = metricTotals["hourly_request_count"] == expectedRequests &&
		metricTotals["hourly_quota"] == expectedRequests*1000 && metricTotals["hourly_token_used"] == expectedRequests*100
	checks["daily_fact_matches_hourly"] = metricTotals["daily_fact_request_count"] == expectedRequests
	checks["site_daily_matches_hourly"] = metricTotals["site_daily_request_count"] == expectedRequests
	checks["site_hourly_matches_hourly"] = metricTotals["site_hourly_request_count"] == expectedRequests
	for name, passed := range checks {
		if !passed {
			return a49SeedReport{}, fmt.Errorf("A49 seed invariant %s failed", name)
		}
	}
	finished := time.Now().UTC()
	return a49SeedReport{
		SchemaVersion: evidenceSchemaVersion, AcceptanceID: a49AcceptanceID, Status: "passed",
		Mode: seed.profile.Mode, EvidenceClass: seed.profile.evidenceClass(), AcceptanceEligible: seed.profile.AcceptanceEligible,
		FixturePath: seed.profile.FixturePath, FixtureSHA256: seed.profile.FixtureSHA256,
		FixedNowUnix: seed.fixedNow, Seed: capacity.Seed,
		StartedAt: started.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano),
		DurationMilliseconds: finished.Sub(started).Milliseconds(), ExpectedRows: expected,
		ActualRows: actual, MetricTotals: metricTotals, Checks: checks,
	}, nil
}
