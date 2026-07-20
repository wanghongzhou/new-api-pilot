package dto

import (
	"sort"
	"strconv"
	"strings"
)

const (
	ExportFormatCSV  = "csv"
	ExportFormatXLSX = "xlsx"

	ExportStatusPending = "pending"
	ExportStatusRunning = "running"
	ExportStatusSuccess = "success"
	ExportStatusFailed  = "failed"
	ExportStatusExpired = "expired"
)

type ExportFilters struct {
	StartTimestamp          int64    `json:"start_timestamp"`
	EndTimestamp            int64    `json:"end_timestamp"`
	Granularity             string   `json:"granularity"`
	SiteIDs                 []string `json:"site_ids"`
	CustomerIDs             []string `json:"customer_ids"`
	AccountIDs              []string `json:"account_ids"`
	ModelNames              []string `json:"model_names"`
	ChannelKeys             []string `json:"channel_keys"`
	UseGroups               []string `json:"use_groups"`
	TokenKeys               []string `json:"token_keys"`
	NodeNames               []string `json:"node_names"`
	LogType                 *int     `json:"log_type,omitempty"`
	Username                string   `json:"username,omitempty"`
	TokenName               string   `json:"token_name,omitempty"`
	ChannelID               *string  `json:"channel_id,omitempty"`
	RequestID               string   `json:"request_id,omitempty"`
	UpstreamRequestID       string   `json:"upstream_request_id,omitempty"`
	InventoryRoles          []int    `json:"inventory_roles,omitempty"`
	InventoryStatuses       []int    `json:"inventory_statuses,omitempty"`
	InventoryStates         []string `json:"inventory_states,omitempty"`
	SubscriptionPlanEnabled *bool    `json:"subscription_plan_enabled,omitempty"`
	PricingGroup            string   `json:"pricing_group,omitempty"`
	Keyword                 string   `json:"keyword,omitempty"`
	RemoteUserID            *string  `json:"remote_user_id,omitempty"`
	MinBalance              *string  `json:"min_balance,omitempty"`
	MaxBalance              *string  `json:"max_balance,omitempty"`
	ChannelTypes            []int    `json:"channel_types,omitempty"`
	ChannelStatuses         []int    `json:"channel_statuses,omitempty"`
	ChannelTags             []string `json:"channel_tags,omitempty"`
	ChannelStates           []string `json:"channel_states,omitempty"`
	MinResponseTimeMS       *string  `json:"min_response_time_ms,omitempty"`
	MaxResponseTimeMS       *string  `json:"max_response_time_ms,omitempty"`
	FinanceStatuses         []string `json:"finance_statuses,omitempty"`
	FinanceProviders        []string `json:"finance_providers,omitempty"`
	FinanceMethods          []string `json:"finance_methods,omitempty"`
	FinanceStates           []string `json:"finance_states,omitempty"`
	RemoteID                *string  `json:"remote_id,omitempty"`
	RemoteChannelID         *string  `json:"remote_channel_id,omitempty"`
	TaskID                  string   `json:"task_id,omitempty"`
	TaskPlatforms           []string `json:"task_platforms,omitempty"`
	TaskActions             []string `json:"task_actions,omitempty"`
	TaskStatuses            []string `json:"task_statuses,omitempty"`
	TaskModels              []string `json:"task_models,omitempty"`
	SystemTaskTypes         []string `json:"types,omitempty"`
	SystemTaskStatuses      []string `json:"statuses,omitempty"`
	SystemTaskErrorPresent  *bool    `json:"error_present,omitempty"`
	CreatedStart            int64    `json:"created_start,omitempty"`
	CreatedEnd              int64    `json:"created_end,omitempty"`
	ModelVendorID           *string  `json:"model_vendor_id,omitempty"`
	ModelStatuses           []int    `json:"model_statuses,omitempty"`
	ModelSyncOfficial       []int    `json:"model_sync_official,omitempty"`
	RankingPeriod           string   `json:"ranking_period,omitempty"`
	SortBy                  string   `json:"sort_by"`
	SortOrder               string   `json:"sort_order"`
}

type ExportCreateRequest struct {
	Format         string        `json:"format"`
	StatisticsType string        `json:"statistics_type"`
	Filters        ExportFilters `json:"filters"`
}

