package model

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

type AlertEvaluationSnapshot struct {
	Sites             []AlertSiteEvaluationSnapshot
	Instances         []AlertInstanceEvaluationSnapshot
	Accounts          []AlertAccountEvaluationSnapshot
	Channels          []AlertChannelEvaluationSnapshot
	CollectionWindows []AlertCollectionEvaluationSnapshot
	Backfills         []AlertBackfillEvaluationSnapshot
	Validations       []AlertValidationEvaluationSnapshot
}

type AlertChannelEvaluationSnapshot struct {
	SiteID            int64   `gorm:"column:site_id"`
	SiteName          string  `gorm:"column:site_name"`
	SiteConfigVersion int     `gorm:"column:site_config_version"`
	ManagementStatus  string  `gorm:"column:management_status"`
	AuthStatus        string  `gorm:"column:auth_status"`
	StatisticsEndAt   *int64  `gorm:"column:statistics_end_at"`
	SiteUpdatedAt     int64   `gorm:"column:site_updated_at"`
	HourTS            *int64  `gorm:"column:hour_ts"`
	CollectedAt       *int64  `gorm:"column:collected_at"`
	ChannelCount      *int64  `gorm:"column:channel_count"`
	DataStatus        *string `gorm:"column:data_status"`
	ConfigVersion     *int    `gorm:"column:config_version"`
	BalanceTotal      *string `gorm:"column:balance_total"`
	ResponseTimeAvgMS *string `gorm:"column:response_time_avg_ms"`
	AvailabilityRate  *string `gorm:"column:availability_rate"`
}

type AlertSiteEvaluationSnapshot struct {
	ID                    int64  `gorm:"column:id"`
	Name                  string `gorm:"column:name"`
	ConfigVersion         int    `gorm:"column:config_version"`
	ManagementStatus      string `gorm:"column:management_status"`
	AuthStatus            string `gorm:"column:auth_status"`
	Version               string `gorm:"column:version"`
	DataExportEnabled     bool   `gorm:"column:data_export_enabled"`
	ProbeFailCount        int    `gorm:"column:probe_fail_count"`
	LastProbeAt           *int64 `gorm:"column:last_probe_at"`
	StatisticsStartAt     *int64 `gorm:"column:statistics_start_at"`
	StatisticsEndAt       *int64 `gorm:"column:statistics_end_at"`
	ResourceSampleAt      *int64 `gorm:"column:resource_sample_at"`
	ResourceSampleID      *int64 `gorm:"column:resource_sample_id"`
	ResourceInstanceCount *int   `gorm:"column:resource_instance_count"`
	UpdatedAt             int64  `gorm:"column:updated_at"`
}

type AlertInstanceEvaluationSnapshot struct {
	ID               int64   `gorm:"column:id"`
	SiteID           int64   `gorm:"column:site_id"`
	SiteName         string  `gorm:"column:site_name"`
	ManagementStatus string  `gorm:"column:management_status"`
	AuthStatus       string  `gorm:"column:auth_status"`
	StatisticsEndAt  *int64  `gorm:"column:statistics_end_at"`
	SiteUpdatedAt    int64   `gorm:"column:site_updated_at"`
	NodeName         string  `gorm:"column:node_name"`
	CurrentStatus    string  `gorm:"column:current_status"`
	LastSeenAt       *int64  `gorm:"column:last_seen_at"`
	LastSyncedAt     int64   `gorm:"column:last_synced_at"`
	SampledAt        *int64  `gorm:"column:sampled_at"`
	SampleID         *int64  `gorm:"column:sample_id"`
	SampleStatus     *string `gorm:"column:sample_status"`
	CPUPercent       *string `gorm:"column:cpu_percent"`
	MemoryPercent    *string `gorm:"column:memory_percent"`
	DiskUsedPercent  *string `gorm:"column:disk_used_percent"`
	UpdatedAt        int64   `gorm:"column:updated_at"`
	RetiredAt        *int64  `gorm:"column:retired_at"`
}

