package ops

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"new-api-pilot/common"
)

func TestMySQLVerifyRestoreFullAndDetectsCorruption(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	cipher := mustTestCipher(t, reencryptTestOldKey)
	manifestPath := writeDatabaseManifestFixture(t, ctx, database, cipher.KeyID())

	before := readVerifyWriteGuard(t, ctx, database)
	report, err := RunVerifyRestore(ctx, VerifyRestoreOptions{
		Database: database, Cipher: cipher, ManifestPath: manifestPath,
	})
	if err != nil || report.Status != "success" || report.Summary.Failed != 0 {
		_, aggregationErr := verifyAggregations(ctx, database)
		t.Fatalf("full verification report = %#v, error = %v, aggregation error = %v", report, err, aggregationErr)
	}
	after := readVerifyWriteGuard(t, ctx, database)
	if before != after {
		t.Fatalf("read-only verification changed database guard: before=%#v after=%#v", before, after)
	}

	t.Run("ciphertext", func(t *testing.T) {
		result, err := database.ExecContext(ctx, `INSERT INTO site
  (name, base_url, access_token_encrypted, created_at, updated_at)
VALUES ('verify cipher fixture', ?, 'invalid', ?, ?)`,
			"https://verify-cipher.invalid", time.Now().Unix(), time.Now().Unix(),
		)
		if err != nil {
			t.Fatalf("insert ciphertext fixture: %v", err)
		}
		id, _ := result.LastInsertId()
		t.Cleanup(func() { _, _ = database.Exec("DELETE FROM site WHERE id = ?", id) })
		assertVerifyFailureCode(t, ctx, database, cipher, manifestPath, "VERIFY_CIPHERTEXT_INVALID")
	})

	t.Run("active maintenance job", func(t *testing.T) {
		result, err := database.ExecContext(ctx, `INSERT INTO encryption_reencrypt_job
  (old_key_id, new_key_id, active_key, state, inventory_hash, total_items, staged_items, created_at, updated_at)
VALUES (?, ?, 'active', 'staging', ?, 0, 0, ?, ?)`,
			strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64),
			time.Now().Unix(), time.Now().Unix(),
		)
		if err != nil {
			t.Fatalf("insert active-job fixture: %v", err)
		}
		id, _ := result.LastInsertId()
		t.Cleanup(func() { _, _ = database.Exec("DELETE FROM encryption_reencrypt_job WHERE id = ?", id) })
		assertVerifyFailureCode(t, ctx, database, cipher, manifestPath, "VERIFY_ACTIVE_KEY_INVALID")
	})

	t.Run("aggregation", func(t *testing.T) {
		result, err := database.ExecContext(ctx, `INSERT INTO global_stat_hourly
  (hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (3600, 1, 2, 3, 1, 'complete', ?, ?, ?)`,
			time.Now().Unix(), time.Now().Unix(), time.Now().Unix(),
		)
		if err != nil {
			t.Fatalf("insert aggregation fixture: %v", err)
		}
		id, _ := result.LastInsertId()
		t.Cleanup(func() { _, _ = database.Exec("DELETE FROM global_stat_hourly WHERE id = ?", id) })
		assertVerifyFailureCode(t, ctx, database, cipher, manifestPath, "VERIFY_AGGREGATION_INVALID")
	})
}

