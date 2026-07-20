package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/dto"
)

var ErrUpstreamLogFence = errors.New("upstream log site config changed")

type UpstreamLogFact struct {
	ID                int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID            int64  `gorm:"column:site_id"`
	ConfigVersion     int    `gorm:"column:config_version"`
	UpstreamLogKey    string `gorm:"column:upstream_log_key"`
	UpstreamLogID     int64  `gorm:"column:upstream_log_id"`
	CreatedAt         int64  `gorm:"column:created_at"`
	Type              int    `gorm:"column:type"`
	RemoteUserID      int64  `gorm:"column:remote_user_id"`
	Username          string `gorm:"column:username"`
	ModelName         string `gorm:"column:model_name"`
	TokenID           int64  `gorm:"column:token_id"`
	TokenName         string `gorm:"column:token_name"`
	ChannelID         int64  `gorm:"column:channel_id"`
	UseGroup          string `gorm:"column:use_group"`
	RequestID         string `gorm:"column:request_id"`
	UpstreamRequestID string `gorm:"column:upstream_request_id"`
	Quota             int64  `gorm:"column:quota"`
	PromptTokens      int64  `gorm:"column:prompt_tokens"`
	CompletionTokens  int64  `gorm:"column:completion_tokens"`
	UseTimeSeconds    int64  `gorm:"column:use_time_seconds"`
	IsStream          bool   `gorm:"column:is_stream"`
	ContentRedacted   string `gorm:"column:content_redacted"`
	IP                string `gorm:"column:ip"`
	CollectedAt       int64  `gorm:"column:collected_at"`
}

func (UpstreamLogFact) TableName() string { return "upstream_log_fact" }

type UpstreamLogCollectionState struct {
	SiteID          int64  `gorm:"column:site_id;primaryKey"`
	ConfigVersion   int    `gorm:"column:config_version"`
	Status          string `gorm:"column:status"`
	WindowStart     int64  `gorm:"column:window_start"`
	WindowEnd       int64  `gorm:"column:window_end"`
	LastSuccessAt   *int64 `gorm:"column:last_success_at"`
	LastErrorCode   string `gorm:"column:last_error_code"`
	LastErrorParams []byte `gorm:"column:last_error_params"`
	UpdatedAt       int64  `gorm:"column:updated_at"`
}

func (UpstreamLogCollectionState) TableName() string { return "upstream_log_collection_state" }

type UpstreamLogReadRow struct {
	UpstreamLogFact
	SiteName string `gorm:"column:site_name"`
}

type UpstreamLogRepository struct{ db *gorm.DB }

func NewUpstreamLogRepository(db *gorm.DB) *UpstreamLogRepository {
	return &UpstreamLogRepository{db: db}
}

func (repository *UpstreamLogRepository) CommitWindow(
	ctx context.Context, siteID int64, expectedConfigVersion int, start, end, now int64,
	facts []UpstreamLogFact, status string, errorCode string, errorParams []byte,
) error {
	if repository == nil || repository.db == nil || siteID <= 0 || expectedConfigVersion <= 0 || start <= 0 || end < start || now <= 0 {
		return ErrCollectionRunContract
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, siteID).Error; err != nil {
			return err
		}
		if site.ConfigVersion != expectedConfigVersion {
			return ErrUpstreamLogFence
		}
		uniqueFacts := make([]UpstreamLogFact, 0, len(facts))
		seenKeys := make(map[string]struct{}, len(facts))
		for index := range facts {
			if _, exists := seenKeys[facts[index].UpstreamLogKey]; exists {
				continue
			}
			seenKeys[facts[index].UpstreamLogKey] = struct{}{}
			facts[index].SiteID = siteID
			facts[index].ConfigVersion = expectedConfigVersion
			facts[index].CollectedAt = now
			uniqueFacts = append(uniqueFacts, facts[index])
		}
		if len(uniqueFacts) > 0 {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "config_version"}, {Name: "upstream_log_key"}}, DoNothing: true}).CreateInBatches(&uniqueFacts, 100).Error; err != nil {
				return fmt.Errorf("insert upstream log facts: %w", err)
			}
		}
		state := UpstreamLogCollectionState{SiteID: siteID, ConfigVersion: expectedConfigVersion, Status: status,
			WindowStart: start, WindowEnd: end, LastErrorCode: errorCode, LastErrorParams: errorParams, UpdatedAt: now}
		if status == dto.LogCollectionComplete {
			state.LastSuccessAt = &now
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{
			"config_version", "status", "window_start", "window_end", "last_success_at", "last_error_code", "last_error_params", "updated_at",
		})}).Create(&state).Error
	})
}

