package dto

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"new-api-pilot/constant"
)

type SiteCreateRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Remark  string `json:"remark"`
}

func (request *SiteCreateRequest) Normalize() {
	request.Name = strings.TrimSpace(request.Name)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.Remark = strings.TrimSpace(request.Remark)
}

func (request SiteCreateRequest) Validate() map[string]string {
	return validateSiteFields(request.Name, request.BaseURL, request.Remark)
}

type SiteUpdateRequest struct {
	Name                  string `json:"name"`
	BaseURL               string `json:"base_url"`
	Remark                string `json:"remark"`
	BaseURLPreflightToken string `json:"base_url_preflight_token,omitempty"`
	ConfirmSameSite       bool   `json:"confirm_same_site,omitempty"`
}

func (request *SiteUpdateRequest) Normalize() {
	request.Name = strings.TrimSpace(request.Name)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.Remark = strings.TrimSpace(request.Remark)
	request.BaseURLPreflightToken = strings.TrimSpace(request.BaseURLPreflightToken)
}

func (request SiteUpdateRequest) Validate() map[string]string {
	return validateSiteFields(request.Name, request.BaseURL, request.Remark)
}

type SiteBaseURLPreflightRequest struct {
	BaseURL string `json:"base_url"`
}

func (request *SiteBaseURLPreflightRequest) Normalize() {
	request.BaseURL = strings.TrimSpace(request.BaseURL)
}

func (request SiteBaseURLPreflightRequest) Validate() map[string]string {
	errors := map[string]string{}
	if !validSiteString(request.BaseURL, 1, 255) {
		errors["base_url"] = "must contain 1 to 255 Unicode characters"
	}
	return nilIfNoSiteErrors(errors)
}

type SiteAuthorizeRequest struct {
	Mode                 string  `json:"mode"`
	RootUserID           *string `json:"root_user_id,omitempty"`
	AccessToken          *string `json:"access_token,omitempty"`
	Username             *string `json:"username,omitempty"`
	Password             *string `json:"password,omitempty"`
	ConfirmTokenRotation *bool   `json:"confirm_token_rotation,omitempty"`
}

func (request *SiteAuthorizeRequest) Normalize() {
	request.Mode = strings.TrimSpace(request.Mode)
	if request.RootUserID != nil {
		value := strings.TrimSpace(*request.RootUserID)
		request.RootUserID = &value
	}
	if request.Username != nil {
		value := strings.TrimSpace(*request.Username)
		request.Username = &value
	}
}

func (request SiteAuthorizeRequest) Validate() map[string]string {
	errors := map[string]string{}
	switch request.Mode {
	case "existing_token":
		if request.RootUserID == nil || !validPositiveIDString(*request.RootUserID) {
			errors["root_user_id"] = "must be a positive decimal int64 string"
		}
		if request.AccessToken == nil || !validSiteString(*request.AccessToken, 1, 4096) || strings.ContainsAny(*request.AccessToken, "\r\n") {
			errors["access_token"] = "must contain 1 to 4096 characters without line breaks"
		}
		if request.Username != nil {
			errors["username"] = "is not allowed in existing_token mode"
		}
		if request.Password != nil {
			errors["password"] = "is not allowed in existing_token mode"
		}
		if request.ConfirmTokenRotation != nil {
			errors["confirm_token_rotation"] = "is not allowed in existing_token mode"
		}
	case "login_generate_token":
		if request.Username == nil || !validSiteString(*request.Username, 1, 128) {
			errors["username"] = "must contain 1 to 128 Unicode characters"
		}
		if request.Password == nil || !validSiteString(*request.Password, 1, 1024) {
			errors["password"] = "must contain 1 to 1024 Unicode characters"
		}
		if request.ConfirmTokenRotation == nil || !*request.ConfirmTokenRotation {
			errors["confirm_token_rotation"] = "must be true"
		}
		if request.RootUserID != nil {
			errors["root_user_id"] = "is not allowed in login_generate_token mode"
		}
		if request.AccessToken != nil {
			errors["access_token"] = "is not allowed in login_generate_token mode"
		}
	default:
		errors["mode"] = "must be existing_token or login_generate_token"
	}
	return nilIfNoSiteErrors(errors)
}

