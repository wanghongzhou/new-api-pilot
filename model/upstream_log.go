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
	ID                  int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID              int64  `gorm:"column:site_id"`
	ConfigVersion       int    `gorm:"column:config_version"`
	UpstreamLogKey      string `gorm:"column:upstream_log_key"`
	UpstreamLogID       int64  `gorm:"column:upstream_log_id"`
	CreatedAt           int64  `gorm:"column:created_at"`
	Type                int    `gorm:"column:type"`
	RemoteUserID        int64  `gorm:"column:remote_user_id"`
	Username            string `gorm:"column:username"`
	ModelName           string `gorm:"column:model_name"`
	TokenID             int64  `gorm:"column:token_id"`
	TokenName           string `gorm:"column:token_name"`
	ChannelID           int64  `gorm:"column:channel_id"`
	UseGroup            string `gorm:"column:use_group"`
	RequestID           string `gorm:"column:request_id"`
	UpstreamRequestID   string `gorm:"column:upstream_request_id"`
	Quota               int64  `gorm:"column:quota"`
	PromptTokens        int64  `gorm:"column:prompt_tokens"`
	CompletionTokens    int64  `gorm:"column:completion_tokens"`
	CacheReadTokens     int64  `gorm:"column:cache_read_tokens"`
	CacheCreationTokens int64  `gorm:"column:cache_creation_tokens"`
	CacheCreation5m     int64  `gorm:"column:cache_creation_tokens_5m"`
	CacheCreation1h     int64  `gorm:"column:cache_creation_tokens_1h"`
	UseTimeSeconds      int64  `gorm:"column:use_time_seconds"`
	IsStream            bool   `gorm:"column:is_stream"`
	FirstResponseTimeMs *int64 `gorm:"column:first_response_time_ms"`
	StreamStatus        string `gorm:"column:stream_status"`
	StreamEndReason     string `gorm:"column:stream_end_reason"`
	StreamErrorCount    int64  `gorm:"column:stream_error_count"`
	ContentRedacted     string `gorm:"column:content_redacted"`
	IP                  string `gorm:"column:ip"`
	CollectedAt         int64  `gorm:"column:collected_at"`
}

func (UpstreamLogFact) TableName() string { return "upstream_log_fact" }

type UpstreamLogCollectionState struct {
	SiteID              int64  `gorm:"column:site_id;primaryKey"`
	ConfigVersion       int    `gorm:"column:config_version"`
	Status              string `gorm:"column:status"`
	WindowStart         int64  `gorm:"column:window_start"`
	WindowEnd           int64  `gorm:"column:window_end"`
	HistoryStartAt      *int64 `gorm:"column:history_start_at"`
	LastSuccessAt       *int64 `gorm:"column:last_success_at"`
	BackfillCompletedAt *int64 `gorm:"column:backfill_completed_at"`
	LastErrorCode       string `gorm:"column:last_error_code"`
	LastErrorParams     []byte `gorm:"column:last_error_params"`
	UpdatedAt           int64  `gorm:"column:updated_at"`
}

func (UpstreamLogCollectionState) TableName() string { return "upstream_log_collection_state" }

type UpstreamLogReadRow struct {
	UpstreamLogFact
	SiteName        string  `gorm:"column:site_name"`
	QuotaPerUnit    *string `gorm:"column:quota_per_unit"`
	USDExchangeRate *string `gorm:"column:usd_exchange_rate"`
	LastRateAt      *int64  `gorm:"column:last_rate_at"`
}

type UpstreamLogStatReadRow struct {
	SiteID          int64   `gorm:"column:site_id"`
	SiteName        string  `gorm:"column:site_name"`
	Quota           int64   `gorm:"column:quota"`
	QuotaPerUnit    *string `gorm:"column:quota_per_unit"`
	USDExchangeRate *string `gorm:"column:usd_exchange_rate"`
	LastRateAt      *int64  `gorm:"column:last_rate_at"`
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
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "site_id"}, {Name: "config_version"}, {Name: "upstream_log_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"first_response_time_ms", "stream_status", "stream_end_reason", "stream_error_count", "cache_read_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h"}),
			}).CreateInBatches(&uniqueFacts, 100).Error; err != nil {
				return fmt.Errorf("insert upstream log facts: %w", err)
			}
		}
		state := UpstreamLogCollectionState{SiteID: siteID, ConfigVersion: expectedConfigVersion, Status: status,
			WindowStart: start, WindowEnd: end, LastErrorCode: errorCode, LastErrorParams: errorParams, UpdatedAt: now}
		var existing UpstreamLogCollectionState
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", siteID).Take(&existing).Error
		if findErr == nil {
			state.HistoryStartAt = existing.HistoryStartAt
			state.BackfillCompletedAt = existing.BackfillCompletedAt
			state.LastSuccessAt = existing.LastSuccessAt
			if status != dto.LogCollectionComplete {
				state.WindowStart = existing.WindowStart
				state.WindowEnd = existing.WindowEnd
			} else if end < existing.WindowEnd {
				state.Status = existing.Status
				state.WindowStart = existing.WindowStart
				state.WindowEnd = existing.WindowEnd
				state.LastErrorCode = existing.LastErrorCode
				state.LastErrorParams = existing.LastErrorParams
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if status == dto.LogCollectionComplete {
			if state.HistoryStartAt == nil || start < *state.HistoryStartAt {
				value := start
				state.HistoryStartAt = &value
			}
			if findErr != nil || end >= existing.WindowEnd {
				state.LastSuccessAt = &now
			}
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{
			"config_version", "status", "window_start", "window_end", "history_start_at", "last_success_at", "backfill_completed_at", "last_error_code", "last_error_params", "updated_at",
		})}).Create(&state).Error
	})
}

