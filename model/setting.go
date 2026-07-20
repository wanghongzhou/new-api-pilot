package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PlatformSetting struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Key       string `gorm:"column:setting_key"`
	Value     string `gorm:"column:setting_value"`
	ValueType string `gorm:"column:value_type"`
	Secret    bool   `gorm:"column:is_secret"`
	UpdatedAt int64  `gorm:"column:updated_at"`
}

func (PlatformSetting) TableName() string { return "platform_setting" }

type SettingRepository struct{ db *gorm.DB }

func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

func (repository *SettingRepository) Transaction(
	ctx context.Context,
	operation func(*SettingRepository) error,
) error {
	if repository == nil || repository.db == nil || operation == nil {
		return errors.New("setting repository transaction dependencies are required")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(NewSettingRepository(tx))
	})
}

func (repository *SettingRepository) List(ctx context.Context, keys []string) ([]PlatformSetting, error) {
	if repository == nil || repository.db == nil || len(keys) == 0 {
		return nil, errors.New("setting repository read dependencies are required")
	}
	var rows []PlatformSetting
	err := repository.db.WithContext(ctx).Where("setting_key IN ?", keys).Order("setting_key ASC").Find(&rows).Error
	return rows, err
}

func (repository *SettingRepository) ListForUpdate(ctx context.Context, keys []string) ([]PlatformSetting, error) {
	if repository == nil || repository.db == nil || len(keys) == 0 {
		return nil, errors.New("setting repository lock dependencies are required")
	}
	var rows []PlatformSetting
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("setting_key IN ?", keys).Order("setting_key ASC").Find(&rows).Error
	return rows, err
}

func (repository *SettingRepository) UpdateValue(
	ctx context.Context,
	id int64,
	value string,
	updatedAt int64,
) error {
	if repository == nil || repository.db == nil || id <= 0 || updatedAt <= 0 {
		return errors.New("invalid setting update")
	}
	result := repository.db.WithContext(ctx).Model(&PlatformSetting{}).Where("id = ?", id).
		Updates(map[string]any{"setting_value": value, "updated_at": updatedAt})
	if result.Error != nil {
		return fmt.Errorf("update platform setting: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("platform setting update lost its locked row")
	}
	return nil
}
