package model

import (
	"context"
	"fmt"

	"new-api-pilot/constant"
)

type AlertWindowIdentity struct {
	ID        int64 `gorm:"column:id"`
	HourTS    int64 `gorm:"column:hour_ts"`
	UpdatedAt int64 `gorm:"column:updated_at"`
}

const alertChannelSnapshotSelect = `SELECT s.id AS site_id, s.name AS site_name, s.config_version AS site_config_version,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
h.hour_ts, h.collected_at, h.channel_count, h.data_status, h.config_version,
CAST(h.balance_total AS CHAR) AS balance_total,
CAST(h.response_time_avg_ms AS CHAR) AS response_time_avg_ms,
CAST(h.availability_rate AS CHAR) AS availability_rate
FROM site s
LEFT JOIN site_channel_inventory_hourly h
  ON h.site_id = s.id AND h.hour_ts = ? AND h.collected_at = ?`

const alertSiteSnapshotSelect = `SELECT s.id, s.name, s.config_version, s.management_status, s.auth_status,
s.version, s.data_export_enabled, s.probe_fail_count, s.last_probe_at,
s.statistics_start_at, s.statistics_end_at, s.updated_at,
latest.id AS resource_sample_id, latest.minute_ts AS resource_sample_at,
latest.instance_count AS resource_instance_count
FROM site s
LEFT JOIN site_status_minutely latest
  ON latest.site_id = s.id
 AND latest.minute_ts = (SELECT MAX(m.minute_ts) FROM site_status_minutely m WHERE m.site_id = s.id)`

const alertInstanceSnapshotSelect = `SELECT i.id, i.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
i.node_name, i.current_status, i.last_seen_at, i.last_synced_at, i.updated_at,
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
 )`

const alertAccountSnapshotSelect = `SELECT a.id, a.site_id, a.username, a.display_name,
a.managed_status, a.remote_state, a.remote_status, a.quota, a.last_synced_at,
c.status AS customer_status, s.management_status AS site_management_status,
s.auth_status AS site_auth_status, s.statistics_end_at, a.updated_at,
s.updated_at AS site_updated_at, c.updated_at AS customer_updated_at
FROM account a
JOIN customer c ON c.id = a.customer_id
JOIN site s ON s.id = a.site_id`

func (repository *AlertEvaluationRepository) LoadProbeAlertSnapshot(
	ctx context.Context,
	siteID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	rows, err := repository.scanAlertSites(ctx, alertSiteSnapshotSelect+`
WHERE s.id = ? AND s.last_probe_at = ?`, siteID, observedAt)
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed probe alert snapshot: %w", err)
	}
	return AlertEvaluationSnapshot{Sites: rows}, nil
}

func (repository *CollectionTaskRepository) CommittedCollectionWindowAlertIdentity(
	ctx context.Context,
	siteID int64,
	hourTS int64,
) (AlertWindowIdentity, error) {
	if repository == nil || repository.db == nil || siteID <= 0 || hourTS <= 0 {
		return AlertWindowIdentity{}, fmt.Errorf("collection window alert identity repository is required")
	}
	var identity AlertWindowIdentity
	err := repository.db.WithContext(ctx).Raw(`SELECT id, hour_ts, updated_at
FROM collection_window
WHERE site_id = ? AND hour_ts = ?`, siteID, hourTS).Scan(&identity).Error
	if err != nil {
		return AlertWindowIdentity{}, fmt.Errorf("load committed collection window alert identity: %w", err)
	}
	if identity.ID <= 0 || identity.HourTS != hourTS || identity.UpdatedAt <= 0 {
		return AlertWindowIdentity{}, fmt.Errorf("committed collection window alert identity is missing")
	}
	return identity, nil
}

