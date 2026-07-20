package model

import (
	"context"
	"math"
	"sort"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
)

const (
	SiteUserInventoryNormal           = "normal"
	SiteUserInventoryMissing          = "missing"
	SiteUserInventoryDeleted          = "deleted"
	SiteUserInventoryIdentityMismatch = "identity_mismatch"
)

type SiteUserInventory struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID          int64  `gorm:"column:site_id"`
	RemoteUserID    int64  `gorm:"column:remote_user_id"`
	RemoteCreatedAt int64  `gorm:"column:remote_created_at"`
	Username        string `gorm:"column:username"`
	DisplayName     string `gorm:"column:display_name"`
	RemoteRole      int    `gorm:"column:remote_role"`
	RemoteStatus    int    `gorm:"column:remote_status"`
	RemoteGroup     string `gorm:"column:remote_group"`
	Quota           int64  `gorm:"column:quota"`
	UsedQuota       int64  `gorm:"column:used_quota"`
	RequestCount    int64  `gorm:"column:request_count"`
	LastLoginAt     int64  `gorm:"column:last_login_at"`
	RemoteState     string `gorm:"column:remote_state"`
	MissingCount    int    `gorm:"column:missing_count"`
	ConfigVersion   int    `gorm:"column:config_version"`
	FirstSeenAt     int64  `gorm:"column:first_seen_at"`
	LastSeenAt      *int64 `gorm:"column:last_seen_at"`
	CreatedAt       int64  `gorm:"column:created_at"`
	UpdatedAt       int64  `gorm:"column:updated_at"`
}

func (SiteUserInventory) TableName() string { return "site_user_inventory" }

type SiteUserInventoryHourly struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID          int64  `gorm:"column:site_id"`
	RemoteRole      int    `gorm:"column:remote_role"`
	RemoteStatus    int    `gorm:"column:remote_status"`
	RemoteGroup     string `gorm:"column:remote_group"`
	HourTS          int64  `gorm:"column:hour_ts"`
	UserCount       int64  `gorm:"column:user_count"`
	NewUserCount    int64  `gorm:"column:new_user_count"`
	ActiveUserCount int64  `gorm:"column:active_user_count"`
	Quota           int64  `gorm:"column:quota"`
	UsedQuota       int64  `gorm:"column:used_quota"`
	RequestCount    int64  `gorm:"column:request_count"`
	DataStatus      string `gorm:"column:data_status"`
	ConfigVersion   int    `gorm:"column:config_version"`
	CollectedAt     int64  `gorm:"column:collected_at"`
}

func (SiteUserInventoryHourly) TableName() string { return "site_user_inventory_hourly" }

