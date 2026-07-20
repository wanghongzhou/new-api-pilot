package dto

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"new-api-pilot/common"
)

const (
	StatisticsScopeGlobal   = "global"
	StatisticsScopeSite     = "site"
	StatisticsScopeCustomer = "customer"
	StatisticsScopeAccount  = "account"
	StatisticsScopeModel    = "model"
	StatisticsScopeChannel  = "channel"
	StatisticsScopeGroup    = "group"
	StatisticsScopeToken    = "token"
	StatisticsScopeNode     = "node"

	StatisticsGranularityHour  = "hour"
	StatisticsGranularityDay   = "day"
	StatisticsGranularityMonth = "month"
	StatisticsGranularityYear  = "year"
)

type StatisticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    string
	SiteIDs        []int64
	CustomerIDs    []int64
	AccountIDs     []int64
	ModelNames     []string
	ChannelKeys    []string
	UseGroups      []string
	TokenKeys      []string
	NodeNames      []string
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
}

type StatisticsOptionQuery struct {
	Keyword  string
	SiteIDs  []int64
	Page     int
	PageSize int
}

func (query *StatisticsOptionQuery) Normalize() {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.SiteIDs = uniquePositiveIDs(query.SiteIDs)
}

func (query StatisticsOptionQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Page < 1 {
		errors["p"] = "must be at least 1"
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		errors["page_size"] = "must be between 1 and 100"
	}
	if !statisticsPaginationValid(query.Page, query.PageSize) {
		errors["p"] = "is too large for page_size"
	}
	if len(query.SiteIDs) > 100 {
		errors["site_ids"] = "must contain at most 100 unique IDs"
	}
	for _, siteID := range query.SiteIDs {
		if siteID <= 0 {
			errors["site_ids"] = "must contain positive IDs"
			break
		}
	}
	if !utf8.ValidString(query.Keyword) || utf8.RuneCountInString(query.Keyword) > 255 {
		errors["keyword"] = "must contain at most 255 Unicode characters"
	}
	return nilIfEmpty(errors)
}

func (query StatisticsOptionQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(query.Page, query.PageSize)
	return offset
}

func (query *StatisticsQuery) Normalize() {
	query.Granularity = strings.ToLower(strings.TrimSpace(query.Granularity))
	query.SortBy = strings.ToLower(strings.TrimSpace(query.SortBy))
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	query.SiteIDs = uniquePositiveIDs(query.SiteIDs)
	query.CustomerIDs = uniquePositiveIDs(query.CustomerIDs)
	query.AccountIDs = uniquePositiveIDs(query.AccountIDs)
	query.ModelNames = uniqueStatisticsStrings(query.ModelNames)
	query.ChannelKeys = uniqueStatisticsStrings(query.ChannelKeys)
	query.UseGroups = uniqueStatisticsStrings(query.UseGroups)
	query.TokenKeys = uniqueStatisticsStrings(query.TokenKeys)
	query.NodeNames = uniqueStatisticsStrings(query.NodeNames)
}

func (query StatisticsQuery) Validate(scope string) map[string]string {
	return query.validate(scope, false)
}

// ValidateForExport keeps the statistics contract but permits the complete
// retained history instead of applying the interactive page span limits.
func (query StatisticsQuery) ValidateForExport(scope string) map[string]string {
	return query.validate(scope, true)
}

