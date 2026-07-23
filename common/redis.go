package common

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type FastTaskHistoryRecord struct {
	SiteID     int64  `json:"site_id"`
	TaskType   string `json:"task_type"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	RequestID  string `json:"request_id"`
}

type FastTaskHistorySettingsProvider interface {
	FastTaskHistorySettings() (time.Duration, int)
}

type RedisStore struct {
	Client    *redis.Client
	Retention time.Duration
	Count     int
	settings  FastTaskHistorySettingsProvider
}

func NewRedisStore(dsn string, db int, timeout, retention time.Duration, count int) (*RedisStore, error) {
	if dsn == "" {
		dsn = "redis://localhost:6379/0"
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_DSN: %w", err)
	}
	opt.DB = db
	opt.DialTimeout = timeout
	opt.ReadTimeout = timeout
	opt.WriteTimeout = timeout
	if count <= 0 {
		count = 100
	}
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	return &RedisStore{Client: redis.NewClient(opt), Retention: retention, Count: count}, nil
}
func (s *RedisStore) Ping(ctx context.Context) error {
	if s == nil || s.Client == nil {
		return fmt.Errorf("redis unavailable")
	}
	return s.Client.Ping(ctx).Err()
}
func (s *RedisStore) Close() error {
	if s == nil || s.Client == nil {
		return nil
	}
	return s.Client.Close()
}
func (s *RedisStore) SetFastTaskHistorySettingsProvider(provider FastTaskHistorySettingsProvider) {
	if s != nil {
		s.settings = provider
	}
}
func (s *RedisStore) Add(ctx context.Context, rec FastTaskHistoryRecord) error {
	if s == nil || s.Client == nil {
		return fmt.Errorf("redis unavailable")
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	retention, count := s.Retention, s.Count
	if s.settings != nil {
		retention, count = s.settings.FastTaskHistorySettings()
	}
	if retention <= 0 || count <= 0 {
		return fmt.Errorf("fast task history settings are invalid")
	}
	key := fmt.Sprintf("new-api-pilot:fast-task:%d:%s", rec.SiteID, rec.TaskType)
	pipe := s.Client.TxPipeline()
	pipe.LPush(ctx, key, b)
	pipe.LTrim(ctx, key, 0, int64(count-1))
	pipe.Expire(ctx, key, retention)
	_, err = pipe.Exec(ctx)
	return err
}
func (s *RedisStore) List(ctx context.Context, siteID int64, taskType string, offset, limit int) ([]FastTaskHistoryRecord, error) {
	rows, _, _, err := s.ListFiltered(ctx, siteID, taskType, "", offset, limit)
	return rows, err
}

func (s *RedisStore) ListFiltered(ctx context.Context, siteID int64, taskType, status string, offset, limit int) ([]FastTaskHistoryRecord, int, bool, error) {
	if s == nil || s.Client == nil {
		return nil, 0, false, fmt.Errorf("redis unavailable")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	key := fmt.Sprintf("new-api-pilot:fast-task:%d:%s", siteID, taskType)
	vals, err := s.Client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, 0, false, err
	}
	all := make([]FastTaskHistoryRecord, 0, len(vals))
	for _, v := range vals {
		var r FastTaskHistoryRecord
		if json.Unmarshal([]byte(v), &r) == nil && (status == "" || r.Status == status) {
			all = append(all, r)
		}
	}
	total := len(all)
	if offset >= total {
		return []FastTaskHistoryRecord{}, total, false, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, end < total, nil
}