func (repository *UpstreamLogRepository) LoadState(ctx context.Context, siteID int64) (UpstreamLogCollectionState, error) {
	if repository == nil || repository.db == nil || siteID <= 0 {
		return UpstreamLogCollectionState{}, ErrCollectionRunContract
	}
	var state UpstreamLogCollectionState
	err := repository.db.WithContext(ctx).Where("site_id = ?", siteID).Take(&state).Error
	return state, err
}

func (repository *UpstreamLogRepository) MarkBackfillCompleted(ctx context.Context, siteID int64, expectedConfigVersion int, targetStart, now int64) error {
	if repository == nil || repository.db == nil || siteID <= 0 || expectedConfigVersion <= 0 || targetStart <= 0 || now <= 0 {
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
		var state UpstreamLogCollectionState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", siteID).Take(&state).Error; err != nil {
			return err
		}
		if state.HistoryStartAt == nil || *state.HistoryStartAt > targetStart {
			return ErrCollectionRunContract
		}
		state.BackfillCompletedAt = &now
		state.UpdatedAt = now
		return tx.Save(&state).Error
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
	err := db.Select("l.*, s.name AS site_name, CAST(s.quota_per_unit AS CHAR) AS quota_per_unit, CAST(s.usd_exchange_rate AS CHAR) AS usd_exchange_rate, s.last_rate_at").Order("l.created_at DESC").Order("l.id DESC").Limit(query.PageSize).Offset(query.Offset()).Scan(&rows).Error
	return rows, total, err
}

func (repository *UpstreamLogRepository) Stats(ctx context.Context, query dto.LogQuery, now int64) ([]UpstreamLogStatReadRow, int64, int64, error) {
	if repository == nil || repository.db == nil || now <= 0 {
		return nil, 0, 0, ErrCollectionRunContract
	}
	if query.Type != nil && *query.Type != 2 {
		return []UpstreamLogStatReadRow{}, 0, 0, nil
	}
	usage := applyUpstreamLogFilters(repository.db.WithContext(ctx).Table("upstream_log_fact AS l").Joins("JOIN site AS s ON s.id = l.site_id"), query, true).
		Where("l.type = ?", 2)
	var breakdown []UpstreamLogStatReadRow
	if err := usage.Select("l.site_id, s.name AS site_name, COALESCE(SUM(l.quota), 0) AS quota, CAST(s.quota_per_unit AS CHAR) AS quota_per_unit, CAST(s.usd_exchange_rate AS CHAR) AS usd_exchange_rate, s.last_rate_at").
		Group("l.site_id, s.name, s.quota_per_unit, s.usd_exchange_rate, s.last_rate_at").Order("l.site_id ASC").Scan(&breakdown).Error; err != nil {
		return nil, 0, 0, err
	}
	realtime := applyUpstreamLogFilters(repository.db.WithContext(ctx).Table("upstream_log_fact AS l"), query, false).
		Where("l.type = ? AND l.created_at >= ? AND l.created_at < ?", 2, now-60, now+1)
	var totals struct {
		RPM int64 `gorm:"column:rpm"`
		TPM int64 `gorm:"column:tpm"`
	}
	if err := realtime.Select("COUNT(*) AS rpm, COALESCE(SUM(l.prompt_tokens + l.completion_tokens), 0) AS tpm").Scan(&totals).Error; err != nil {
		return nil, 0, 0, err
	}
	return breakdown, totals.RPM, totals.TPM, nil
}

func applyUpstreamLogFilters(db *gorm.DB, query dto.LogQuery, includeRange bool) *gorm.DB {
	if includeRange {
		db = db.Where("l.created_at >= ? AND l.created_at < ?", query.StartTimestamp, query.EndTimestamp)
	}
	if len(query.SiteIDs) > 0 {
		db = db.Where("l.site_id IN ?", query.SiteIDs)
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
	return db
}

func (repository *UpstreamLogRepository) LoadFallbackRate(ctx context.Context) (string, string, error) {
	var settings []PlatformSetting
	if err := repository.db.WithContext(ctx).Where("setting_key IN ?", []string{"rate.fallback_quota_per_unit", "rate.fallback_usd_exchange_rate"}).Find(&settings).Error; err != nil {
		return "", "", err
	}
	var quotaPerUnit, exchangeRate string
	for _, setting := range settings {
		switch setting.Key {
		case "rate.fallback_quota_per_unit":
			quotaPerUnit = setting.Value
		case "rate.fallback_usd_exchange_rate":
			exchangeRate = setting.Value
		}
	}
	return quotaPerUnit, exchangeRate, nil
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