func TestMySQLVerifyAggregationsDetectsSixScopeRowAndStateCorruption(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)

	t.Run("valid complete row set", func(t *testing.T) {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin complete aggregation fixture: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		insertCompleteAggregationVerifyFixture(t, ctx, tx)
		if _, err := verifyAggregations(ctx, tx); err != nil {
			t.Fatalf("verify complete aggregation fixture: %v", err)
		}
	})

	tests := []struct {
		name   string
		spec   string
		mutate func(*testing.T, context.Context, *sql.Tx, aggregationVerifyFixture)
	}{
		{
			name: "account metric corruption",
			spec: "account_hourly",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE account_stat_hourly SET request_count = request_count + 1 WHERE account_id = ? AND hour_ts = ?",
					fixture.accountID, fixture.hourTS)
			},
		},
		{
			name: "customer orphan zero row",
			spec: "customer_hourly",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO customer_stat_hourly
  (customer_id, site_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 0, 0, 0, 0, 'complete', ?, ?, ?)`, fixture.customerID, fixture.siteID,
					fixture.hourTS+3600, fixture.now, fixture.now, fixture.now)
			},
		},
		{
			name: "site orphan partial zero row",
			spec: "site_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO site_stat_daily
  (site_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 0, 0, 0, 0, 'partial', 0, ?, ?, ?)`, fixture.siteID, fixture.dateKey+1,
					fixture.now, fixture.now, fixture.now)
			},
		},
		{
			name: "global status corruption",
			spec: "global_hourly",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE global_stat_hourly SET data_status = 'partial' WHERE hour_ts = ?", fixture.hourTS)
			},
		},
		{
			name: "model status corruption",
			spec: "model_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE model_stat_daily SET data_status = 'partial' WHERE site_id = ? AND date_key = ?",
					fixture.siteID, fixture.dateKey)
			},
		},
		{
			name: "channel future final corruption",
			spec: "channel_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, fixture aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE channel_stat_daily SET is_final = 1 WHERE site_id = ? AND date_key = ?",
					fixture.siteID, fixture.dateKey)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin aggregation corruption fixture: %v", err)
			}
			defer func() { _ = tx.Rollback() }()
			fixture := insertCompleteAggregationVerifyFixture(t, ctx, tx)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, true)
			test.mutate(t, ctx, tx, fixture)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, false)
		})
	}
}

func TestMySQLVerifyAggregationsRequiresCoveragePartialZeroRows(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	t.Run("valid partial zero row set", func(t *testing.T) {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin valid partial-zero fixture: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		insertPartialZeroAggregationVerifyFixture(t, ctx, tx)
		if _, err := verifyAggregations(ctx, tx); err != nil {
			t.Fatalf("verify partial-zero aggregation fixture: %v", err)
		}
	})
	tests := []struct {
		name  string
		spec  string
		query string
		args  func(aggregationVerifyFixture) []any
	}{
		{
			name: "account", spec: "account_daily",
			query: "DELETE FROM account_stat_daily WHERE account_id = ? AND date_key = ?",
			args:  func(f aggregationVerifyFixture) []any { return []any{f.accountID, f.dateKey} },
		},
		{
			name: "customer", spec: "customer_daily",
			query: "DELETE FROM customer_stat_daily WHERE customer_id = ? AND site_id = ? AND date_key = ?",
			args:  func(f aggregationVerifyFixture) []any { return []any{f.customerID, f.siteID, f.dateKey} },
		},
		{
			name: "site", spec: "site_daily",
			query: "DELETE FROM site_stat_daily WHERE site_id = ? AND date_key = ?",
			args:  func(f aggregationVerifyFixture) []any { return []any{f.siteID, f.dateKey} },
		},
		{
			name: "global", spec: "global_daily",
			query: "DELETE FROM global_stat_daily WHERE date_key = ?",
			args:  func(f aggregationVerifyFixture) []any { return []any{f.dateKey} },
		},
		{
			name: "global hourly", spec: "global_hourly",
			query: "DELETE FROM global_stat_hourly WHERE hour_ts = ?",
			args:  func(f aggregationVerifyFixture) []any { return []any{f.hourTS} },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin partial-zero fixture: %v", err)
			}
			defer func() { _ = tx.Rollback() }()
			fixture := insertPartialZeroAggregationVerifyFixture(t, ctx, tx)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, true)
			execAggregationVerifyFixture(t, ctx, tx, test.query, test.args(fixture)...)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, false)
		})
	}
}

