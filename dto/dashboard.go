package dto

import "strings"

const (
	DashboardTopTypeSite     = "site"
	DashboardTopTypeCustomer = "customer"
	DashboardTopTypeModel    = "model"
	DashboardTopTypeChannel  = "channel"

	DashboardTopMetricRequestCount = "request_count"
	DashboardTopMetricQuota        = "quota"
)

type DashboardTrendQuery struct {
	Days int
}

func (query *DashboardTrendQuery) Normalize() {
	if query.Days == 0 {
		query.Days = 30
	}
}

func (query DashboardTrendQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Days < 1 || query.Days > 90 {
		errors["days"] = "must be between 1 and 90"
	}
	return nilIfEmpty(errors)
}

type DashboardTopQuery struct {
	Type   string
	Metric string
	Limit  int
}

func (query *DashboardTopQuery) Normalize() {
	query.Type = strings.ToLower(strings.TrimSpace(query.Type))
	query.Metric = strings.ToLower(strings.TrimSpace(query.Metric))
	if query.Limit == 0 {
		query.Limit = 5
	}
}

func (query DashboardTopQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Type != DashboardTopTypeSite && query.Type != DashboardTopTypeCustomer &&
		query.Type != DashboardTopTypeModel && query.Type != DashboardTopTypeChannel {
		errors["type"] = "must be site, customer, model, or channel"
	}
	if query.Metric != DashboardTopMetricRequestCount && query.Metric != DashboardTopMetricQuota {
		errors["metric"] = "must be request_count or quota"
	}
	if query.Limit < 1 || query.Limit > 20 {
		errors["limit"] = "must be between 1 and 20"
	}
	return nilIfEmpty(errors)
}

type DashboardUsageSummary struct {
	UsageSummary
	SiteBreakdown []SiteQuotaBreakdown `json:"site_breakdown"`
	Reason        *MessageRef          `json:"reason"`
}

type DashboardSummary struct {
	Today                     DashboardUsageSummary `json:"today"`
	ActiveAccountsToday       *string               `json:"active_accounts_today"`
	SiteCount                 int                   `json:"site_count"`
	OnlineSiteCount           int                   `json:"online_site_count"`
	OfflineSiteCount          int                   `json:"offline_site_count"`
	CustomerCount             int                   `json:"customer_count"`
	ManagedAccountCount       int                   `json:"managed_account_count"`
	InstanceCount             *int                  `json:"instance_count"`
	OnlineInstanceCount       *int                  `json:"online_instance_count"`
	ResourceCompleteSiteCount int                   `json:"resource_complete_site_count"`
	ResourceExpectedSiteCount int                   `json:"resource_expected_site_count"`
	ResourceStaleSiteIDs      []string              `json:"resource_stale_site_ids"`
	ResourceDataStatus        string                `json:"resource_data_status"`
	ResourceAsOf              *int64                `json:"resource_as_of"`
	ResourceReason            *MessageRef           `json:"resource_reason"`
	RPM                       *string               `json:"rpm"`
	TPM                       *string               `json:"tpm"`
	RealtimeCompleteSiteCount int                   `json:"realtime_complete_site_count"`
	RealtimeExpectedSiteCount int                   `json:"realtime_expected_site_count"`
	StaleSiteIDs              []string              `json:"stale_site_ids"`
	RealtimeDataStatus        string                `json:"realtime_data_status"`
	RealtimeAsOf              *int64                `json:"realtime_as_of"`
	RealtimeReason            *MessageRef           `json:"realtime_reason"`
}

type RankingItem struct {
	DimensionType string               `json:"dimension_type"`
	DimensionID   string               `json:"dimension_id"`
	DimensionName string               `json:"dimension_name"`
	SiteID        *string              `json:"site_id"`
	Value         *string              `json:"value"`
	DataStatus    string               `json:"data_status"`
	SiteBreakdown []SiteQuotaBreakdown `json:"site_breakdown"`
	AsOf          *int64               `json:"as_of"`
	IsFinal       bool                 `json:"is_final"`
	Reason        *MessageRef          `json:"reason"`
}

type DashboardRankingItem = RankingItem

type DashboardSiteHealthItem struct {
	SiteID           string `json:"site_id"`
	SiteName         string `json:"site_name"`
	ManagementStatus string `json:"management_status"`
	OnlineStatus     string `json:"online_status"`
	AuthStatus       string `json:"auth_status"`
	StatisticsStatus string `json:"statistics_status"`
	HealthStatus     string `json:"health_status"`
	UpdatedAt        int64  `json:"updated_at"`
}

type DashboardHealth struct {
	FiringAlertCount          int64                     `json:"firing_alert_count"`
	CriticalAlertCount        int64                     `json:"critical_alert_count"`
	WarningAlertCount         int64                     `json:"warning_alert_count"`
	AuthExpiredSiteIDs        []string                  `json:"auth_expired_site_ids"`
	StatisticsNotReadySiteIDs []string                  `json:"statistics_not_ready_site_ids"`
	YesterdayValidationStatus string                    `json:"yesterday_validation_status"`
	Completeness              Completeness              `json:"completeness"`
	LatestAlerts              []AlertEventItem          `json:"latest_alerts"`
	Sites                     []DashboardSiteHealthItem `json:"sites"`
	AsOf                      *int64                    `json:"as_of"`
	IsFinal                   bool                      `json:"is_final"`
	Reason                    *MessageRef               `json:"reason"`
}
