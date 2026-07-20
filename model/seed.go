package model

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Seeder struct {
	DB  *sql.DB
	Now func() time.Time
}

func NewSeeder(db *sql.DB) *Seeder {
	return &Seeder{DB: db, Now: time.Now}
}

func (seeder *Seeder) Run(ctx context.Context) error {
	if seeder.Now == nil {
		seeder.Now = time.Now
	}
	tx, err := seeder.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := seeder.Now().Unix()
	for _, setting := range defaultSettings {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO platform_setting
  (setting_key, setting_value, value_type, is_secret, updated_at)
VALUES (?, ?, ?, ?, ?)`, setting.Key, setting.Value, setting.Type, setting.Secret, now); err != nil {
			return fmt.Errorf("seed setting %s: %w", setting.Key, err)
		}
	}
	for _, rule := range defaultAlertRules {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO alert_rule
  (rule_key, name, enabled, level, metric, compare_operator,
   threshold_value, for_times, scope_type, scope_id, created_at, updated_at)
VALUES (?, ?, 1, ?, ?, ?, ?, ?, 'global', 0, ?, ?)`,
			rule.Key, rule.Name, rule.Level, rule.Metric, rule.Operator,
			rule.Threshold, rule.ForTimes, now, now,
		); err != nil {
			return fmt.Errorf("seed alert rule %s/%s: %w", rule.Key, rule.Level, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seeds: %w", err)
	}
	return nil
}

type settingSeed struct {
	Key    string
	Value  string
	Type   string
	Secret bool
}

var defaultSettings = []settingSeed{
	{Key: "collector.probe_interval_seconds", Value: "60", Type: "int"},
	{Key: "collector.realtime_interval_seconds", Value: "60", Type: "int"},
	{Key: "collector.resource_interval_seconds", Value: "60", Type: "int"},
	{Key: "collector.usage_delay_minutes", Value: "5", Type: "int"},
	{Key: "collector.minute_retention_days", Value: "90", Type: "int"},
	{Key: "logs.retention_days", Value: "30", Type: "int"},
	{Key: "performance.retention_days", Value: "90", Type: "int"},
	{Key: "task.retention_days", Value: "90", Type: "int"},
	{Key: "system_task_terminal_retention_days", Value: "90", Type: "int"},
	{Key: "collector.probe_concurrency", Value: "20", Type: "int"},
	{Key: "collector.realtime_concurrency", Value: "10", Type: "int"},
	{Key: "collector.resource_concurrency", Value: "10", Type: "int"},
	{Key: "collector.metadata_concurrency", Value: "5", Type: "int"},
	{Key: "collector.usage_concurrency", Value: "5", Type: "int"},
	{Key: "collector.backfill_concurrency", Value: "2", Type: "int"},
	{Key: "collector.manual_backfill_max_days", Value: "366", Type: "int"},
	{Key: "export.file_ttl_hours", Value: "24", Type: "int"},
	{Key: "export.max_active_per_user", Value: "3", Type: "int"},
	{Key: "export.max_active_global", Value: "10", Type: "int"},
	{Key: "export.max_file_bytes", Value: "2147483648", Type: "int"},
	{Key: "export.min_free_disk_bytes", Value: "5368709120", Type: "int"},
	{Key: "rate.fallback_quota_per_unit", Value: "", Type: "decimal"},
	{Key: "rate.fallback_usd_exchange_rate", Value: "", Type: "decimal"},
	{Key: "notification.dingtalk.enabled", Value: "false", Type: "bool"},
	{Key: "notification.dingtalk.webhook", Value: "", Type: "string", Secret: true},
	{Key: "notification.dingtalk.secret", Value: "", Type: "string", Secret: true},
}

type alertRuleSeed struct {
	Key       string
	Name      string
	Level     string
	Metric    string
	Operator  string
	Threshold string
	ForTimes  int
}

var defaultAlertRules = []alertRuleSeed{
	{Key: "site_offline", Name: "Site Offline", Level: "critical", Metric: "site.probe_fail_count", Operator: ">=", Threshold: "3", ForTimes: 1},
	{Key: "site_auth_expired", Name: "Site Authorization Expired", Level: "critical", Metric: "site.auth_expired", Operator: "==", Threshold: "1", ForTimes: 1},
	{Key: "site_export_disabled", Name: "Data Export Disabled", Level: "warning", Metric: "site.data_export_enabled", Operator: "==", Threshold: "0", ForTimes: 1},
	{Key: "collection_missing", Name: "Collection Window Missing", Level: "critical", Metric: "collection.missing", Operator: ">=", Threshold: "1", ForTimes: 1},
	{Key: "backfill_failed", Name: "Backfill Failed", Level: "warning", Metric: "collection.backfill_failed", Operator: ">=", Threshold: "1", ForTimes: 1},
	{Key: "validation_failed", Name: "Statistics Validation Failed", Level: "critical", Metric: "collection.validation_failed", Operator: ">=", Threshold: "1", ForTimes: 1},
	{Key: "instance_stale", Name: "Instance Stale", Level: "warning", Metric: "instance.stale_seconds", Operator: ">=", Threshold: "90", ForTimes: 1},
	{Key: "instance_offline", Name: "Instance Offline", Level: "critical", Metric: "instance.online", Operator: "==", Threshold: "0", ForTimes: 3},
	{Key: "cpu_high", Name: "CPU High", Level: "warning", Metric: "instance.cpu_percent", Operator: ">=", Threshold: "85", ForTimes: 3},
	{Key: "cpu_high", Name: "CPU Critical", Level: "critical", Metric: "instance.cpu_percent", Operator: ">=", Threshold: "95", ForTimes: 3},
	{Key: "memory_high", Name: "Memory High", Level: "warning", Metric: "instance.memory_percent", Operator: ">=", Threshold: "85", ForTimes: 3},
	{Key: "memory_high", Name: "Memory Critical", Level: "critical", Metric: "instance.memory_percent", Operator: ">=", Threshold: "95", ForTimes: 3},
	{Key: "disk_high", Name: "Disk High", Level: "warning", Metric: "instance.disk_percent", Operator: ">=", Threshold: "85", ForTimes: 3},
	{Key: "disk_high", Name: "Disk Critical", Level: "critical", Metric: "instance.disk_percent", Operator: ">=", Threshold: "95", ForTimes: 1},
	{Key: "site_no_instance", Name: "All Instances Unavailable", Level: "critical", Metric: "site.online_instances", Operator: "<=", Threshold: "0", ForTimes: 1},
	{Key: "account_missing", Name: "Managed Account Missing", Level: "critical", Metric: "account.remote_exists", Operator: "==", Threshold: "0", ForTimes: 1},
	{Key: "account_identity_mismatch", Name: "Managed Account Identity Mismatch", Level: "critical", Metric: "account.identity_match", Operator: "==", Threshold: "0", ForTimes: 1},
	{Key: "account_disabled", Name: "Managed Account Disabled", Level: "warning", Metric: "account.remote_enabled", Operator: "==", Threshold: "0", ForTimes: 1},
	{Key: "account_quota_empty", Name: "Managed Account Quota Empty", Level: "warning", Metric: "account.quota", Operator: "<=", Threshold: "0", ForTimes: 1},
	{Key: "channel_balance_low", Name: "Channel Balance Low", Level: "warning", Metric: "channel.balance_total", Operator: "<=", Threshold: "100", ForTimes: 1},
	{Key: "channel_balance_low", Name: "Channel Balance Empty", Level: "critical", Metric: "channel.balance_total", Operator: "<=", Threshold: "0", ForTimes: 1},
	{Key: "channel_response_time_high", Name: "Channel Response Time High", Level: "warning", Metric: "channel.response_time_avg_ms", Operator: ">=", Threshold: "1000", ForTimes: 3},
	{Key: "channel_response_time_high", Name: "Channel Response Time Critical", Level: "critical", Metric: "channel.response_time_avg_ms", Operator: ">=", Threshold: "3000", ForTimes: 1},
	{Key: "channel_availability_low", Name: "Channel Availability Low", Level: "warning", Metric: "channel.availability_rate", Operator: "<=", Threshold: "0.99", ForTimes: 3},
	{Key: "channel_availability_low", Name: "Channel Availability Critical", Level: "critical", Metric: "channel.availability_rate", Operator: "<=", Threshold: "0.90", ForTimes: 1},
}
