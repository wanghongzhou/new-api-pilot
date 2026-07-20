package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

const (
	siteName = "A22 恢复演练站点"
	modelKey = "a22-model"
	channel  = int64(2201)
)

func runSeed(arguments []string) error {
	fixturePath := defaultFixturePath
	reportPath := "a22-seed.json"
	_, err := parseNoPositionals("seed", arguments, func(flags *flag.FlagSet) {
		flags.StringVar(&fixturePath, "fixture", fixturePath, "F05 fixture path")
		flags.StringVar(&reportPath, "report", reportPath, "seed report path")
	})
	if err != nil {
		return err
	}
	profile, fixtureSHA, err := loadProfile(fixturePath)
	if err != nil {
		return fmt.Errorf("load F05 profile: %w", err)
	}
	dsn, keyText, adminPassword, siteToken, secretValue, err := seedEnvironment()
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return errors.New("ENCRYPTION_KEY must be Base64-encoded 32 bytes")
	}
	cipher, err := common.NewCipher(key)
	if err != nil {
		return err
	}
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return err
	}
	var actualDatabase string
	if err := database.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&actualDatabase); err != nil || actualDatabase != databaseName {
		return errors.New("DATABASE_DSN does not select the isolated A22 database")
	}
	seeder := model.NewSeeder(database)
	seeder.Now = func() time.Time { return time.Unix(profile.Clock.NowUnix, 0) }
	if err := seeder.Run(ctx); err != nil {
		return fmt.Errorf("seed defaults: %w", err)
	}
	passwordHash, err := common.HashPassword(adminPassword)
	if err != nil {
		return fmt.Errorf("hash A22 admin password: %w", err)
	}
	report, err := seedFixture(ctx, database, cipher, profile, fixtureSHA, passwordHash, siteToken, secretValue)
	if err != nil {
		return err
	}
	return writeJSON(reportPath, report)
}

func seedEnvironment() (dsn, key, password, token, secret string, err error) {
	dsn = os.Getenv("DATABASE_DSN")
	key = os.Getenv("ENCRYPTION_KEY")
	password = os.Getenv("A22_ADMIN_PASSWORD")
	token = os.Getenv("A22_SITE_TOKEN")
	secret = os.Getenv("A22_SECRET_SETTING")
	if dsn == "" || key == "" || password == "" || token == "" || secret == "" {
		err = errors.New("DATABASE_DSN, ENCRYPTION_KEY and A22 secret environment variables are required")
	}
	return
}

func seedFixture(
	ctx context.Context,
	database *sql.DB,
	cipher *common.Cipher,
	profile a22Profile,
	fixtureSHA, passwordHash, siteToken, secretValue string,
) (seedReport, error) {
	var currentUnix int64
	if err := database.QueryRowContext(ctx, "SELECT UNIX_TIMESTAMP()").Scan(&currentUnix); err != nil {
		return seedReport{}, err
	}
	lastBusiness := currentUnix - 60
	hour := fixtureHour(profile)
	dateKey := fixtureDateKey(profile)
	dateEnd := hour + 3600
	taskFuture := currentUnix + 86400

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return seedReport{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"platform_user", "site", "customer", "account", "collection_run", "usage_fact_hourly"} {
		var count int64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
			return seedReport{}, err
		}
		if count != 0 {
			return seedReport{}, fmt.Errorf("A22 fixture table %s is not empty", table)
		}
	}
	adminResult, err := tx.ExecContext(ctx, `INSERT INTO platform_user
  (username,password_hash,display_name,role,status,must_change_password,session_version,created_at,updated_at)
VALUES ('admin',?,'A22 管理员','admin',1,0,1,?,?)`, passwordHash, currentUnix, currentUnix)
	if err != nil {
		return seedReport{}, err
	}
	adminID, _ := adminResult.LastInsertId()
	siteResult, err := tx.ExecContext(ctx, `INSERT INTO site
  (name,base_url,config_version,remark,management_status,online_status,auth_status,statistics_status,
   health_status,root_user_id,root_created_at,version,system_name,quota_per_unit,usd_exchange_rate,
   last_rate_at,data_export_enabled,current_rpm,current_tpm,last_realtime_stat_at,probe_fail_count,
   last_probe_at,last_probe_success_at,statistics_start_at,statistics_start_source,monitoring_start_at,
   created_at,updated_at)
VALUES (?, 'http://a22-upstream.invalid', 1, 'A22 controlled technical drill', 'active', 'online',
  'authorized', 'ready', 'ok', 9001, ?, 'v-a22', 'A22 fixture', '1.0000000000',
  '7.0000000000', ?, 1, 2, 3, ?, 0, ?, ?, ?, 'manual', ?, ?, ?)`,
		siteName, currentUnix, currentUnix, currentUnix, currentUnix, currentUnix, hour, currentUnix, currentUnix, currentUnix)
	if err != nil {
		return seedReport{}, err
	}
	siteID, _ := siteResult.LastInsertId()
	encryptedToken, err := cipher.Encrypt([]byte(siteToken), fmt.Sprintf("site:%d:access_token", siteID))
	if err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE site SET access_token_encrypted=? WHERE id=?", encryptedToken, siteID); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO site_channel
  (site_id,remote_channel_id,name,last_synced_at,remote_missing,created_at,updated_at)