type AlertAccountEvaluationSnapshot struct {
	ID                int64  `gorm:"column:id"`
	SiteID            int64  `gorm:"column:site_id"`
	Username          string `gorm:"column:username"`
	DisplayName       string `gorm:"column:display_name"`
	ManagedStatus     string `gorm:"column:managed_status"`
	RemoteState       string `gorm:"column:remote_state"`
	RemoteStatus      int    `gorm:"column:remote_status"`
	Quota             int64  `gorm:"column:quota"`
	LastSyncedAt      *int64 `gorm:"column:last_synced_at"`
	CustomerStatus    string `gorm:"column:customer_status"`
	SiteManagement    string `gorm:"column:site_management_status"`
	SiteAuthStatus    string `gorm:"column:site_auth_status"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
	UpdatedAt         int64  `gorm:"column:updated_at"`
	SiteUpdatedAt     int64  `gorm:"column:site_updated_at"`
	CustomerUpdatedAt int64  `gorm:"column:customer_updated_at"`
}

type AlertCollectionEvaluationSnapshot struct {
	ID                int64  `gorm:"column:id"`
	SiteID            int64  `gorm:"column:site_id"`
	SiteName          string `gorm:"column:site_name"`
	ManagementStatus  string `gorm:"column:management_status"`
	AuthStatus        string `gorm:"column:auth_status"`
	DataExportEnabled bool   `gorm:"column:data_export_enabled"`
	StatisticsStartAt *int64 `gorm:"column:statistics_start_at"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
	HourTS            int64  `gorm:"column:hour_ts"`
	Status            string `gorm:"column:status"`
	LastErrorCode     string `gorm:"column:last_error_code"`
	UpdatedAt         int64  `gorm:"column:updated_at"`
	SiteUpdatedAt     int64  `gorm:"column:site_updated_at"`
}

