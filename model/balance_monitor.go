package model

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"new-api-pilot/dto"
	"strconv"
)

type BalanceMonitorRecord struct {
	ID              int64 `gorm:"primaryKey"`
	SourceID        string
	RecordID        string
	Revision        int64
	Kind            string
	AccountID       string
	SiteID          string
	Day             string
	SampledAt       int64
	ReceivedAt      int64
	ConsumptionYuan *string
	Payload         json.RawMessage `gorm:"type:json"`
}

func (BalanceMonitorRecord) TableName() string { return "balance_monitor_record" }

type BalanceMonitorRepository struct{ db *gorm.DB }

func NewBalanceMonitorRepository(db *gorm.DB) *BalanceMonitorRepository {
	return &BalanceMonitorRepository{db: db}
}
func (r *BalanceMonitorRepository) Ingest(ctx context.Context, rows []dto.BalanceRecord, received int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, v := range rows {
			revision, _ := strconv.ParseInt(v.Revision, 10, 64)
			sampled, _ := strconv.ParseInt(v.SampledAt, 10, 64)
			payload, e := json.Marshal(v)
			if e != nil {
				return e
			}
			row := BalanceMonitorRecord{SourceID: v.SourceID, RecordID: v.RecordID, Revision: revision, Kind: v.Kind, AccountID: v.AccountID, SiteID: v.SiteID, Day: v.Day, SampledAt: sampled, ReceivedAt: received, ConsumptionYuan: v.ConsumptionYuan, Payload: payload}
			// Assign revision last: MySQL evaluates duplicate-key assignments left-to-right.
			assignments := []clause.Assignment{}
			for _, column := range []string{"kind", "account_id", "site_id", "day", "sampled_at", "received_at", "consumption_yuan", "payload"} {
				assignments = append(assignments, clause.Assignment{Column: clause.Column{Name: column}, Value: clause.Expr{SQL: "IF(VALUES(revision) > revision, VALUES(" + column + "), " + column + ")"}})
			}
			assignments = append(assignments, clause.Assignment{Column: clause.Column{Name: "revision"}, Value: clause.Expr{SQL: "GREATEST(revision, VALUES(revision))"}})
			if e := tx.Clauses(clause.OnConflict{DoUpdates: clause.Set(assignments)}).Create(&row).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
func (r *BalanceMonitorRepository) List(ctx context.Context, q dto.BalanceQuery) (dto.BalancePage, error) {
	result := dto.BalancePage{Items: []json.RawMessage{}, Total: "0"}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&BalanceMonitorRecord{}).Where("kind = ?", q.Kind)
		if q.Kind == "daily" || q.Kind == "interval" || q.Kind == "monthly" {
			query = query.Where("day >= ? AND day <= ?", q.Start, q.End)
		}
		if q.AccountID != "" {
			query = query.Where("account_id = ?", q.AccountID)
		}
		if q.SiteID != "" {
			query = query.Where("site_id = ?", q.SiteID)
		}
		var count int64
		if e := query.Count(&count).Error; e != nil {
			return e
		}
		result.Total = strconv.FormatInt(count, 10)
		var rows []BalanceMonitorRecord
		if e := query.Order("day DESC, sampled_at DESC, id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			var item map[string]any
			if e := json.Unmarshal(row.Payload, &item); e != nil {
				return e
			}
			item["received_at"] = strconv.FormatInt(row.ReceivedAt, 10)
			payload, e := json.Marshal(item)
			if e != nil {
				return e
			}
			result.Items = append(result.Items, payload)
		}
		return nil
	})
	return result, err
}
