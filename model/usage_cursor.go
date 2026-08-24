package model

import (
	"context"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
)

const UsageCursorKey = "usage"

type CollectionCursor struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	CursorKey        string `gorm:"column:cursor_key"`
	LastCompleteHour *int64 `gorm:"column:last_complete_hour"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (CollectionCursor) TableName() string { return "collection_cursor" }

func ReconcileUsageCursor(
	ctx context.Context,
	tx *gorm.DB,
	siteID int64,
	statisticsStartAt int64,
	now int64,
) (CollectionCursor, error) {
	if tx == nil || siteID <= 0 || statisticsStartAt <= 0 || statisticsStartAt%3600 != 0 || now <= 0 {
		return CollectionCursor{}, ErrCollectionRunContract
	}
	cursor := CollectionCursor{SiteID: siteID, CursorKey: UsageCursorKey, UpdatedAt: now}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_id"}, {Name: "cursor_key"}},
		DoNothing: true,
	}).Create(&cursor).Error; err != nil {
		return CollectionCursor{}, err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("site_id = ? AND cursor_key = ?", siteID, UsageCursorKey).First(&cursor).Error; err != nil {
		return CollectionCursor{}, err
	}

	var windows []CollectionWindow
	if err := tx.WithContext(ctx).Where("site_id = ? AND hour_ts >= ?", siteID, statisticsStartAt).
		Order("hour_ts ASC").Find(&windows).Error; err != nil {
		return CollectionCursor{}, err
	}
	expected := statisticsStartAt
	var highest *int64
	for _, window := range windows {
		if window.HourTS != expected || window.Status != CollectionWindowStatusComplete {
			break
		}
		value := window.HourTS
		highest = &value
		if expected > math.MaxInt64-3600 {
			break
		}
		expected += 3600
	}
	updates := map[string]any{"updated_at": now}
	if highest != nil {
		updates["last_complete_hour"] = highest
	}
	if err := tx.WithContext(ctx).Model(&CollectionCursor{}).Where("id = ?", cursor.ID).
		Updates(updates).Error; err != nil {
		return CollectionCursor{}, err
	}
	if highest != nil {
		cursor.LastCompleteHour = highest
	}
	cursor.UpdatedAt = now
	return cursor, nil
}

func FindUsageCursor(ctx context.Context, db *gorm.DB, siteID int64) (CollectionCursor, error) {
	if db == nil || siteID <= 0 {
		return CollectionCursor{}, ErrCollectionRunContract
	}
	var cursor CollectionCursor
	err := db.WithContext(ctx).Where("site_id = ? AND cursor_key = ?", siteID, UsageCursorKey).First(&cursor).Error
	return cursor, err
}

func (repository *SiteRepository) ReconcileUsageCursor(ctx context.Context, siteID, statisticsStartAt, now int64) (CollectionCursor, error) {
	if repository == nil || repository.db == nil {
		return CollectionCursor{}, ErrCollectionRunContract
	}
	return ReconcileUsageCursor(ctx, repository.db, siteID, statisticsStartAt, now)
}

func (repository *SiteRepository) UsageBackfillStatisticsStatus(
	ctx context.Context,
	siteID int64,
	configVersion int,
) (string, error) {
	if repository == nil || repository.db == nil || siteID <= 0 || configVersion <= 0 {
		return "", ErrCollectionRunContract
	}
	var site Site
	if err := repository.db.WithContext(ctx).Select("id", "config_version", "statistics_start_at").
		Where("id = ? AND config_version = ?", siteID, configVersion).Take(&site).Error; err != nil {
		return "", err
	}
	var activeCount int64
	if err := repository.db.WithContext(ctx).Model(&CollectionRun{}).
		Where("site_id = ? AND site_config_version = ? AND task_type = ? AND target_type = 'site' AND target_id = ? AND status IN ?",
			siteID, configVersion, constant.TaskTypeUsageBackfill, siteID,
			[]string{CollectionTaskStatusPending, CollectionTaskStatusRunning}).Count(&activeCount).Error; err != nil {
		return "", err
	}
	if activeCount > 0 {
		return constant.SiteStatisticsBackfilling, nil
	}
	statisticsStart := int64(0)
	if site.StatisticsStartAt != nil {
		statisticsStart = *site.StatisticsStartAt
	}
	var unresolvedTargets int64
	if err := repository.db.WithContext(ctx).Table("collection_run_window AS rw").
		Joins("JOIN collection_run AS r ON r.id = rw.run_id").
		Joins("LEFT JOIN collection_window AS cw ON cw.site_id = rw.site_id AND cw.hour_ts = rw.hour_ts").
		Where("r.site_id = ? AND r.site_config_version = ? AND r.task_type = ? AND r.target_type = 'site' AND r.target_id = ?",
			siteID, configVersion, constant.TaskTypeUsageBackfill, siteID).
		Where("rw.site_id = ? AND rw.hour_ts >= ? AND (cw.id IS NULL OR cw.status <> ?)",
			siteID, statisticsStart, CollectionWindowStatusComplete).
		Distinct("rw.hour_ts").Count(&unresolvedTargets).Error; err != nil {
		return "", err
	}
	if unresolvedTargets > 0 {
		return constant.SiteStatisticsPartial, nil
	}
	var explicitIncomplete int64
	if err := repository.db.WithContext(ctx).Model(&CollectionWindow{}).
		Where("site_id = ? AND hour_ts >= ? AND status IN ?", siteID, statisticsStart,
			[]string{CollectionWindowStatusMissing, CollectionWindowStatusUnavailable}).Count(&explicitIncomplete).Error; err != nil {
		return "", err
	}
	if explicitIncomplete > 0 {
		return constant.SiteStatisticsPartial, nil
	}
	return constant.SiteStatisticsReady, nil
}
