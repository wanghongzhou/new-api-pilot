package testsupport

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"new-api-pilot/model"
)

type platformSettingSeed struct {
	key    string
	value  string
	typeID string
	secret bool
}

var platformSettingSeeds = []platformSettingSeed{
	{key: "collector.probe_interval_seconds", value: "60", typeID: "int"},
	{key: "collector.realtime_interval_seconds", value: "60", typeID: "int"},
	{key: "collector.resource_interval_seconds", value: "60", typeID: "int"},
	{key: "collector.usage_delay_minutes", value: "5", typeID: "int"},
	{key: "collector.minute_retention_days", value: "90", typeID: "int"},
	{key: "logs.retention_days", value: "90", typeID: "int"},
	{key: "performance.retention_days", value: "90", typeID: "int"},
	{key: "task.retention_days", value: "90", typeID: "int"},
	{key: "system_task_terminal_retention_days", value: "90", typeID: "int"},
	{key: "collector.probe_concurrency", value: "20", typeID: "int"},
	{key: "collector.realtime_concurrency", value: "10", typeID: "int"},
	{key: "collector.resource_concurrency", value: "10", typeID: "int"},
	{key: "collector.metadata_concurrency", value: "5", typeID: "int"},
	{key: "collector.usage_concurrency", value: "5", typeID: "int"},
	{key: "collector.backfill_concurrency", value: "2", typeID: "int"},
	{key: "collector.manual_backfill_max_days", value: "366", typeID: "int"},
	{key: "fast_task.history_retention_seconds", value: "86400", typeID: "int"},
	{key: "fast_task.history_count", value: "100", typeID: "int"},
	{key: "upstream.allowed_host_suffixes", value: "", typeID: "string"},
	{key: "upstream.allowed_cidrs", value: "", typeID: "string"},
	{key: "upstream.connect_timeout_seconds", value: "5", typeID: "int"},
	{key: "upstream.response_header_timeout_seconds", value: "15", typeID: "int"},
	{key: "upstream.request_timeout_seconds", value: "30", typeID: "int"},
	{key: "upstream.export_timeout_seconds", value: "120", typeID: "int"},
	{key: "upstream.rate_limit_requests", value: "300", typeID: "int"},
	{key: "upstream.rate_limit_window_seconds", value: "180", typeID: "int"},
	{key: "upstream.max_inflight_per_origin", value: "4", typeID: "int"},
	{key: "export.file_ttl_hours", value: "24", typeID: "int"},
	{key: "export.max_active_per_user", value: "3", typeID: "int"},
	{key: "export.max_active_global", value: "10", typeID: "int"},
	{key: "export.max_file_bytes", value: "2147483648", typeID: "int"},
	{key: "export.min_free_disk_bytes", value: "5368709120", typeID: "int"},
	{key: "rate.fallback_quota_per_unit", value: "500000", typeID: "decimal"},
	{key: "rate.fallback_usd_exchange_rate", value: "6.8", typeID: "decimal"},
	{key: "notification.dingtalk.enabled", value: "false", typeID: "bool"},
	{key: "notification.dingtalk.webhook", value: "", typeID: "string", secret: true},
	{key: "notification.dingtalk.secret", value: "", typeID: "string", secret: true},
}

// ResetPlatformSettings provides the deterministic platform-setting baseline used by acceptance tests.
func ResetPlatformSettings(ctx context.Context, database *gorm.DB, updatedAt int64) error {
	if database == nil || updatedAt <= 0 {
		return errors.New("platform setting reset dependencies are invalid")
	}
	if err := database.WithContext(ctx).Exec("DELETE FROM platform_setting").Error; err != nil {
		return err
	}
	rows := make([]model.PlatformSetting, 0, len(platformSettingSeeds))
	for _, setting := range platformSettingSeeds {
		rows = append(rows, model.PlatformSetting{
			Key: setting.key, Value: setting.value, ValueType: setting.typeID,
			Secret: setting.secret, UpdatedAt: updatedAt,
		})
	}
	return database.WithContext(ctx).Create(&rows).Error
}