func (request SiteAuthorizeRequest) ParsedRootUserID() (int64, bool) {
	if request.RootUserID == nil {
		return 0, false
	}
	value, err := strconv.ParseInt(*request.RootUserID, 10, 64)
	return value, err == nil && value > 0
}

type SiteBatchRefreshRequest struct {
	SiteIDs []string `json:"site_ids"`
}

func (request SiteBatchRefreshRequest) Validate() map[string]string {
	errors := map[string]string{}
	if len(request.SiteIDs) < 1 || len(request.SiteIDs) > 100 {
		errors["site_ids"] = "must contain between 1 and 100 IDs"
		return errors
	}
	seen := make(map[int64]struct{}, len(request.SiteIDs))
	for _, raw := range request.SiteIDs {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			errors["site_ids"] = "must contain positive decimal int64 strings"
			break
		}
		if _, duplicate := seen[value]; duplicate {
			errors["site_ids"] = "must not contain duplicate IDs"
			break
		}
		seen[value] = struct{}{}
	}
	return nilIfNoSiteErrors(errors)
}

func (request SiteBatchRefreshRequest) ParsedSiteIDs() []int64 {
	ids := make([]int64, 0, len(request.SiteIDs))
	for _, raw := range request.SiteIDs {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && value > 0 {
			ids = append(ids, value)
		}
	}
	return ids
}

type SiteBackfillRequest struct {
	StartTimestamp *int64 `json:"start_timestamp,omitempty"`
	EndTimestamp   *int64 `json:"end_timestamp,omitempty"`
	OnlyMissing    *bool  `json:"only_missing,omitempty"`
}

func (request SiteBackfillRequest) Validate() map[string]string {
	errors := map[string]string{}
	if request.StartTimestamp != nil && (*request.StartTimestamp <= 0 || *request.StartTimestamp%3600 != 0) {
		errors["start_timestamp"] = "must be a positive Beijing-hour timestamp"
	}
	if request.EndTimestamp != nil && (*request.EndTimestamp <= 0 || *request.EndTimestamp%3600 != 0) {
		errors["end_timestamp"] = "must be a positive Beijing-hour timestamp"
	}
	if request.StartTimestamp != nil && request.EndTimestamp != nil && *request.EndTimestamp <= *request.StartTimestamp {
		errors["end_timestamp"] = "must be after start_timestamp"
	}
	return nilIfNoSiteErrors(errors)
}

func (request SiteBackfillRequest) MissingOnly() bool {
	return request.OnlyMissing == nil || *request.OnlyMissing
}

type SiteStatisticsEndRequest struct {
	StatisticsEndAt int64 `json:"statistics_end_at"`
}

func (request SiteStatisticsEndRequest) Validate() map[string]string {
	if request.StatisticsEndAt <= 0 || request.StatisticsEndAt%3600 != 0 {
		return map[string]string{"statistics_end_at": "must be a positive Beijing-hour timestamp"}
	}
	return nil
}

type SiteListQuery struct {
	Page               int
	PageSize           int
	Keyword            string
	ManagementStatuses []string
	OnlineStatuses     []string
	AuthStatuses       []string
	StatisticsStatuses []string
	HealthStatuses     []string
	SortBy             string
	SortOrder          string
}

type CollectionRunListQuery struct {
	Page      int
	PageSize  int
	TaskType  string
	Status    string
	SortBy    string
	SortOrder string
}

type CollectionRunWindowListQuery struct {
	Page     int
	PageSize int
	Status   string
}

type RateInfo struct {
	QuotaPerUnit    *string `json:"quota_per_unit"`
	USDExchangeRate *string `json:"usd_exchange_rate"`
	Source          string  `json:"source"`
	UpdatedAt       *int64  `json:"updated_at"`
}

type SiteRealtimeInfo struct {
	RPM       *string `json:"rpm"`
	TPM       *string `json:"tpm"`
	UpdatedAt *int64  `json:"updated_at"`
	Expired   bool    `json:"expired"`
}

type SiteResourceSummary struct {
	InstanceCount       *int     `json:"instance_count"`
	OnlineInstanceCount *int     `json:"online_instance_count"`
	CPUMaxPercent       *float64 `json:"cpu_max_percent"`
	MemoryMaxPercent    *float64 `json:"memory_max_percent"`
	DiskMaxUsedPercent  *float64 `json:"disk_max_used_percent"`
	UpdatedAt           *int64   `json:"updated_at"`
	DataStatus          string   `json:"data_status"`
}

