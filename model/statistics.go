package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var ErrStatisticsReadContract = errors.New("invalid statistics read contract")

type StatisticsChannelKey struct {
	SiteID    int64
	ChannelID int64
}

type StatisticsReadRequest struct {
	Scope          string
	Granularity    string
	StartTimestamp int64
	EndTimestamp   int64
	StartDateKey   int
	EndDateKey     int
	SiteIDs        []int64
	CustomerIDs    []int64
	AccountIDs     []int64
	ModelNames     []string
	ChannelKeys    []StatisticsChannelKey
	UseGroups      []string
	TokenKeys      []StatisticsChannelKey
	NodeNames      []string
}

type StatisticsSite struct {
	ID                int64   `gorm:"column:id"`
	Name              string  `gorm:"column:name"`
	ManagementStatus  string  `gorm:"column:management_status"`
	OnlineStatus      string  `gorm:"column:online_status"`
	AuthStatus        string  `gorm:"column:auth_status"`
	StatisticsStatus  string  `gorm:"column:statistics_status"`
	HealthStatus      string  `gorm:"column:health_status"`
	QuotaPerUnit      *string `gorm:"column:quota_per_unit"`
	USDExchangeRate   *string `gorm:"column:usd_exchange_rate"`
	LastRateAt        *int64  `gorm:"column:last_rate_at"`
	StatisticsStartAt *int64  `gorm:"column:statistics_start_at"`
	StatisticsEndAt   *int64  `gorm:"column:statistics_end_at"`
	DisabledAt        *int64  `gorm:"column:disabled_at"`
}

type StatisticsCustomer struct {
	ID                 int64  `gorm:"column:id"`
	Name               string `gorm:"column:name"`
	Status             string `gorm:"column:status"`
	StatisticsPausedAt *int64 `gorm:"column:statistics_paused_at"`
}

type StatisticsAccount struct {
	ID                 int64  `gorm:"column:id"`
	SiteID             int64  `gorm:"column:site_id"`
	CustomerID         int64  `gorm:"column:customer_id"`
	RemoteUserID       int64  `gorm:"column:remote_user_id"`
	RemoteCreatedAt    int64  `gorm:"column:remote_created_at"`
	Username           string `gorm:"column:username"`
	DisplayName        string `gorm:"column:display_name"`
	StatisticsPausedAt *int64 `gorm:"column:statistics_paused_at"`
}

type StatisticsChannel struct {
	SiteID          int64  `gorm:"column:site_id"`
	RemoteChannelID int64  `gorm:"column:remote_channel_id"`
	Name            string `gorm:"column:name"`
	RemoteMissing   bool   `gorm:"column:remote_missing"`
}

type StatisticsModelOption struct {
	SiteID    int64  `gorm:"column:site_id"`
	SiteName  string `gorm:"column:site_name"`
	ModelName string `gorm:"column:model_name"`
}

type StatisticsChannelOption struct {
	SiteID          int64  `gorm:"column:site_id"`
	SiteName        string `gorm:"column:site_name"`
	RemoteChannelID int64  `gorm:"column:remote_channel_id"`
	Name            string `gorm:"column:name"`
	RemoteMissing   bool   `gorm:"column:remote_missing"`
}

type StatisticsGroupOption struct {
	SiteID   int64  `gorm:"column:site_id"`
	SiteName string `gorm:"column:site_name"`
	UseGroup string `gorm:"column:use_group"`
}

type StatisticsTokenOption struct {
	SiteID    int64  `gorm:"column:site_id"`
	SiteName  string `gorm:"column:site_name"`
	TokenID   int64  `gorm:"column:token_id"`
	TokenName string `gorm:"column:token_name"`
}

type StatisticsNodeOption struct {
	SiteID   int64  `gorm:"column:site_id"`
	SiteName string `gorm:"column:site_name"`
	NodeName string `gorm:"column:node_name"`
}

type StatisticsWindow struct {
	RowKind         string `gorm:"column:row_kind"`
	SiteID          int64  `gorm:"column:site_id"`
	HourTS          int64  `gorm:"column:hour_ts"`
	Status          string `gorm:"column:status"`
	VerifiedAt      *int64 `gorm:"column:verified_at"`
	LastErrorCode   string `gorm:"column:last_error_code"`
	LastErrorParams []byte `gorm:"column:last_error_params"`
	ActiveTaskType  string `gorm:"column:active_task_type"`
}