func (query StatisticsQuery) validate(scope string, export bool) map[string]string {
	errors := map[string]string{}
	if query.StartTimestamp <= 0 {
		errors["start_timestamp"] = "must be a positive Unix timestamp"
	}
	if query.EndTimestamp <= query.StartTimestamp {
		errors["end_timestamp"] = "must be after start_timestamp"
	}
	if query.Page < 1 {
		errors["p"] = "must be at least 1"
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		errors["page_size"] = "must be between 1 and 100"
	}
	if !statisticsPaginationValid(query.Page, query.PageSize) {
		errors["p"] = "is too large for page_size"
	}
	if len(query.SiteIDs) > 100 || len(query.CustomerIDs) > 100 || len(query.AccountIDs) > 100 ||
		len(query.ModelNames) > 100 || len(query.ChannelKeys) > 100 || len(query.UseGroups) > 100 ||
		len(query.TokenKeys) > 100 || len(query.NodeNames) > 100 {
		errors["filters"] = "each filter supports at most 100 values"
	}
	for _, value := range append(append(append([]int64{}, query.SiteIDs...), query.CustomerIDs...), query.AccountIDs...) {
		if value <= 0 {
			errors["filters"] = "IDs must be positive"
			break
		}
	}
	for _, value := range query.ModelNames {
		if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 255 {
			errors["model_names"] = "values must contain 1 to 255 Unicode characters"
			break
		}
	}
	for _, value := range query.ChannelKeys {
		parts := strings.Split(value, ":")
		if len(parts) != 2 || !canonicalPositiveInt64(parts[0]) || !canonicalNonNegativeInt64(parts[1]) {
			errors["channel_keys"] = "values must use canonical site_id:remote_channel_id keys"
			break
		}
	}
	for _, value := range query.UseGroups {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 128 {
			errors["use_groups"] = "values must contain at most 128 Unicode characters"
		}
	}
	for _, value := range query.NodeNames {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 128 {
			errors["node_names"] = "values must contain at most 128 Unicode characters"
		}
	}
	for _, value := range query.TokenKeys {
		parts := strings.Split(value, ":")
		if len(parts) != 2 || !canonicalPositiveInt64(parts[0]) || !canonicalNonNegativeInt64(parts[1]) {
			errors["token_keys"] = "values must use canonical site_id:token_id keys"
		}
	}
	if export {
		validateStatisticsExportRange(query, errors)
	} else {
		validateStatisticsRange(query, errors)
	}
	validateStatisticsScope(scope, query, errors)
	return nilIfEmpty(errors)
}

func (query StatisticsQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(query.Page, query.PageSize)
	return offset
}

func statisticsPaginationValid(page, pageSize int) bool {
	_, valid := statisticsPaginationOffset(page, pageSize)
	return valid
}

func statisticsPaginationOffset(page, pageSize int) (int, bool) {
	if page < 1 || pageSize < 1 {
		return 0, false
	}
	maximum := int(^uint(0) >> 1)
	if page > 1 && page-1 > maximum/pageSize {
		return 0, false
	}
	return (page - 1) * pageSize, true
}