func (repository *AlertEvaluationRepository) LoadResourceAlertSnapshot(
	ctx context.Context,
	siteID int64,
	minuteTS int64,
) (AlertEvaluationSnapshot, error) {
	if err := repository.requireAlertEvaluationRepository(); err != nil {
		return AlertEvaluationSnapshot{}, err
	}
	var sites []AlertSiteEvaluationSnapshot
	err := repository.db.WithContext(ctx).Raw(`SELECT s.id, s.name, s.config_version, s.management_status, s.auth_status,
s.version, s.data_export_enabled, s.probe_fail_count, s.last_probe_at,
s.statistics_start_at, s.statistics_end_at, s.updated_at,
sample.id AS resource_sample_id, sample.minute_ts AS resource_sample_at,
sample.instance_count AS resource_instance_count
FROM site s
JOIN site_status_minutely sample ON sample.site_id = s.id AND sample.minute_ts = ?
WHERE s.id = ?`, minuteTS, siteID).Scan(&sites).Error
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed site resource alert snapshot: %w", err)
	}
	if len(sites) == 0 {
		return AlertEvaluationSnapshot{}, nil
	}
	var instances []AlertInstanceEvaluationSnapshot
	err = repository.db.WithContext(ctx).Raw(`SELECT i.id, i.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
i.node_name, i.current_status, i.last_seen_at, i.last_synced_at, i.updated_at,
sample.id AS sample_id, sample.minute_ts AS sampled_at, sample.status AS sample_status,
CAST(sample.cpu_percent AS CHAR) AS cpu_percent,
CAST(sample.memory_percent AS CHAR) AS memory_percent,
CAST(sample.disk_used_percent AS CHAR) AS disk_used_percent
FROM site_instance i
JOIN site s ON s.id = i.site_id
JOIN site_instance_status_minutely sample
  ON sample.site_id = i.site_id AND sample.node_name = i.node_name AND sample.minute_ts = ?
WHERE i.site_id = ?
ORDER BY i.node_name`, minuteTS, siteID).Scan(&instances).Error
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed instance resource alert snapshots: %w", err)
	}
	return AlertEvaluationSnapshot{Sites: sites, Instances: instances}, nil
}

func (repository *AlertEvaluationRepository) LoadUserAlertSnapshot(
	ctx context.Context,
	siteID int64,
	accountID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	var rows []AlertAccountEvaluationSnapshot
	var err error
	switch {
	case accountID > 0 && siteID > 0:
		err = repository.scanAlertAccounts(ctx, &rows, alertAccountSnapshotSelect+`
WHERE a.id = ? AND a.site_id = ? AND (a.last_synced_at = ? OR a.updated_at = ?)`,
			accountID, siteID, observedAt, observedAt)
	case accountID > 0:
		err = repository.scanAlertAccounts(ctx, &rows, alertAccountSnapshotSelect+`
WHERE a.id = ? AND (a.last_synced_at = ? OR a.updated_at = ?)`, accountID, observedAt, observedAt)
	default:
		err = repository.scanAlertAccounts(ctx, &rows, alertAccountSnapshotSelect+`
WHERE a.site_id = ? AND (a.last_synced_at = ? OR a.updated_at = ?)
ORDER BY a.id`, siteID, observedAt, observedAt)
	}
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed user alert snapshots: %w", err)
	}
	return AlertEvaluationSnapshot{Accounts: rows}, nil
}

func (repository *AlertEvaluationRepository) LoadChannelAlertSnapshot(
	ctx context.Context,
	siteID int64,
	hourTS int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	var rows []AlertChannelEvaluationSnapshot
	if err := repository.scanRows(ctx, &rows, alertChannelSnapshotSelect+`
WHERE s.id = ?`, hourTS, observedAt, siteID); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed channel alert snapshot: %w", err)
	}
	return AlertEvaluationSnapshot{Channels: rows}, nil
}

func (repository *AlertEvaluationRepository) LoadWindowAlertSnapshot(
	ctx context.Context,
	windowType string,
	rowID int64,
	hourTS int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	switch windowType {
	case "collection_window":
		return repository.loadCommittedCollectionWindow(ctx, rowID, hourTS, observedAt)
	case "validation_run_window":
		return repository.loadCommittedValidationWindow(ctx, rowID, hourTS, observedAt)
	case "backfill_run":
		return repository.loadCommittedBackfillRun(ctx, rowID, observedAt)
	default:
		return AlertEvaluationSnapshot{}, fmt.Errorf("unsupported committed alert window type")
	}
}

func (repository *AlertEvaluationRepository) LoadAuthAlertSnapshot(
	ctx context.Context,
	siteID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	return repository.loadCommittedSiteScope(ctx, siteID, observedAt)
}