func applySiteUserInventorySnapshot(tx *gorm.DB, site Site, observedAt, hourTS int64, observations []SiteUserObservation) (int64, error) {
	if tx == nil || site.ID <= 0 || site.ConfigVersion <= 0 || observedAt <= 0 || hourTS <= 0 || hourTS%3600 != 0 || len(observations) > 100000 {
		return 0, ErrCollectionRunContract
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].RemoteUserID < observations[j].RemoteUserID })
	byID := make(map[int64]SiteUserObservation, len(observations))
	for _, item := range observations {
		if item.RemoteUserID <= 0 || item.RemoteCreatedAt <= 0 || item.RemoteCreatedAt > observedAt || item.Quota < 0 || item.UsedQuota < 0 || item.RequestCount < 0 || item.LastLoginAt < 0 ||
			!validInventoryString(item.Username, 255) || !validInventoryString(item.DisplayName, 255) || !validInventoryString(item.RemoteGroup, 128) {
			return 0, ErrAccountObservationInvalid
		}
		if _, exists := byID[item.RemoteUserID]; exists {
			return 0, ErrAccountObservationInvalid
		}
		byID[item.RemoteUserID] = item
	}
	var existing []SiteUserInventory
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("site_id = ?", site.ID).Order("remote_user_id ASC").Find(&existing).Error; err != nil {
		return 0, err
	}
	existingByID := make(map[int64]SiteUserInventory, len(existing))
	for _, item := range existing {
		existingByID[item.RemoteUserID] = item
	}
	var writes int64
	for _, observation := range observations {
		current, exists := existingByID[observation.RemoteUserID]
		if exists && current.RemoteCreatedAt != observation.RemoteCreatedAt {
			if err := tx.Model(&SiteUserInventory{}).Where("id = ?", current.ID).Updates(map[string]any{
				"remote_state": SiteUserInventoryIdentityMismatch, "config_version": site.ConfigVersion, "updated_at": observedAt,
			}).Error; err != nil {
				return 0, err
			}
			writes++
			continue
		}
		state := SiteUserInventoryNormal
		if observation.Deleted {
			state = SiteUserInventoryDeleted
		}
		lastSeen := observedAt
		inventory := SiteUserInventory{SiteID: site.ID, RemoteUserID: observation.RemoteUserID, RemoteCreatedAt: observation.RemoteCreatedAt,
			Username: observation.Username, DisplayName: observation.DisplayName, RemoteRole: observation.RemoteRole, RemoteStatus: observation.RemoteStatus,
			RemoteGroup: observation.RemoteGroup, Quota: observation.Quota, UsedQuota: observation.UsedQuota, RequestCount: observation.RequestCount,
			LastLoginAt: observation.LastLoginAt, RemoteState: state, MissingCount: 0, ConfigVersion: site.ConfigVersion,
			FirstSeenAt: observedAt, LastSeenAt: &lastSeen, CreatedAt: observedAt, UpdatedAt: observedAt}
		if exists {
			inventory.ID, inventory.FirstSeenAt, inventory.CreatedAt = current.ID, current.FirstSeenAt, current.CreatedAt
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "site_id"}, {Name: "remote_user_id"}}, DoUpdates: clause.AssignmentColumns([]string{
			"username", "display_name", "remote_role", "remote_status", "remote_group", "quota", "used_quota", "request_count", "last_login_at",
			"remote_state", "missing_count", "config_version", "last_seen_at", "updated_at",
		})}).Create(&inventory).Error; err != nil {
			return 0, err
		}
		writes++
	}
	for _, current := range existing {
		if _, exists := byID[current.RemoteUserID]; exists {
			continue
		}
		if current.RemoteState == SiteUserInventoryIdentityMismatch {
			continue
		}
		missing := current.MissingCount
		if missing < math.MaxInt32 {
			missing++
		}
		if err := tx.Model(&SiteUserInventory{}).Where("id = ?", current.ID).Updates(map[string]any{
			"remote_state": SiteUserInventoryMissing, "missing_count": missing, "config_version": site.ConfigVersion, "updated_at": observedAt,
		}).Error; err != nil {
			return 0, err
		}
		writes++
	}
	hourly, err := inventoryHourlyRows(site.ID, site.ConfigVersion, hourTS, observedAt, observations, existingByID)
	if err != nil {
		return 0, err
	}
	if err := tx.Where("site_id = ? AND hour_ts = ?", site.ID, hourTS).Delete(&SiteUserInventoryHourly{}).Error; err != nil {
		return 0, err
	}
	if len(hourly) > 0 {
		if err := tx.Create(&hourly).Error; err != nil {
			return 0, err
		}
	}
	return writes + int64(len(hourly)), nil
}

func validInventoryString(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit
}

