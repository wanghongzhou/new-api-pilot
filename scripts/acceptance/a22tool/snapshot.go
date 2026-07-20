package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"new-api-pilot/common"
)

var serverUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func runSnapshot(arguments []string) error {
	reportPath := "a22-snapshot.json"
	role := ""
	_, err := parseNoPositionals("snapshot", arguments, func(flags *flag.FlagSet) {
		flags.StringVar(&reportPath, "report", reportPath, "snapshot report path")
		flags.StringVar(&role, "role", role, "source or target")
	})
	if err != nil {
		return err
	}
	if role != "source" && role != "target" {
		return errors.New("snapshot role must be source or target")
	}
	dsn := os.Getenv("DATABASE_DSN")
	keyText := os.Getenv("ENCRYPTION_KEY")
	siteToken := os.Getenv("A22_SITE_TOKEN")
	secretValue := os.Getenv("A22_SECRET_SETTING")
	if dsn == "" || keyText == "" || siteToken == "" || secretValue == "" {
		return errors.New("snapshot database and secret environment is incomplete")
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
	report, err := createSnapshot(ctx, database, cipher, role, siteToken, secretValue)
	if err != nil {
		return err
	}
	return writeJSON(reportPath, report)
}

func createSnapshot(
	ctx context.Context,
	database *sql.DB,
	cipher *common.Cipher,
	role, expectedSiteToken, expectedSecret string,
) (snapshotReport, error) {
	var serverUUID, actualDatabase, mysqlVersion string
	if err := database.QueryRowContext(ctx, "SELECT @@server_uuid, DATABASE(), VERSION()").
		Scan(&serverUUID, &actualDatabase, &mysqlVersion); err != nil {
		return snapshotReport{}, err
	}
	if !serverUUIDPattern.MatchString(serverUUID) || actualDatabase != databaseName {
		return snapshotReport{}, errors.New("A22 database identity is invalid")
	}
	digest := sha256.New()
	tableCounts, err := hashDatabase(ctx, database, digest)
	if err != nil {
		return snapshotReport{}, err
	}
	tasks, err := statusCounts(ctx, database, "collection_run", "status")
	if err != nil {
		return snapshotReport{}, err
	}
	runWindows, err := statusCounts(ctx, database, "collection_run_window", "status")
	if err != nil {
		return snapshotReport{}, err
	}
	collectionWindows, err := statusCounts(ctx, database, "collection_window", "status")
	if err != nil {
		return snapshotReport{}, err
	}
	activeKeys, err := loadActiveKeyCounts(ctx, database)
	if err != nil {
		return snapshotReport{}, err
	}
	aggregates, err := loadAggregates(ctx, database)
	if err != nil {
		return snapshotReport{}, err
	}
	var siteID int64
	var encryptedToken string
	if err := database.QueryRowContext(ctx, "SELECT id, access_token_encrypted FROM site WHERE name=?", siteName).
		Scan(&siteID, &encryptedToken); err != nil {
		return snapshotReport{}, err
	}
	plaintextToken, err := cipher.Decrypt(encryptedToken, fmt.Sprintf("site:%d:access_token", siteID))
	if err != nil || string(plaintextToken) != expectedSiteToken {
		return snapshotReport{}, errors.New("A22 site token cannot be verified")
	}
	var encryptedSetting string
	if err := database.QueryRowContext(ctx, `SELECT setting_value FROM platform_setting
WHERE setting_key='notification.dingtalk.webhook' AND is_secret=1`).Scan(&encryptedSetting); err != nil {
		return snapshotReport{}, err
	}
	plaintextSetting, err := cipher.Decrypt(encryptedSetting, "setting:notification.dingtalk.webhook")
	if err != nil || string(plaintextSetting) != expectedSecret {
		return snapshotReport{}, errors.New("A22 secret setting cannot be verified")
	}
	var lastBusinessText string
	if err := database.QueryRowContext(ctx, `SELECT setting_value FROM platform_setting
WHERE setting_key='a22.last_business_time_unix' AND is_secret=0`).Scan(&lastBusinessText); err != nil {
		return snapshotReport{}, err
	}
	lastBusiness, err := strconv.ParseInt(lastBusinessText, 10, 64)
	if err != nil || lastBusiness <= 0 {
		return snapshotReport{}, errors.New("A22 business time sentinel is invalid")
	}
	uuidHash := sha256.Sum256([]byte(serverUUID))
	return snapshotReport{
		SchemaVersion: 1, AcceptanceID: acceptanceID, Status: "passed", Role: role,
		Database: actualDatabase, ServerUUID: serverUUID, ServerUUIDFingerprint: hex.EncodeToString(uuidHash[:])[:12],
		MySQLVersion: mysqlVersion, SnapshotSHA256: hex.EncodeToString(digest.Sum(nil)), TableCounts: tableCounts,
		TaskStatuses: tasks, RunWindowStatuses: runWindows, CollectionWindowStates: collectionWindows,
		ActiveKeys: activeKeys, Aggregates: aggregates, SiteTokenDecrypted: true, SecretSettingDecrypted: true,
		LastBusinessTime: lastBusiness, ProductionReleaseOK: false,
	}, nil
}

func hashDatabase(ctx context.Context, database *sql.DB, digest hash.Hash) (map[string]int64, error) {
	rows, err := database.QueryContext(ctx, `SELECT table_name FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name ASC`)
	if err != nil {
		return nil, err
	}
	tables := make([]string, 0, 64)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, errors.New("A22 snapshot found no tables")
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		columns, primary, err := tableShape(ctx, database, table)
		if err != nil {
			return nil, err
		}
		if len(columns) == 0 || len(primary) == 0 {
			return nil, fmt.Errorf("A22 snapshot table %s lacks a stable shape", table)
		}
		quotedColumns := make([]string, len(columns))
		for index, column := range columns {
			quotedColumns[index] = quoteIdentifier(column)
		}
		quotedPrimary := make([]string, len(primary))
		for index, column := range primary {
			quotedPrimary[index] = quoteIdentifier(column)
		}
		query := "SELECT CAST(JSON_ARRAY(" + strings.Join(quotedColumns, ",") + ") AS CHAR) FROM " +
			quoteIdentifier(table) + " ORDER BY " + strings.Join(quotedPrimary, ",")
		dataRows, err := database.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("snapshot table %s: %w", table, err)
		}
		writeHashValue(digest, table)
		var count int64
		for dataRows.Next() {
			var payload string
			if err := dataRows.Scan(&payload); err != nil {
				_ = dataRows.Close()
				return nil, err
			}
			writeHashValue(digest, payload)
			count++
		}
		if err := dataRows.Close(); err != nil {
			return nil, err
		}
		counts[table] = count
	}
	return counts, nil
}