func validateStatisticsRange(query StatisticsQuery, fieldErrors map[string]string) {
	if query.StartTimestamp <= 0 || query.EndTimestamp <= query.StartTimestamp {
		return
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Unix(query.StartTimestamp, 0).In(location)
	end := time.Unix(query.EndTimestamp, 0).In(location)
	aligned := func(value time.Time, day, month bool) bool {
		if value.Minute() != 0 || value.Second() != 0 || value.Nanosecond() != 0 {
			return false
		}
		if day && value.Hour() != 0 {
			return false
		}
		return !month || value.Day() == 1
	}
	switch query.Granularity {
	case StatisticsGranularityHour:
		if !aligned(start, false, false) || !aligned(end, false, false) {
			fieldErrors["range"] = "hour ranges must align to Beijing hour boundaries"
		} else if query.EndTimestamp-query.StartTimestamp > 31*24*60*60 {
			fieldErrors["range"] = "hour ranges must not exceed 31 days"
		}
	case StatisticsGranularityDay:
		if !aligned(start, true, false) || !aligned(end, true, false) {
			fieldErrors["range"] = "day ranges must align to Beijing day boundaries"
		} else if end.After(start.AddDate(2, 0, 0)) {
			fieldErrors["range"] = "day ranges must not exceed 2 years"
		}
	case StatisticsGranularityMonth:
		if !aligned(start, true, true) || !aligned(end, true, true) {
			fieldErrors["range"] = "month ranges must align to Beijing month boundaries"
		} else if end.After(start.AddDate(20, 0, 0)) {
			fieldErrors["range"] = "month ranges must not exceed 20 years"
		}
	case StatisticsGranularityYear:
		if !aligned(start, true, true) || !aligned(end, true, true) || start.Month() != time.January || end.Month() != time.January {
			fieldErrors["range"] = "year ranges must align to Beijing year boundaries"
		}
	default:
		fieldErrors["granularity"] = "must be hour, day, month, or year"
	}
}

const statisticsMaximumExportBuckets int64 = 1_000_000

func validateStatisticsExportRange(query StatisticsQuery, fieldErrors map[string]string) {
	if query.StartTimestamp <= 0 || query.EndTimestamp <= query.StartTimestamp {
		return
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Unix(query.StartTimestamp, 0).In(location)
	end := time.Unix(query.EndTimestamp, 0).In(location)
	aligned := func(value time.Time, day, month bool) bool {
		if value.Minute() != 0 || value.Second() != 0 || value.Nanosecond() != 0 {
			return false
		}
		if day && value.Hour() != 0 {
			return false
		}
		return !month || value.Day() == 1
	}
	var buckets int64
	switch query.Granularity {
	case StatisticsGranularityHour:
		if !aligned(start, false, false) || !aligned(end, false, false) {
			fieldErrors["range"] = "hour ranges must align to Beijing hour boundaries"
			return
		}
		buckets = (query.EndTimestamp - query.StartTimestamp) / int64(time.Hour/time.Second)
	case StatisticsGranularityDay:
		if !aligned(start, true, false) || !aligned(end, true, false) {
			fieldErrors["range"] = "day ranges must align to Beijing day boundaries"
			return
		}
		buckets = (query.EndTimestamp - query.StartTimestamp) / int64(24*time.Hour/time.Second)
	case StatisticsGranularityMonth:
		if !aligned(start, true, true) || !aligned(end, true, true) {
			fieldErrors["range"] = "month ranges must align to Beijing month boundaries"
			return
		}
		buckets = int64((end.Year()-start.Year())*12 + int(end.Month()-start.Month()))
	case StatisticsGranularityYear:
		if !aligned(start, true, true) || !aligned(end, true, true) ||
			start.Month() != time.January || end.Month() != time.January {
			fieldErrors["range"] = "year ranges must align to Beijing year boundaries"
			return
		}
		buckets = int64(end.Year() - start.Year())
	default:
		fieldErrors["granularity"] = "must be hour, day, month, or year"
		return
	}
	if buckets <= 0 || buckets > statisticsMaximumExportBuckets {
		fieldErrors["range"] = "export range contains too many time buckets"
	}
}

func validateStatisticsScope(scope string, query StatisticsQuery, fieldErrors map[string]string) {
	allowedSort := map[string]struct{}{"request_count": {}, "quota": {}, "token_used": {}, "bucket_start": {}}
	if scope != StatisticsScopeGlobal {
		allowedSort["name"] = struct{}{}
	}
	allowedSort["active_users"] = struct{}{}
	if _, ok := allowedSort[query.SortBy]; !ok {
		fieldErrors["sort_by"] = "is not supported for this statistics scope"
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		fieldErrors["sort_order"] = "must be asc or desc"
	}
	invalid := func(key string, present bool) {
		if present {
			fieldErrors[key] = "is not supported for this statistics scope"
		}
	}
	switch scope {
	case StatisticsScopeGlobal, StatisticsScopeSite:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeCustomer:
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeAccount:
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeModel:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeChannel:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeGroup:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeToken:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("node_names", len(query.NodeNames) > 0)
	case StatisticsScopeNode:
		invalid("customer_ids", len(query.CustomerIDs) > 0)
		invalid("account_ids", len(query.AccountIDs) > 0)
		invalid("model_names", len(query.ModelNames) > 0)
		invalid("channel_keys", len(query.ChannelKeys) > 0)
		invalid("use_groups", len(query.UseGroups) > 0)
		invalid("token_keys", len(query.TokenKeys) > 0)
	default:
		fieldErrors["scope"] = "must be global, site, customer, account, model, channel, group, token, or node"
	}
}

func uniquePositiveIDs(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueStatisticsStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func canonicalPositiveInt64(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func canonicalNonNegativeInt64(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed >= 0 && strconv.FormatInt(parsed, 10) == value
}

type StatisticsRange struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	Timezone       string `json:"timezone"`
	AsOf           int64  `json:"as_of"`
}

type StatisticsSummary struct {
	RequestCount *string `json:"request_count"`
	Quota        *string `json:"quota"`
	TokenUsed    *string `json:"token_used"`
	ActiveUsers  *string `json:"active_users"`
	DataStatus   string  `json:"data_status"`
	IsPartial    bool    `json:"is_partial"`
}

type TrendPoint struct {
	BucketStart       int64                `json:"bucket_start"`
	BucketEnd         int64                `json:"bucket_end"`
	RequestCount      *string              `json:"request_count"`
	Quota             *string              `json:"quota"`
	TokenUsed         *string              `json:"token_used"`
	ActiveUsers       *string              `json:"active_users"`
	DataStatus        string               `json:"data_status"`
	IsFinal           bool                 `json:"is_final"`
	AsOf              *int64               `json:"as_of"`
	CompleteSiteCount int                  `json:"complete_site_count"`
	ExpectedSiteCount int                  `json:"expected_site_count"`
	SiteBreakdown     []SiteQuotaBreakdown `json:"site_breakdown"`
	Reason            *MessageRef          `json:"reason"`
}

type StatisticsBreakdownBase struct {
	DimensionID      string               `json:"dimension_id"`
	DimensionName    string               `json:"dimension_name"`
	SiteID           *string              `json:"site_id"`
	SiteName         *string              `json:"site_name"`
	BucketStart      int64                `json:"bucket_start"`
	BucketEnd        int64                `json:"bucket_end"`
	RequestCount     *string              `json:"request_count"`
	Quota            *string              `json:"quota"`
	TokenUsed        *string              `json:"token_used"`
	ActiveUsers      *string              `json:"active_users"`
	DataStatus       string               `json:"data_status"`
	IsFinal          bool                 `json:"is_final"`
	AsOf             *int64               `json:"as_of"`
	SiteBreakdown    []SiteQuotaBreakdown `json:"site_breakdown"`
	CompletenessRate float64              `json:"completeness_rate"`
}

type StatisticsBreakdownItem interface {
	statisticsBreakdownItem()
}

type GlobalStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType     string `json:"dimension_type"`
	CompleteSiteCount int    `json:"complete_site_count"`
	ExpectedSiteCount int    `json:"expected_site_count"`
}

func (GlobalStatisticsBreakdown) statisticsBreakdownItem() {}

type SiteStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType    string   `json:"dimension_type"`
	ManagementStatus string   `json:"management_status"`
	OnlineStatus     string   `json:"online_status"`
	AuthStatus       string   `json:"auth_status"`
	StatisticsStatus string   `json:"statistics_status"`
	HealthStatus     string   `json:"health_status"`
	Rate             RateInfo `json:"rate"`
}

func (SiteStatisticsBreakdown) statisticsBreakdownItem() {}

type CustomerStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	AccountCount  int    `json:"account_count"`
	SiteCount     int    `json:"site_count"`
}