func TestMySQLVerifyAggregationsRecomputesSixScopePartialCoverage(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)

	t.Run("valid partial row set", func(t *testing.T) {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin partial aggregation fixture: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		insertPartialDataAggregationVerifyFixture(t, ctx, tx)
		if _, err := verifyAggregations(ctx, tx); err != nil {
			t.Fatalf("verify partial aggregation fixture: %v", err)
		}
	})

	tests := []struct {
		name   string
		spec   string
		mutate func(*testing.T, context.Context, *sql.Tx, aggregationVerifyFixture)
	}{
		{
			name: "account", spec: "account_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE account_stat_daily SET data_status = 'complete' WHERE account_id = ? AND date_key = ?",
					f.accountID, f.dateKey)
			},
		},
		{
			name: "customer", spec: "customer_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE customer_stat_daily SET data_status = 'complete' WHERE customer_id = ? AND site_id = ? AND date_key = ?",
					f.customerID, f.siteID, f.dateKey)
			},
		},
		{
			name: "site", spec: "site_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE site_stat_daily SET data_status = 'complete' WHERE site_id = ? AND date_key = ?",
					f.siteID, f.dateKey)
			},
		},
		{
			name: "global", spec: "global_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE global_stat_daily SET data_status = 'complete' WHERE date_key = ?", f.dateKey)
			},
		},
		{
			name: "model", spec: "model_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE model_stat_daily SET data_status = 'complete' WHERE site_id = ? AND date_key = ?",
					f.siteID, f.dateKey)
			},
		},
		{
			name: "channel", spec: "channel_daily",
			mutate: func(t *testing.T, ctx context.Context, tx *sql.Tx, f aggregationVerifyFixture) {
				execAggregationVerifyFixture(t, ctx, tx,
					"UPDATE channel_stat_daily SET data_status = 'complete' WHERE site_id = ? AND date_key = ?",
					f.siteID, f.dateKey)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin partial coverage corruption fixture: %v", err)
			}
			defer func() { _ = tx.Rollback() }()
			fixture := insertPartialDataAggregationVerifyFixture(t, ctx, tx)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, true)
			test.mutate(t, ctx, tx, fixture)
			assertAggregationSpecMatch(t, ctx, tx, test.spec, false)
		})
	}
}

func TestMySQLVerifyAggregationsAcceptsPastVerifiedFinalRows(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin final aggregation fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	fixture := insertCompleteAggregationVerifyFixture(t, ctx, tx)
	moveCompleteAggregationFixtureToPastFinal(t, ctx, tx, &fixture)
	if _, err := verifyAggregations(ctx, tx); err != nil {
		t.Fatalf("verify past final aggregation fixture: %v", err)
	}
}

type aggregationVerifyFixture struct {
	siteID     int64
	customerID int64
	accountID  int64
	hourTS     int64
	dateKey    int
	now        int64
}

