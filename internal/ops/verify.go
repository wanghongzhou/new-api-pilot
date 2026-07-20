package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

type VerifyRestoreOptions struct {
	Database     *sql.DB
	Cipher       *common.Cipher
	ManifestPath string
}

type secretVerificationError struct {
	RowType string
	RowID   int64
}

func (err secretVerificationError) Error() string { return "encrypted row cannot be decrypted" }

func RunVerifyRestore(ctx context.Context, options VerifyRestoreOptions) (VerifyReport, error) {
	keyID := ""
	if options.Cipher != nil {
		keyID = options.Cipher.KeyID()
	}
	report := newVerifyReport(keyID)
	if options.Database == nil || options.Cipher == nil || options.ManifestPath == "" {
		report.addCheck(failedCheck("configuration", "VERIFY_CONFIG_INVALID"))
		return report, errors.New("verify restore configuration is invalid")
	}
	validated, err := ValidateBackupManifest(options.ManifestPath, keyID)
	if err != nil {
		report.addCheck(failedCheck("backup_manifest", "VERIFY_MANIFEST_INVALID"))
		return report, fmt.Errorf("VERIFY_MANIFEST_INVALID: %w", err)
	}
	report.BackupID = validated.Manifest.BackupID
	report.addCheck(passedCheck("backup_manifest", map[string]any{
		"dump_size_bytes": validated.Manifest.DumpSizeBytes,
		"manifest_sha256": validated.ManifestHash,
	}))

	transaction, err := options.Database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		report.addCheck(failedCheck("read_only_transaction", "VERIFY_DATABASE_ERROR"))
		return report, fmt.Errorf("begin read-only verification: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	report.addCheck(passedCheck("read_only_transaction", nil))

	runVerifyCheck(&report, "migrations", "VERIFY_MIGRATION_INVALID", func() (map[string]any, error) {
		return verifyMigrations(ctx, transaction, validated.Manifest.Migrations)
	})
	runVerifyCheck(&report, "schema", "VERIFY_SCHEMA_INVALID", func() (map[string]any, error) {
		summary, err := model.VerifyAuthoritativeSchema(ctx, transaction)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"tables": summary.Tables, "columns": summary.Columns, "indexes": summary.Indexes,
			"foreign_keys": summary.ForeignKeys,
		}, nil
	})
	runVerifyCheck(&report, "seeds", "VERIFY_SEED_INVALID", func() (map[string]any, error) {
		return verifySeeds(ctx, transaction)
	})
	runVerifyCheck(&report, "foreign_keys", "VERIFY_FOREIGN_KEY_INVALID", func() (map[string]any, error) {
		return verifyForeignKeys(ctx, transaction)
	})
	secretDetails, secretErr := verifyEncryptedRows(ctx, transaction, options.Cipher)
	if secretErr == nil {
		report.addCheck(passedCheck("encrypted_rows", secretDetails))
	} else {
		check := failedCheck("encrypted_rows", "VERIFY_CIPHERTEXT_INVALID")
		var rowErr secretVerificationError
		if errors.As(secretErr, &rowErr) {
			check.Details = map[string]any{
				"row_type": rowErr.RowType,
				"row_id":   strconv.FormatInt(rowErr.RowID, 10),
			}
			report.Error = &OperationError{
				Code: "VERIFY_CIPHERTEXT_INVALID", RowType: rowErr.RowType,
				RowID: strconv.FormatInt(rowErr.RowID, 10),
			}
		}
		report.addCheck(check)
	}
	runVerifyCheck(&report, "critical_counts", "VERIFY_COUNT_INVALID", func() (map[string]any, error) {
		return verifyCriticalCounts(ctx, transaction)
	})
	runVerifyCheck(&report, "collection_windows", "VERIFY_WINDOW_INVALID", func() (map[string]any, error) {
		return verifyCollectionWindows(ctx, transaction)
	})
	runVerifyCheck(&report, "collection_cursors", "VERIFY_CURSOR_INVALID", func() (map[string]any, error) {
		return verifyCollectionCursors(ctx, transaction)
	})
	runVerifyCheck(&report, "active_keys", "VERIFY_ACTIVE_KEY_INVALID", func() (map[string]any, error) {
		return verifyActiveKeys(ctx, transaction)
	})
	runVerifyCheck(&report, "aggregations", "VERIFY_AGGREGATION_INVALID", func() (map[string]any, error) {
		return verifyAggregations(ctx, transaction)
	})

	if report.Summary.Failed == 0 {
		report.Status = "success"
		report.Error = nil
		return report, nil
	}
	return report, errors.New("restore verification failed")
}