func (repository *AlertEvaluationRepository) LoadLifecycleAlertSnapshot(
	ctx context.Context,
	scopeType string,
	scopeID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	switch scopeType {
	case "site":
		return repository.loadCommittedSiteScope(ctx, scopeID, observedAt)
	case "customer":
		var rows []AlertAccountEvaluationSnapshot
		err := repository.scanAlertAccounts(ctx, &rows, alertAccountSnapshotSelect+`
WHERE c.id = ? AND c.updated_at = ?
ORDER BY a.id`, scopeID, observedAt)
		if err != nil {
			return AlertEvaluationSnapshot{}, fmt.Errorf("load committed customer lifecycle alert snapshots: %w", err)
		}
		return AlertEvaluationSnapshot{Accounts: rows}, nil
	case "account":
		var rows []AlertAccountEvaluationSnapshot
		err := repository.scanAlertAccounts(ctx, &rows, alertAccountSnapshotSelect+`
WHERE a.id = ? AND a.updated_at = ?`, scopeID, observedAt)
		if err != nil {
			return AlertEvaluationSnapshot{}, fmt.Errorf("load committed account lifecycle alert snapshot: %w", err)
		}
		return AlertEvaluationSnapshot{Accounts: rows}, nil
	default:
		return AlertEvaluationSnapshot{}, fmt.Errorf("unsupported committed alert lifecycle scope")
	}
}

func (repository *AlertEvaluationRepository) loadCommittedSiteScope(
	ctx context.Context,
	siteID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	sites, err := repository.scanAlertSites(ctx, alertSiteSnapshotSelect+`
WHERE s.id = ? AND s.updated_at = ?`, siteID, observedAt)
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed site lifecycle snapshot: %w", err)
	}
	if len(sites) == 0 {
		return AlertEvaluationSnapshot{}, nil
	}
	var snapshot AlertEvaluationSnapshot
	snapshot.Sites = sites
	if err := repository.scanRows(ctx, &snapshot.Instances, alertInstanceSnapshotSelect+`
WHERE i.site_id = ?
ORDER BY i.node_name`, siteID); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle instance snapshots: %w", err)
	}
	if err := repository.scanAlertAccounts(ctx, &snapshot.Accounts, alertAccountSnapshotSelect+`
WHERE a.site_id = ?
ORDER BY a.id`, siteID); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle account snapshots: %w", err)
	}
	if err := repository.scanRows(ctx, &snapshot.Channels, `SELECT s.id AS site_id, s.name AS site_name, s.config_version AS site_config_version,
s.management_status, s.auth_status, s.statistics_end_at, s.updated_at AS site_updated_at,
h.hour_ts, h.collected_at, h.channel_count, h.data_status, h.config_version,
CAST(h.balance_total AS CHAR) AS balance_total,
CAST(h.response_time_avg_ms AS CHAR) AS response_time_avg_ms,
CAST(h.availability_rate AS CHAR) AS availability_rate
FROM site s
LEFT JOIN site_channel_inventory_hourly h
  ON h.site_id = s.id
 AND h.hour_ts = (SELECT MAX(latest.hour_ts) FROM site_channel_inventory_hourly latest WHERE latest.site_id = s.id)
WHERE s.id = ?`, siteID); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle channel snapshot: %w", err)
	}
	if err := repository.scanRows(ctx, &snapshot.CollectionWindows, `SELECT cw.id, cw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.updated_at AS site_updated_at,
s.statistics_start_at, s.statistics_end_at,
cw.hour_ts, cw.status, cw.last_error_code, cw.updated_at
FROM collection_window cw
JOIN site s ON s.id = cw.site_id
WHERE cw.site_id = ? AND (cw.status = 'missing' OR cw.last_error_code = ? OR EXISTS (
  SELECT 1 FROM alert_event e
  WHERE e.active_key IS NOT NULL AND e.site_id = cw.site_id
    AND e.target_type = 'collection'
    AND e.target_key = CONCAT(CAST(cw.site_id AS CHAR), '/', CAST(cw.hour_ts AS CHAR))
    AND e.rule_key IN ('collection_missing', 'validation_failed')
))
ORDER BY cw.hour_ts`, siteID, string(constant.MessageDataValidationMismatch)); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle collection snapshots: %w", err)
	}
	if err := repository.scanRows(ctx, &snapshot.Backfills, `SELECT r.id AS run_id, r.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at, r.status, r.error_code, r.updated_at,
EXISTS (SELECT 1 FROM collection_run_window rw WHERE rw.run_id = r.id) AS has_windows,
NOT EXISTS (
  SELECT 1 FROM collection_run_window rw
  LEFT JOIN collection_window cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts
  WHERE rw.run_id = r.id AND (cw.id IS NULL OR cw.status NOT IN ('complete', 'unavailable'))
) AS facts_repaired
FROM collection_run r
JOIN site s ON s.id = r.site_id
WHERE r.site_id = ? AND r.task_type = ? AND (r.status = 'failed' OR EXISTS (
  SELECT 1 FROM alert_event e
  WHERE e.active_key IS NOT NULL AND e.rule_key = 'backfill_failed'
    AND e.target_type = 'collection'
    AND e.target_key = CONCAT(CAST(r.site_id AS CHAR), '/', CAST(r.id AS CHAR))
))
ORDER BY r.id`, siteID, constant.TaskTypeUsageBackfill); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle backfill snapshots: %w", err)
	}
	if err := repository.scanRows(ctx, &snapshot.Validations, `WITH ranked_validation AS (
  SELECT rw.site_id, rw.hour_ts, rw.status, rw.error_code, rw.updated_at, rw.id,
         ROW_NUMBER() OVER (PARTITION BY rw.site_id, rw.hour_ts ORDER BY rw.updated_at DESC, rw.id DESC) AS row_rank
  FROM collection_run_window rw
  JOIN collection_run r ON r.id = rw.run_id
  WHERE r.task_type = ? AND rw.site_id = ?
)
SELECT ranked.id AS run_window_id, ranked.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at,
ranked.hour_ts, ranked.status, ranked.error_code, ranked.updated_at, cw.status AS fact_status
FROM ranked_validation ranked
JOIN site s ON s.id = ranked.site_id
LEFT JOIN collection_window cw ON cw.site_id = ranked.site_id AND cw.hour_ts = ranked.hour_ts
WHERE ranked.row_rank = 1 AND (ranked.status = 'failed' OR EXISTS (
  SELECT 1 FROM alert_event e
  WHERE e.active_key IS NOT NULL AND e.rule_key = 'validation_failed'
    AND e.target_type = 'collection'
    AND e.target_key = CONCAT(CAST(ranked.site_id AS CHAR), '/', CAST(ranked.hour_ts AS CHAR))
))
ORDER BY ranked.hour_ts`, constant.TaskTypeUsageValidation, siteID); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load site lifecycle validation snapshots: %w", err)
	}
	return snapshot, nil
}