func inventoryHourlyRows(siteID int64, configVersion int, hourTS, observedAt int64, observations []SiteUserObservation, existing map[int64]SiteUserInventory) ([]SiteUserInventoryHourly, error) {
	type key struct {
		role, status int
		group        string
	}
	rows := map[key]SiteUserInventoryHourly{}
	for _, item := range observations {
		if item.Deleted {
			continue
		}
		if prior, ok := existing[item.RemoteUserID]; ok && prior.RemoteCreatedAt != item.RemoteCreatedAt {
			continue
		}
		k := key{item.RemoteRole, item.RemoteStatus, item.RemoteGroup}
		row := rows[k]
		row.SiteID, row.ConfigVersion, row.RemoteRole, row.RemoteStatus, row.RemoteGroup = siteID, configVersion, k.role, k.status, k.group
		row.HourTS, row.DataStatus, row.CollectedAt = hourTS, "complete", observedAt
		var ok bool
		if row.UserCount, ok = addInventoryMetric(row.UserCount, 1); !ok {
			return nil, ErrAccountObservationInvalid
		}
		if item.RemoteCreatedAt >= hourTS && item.RemoteCreatedAt < hourTS+3600 {
			row.NewUserCount++
		}
		if item.LastLoginAt >= hourTS && item.LastLoginAt < hourTS+3600 {
			row.ActiveUserCount++
		}
		if row.Quota, ok = addInventoryMetric(row.Quota, item.Quota); !ok {
			return nil, ErrAccountObservationInvalid
		}
		if row.UsedQuota, ok = addInventoryMetric(row.UsedQuota, item.UsedQuota); !ok {
			return nil, ErrAccountObservationInvalid
		}
		if row.RequestCount, ok = addInventoryMetric(row.RequestCount, item.RequestCount); !ok {
			return nil, ErrAccountObservationInvalid
		}
		rows[k] = row
	}
	result := make([]SiteUserInventoryHourly, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RemoteRole != result[j].RemoteRole {
			return result[i].RemoteRole < result[j].RemoteRole
		}
		if result[i].RemoteStatus != result[j].RemoteStatus {
			return result[i].RemoteStatus < result[j].RemoteStatus
		}
		return result[i].RemoteGroup < result[j].RemoteGroup
	})
	return result, nil
}