type StatisticsMetricRow struct {
	DimensionID      string `gorm:"column:dimension_id"`
	DimensionName    string `gorm:"column:dimension_name"`
	SiteID           int64  `gorm:"column:site_id"`
	BucketKey        int64  `gorm:"column:bucket_key"`
	RequestCount     string `gorm:"column:request_count"`
	Quota            string `gorm:"column:quota"`
	TokenUsed        string `gorm:"column:token_used"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
}

type StatisticsActiveRow struct {
	RowKind     string `gorm:"column:row_kind"`
	DimensionID string `gorm:"column:dimension_id"`
	SiteID      int64  `gorm:"column:site_id"`
	BucketKey   int64  `gorm:"column:bucket_key"`
	ActiveUsers string `gorm:"column:active_users"`
}

type StatisticsFallbackRates struct {
	QuotaPerUnit      string
	USDExchangeRate   string
	UsageDelayMinutes int
}

type StatisticsRepository struct {
	db *gorm.DB
}

func NewStatisticsRepository(db *gorm.DB) *StatisticsRepository { return &StatisticsRepository{db: db} }

func (repository *StatisticsRepository) LoadSites(ctx context.Context, ids []int64) ([]StatisticsSite, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrStatisticsReadContract
	}
	query := repository.db.WithContext(ctx).Table("site").Select(`id, name, management_status, online_status,
		auth_status, statistics_status, health_status, CAST(quota_per_unit AS CHAR) AS quota_per_unit,
		CAST(usd_exchange_rate AS CHAR) AS usd_exchange_rate, last_rate_at, statistics_start_at,
		statistics_end_at, disabled_at`)
	if len(ids) > 0 {
		query = query.Where("id IN ?", ids)
	}
	var rows []StatisticsSite
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics sites: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadCustomers(ctx context.Context, ids []int64) ([]StatisticsCustomer, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrStatisticsReadContract
	}
	query := repository.db.WithContext(ctx).Table("customer").Select("id, name, status, statistics_paused_at")
	if len(ids) > 0 {
		query = query.Where("id IN ?", ids)
	}
	var rows []StatisticsCustomer
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics customers: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadAccounts(ctx context.Context, request StatisticsReadRequest) ([]StatisticsAccount, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrStatisticsReadContract
	}
	query := repository.db.WithContext(ctx).Table("account").
		Select("id, site_id, customer_id, remote_user_id, remote_created_at, username, display_name, statistics_paused_at")
	if len(request.SiteIDs) > 0 {
		query = query.Where("site_id IN ?", request.SiteIDs)
	}
	if len(request.CustomerIDs) > 0 {
		query = query.Where("customer_id IN ?", request.CustomerIDs)
	}
	if len(request.AccountIDs) > 0 {
		query = query.Where("id IN ?", request.AccountIDs)
	}
	var rows []StatisticsAccount
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics accounts: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadChannels(ctx context.Context, request StatisticsReadRequest) ([]StatisticsChannel, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrStatisticsReadContract
	}
	query := repository.db.WithContext(ctx).Table("site_channel").
		Select("site_id, remote_channel_id, name, remote_missing")
	if len(request.SiteIDs) > 0 {
		query = query.Where("site_id IN ?", request.SiteIDs)
	}
	query = applyStatisticsChannelFilter(query, "site_id", "remote_channel_id", request.ChannelKeys)
	var rows []StatisticsChannel
	if err := query.Order("site_id ASC, remote_channel_id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics channels: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadModelOptions(
	ctx context.Context,
	keyword string,
	siteIDs []int64,
	limit, offset int,
) ([]StatisticsModelOption, int64, error) {
	if repository == nil || repository.db == nil || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, ErrStatisticsReadContract
	}
	base := `FROM (
  SELECT st.site_id, s.name AS site_name, st.model_name
  FROM model_stat_hourly AS st
  JOIN site AS s ON s.id = st.site_id
  GROUP BY st.site_id, s.name, st.model_name
) AS options
WHERE 1 = 1`
	where, args := statisticsOptionsWhere(siteIDs, keyword, "options.model_name")
	base += where
	var total int64
	if err := repository.db.WithContext(ctx).Raw("SELECT COUNT(*) "+base, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count statistics model options: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), limit, offset)
	var rows []StatisticsModelOption
	if err := repository.db.WithContext(ctx).Raw(`SELECT site_id, site_name, model_name `+base+`
ORDER BY site_name ASC, site_id ASC, model_name ASC
LIMIT ? OFFSET ?`, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("load statistics model options: %w", err)
	}
	return rows, total, nil
}

func (repository *StatisticsRepository) LoadChannelOptions(
	ctx context.Context,
	keyword string,
	siteIDs []int64,
	limit, offset int,
) ([]StatisticsChannelOption, int64, error) {
	if repository == nil || repository.db == nil || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, ErrStatisticsReadContract
	}
	base := `FROM (
  SELECT s.id AS site_id, s.name AS site_name, 0 AS remote_channel_id,
    '未知通道' AS name, 0 AS remote_missing
  FROM site AS s
  UNION ALL
  SELECT sc.site_id, s.name AS site_name, sc.remote_channel_id, sc.name, sc.remote_missing
  FROM site_channel AS sc
  JOIN site AS s ON s.id = sc.site_id
  WHERE sc.remote_channel_id <> 0
) AS options
WHERE 1 = 1`
	where, args := statisticsOptionsWhere(siteIDs, keyword, "options.name")
	base += where
	var total int64
	if err := repository.db.WithContext(ctx).Raw("SELECT COUNT(*) "+base, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count statistics channel options: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), limit, offset)
	var rows []StatisticsChannelOption
	if err := repository.db.WithContext(ctx).Raw(`SELECT site_id, site_name, remote_channel_id, name, remote_missing `+base+`
ORDER BY site_name ASC, site_id ASC, remote_channel_id ASC
LIMIT ? OFFSET ?`, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("load statistics channel options: %w", err)
	}
	return rows, total, nil
}

func (repository *StatisticsRepository) LoadFlowOptions(ctx context.Context, kind, keyword string, siteIDs []int64, limit, offset int) (any, int64, error) {
	if repository == nil || repository.db == nil || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, ErrStatisticsReadContract
	}
	base, nameColumn := "", ""
	switch kind {
	case "group":
		base = `FROM (
  SELECT f.site_id, s.name AS site_name, f.use_group
  FROM usage_fact_hourly AS f
  JOIN site AS s ON s.id = f.site_id
  JOIN collection_window AS w ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY f.site_id, s.name, f.use_group
) AS options
WHERE 1 = 1`
		nameColumn = "options.use_group"
	case "token":
		base = `FROM (
  SELECT f.site_id, s.name AS site_name, f.token_id,
    COALESCE(MIN(NULLIF(f.token_name, '')), '') AS token_name
  FROM usage_fact_hourly AS f
  JOIN site AS s ON s.id = f.site_id
  JOIN collection_window AS w ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY f.site_id, s.name, f.token_id
) AS options
WHERE 1 = 1`
		nameColumn = "options.token_name"
	case "node":
		base = `FROM (
  SELECT f.site_id, s.name AS site_name, f.node_name
  FROM usage_fact_hourly AS f
  JOIN site AS s ON s.id = f.site_id
  JOIN collection_window AS w ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
  GROUP BY f.site_id, s.name, f.node_name
) AS options
WHERE 1 = 1`
		nameColumn = "options.node_name"
	default:
		return nil, 0, ErrStatisticsReadContract
	}
	where, args := statisticsOptionsWhere(siteIDs, keyword, nameColumn)
	base += where
	var total int64
	if err := repository.db.WithContext(ctx).Raw("SELECT COUNT(*) "+base, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), limit, offset)
	switch kind {
	case "group":
		var rows []StatisticsGroupOption
		err := repository.db.WithContext(ctx).Raw("SELECT site_id,site_name,use_group "+base+" ORDER BY site_name,site_id,use_group LIMIT ? OFFSET ?", queryArgs...).Scan(&rows).Error
		return rows, total, err
	case "token":
		var rows []StatisticsTokenOption
		err := repository.db.WithContext(ctx).Raw("SELECT site_id,site_name,token_id,token_name "+base+" ORDER BY site_name,site_id,token_id LIMIT ? OFFSET ?", queryArgs...).Scan(&rows).Error
		return rows, total, err
	default:
		var rows []StatisticsNodeOption
		err := repository.db.WithContext(ctx).Raw("SELECT site_id,site_name,node_name "+base+" ORDER BY site_name,site_id,node_name LIMIT ? OFFSET ?", queryArgs...).Scan(&rows).Error
		return rows, total, err
	}
}

func statisticsOptionsWhere(siteIDs []int64, keyword, nameColumn string) (string, []any) {
	where := ""
	args := make([]any, 0, 2)
	if len(siteIDs) > 0 {
		where += "\n  AND options.site_id IN ?"
		args = append(args, siteIDs)
	}
	if keyword != "" {
		where += "\n  AND " + nameColumn + " LIKE ? ESCAPE '='"
		escaped := strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(keyword)
		args = append(args, "%"+escaped+"%")
	}
	return where, args
}

func (repository *StatisticsRepository) LoadWindows(ctx context.Context, siteIDs []int64, start, end int64) ([]StatisticsWindow, error) {
	if repository == nil || repository.db == nil || len(siteIDs) == 0 || start <= 0 || end <= start {
		return nil, ErrStatisticsReadContract
	}
	var rows []StatisticsWindow
	err := repository.db.WithContext(ctx).Raw(`
SELECT 'fact' AS row_kind, cw.site_id, cw.hour_ts, cw.status, cw.verified_at,
  cw.last_error_code, cw.last_error_params, '' AS active_task_type
FROM collection_window AS cw
WHERE cw.site_id IN ? AND cw.hour_ts >= ? AND cw.hour_ts < ?
UNION ALL
SELECT 'active' AS row_kind, rw.site_id, rw.hour_ts, '' AS status, NULL AS verified_at,
  '' AS last_error_code, NULL AS last_error_params, r.task_type AS active_task_type
FROM collection_run_window AS rw
JOIN collection_run AS r ON r.id = rw.run_id
WHERE rw.site_id IN ? AND rw.hour_ts >= ? AND rw.hour_ts < ?
	AND r.site_id = rw.site_id
	AND r.target_type = 'site'
	AND r.target_id = rw.site_id
	AND r.start_timestamp IS NOT NULL
	AND r.end_timestamp IS NOT NULL
	AND rw.hour_ts >= r.start_timestamp
	AND rw.hour_ts < r.end_timestamp
  AND r.status IN ('pending','running')
  AND rw.status IN ('pending','running')
  AND r.task_type IN ('usage_hour','usage_backfill','usage_validation')
ORDER BY site_id ASC, hour_ts ASC, row_kind ASC`,
		siteIDs, start, end, siteIDs, start, end).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load statistics windows: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadMetricRows(ctx context.Context, request StatisticsReadRequest) ([]StatisticsMetricRow, error) {
	if repository == nil || repository.db == nil || request.StartTimestamp <= 0 || request.EndTimestamp <= request.StartTimestamp {
		return nil, ErrStatisticsReadContract
	}
	table, dimension, siteColumn, joins, err := statisticsMetricSource(request.Scope, request.Granularity)
	if err != nil {
		return nil, err
	}
	bucket, rangeColumn := "st.hour_ts", "st.hour_ts"
	if request.Granularity != "hour" {
		rangeColumn = "st.date_key"
		switch request.Granularity {
		case "day":
			bucket = "st.date_key"
		case "month":
			bucket = "st.date_key DIV 100"
		case "year":
			bucket = "st.date_key DIV 10000"
		default:
			return nil, ErrStatisticsReadContract
		}
	}
	from := table + " AS st " + joins
	calculatedColumn := "st.last_calculated_at"
	if (request.Scope == "group" || request.Scope == "token" || request.Scope == "node") && request.Granularity == "hour" {
		calculatedColumn = "st.collected_at"
	}
	dimensionName := dimension
	if request.Scope == "token" {
		dimensionName = "COALESCE(MIN(NULLIF(st.token_name, '') COLLATE utf8mb4_bin), '')"
	}
	selectSQL := fmt.Sprintf(`%s AS dimension_id, %s AS dimension_name, %s AS site_id, %s AS bucket_key,
		CAST(SUM(st.request_count) AS CHAR) AS request_count,
		CAST(SUM(st.quota) AS CHAR) AS quota,
		CAST(SUM(st.token_used) AS CHAR) AS token_used,
		MAX(%s) AS last_calculated_at`, dimension, dimensionName, siteColumn, bucket, calculatedColumn)
	query := repository.db.WithContext(ctx).Table(from).Select(selectSQL)
	if request.Granularity == "hour" {
		query = query.Where(rangeColumn+" >= ? AND "+rangeColumn+" < ?", request.StartTimestamp, request.EndTimestamp)
	} else {
		query = query.Where(rangeColumn+" >= ? AND "+rangeColumn+" < ?", request.StartDateKey, request.EndDateKey)
	}
	query = applyStatisticsMetricFilters(query, request)
	group := strings.Join([]string{dimension, siteColumn, bucket}, ", ")
	var rows []StatisticsMetricRow
	if err := query.Group(group).Order("bucket_key ASC, site_id ASC, dimension_id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics metrics: %w", err)
	}
	return rows, nil
}

func (repository *StatisticsRepository) LoadActiveRows(
	ctx context.Context,
	request StatisticsReadRequest,
) ([]StatisticsActiveRow, error) {
	if repository == nil || repository.db == nil || request.StartTimestamp <= 0 || request.EndTimestamp <= request.StartTimestamp {
		return nil, ErrStatisticsReadContract
	}
	query, args, err := statisticsActiveQuery(request)
	if err != nil {
		return nil, err
	}
	var rows []StatisticsActiveRow
	if err := repository.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load statistics active users: %w", err)
	}
	return rows, nil
}

func statisticsActiveQuery(request StatisticsReadRequest) (string, []any, error) {
	from, dimension, dimensionIdentity, totalIdentity, bucket, where, args, err := statisticsActiveSource(request)
	if err != nil {
		return "", nil, err
	}
	if (request.Scope == "global" || request.Scope == "site") &&
		(request.Granularity == "hour" || request.Granularity == "day") {
		return statisticsSiteAggregateActiveQuery(request, from, totalIdentity, where, args)
	}
	totalKey := "CAST(CONCAT_WS(':', " + totalIdentity + ") AS BINARY)"
	query := fmt.Sprintf(`WITH active_base AS (
  SELECT %s AS dimension_id, f.site_id AS site_id, %s AS bucket_key,
    %s AS dimension_identity, %s AS total_identity
  FROM %s
  WHERE %s
  GROUP BY dimension_id, site_id, bucket_key, dimension_identity, total_identity
)
SELECT 'dimension' AS row_kind, dimension_id, site_id, bucket_key,
  CAST(COUNT(DISTINCT dimension_identity) AS CHAR) AS active_users
FROM active_base
GROUP BY dimension_id, site_id, bucket_key
UNION ALL
SELECT 'trend' AS row_kind, '' AS dimension_id, 0 AS site_id, bucket_key,
  CAST(COUNT(DISTINCT total_identity) AS CHAR) AS active_users
FROM active_base
GROUP BY bucket_key
UNION ALL
SELECT 'site' AS row_kind, '' AS dimension_id, site_id, bucket_key,
  CAST(COUNT(DISTINCT dimension_identity) AS CHAR) AS active_users
FROM active_base
GROUP BY site_id, bucket_key
UNION ALL
SELECT 'summary' AS row_kind, '' AS dimension_id, 0 AS site_id, 0 AS bucket_key,
  CAST(COUNT(DISTINCT total_identity) AS CHAR) AS active_users
FROM active_base`, dimension, bucket, dimensionIdentity, totalKey, from, where)
	return query, args, nil
}

func statisticsSiteAggregateActiveQuery(
	request StatisticsReadRequest,
	factFrom, totalIdentity, factWhere string,
	factArgs []any,
) (string, []any, error) {
	table := "site_stat_hourly"
	rangeColumn := "hour_ts"
	start, end := any(request.StartTimestamp), any(request.EndTimestamp)
	if request.Granularity == "day" {
		table = "site_stat_daily"
		rangeColumn = "date_key"
		start, end = request.StartDateKey, request.EndDateKey
	}
	statWhere := "st." + rangeColumn + " >= ? AND st." + rangeColumn + " < ?"
	statArgs := []any{start, end}
	if len(request.SiteIDs) > 0 {
		statWhere += " AND st.site_id IN ?"
		statArgs = append(statArgs, request.SiteIDs)
	}
	statDimension := "CAST(st.site_id AS CHAR)"
	if request.Scope == "global" {
		statDimension = "'global'"
	}
	totalKey := "CAST(CONCAT_WS(':', " + totalIdentity + ") AS BINARY)"
	query := fmt.Sprintf(`WITH site_active AS (
  SELECT st.site_id, st.%s AS bucket_key, st.active_users
  FROM %s AS st
  WHERE %s
)
SELECT 'dimension' AS row_kind, %s AS dimension_id, site_id, bucket_key,
  CAST(active_users AS CHAR) AS active_users
FROM site_active AS st
UNION ALL
SELECT 'trend' AS row_kind, '' AS dimension_id, 0 AS site_id, bucket_key,
  CAST(SUM(active_users) AS CHAR) AS active_users
FROM site_active
GROUP BY bucket_key
UNION ALL
SELECT 'site' AS row_kind, '' AS dimension_id, site_id, bucket_key,
  CAST(active_users AS CHAR) AS active_users
FROM site_active
UNION ALL
SELECT 'summary' AS row_kind, '' AS dimension_id, 0 AS site_id, 0 AS bucket_key,
  CAST(COUNT(DISTINCT %s) AS CHAR) AS active_users
FROM %s
WHERE %s`, rangeColumn, table, statWhere, statDimension, totalKey, factFrom, factWhere)
	allArgs := make([]any, 0, len(statArgs)+len(factArgs))
	allArgs = append(allArgs, statArgs...)
	allArgs = append(allArgs, factArgs...)
	return query, allArgs, nil
}

func (repository *StatisticsRepository) LoadFallbackRates(ctx context.Context) (StatisticsFallbackRates, error) {
	if repository == nil || repository.db == nil {
		return StatisticsFallbackRates{}, ErrStatisticsReadContract
	}
	type row struct {
		Key   string `gorm:"column:setting_key"`
		Value string `gorm:"column:setting_value"`
		Type  string `gorm:"column:value_type"`
	}
	var rows []row
	err := repository.db.WithContext(ctx).Table("platform_setting").
		Select("setting_key, setting_value, value_type").
		Where("setting_key IN ?", []string{
			"rate.fallback_quota_per_unit", "rate.fallback_usd_exchange_rate", "collector.usage_delay_minutes",
		}).
		Find(&rows).Error
	if err != nil {
		return StatisticsFallbackRates{}, fmt.Errorf("load statistics fallback rates: %w", err)
	}
	result := StatisticsFallbackRates{}
	seen := make(map[string]struct{}, len(rows))
	for _, item := range rows {
		seen[item.Key] = struct{}{}
		switch item.Key {
		case "rate.fallback_quota_per_unit":
			if item.Type != "decimal" {
				return StatisticsFallbackRates{}, fmt.Errorf("statistics fallback rate %s has an invalid type", item.Key)
			}
			result.QuotaPerUnit = item.Value
		case "rate.fallback_usd_exchange_rate":
			if item.Type != "decimal" {
				return StatisticsFallbackRates{}, fmt.Errorf("statistics fallback rate %s has an invalid type", item.Key)
			}
			result.USDExchangeRate = item.Value
		case "collector.usage_delay_minutes":
			if item.Type != "int" {
				return StatisticsFallbackRates{}, fmt.Errorf("statistics setting %s has an invalid type", item.Key)
			}
			value, parseErr := strconv.Atoi(item.Value)
			if parseErr != nil || value < 0 || value > 60 {
				return StatisticsFallbackRates{}, fmt.Errorf("statistics setting %s has an invalid value", item.Key)
			}
			result.UsageDelayMinutes = value
		}
	}
	for _, key := range []string{
		"rate.fallback_quota_per_unit", "rate.fallback_usd_exchange_rate", "collector.usage_delay_minutes",
	} {
		if _, exists := seen[key]; !exists {
			return StatisticsFallbackRates{}, fmt.Errorf("statistics setting %s is missing", key)
		}
	}
	return result, nil
}

func statisticsMetricSource(scope, granularity string) (string, string, string, string, error) {
	suffix := "hourly"
	if granularity != "hour" {
		suffix = "daily"
	}
	switch scope {
	case "global":
		return "site_stat_" + suffix, "'global'", "st.site_id", "", nil
	case "site":
		return "site_stat_" + suffix, "CAST(st.site_id AS CHAR)", "st.site_id", "", nil
	case "customer":
		return "customer_stat_" + suffix, "CAST(st.customer_id AS CHAR)", "st.site_id", "", nil
	case "account":
		return "account_stat_" + suffix, "CAST(st.account_id AS CHAR)", "a.site_id", "JOIN account a ON a.id = st.account_id", nil
	case "model":
		return "model_stat_" + suffix, "st.model_name", "st.site_id", "", nil
	case "channel":
		return "channel_stat_" + suffix, "CAST(st.channel_id AS CHAR)", "st.site_id", "", nil
	case "group", "token", "node":
		table := "usage_fact_hourly"
		if suffix == "daily" {
			table = "usage_fact_daily"
		}
		column := map[string]string{"group": "use_group", "token": "token_id", "node": "node_name"}[scope]
		joins := ""
		if suffix == "hourly" {
			joins = "JOIN collection_window w ON w.site_id=st.site_id AND w.hour_ts=st.hour_ts AND w.status='complete'"
		}
		return table, "CAST(st." + column + " AS CHAR)", "st.site_id", joins, nil
	default:
		return "", "", "", "", ErrStatisticsReadContract
	}
}

func applyStatisticsMetricFilters(query *gorm.DB, request StatisticsReadRequest) *gorm.DB {
	siteColumn := "st.site_id"
	if request.Scope == "account" {
		siteColumn = "a.site_id"
	}
	if len(request.SiteIDs) > 0 {
		query = query.Where(siteColumn+" IN ?", request.SiteIDs)
	}
	switch request.Scope {
	case "customer":
		if len(request.CustomerIDs) > 0 {
			query = query.Where("st.customer_id IN ?", request.CustomerIDs)
		}
	case "account":
		if len(request.CustomerIDs) > 0 {
			query = query.Where("a.customer_id IN ?", request.CustomerIDs)
		}
		if len(request.AccountIDs) > 0 {
			query = query.Where("st.account_id IN ?", request.AccountIDs)
		}
	case "model":
		if len(request.ModelNames) > 0 {
			query = query.Where("st.model_name IN ?", request.ModelNames)
		}
	case "channel":
		query = applyStatisticsChannelFilter(query, "st.site_id", "st.channel_id", request.ChannelKeys)
	case "group":
		if len(request.UseGroups) > 0 {
			query = query.Where("st.use_group IN ?", request.UseGroups)
		}
	case "token":
		query = applyStatisticsChannelFilter(query, "st.site_id", "st.token_id", request.TokenKeys)
	case "node":
		if len(request.NodeNames) > 0 {
			query = query.Where("st.node_name IN ?", request.NodeNames)
		}
	}
	return query
}

func applyStatisticsChannelFilter(query *gorm.DB, siteColumn, channelColumn string, keys []StatisticsChannelKey) *gorm.DB {
	if len(keys) == 0 {
		return query
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		clauses = append(clauses, "("+siteColumn+" = ? AND "+channelColumn+" = ?)")
		args = append(args, key.SiteID, key.ChannelID)
	}
	return query.Where("("+strings.Join(clauses, " OR ")+")", args...)
}

func statisticsActiveSource(
	request StatisticsReadRequest,
) (from, dimension, dimensionIdentity, totalIdentity, bucket, where string, args []any, err error) {
	useHourly := request.Granularity == "hour" || request.Scope == "customer" || request.Scope == "account"
	if useHourly {
		from = `usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
JOIN site AS s ON s.id = f.site_id`
		where = `f.hour_ts >= ? AND f.hour_ts < ?
  AND f.remote_user_id > 0
  AND s.statistics_start_at IS NOT NULL AND s.statistics_start_at < f.hour_ts + 3600
  AND (s.statistics_end_at IS NULL OR s.statistics_end_at > f.hour_ts)`
		args = []any{request.StartTimestamp, request.EndTimestamp}
	} else {
		from = "usage_fact_daily AS f JOIN site AS s ON s.id = f.site_id"
		where = "f.date_key >= ? AND f.date_key < ? AND f.remote_user_id > 0"
		args = []any{request.StartDateKey, request.EndDateKey}
	}
	switch request.Granularity {
	case "hour":
		bucket = "f.hour_ts"
	case "day":
		if useHourly {
			bucket = "CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, f.hour_ts, '1970-01-01 08:00:00'), '%Y%m%d') AS UNSIGNED)"
		} else {
			bucket = "f.date_key"
		}
	case "month":
		if useHourly {
			bucket = "CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, f.hour_ts, '1970-01-01 08:00:00'), '%Y%m') AS UNSIGNED)"
		} else {
			bucket = "f.date_key DIV 100"
		}
	case "year":
		if useHourly {
			bucket = "CAST(DATE_FORMAT(TIMESTAMPADD(SECOND, f.hour_ts, '1970-01-01 08:00:00'), '%Y') AS UNSIGNED)"
		} else {
			bucket = "f.date_key DIV 10000"
		}
	default:
		return "", "", "", "", "", "", nil, ErrStatisticsReadContract
	}
	switch request.Scope {
	case "global":
		dimension = "'global'"
		dimensionIdentity = "f.remote_user_id"
		totalIdentity = "f.site_id, f.remote_user_id"
	case "site":
		dimension = "CAST(f.site_id AS CHAR)"
		dimensionIdentity = "f.remote_user_id"
		totalIdentity = "f.site_id, f.remote_user_id"
	case "customer":
		from += `
JOIN account AS a ON a.site_id = f.site_id AND a.remote_user_id = f.remote_user_id
JOIN customer AS c ON c.id = a.customer_id`
		where += `
  AND a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)
  AND (c.statistics_paused_at IS NULL OR f.hour_ts < c.statistics_paused_at)`
		dimension = "CAST(a.customer_id AS CHAR)"
		dimensionIdentity = "a.id"
		totalIdentity = "a.id"
	case "account":
		from += `
