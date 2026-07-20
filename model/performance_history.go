package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"sort"
	"strconv"
	"unicode/utf8"
)

const (
	PerformanceMetricSourceOfficialAverage = "official_average"
	PerformanceMetricSourceCounterReady    = "counter_ready"
	performanceHistoryStatisticsRowLimit   = 100000
)

var ErrPerformanceHistoryResultTooLarge = errors.New("performance history result set exceeds capacity")

type SitePerformanceMetricBucket struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID         int64  `gorm:"column:site_id"`
	ModelName      string `gorm:"column:model_name"`
	RemoteGroup    string `gorm:"column:remote_group"`
	BucketTS       int64  `gorm:"column:bucket_ts"`
	SeriesSchema   string `gorm:"column:series_schema"`
	MetricSource   string `gorm:"column:metric_source"`
	AvgTTFTMS      string `gorm:"column:avg_ttft_ms"`
	AvgLatencyMS   string `gorm:"column:avg_latency_ms"`
	SuccessRate    string `gorm:"column:success_rate"`
	AvgTPS         string `gorm:"column:avg_tps"`
	RequestCount   *int64 `gorm:"column:request_count"`
	SuccessCount   *int64 `gorm:"column:success_count"`
	TotalLatencyMS *int64 `gorm:"column:total_latency_ms"`
	TTFTSumMS      *int64 `gorm:"column:ttft_sum_ms"`
	TTFTCount      *int64 `gorm:"column:ttft_count"`
	OutputTokens   *int64 `gorm:"column:output_tokens"`
	GenerationMS   *int64 `gorm:"column:generation_ms"`
	ConfigVersion  int    `gorm:"column:config_version"`
	CollectedAt    int64  `gorm:"column:collected_at"`
	CreatedAt      int64  `gorm:"column:created_at"`
	UpdatedAt      int64  `gorm:"column:updated_at"`
}

func (SitePerformanceMetricBucket) TableName() string { return "site_performance_metric_bucket" }

