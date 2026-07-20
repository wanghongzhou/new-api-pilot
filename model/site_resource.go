package model

import (
	"context"

	"gorm.io/gorm"
)

type SiteResourceScope struct {
	SiteID            int64  `gorm:"column:site_id"`
	MonitoringStartAt *int64 `gorm:"column:monitoring_start_at"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
	NodeID            *int64 `gorm:"column:node_id"`
	NodeFirstSeenAt   *int64 `gorm:"column:node_first_seen_at"`
}

func (scope SiteResourceScope) NodeExists() bool {
	return scope.NodeID != nil && *scope.NodeID > 0 && scope.NodeFirstSeenAt != nil
}

type SiteResourceReadRequest struct {
	SiteID         int64
	NodeName       *string
	Granularity    string
	StartTimestamp int64
	EndTimestamp   int64
	StartDateKey   int
	EndDateKey     int
}

type SiteResourceRow struct {
	BucketStart         int64    `gorm:"column:bucket_start"`
	DateKey             int      `gorm:"column:date_key"`
	CPUMaxPercent       *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent       *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent    *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent    *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent  *float64 `gorm:"column:disk_max_used_percent"`
	DiskLastUsedPercent *float64 `gorm:"column:disk_last_used_percent"`
	InstanceCount       *int     `gorm:"column:instance_count"`
	OnlineInstanceCount *int     `gorm:"column:online_instance_count"`
	SampleCount         int      `gorm:"column:sample_count"`
	ExpectedSampleCount int      `gorm:"column:expected_sample_count"`
	HealthStatus        string   `gorm:"column:health_status"`
	DataStatus          string   `gorm:"column:data_status"`
	SourceAsOf          *int64   `gorm:"column:source_as_of"`
	IsFinal             bool     `gorm:"column:is_final"`
}

func (repository *SiteRepository) FindResourceScope(
	ctx context.Context,
	siteID int64,
	nodeName *string,
) (SiteResourceScope, error) {
	var scope SiteResourceScope
	var result *gorm.DB
	if nodeName == nil {
		result = repository.db.WithContext(ctx).Raw(`SELECT id AS site_id, monitoring_start_at, statistics_end_at,
       NULL AS node_id, NULL AS node_first_seen_at
FROM site
WHERE id = ?`, siteID).Scan(&scope)
	} else {
		result = repository.db.WithContext(ctx).Raw(`SELECT s.id AS site_id, s.monitoring_start_at, s.statistics_end_at,
       i.id AS node_id, i.first_seen_at AS node_first_seen_at
FROM site s
LEFT JOIN site_instance i ON i.site_id = s.id AND i.node_name = ?
WHERE s.id = ?`, *nodeName, siteID).Scan(&scope)
	}
	if result.Error != nil {
		return SiteResourceScope{}, result.Error
	}
	if result.RowsAffected != 1 {
		return SiteResourceScope{}, gorm.ErrRecordNotFound
	}
	return scope, nil
}

func (repository *SiteRepository) ListResourceRows(
	ctx context.Context,
	request SiteResourceReadRequest,
) ([]SiteResourceRow, error) {
	var rows []SiteResourceRow
	var query *gorm.DB
	switch {
	case request.Granularity == "minute" && request.NodeName == nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT minute_ts AS bucket_start, 0 AS date_key,
       cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
       disk_max_used_percent, NULL AS disk_last_used_percent,
       instance_count, online_instance_count, 1 AS sample_count, 1 AS expected_sample_count,
       health_status, 'complete' AS data_status, minute_ts AS source_as_of, 0 AS is_final
FROM site_status_minutely
WHERE site_id = ? AND minute_ts >= ? AND minute_ts < ?
ORDER BY minute_ts ASC`, request.SiteID, request.StartTimestamp, request.EndTimestamp)
	case request.Granularity == "minute" && request.NodeName != nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT minute_ts AS bucket_start, 0 AS date_key,
       cpu_percent AS cpu_max_percent, cpu_percent AS cpu_avg_percent,
       memory_percent AS memory_max_percent, memory_percent AS memory_avg_percent,
       disk_used_percent AS disk_max_used_percent, disk_used_percent AS disk_last_used_percent,
       NULL AS instance_count, NULL AS online_instance_count, 1 AS sample_count, 1 AS expected_sample_count,
       CASE status WHEN 'online' THEN 'ok' WHEN 'stale' THEN 'warning'
                   WHEN 'offline' THEN 'critical' ELSE 'unavailable' END AS health_status,
       'complete' AS data_status, minute_ts AS source_as_of, 0 AS is_final
