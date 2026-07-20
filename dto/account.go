package dto

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	AccountRemoteStateNormal           = "normal"
	AccountRemoteStateMissing          = "missing"
	AccountRemoteStateIdentityMismatch = "identity_mismatch"
	AccountManagedStatusActive         = "active"
	AccountManagedStatusArchived       = "archived"
)

type AccountCreateRequest struct {
	SiteID       string `json:"site_id"`
	CustomerID   string `json:"customer_id"`
	RemoteUserID string `json:"remote_user_id"`
	Remark       string `json:"remark,omitempty"`
}

func (request *AccountCreateRequest) Normalize() {
	request.SiteID = strings.TrimSpace(request.SiteID)
	request.CustomerID = strings.TrimSpace(request.CustomerID)
	request.RemoteUserID = strings.TrimSpace(request.RemoteUserID)
	request.Remark = strings.TrimSpace(request.Remark)
}

func (request AccountCreateRequest) Validate() map[string]string {
	errors := map[string]string{}
	if !validPositiveIDString(request.SiteID) {
		errors["site_id"] = "must be a canonical positive decimal int64 string"
	}
	if !validPositiveIDString(request.CustomerID) {
		errors["customer_id"] = "must be a canonical positive decimal int64 string"
	}
	if !validPositiveIDString(request.RemoteUserID) {
		errors["remote_user_id"] = "must be a canonical positive decimal int64 string"
	}
	if !validSiteString(request.Remark, 0, 500) {
		errors["remark"] = "must not exceed 500 Unicode characters"
	}
	return nilIfNoSiteErrors(errors)
}

func (request AccountCreateRequest) BindingIDs() (siteID, customerID, remoteUserID int64, err error) {
	if fieldErrors := request.Validate(); fieldErrors != nil {
		return 0, 0, 0, fmt.Errorf("invalid account binding IDs")
	}
	siteID, _ = strconv.ParseInt(request.SiteID, 10, 64)
	customerID, _ = strconv.ParseInt(request.CustomerID, 10, 64)
	remoteUserID, _ = strconv.ParseInt(request.RemoteUserID, 10, 64)
	return siteID, customerID, remoteUserID, nil
}

type AccountUpdateRequest struct {
	Remark string `json:"remark"`
}

func (request *AccountUpdateRequest) Normalize() {
	request.Remark = strings.TrimSpace(request.Remark)
}

func (request AccountUpdateRequest) Validate() map[string]string {
	if !validSiteString(request.Remark, 0, 500) {
		return map[string]string{"remark": "must not exceed 500 Unicode characters"}
	}
	return nil
}

func ValidAccountRemoteState(state string) bool {
	return state == AccountRemoteStateNormal || state == AccountRemoteStateMissing ||
		state == AccountRemoteStateIdentityMismatch
}

func ValidAccountManagedStatus(status string) bool {
	return status == AccountManagedStatusActive || status == AccountManagedStatusArchived
}

type AccountListQuery struct {
	Page          int
	PageSize      int
	Keyword       string
	SiteID        string
	CustomerID    string
	RemoteStatus  *int
	RemoteState   string
	ManagedStatus string
	SortBy        string
	SortOrder     string
}