VALUES (?,?,'A22 渠道',?,0,?,?)`, siteID, channel, currentUnix, currentUnix, currentUnix); err != nil {
		return seedReport{}, err
	}
	for _, capabilityKey := range constant.SiteCapabilityKeys() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO site_capability
  (site_id,capability_key,status,message_code,message_params,checked_at)
VALUES (?,?,'passed',?,NULL,?)`, siteID, capabilityKey, string(constant.MessageCapabilityOK), currentUnix); err != nil {
			return seedReport{}, err
		}
	}
	customerResult, err := tx.ExecContext(ctx, `INSERT INTO customer
  (name,contact,remark,status,statistics_backfill_status,created_at,updated_at)
VALUES ('A22 客户','ops@example.invalid','A22 restore fixture','using','success',?,?)`, currentUnix, currentUnix)
	if err != nil {
		return seedReport{}, err
	}
	customerID, _ := customerResult.LastInsertId()
	accountResult, err := tx.ExecContext(ctx, `INSERT INTO account
  (site_id,customer_id,remote_user_id,remote_created_at,username,display_name,remote_group,remote_status,
   remote_state,remote_missing_count,last_remote_seen_at,quota,used_quota,request_count,managed_status,
   statistics_backfill_status,last_synced_at,remark,created_at,updated_at)
VALUES (?,?,22001,?,'a22-user','A22 User','default',1,'normal',0,?,100,5,3,'active','success',?,'',?,?)`,
		siteID, customerID, hour-3600, currentUnix, currentUnix, currentUnix, currentUnix)
	if err != nil {
		return seedReport{}, err
	}
	accountID, _ := accountResult.LastInsertId()

	runIDs := make(map[string]int64, 4)
	for _, run := range []struct {
		status, activeKey string
		initialized       bool
		completed, failed int
	}{
		{status: "pending", activeKey: "a22:collection:pending"},
		{status: "running", activeKey: "a22:collection:running", initialized: true},
		{status: "success", initialized: true, completed: 1},
		{status: "failed", initialized: true, failed: 1},
	} {
		var active any
		if run.activeKey != "" {
			active = run.activeKey
		}
		var initialized any
		if run.initialized {
			initialized = currentUnix
		}
		var started, finished any
		if run.status != "pending" {
			started = currentUnix
		}
		if run.status == "success" || run.status == "failed" {
			finished = currentUnix
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO collection_run
  (site_id,site_config_version,task_type,target_type,target_id,trigger_type,start_timestamp,end_timestamp,
   scope,active_key,status,fetched_rows,written_rows,retry_count,priority,next_attempt_at,heartbeat_at,
   windows_initialized_at,total_windows,completed_windows,failed_windows,created_request_id,last_request_id,
   error_code,error_params,error_message,started_at,finished_at,created_at,updated_at)
VALUES (?,1,'usage_hour','site',?,'schedule',?,?,JSON_OBJECT(),?,?,1,1,0,100,?,?,?, ?,?,?,
  CONCAT('a22-',?,'-create'),CONCAT('a22-',?,'-last'),'',NULL,NULL,?,?,?,?)`,
			siteID, siteID, hour, hour+3600, active, run.status, taskFuture, taskFuture, initialized,
			boolInt(run.initialized), run.completed, run.failed, run.status, run.status, started, finished, currentUnix, currentUnix)
		if insertErr != nil {
			return seedReport{}, insertErr
		}
		runIDs[run.status], _ = result.LastInsertId()
	}
	for _, window := range []struct {
		runStatus, status string
		attempt           int
	}{
		{runStatus: "running", status: "running", attempt: 1},
		{runStatus: "success", status: "success", attempt: 1},
		{runStatus: "failed", status: "failed", attempt: 2},
	} {
		var finished any
		if window.status != "running" {
			finished = currentUnix
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO collection_run_window
  (run_id,site_id,hour_ts,status,attempt_count,next_retry_at,fetched_rows,written_rows,error_code,
   error_params,error_message,started_at,finished_at,updated_at)
VALUES (?,?,?,?,?,?,1,1,'',NULL,NULL,?,?,?)`, runIDs[window.runStatus], siteID, hour, window.status,
			window.attempt, taskFuture, currentUnix, finished, currentUnix)
		if err != nil {
			return seedReport{}, err
		}
	}
	sourceHash := common.KeyFingerprint([]byte("a22-fixture-source-hash"))
	if _, err := tx.ExecContext(ctx, `INSERT INTO collection_window
  (site_id,hour_ts,status,fetched_rows,source_hash,last_fact_run_id,verified_at,last_error_code,updated_at)
VALUES (?,?,'complete',1,?,?,?,'',?)`, siteID, hour, sourceHash, runIDs["success"], dateEnd, currentUnix); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO collection_cursor
  (site_id,cursor_key,last_complete_hour,updated_at) VALUES (?,'usage',?,?)`, siteID, hour, currentUnix); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO usage_fact_hourly
  (site_id,remote_user_id,username_snapshot,model_name,channel_id,hour_ts,request_count,quota,token_used,collected_at)
VALUES (?,22001,'a22-user',?,?,?,3,5,7,?)`, siteID, modelKey, channel, hour, currentUnix); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO usage_fact_daily
  (site_id,remote_user_id,username_snapshot,model_name,channel_id,date_key,request_count,quota,token_used,
   is_final,last_calculated_at,created_at,updated_at)