func (request *ExportCreateRequest) Normalize() {
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	request.StatisticsType = strings.ToLower(strings.TrimSpace(request.StatisticsType))
	request.Filters.Normalize()
}

func (request ExportCreateRequest) Validate() map[string]string {
	errors := map[string]string{}
	if request.Format != ExportFormatCSV && request.Format != ExportFormatXLSX {
		errors["format"] = "must be csv or xlsx"
	}
	if !validExportScope(request.StatisticsType) {
		errors["statistics_type"] = "must be global, site, customer, account, model, channel, group, token, node, or logs"
	}
	if validExportScope(request.StatisticsType) {
		for key, value := range request.Filters.Validate(request.StatisticsType) {
			errors["filters."+key] = value
		}
	}
	return nilIfEmpty(errors)
}

func (filters *ExportFilters) Normalize() {
	filters.Granularity = strings.ToLower(strings.TrimSpace(filters.Granularity))
	filters.SortBy = strings.ToLower(strings.TrimSpace(filters.SortBy))
	filters.SortOrder = strings.ToLower(strings.TrimSpace(filters.SortOrder))
	filters.SiteIDs = canonicalExportSet(filters.SiteIDs)
	filters.CustomerIDs = canonicalExportSet(filters.CustomerIDs)
	filters.AccountIDs = canonicalExportSet(filters.AccountIDs)
	filters.ModelNames = canonicalExportSet(filters.ModelNames)
	filters.ChannelKeys = canonicalExportSet(filters.ChannelKeys)
	filters.UseGroups = canonicalExportSet(filters.UseGroups)
	filters.TokenKeys = canonicalExportSet(filters.TokenKeys)
	filters.NodeNames = canonicalExportSet(filters.NodeNames)
	filters.Username = strings.TrimSpace(filters.Username)
	filters.TokenName = strings.TrimSpace(filters.TokenName)
	filters.RequestID = strings.TrimSpace(filters.RequestID)
	filters.UpstreamRequestID = strings.TrimSpace(filters.UpstreamRequestID)
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	if filters.RemoteUserID != nil {
		value := strings.TrimSpace(*filters.RemoteUserID)
		filters.RemoteUserID = &value
	}
	filters.InventoryStates = normalizeEnumList(filters.InventoryStates)
	filters.ChannelTags = canonicalExportSet(filters.ChannelTags)
	filters.ChannelStates = normalizeEnumList(filters.ChannelStates)
	filters.FinanceStatuses = normalizeEnumList(filters.FinanceStatuses)
	filters.FinanceProviders = canonicalExportSet(filters.FinanceProviders)
	filters.FinanceMethods = canonicalExportSet(filters.FinanceMethods)
	filters.FinanceStates = normalizeEnumList(filters.FinanceStates)
	if filters.RemoteID != nil {
		value := strings.TrimSpace(*filters.RemoteID)
		filters.RemoteID = &value
	}
	if filters.RemoteChannelID != nil {
		value := strings.TrimSpace(*filters.RemoteChannelID)
		filters.RemoteChannelID = &value
	}
	filters.TaskID = strings.TrimSpace(filters.TaskID)
	filters.TaskPlatforms = canonicalExportSet(filters.TaskPlatforms)
	filters.TaskActions = canonicalExportSet(filters.TaskActions)
	filters.TaskStatuses = normalizeEnumList(filters.TaskStatuses)
	filters.TaskModels = canonicalExportSet(filters.TaskModels)
	filters.SystemTaskTypes = normalizeEnumList(filters.SystemTaskTypes)
	filters.SystemTaskStatuses = normalizeEnumList(filters.SystemTaskStatuses)
	if filters.MinResponseTimeMS != nil {
		value := strings.TrimSpace(*filters.MinResponseTimeMS)
		filters.MinResponseTimeMS = &value
	}
	if filters.MaxResponseTimeMS != nil {
		value := strings.TrimSpace(*filters.MaxResponseTimeMS)
		filters.MaxResponseTimeMS = &value
	}
	if filters.MinBalance != nil {
		value := strings.TrimSpace(*filters.MinBalance)
		filters.MinBalance = &value
	}
	if filters.MaxBalance != nil {
		value := strings.TrimSpace(*filters.MaxBalance)
		filters.MaxBalance = &value
	}
	if filters.ChannelID != nil {
		value := strings.TrimSpace(*filters.ChannelID)
		filters.ChannelID = &value
	}
}