func insertCompleteAggregationVerifyFixture(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
) aggregationVerifyFixture {
	t.Helper()
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	hourTS := time.Date(2099, 1, 2, 0, 0, 0, 0, location).Unix()
	fixture := insertAggregationVerifyEntities(t, ctx, tx, hourTS, hourTS+3600, 20990102)
	dateEnd := hourTS + 86400
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO collection_window
  (site_id, hour_ts, status, fetched_rows, source_hash, verified_at, updated_at)
VALUES (?, ?, 'complete', 1, ?, ?, ?)`, fixture.siteID, hourTS, strings.Repeat("a", 64), dateEnd, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO usage_fact_hourly
  (site_id, remote_user_id, username_snapshot, model_name, channel_id, hour_ts,
   request_count, quota, token_used, collected_at)
VALUES (?, 990001, 'verify-user', 'verify-model', 77, ?, 3, 5, 7, ?)`, fixture.siteID, hourTS, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO usage_fact_daily
  (site_id, remote_user_id, username_snapshot, model_name, channel_id, date_key,
   request_count, quota, token_used, is_final, last_calculated_at, created_at, updated_at)
VALUES (?, 990001, 'verify-user', 'verify-model', 77, ?, 3, 5, 7, 0, ?, ?, ?)`,
		fixture.siteID, fixture.dateKey, fixture.now, fixture.now, fixture.now)

	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO account_stat_hourly
  (account_id, hour_ts, request_count, quota, token_used, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 3, 5, 7, 'complete', ?, ?, ?)`, fixture.accountID, hourTS,
		fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO account_stat_daily
  (account_id, date_key, request_count, quota, token_used, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 3, 5, 7, 'complete', 0, ?, ?, ?)`, fixture.accountID, fixture.dateKey,
		fixture.now, fixture.now, fixture.now)

	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO customer_stat_hourly
  (customer_id, site_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 3, 5, 7, 1, 'complete', ?, ?, ?)`, fixture.customerID, fixture.siteID, hourTS,
		fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO customer_stat_daily
  (customer_id, site_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 3, 5, 7, 1, 'complete', 0, ?, ?, ?)`, fixture.customerID, fixture.siteID,
		fixture.dateKey, fixture.now, fixture.now, fixture.now)

	for _, table := range []string{"site", "model", "channel"} {
		var hourlyQuery, dailyQuery string
		var hourlyArgs, dailyArgs []any
		switch table {
		case "site":
			hourlyQuery = `INSERT INTO site_stat_hourly
  (site_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 3, 5, 7, 1, 'complete', ?, ?, ?)`
			dailyQuery = `INSERT INTO site_stat_daily
  (site_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 3, 5, 7, 1, 'complete', 0, ?, ?, ?)`
			hourlyArgs = []any{fixture.siteID, hourTS, fixture.now, fixture.now, fixture.now}
			dailyArgs = []any{fixture.siteID, fixture.dateKey, fixture.now, fixture.now, fixture.now}
		case "model":
			hourlyQuery = `INSERT INTO model_stat_hourly
  (site_id, model_name, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, 'verify-model', ?, 3, 5, 7, 1, 'complete', ?, ?, ?)`
			dailyQuery = `INSERT INTO model_stat_daily
  (site_id, model_name, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, 'verify-model', ?, 3, 5, 7, 1, 'complete', 0, ?, ?, ?)`
			hourlyArgs = []any{fixture.siteID, hourTS, fixture.now, fixture.now, fixture.now}
			dailyArgs = []any{fixture.siteID, fixture.dateKey, fixture.now, fixture.now, fixture.now}
		case "channel":
			hourlyQuery = `INSERT INTO channel_stat_hourly
  (site_id, channel_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, 77, ?, 3, 5, 7, 1, 'complete', ?, ?, ?)`
			dailyQuery = `INSERT INTO channel_stat_daily
  (site_id, channel_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, 77, ?, 3, 5, 7, 1, 'complete', 0, ?, ?, ?)`
			hourlyArgs = []any{fixture.siteID, hourTS, fixture.now, fixture.now, fixture.now}
			dailyArgs = []any{fixture.siteID, fixture.dateKey, fixture.now, fixture.now, fixture.now}
		}
		execAggregationVerifyFixture(t, ctx, tx, hourlyQuery, hourlyArgs...)
		execAggregationVerifyFixture(t, ctx, tx, dailyQuery, dailyArgs...)
	}
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO global_stat_hourly
  (hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, 3, 5, 7, 1, 'complete', ?, ?, ?)`, hourTS, fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO global_stat_daily
  (date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, 3, 5, 7, 1, 'complete', 0, ?, ?, ?)`, fixture.dateKey, fixture.now, fixture.now, fixture.now)
	return fixture
}

func insertPartialZeroAggregationVerifyFixture(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
) aggregationVerifyFixture {
	t.Helper()
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	hourTS := time.Date(2099, 2, 2, 0, 0, 0, 0, location).Unix()
	fixture := insertAggregationVerifyEntities(t, ctx, tx, hourTS, hourTS+2*3600, 20990202)
	result, err := tx.ExecContext(ctx, `INSERT INTO site
  (name, base_url, statistics_start_at, statistics_start_source, statistics_end_at, created_at, updated_at)
VALUES ('aggregation verify missing site', ?, ?, 'root_created', ?, ?, ?)`,
		"https://aggregation-verify-missing.invalid/"+strings.ReplaceAll(t.Name(), "/", "-"),
		hourTS, hourTS+3600, fixture.now, fixture.now)
	if err != nil {
		t.Fatalf("insert aggregation verify missing site: %v", err)
	}
	missingSiteID, _ := result.LastInsertId()
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO collection_window
  (site_id, hour_ts, status, fetched_rows, source_hash, verified_at, updated_at)
VALUES (?, ?, 'complete', 0, ?, NULL, ?), (?, ?, 'missing', 0, '', NULL, ?)`,
		fixture.siteID, hourTS, strings.Repeat("b", 64), fixture.now,
		fixture.siteID, hourTS+3600, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO collection_window
  (site_id, hour_ts, status, fetched_rows, source_hash, verified_at, updated_at)
VALUES (?, ?, 'missing', 0, '', NULL, ?)`, missingSiteID, hourTS, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO account_stat_daily
  (account_id, date_key, request_count, quota, token_used, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 0, 0, 0, 'partial', 0, ?, ?, ?)`, fixture.accountID, fixture.dateKey,
		fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO customer_stat_daily
  (customer_id, site_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 0, 0, 0, 0, 'partial', 0, ?, ?, ?)`, fixture.customerID, fixture.siteID,
		fixture.dateKey, fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO site_stat_daily
  (site_id, date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, 0, 0, 0, 0, 'partial', 0, ?, ?, ?)`, fixture.siteID, fixture.dateKey,
		fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO global_stat_daily
  (date_key, request_count, quota, token_used, active_users, data_status, is_final,
   last_calculated_at, created_at, updated_at)
VALUES (?, 0, 0, 0, 0, 'partial', 0, ?, ?, ?)`, fixture.dateKey, fixture.now, fixture.now, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO global_stat_hourly
  (hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, 0, 0, 0, 0, 'partial', ?, ?, ?)`, fixture.hourTS, fixture.now, fixture.now, fixture.now)
	return fixture
}