VALUES (?,22001,'a22-user',?,?,?,3,5,7,1,?,?,?)`, siteID, modelKey, channel, dateKey, currentUnix, currentUnix, currentUnix); err != nil {
		return seedReport{}, err
	}
	if err := insertAggregates(ctx, tx, accountID, customerID, siteID, hour, dateKey, currentUnix); err != nil {
		return seedReport{}, err
	}
	var alertRuleID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM alert_rule
WHERE rule_key='site_offline' AND level='critical' AND scope_type='global' AND scope_id=0`).Scan(&alertRuleID); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_event
  (rule_id,rule_key,site_id,target_type,target_key,active_key,level,status,consecutive_count,current_value,
   threshold_value,message_code,message_params,message,first_observed_at,first_fired_at,last_fired_at,
   resolved_at,created_at,updated_at)
VALUES (?,'site_offline',?,'site',?,NULL,'critical','resolved',3,3,3,'A22_RESOLVED',JSON_OBJECT(),
  'A22 resolved fixture',?,?,?,?,?,?)`, alertRuleID, siteID, fmt.Sprintf("site:%d", siteID),
		currentUnix, currentUnix, currentUnix, currentUnix, currentUnix, currentUnix); err != nil {
		return seedReport{}, err
	}
	filterHash := common.KeyFingerprint([]byte("a22-export-filter"))
	if _, err := tx.ExecContext(ctx, `INSERT INTO export_job
  (user_id,format,statistics_type,filters,filter_hash,active_key,rate_snapshot,data_snapshot_at,status,
   progress,attempt_count,next_attempt_at,heartbeat_at,file_path,file_name,file_size,row_count,error_code,
   error_params,error_message,expires_at,started_at,finished_at,created_at,updated_at)