JOIN account AS a ON a.site_id = f.site_id AND a.remote_user_id = f.remote_user_id`
		where += `
  AND a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at)`
		dimension = "CAST(a.id AS CHAR)"
		dimensionIdentity = "a.id"
		totalIdentity = "a.id"
	case "model":
		dimension = "f.model_name"
		dimensionIdentity = "f.remote_user_id"
		totalIdentity = "f.site_id, f.remote_user_id"
	case "channel":
		dimension = "CAST(f.channel_id AS CHAR)"
		dimensionIdentity = "f.remote_user_id"
		totalIdentity = "f.site_id, f.remote_user_id"
	case "group", "token", "node":
		column := map[string]string{"group": "use_group", "token": "token_id", "node": "node_name"}[request.Scope]
		dimension = "CAST(f." + column + " AS CHAR)"
		dimensionIdentity = "f.remote_user_id"
		totalIdentity = "f.site_id, f.remote_user_id"
	default:
		return "", "", "", "", "", "", nil, ErrStatisticsReadContract
	}
	if len(request.SiteIDs) > 0 {
		where += "\n  AND f.site_id IN ?"
		args = append(args, request.SiteIDs)
	}
	switch request.Scope {
	case "customer":
		if len(request.CustomerIDs) > 0 {
			where += "\n  AND a.customer_id IN ?"
			args = append(args, request.CustomerIDs)
		}
	case "account":
		if len(request.CustomerIDs) > 0 {
			where += "\n  AND a.customer_id IN ?"
			args = append(args, request.CustomerIDs)
		}
		if len(request.AccountIDs) > 0 {
			where += "\n  AND a.id IN ?"
			args = append(args, request.AccountIDs)
		}
	case "model":
		if len(request.ModelNames) > 0 {
			where += "\n  AND f.model_name IN ?"
			args = append(args, request.ModelNames)
		}
	case "channel":
		if len(request.ChannelKeys) > 0 {
			parts := make([]string, 0, len(request.ChannelKeys))
			for _, key := range request.ChannelKeys {
				parts = append(parts, "(f.site_id = ? AND f.channel_id = ?)")
				args = append(args, key.SiteID, key.ChannelID)
			}
			where += "\n  AND (" + strings.Join(parts, " OR ") + ")"
		}
	case "group":
		if len(request.UseGroups) > 0 {
			where += "\n  AND f.use_group IN ?"
			args = append(args, request.UseGroups)
		}
	case "token":
		if len(request.TokenKeys) > 0 {
			parts := make([]string, 0, len(request.TokenKeys))
			for _, key := range request.TokenKeys {
				parts = append(parts, "(f.site_id = ? AND f.token_id = ?)")
				args = append(args, key.SiteID, key.ChannelID)
			}
			where += "\n  AND (" + strings.Join(parts, " OR ") + ")"
		}
	case "node":
		if len(request.NodeNames) > 0 {
			where += "\n  AND f.node_name IN ?"
			args = append(args, request.NodeNames)
		}
	}
	return from, dimension, dimensionIdentity, totalIdentity, bucket, where, args, nil
}