func runVerifyCheck(
	report *VerifyReport,
	name, code string,
	check func() (map[string]any, error),
) {
	details, err := check()
	if err != nil {
		report.addCheck(failedCheck(name, code))
		return
	}
	report.addCheck(passedCheck(name, details))
}

func passedCheck(name string, details map[string]any) VerifyCheck {
	return VerifyCheck{Name: name, Status: "passed", Details: details}
}

func failedCheck(name, code string) VerifyCheck {
	return VerifyCheck{Name: name, Status: "failed", Code: code}
}

func verifyMigrations(
	ctx context.Context,
	queryer queryContext,
	manifest []ManifestMigration,
) (map[string]any, error) {
	repository, err := validateManifestMigrations(manifest)
	if err != nil {
		return nil, err
	}
	database, err := model.ReadMigrationVersions(ctx, queryer)
	if err != nil {
		return nil, err
	}
	if err := model.ValidateMigrationVersionPrefix(repository, database, true); err != nil {
		return nil, err
	}
	var progress int64
	if err := queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_progress").Scan(&progress); err != nil {
		return nil, err
	}
	if progress != 0 {
		return nil, errors.New("migration progress contains an unfinished checkpoint")
	}
	return map[string]any{"applied": len(repository), "progress_rows": progress}, nil
}

type expectedSetting struct {
	Key    string
	Type   string
	Secret bool
}

var expectedSettings = []expectedSetting{
	{Key: "collector.probe_interval_seconds", Type: "int"},
	{Key: "collector.realtime_interval_seconds", Type: "int"},
	{Key: "collector.resource_interval_seconds", Type: "int"},
	{Key: "collector.usage_delay_minutes", Type: "int"},
	{Key: "collector.minute_retention_days", Type: "int"},
	{Key: "collector.probe_concurrency", Type: "int"},
	{Key: "collector.realtime_concurrency", Type: "int"},
	{Key: "collector.resource_concurrency", Type: "int"},
	{Key: "collector.metadata_concurrency", Type: "int"},
	{Key: "collector.usage_concurrency", Type: "int"},
	{Key: "collector.backfill_concurrency", Type: "int"},
	{Key: "collector.manual_backfill_max_days", Type: "int"},
	{Key: "export.file_ttl_hours", Type: "int"},
	{Key: "export.max_active_per_user", Type: "int"},
	{Key: "export.max_active_global", Type: "int"},
	{Key: "export.max_file_bytes", Type: "int"},
	{Key: "export.min_free_disk_bytes", Type: "int"},
	{Key: "rate.fallback_quota_per_unit", Type: "decimal"},
	{Key: "rate.fallback_usd_exchange_rate", Type: "decimal"},
	{Key: "notification.dingtalk.enabled", Type: "bool"},
	{Key: "notification.dingtalk.webhook", Type: "string", Secret: true},
	{Key: "notification.dingtalk.secret", Type: "string", Secret: true},
}

var expectedAlertRules = [][2]string{
	{"site_offline", "critical"}, {"site_auth_expired", "critical"},
	{"site_export_disabled", "warning"},
	{"collection_missing", "critical"}, {"backfill_failed", "warning"},
	{"validation_failed", "critical"}, {"instance_stale", "warning"},
	{"instance_offline", "critical"}, {"cpu_high", "warning"}, {"cpu_high", "critical"},
	{"memory_high", "warning"}, {"memory_high", "critical"}, {"disk_high", "warning"},
	{"disk_high", "critical"}, {"site_no_instance", "critical"}, {"account_missing", "critical"},
	{"account_identity_mismatch", "critical"}, {"account_disabled", "warning"},
	{"account_quota_empty", "warning"},
}

