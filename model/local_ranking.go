package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/dto"
)

type LocalRankingRow struct {
	DimensionID, DimensionName                        string
	SiteID                                            int64
	SiteName                                          string
	TokenUsed, RequestCount, Quota, PreviousTokenUsed int64
	AsOf                                              *int64
}
type LocalRankingHistoryRow struct {
	DimensionID            string
	BucketStart, TokenUsed int64
}
type LocalRankingRepository struct{ db *gorm.DB }

func NewLocalRankingRepository(db *gorm.DB) *LocalRankingRepository {
	return &LocalRankingRepository{db: db}
}
func rankingExpr(kind string) (string, string, error) {
	if kind == "model" {
		return "f.model_name", "f.model_name", nil
	}
	if kind == "vendor" {
		id := "COALESCE(vm.vendor_id,0)"
		return "CAST(" + id + " AS CHAR)", "CASE WHEN " + id + "=0 THEN 'unknown' ELSE CAST(" + id + " AS CHAR) END", nil
	}
	return "", "", errors.New("invalid ranking kind")
}
func applyRankingSites(db *gorm.DB, ids []int64) *gorm.DB {
	if len(ids) > 0 {
		return db.Where("f.site_id IN ?", ids)
	}
	return db
}
func (r *LocalRankingRepository) Rows(ctx context.Context, q dto.LocalRankingQuery, kind string, start, end, previousStart int64, site bool) ([]LocalRankingRow, error) {
	id, name, err := rankingExpr(kind)
	if err != nil {
		return nil, err
	}
	siteID, siteName := "0", "''"
	group := id + "," + name
	if site {
		siteID = "f.site_id"
		siteName = "MAX(s.name)"
		group = id + "," + name + ",f.site_id"
	}
	db := applyRankingSites(r.db.WithContext(ctx).Table("usage_fact_hourly f").Joins("JOIN site s ON s.id=f.site_id"), q.SiteIDs)
	if kind == "vendor" {
		db = db.Joins("LEFT JOIN (SELECT site_id,model_name,MIN(vendor_id) vendor_id FROM site_model_meta WHERE name_rule=0 GROUP BY site_id,model_name) vm ON vm.site_id=f.site_id AND vm.model_name=f.model_name")
	}
	sql := id + " dimension_id," + name + " dimension_name," + siteID + " site_id," + siteName + " site_name,SUM(CASE WHEN f.hour_ts>=? AND f.hour_ts<? THEN f.token_used ELSE 0 END) token_used,SUM(CASE WHEN f.hour_ts>=? AND f.hour_ts<? THEN f.request_count ELSE 0 END) request_count,SUM(CASE WHEN f.hour_ts>=? AND f.hour_ts<? THEN f.quota ELSE 0 END) quota,SUM(CASE WHEN f.hour_ts>=? AND f.hour_ts<? THEN f.token_used ELSE 0 END) previous_token_used,MAX(CASE WHEN f.hour_ts>=? AND f.hour_ts<? THEN f.collected_at END) as_of"
	var rows []LocalRankingRow
	err = db.Where("f.hour_ts>=? AND f.hour_ts<?", previousStart, end).Select(sql, start, end, start, end, start, end, previousStart, start, start, end).Group(group).Having("token_used>0 OR previous_token_used>0").Order("token_used DESC,dimension_id").Scan(&rows).Error
	return rows, err
}
func (r *LocalRankingRepository) History(ctx context.Context, q dto.LocalRankingQuery, kind string, start, end int64) ([]LocalRankingHistoryRow, error) {
	id, _, err := rankingExpr(kind)
	if err != nil {
		return nil, err
	}
	db := applyRankingSites(r.db.WithContext(ctx).Table("usage_fact_hourly f"), q.SiteIDs)
	if kind == "vendor" {
		db = db.Joins("LEFT JOIN (SELECT site_id,model_name,MIN(vendor_id) vendor_id FROM site_model_meta WHERE name_rule=0 GROUP BY site_id,model_name) vm ON vm.site_id=f.site_id AND vm.model_name=f.model_name")
	}
	var rows []LocalRankingHistoryRow
	err = db.Where("f.hour_ts>=? AND f.hour_ts<?", start, end).Select(id + " dimension_id,FLOOR((f.hour_ts+28800)/86400)*86400-28800 bucket_start,SUM(f.token_used) token_used").Group("dimension_id,bucket_start").Order("bucket_start,dimension_id").Scan(&rows).Error
	return rows, err
}

type RankingCompletenessRow struct {
	SiteID                                        int64
	SiteName                                      string
	CompleteCount, UnavailableCount, MissingCount int64
	AsOf                                          *int64
}

func (r *LocalRankingRepository) Completeness(ctx context.Context, q dto.LocalRankingQuery, start, end int64) ([]RankingCompletenessRow, error) {
	db := r.db.WithContext(ctx).Table("site s").Joins("LEFT JOIN collection_window w ON w.site_id=s.id AND w.hour_ts>=? AND w.hour_ts<?", start, end).Where("s.management_status='active'")
	if len(q.SiteIDs) > 0 {
		db = db.Where("s.id IN ?", q.SiteIDs)
	}
	var rows []RankingCompletenessRow
	err := db.Select("s.id site_id,s.name site_name,SUM(w.status='complete') complete_count,SUM(w.status='unavailable') unavailable_count,SUM(w.status='missing') missing_count,MAX(CASE WHEN w.status='complete' THEN w.hour_ts+3600 END) as_of").Group("s.id").Order("s.id").Scan(&rows).Error
	return rows, err
}