func (repository *AlertEvaluationRepository) loadCommittedCollectionWindow(
	ctx context.Context,
	rowID int64,
	hourTS int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	var rows []AlertCollectionEvaluationSnapshot
	err := repository.scanRows(ctx, &rows, `SELECT cw.id, cw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.updated_at AS site_updated_at,
s.statistics_start_at, s.statistics_end_at,
cw.hour_ts, cw.status, cw.last_error_code, cw.updated_at
FROM collection_window cw
JOIN site s ON s.id = cw.site_id
WHERE cw.id = ? AND cw.hour_ts = ? AND cw.updated_at = ?`, rowID, hourTS, observedAt)
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed collection window alert snapshot: %w", err)
	}
	snapshot := AlertEvaluationSnapshot{CollectionWindows: rows}
	if len(rows) == 0 {
		return snapshot, nil
	}
	if err := repository.loadLatestValidationForHour(ctx, rows[0].SiteID, hourTS, &snapshot.Validations); err != nil {
		return AlertEvaluationSnapshot{}, err
	}
	return snapshot, nil
}

func (repository *AlertEvaluationRepository) loadCommittedValidationWindow(
	ctx context.Context,
	rowID int64,
	hourTS int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	var validations []AlertValidationEvaluationSnapshot
	err := repository.scanRows(ctx, &validations, `SELECT rw.id AS run_window_id, rw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at,
rw.hour_ts, rw.status, rw.error_code, rw.updated_at, cw.status AS fact_status
FROM collection_run_window rw
JOIN collection_run r ON r.id = rw.run_id AND r.task_type = ?
JOIN site s ON s.id = rw.site_id
LEFT JOIN collection_window cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts
WHERE rw.id = ? AND rw.hour_ts = ? AND rw.updated_at = ?`,
		constant.TaskTypeUsageValidation, rowID, hourTS, observedAt)
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed validation window alert snapshot: %w", err)
	}
	snapshot := AlertEvaluationSnapshot{Validations: validations}
	if len(validations) == 0 {
		return snapshot, nil
	}
	if err := repository.scanRows(ctx, &snapshot.CollectionWindows, `SELECT cw.id, cw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.updated_at AS site_updated_at,
s.statistics_start_at, s.statistics_end_at,
cw.hour_ts, cw.status, cw.last_error_code, cw.updated_at
FROM collection_window cw
JOIN site s ON s.id = cw.site_id
WHERE cw.site_id = ? AND cw.hour_ts = ?`, validations[0].SiteID, hourTS); err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load validation fact alert snapshot: %w", err)
	}
	return snapshot, nil
}