func (CustomerStatisticsBreakdown) statisticsBreakdownItem() {}

type AccountStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	CustomerID    string `json:"customer_id"`
	CustomerName  string `json:"customer_name"`
	RemoteUserID  string `json:"remote_user_id"`
}

func (AccountStatisticsBreakdown) statisticsBreakdownItem() {}

type ModelStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	ModelName     string `json:"model_name"`
}

func (ModelStatisticsBreakdown) statisticsBreakdownItem() {}

type ChannelStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType   string `json:"dimension_type"`
	RemoteChannelID string `json:"remote_channel_id"`
	RemoteMissing   bool   `json:"remote_missing"`
}

func (ChannelStatisticsBreakdown) statisticsBreakdownItem() {}

type GroupStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	UseGroup      string `json:"use_group"`
}

func (GroupStatisticsBreakdown) statisticsBreakdownItem() {}

type TokenStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	TokenID       string `json:"token_id"`
	TokenName     string `json:"token_name"`
}

func (TokenStatisticsBreakdown) statisticsBreakdownItem() {}

type NodeStatisticsBreakdown struct {
	StatisticsBreakdownBase
	DimensionType string `json:"dimension_type"`
	NodeName      string `json:"node_name"`
}

func (NodeStatisticsBreakdown) statisticsBreakdownItem() {}

type StatisticsResponse struct {
	Scope         string                                   `json:"scope"`
	Granularity   string                                   `json:"granularity"`
	Range         StatisticsRange                          `json:"range"`
	Summary       StatisticsSummary                        `json:"summary"`
	Trend         []TrendPoint                             `json:"trend"`
	Breakdown     common.PageData[StatisticsBreakdownItem] `json:"breakdown"`
	SiteBreakdown []SiteQuotaBreakdown                     `json:"site_breakdown"`
	Completeness  Completeness                             `json:"completeness"`
}

type ModelOption struct {
	Key       string `json:"key"`
	SiteID    string `json:"site_id"`
	SiteName  string `json:"site_name"`
	ModelName string `json:"model_name"`
}

type ChannelOption struct {
	Key             string `json:"key"`
	SiteID          string `json:"site_id"`
	SiteName        string `json:"site_name"`
	RemoteChannelID string `json:"remote_channel_id"`
	Name            string `json:"name"`
	RemoteMissing   bool   `json:"remote_missing"`
}

type GroupOption struct {
	Key      string `json:"key"`
	SiteID   string `json:"site_id"`
	SiteName string `json:"site_name"`
	UseGroup string `json:"use_group"`
}

type TokenOption struct {
	Key       string `json:"key"`
	SiteID    string `json:"site_id"`
	SiteName  string `json:"site_name"`
	TokenID   string `json:"token_id"`
	TokenName string `json:"token_name"`
}

type NodeOption struct {
	Key      string `json:"key"`
	SiteID   string `json:"site_id"`
	SiteName string `json:"site_name"`
	NodeName string `json:"node_name"`
}