type AlertBackfillEvaluationSnapshot struct {
	RunID             int64  `gorm:"column:run_id"`
	SiteID            int64  `gorm:"column:site_id"`
	SiteName          string `gorm:"column:site_name"`
	ManagementStatus  string `gorm:"column:management_status"`
	AuthStatus        string `gorm:"column:auth_status"`
	DataExportEnabled bool   `gorm:"column:data_export_enabled"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
	Status            string `gorm:"column:status"`
	ErrorCode         string `gorm:"column:error_code"`
	HasWindows        bool   `gorm:"column:has_windows"`
	FactsRepaired     bool   `gorm:"column:facts_repaired"`
	UpdatedAt         int64  `gorm:"column:updated_at"`
	SiteUpdatedAt     int64  `gorm:"column:site_updated_at"`
}

type AlertValidationEvaluationSnapshot struct {
	RunWindowID       int64   `gorm:"column:run_window_id"`
	SiteID            int64   `gorm:"column:site_id"`
	SiteName          string  `gorm:"column:site_name"`
	ManagementStatus  string  `gorm:"column:management_status"`
	AuthStatus        string  `gorm:"column:auth_status"`
	DataExportEnabled bool    `gorm:"column:data_export_enabled"`
	StatisticsEndAt   *int64  `gorm:"column:statistics_end_at"`
	HourTS            int64   `gorm:"column:hour_ts"`
	Status            string  `gorm:"column:status"`
	ErrorCode         string  `gorm:"column:error_code"`
	FactStatus        *string `gorm:"column:fact_status"`
	UpdatedAt         int64   `gorm:"column:updated_at"`
	SiteUpdatedAt     int64   `gorm:"column:site_updated_at"`
}

type AlertEvaluationRepository struct{ db *gorm.DB }

func NewAlertEvaluationRepository(db *gorm.DB) *AlertEvaluationRepository {
	return &AlertEvaluationRepository{db: db}
}

func (repository *AlertEvaluationRepository) LoadSnapshot(ctx context.Context) (AlertEvaluationSnapshot, error) {
	if repository == nil || repository.db == nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("alert evaluation repository is required")
	}
	var snapshot AlertEvaluationSnapshot
	loaders := []func(context.Context) error{
		func(ctx context.Context) error {
			var err error
			snapshot.Sites, err = repository.listSites(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.Instances, err = repository.listInstances(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.Accounts, err = repository.listAccounts(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.Channels, err = repository.listChannels(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.CollectionWindows, err = repository.listCollectionWindows(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.Backfills, err = repository.listBackfills(ctx)
			return err
		},
		func(ctx context.Context) error {
			var err error
			snapshot.Validations, err = repository.listValidations(ctx)
			return err
		},
	}
	for _, load := range loaders {
		if err := load(ctx); err != nil {
			return AlertEvaluationSnapshot{}, err
		}
	}
	return snapshot, nil
}

func (repository *AlertEvaluationRepository) listChannels(ctx context.Context) ([]AlertChannelEvaluationSnapshot, error) {
	var rows []AlertChannelEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT s.id AS site_id, s.name AS site_name, s.config_version AS site_config_version,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
h.hour_ts, h.collected_at, h.channel_count, h.data_status, h.config_version,
CAST(h.balance_total AS CHAR) AS balance_total,
CAST(h.response_time_avg_ms AS CHAR) AS response_time_avg_ms,
CAST(h.availability_rate AS CHAR) AS availability_rate
FROM site s
LEFT JOIN site_channel_inventory_hourly h
  ON h.site_id = s.id
 AND h.hour_ts = (SELECT MAX(latest.hour_ts) FROM site_channel_inventory_hourly latest WHERE latest.site_id = s.id)
ORDER BY s.id`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert channel snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listSites(ctx context.Context) ([]AlertSiteEvaluationSnapshot, error) {
	var rows []AlertSiteEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT s.id, s.name, s.config_version, s.management_status, s.auth_status,
s.version, s.data_export_enabled, s.probe_fail_count, s.last_probe_at,
s.statistics_start_at, s.statistics_end_at, s.updated_at,
latest.id AS resource_sample_id, latest.minute_ts AS resource_sample_at,
latest.instance_count AS resource_instance_count
FROM site s
LEFT JOIN site_status_minutely latest
  ON latest.site_id = s.id
 AND latest.minute_ts = (SELECT MAX(m.minute_ts) FROM site_status_minutely m WHERE m.site_id = s.id)
ORDER BY s.id`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert site snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listInstances(ctx context.Context) ([]AlertInstanceEvaluationSnapshot, error) {
	var rows []AlertInstanceEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT i.id, i.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
i.node_name, i.current_status, i.last_seen_at, i.last_synced_at, i.updated_at, i.retired_at,
latest.id AS sample_id, latest.minute_ts AS sampled_at, latest.status AS sample_status,
CAST(latest.cpu_percent AS CHAR) AS cpu_percent,
CAST(latest.memory_percent AS CHAR) AS memory_percent,
CAST(latest.disk_used_percent AS CHAR) AS disk_used_percent
FROM site_instance i
JOIN site s ON s.id = i.site_id
LEFT JOIN site_instance_status_minutely latest
  ON latest.site_id = i.site_id AND latest.node_name = i.node_name
 AND latest.minute_ts = (
   SELECT MAX(m.minute_ts) FROM site_instance_status_minutely m
   WHERE m.site_id = i.site_id AND m.node_name = i.node_name
 )
ORDER BY i.site_id, i.node_name`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert instance snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listAccounts(ctx context.Context) ([]AlertAccountEvaluationSnapshot, error) {
	var rows []AlertAccountEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT a.id, a.site_id, a.username, a.display_name,
a.managed_status, a.remote_state, a.remote_status, a.quota, a.last_synced_at,
c.status AS customer_status, s.management_status AS site_management_status,
s.auth_status AS site_auth_status, s.statistics_end_at, a.updated_at,
s.updated_at AS site_updated_at, c.updated_at AS customer_updated_at
FROM account a
JOIN customer c ON c.id = a.customer_id
JOIN site s ON s.id = a.site_id
ORDER BY a.id`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert account snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listCollectionWindows(ctx context.Context) ([]AlertCollectionEvaluationSnapshot, error) {
	var rows []AlertCollectionEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT cw.id, cw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.updated_at AS site_updated_at,
s.statistics_start_at, s.statistics_end_at,
cw.hour_ts, cw.status, cw.last_error_code, cw.updated_at
FROM collection_window cw
JOIN site s ON s.id = cw.site_id
WHERE cw.status = 'missing'
   OR cw.last_error_code = ?
   OR EXISTS (
      SELECT 1 FROM alert_event e
      WHERE e.active_key IS NOT NULL AND e.site_id = cw.site_id
        AND e.target_type = 'collection'
        AND e.target_key = CONCAT(CAST(cw.site_id AS CHAR), '/', CAST(cw.hour_ts AS CHAR))
        AND e.rule_key IN ('collection_missing', 'validation_failed')
   )
ORDER BY cw.site_id, cw.hour_ts`, string(constant.MessageDataValidationMismatch)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert collection snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listBackfills(ctx context.Context) ([]AlertBackfillEvaluationSnapshot, error) {
	var rows []AlertBackfillEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT r.id AS run_id, r.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at,
r.status, r.error_code, r.updated_at,
EXISTS (SELECT 1 FROM collection_run_window rw WHERE rw.run_id = r.id) AS has_windows,
NOT EXISTS (
  SELECT 1 FROM collection_run_window rw
  LEFT JOIN collection_window cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts
  WHERE rw.run_id = r.id AND (cw.id IS NULL OR cw.status NOT IN ('complete', 'unavailable'))
) AS facts_repaired
FROM collection_run r
JOIN site s ON s.id = r.site_id
WHERE r.task_type = ? AND r.site_id IS NOT NULL
  AND (r.status = 'failed' OR EXISTS (
    SELECT 1 FROM alert_event e
    WHERE e.active_key IS NOT NULL AND e.rule_key = 'backfill_failed'
      AND e.target_type = 'collection'
      AND e.target_key = CONCAT(CAST(r.site_id AS CHAR), '/', CAST(r.id AS CHAR))
  ))
ORDER BY r.id`, constant.TaskTypeUsageBackfill).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert backfill snapshots: %w", err)
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) listValidations(ctx context.Context) ([]AlertValidationEvaluationSnapshot, error) {
	var rows []AlertValidationEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`WITH ranked_validation AS (
  SELECT rw.site_id, rw.hour_ts, rw.status, rw.error_code, rw.updated_at, rw.id,
         ROW_NUMBER() OVER (PARTITION BY rw.site_id, rw.hour_ts ORDER BY rw.updated_at DESC, rw.id DESC) AS row_rank
  FROM collection_run_window rw
  JOIN collection_run r ON r.id = rw.run_id
  WHERE r.task_type = ?
)
SELECT ranked.id AS run_window_id, ranked.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at,
ranked.hour_ts, ranked.status, ranked.error_code, ranked.updated_at, cw.status AS fact_status
FROM ranked_validation ranked
JOIN site s ON s.id = ranked.site_id
LEFT JOIN collection_window cw ON cw.site_id = ranked.site_id AND cw.hour_ts = ranked.hour_ts
WHERE ranked.row_rank = 1
  AND (ranked.status = 'failed' OR EXISTS (
    SELECT 1 FROM alert_event e
    WHERE e.active_key IS NOT NULL AND e.rule_key = 'validation_failed'
      AND e.target_type = 'collection'
      AND e.target_key = CONCAT(CAST(ranked.site_id AS CHAR), '/', CAST(ranked.hour_ts AS CHAR))
  ))
ORDER BY ranked.site_id, ranked.hour_ts`, constant.TaskTypeUsageValidation).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load alert validation snapshots: %w", err)
	}
	return rows, nil
}
