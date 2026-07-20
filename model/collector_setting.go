package model

import (
	"context"
	"fmt"
	"strconv"

	"gorm.io/gorm"
)

// CollectorSettings is loaded at runtime so scheduler cadence and queue limits
// can change without restarting the process.
type CollectorSettings struct {
	ProbeIntervalSeconds    int
	RealtimeIntervalSeconds int
	ResourceIntervalSeconds int
	UsageDelayMinutes       int
	MinuteRetentionDays     int
	ProbeConcurrency        int
	RealtimeConcurrency     int
	ResourceConcurrency     int
	MetadataConcurrency     int
	UsageConcurrency        int
	BackfillConcurrency     int
	ManualBackfillMaxDays   int
}

type CollectorSettingRepository struct {
	db *gorm.DB
}

func NewCollectorSettingRepository(db *gorm.DB) *CollectorSettingRepository {
	return &CollectorSettingRepository{db: db}
}

func (repository *CollectorSettingRepository) Load(ctx context.Context) (CollectorSettings, error) {
	if repository == nil || repository.db == nil {
		return CollectorSettings{}, fmt.Errorf("collector setting repository is required")
	}
	keys := []string{
		"collector.probe_interval_seconds",
		"collector.realtime_interval_seconds",
		"collector.resource_interval_seconds",
		"collector.usage_delay_minutes",
		"collector.minute_retention_days",
		"collector.probe_concurrency",
		"collector.realtime_concurrency",
		"collector.resource_concurrency",
		"collector.metadata_concurrency",
		"collector.usage_concurrency",
		"collector.backfill_concurrency",
		"collector.manual_backfill_max_days",
	}
	type settingRow struct {
		Key    string `gorm:"column:setting_key"`
		Value  string `gorm:"column:setting_value"`
		Type   string `gorm:"column:value_type"`
		Secret bool   `gorm:"column:is_secret"`
	}
	var rows []settingRow
	if err := repository.db.WithContext(ctx).Table("platform_setting").
		Select("setting_key, setting_value, value_type, is_secret").
		Where("setting_key IN ?", keys).Find(&rows).Error; err != nil {
		return CollectorSettings{}, fmt.Errorf("load collector settings: %w", err)
	}
	values := make(map[string]int, len(rows))
	for _, row := range rows {
		value, err := strconv.Atoi(row.Value)
		if err != nil || row.Type != "int" || row.Secret {
			return CollectorSettings{}, fmt.Errorf("collector setting %s has an invalid integer contract", row.Key)
		}
		values[row.Key] = value
	}
	for _, key := range keys {
		if _, exists := values[key]; !exists {
			return CollectorSettings{}, fmt.Errorf("collector setting %s is missing", key)
		}
	}
	settings := CollectorSettings{
		ProbeIntervalSeconds:    values["collector.probe_interval_seconds"],
		RealtimeIntervalSeconds: values["collector.realtime_interval_seconds"],
		ResourceIntervalSeconds: values["collector.resource_interval_seconds"],
		UsageDelayMinutes:       values["collector.usage_delay_minutes"],
		MinuteRetentionDays:     values["collector.minute_retention_days"],
		ProbeConcurrency:        values["collector.probe_concurrency"],
		RealtimeConcurrency:     values["collector.realtime_concurrency"],
		ResourceConcurrency:     values["collector.resource_concurrency"],
		MetadataConcurrency:     values["collector.metadata_concurrency"],
		UsageConcurrency:        values["collector.usage_concurrency"],
		BackfillConcurrency:     values["collector.backfill_concurrency"],
		ManualBackfillMaxDays:   values["collector.manual_backfill_max_days"],
	}
	if err := validateCollectorSettings(settings); err != nil {
		return CollectorSettings{}, err
	}
	return settings, nil
}

func validateCollectorSettings(settings CollectorSettings) error {
	for name, value := range map[string]int{
		"probe_interval_seconds":    settings.ProbeIntervalSeconds,
		"realtime_interval_seconds": settings.RealtimeIntervalSeconds,
		"resource_interval_seconds": settings.ResourceIntervalSeconds,
	} {
		if value < 1 || value > 3600 {
			return fmt.Errorf("collector setting %s is outside the supported range", name)
		}
	}
	if settings.UsageDelayMinutes < 0 || settings.UsageDelayMinutes > 60 ||
		settings.MinuteRetentionDays < 1 || settings.MinuteRetentionDays > 3660 ||
		settings.ManualBackfillMaxDays < 1 || settings.ManualBackfillMaxDays > 3660 {
		return fmt.Errorf("collector timing setting is outside the supported range")
	}
	for name, value := range map[string]int{
		"probe_concurrency":    settings.ProbeConcurrency,
		"realtime_concurrency": settings.RealtimeConcurrency,
		"resource_concurrency": settings.ResourceConcurrency,
		"metadata_concurrency": settings.MetadataConcurrency,
		"usage_concurrency":    settings.UsageConcurrency,
		"backfill_concurrency": settings.BackfillConcurrency,
	} {
		if value < 1 || value > 1000 {
			return fmt.Errorf("collector setting %s is outside the supported range", name)
		}
	}
	return nil
}