func tableShape(ctx context.Context, database *sql.DB, table string) ([]string, []string, error) {
	read := func(query string) ([]string, error) {
		rows, err := database.QueryContext(ctx, query, table)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		values := make([]string, 0, 32)
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, rows.Err()
	}
	columns, err := read(`SELECT column_name FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name=? ORDER BY ordinal_position`)
	if err != nil {
		return nil, nil, err
	}
	primary, err := read(`SELECT column_name FROM information_schema.key_column_usage
WHERE table_schema=DATABASE() AND table_name=? AND constraint_name='PRIMARY' ORDER BY ordinal_position`)
	return columns, primary, err
}

func writeHashValue(digest hash.Hash, value string) {
	_, _ = fmt.Fprintf(digest, "%d:", len(value))
	_, _ = io.WriteString(digest, value)
	_, _ = io.WriteString(digest, "\n")
}

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func statusCounts(ctx context.Context, database *sql.DB, table, column string) (map[string]int64, error) {
	query := "SELECT " + quoteIdentifier(column) + ", COUNT(*) FROM " + quoteIdentifier(table) +
		" GROUP BY " + quoteIdentifier(column) + " ORDER BY " + quoteIdentifier(column)
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

func loadActiveKeyCounts(ctx context.Context, database *sql.DB) (map[string]int64, error) {
	queries := map[string]string{
		"collection_pending":           "SELECT COUNT(*) FROM collection_run WHERE status='pending' AND active_key IS NOT NULL",
		"collection_running":           "SELECT COUNT(*) FROM collection_run WHERE status='running' AND active_key IS NOT NULL",
		"collection_terminal_with_key": "SELECT COUNT(*) FROM collection_run WHERE status NOT IN ('pending','running') AND active_key IS NOT NULL",
		"export_active":                "SELECT COUNT(*) FROM export_job WHERE active_key IS NOT NULL",
		"alert_active":                 "SELECT COUNT(*) FROM alert_event WHERE active_key IS NOT NULL",
		"maintenance_active":           "SELECT COUNT(*) FROM encryption_reencrypt_job WHERE active_key IS NOT NULL",
	}
	keys := make([]string, 0, len(queries))
	for key := range queries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]int64, len(keys))
	for _, key := range keys {
		var count int64
		if err := database.QueryRowContext(ctx, queries[key]).Scan(&count); err != nil {
			return nil, err
		}
		result[key] = count
	}
	return result, nil
}

func loadAggregates(ctx context.Context, database *sql.DB) (map[string]scopeAggregates, error) {
	tables := map[string][2]string{
		"account":  {"account_stat_hourly", "account_stat_daily"},
		"customer": {"customer_stat_hourly", "customer_stat_daily"},
		"site":     {"site_stat_hourly", "site_stat_daily"},
		"global":   {"global_stat_hourly", "global_stat_daily"},
		"model":    {"model_stat_hourly", "model_stat_daily"},
		"channel":  {"channel_stat_hourly", "channel_stat_daily"},
	}
	result := make(map[string]scopeAggregates, len(tables))
	for scope, pair := range tables {
		hourly, err := loadAggregateMetric(ctx, database, pair[0], false, scope != "account")
		if err != nil {
			return nil, err
		}
		daily, err := loadAggregateMetric(ctx, database, pair[1], true, scope != "account")
		if err != nil {
			return nil, err
		}
		result[scope] = scopeAggregates{Hourly: hourly, Daily: daily}
	}
	return result, nil
}

func loadAggregateMetric(ctx context.Context, database *sql.DB, table string, daily, active bool) (aggregateMetric, error) {
	activeExpression := "'0'"
	if active {
		activeExpression = "CAST(COALESCE(SUM(active_users),0) AS CHAR)"
	}
	finalExpression := "0"
	if daily {
		finalExpression = "COALESCE(MIN(is_final),0)"
	}
	query := `SELECT COUNT(*), CAST(COALESCE(SUM(request_count),0) AS CHAR),
CAST(COALESCE(SUM(quota),0) AS CHAR), CAST(COALESCE(SUM(token_used),0) AS CHAR), ` + activeExpression + `,
COALESCE(MIN(data_status),''), ` + finalExpression + ` FROM ` + quoteIdentifier(table)
	var metric aggregateMetric
	var final int
	if err := database.QueryRowContext(ctx, query).Scan(&metric.Rows, &metric.Requests, &metric.Quota,
		&metric.Tokens, &metric.ActiveUsers, &metric.DataStatus, &final); err != nil {
		return aggregateMetric{}, err
	}
	metric.IsFinal = final == 1
	return metric, nil
}
