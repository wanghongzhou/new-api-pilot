package model

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/dto"
)

const (
	SiteChannelInventoryNormal  = "normal"
	SiteChannelInventoryMissing = "missing"
)

type SiteChannelInventory struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	RemoteChannelID  int64  `gorm:"column:remote_channel_id"`
	Name             string `gorm:"column:name"`
	RemoteType       int    `gorm:"column:remote_type"`
	RemoteStatus     int32  `gorm:"column:remote_status"`
	TestTime         int64  `gorm:"column:test_time"`
	ResponseTimeMS   int64  `gorm:"column:response_time_ms"`
	Balance          string `gorm:"column:balance"`
	BalanceUpdatedAt int64  `gorm:"column:balance_updated_at"`
	Models           string `gorm:"column:models"`
	RemoteGroup      string `gorm:"column:remote_group"`
	UsedQuota        int64  `gorm:"column:used_quota"`
	Priority         int64  `gorm:"column:priority"`
	Weight           int64  `gorm:"column:weight"`
	AutoBan          int    `gorm:"column:auto_ban"`
	Tag              string `gorm:"column:tag"`
	RemoteState      string `gorm:"column:remote_state"`
	MissingCount     int    `gorm:"column:missing_count"`
	ConfigVersion    int    `gorm:"column:config_version"`
	FirstSeenAt      int64  `gorm:"column:first_seen_at"`
	LastSeenAt       *int64 `gorm:"column:last_seen_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (SiteChannelInventory) TableName() string { return "site_channel_inventory" }

type SiteChannelInventoryHourly struct {
	ID                int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID            int64  `gorm:"column:site_id"`
	HourTS            int64  `gorm:"column:hour_ts"`
	ChannelCount      int64  `gorm:"column:channel_count"`
	AvailableCount    int64  `gorm:"column:available_count"`
	UnavailableCount  int64  `gorm:"column:unavailable_count"`
	BalanceTotal      string `gorm:"column:balance_total"`
	ResponseTimeAvgMS string `gorm:"column:response_time_avg_ms"`
	ResponseTimeMaxMS int64  `gorm:"column:response_time_max_ms"`
	AvailabilityRate  string `gorm:"column:availability_rate"`
	DataStatus        string `gorm:"column:data_status"`
	ConfigVersion     int    `gorm:"column:config_version"`
	CollectedAt       int64  `gorm:"column:collected_at"`
}

func (SiteChannelInventoryHourly) TableName() string { return "site_channel_inventory_hourly" }

func applySiteChannelInventorySnapshot(ctx context.Context, db *gorm.DB, siteID, syncedAt int64, channels []SiteChannel) error {
	if db == nil || siteID <= 0 || syncedAt <= 0 || len(channels) > 100000 {
		return errors.New("invalid channel inventory snapshot")
	}
	channels = append([]SiteChannel(nil), channels...)
	sort.Slice(channels, func(i, j int) bool { return channels[i].RemoteChannelID < channels[j].RemoteChannelID })
	for index, channel := range channels {
		if channel.RemoteChannelID <= 0 || !utf8.ValidString(channel.Name) || utf8.RuneCountInString(channel.Name) > 255 ||
			!utf8.ValidString(channel.Models) || len(channel.Models) > 65535 || !utf8.ValidString(channel.RemoteGroup) ||
			utf8.RuneCountInString(channel.RemoteGroup) > 128 || !utf8.ValidString(channel.Tag) || utf8.RuneCountInString(channel.Tag) > 255 ||
			channel.TestTime < 0 || channel.ResponseTimeMS < 0 || channel.BalanceUpdatedAt < 0 || channel.UsedQuota < 0 || channel.Priority < 0 || channel.Weight < 0 {
			return errors.New("invalid channel inventory observation")
		}
		if _, ok := new(big.Rat).SetString(channel.Balance); !ok {
			return errors.New("invalid channel inventory balance")
		}
		if index > 0 && channels[index-1].RemoteChannelID == channel.RemoteChannelID {
			return errors.New("duplicate channel inventory observation")
		}
	}
	var site Site
	if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&site, siteID).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Model(&SiteChannelInventory{}).Where("site_id = ?", siteID).
		Updates(map[string]any{"remote_state": SiteChannelInventoryMissing, "missing_count": gorm.Expr("missing_count + 1"), "last_seen_at": nil, "updated_at": syncedAt}).Error; err != nil {
		return err
	}
	balanceTotal := new(big.Rat)
	var responseTotal, responseMax, available int64
	for _, channel := range channels {
		seen := syncedAt
		row := SiteChannelInventory{SiteID: siteID, RemoteChannelID: channel.RemoteChannelID, Name: channel.Name,
			RemoteType: channel.RemoteType, RemoteStatus: channel.RemoteStatus, TestTime: channel.TestTime, ResponseTimeMS: channel.ResponseTimeMS,
			Balance: channel.Balance, BalanceUpdatedAt: channel.BalanceUpdatedAt, Models: channel.Models, RemoteGroup: channel.RemoteGroup,
			UsedQuota: channel.UsedQuota, Priority: channel.Priority, Weight: channel.Weight, AutoBan: channel.AutoBan, Tag: channel.Tag,
			RemoteState: SiteChannelInventoryNormal, ConfigVersion: site.ConfigVersion, FirstSeenAt: syncedAt, LastSeenAt: &seen, CreatedAt: syncedAt, UpdatedAt: syncedAt}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_channel_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "remote_type", "remote_status", "test_time", "response_time_ms", "balance", "balance_updated_at", "models", "remote_group", "used_quota", "priority", "weight", "auto_ban", "tag", "remote_state", "missing_count", "config_version", "last_seen_at", "updated_at"})}).
			Create(&row).Error; err != nil {
			return err
		}
		if err := db.WithContext(ctx).Model(&SiteChannelInventory{}).Where("site_id = ? AND remote_channel_id = ?", siteID, channel.RemoteChannelID).Update("missing_count", 0).Error; err != nil {
			return err
		}
		value, _ := new(big.Rat).SetString(channel.Balance)
		balanceTotal.Add(balanceTotal, value)
		responseTotal += channel.ResponseTimeMS
		if channel.ResponseTimeMS > responseMax {
			responseMax = channel.ResponseTimeMS
		}
		if channel.RemoteStatus == 1 {
			available++
		}
	}
	hour := syncedAt - syncedAt%3600
	if err := db.WithContext(ctx).Where("site_id = ? AND hour_ts = ?", siteID, hour).Delete(&SiteChannelInventoryHourly{}).Error; err != nil {
		return err
	}
	count := int64(len(channels))
	hourly := SiteChannelInventoryHourly{SiteID: siteID, HourTS: hour, ChannelCount: count, AvailableCount: available,
		UnavailableCount: count - available, BalanceTotal: ratDecimal(balanceTotal, 10), ResponseTimeMaxMS: responseMax,
		ResponseTimeAvgMS: ratioDecimal(responseTotal, count, 10), AvailabilityRate: ratioDecimal(available, count, 10),
		DataStatus: "complete", ConfigVersion: site.ConfigVersion, CollectedAt: syncedAt}
	return db.WithContext(ctx).Create(&hourly).Error
}

func ratioDecimal(numerator, denominator int64, scale int) string {
	if denominator == 0 {
		return "0"
	}
	return ratDecimal(new(big.Rat).SetFrac(big.NewInt(numerator), big.NewInt(denominator)), scale)
}

func ratDecimal(value *big.Rat, scale int) string {
	result := value.FloatString(scale)
	for len(result) > 1 && result[len(result)-1] == '0' {
		result = result[:len(result)-1]
	}
	if result[len(result)-1] == '.' {
		result = result[:len(result)-1]
	}
	if result == "-0" {
		return "0"
	}
	if _, err := strconv.ParseFloat(result, 64); err != nil {
		return "0"
	}
	return result
}

type SiteChannelInventoryReadRow struct {
	SiteChannelInventory
	SiteName string `gorm:"column:site_name"`
}
type SiteChannelInventoryMetricRow struct {
	DimensionID       string `gorm:"column:dimension_id"`
	DimensionName     string `gorm:"column:dimension_name"`
	SiteID            int64  `gorm:"column:site_id"`
	SiteName          string `gorm:"column:site_name"`
	BucketStart       int64  `gorm:"column:bucket_start"`
	ChannelCount      int64  `gorm:"column:channel_count"`
	AvailableCount    int64  `gorm:"column:available_count"`
	UnavailableCount  int64  `gorm:"column:unavailable_count"`
	MissingCount      int64  `gorm:"column:missing_count"`
	BalanceTotal      string `gorm:"column:balance_total"`
	UsedQuota         int64  `gorm:"column:used_quota"`
	ResponseTimeAvgMS string `gorm:"column:response_time_avg_ms"`
	ResponseTimeMaxMS int64  `gorm:"column:response_time_max_ms"`
	AvailabilityRate  string `gorm:"column:availability_rate"`
	AsOf              *int64 `gorm:"column:as_of"`
}
type SiteChannelInventoryRepository struct{ db *gorm.DB }

func NewSiteChannelInventoryRepository(db *gorm.DB) *SiteChannelInventoryRepository {
	return &SiteChannelInventoryRepository{db: db}
}
func (r *SiteChannelInventoryRepository) List(ctx context.Context, q dto.ChannelInventoryQuery) ([]SiteChannelInventoryReadRow, int64, error) {
	db := r.db.WithContext(ctx).Table("site_channel_inventory AS c").Joins("JOIN site AS s ON s.id=c.site_id")
	if len(q.SiteIDs) > 0 {
		db = db.Where("c.site_id IN ?", q.SiteIDs)
	}
	if q.Keyword != "" {
		p := "%" + escapeLike(q.Keyword) + "%"
		db = db.Where("(c.name LIKE ? ESCAPE '\\\\' OR c.models LIKE ? ESCAPE '\\\\')", p, p)
	}
	if len(q.Types) > 0 {
		db = db.Where("c.remote_type IN ?", q.Types)
	}
	if len(q.Statuses) > 0 {
		db = db.Where("c.remote_status IN ?", q.Statuses)
	}
	if len(q.Groups) > 0 {
		db = db.Where("c.remote_group IN ?", q.Groups)
	}
	if len(q.Tags) > 0 {
		db = db.Where("c.tag IN ?", q.Tags)
	}
	if len(q.States) > 0 {
		db = db.Where("c.remote_state IN ?", q.States)
	}
	if q.MinBalance != nil {
		db = db.Where("c.balance >= ?", *q.MinBalance)
	}
	if q.MaxBalance != nil {
		db = db.Where("c.balance <= ?", *q.MaxBalance)
	}
	if q.MinResponseTimeMS != nil {
		db = db.Where("c.response_time_ms >= ?", *q.MinResponseTimeMS)
	}
	if q.MaxResponseTimeMS != nil {
		db = db.Where("c.response_time_ms <= ?", *q.MaxResponseTimeMS)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SiteChannelInventoryReadRow
	err := db.Select("c.*,s.name AS site_name").Order("c.site_id,c.remote_channel_id").Limit(q.PageSize).Offset(q.Offset()).Scan(&rows).Error
	return rows, total, err
}
func (r *SiteChannelInventoryRepository) Current(ctx context.Context, q dto.ChannelInventoryStatisticsQuery, dim string) ([]SiteChannelInventoryMetricRow, error) {
	expr := map[string]string{"summary": "'summary'", "type": "CAST(c.remote_type AS CHAR)", "status": "CAST(c.remote_status AS CHAR)", "group": "c.remote_group", "tag": "c.tag", "site": "CAST(c.site_id AS CHAR)"}[dim]
	if expr == "" {
		return nil, errors.New("invalid channel metric dimension")
	}
	db := r.db.WithContext(ctx).Table("site_channel_inventory AS c").Joins("JOIN site AS s ON s.id=c.site_id")
	db = applyChannelMetricFilters(db, q, "c")
	siteID, siteName := "0", "''"
	if dim == "site" {
		siteID, siteName = "c.site_id", "MAX(s.name)"
	}
	sql := expr + " AS dimension_id," + expr + " AS dimension_name," + siteID + " AS site_id," + siteName + ` AS site_name,SUM(c.remote_state='normal') AS channel_count,SUM(c.remote_state='normal' AND c.remote_status=1) AS available_count,SUM(c.remote_state='normal' AND c.remote_status<>1) AS unavailable_count,SUM(c.remote_state='missing') AS missing_count,COALESCE(SUM(CASE WHEN c.remote_state='normal' THEN c.balance ELSE 0 END),0) AS balance_total,COALESCE(SUM(CASE WHEN c.remote_state='normal' THEN c.used_quota ELSE 0 END),0) AS used_quota,COALESCE(AVG(CASE WHEN c.remote_state='normal' THEN c.response_time_ms END),0) AS response_time_avg_ms,COALESCE(MAX(CASE WHEN c.remote_state='normal' THEN c.response_time_ms END),0) AS response_time_max_ms,COALESCE(SUM(c.remote_state='normal' AND c.remote_status=1)/NULLIF(SUM(c.remote_state='normal'),0),0) AS availability_rate,MAX(c.updated_at) AS as_of`
	db = db.Select(sql)
	if dim != "summary" {
		db = db.Group(expr)
		if dim == "site" {
			db = db.Group("c.site_id")
		}
	}
	var rows []SiteChannelInventoryMetricRow
	err := db.Order("dimension_id").Scan(&rows).Error
	return rows, err
}
func (r *SiteChannelInventoryRepository) Trend(ctx context.Context, q dto.ChannelInventoryStatisticsQuery) ([]SiteChannelInventoryMetricRow, error) {
	db := r.db.WithContext(ctx).Table("site_channel_inventory_hourly AS h").Where("h.hour_ts>=? AND h.hour_ts<?", q.StartTimestamp, q.EndTimestamp)
	if len(q.SiteIDs) > 0 {
		db = db.Where("h.site_id IN ?", q.SiteIDs)
	}
	var rows []SiteChannelInventoryMetricRow
	err := db.Select(`h.hour_ts AS bucket_start,SUM(h.channel_count) AS channel_count,SUM(h.available_count) AS available_count,SUM(h.unavailable_count) AS unavailable_count,SUM(h.balance_total) AS balance_total,0 AS used_quota,SUM(h.response_time_avg_ms*h.channel_count)/NULLIF(SUM(h.channel_count),0) AS response_time_avg_ms,MAX(h.response_time_max_ms) AS response_time_max_ms,SUM(h.available_count)/NULLIF(SUM(h.channel_count),0) AS availability_rate,MAX(h.collected_at) AS as_of`).Group("h.hour_ts").Order("h.hour_ts").Scan(&rows).Error
	return rows, err
}
func applyChannelMetricFilters(db *gorm.DB, q dto.ChannelInventoryStatisticsQuery, a string) *gorm.DB {
	if len(q.SiteIDs) > 0 {
		db = db.Where(a+".site_id IN ?", q.SiteIDs)
	}
	if len(q.Types) > 0 {
		db = db.Where(a+".remote_type IN ?", q.Types)
	}
	if len(q.Statuses) > 0 {
		db = db.Where(a+".remote_status IN ?", q.Statuses)
	}
	if len(q.Groups) > 0 {
		db = db.Where(a+".remote_group IN ?", q.Groups)
	}
	if len(q.Tags) > 0 {
		db = db.Where(a+".tag IN ?", q.Tags)
	}
	return db
}