func addInventoryMetric(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

type SiteUserInventoryReadRow struct {
	SiteUserInventory
	SiteName  string `gorm:"column:site_name"`
	AccountID *int64 `gorm:"column:account_id"`
	Balance   int64  `gorm:"column:balance"`
}

type SiteUserInventoryMetricRow struct {
	SiteID            int64  `gorm:"column:site_id"`
	SiteName          string `gorm:"column:site_name"`
	DimensionID       string `gorm:"column:dimension_id"`
	DimensionName     string `gorm:"column:dimension_name"`
	BucketStart       int64  `gorm:"column:bucket_start"`
	UserCount         int64  `gorm:"column:user_count"`
	NewUserCount      int64  `gorm:"column:new_user_count"`
	ActiveUserCount   int64  `gorm:"column:active_user_count"`
	Quota             int64  `gorm:"column:quota"`
	UsedQuota         int64  `gorm:"column:used_quota"`
	RequestCount      int64  `gorm:"column:request_count"`
	AsOf              *int64 `gorm:"column:as_of"`
	CompleteSiteCount int64  `gorm:"column:complete_site_count"`
}

type SiteUserInventoryCompletenessRow struct {
	SiteID          int64
	SiteName        string
	InventoryCount  int64
	AsOf            *int64
	LatestRunStatus string
}

type SiteUserInventoryCoverageRow struct {
	BucketStart       int64 `gorm:"column:bucket_start"`
	CompleteSiteCount int64 `gorm:"column:complete_site_count"`
}

type SiteUserInventoryRepository struct{ db *gorm.DB }

func NewSiteUserInventoryRepository(db *gorm.DB) *SiteUserInventoryRepository {
	return &SiteUserInventoryRepository{db: db}
}

func (repository *SiteUserInventoryRepository) List(ctx context.Context, query dto.UserInventoryQuery) ([]SiteUserInventoryReadRow, int64, error) {
	db := repository.db.WithContext(ctx).Table("site_user_inventory AS u").Joins("JOIN site AS s ON s.id = u.site_id").
		Joins("LEFT JOIN account AS a ON a.site_id = u.site_id AND a.remote_user_id = u.remote_user_id")
	if len(query.SiteIDs) > 0 {
		db = db.Where("u.site_id IN ?", query.SiteIDs)
	}
	if query.Keyword != "" {
		pattern := "%" + escapeLike(query.Keyword) + "%"
		db = db.Where("(u.username LIKE ? ESCAPE '\\\\' OR u.display_name LIKE ? ESCAPE '\\\\')", pattern, pattern)
	}
	if query.RemoteUserID != nil {
		db = db.Where("u.remote_user_id = ?", *query.RemoteUserID)
	}
	if len(query.Roles) > 0 {
		db = db.Where("u.remote_role IN ?", query.Roles)
	}
	if len(query.Statuses) > 0 {
		db = db.Where("u.remote_status IN ?", query.Statuses)
	}
	if len(query.Groups) > 0 {
		db = db.Where("u.remote_group IN ?", query.Groups)
	}
	if len(query.States) > 0 {
		db = db.Where("u.remote_state IN ?", query.States)
	}
	if query.MinBalance != nil {
		db = db.Where("u.quota - u.used_quota >= ?", *query.MinBalance)
	}
	if query.MaxBalance != nil {
		db = db.Where("u.quota - u.used_quota <= ?", *query.MaxBalance)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []SiteUserInventoryReadRow
	err := db.Select("u.*, s.name AS site_name, a.id AS account_id, u.quota - u.used_quota AS balance").
		Order("u.site_id ASC").Order("u.remote_user_id ASC").Limit(query.PageSize).Offset(query.Offset()).Scan(&rows).Error
	return rows, total, err
}

func (repository *SiteUserInventoryRepository) CurrentMetrics(ctx context.Context, query dto.UserInventoryStatisticsQuery, dimension string) ([]SiteUserInventoryMetricRow, error) {
	dimensionSQL := map[string]string{"summary": "'summary'", "role": "CAST(u.remote_role AS CHAR)", "status": "CAST(u.remote_status AS CHAR)", "group": "u.remote_group", "site": "CAST(u.site_id AS CHAR)"}[dimension]
	if dimensionSQL == "" {
		return nil, ErrCollectionRunContract
	}
	db := repository.db.WithContext(ctx).Table("site_user_inventory AS u").Joins("JOIN site AS s ON s.id = u.site_id").Where("u.remote_state = ?", SiteUserInventoryNormal)
	db = applyInventoryStatisticsFilters(db, query, "u")
	siteIDSQL, siteNameSQL := "0", "''"
	if dimension == "site" {
		siteIDSQL, siteNameSQL = "u.site_id", "MAX(s.name)"
	}
	selectSQL := dimensionSQL + ` AS dimension_id, ` + dimensionSQL + ` AS dimension_name,
` + siteIDSQL + ` AS site_id,
` + siteNameSQL + ` AS site_name,
COUNT(*) AS user_count,
SUM(u.remote_created_at >= ? AND u.remote_created_at < ?) AS new_user_count,
SUM(u.last_login_at >= ? AND u.last_login_at < ?) AS active_user_count,
COALESCE(SUM(u.quota),0) AS quota, COALESCE(SUM(u.used_quota),0) AS used_quota,
COALESCE(SUM(u.request_count),0) AS request_count, MAX(u.updated_at) AS as_of`
	db = db.Select(selectSQL, query.StartTimestamp, query.EndTimestamp, query.StartTimestamp, query.EndTimestamp)
	if dimension != "summary" {
		db = db.Group(dimensionSQL)
		if dimension == "site" {
			db = db.Group("u.site_id")
		}
	}
	var rows []SiteUserInventoryMetricRow
	err := db.Order("dimension_id ASC").Scan(&rows).Error
	return rows, err
}

func (repository *SiteUserInventoryRepository) Trend(ctx context.Context, query dto.UserInventoryStatisticsQuery) ([]SiteUserInventoryMetricRow, error) {
	db := repository.db.WithContext(ctx).Table("site_user_inventory_hourly AS h").Where("h.hour_ts >= ? AND h.hour_ts < ?", query.StartTimestamp, query.EndTimestamp)
	db = applyInventoryStatisticsFilters(db, query, "h")
	var rows []SiteUserInventoryMetricRow
	err := db.Select(`h.hour_ts AS bucket_start, SUM(h.user_count) AS user_count, SUM(h.new_user_count) AS new_user_count,
SUM(h.active_user_count) AS active_user_count, SUM(h.quota) AS quota, SUM(h.used_quota) AS used_quota,
SUM(h.request_count) AS request_count, MAX(h.collected_at) AS as_of`).Group("h.hour_ts").Order("h.hour_ts ASC").Scan(&rows).Error
	return rows, err
}

func (repository *SiteUserInventoryRepository) Completeness(ctx context.Context, siteIDs []int64) ([]SiteUserInventoryCompletenessRow, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrCollectionRunContract
	}
	type siteRow struct {
		ID   int64  `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var sites []siteRow
	siteQuery := repository.db.WithContext(ctx).Table("site").Select("id, name")
	if len(siteIDs) > 0 {
		siteQuery = siteQuery.Where("id IN ?", siteIDs)
	}
	if err := siteQuery.Order("id ASC").Scan(&sites).Error; err != nil || len(sites) == 0 {
		return nil, err
	}
	ids := make([]int64, 0, len(sites))
	for _, site := range sites {
		ids = append(ids, site.ID)
	}
	type inventoryRow struct {
		SiteID int64  `gorm:"column:site_id"`
		Count  int64  `gorm:"column:inventory_count"`
		AsOf   *int64 `gorm:"column:as_of"`
	}
	var inventories []inventoryRow
	if err := repository.db.WithContext(ctx).Table("site_user_inventory").
		Select("site_id, COUNT(*) AS inventory_count, MAX(updated_at) AS as_of").
		Where("site_id IN ?", ids).Group("site_id").Scan(&inventories).Error; err != nil {
		return nil, err
	}
	type runRow struct {
		SiteID int64  `gorm:"column:site_id"`
		Status string `gorm:"column:status"`
	}
	var runs []runRow
	if err := repository.db.WithContext(ctx).Raw(`SELECT r.site_id, r.status
FROM collection_run AS r
JOIN (
  SELECT site_id, MAX(id) AS id
  FROM collection_run
  WHERE task_type = ? AND site_id IN ?
  GROUP BY site_id
) AS latest ON latest.id = r.id`, constant.TaskTypeUserSync, ids).Scan(&runs).Error; err != nil {
		return nil, err
	}
	inventoryBySite := make(map[int64]inventoryRow, len(inventories))
	for _, row := range inventories {
		inventoryBySite[row.SiteID] = row
	}
	runBySite := make(map[int64]string, len(runs))
	for _, row := range runs {
		runBySite[row.SiteID] = row.Status
	}
	result := make([]SiteUserInventoryCompletenessRow, 0, len(sites))
	for _, site := range sites {
		inventory := inventoryBySite[site.ID]
		result = append(result, SiteUserInventoryCompletenessRow{SiteID: site.ID, SiteName: site.Name,
			InventoryCount: inventory.Count, AsOf: inventory.AsOf, LatestRunStatus: runBySite[site.ID]})
	}
	return result, nil
}

func (repository *SiteUserInventoryRepository) TrendCoverage(ctx context.Context, query dto.UserInventoryStatisticsQuery) ([]SiteUserInventoryCoverageRow, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrCollectionRunContract
	}
	db := repository.db.WithContext(ctx).Table("site_user_inventory_hourly AS h").
		Where("h.hour_ts >= ? AND h.hour_ts < ?", query.StartTimestamp, query.EndTimestamp)
	if len(query.SiteIDs) > 0 {
		db = db.Where("h.site_id IN ?", query.SiteIDs)
	}
	var rows []SiteUserInventoryCoverageRow
	err := db.Select("h.hour_ts AS bucket_start, COUNT(DISTINCT h.site_id) AS complete_site_count").
		Group("h.hour_ts").Order("h.hour_ts ASC").Scan(&rows).Error
	return rows, err
}

func applyInventoryStatisticsFilters(db *gorm.DB, query dto.UserInventoryStatisticsQuery, alias string) *gorm.DB {
	if len(query.SiteIDs) > 0 {
		db = db.Where(alias+".site_id IN ?", query.SiteIDs)
	}
	if len(query.Roles) > 0 {
		db = db.Where(alias+".remote_role IN ?", query.Roles)
	}
	if len(query.Statuses) > 0 {
		db = db.Where(alias+".remote_status IN ?", query.Statuses)
	}
	if len(query.Groups) > 0 {
		db = db.Where(alias+".remote_group IN ?", query.Groups)
	}
	return db
}