func (query *AccountListQuery) Normalize() {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.SiteID = strings.TrimSpace(query.SiteID)
	query.CustomerID = strings.TrimSpace(query.CustomerID)
	query.RemoteState = strings.TrimSpace(query.RemoteState)
	query.ManagedStatus = strings.TrimSpace(query.ManagedStatus)
	query.SortBy = strings.TrimSpace(query.SortBy)
	query.SortOrder = strings.TrimSpace(query.SortOrder)
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.SortBy == "" {
		query.SortBy = "updated_at"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
}

func (query AccountListQuery) Validate() map[string]string {
	errors := validateListQuery(query.Page, query.PageSize, query.Keyword, query.SortOrder)
	if query.SiteID != "" && !validPositiveIDString(query.SiteID) {
		errors["site_id"] = "must be a canonical positive decimal int64 string"
	}
	if query.CustomerID != "" && !validPositiveIDString(query.CustomerID) {
		errors["customer_id"] = "must be a canonical positive decimal int64 string"
	}
	if query.RemoteState != "" && !ValidAccountRemoteState(query.RemoteState) {
		errors["remote_state"] = "must be one of normal, missing, identity_mismatch"
	}
	if query.ManagedStatus != "" && !ValidAccountManagedStatus(query.ManagedStatus) {
		errors["managed_status"] = "must be active or archived"
	}
	if !containsString([]string{"updated_at", "username", "today_quota", "quota"}, query.SortBy) {
		errors["sort_by"] = "must be one of updated_at, username, today_quota, quota"
	}
	return nilIfNoSiteErrors(errors)
}

func (query AccountListQuery) Offset() int {
	if query.Page < 1 || query.PageSize < 1 {
		return 0
	}
	return (query.Page - 1) * query.PageSize
}

type RemoteUserListQuery struct {
	Page     int
	PageSize int
	Keyword  string
}

func (query *RemoteUserListQuery) Normalize() {
	query.Keyword = strings.TrimSpace(query.Keyword)
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
}

func (query RemoteUserListQuery) Validate() map[string]string {
	return nilIfNoSiteErrors(validateListQuery(query.Page, query.PageSize, query.Keyword, "desc"))
}

func (query RemoteUserListQuery) Offset() int {
	if query.Page < 1 || query.PageSize < 1 {
		return 0
	}
	return (query.Page - 1) * query.PageSize
}

type AccountListItem struct {
	ID              string          `json:"id"`
	SiteID          string          `json:"site_id"`
	SiteName        string          `json:"site_name"`
	CustomerID      string          `json:"customer_id"`
	CustomerName    string          `json:"customer_name"`
	RemoteUserID    string          `json:"remote_user_id"`
	RemoteCreatedAt int64           `json:"remote_created_at"`
	Username        string          `json:"username"`
	DisplayName     string          `json:"display_name"`
	RemoteGroup     string          `json:"remote_group"`
	RemoteStatus    int             `json:"remote_status"`
	RemoteState     string          `json:"remote_state"`
	ManagedStatus   string          `json:"managed_status"`
	Quota           string          `json:"quota"`
	UsedQuota       string          `json:"used_quota"`
	RequestCount    string          `json:"request_count"`
	Rate            RateInfo        `json:"rate"`
	LastSyncedAt    *int64          `json:"last_synced_at"`
	Today           UsageSummary    `json:"today"`
	Backfill        BackfillSummary `json:"backfill"`
	UpdatedAt       int64           `json:"updated_at"`
}

type AccountDetail struct {
	AccountListItem
	Remark             string       `json:"remark"`
	RemoteMissingCount int          `json:"remote_missing_count"`
	LastRemoteSeenAt   *int64       `json:"last_remote_seen_at"`
	StatisticsPausedAt *int64       `json:"statistics_paused_at"`
	Completeness       Completeness `json:"completeness"`
	CreatedAt          int64        `json:"created_at"`
}

type RemoteUserItem struct {
	ID                  string  `json:"id"`
	Username            string  `json:"username"`
	DisplayName         string  `json:"display_name"`
	Role                int     `json:"role"`
	Status              int     `json:"status"`
	Group               string  `json:"group"`
	Quota               string  `json:"quota"`
	UsedQuota           string  `json:"used_quota"`
	RequestCount        string  `json:"request_count"`
	CreatedAt           int64   `json:"created_at"`
	LastLoginAt         *int64  `json:"last_login_at"`
	AlreadyManaged      bool    `json:"already_managed"`
	ManagedAccountID    *string `json:"managed_account_id"`
	ManagedCustomerName string  `json:"managed_customer_name"`
}