func insertPartialDataAggregationVerifyFixture(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
) aggregationVerifyFixture {
	t.Helper()
	fixture := insertCompleteAggregationVerifyFixture(t, ctx, tx)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE site SET statistics_end_at = ? WHERE id = ?", fixture.hourTS+2*3600, fixture.siteID)
	execAggregationVerifyFixture(t, ctx, tx, `INSERT INTO collection_window
  (site_id, hour_ts, status, fetched_rows, source_hash, verified_at, updated_at)
VALUES (?, ?, 'missing', 0, '', NULL, ?)`, fixture.siteID, fixture.hourTS+3600, fixture.now)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE account_stat_daily SET data_status = 'partial' WHERE account_id = ? AND date_key = ?",
		fixture.accountID, fixture.dateKey)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE customer_stat_daily SET data_status = 'partial' WHERE customer_id = ? AND site_id = ? AND date_key = ?",
		fixture.customerID, fixture.siteID, fixture.dateKey)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE site_stat_daily SET data_status = 'partial' WHERE site_id = ? AND date_key = ?",
		fixture.siteID, fixture.dateKey)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE global_stat_daily SET data_status = 'partial' WHERE date_key = ?", fixture.dateKey)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE model_stat_daily SET data_status = 'partial' WHERE site_id = ? AND date_key = ?",
		fixture.siteID, fixture.dateKey)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE channel_stat_daily SET data_status = 'partial' WHERE site_id = ? AND date_key = ?",
		fixture.siteID, fixture.dateKey)
	return fixture
}

func moveCompleteAggregationFixtureToPastFinal(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
	fixture *aggregationVerifyFixture,
) {
	t.Helper()
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	hourTS := time.Date(2001, 3, 4, 0, 0, 0, 0, location).Unix()
	dateKey := 20010304
	dateEnd := hourTS + 86400
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE site SET statistics_start_at = ?, statistics_end_at = ? WHERE id = ?",
		hourTS, hourTS+3600, fixture.siteID)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE account SET remote_created_at = ? WHERE id = ?", hourTS, fixture.accountID)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE collection_window SET hour_ts = ?, verified_at = ? WHERE site_id = ? AND hour_ts = ?",
		hourTS, dateEnd, fixture.siteID, fixture.hourTS)
	execAggregationVerifyFixture(t, ctx, tx,
		"UPDATE usage_fact_hourly SET hour_ts = ? WHERE site_id = ? AND hour_ts = ?",
		hourTS, fixture.siteID, fixture.hourTS)
	for _, table := range []string{
		"account_stat_hourly", "customer_stat_hourly", "site_stat_hourly",
		"model_stat_hourly", "channel_stat_hourly", "global_stat_hourly",
	} {
		execAggregationVerifyFixture(t, ctx, tx,
			"UPDATE "+table+" SET hour_ts = ? WHERE hour_ts = ?", hourTS, fixture.hourTS)
	}
	for _, table := range []string{
		"usage_fact_daily", "account_stat_daily", "customer_stat_daily", "site_stat_daily",
		"model_stat_daily", "channel_stat_daily", "global_stat_daily",
	} {
		execAggregationVerifyFixture(t, ctx, tx,
			"UPDATE "+table+" SET date_key = ?, is_final = 1 WHERE date_key = ?", dateKey, fixture.dateKey)
	}
	fixture.hourTS = hourTS
	fixture.dateKey = dateKey
}