type SitePerformanceCollectionState struct {
	SiteID           int64  `gorm:"column:site_id;primaryKey"`
	CapabilityStatus string `gorm:"column:capability_status"`
	LastSuccessAt    *int64 `gorm:"column:last_success_at"`
	LastFailureAt    *int64 `gorm:"column:last_failure_at"`
	LastErrorCode    string `gorm:"column:last_error_code"`
	ConfigVersion    int    `gorm:"column:config_version"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (SitePerformanceCollectionState) TableName() string { return "site_performance_collection_state" }
func (r *SiteRepository) ApplyPerformanceHistorySnapshot(ctx context.Context, expected Site, observedAt, start, end int64, history dto.UpstreamPerformanceHistory) (int64, error) {
	if r == nil || r.db == nil || expected.ID <= 0 || observedAt <= 0 || start <= 0 || end <= start {
		return 0, errors.New("invalid performance snapshot")
	}
	rows := make([]SitePerformanceMetricBucket, 0)
	seen := map[string]struct{}{}
	source := PerformanceMetricSourceOfficialAverage
	if history.CounterReady {
		source = PerformanceMetricSourceCounterReady
	}
	for _, m := range history.Models {
		if !utf8.ValidString(m.ModelName) || m.ModelName == "" || len(m.ModelName) > 255 || !utf8.ValidString(m.SeriesSchema) || m.SeriesSchema == "" || len(m.SeriesSchema) > 64 {
			return 0, errors.New("invalid performance model")
		}
		for _, g := range m.Groups {
			if !utf8.ValidString(g.Group) || len(g.Group) > 128 {
				return 0, errors.New("invalid performance group")
			}
			for _, b := range g.Series {
				if b.Timestamp < start || b.Timestamp >= end {
					return 0, errors.New("performance bucket outside window")
				}
				key := m.ModelName + "\x00" + g.Group + "\x00" + strconv.FormatInt(b.Timestamp, 10)
				if _, ok := seen[key]; ok {
					return 0, errors.New("duplicate performance bucket")
				}
				seen[key] = struct{}{}
				row := SitePerformanceMetricBucket{SiteID: expected.ID, ModelName: m.ModelName, RemoteGroup: g.Group, BucketTS: b.Timestamp, SeriesSchema: m.SeriesSchema, MetricSource: source, AvgTTFTMS: b.AvgTTFTMS, AvgLatencyMS: b.AvgLatencyMS, SuccessRate: b.SuccessRate, AvgTPS: b.AvgTPS, ConfigVersion: expected.ConfigVersion, CollectedAt: observedAt, CreatedAt: observedAt, UpdatedAt: observedAt}
				if source == PerformanceMetricSourceCounterReady {
					row.RequestCount = b.Counters.RequestCount
					row.SuccessCount = b.Counters.SuccessCount
					row.TotalLatencyMS = b.Counters.TotalLatencyMS
					row.TTFTSumMS = b.Counters.TTFTSumMS
					row.TTFTCount = b.Counters.TTFTCount
					row.OutputTokens = b.Counters.OutputTokens
					row.GenerationMS = b.Counters.GenerationMS
				}
				rows = append(rows, row)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ModelName != rows[j].ModelName {
			return rows[i].ModelName < rows[j].ModelName
		}
		if rows[i].RemoteGroup != rows[j].RemoteGroup {
			return rows[i].RemoteGroup < rows[j].RemoteGroup
		}
		return rows[i].BucketTS < rows[j].BucketTS
	})
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, expected.ID).Error; err != nil {
			return err
		}
		if site.ConfigVersion != expected.ConfigVersion || site.BaseURL != expected.BaseURL || site.AuthStatus != constant.SiteAuthAuthorized || site.ManagementStatus != constant.SiteManagementActive {
			return ErrSiteRunConfigChanged
		}
		if err := tx.Where("site_id=? AND bucket_ts>=? AND bucket_ts<?", site.ID, start, end).Delete(&SitePerformanceMetricBucket{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}
		now := observedAt
		status := "average_only"
		if history.CounterReady {
			status = "available"
		}
		state := SitePerformanceCollectionState{SiteID: site.ID, CapabilityStatus: status, LastSuccessAt: &now, LastErrorCode: "", ConfigVersion: site.ConfigVersion, UpdatedAt: now}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"capability_status", "last_success_at", "last_error_code", "config_version", "updated_at"})}).Create(&state).Error
	})
	return int64(len(rows)), err
}
func (r *SiteRepository) MarkPerformanceUnavailable(ctx context.Context, expected Site, observedAt int64, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site Site
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, expected.ID).Error; err != nil {
			return err
		}
		if site.ConfigVersion != expected.ConfigVersion {
			return ErrSiteRunConfigChanged
		}
		now := observedAt
		state := SitePerformanceCollectionState{SiteID: site.ID, CapabilityStatus: "unavailable", LastFailureAt: &now, LastErrorCode: code, ConfigVersion: site.ConfigVersion, UpdatedAt: now}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}}, DoUpdates: clause.AssignmentColumns([]string{"capability_status", "last_failure_at", "last_error_code", "config_version", "updated_at"})}).Create(&state).Error
	})
}

type PerformanceHistoryReadRow struct {
	SitePerformanceMetricBucket
	SiteName string `gorm:"column:site_name"`
}
type PerformanceHistoryRepository struct{ db *gorm.DB }

func NewPerformanceHistoryRepository(db *gorm.DB) *PerformanceHistoryRepository {
	return &PerformanceHistoryRepository{db: db}
}
func (r *PerformanceHistoryRepository) List(ctx context.Context, q dto.PerformanceHistoryQuery) ([]PerformanceHistoryReadRow, int64, error) {
	db := r.db.WithContext(ctx).Table("site_performance_metric_bucket AS p").Joins("JOIN site AS s ON s.id=p.site_id").Where("p.bucket_ts>=? AND p.bucket_ts<?", q.StartTimestamp, q.EndTimestamp)
	if len(q.SiteIDs) > 0 {
		db = db.Where("p.site_id IN ?", q.SiteIDs)
	}
	if len(q.ModelNames) > 0 {
		db = db.Where("p.model_name IN ?", q.ModelNames)
	}
	if len(q.Groups) > 0 {
		db = db.Where("p.remote_group IN ?", q.Groups)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []PerformanceHistoryReadRow
	err := db.Select("p.*,s.name AS site_name").Order("p.bucket_ts,p.site_id,p.model_name,p.remote_group").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *PerformanceHistoryRepository) All(ctx context.Context, q dto.PerformanceHistoryQuery) ([]PerformanceHistoryReadRow, error) {
	q.Page = 1
	q.PageSize = 100
	db := r.db.WithContext(ctx).Table("site_performance_metric_bucket AS p").Joins("JOIN site AS s ON s.id=p.site_id").Where("p.bucket_ts>=? AND p.bucket_ts<?", q.StartTimestamp, q.EndTimestamp)
	if len(q.SiteIDs) > 0 {
		db = db.Where("p.site_id IN ?", q.SiteIDs)
	}
	if len(q.ModelNames) > 0 {
		db = db.Where("p.model_name IN ?", q.ModelNames)
	}
	if len(q.Groups) > 0 {
		db = db.Where("p.remote_group IN ?", q.Groups)
	}
	var rows []PerformanceHistoryReadRow
	err := db.Select("p.*,s.name AS site_name").Order("p.bucket_ts,p.site_id,p.model_name,p.remote_group").Limit(performanceHistoryStatisticsRowLimit + 1).Scan(&rows).Error
	if err == nil && len(rows) > performanceHistoryStatisticsRowLimit {
		return nil, ErrPerformanceHistoryResultTooLarge
	}
	return rows, err
}
func WeightedPerformance(rows []PerformanceHistoryReadRow) (success, latency, ttft, tps, requests string, ok bool) {
	if len(rows) == 0 {
		return "0", "0", "0", "0", "0", false
	}
	req, succ, lat := new(big.Int), new(big.Int), new(big.Int)
	ttftSum, ttftCount, out, gen := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	for _, r := range rows {
		if r.MetricSource != PerformanceMetricSourceCounterReady || r.RequestCount == nil || r.SuccessCount == nil || r.TotalLatencyMS == nil || r.TTFTSumMS == nil || r.TTFTCount == nil || r.OutputTokens == nil || r.GenerationMS == nil {
			return "0", "0", "0", "0", "0", false
		}
		req.Add(req, big.NewInt(*r.RequestCount))
		succ.Add(succ, big.NewInt(*r.SuccessCount))
		lat.Add(lat, big.NewInt(*r.TotalLatencyMS))
		ttftSum.Add(ttftSum, big.NewInt(*r.TTFTSumMS))
		ttftCount.Add(ttftCount, big.NewInt(*r.TTFTCount))
		out.Add(out, big.NewInt(*r.OutputTokens))
		gen.Add(gen, big.NewInt(*r.GenerationMS))
	}
	tpsNumerator := new(big.Int).Mul(out, big.NewInt(1000))
	return rationalBig(succ, req), rationalBig(lat, req), rationalBig(ttftSum, ttftCount), rationalBig(tpsNumerator, gen), req.String(), true
}
func rationalBig(n, d *big.Int) string {
	if d.Sign() <= 0 {
		return "0"
	}
	return new(big.Rat).SetFrac(n, d).FloatString(10)
}
func (r *PerformanceHistoryRepository) DeleteBefore(ctx context.Context, cutoff int64) error {
	return r.db.WithContext(ctx).Where("bucket_ts<?", cutoff).Delete(&SitePerformanceMetricBucket{}).Error
}
func (r *SiteRepository) DeletePerformanceBefore(ctx context.Context, cutoff int64) error {
	return r.db.WithContext(ctx).Where("bucket_ts<?", cutoff).Delete(&SitePerformanceMetricBucket{}).Error
}