func (repository *AlertEvaluationRepository) loadCommittedBackfillRun(
	ctx context.Context,
	runID int64,
	observedAt int64,
) (AlertEvaluationSnapshot, error) {
	var rows []AlertBackfillEvaluationSnapshot
	err := repository.scanRows(ctx, &rows, `SELECT r.id AS run_id, r.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at, r.status, r.error_code, GREATEST(r.updated_at, ?) AS updated_at,
EXISTS (SELECT 1 FROM collection_run_window rw WHERE rw.run_id = r.id) AS has_windows,
NOT EXISTS (
  SELECT 1 FROM collection_run_window rw
  LEFT JOIN collection_window cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts
  WHERE rw.run_id = r.id AND (cw.id IS NULL OR cw.status NOT IN ('complete', 'unavailable'))
) AS facts_repaired
FROM collection_run r
JOIN site s ON s.id = r.site_id
WHERE r.task_type = ? AND (
  (r.id = ? AND r.updated_at = ?) OR
  (r.site_id = (
    SELECT committed.site_id FROM collection_run committed
    WHERE committed.id = ? AND committed.updated_at = ? AND committed.task_type = ?
  ) AND EXISTS (
    SELECT 1 FROM alert_event e
    WHERE e.active_key IS NOT NULL AND e.rule_key = 'backfill_failed'
      AND e.target_type = 'collection'
      AND e.target_key = CONCAT(CAST(r.site_id AS CHAR), '/', CAST(r.id AS CHAR))
  ))
)
ORDER BY r.id`, observedAt, constant.TaskTypeUsageBackfill, runID, observedAt,
		runID, observedAt, constant.TaskTypeUsageBackfill)
	if err != nil {
		return AlertEvaluationSnapshot{}, fmt.Errorf("load committed backfill run alert snapshot: %w", err)
	}
	return AlertEvaluationSnapshot{Backfills: rows}, nil
}

func (repository *AlertEvaluationRepository) loadLatestValidationForHour(
	ctx context.Context,
	siteID int64,
	hourTS int64,
	rows *[]AlertValidationEvaluationSnapshot,
) error {
	err := repository.scanRows(ctx, rows, `SELECT rw.id AS run_window_id, rw.site_id, s.name AS site_name,
s.management_status, s.auth_status, s.data_export_enabled, s.statistics_end_at,
s.updated_at AS site_updated_at,
rw.hour_ts, rw.status, rw.error_code, rw.updated_at, cw.status AS fact_status
FROM collection_run_window rw
JOIN collection_run r ON r.id = rw.run_id AND r.task_type = ?
JOIN site s ON s.id = rw.site_id
LEFT JOIN collection_window cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts
WHERE rw.site_id = ? AND rw.hour_ts = ?
ORDER BY rw.updated_at DESC, rw.id DESC
LIMIT 1`, constant.TaskTypeUsageValidation, siteID, hourTS)
	if err != nil {
		return fmt.Errorf("load latest validation alert snapshot: %w", err)
	}
	return nil
}

func (repository *AlertEvaluationRepository) scanAlertSites(
	ctx context.Context,
	query string,
	args ...any,
) ([]AlertSiteEvaluationSnapshot, error) {
	var rows []AlertSiteEvaluationSnapshot
	if err := repository.scanRows(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (repository *AlertEvaluationRepository) scanAlertAccounts(
	ctx context.Context,
	rows *[]AlertAccountEvaluationSnapshot,
	query string,
	args ...any,
) error {
	return repository.scanRows(ctx, rows, query, args...)
}

func (repository *AlertEvaluationRepository) scanRows(
	ctx context.Context,
	destination any,
	query string,
	args ...any,
) error {
	if err := repository.requireAlertEvaluationRepository(); err != nil {
		return err
	}
	return repository.db.WithContext(ctx).Raw(query, args...).Scan(destination).Error
}

func (repository *AlertEvaluationRepository) requireAlertEvaluationRepository() error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("alert evaluation repository is required")
	}
	return nil
}