func insertAggregationVerifyEntities(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
	statisticsStart, statisticsEnd int64,
	dateKey int,
) aggregationVerifyFixture {
	t.Helper()
	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx, `INSERT INTO site
  (name, base_url, statistics_start_at, statistics_start_source, statistics_end_at, created_at, updated_at)
VALUES ('aggregation verify site', ?, ?, 'root_created', ?, ?, ?)`,
		"https://aggregation-verify.invalid/"+strings.ReplaceAll(t.Name(), "/", "-"),
		statisticsStart, statisticsEnd, now, now)
	if err != nil {
		t.Fatalf("insert aggregation verify site: %v", err)
	}
	siteID, _ := result.LastInsertId()
	result, err = tx.ExecContext(ctx, `INSERT INTO customer (name, created_at, updated_at)
VALUES ('aggregation verify customer', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert aggregation verify customer: %v", err)
	}
	customerID, _ := result.LastInsertId()
	result, err = tx.ExecContext(ctx, `INSERT INTO account
  (site_id, customer_id, remote_user_id, remote_created_at, username, created_at, updated_at)
VALUES (?, ?, 990001, ?, 'verify-user', ?, ?)`, siteID, customerID, statisticsStart, now, now)
	if err != nil {
		t.Fatalf("insert aggregation verify account: %v", err)
	}
	accountID, _ := result.LastInsertId()
	return aggregationVerifyFixture{
		siteID: siteID, customerID: customerID, accountID: accountID,
		hourTS: statisticsStart, dateKey: dateKey, now: now,
	}
}

func execAggregationVerifyFixture(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
	query string,
	args ...any,
) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("mutate aggregation verify fixture: %v", err)
	}
}

func assertAggregationSpecMatch(
	t *testing.T,
	ctx context.Context,
	queryer queryContext,
	name string,
	wantMatch bool,
) {
	t.Helper()
	for _, spec := range aggregationSpecs {
		if spec.Name != name {
			continue
		}
		actual, err := streamMetricHash(ctx, queryer, spec.Actual)
		if err != nil {
			t.Fatalf("read %s aggregation fixture: %v", name, err)
		}
		expected, err := streamMetricHash(ctx, queryer, spec.Expected)
		if err != nil {
			t.Fatalf("recompute %s aggregation fixture: %v", name, err)
		}
		if (actual == expected) != wantMatch {
			t.Fatalf("%s aggregation equality = %t, want %t; actual=%#v expected=%#v",
				name, actual == expected, wantMatch, actual, expected)
		}
		return
	}
	t.Fatalf("aggregation spec %q not found", name)
}

func TestMySQLVerifyRestoreRejectsAuthoritativeSchemaCorruption(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	cipher := mustTestCipher(t, reencryptTestOldKey)
	manifestPath := writeDatabaseManifestFixture(t, ctx, database, cipher.KeyID())

	tests := []struct {
		name    string
		corrupt string
		restore string
	}{
		{
			name:    "missing foreign key",
			corrupt: "ALTER TABLE site_capability DROP FOREIGN KEY fk_site_capability_site",
			restore: `ALTER TABLE site_capability ADD CONSTRAINT fk_site_capability_site
FOREIGN KEY (site_id) REFERENCES site (id) ON UPDATE RESTRICT ON DELETE RESTRICT`,
		},
		{
			name:    "missing secondary index",
			corrupt: "ALTER TABLE platform_user DROP INDEX idx_platform_user_role_status",
			restore: "ALTER TABLE platform_user ADD KEY idx_platform_user_role_status (role, status)",
		},
		{
			name:    "changed column contract",
			corrupt: "ALTER TABLE platform_user MODIFY COLUMN display_name VARCHAR(129) NOT NULL DEFAULT '' AFTER password_hash",
			restore: "ALTER TABLE platform_user MODIFY COLUMN display_name VARCHAR(128) NOT NULL DEFAULT '' AFTER password_hash",
		},
		{
			name:    "changed engine",
			corrupt: "ALTER TABLE aggregation_bucket_lock ENGINE=MyISAM",
			restore: "ALTER TABLE aggregation_bucket_lock ENGINE=InnoDB",
		},
		{
			name: "unknown table",
			corrupt: `CREATE TABLE unexpected_restore_table (
id BIGINT NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			restore: "DROP TABLE unexpected_restore_table",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := database.ExecContext(ctx, test.corrupt); err != nil {
				t.Fatalf("inject schema corruption: %v", err)
			}
			restored := false
			defer func() {
				if restored {
					return
				}
				if _, err := database.ExecContext(context.Background(), test.restore); err != nil {
					t.Errorf("restore schema contract: %v", err)
				}
			}()
			report, err := RunVerifyRestore(ctx, VerifyRestoreOptions{
				Database: database, Cipher: cipher, ManifestPath: manifestPath,
			})
			if err == nil || report.Status != "failed" || report.Error == nil ||
				report.Error.Code != "VERIFY_SCHEMA_INVALID" {
				t.Fatalf("schema corruption report = %#v, error = %v", report, err)
			}
			if _, err := database.ExecContext(ctx, test.restore); err != nil {
				t.Fatalf("restore schema contract: %v", err)
			}
			restored = true
		})
	}
}

