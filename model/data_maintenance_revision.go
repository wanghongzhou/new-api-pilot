package model

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"gorm.io/gorm/clause"
)

type maintenanceRevisionSite struct {
	ID                int64  `gorm:"column:id"`
	ConfigVersion     int    `gorm:"column:config_version"`
	MonitoringStartAt *int64 `gorm:"column:monitoring_start_at"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
}

func (repository *DataMaintenanceRepository) lockResourceRevisionRows(ctx context.Context, siteID, start, end int64) error {
	var pauses []SiteMonitoringPause
	if err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id=? AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)", siteID, end, start).Order("id ASC").Find(&pauses).Error; err != nil {
		return err
	}
	var lifecycles []SiteInstanceLifecycle
	return repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id=? AND evidence_status='known' AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)", siteID, end, start).Order("node_name COLLATE utf8mb4_bin ASC,start_minute_ts ASC,id ASC").Find(&lifecycles).Error
}

type maintenanceRevisionPause struct {
	ID, SiteID, StartMinuteTS int64
	EndMinuteTS               *int64
	CreatedAt                 int64
}
type maintenanceRevisionLifecycle struct {
	ID, SiteID    int64
	NodeName      string
	StartMinuteTS int64
	EndMinuteTS   *int64
	UpdatedAt     int64
}

func (repository *DataMaintenanceRepository) ResourceScopeRevision(ctx context.Context, siteID, start, end int64) (string, error) {
	if repository == nil || repository.db == nil || start <= 0 || end <= start || siteID < 0 {
		return "", ErrDataMaintenanceContract
	}
	siteQuery := repository.db.WithContext(ctx).Model(&Site{}).Select("id,config_version,monitoring_start_at,statistics_end_at").
		Where("monitoring_start_at IS NOT NULL AND monitoring_start_at < ? AND (statistics_end_at IS NULL OR statistics_end_at > ?)", end, start)
	if siteID > 0 {
		siteQuery = siteQuery.Where("id=?", siteID)
	}
	var sites []maintenanceRevisionSite
	if err := siteQuery.Order("id ASC").Find(&sites).Error; err != nil {
		return "", err
	}
	siteIDs := make([]int64, 0, len(sites))
	siteRows := make([]any, 0, len(sites))
	for _, site := range sites {
		siteIDs = append(siteIDs, site.ID)
		siteRows = append(siteRows, []any{site.ID, site.ConfigVersion, site.MonitoringStartAt, site.StatisticsEndAt})
	}
	pauseRows := make([]any, 0)
	lifecycleRows := make([]any, 0)
	if len(siteIDs) > 0 {
		var pauses []maintenanceRevisionPause
		if err := repository.db.WithContext(ctx).Table("site_monitoring_pause").Select("id,site_id,start_minute_ts,end_minute_ts,created_at").Where("site_id IN ? AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)", siteIDs, end, start).Order("id ASC,site_id ASC,start_minute_ts ASC,end_minute_ts ASC,created_at ASC").Find(&pauses).Error; err != nil {
			return "", err
		}
		for _, pause := range pauses {
			pauseRows = append(pauseRows, []any{pause.ID, pause.SiteID, pause.StartMinuteTS, pause.EndMinuteTS, pause.CreatedAt})
		}
		var lifecycles []maintenanceRevisionLifecycle
		if err := repository.db.WithContext(ctx).Table("site_instance_lifecycle").Select("id,site_id,node_name,start_minute_ts,end_minute_ts,updated_at").Where("site_id IN ? AND evidence_status='known' AND start_minute_ts < ? AND (end_minute_ts IS NULL OR end_minute_ts > ?)", siteIDs, end, start).Order("id ASC,site_id ASC,node_name COLLATE utf8mb4_bin ASC,start_minute_ts ASC,end_minute_ts ASC,updated_at ASC").Find(&lifecycles).Error; err != nil {
			return "", err
		}
		for _, lifecycle := range lifecycles {
			lifecycleRows = append(lifecycleRows, []any{lifecycle.ID, lifecycle.SiteID, lifecycle.NodeName, lifecycle.StartMinuteTS, lifecycle.EndMinuteTS, lifecycle.UpdatedAt})
		}
	}
	return resourceScopeRevisionFromRows(siteRows, pauseRows, lifecycleRows)
}

func resourceScopeRevisionFromRows(siteRows, pauseRows, lifecycleRows []any) (string, error) {
	canonical := []any{[]any{"sites", siteRows}, []any{"pauses", pauseRows}, []any{"lifecycles", lifecycleRows}}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode resource scope revision: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(payload)), nil
}