func verifySeeds(ctx context.Context, queryer queryContext) (map[string]any, error) {
	for _, expected := range expectedSettings {
		var valueType string
		var secret bool
		if err := queryer.QueryRowContext(ctx, `SELECT value_type, is_secret FROM platform_setting
WHERE setting_key = ?`, expected.Key).Scan(&valueType, &secret); err != nil {
			return nil, err
		}
		if valueType != expected.Type || secret != expected.Secret {
			return nil, fmt.Errorf("setting seed %s metadata differs", expected.Key)
		}
	}
	for _, expected := range expectedAlertRules {
		var count int
		if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_rule
WHERE rule_key = ? AND level = ? AND scope_type = 'global' AND scope_id = 0`, expected[0], expected[1]).Scan(&count); err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, fmt.Errorf("alert seed %s/%s is missing or duplicated", expected[0], expected[1])
		}
	}
	return map[string]any{"settings": len(expectedSettings), "alert_rules": len(expectedAlertRules)}, nil
}

type foreignKey struct {
	Name        string
	ChildTable  string
	ParentTable string
	ChildCols   []string
	ParentCols  []string
}

func verifyForeignKeys(ctx context.Context, queryer queryContext) (map[string]any, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT constraint_name, table_name, column_name,
       referenced_table_name, referenced_column_name
FROM information_schema.key_column_usage
WHERE table_schema = DATABASE() AND referenced_table_name IS NOT NULL
ORDER BY constraint_name ASC, ordinal_position ASC`)
	if err != nil {
		return nil, err
	}
	keys := make([]foreignKey, 0)
	byName := make(map[string]int)
	for rows.Next() {
		var name, childTable, childColumn, parentTable, parentColumn string
		if err := rows.Scan(&name, &childTable, &childColumn, &parentTable, &parentColumn); err != nil {
			_ = rows.Close()
			return nil, err
		}
		index, exists := byName[name]
		if !exists {
			index = len(keys)
			byName[name] = index
			keys = append(keys, foreignKey{Name: name, ChildTable: childTable, ParentTable: parentTable})
		}
		keys[index].ChildCols = append(keys[index].ChildCols, childColumn)
		keys[index].ParentCols = append(keys[index].ParentCols, parentColumn)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, key := range keys {
		joins := make([]string, 0, len(key.ChildCols))
		nonNull := make([]string, 0, len(key.ChildCols))
		for index := range key.ChildCols {
			joins = append(joins, "c."+quoteIdentifier(key.ChildCols[index])+" = p."+quoteIdentifier(key.ParentCols[index]))
			nonNull = append(nonNull, "c."+quoteIdentifier(key.ChildCols[index])+" IS NOT NULL")
		}
		query := "SELECT COUNT(*) FROM " + quoteIdentifier(key.ChildTable) + " c LEFT JOIN " +
			quoteIdentifier(key.ParentTable) + " p ON " + strings.Join(joins, " AND ") + " WHERE " +
			strings.Join(nonNull, " AND ") + " AND p." + quoteIdentifier(key.ParentCols[0]) + " IS NULL"
		var orphans int64
		if err := queryer.QueryRowContext(ctx, query).Scan(&orphans); err != nil {
			return nil, err
		}
		if orphans != 0 {
			return nil, fmt.Errorf("foreign key %s has orphan rows", key.Name)
		}
	}
	return map[string]any{"constraints": len(keys), "orphans": 0}, nil
}

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func verifyEncryptedRows(
	ctx context.Context,
	queryer queryContext,
	cipher *common.Cipher,
) (map[string]any, error) {
	rows, err := loadSecretRows(ctx, queryer)
	if err != nil {
		return nil, err
	}
	counts := ReencryptCounts{}
	setInventoryCounts(&counts, rows)
	for _, row := range rows {
		if _, err := cipher.Decrypt(row.Ciphertext, row.AAD); err != nil {
			return nil, secretVerificationError{RowType: row.Type, RowID: row.ID}
		}
	}
	return map[string]any{
		"total": counts.Total, "site_tokens": counts.SiteTokens, "secret_settings": counts.Settings,
	}, nil
}

var criticalCountTables = []string{
	"platform_user", "site", "customer", "account", "collection_run", "collection_run_window",
	"collection_window", "usage_fact_hourly", "usage_fact_daily", "account_stat_hourly",
	"account_stat_daily", "customer_stat_hourly", "customer_stat_daily", "site_stat_hourly",
	"site_stat_daily", "global_stat_hourly", "global_stat_daily", "model_stat_hourly",
	"model_stat_daily", "channel_stat_hourly", "channel_stat_daily", "alert_event", "export_job",
}

func verifyCriticalCounts(ctx context.Context, queryer queryContext) (map[string]any, error) {
	counts := make(map[string]any, len(criticalCountTables))
	for _, table := range criticalCountTables {
		var count int64
		if err := queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table)).Scan(&count); err != nil {
			return nil, err
		}
		if count < 0 {
			return nil, errors.New("negative table count")
		}
		counts[table] = count
	}
	return counts, nil
}