func assertVerifyFailureCode(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	cipher *common.Cipher,
	manifestPath string,
	want string,
) {
	t.Helper()
	report, err := RunVerifyRestore(ctx, VerifyRestoreOptions{
		Database: database, Cipher: cipher, ManifestPath: manifestPath,
	})
	if err == nil || report.Status != "failed" || report.Error == nil || report.Error.Code != want {
		t.Fatalf("verification failure report = %#v, error = %v, want %s", report, err, want)
	}
}

type verifyWriteGuard struct {
	Migrations int64
	Progress   int64
	Jobs       int64
	Items      int64
}

func readVerifyWriteGuard(t *testing.T, ctx context.Context, database *sql.DB) verifyWriteGuard {
	t.Helper()
	guard := verifyWriteGuard{}
	queries := []struct {
		query string
		value *int64
	}{
		{"SELECT COUNT(*) FROM schema_migration", &guard.Migrations},
		{"SELECT COUNT(*) FROM schema_migration_progress", &guard.Progress},
		{"SELECT COUNT(*) FROM encryption_reencrypt_job", &guard.Jobs},
		{"SELECT COUNT(*) FROM encryption_reencrypt_item", &guard.Items},
	}
	for _, item := range queries {
		if err := database.QueryRowContext(ctx, item.query).Scan(item.value); err != nil {
			t.Fatalf("read verification write guard: %v", err)
		}
	}
	return guard
}

func writeDatabaseManifestFixture(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	keyID string,
) string {
	t.Helper()
	directory := t.TempDir()
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	if _, err := zipper.Write([]byte("-- CHANGE REPLICATION SOURCE TO SOURCE_LOG_FILE='binlog.000001', SOURCE_LOG_POS=123;\nSELECT 1;\n")); err != nil {
		t.Fatalf("write dump fixture: %v", err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatalf("close dump fixture: %v", err)
	}
	dumpName := "database.sql.gz"
	dumpPath := filepath.Join(directory, dumpName)
	if err := os.WriteFile(dumpPath, compressed.Bytes(), 0o600); err != nil {
		t.Fatalf("store dump fixture: %v", err)
	}
	dumpHash := sha256.Sum256(compressed.Bytes())
	dumpHashText := hex.EncodeToString(dumpHash[:])
	writeSidecar(t, dumpPath+".sha256", dumpHashText, dumpName)

	rows, err := database.QueryContext(ctx, "SELECT version, checksum FROM schema_migration ORDER BY version ASC")
	if err != nil {
		t.Fatalf("read manifest migrations: %v", err)
	}
	migrations := make([]ManifestMigration, 0)
	for rows.Next() {
		var migration ManifestMigration
		if err := rows.Scan(&migration.Version, &migration.Checksum); err != nil {
			_ = rows.Close()
			t.Fatalf("scan manifest migration: %v", err)
		}
		migrations = append(migrations, migration)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close manifest migrations: %v", err)
	}
	manifest := BackupManifest{
		SchemaVersion:   1,
		BackupID:        "backup-20260714T010203Z-89abcdef",
		CreatedAtUTC:    "2026-07-14T01:02:03Z",
		Database:        "new_api_pilot_d2",
		DumpFile:        dumpName,
		DumpSHA256:      dumpHashText,
		DumpSizeBytes:   int64(compressed.Len()),
		ImageDigest:     "sha256:" + strings.Repeat("a", 64),
		EncryptionKeyID: keyID,
		MySQLVersion:    "8.4",
		ServerUUID:      "00000000-0000-0000-0000-000000000001",
		Source:          BackupSource{LogFile: "binlog.000001", LogPosition: 123},
		Migrations:      migrations,
		ExportFiles:     "excluded_regenerable",
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal database manifest: %v", err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, payload, 0o600); err != nil {
		t.Fatalf("store database manifest: %v", err)
	}
	manifestHash := sha256.Sum256(payload)
	writeSidecar(t, manifestPath+".sha256", hex.EncodeToString(manifestHash[:]), "manifest.json")
	return manifestPath
}