type UsageSummary struct {
	RequestCount *string `json:"request_count"`
	Quota        *string `json:"quota"`
	TokenUsed    *string `json:"token_used"`
	ActiveUsers  *string `json:"active_users"`
	AsOf         *int64  `json:"as_of"`
	DataStatus   string  `json:"data_status"`
	IsFinal      bool    `json:"is_final,omitempty"`
}

// SitePerformanceSummary is a range aggregate returned by the upstream
// performance endpoint. Counts are strings because they are bigint values.
type SitePerformanceSummary struct {
	Hours        int                    `json:"hours"`
	SampledAt    *int64                 `json:"sampled_at"`
	DataStatus   string                 `json:"data_status"`
	RequestCount string                 `json:"request_count"`
	SuccessRate  float64                `json:"success_rate"`
	AvgLatencyMS float64                `json:"avg_latency_ms"`
	AvgTPS       float64                `json:"avg_tps"`
	Models       []SitePerformanceModel `json:"models"`
}

type SitePerformanceModel struct {
	ModelName    string  `json:"model_name"`
	RequestCount string  `json:"request_count"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	AvgTPS       float64 `json:"avg_tps"`
}

type SiteListItem struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	BaseURL           string                 `json:"base_url"`
	ManagementStatus  string                 `json:"management_status"`
	OnlineStatus      string                 `json:"online_status"`
	AuthStatus        string                 `json:"auth_status"`
	StatisticsStatus  string                 `json:"statistics_status"`
	HealthStatus      string                 `json:"health_status"`
	Version           *string                `json:"version"`
	SystemName        *string                `json:"system_name"`
	DataExportEnabled *bool                  `json:"data_export_enabled"`
	Rate              RateInfo               `json:"rate"`
	Realtime          SiteRealtimeInfo       `json:"realtime"`
	Resource          SiteResourceSummary    `json:"resource"`
	Today             UsageSummary           `json:"today"`
	Performance       SitePerformanceSummary `json:"performance"`
	CompletenessRate  float64                `json:"completeness_rate"`
	DisabledAt        *int64                 `json:"disabled_at"`
	UpdatedAt         int64                  `json:"updated_at"`
}

type BackfillSummary struct {
	Status           string      `json:"status"`
	Progress         float64     `json:"progress"`
	TotalWindows     int         `json:"total_windows"`
	CompletedWindows int         `json:"completed_windows"`
	FailedWindows    int         `json:"failed_windows"`
	StartTimestamp   *int64      `json:"start_timestamp"`
	EndTimestamp     *int64      `json:"end_timestamp"`
	LatestError      *MessageRef `json:"latest_error"`
	RunID            *string     `json:"run_id"`
}

type Completeness struct {
	DataStatus             string         `json:"data_status"`
	CompleteSiteCount      int            `json:"complete_site_count"`
	ExpectedSiteCount      int            `json:"expected_site_count"`
	UnitType               string         `json:"unit_type"`
	CompleteUnitCount      int64          `json:"complete_unit_count"`
	ExpectedUnitCount      int64          `json:"expected_unit_count"`
	CompletenessRate       float64        `json:"completeness_rate"`
	MissingSiteIDs         []string       `json:"missing_site_ids"`
	MissingRanges          []MissingRange `json:"missing_ranges"`
	MissingRangeTotal      int            `json:"missing_range_total"`
	MissingRangesTruncated bool           `json:"missing_ranges_truncated"`
	LastVerifiedAt         *int64         `json:"last_verified_at"`
}

type MissingRange struct {
	SiteID         string     `json:"site_id"`
	Status         string     `json:"status"`
	StartTimestamp int64      `json:"start_timestamp"`
	EndTimestamp   int64      `json:"end_timestamp"`
	Reason         MessageRef `json:"reason"`
}

type SiteDetail struct {
	SiteListItem
	Remark                string          `json:"remark"`
	ConfigVersion         int             `json:"config_version"`
	RootUserID            *string         `json:"root_user_id"`
	RootCreatedAt         *int64          `json:"root_created_at"`
	StatisticsStartAt     *int64          `json:"statistics_start_at"`
	StatisticsStartSource *string         `json:"statistics_start_source"`
	StatisticsEndAt       *int64          `json:"statistics_end_at"`
	MonitoringStartAt     *int64          `json:"monitoring_start_at"`
	LastProbeAt           *int64          `json:"last_probe_at"`
	LastProbeSuccessAt    *int64          `json:"last_probe_success_at"`
	Backfill              BackfillSummary `json:"backfill"`
	Completeness          Completeness    `json:"completeness"`
}

type SiteCapabilityResult struct {
	Key     string     `json:"key"`
	Status  string     `json:"status"`
	Message MessageRef `json:"message"`
}

type SiteFirstUserProof struct {
	SnapshotTotal     int64  `json:"snapshot_total"`
	MinUserID         string `json:"min_user_id"`
	EarliestCreatedAt int64  `json:"earliest_created_at"`
	Passed            bool   `json:"passed"`
}

type SiteAuthorizationResult struct {
	RootUserID         string                 `json:"root_user_id"`
	Version            *string                `json:"version"`
	SystemName         *string                `json:"system_name"`
	DataExportEnabled  *bool                  `json:"data_export_enabled"`
	FirstUserProof     SiteFirstUserProof     `json:"first_user_proof"`
	Capabilities       []SiteCapabilityResult `json:"capabilities"`
	FlowDataValidation string                 `json:"flow_data_validation"`
	RootCreatedAt      int64                  `json:"root_created_at"`
	StatisticsStartAt  int64                  `json:"statistics_start_at"`
	BackfillRunID      *string                `json:"backfill_run_id"`
}

type SitePublicIdentity struct {
	BaseURL    string `json:"base_url"`
	SystemName string `json:"system_name"`
	Version    string `json:"version"`
}

type SiteBaseURLPreflightResult struct {
	NormalizedBaseURL string             `json:"normalized_base_url"`
	ChangeType        string             `json:"change_type"`
	OldPublic         SitePublicIdentity `json:"old_public"`
	CandidatePublic   SitePublicIdentity `json:"candidate_public"`
	ContractStatus    string             `json:"contract_status"`
	PreflightToken    string             `json:"preflight_token"`
	ExpiresAt         int64              `json:"expires_at"`
}

type SiteProbeResult struct {
	ProbeSuccess      bool    `json:"probe_success"`
	OnlineStatus      string  `json:"online_status"`
	ContractStatus    string  `json:"contract_status"`
	Version           *string `json:"version"`
	SystemName        *string `json:"system_name"`
	DataExportEnabled *bool   `json:"data_export_enabled"`
	ProbedAt          int64   `json:"probed_at"`
}

type CollectionRunItem struct {
	ID                 string      `json:"id"`
	SiteID             *string     `json:"site_id"`
	SiteConfigVersion  int         `json:"site_config_version"`
	TaskType           string      `json:"task_type"`
	TargetType         string      `json:"target_type"`
	TargetID           string      `json:"target_id"`
	TriggerType        string      `json:"trigger_type"`
	StartTimestamp     *int64      `json:"start_timestamp"`
	EndTimestamp       *int64      `json:"end_timestamp"`
	Status             string      `json:"status"`
	Priority           int         `json:"priority"`
	Progress           float64     `json:"progress"`
	WindowsInitialized bool        `json:"windows_initialized"`
	TotalWindows       int         `json:"total_windows"`
	CompletedWindows   int         `json:"completed_windows"`
	FailedWindows      int         `json:"failed_windows"`
	CreatedRequestID   string      `json:"created_request_id"`
	LastRequestID      string      `json:"last_request_id"`
	FetchedRows        string      `json:"fetched_rows"`
	WrittenRows        string      `json:"written_rows"`
	RetryCount         int         `json:"retry_count"`
	Error              *MessageRef `json:"error"`
	NextAttemptAt      *int64      `json:"next_attempt_at"`
	StartedAt          *int64      `json:"started_at"`
	FinishedAt         *int64      `json:"finished_at"`
	CreatedAt          int64       `json:"created_at"`
	Deduplicated       bool        `json:"deduplicated"`
}

type CollectionRunWindowItem struct {
	ID           string      `json:"id"`
	RunID        string      `json:"run_id"`
	SiteID       string      `json:"site_id"`
	HourTS       int64       `json:"hour_ts"`
	Status       string      `json:"status"`
	FactStatus   string      `json:"fact_status"`
	FetchedRows  string      `json:"fetched_rows"`
	WrittenRows  string      `json:"written_rows"`
	AttemptCount int         `json:"attempt_count"`
	NextRetryAt  *int64      `json:"next_retry_at"`
	VerifiedAt   *int64      `json:"verified_at"`
	Error        *MessageRef `json:"error"`
	StartedAt    *int64      `json:"started_at"`
	FinishedAt   *int64      `json:"finished_at"`
	UpdatedAt    int64       `json:"updated_at"`
}

type SiteInstanceItem struct {
	SiteID                     string   `json:"site_id"`
	NodeName                   string   `json:"node_name"`
	Hostname                   string   `json:"hostname"`
	IsMaster                   bool     `json:"is_master"`
	RuntimeVersion             string   `json:"runtime_version"`
	GOOS                       string   `json:"goos"`
	GOARCH                     string   `json:"goarch"`
	UpstreamStatus             string   `json:"upstream_status"`
	UpstreamStaleAfterSeconds  *int64   `json:"upstream_stale_after_seconds"`
	CurrentStatus              string   `json:"current_status"`
	EffectiveStaleAfterSeconds int      `json:"effective_stale_after_seconds"`
	CPUPercent                 *float64 `json:"cpu_percent"`
	MemoryPercent              *float64 `json:"memory_percent"`
	DiskUsedPercent            *float64 `json:"disk_used_percent"`
	DiskTotalBytes             *string  `json:"disk_total_bytes"`
	DiskUsedBytes              *string  `json:"disk_used_bytes"`
	SampledAt                  *int64   `json:"sampled_at"`
	DataStatus                 string   `json:"data_status"`
	FirstSeenAt                int64    `json:"first_seen_at"`
	StartedAt                  *int64   `json:"started_at"`
	LastSeenAt                 *int64   `json:"last_seen_at"`
	LastSyncedAt               int64    `json:"last_synced_at"`
}

func validateSiteFields(name, baseURL, remark string) map[string]string {
	errors := map[string]string{}
	if !validSiteString(name, 1, 128) {
		errors["name"] = "must contain 1 to 128 Unicode characters"
	}
	if !validSiteString(baseURL, 1, 255) {
		errors["base_url"] = "must contain 1 to 255 Unicode characters"
	}
	if !validSiteString(remark, 0, 500) {
		errors["remark"] = "must not exceed 500 Unicode characters"
	}
	return nilIfNoSiteErrors(errors)
}

func validSiteString(value string, minimum, maximum int) bool {
	if !utf8.ValidString(value) {
		return false
	}
	length := utf8.RuneCountInString(value)
	return length >= minimum && length <= maximum
}

func validPositiveIDString(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func nilIfNoSiteErrors(errors map[string]string) map[string]string {
	if len(errors) == 0 {
		return nil
	}
	return errors
}

func ValidSiteManagementStatus(value string) bool {
	return value == constant.SiteManagementActive || value == constant.SiteManagementDisabled
}

func ValidSiteOnlineStatus(value string) bool {
	return value == constant.SiteOnlineUnknown || value == constant.SiteOnlineOnline || value == constant.SiteOnlineOffline
}

func ValidSiteAuthStatus(value string) bool {
	return value == constant.SiteAuthUnauthorized || value == constant.SiteAuthAuthorized || value == constant.SiteAuthExpired
}

func ValidSiteStatisticsStatus(value string) bool {
	switch value {
	case constant.SiteStatisticsPendingConfig, constant.SiteStatisticsBackfilling, constant.SiteStatisticsReady,
		constant.SiteStatisticsPartial, constant.SiteStatisticsError, constant.SiteStatisticsPaused:
		return true
	default:
		return false
	}
}

func ValidSiteHealthStatus(value string) bool {
	switch value {
	case constant.SiteHealthOK, constant.SiteHealthWarning, constant.SiteHealthCritical, constant.SiteHealthUnavailable:
		return true
	default:
		return false
	}
}
