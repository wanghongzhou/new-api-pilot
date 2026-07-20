package model

import (
	"context"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err := tx.WithContext(ctx).Model(&CollectionCursor{}).Where("id = ?", cursor.ID).
		Updates(map[string]any{"last_complete_hour": highest, "updated_at": now}).Error; err != nil {
		return CollectionCursor{}, err
	}
	cursor.LastCompleteHour = highest
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
