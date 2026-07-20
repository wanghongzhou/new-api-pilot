package model

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Site struct {
	ID                    int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Name                  string  `gorm:"column:name"`
	BaseURL               string  `gorm:"column:base_url"`
	ConfigVersion         int     `gorm:"column:config_version"`
	Remark                string  `gorm:"column:remark"`
	ManagementStatus      string  `gorm:"column:management_status"`
	OnlineStatus          string  `gorm:"column:online_status"`
	AuthStatus            string  `gorm:"column:auth_status"`
	StatisticsStatus      string  `gorm:"column:statistics_status"`
	HealthStatus          string  `gorm:"column:health_status"`
	RootUserID            *int64  `gorm:"column:root_user_id"`
	RootCreatedAt         *int64  `gorm:"column:root_created_at"`
	AccessTokenEncrypted  *string `gorm:"column:access_token_encrypted"`
	Version               string  `gorm:"column:version"`
	SystemName            string  `gorm:"column:system_name"`
	QuotaPerUnit          *string `gorm:"column:quota_per_unit"`
	USDExchangeRate       *string `gorm:"column:usd_exchange_rate"`
	LastRateAt            *int64  `gorm:"column:last_rate_at"`
	DataExportEnabled     bool    `gorm:"column:data_export_enabled"`
	CurrentRPM            int64   `gorm:"column:current_rpm"`
	CurrentTPM            int64   `gorm:"column:current_tpm"`
	LastRealtimeStatAt    *int64  `gorm:"column:last_realtime_stat_at"`
	ProbeFailCount        int     `gorm:"column:probe_fail_count"`
	LastProbeAt           *int64  `gorm:"column:last_probe_at"`
	LastProbeSuccessAt    *int64  `gorm:"column:last_probe_success_at"`
	StatisticsStartAt     *int64  `gorm:"column:statistics_start_at"`
	StatisticsStartSource *string `gorm:"column:statistics_start_source"`
	StatisticsEndAt       *int64  `gorm:"column:statistics_end_at"`
	DisabledAt            *int64  `gorm:"column:disabled_at"`
	MonitoringStartAt     *int64  `gorm:"column:monitoring_start_at"`
	CreatedAt             int64   `gorm:"column:created_at"`
	UpdatedAt             int64   `gorm:"column:updated_at"`
}

func (Site) TableName() string { return "site" }

type SiteFilter struct {
	Keyword            string
	ManagementStatuses []string
	OnlineStatuses     []string
	AuthStatuses       []string
	StatisticsStatuses []string
	HealthStatuses     []string
	SortBy             string
	SortOrder          string
	Offset             int
	Limit              int
}

type SiteRepository struct {
	db *gorm.DB
}

func NewSiteRepository(db *gorm.DB) *SiteRepository {
	return &SiteRepository{db: db}
}

func (repository *SiteRepository) WithTransaction(ctx context.Context, operation func(*SiteRepository) error) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return operation(&SiteRepository{db: tx})
	})
}

func (repository *SiteRepository) Create(ctx context.Context, site *Site) error {
	return repository.db.WithContext(ctx).Create(site).Error
}

func (repository *SiteRepository) Save(ctx context.Context, site *Site) error {
	return repository.db.WithContext(ctx).Save(site).Error
}

func (repository *SiteRepository) FindByID(ctx context.Context, id int64) (Site, error) {
	var site Site
	err := repository.db.WithContext(ctx).First(&site, id).Error
	return site, err
}

func (repository *SiteRepository) FindByIDForUpdate(ctx context.Context, id int64) (Site, error) {
	var site Site
	err := repository.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&site, id).Error
	return site, err
}

func (repository *SiteRepository) FindByBaseURL(ctx context.Context, baseURL string) (Site, error) {
	var site Site
	err := repository.db.WithContext(ctx).Where("base_url = ?", baseURL).First(&site).Error
	return site, err
}

func (repository *SiteRepository) List(ctx context.Context, filter SiteFilter) ([]Site, int64, error) {
	query := repository.db.WithContext(ctx).Model(&Site{})
	if filter.Keyword != "" {
		keyword := "%" + escapeLike(filter.Keyword) + "%"
		query = query.Where("(name LIKE ? ESCAPE '\\\\' OR base_url LIKE ? ESCAPE '\\\\')", keyword, keyword)
	}
	if len(filter.ManagementStatuses) > 0 {
		query = query.Where("management_status IN ?", filter.ManagementStatuses)
	}
	if len(filter.OnlineStatuses) > 0 {
		query = query.Where("online_status IN ?", filter.OnlineStatuses)
	}
	if len(filter.AuthStatuses) > 0 {
		query = query.Where("auth_status IN ?", filter.AuthStatuses)
	}
	if len(filter.StatisticsStatuses) > 0 {
		query = query.Where("statistics_status IN ?", filter.StatisticsStatuses)
	}
	if len(filter.HealthStatuses) > 0 {
		query = query.Where("health_status IN ?", filter.HealthStatuses)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		order = "ASC"
	}
	sortExpressions := map[string]string{
		"name":        "name",
		"updated_at":  "updated_at",
		"today_quota": "updated_at",
		"priority": `CASE
			WHEN health_status = 'critical' THEN 0
			WHEN online_status = 'offline' THEN 1
			WHEN auth_status = 'expired' THEN 2
			WHEN statistics_status IN ('partial','error') THEN 3
			WHEN statistics_status = 'backfilling' THEN 4
			WHEN management_status = 'disabled' THEN 6
			ELSE 5 END`,
	}
	expression, exists := sortExpressions[filter.SortBy]
	if !exists {
		return nil, 0, fmt.Errorf("unsupported site sort %q", filter.SortBy)
	}
	var sites []Site
	err := query.Order(expression + " " + order).Order("id DESC").
		Offset(filter.Offset).Limit(filter.Limit).Find(&sites).Error
	return sites, total, err
}

func (repository *SiteRepository) Delete(ctx context.Context, site *Site) error {
	return repository.db.WithContext(ctx).Delete(site).Error
}