func (filters ExportFilters) Validate(scope string) map[string]string {
	if scope == "logs" {
		query, fields := filters.LogQuery()
		if fields != nil {
			return fields
		}
		return query.Validate()
	}
	if scope == "user_inventory" {
		query, fields := filters.UserInventoryQuery()
		if fields != nil {
			return fields
		}
		return query.Validate()
	}
	if scope == "channel_inventory" {
		query, fields := filters.ChannelInventoryQuery()
		if fields != nil {
			return fields
		}
		return query.Validate()
	}
	if scope == "performance_history" {
		query, fields := filters.PerformanceHistoryQuery()
		if fields != nil {
			return fields
		}
		return query.Validate()
	}
	if scope == "topup_inventory" || scope == "redemption_inventory" {
		query, fields := filters.FinanceInventoryQuery()
		if fields != nil {
			return fields
		}
		return query.Validate()
	}
	if scope == "upstream_tasks" {
		q, fields := filters.UpstreamTaskQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	if scope == "model_catalog" {
		q, fields := filters.ModelCatalogQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	if scope == "model_rankings" || scope == "vendor_rankings" {
		q, fields := filters.LocalRankingQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	if scope == "subscription_plans" {
		q, fields := filters.SubscriptionPlanQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	if scope == "pricing_catalog" || scope == "group_catalog" {
		q, fields := filters.PricingCatalogQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	if scope == "system_tasks" {
		q, fields := filters.SystemTaskQuery()
		if fields != nil {
			return fields
		}
		return q.Validate()
	}
	query, parseErrors := filters.StatisticsQuery(scope)
	if len(parseErrors) > 0 {
		return parseErrors
	}
	return query.ValidateForExport(scope)
}

func (filters ExportFilters) UserInventoryQuery() (UserInventoryQuery, map[string]string) {
	errors := map[string]string{}
	query := UserInventoryQuery{Page: 1, PageSize: 100, Keyword: filters.Keyword, Roles: append([]int(nil), filters.InventoryRoles...),
		Statuses: append([]int(nil), filters.InventoryStatuses...), Groups: append([]string(nil), filters.UseGroups...), States: append([]string(nil), filters.InventoryStates...)}
	query.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	parseBalance := func(value *string, field string) *int64 {
		if value == nil {
			return nil
		}
		parsed, err := strconv.ParseInt(*value, 10, 64)
		if err != nil || strconv.FormatInt(parsed, 10) != *value {
			errors[field] = "must be a canonical int64 string"
			return nil
		}
		return &parsed
	}
	query.MinBalance = parseBalance(filters.MinBalance, "min_balance")
	query.MaxBalance = parseBalance(filters.MaxBalance, "max_balance")
	if filters.RemoteUserID != nil {
		value, err := strconv.ParseInt(*filters.RemoteUserID, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != *filters.RemoteUserID {
			errors["remote_user_id"] = "must be a canonical positive bigint string"
		} else {
			query.RemoteUserID = &value
		}
	}
	query.Normalize()
	return query, nilIfEmpty(errors)
}

func (filters ExportFilters) ChannelInventoryQuery() (ChannelInventoryQuery, map[string]string) {
	errors := map[string]string{}
	query := ChannelInventoryQuery{Page: 1, PageSize: 100, Keyword: filters.Keyword, Types: append([]int(nil), filters.ChannelTypes...), Statuses: append([]int(nil), filters.ChannelStatuses...), Groups: append([]string(nil), filters.UseGroups...), Tags: append([]string(nil), filters.ChannelTags...), States: append([]string(nil), filters.ChannelStates...), MinBalance: filters.MinBalance, MaxBalance: filters.MaxBalance}
	query.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	parse := func(raw *string, field string) *int64 {
		if raw == nil {
			return nil
		}
		v, err := strconv.ParseInt(*raw, 10, 64)
		if err != nil || v < 0 || strconv.FormatInt(v, 10) != *raw {
			errors[field] = "must be a canonical non-negative int64 string"
			return nil
		}
		return &v
	}
	query.MinResponseTimeMS = parse(filters.MinResponseTimeMS, "min_response_time_ms")
	query.MaxResponseTimeMS = parse(filters.MaxResponseTimeMS, "max_response_time_ms")
	query.Normalize()
	return query, nilIfEmpty(errors)
}
func (filters ExportFilters) PerformanceHistoryQuery() (PerformanceHistoryQuery, map[string]string) {
	errors := map[string]string{}
	q := PerformanceHistoryQuery{Page: 1, PageSize: 100, StartTimestamp: filters.StartTimestamp, EndTimestamp: filters.EndTimestamp, ModelNames: append([]string(nil), filters.ModelNames...), Groups: append([]string(nil), filters.UseGroups...)}
	q.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	q.Normalize()
	return q, nilIfEmpty(errors)
}

func (filters ExportFilters) FinanceInventoryQuery() (FinanceInventoryQuery, map[string]string) {
	errors := map[string]string{}
	q := FinanceInventoryQuery{Page: 1, PageSize: 100, StartTimestamp: filters.StartTimestamp, EndTimestamp: filters.EndTimestamp, Statuses: append([]string(nil), filters.FinanceStatuses...), Providers: append([]string(nil), filters.FinanceProviders...), Methods: append([]string(nil), filters.FinanceMethods...), States: append([]string(nil), filters.FinanceStates...), Keyword: filters.Keyword}
	q.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	parse := func(raw *string, field string, positive bool) *int64 {
		if raw == nil {
			return nil
		}
		v, err := strconv.ParseInt(*raw, 10, 64)
		if err != nil || positive && v <= 0 || !positive && v < 0 || strconv.FormatInt(v, 10) != *raw {
			errors[field] = "must be a canonical bigint string"
			return nil
		}
		return &v
	}
	q.RemoteID = parse(filters.RemoteID, "remote_id", true)
	q.RemoteUserID = parse(filters.RemoteUserID, "remote_user_id", false)
	q.Normalize()
	return q, nilIfEmpty(errors)
}
func (filters ExportFilters) UpstreamTaskQuery() (UpstreamTaskQuery, map[string]string) {
	errors := map[string]string{}
	q := UpstreamTaskQuery{Page: 1, PageSize: 100, TaskID: filters.TaskID, Platforms: append([]string(nil), filters.TaskPlatforms...), Groups: append([]string(nil), filters.UseGroups...), Actions: append([]string(nil), filters.TaskActions...), Statuses: append([]string(nil), filters.TaskStatuses...), Models: append([]string(nil), filters.TaskModels...), StartTimestamp: filters.StartTimestamp, EndTimestamp: filters.EndTimestamp}
	q.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	parse := func(raw *string, field string, positive bool) *int64 {
		if raw == nil {
			return nil
		}
		v, err := strconv.ParseInt(*raw, 10, 64)
		if err != nil || positive && v <= 0 || !positive && v < 0 || strconv.FormatInt(v, 10) != *raw {
			errors[field] = "must be canonical bigint"
			return nil
		}
		return &v
	}
	q.RemoteID = parse(filters.RemoteID, "remote_id", true)
	q.RemoteUserID = parse(filters.RemoteUserID, "remote_user_id", false)
	q.RemoteChannelID = parse(filters.RemoteChannelID, "remote_channel_id", false)
	q.Normalize()
	return q, nilIfEmpty(errors)
}
func (filters ExportFilters) ModelCatalogQuery() (ModelCatalogQuery, map[string]string) {
	e := map[string]string{}
	q := ModelCatalogQuery{Page: 1, PageSize: 100, Keyword: filters.Keyword, SiteIDs: parseExportIDs(filters.SiteIDs, "site_ids", e), Statuses: append([]int{}, filters.ModelStatuses...), SyncOfficial: append([]int{}, filters.ModelSyncOfficial...)}
	if filters.ModelVendorID != nil {
		v, err := strconv.ParseInt(*filters.ModelVendorID, 10, 64)
		if err != nil || v < 0 || strconv.FormatInt(v, 10) != *filters.ModelVendorID {
			e["model_vendor_id"] = "must be canonical bigint"
		} else {
			q.VendorID = &v
		}
	}
	q.Normalize()
	return q, nilIfEmpty(e)
}
func (filters ExportFilters) LocalRankingQuery() (LocalRankingQuery, map[string]string) {
	e := map[string]string{}
	q := LocalRankingQuery{Period: filters.RankingPeriod, SiteIDs: parseExportIDs(filters.SiteIDs, "site_ids", e)}
	if q.Period == "" {
		q.Period = "today"
	}
	q.Normalize()
	return q, nilIfEmpty(e)
}
func (filters ExportFilters) SubscriptionPlanQuery() (SubscriptionPlanQuery, map[string]string) {
	e := map[string]string{}
	q := SubscriptionPlanQuery{Page: 1, PageSize: 100, SiteIDs: parseExportIDs(filters.SiteIDs, "site_ids", e), States: append([]string{}, filters.InventoryStates...), Enabled: filters.SubscriptionPlanEnabled, Keyword: filters.Keyword}
	q.Normalize()
	return q, nilIfEmpty(e)
}

func (filters ExportFilters) PricingCatalogQuery() (PricingCatalogQuery, map[string]string) {
	e := map[string]string{}
	q := PricingCatalogQuery{Page: 1, PageSize: 100, SiteIDs: parseExportIDs(filters.SiteIDs, "site_ids", e), States: append([]string{}, filters.InventoryStates...), Keyword: filters.Keyword, Group: filters.PricingGroup}
	q.Normalize()
	return q, nilIfEmpty(e)
}

func (filters ExportFilters) SystemTaskQuery() (SystemTaskQuery, map[string]string) {
	e := map[string]string{}
	q := SystemTaskQuery{Page: 1, PageSize: 100, SiteIDs: parseExportIDs(filters.SiteIDs, "site_ids", e), Types: append([]string{}, filters.SystemTaskTypes...), Statuses: append([]string{}, filters.SystemTaskStatuses...), ErrorPresent: filters.SystemTaskErrorPresent, CreatedStart: filters.CreatedStart, CreatedEnd: filters.CreatedEnd}
	q.Normalize()
	return q, nilIfEmpty(e)
}

func (filters ExportFilters) LogQuery() (LogQuery, map[string]string) {
	errors := map[string]string{}
	query := LogQuery{Page: 1, PageSize: 100, StartTimestamp: filters.StartTimestamp, EndTimestamp: filters.EndTimestamp,
		Type: filters.LogType, Username: filters.Username, ModelName: firstExportValue(filters.ModelNames), TokenName: filters.TokenName,
		UseGroup: firstExportValue(filters.UseGroups), RequestID: filters.RequestID, UpstreamRequestID: filters.UpstreamRequestID}
	query.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	if filters.ChannelID != nil {
		value, err := strconv.ParseInt(*filters.ChannelID, 10, 64)
		if err != nil || value < 0 || strconv.FormatInt(value, 10) != *filters.ChannelID {
			errors["channel_id"] = "must be a canonical non-negative int64 string"
		} else {
			query.ChannelID = &value
		}
	}
	if len(filters.ModelNames) > 1 {
		errors["model_names"] = "logs export supports at most one model"
	}
	if len(filters.UseGroups) > 1 {
		errors["use_groups"] = "logs export supports at most one group"
	}
	query.Normalize()
	return query, nilIfEmpty(errors)
}

func firstExportValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (filters ExportFilters) StatisticsQuery(scope string) (StatisticsQuery, map[string]string) {
	errors := map[string]string{}
	query := StatisticsQuery{
		StartTimestamp: filters.StartTimestamp,
		EndTimestamp:   filters.EndTimestamp,
		Granularity:    filters.Granularity,
		ModelNames:     append([]string(nil), filters.ModelNames...),
		ChannelKeys:    append([]string(nil), filters.ChannelKeys...),
		UseGroups:      append([]string(nil), filters.UseGroups...), TokenKeys: append([]string(nil), filters.TokenKeys...), NodeNames: append([]string(nil), filters.NodeNames...),
		Page:      1,
		PageSize:  100,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
	}
	query.SiteIDs = parseExportIDs(filters.SiteIDs, "site_ids", errors)
	query.CustomerIDs = parseExportIDs(filters.CustomerIDs, "customer_ids", errors)
	query.AccountIDs = parseExportIDs(filters.AccountIDs, "account_ids", errors)
	// Global has a single dimension, so name ordering is equivalent to its
	// documented stable bucket ordering.
	if scope == StatisticsScopeGlobal && query.SortBy == "name" {
		query.SortBy = "bucket_start"
	}
	return query, nilIfEmpty(errors)
}

type ExportListQuery struct {
	Page           int
	PageSize       int
	Statuses       []string
	Format         string
	StatisticsType string
	SortBy         string
	SortOrder      string
}

func (query *ExportListQuery) Normalize() {
	query.Statuses = normalizeEnumList(query.Statuses)
	query.Format = strings.ToLower(strings.TrimSpace(query.Format))
	query.StatisticsType = strings.ToLower(strings.TrimSpace(query.StatisticsType))
	query.SortBy = strings.ToLower(strings.TrimSpace(query.SortBy))
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	if query.SortBy == "" {
		query.SortBy = "created_at"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
}

func (query ExportListQuery) Validate() map[string]string {
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
	if !validEnumList(query.Statuses, ExportStatusPending, ExportStatusRunning, ExportStatusSuccess, ExportStatusFailed, ExportStatusExpired) {
		errors["status"] = "must contain pending, running, success, failed, or expired"
	}
	if query.Format != "" && query.Format != ExportFormatCSV && query.Format != ExportFormatXLSX {
		errors["format"] = "must be csv or xlsx"
	}
	if query.StatisticsType != "" && !validExportScope(query.StatisticsType) {
		errors["statistics_type"] = "must be a supported statistics scope"
	}
	if !containsString([]string{"created_at", "finished_at", "status", "file_size"}, query.SortBy) {
		errors["sort_by"] = "is not supported"
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		errors["sort_order"] = "must be asc or desc"
	}
	return nilIfEmpty(errors)
}

func (query ExportListQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(query.Page, query.PageSize)
	return offset
}

type ExportJobItem struct {
	ID             string        `json:"id"`
	Format         string        `json:"format"`
	StatisticsType string        `json:"statistics_type"`
	Filters        ExportFilters `json:"filters"`
	Status         string        `json:"status"`
	Progress       int           `json:"progress"`
	FileName       string        `json:"file_name"`
	FileSize       string        `json:"file_size"`
	RowCount       string        `json:"row_count"`
	Error          *MessageRef   `json:"error"`
	DataSnapshotAt *int64        `json:"data_snapshot_at"`
	ExpiresAt      *int64        `json:"expires_at"`
	CreatedAt      int64         `json:"created_at"`
	StartedAt      *int64        `json:"started_at"`
	FinishedAt     *int64        `json:"finished_at"`
	Deduplicated   bool          `json:"deduplicated"`
}

func canonicalExportSet(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := uniqueStatisticsStrings(values)
	sort.Strings(result)
	return result
}

func parseExportIDs(values []string, field string, errors map[string]string) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
			errors[field] = "must contain canonical positive ID strings"
			return nil
		}
		result = append(result, parsed)
	}
	return result
}

func validExportScope(value string) bool {
	return containsString([]string{
		StatisticsScopeGlobal, StatisticsScopeSite, StatisticsScopeCustomer,
		StatisticsScopeAccount, StatisticsScopeModel, StatisticsScopeChannel,
		StatisticsScopeGroup, StatisticsScopeToken, StatisticsScopeNode,
		"logs",
		"user_inventory",
		"channel_inventory",
		"performance_history",
		"topup_inventory",
		"redemption_inventory",
		"upstream_tasks",
		"model_catalog",
		"model_rankings", "vendor_rankings",
		"subscription_plans",
		"pricing_catalog", "group_catalog",
		"system_tasks",
	}, value)
}

func validExportStatus(value string) bool {
	return containsString([]string{
		ExportStatusPending, ExportStatusRunning, ExportStatusSuccess,
		ExportStatusFailed, ExportStatusExpired,
	}, value)
}