VALUES (?,'csv','global',JSON_OBJECT(),?,NULL,JSON_OBJECT(),?,'success',100,1,?,NULL,NULL,'a22.csv',128,1,
  '',NULL,NULL,?,?,?, ?,?)`, adminID, filterHash, currentUnix, taskFuture, taskFuture, currentUnix, currentUnix, currentUnix, currentUnix); err != nil {
		return seedReport{}, err
	}
	encryptedSetting, err := cipher.Encrypt([]byte(secretValue), "setting:notification.dingtalk.webhook")
	if err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE platform_setting SET setting_value=?,updated_at=?
WHERE setting_key='notification.dingtalk.webhook' AND is_secret=1`, encryptedSetting, currentUnix); err != nil {
		return seedReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_setting
  (setting_key,setting_value,value_type,is_secret,updated_at)
VALUES ('a22.last_business_time_unix',?,'int',0,?)`, fmt.Sprintf("%d", lastBusiness), currentUnix); err != nil {
		return seedReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return seedReport{}, err
	}

	return seedReport{
		SchemaVersion: 1, AcceptanceID: acceptanceID, Status: "passed", FixtureID: fixtureID,
		FixtureSHA256: fixtureSHA, Database: databaseName, FixedHourUnix: hour, DateKey: dateKey,
		LastBusinessTime: lastBusiness, SiteTokenEncrypted: true, SecretEncrypted: true,
		TaskStatuses:       map[string]int64{"pending": 1, "running": 1, "success": 1, "failed": 1},
		WindowStatuses:     map[string]int64{"running": 1, "success": 1, "failed": 1},
		CollectionStatuses: map[string]int64{"complete": 1},
		AggregateRows: map[string]int64{
			"account_hourly": 1, "account_daily": 1, "customer_hourly": 1, "customer_daily": 1,
			"site_hourly": 1, "site_daily": 1, "global_hourly": 1, "global_daily": 1,
			"model_hourly": 1, "model_daily": 1, "channel_hourly": 1, "channel_daily": 1,
		},
		ProductionReleaseOK: false,
	}, nil
}

func insertAggregates(ctx context.Context, tx *sql.Tx, accountID, customerID, siteID, hour int64, dateKey int, now int64) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO account_stat_hourly (account_id,hour_ts,request_count,quota,token_used,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,?,3,5,7,'complete',?,?,?)`, []any{accountID, hour, now, now, now}},
		{`INSERT INTO account_stat_daily (account_id,date_key,request_count,quota,token_used,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,3,5,7,'complete',1,?,?,?)`, []any{accountID, dateKey, now, now, now}},
		{`INSERT INTO customer_stat_hourly (customer_id,site_id,hour_ts,request_count,quota,token_used,active_users,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',?,?,?)`, []any{customerID, siteID, hour, now, now, now}},
		{`INSERT INTO customer_stat_daily (customer_id,site_id,date_key,request_count,quota,token_used,active_users,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',1,?,?,?)`, []any{customerID, siteID, dateKey, now, now, now}},
		{`INSERT INTO site_stat_hourly (site_id,hour_ts,request_count,quota,token_used,active_users,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,?,3,5,7,1,'complete',?,?,?)`, []any{siteID, hour, now, now, now}},
		{`INSERT INTO site_stat_daily (site_id,date_key,request_count,quota,token_used,active_users,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,3,5,7,1,'complete',1,?,?,?)`, []any{siteID, dateKey, now, now, now}},
		{`INSERT INTO global_stat_hourly (hour_ts,request_count,quota,token_used,active_users,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,3,5,7,1,'complete',?,?,?)`, []any{hour, now, now, now}},
		{`INSERT INTO global_stat_daily (date_key,request_count,quota,token_used,active_users,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,3,5,7,1,'complete',1,?,?,?)`, []any{dateKey, now, now, now}},
		{`INSERT INTO model_stat_hourly (site_id,model_name,hour_ts,request_count,quota,token_used,active_users,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',?,?,?)`, []any{siteID, modelKey, hour, now, now, now}},
		{`INSERT INTO model_stat_daily (site_id,model_name,date_key,request_count,quota,token_used,active_users,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',1,?,?,?)`, []any{siteID, modelKey, dateKey, now, now, now}},
		{`INSERT INTO channel_stat_hourly (site_id,channel_id,hour_ts,request_count,quota,token_used,active_users,data_status,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',?,?,?)`, []any{siteID, channel, hour, now, now, now}},
		{`INSERT INTO channel_stat_daily (site_id,channel_id,date_key,request_count,quota,token_used,active_users,data_status,is_final,last_calculated_at,created_at,updated_at)
VALUES (?,?,?,3,5,7,1,'complete',1,?,?,?)`, []any{siteID, channel, dateKey, now, now, now}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