func verifyCollectionWindows(ctx context.Context, queryer queryContext) (map[string]any, error) {
	queries := []string{
		`SELECT COUNT(*) FROM collection_window
WHERE hour_ts <= 0 OR MOD(hour_ts, 3600) <> 0
   OR status NOT IN ('pending','complete','missing','unavailable')
   OR fetched_rows < 0`,
		`SELECT COUNT(*) FROM collection_run_window w
JOIN collection_run r ON r.id = w.run_id
WHERE w.site_id <> r.site_id OR w.hour_ts <= 0 OR MOD(w.hour_ts, 3600) <> 0
   OR w.status NOT IN ('pending','running','success','failed','unavailable')
   OR w.attempt_count < 0 OR w.fetched_rows < 0 OR w.written_rows < 0
   OR (r.start_timestamp IS NOT NULL AND w.hour_ts < r.start_timestamp)
   OR (r.end_timestamp IS NOT NULL AND w.hour_ts >= r.end_timestamp)`,
		`SELECT COUNT(*) FROM collection_run r
LEFT JOIN (
  SELECT run_id, COUNT(*) total,
         SUM(status = 'success') completed,
         SUM(status = 'failed') failed
  FROM collection_run_window GROUP BY run_id
) w ON w.run_id = r.id
WHERE r.windows_initialized_at IS NOT NULL
  AND (r.total_windows <> COALESCE(w.total, 0)
    OR r.completed_windows <> COALESCE(w.completed, 0)
    OR r.failed_windows <> COALESCE(w.failed, 0))`,
	}
	for _, query := range queries {
		var violations int64
		if err := queryer.QueryRowContext(ctx, query).Scan(&violations); err != nil {
			return nil, err
		}
		if violations != 0 {
			return nil, errors.New("collection window invariant violation")
		}
	}
	return map[string]any{"violations": 0}, nil
}

func verifyCollectionCursors(ctx context.Context, queryer queryContext) (map[string]any, error) {
	query := `SELECT COUNT(*)
FROM collection_cursor c
JOIN site s ON s.id = c.site_id
WHERE c.cursor_key <> 'usage'
   OR (c.last_complete_hour IS NOT NULL AND (
        s.statistics_start_at IS NULL
        OR c.last_complete_hour < s.statistics_start_at
        OR MOD(c.last_complete_hour, 3600) <> 0
        OR NOT EXISTS (
          SELECT 1 FROM collection_window terminal
          WHERE terminal.site_id = c.site_id AND terminal.hour_ts = c.last_complete_hour
            AND terminal.status = 'complete'
        )
        OR (SELECT COUNT(*) FROM collection_window covered
            WHERE covered.site_id = c.site_id
              AND covered.hour_ts BETWEEN s.statistics_start_at AND c.last_complete_hour
              AND covered.status = 'complete') <> ((c.last_complete_hour - s.statistics_start_at) / 3600 + 1)
        OR EXISTS (
          SELECT 1 FROM collection_window next_window
          WHERE next_window.site_id = c.site_id AND next_window.hour_ts = c.last_complete_hour + 3600
            AND next_window.status = 'complete'
        )
   ))
   OR (c.last_complete_hour IS NULL AND s.statistics_start_at IS NOT NULL AND EXISTS (
        SELECT 1 FROM collection_window first_window
        WHERE first_window.site_id = c.site_id AND first_window.hour_ts = s.statistics_start_at
          AND first_window.status = 'complete'
   ))`
	var violations int64
	if err := queryer.QueryRowContext(ctx, query).Scan(&violations); err != nil {
		return nil, err
	}
	if violations != 0 {
		return nil, errors.New("collection cursor invariant violation")
	}
	return map[string]any{"violations": 0}, nil
}

func verifyActiveKeys(ctx context.Context, queryer queryContext) (map[string]any, error) {
	queries := []string{
		`SELECT COUNT(*) FROM collection_run
WHERE (status IN ('pending','running') AND active_key IS NULL)
   OR (status NOT IN ('pending','running') AND active_key IS NOT NULL)`,
		`SELECT COUNT(*) FROM export_job
WHERE (status IN ('pending','running') AND active_key IS NULL)
   OR (status NOT IN ('pending','running') AND active_key IS NOT NULL)
   OR (active_key IS NOT NULL AND active_key <> CONCAT(user_id, ':', format, ':', statistics_type, ':', filter_hash))`,
		`SELECT COUNT(*) FROM alert_event
WHERE (status IN ('pending','firing') AND active_key IS NULL)
   OR (status NOT IN ('pending','firing') AND active_key IS NOT NULL)`,
		`SELECT COUNT(*) FROM encryption_reencrypt_job WHERE active_key IS NOT NULL`,
		`SELECT COUNT(*) FROM encryption_reencrypt_item`,
	}
	for _, query := range queries {
		var violations int64
		if err := queryer.QueryRowContext(ctx, query).Scan(&violations); err != nil {
			return nil, err
		}
		if violations != 0 {
			return nil, errors.New("active key or maintenance staging invariant violation")
		}
	}
	return map[string]any{"violations": 0}, nil
}