func (repository *UpstreamLogRepository) Query(ctx context.Context, query dto.LogQuery) ([]UpstreamLogReadRow, int64, error) {
	if repository == nil || repository.db == nil {
		return nil, 0, errors.New("upstream log repository is required")
	}
	db := repository.db.WithContext(ctx).Table("upstream_log_fact AS l").
		Joins("JOIN site AS s ON s.id = l.site_id").
		Where("l.created_at >= ? AND l.created_at < ?", query.StartTimestamp, query.EndTimestamp)
	if len(query.SiteIDs) > 0 {
		db = db.Where("l.site_id IN ?", query.SiteIDs)
	}
	if query.Type != nil {
		db = db.Where("l.type = ?", *query.Type)
	}
	if query.Username != "" {
		db = db.Where("l.username = ?", query.Username)
	}
	if query.ModelName != "" {
		db = db.Where("l.model_name = ?", query.ModelName)
	}
	if query.TokenName != "" {
		db = db.Where("l.token_name = ?", query.TokenName)
	}
	if query.ChannelID != nil {
		db = db.Where("l.channel_id = ?", *query.ChannelID)
	}
	if query.UseGroup != "" {
		db = db.Where("l.use_group = ?", query.UseGroup)
	}
	if query.RequestID != "" {
		db = db.Where("l.request_id = ?", query.RequestID)
	}
	if query.UpstreamRequestID != "" {
		db = db.Where("l.upstream_request_id = ?", query.UpstreamRequestID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []UpstreamLogReadRow
	err := db.Select("l.*, s.name AS site_name").Order("l.created_at DESC").Order("l.id DESC").Limit(query.PageSize).Offset(query.Offset()).Scan(&rows).Error
	return rows, total, err
}

func (repository *UpstreamLogRepository) LoadStates(ctx context.Context, siteIDs []int64) ([]UpstreamLogCollectionState, error) {
	var states []UpstreamLogCollectionState
	query := repository.db.WithContext(ctx)
	if len(siteIDs) > 0 {
		query = query.Where("site_id IN ?", siteIDs)
	}
	err := query.Find(&states).Error
	return states, err
}

func (repository *UpstreamLogRepository) DeleteBefore(ctx context.Context, cutoff int64, limit int) (int64, error) {
	if repository == nil || repository.db == nil || cutoff <= 0 || limit < 1 || limit > 10000 {
		return 0, ErrCollectionRunContract
	}
	result := repository.db.WithContext(ctx).Exec(`DELETE FROM upstream_log_fact WHERE id IN (
SELECT id FROM (SELECT id FROM upstream_log_fact WHERE created_at < ? ORDER BY id ASC LIMIT ?) AS expired
)`, cutoff, limit)
	return result.RowsAffected, result.Error
}

func (repository *UpstreamLogRepository) LoadRetentionDays(ctx context.Context) (int, error) {
	var setting PlatformSetting
	if err := repository.db.WithContext(ctx).Where("setting_key = ?", "logs.retention_days").First(&setting).Error; err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(setting.Value)
	if err != nil || value < 1 || value > 3650 || setting.ValueType != "int" || setting.Secret {
		return 0, errors.New("invalid log retention setting")
	}
	return value, nil
}
