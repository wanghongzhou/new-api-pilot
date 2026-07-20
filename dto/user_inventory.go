package dto

import (
	"strings"
	"unicode/utf8"
)

type UserInventoryQuery struct {
	Page         int
	PageSize     int
	SiteIDs      []int64
	Keyword      string
	RemoteUserID *int64
	Roles        []int
	Statuses     []int
	Groups       []string
	States       []string
	MinBalance   *int64
	MaxBalance   *int64
}

func (query *UserInventoryQuery) Normalize() {
	query.SiteIDs = uniquePositiveIDs(query.SiteIDs)
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Groups = uniqueStatisticsStrings(query.Groups)
	query.States = normalizeEnumList(query.States)
}

func (query UserInventoryQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 || !statisticsPaginationValid(query.Page, query.PageSize) {
		errors["p"] = "invalid pagination"
	}
	if len(query.SiteIDs) > 100 || len(query.Roles) > 20 || len(query.Statuses) > 20 || len(query.Groups) > 100 || len(query.States) > 4 {
		errors["filters"] = "too many filter values"
	}
	if !utf8.ValidString(query.Keyword) || utf8.RuneCountInString(query.Keyword) > 255 {
		errors["keyword"] = "is invalid or too long"
	}
	for _, group := range query.Groups {
		if !utf8.ValidString(group) || utf8.RuneCountInString(group) > 128 {
			errors["groups"] = "contains an invalid group"
		}
	}
	if !validEnumList(query.States, "normal", "missing", "deleted", "identity_mismatch") {
		errors["states"] = "contains an invalid state"
	}
	if query.MinBalance != nil && query.MaxBalance != nil && *query.MinBalance > *query.MaxBalance {
		errors["balance"] = "minimum must not exceed maximum"
	}
	if query.RemoteUserID != nil && *query.RemoteUserID <= 0 {
		errors["remote_user_id"] = "must be positive"
	}
	return nilIfEmpty(errors)
}

func (query UserInventoryQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(query.Page, query.PageSize)
	return offset
}

type UserInventoryItem struct {
	ID              string  `json:"id"`
	SiteID          string  `json:"site_id"`
	SiteName        string  `json:"site_name"`
	RemoteUserID    string  `json:"remote_user_id"`
	RemoteCreatedAt int64   `json:"remote_created_at"`
	Username        string  `json:"username"`
	DisplayName     string  `json:"display_name"`
	Role            int     `json:"role"`
	Status          int     `json:"status"`
	Group           string  `json:"group"`
	Quota           string  `json:"quota"`
	UsedQuota       string  `json:"used_quota"`
	Balance         string  `json:"balance"`
	RequestCount    string  `json:"request_count"`
	LastLoginAt     int64   `json:"last_login_at"`
	RemoteState     string  `json:"remote_state"`
	MissingCount    int     `json:"missing_count"`
	FirstSeenAt     int64   `json:"first_seen_at"`
	LastSeenAt      *int64  `json:"last_seen_at"`
	AccountID       *string `json:"account_id"`
}

type UserInventoryPage struct {
	Items      []UserInventoryItem `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	DataStatus string              `json:"data_status"`
}

type UserInventoryStatisticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	SiteIDs        []int64
	Roles          []int
	Statuses       []int
	Groups         []string
}

func (query *UserInventoryStatisticsQuery) Normalize() {
	query.SiteIDs = uniquePositiveIDs(query.SiteIDs)
	query.Groups = uniqueStatisticsStrings(query.Groups)
}
func (query UserInventoryStatisticsQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.StartTimestamp <= 0 || query.EndTimestamp <= query.StartTimestamp || query.StartTimestamp%3600 != 0 || query.EndTimestamp%3600 != 0 || query.EndTimestamp-query.StartTimestamp > 366*24*3600 {
		errors["range"] = "must be aligned and at most one year"
	}
	if len(query.SiteIDs) > 100 || len(query.Roles) > 20 || len(query.Statuses) > 20 || len(query.Groups) > 100 {
		errors["filters"] = "too many values"
	}
	return nilIfEmpty(errors)
}

type UserInventoryMetric struct {
	UserCount       string `json:"user_count"`
	NewUserCount    string `json:"new_user_count"`
	ActiveUserCount string `json:"active_user_count"`
	Quota           string `json:"quota"`
	UsedQuota       string `json:"used_quota"`
	Balance         string `json:"balance"`
	RequestCount    string `json:"request_count"`
}

type UserInventoryTrendPoint struct {
	BucketStart int64 `json:"bucket_start"`
	BucketEnd   int64 `json:"bucket_end"`
	UserInventoryMetric
	DataStatus string `json:"data_status"`
}
type UserInventoryBreakdown struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	SiteID        string `json:"site_id"`
	UserInventoryMetric
}
type UserInventorySiteBreakdown struct {
	SiteID   string `json:"site_id"`
	SiteName string `json:"site_name"`
	UserInventoryMetric
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}
type UserInventoryStatisticsResponse struct {
	Summary         UserInventoryMetric          `json:"summary"`
	Trend           []UserInventoryTrendPoint    `json:"trend"`
	RoleBreakdown   []UserInventoryBreakdown     `json:"role_breakdown"`
	StatusBreakdown []UserInventoryBreakdown     `json:"status_breakdown"`
	GroupBreakdown  []UserInventoryBreakdown     `json:"group_breakdown"`
	SiteBreakdown   []UserInventorySiteBreakdown `json:"site_breakdown"`
	DataStatus      string                       `json:"data_status"`
}
