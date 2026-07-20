package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

type OperationalTaskState struct {
	TaskType        string `gorm:"column:task_type"`
	Status          string `gorm:"column:status"`
	Count           int64  `gorm:"column:count"`
	OldestCreatedAt *int64 `gorm:"column:oldest_created_at"`
}

type OperationalStatusCount struct {
	Status string `gorm:"column:status"`
	Count  int64  `gorm:"column:count"`
}

type OperationalAlertCount struct {
	Level  string `gorm:"column:level"`
	Status string `gorm:"column:status"`
	Count  int64  `gorm:"column:count"`
}

// OperationalEligibleSite deliberately omits the site identity. Metrics only
// need an aggregate lag and stale count, never a per-site label or value.
type OperationalEligibleSite struct {
	MonitoringStartAt  *int64 `gorm:"column:monitoring_start_at"`
	StatisticsStartAt  *int64 `gorm:"column:statistics_start_at"`
	NewestCompleteHour *int64 `gorm:"column:newest_complete_hour"`
}

type OperationalMetricsSnapshot struct {
	Tasks           []OperationalTaskState
	Windows         []OperationalStatusCount
	EligibleSites   []OperationalEligibleSite
	Alerts          []OperationalAlertCount
	AlertDeliveries []OperationalStatusCount
	Exports         []OperationalStatusCount
	DatabaseUnix    int64
}

type OperationalMetricsRepository struct {
	db *gorm.DB
}

func NewOperationalMetricsRepository(db *gorm.DB) *OperationalMetricsRepository {
	return &OperationalMetricsRepository{db: db}
}

func (repository *OperationalMetricsRepository) Snapshot(
	ctx context.Context,
) (OperationalMetricsSnapshot, error) {
	if repository == nil || repository.db == nil || ctx == nil {
		return OperationalMetricsSnapshot{}, errors.New("operational metrics repository is required")
	}
	var snapshot OperationalMetricsSnapshot
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT task_type, status, COUNT(*) AS count,
MIN(CASE WHEN status = 'pending' THEN created_at END) AS oldest_created_at
FROM collection_run
WHERE status IN ('pending', 'running')
GROUP BY task_type, status`).Scan(&snapshot.Tasks).Error; err != nil {
			return fmt.Errorf("read collection task metrics: %w", err)
		}
		if err := tx.Raw(`SELECT status, COUNT(*) AS count
FROM collection_window
WHERE status IN ('pending', 'complete', 'missing', 'unavailable')
GROUP BY status`).Scan(&snapshot.Windows).Error; err != nil {
			return fmt.Errorf("read collection window metrics: %w", err)
		}
		if err := tx.Raw(`SELECT s.monitoring_start_at, s.statistics_start_at,
MAX(CASE WHEN w.status = 'complete' THEN w.hour_ts END) AS newest_complete_hour
FROM site AS s
LEFT JOIN collection_window AS w ON w.site_id = s.id
WHERE s.management_status = ? AND s.auth_status = ?
  AND s.statistics_end_at IS NULL AND s.online_status <> ?
  AND s.data_export_enabled = TRUE
GROUP BY s.id, s.monitoring_start_at, s.statistics_start_at`,
			constant.SiteManagementActive,
			constant.SiteAuthAuthorized,
			constant.SiteOnlineOffline,
		).Scan(&snapshot.EligibleSites).Error; err != nil {
			return fmt.Errorf("read collection lag metrics: %w", err)
		}
		if err := tx.Raw(`SELECT level, status, COUNT(*) AS count
FROM alert_event
WHERE level IN ('warning', 'critical') AND status IN ('pending', 'firing')
GROUP BY level, status`).Scan(&snapshot.Alerts).Error; err != nil {
			return fmt.Errorf("read alert event metrics: %w", err)
		}
		if err := tx.Raw(`SELECT status, COUNT(*) AS count
FROM alert_delivery
WHERE status IN ('pending', 'success', 'failed')
GROUP BY status`).Scan(&snapshot.AlertDeliveries).Error; err != nil {
			return fmt.Errorf("read alert delivery metrics: %w", err)
		}
		if err := tx.Raw(`SELECT status, COUNT(*) AS count
FROM export_job
WHERE status IN ('pending', 'running', 'success', 'failed', 'expired')
GROUP BY status`).Scan(&snapshot.Exports).Error; err != nil {
			return fmt.Errorf("read export job metrics: %w", err)
		}
		var clock struct {
			DatabaseUnix int64 `gorm:"column:database_unix"`
		}
		if err := tx.Raw(`SELECT UNIX_TIMESTAMP() AS database_unix`).Scan(&clock).Error; err != nil {
			return fmt.Errorf("read database clock metric: %w", err)
		}
		snapshot.DatabaseUnix = clock.DatabaseUnix
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
	if err != nil {
		return OperationalMetricsSnapshot{}, err
	}
	return snapshot, nil
}