FROM site_instance_status_minutely
WHERE site_id = ? AND node_name = ? AND minute_ts >= ? AND minute_ts < ?
ORDER BY minute_ts ASC`, request.SiteID, *request.NodeName, request.StartTimestamp, request.EndTimestamp)
	case request.Granularity == "hour" && request.NodeName == nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT hour_ts AS bucket_start, 0 AS date_key,
       cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
       disk_max_used_percent, NULL AS disk_last_used_percent,
       instance_count_max AS instance_count, online_instance_count_min AS online_instance_count,
       sample_count, expected_sample_count, health_status, data_status,
       last_calculated_at AS source_as_of, 0 AS is_final
FROM site_status_hourly
WHERE site_id = ? AND hour_ts >= ? AND hour_ts < ?
ORDER BY hour_ts ASC`, request.SiteID, request.StartTimestamp, request.EndTimestamp)
	case request.Granularity == "hour" && request.NodeName != nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT hour_ts AS bucket_start, 0 AS date_key,
       cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
       disk_max_used_percent, disk_last_used_percent,
       NULL AS instance_count, NULL AS online_instance_count,
       sample_count, expected_sample_count,
       CASE WHEN sample_count = 0 THEN 'unavailable'
            WHEN abnormal_samples = 0 THEN 'ok'
            WHEN abnormal_samples < sample_count THEN 'warning'
            ELSE 'critical' END AS health_status,
       data_status, last_calculated_at AS source_as_of, 0 AS is_final
FROM site_instance_status_hourly
WHERE site_id = ? AND node_name = ? AND hour_ts >= ? AND hour_ts < ?
ORDER BY hour_ts ASC`, request.SiteID, *request.NodeName, request.StartTimestamp, request.EndTimestamp)
	case request.Granularity == "day" && request.NodeName == nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT 0 AS bucket_start, date_key,
       cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
       disk_max_used_percent, NULL AS disk_last_used_percent,
       instance_count_max AS instance_count, online_instance_count_min AS online_instance_count,
       sample_count, expected_sample_count, health_status, data_status,
       last_calculated_at AS source_as_of, is_final
FROM site_status_daily
WHERE site_id = ? AND date_key >= ? AND date_key < ?
ORDER BY date_key ASC`, request.SiteID, request.StartDateKey, request.EndDateKey)
	case request.Granularity == "day" && request.NodeName != nil:
		query = repository.db.WithContext(ctx).Raw(`SELECT 0 AS bucket_start, date_key,
       cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
       disk_max_used_percent, disk_last_used_percent,
       NULL AS instance_count, NULL AS online_instance_count,
       sample_count, expected_sample_count,
       CASE WHEN sample_count = 0 THEN 'unavailable'
            WHEN abnormal_samples = 0 THEN 'ok'
            WHEN abnormal_samples < sample_count THEN 'warning'
            ELSE 'critical' END AS health_status,
       data_status, last_calculated_at AS source_as_of, is_final
FROM site_instance_status_daily
WHERE site_id = ? AND node_name = ? AND date_key >= ? AND date_key < ?
ORDER BY date_key ASC`, request.SiteID, *request.NodeName, request.StartDateKey, request.EndDateKey)
	default:
		return nil, gorm.ErrInvalidValue
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (repository *SiteRepository) ListMonitoringPauses(
	ctx context.Context,
	siteID, startTimestamp, endTimestamp int64,
) ([]SiteMonitoringPause, error) {
	var pauses []SiteMonitoringPause
	err := repository.db.WithContext(ctx).
		Where("site_id = ? AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)",
			siteID, endTimestamp, startTimestamp).
		Order("start_minute_ts ASC").
		Find(&pauses).Error
	return pauses, err
}
