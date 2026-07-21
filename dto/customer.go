package dto

import (
	"regexp"
	"strings"
)

const (
	CustomerStatusCommunicating = "communicating"
	CustomerStatusSigning       = "signing"
	CustomerStatusUsing         = "using"
	CustomerStatusDisabled      = "disabled"
)

type CustomerCreateRequest struct {
	Name           string `json:"name"`
	Contact        string `json:"contact,omitempty"`
	Remark         string `json:"remark,omitempty"`
	ContractAmount string `json:"contract_amount"`
	PaymentAmount  string `json:"payment_amount"`
	Status         string `json:"status"`
}

func (request *CustomerCreateRequest) Normalize() {
	request.Name = strings.TrimSpace(request.Name)
	request.Contact = strings.TrimSpace(request.Contact)
	request.Remark = strings.TrimSpace(request.Remark)
	request.ContractAmount = strings.TrimSpace(request.ContractAmount)
	request.PaymentAmount = strings.TrimSpace(request.PaymentAmount)
	if request.ContractAmount == "" {
		request.ContractAmount = "0"
	}
	if request.PaymentAmount == "" {
		request.PaymentAmount = "0"
	}
	request.Status = strings.TrimSpace(request.Status)
}

func (request CustomerCreateRequest) Validate() map[string]string {
	return validateCustomerWrite(request.Name, request.Contact, request.Remark, request.ContractAmount, request.PaymentAmount, request.Status)
}

type CustomerUpdateRequest struct {
	Name           string `json:"name"`
	Contact        string `json:"contact,omitempty"`
	Remark         string `json:"remark,omitempty"`
	ContractAmount string `json:"contract_amount"`
	PaymentAmount  string `json:"payment_amount"`
	Status         string `json:"status"`
}

func (request *CustomerUpdateRequest) Normalize() {
	request.Name = strings.TrimSpace(request.Name)
	request.Contact = strings.TrimSpace(request.Contact)
	request.Remark = strings.TrimSpace(request.Remark)
	request.ContractAmount = strings.TrimSpace(request.ContractAmount)
	request.PaymentAmount = strings.TrimSpace(request.PaymentAmount)
	if request.ContractAmount == "" {
		request.ContractAmount = "0"
	}
	if request.PaymentAmount == "" {
		request.PaymentAmount = "0"
	}
	request.Status = strings.TrimSpace(request.Status)
}

func (request CustomerUpdateRequest) Validate() map[string]string {
	return validateCustomerWrite(request.Name, request.Contact, request.Remark, request.ContractAmount, request.PaymentAmount, request.Status)
}

var customerAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,10})?$`)

func validateCustomerWrite(name, contact, remark, contractAmount, paymentAmount, status string) map[string]string {
	errors := map[string]string{}
	if !validSiteString(name, 1, 128) {
		errors["name"] = "must contain 1 to 128 Unicode characters"
	}
	if !validSiteString(contact, 0, 255) {
		errors["contact"] = "must not exceed 255 Unicode characters"
	}
	if !validSiteString(remark, 0, 500) {
		errors["remark"] = "must not exceed 500 Unicode characters"
	}
	if contractAmount != "" && (!customerAmountPattern.MatchString(contractAmount) || len(strings.Split(contractAmount, ".")[0]) > 28) {
		errors["contract_amount"] = "must be a non-negative decimal with at most 28 integer and 10 fractional digits"
	}
	if paymentAmount != "" && (!customerAmountPattern.MatchString(paymentAmount) || len(strings.Split(paymentAmount, ".")[0]) > 28) {
		errors["payment_amount"] = "must be a non-negative decimal with at most 28 integer and 10 fractional digits"
	}
	if status != CustomerStatusCommunicating && status != CustomerStatusSigning && status != CustomerStatusUsing {
		errors["status"] = "must be one of communicating, signing, using"
	}
	return nilIfNoSiteErrors(errors)
}

func ValidCustomerStatus(status string) bool {
	return status == CustomerStatusCommunicating || status == CustomerStatusSigning ||
		status == CustomerStatusUsing || status == CustomerStatusDisabled
}

type CustomerListQuery struct {
	Page      int
	PageSize  int
	Keyword   string
	Status    string
	SortBy    string
	SortOrder string
}

func (query *CustomerListQuery) Normalize() {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Status = strings.TrimSpace(query.Status)
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

func (query CustomerListQuery) Validate() map[string]string {
	errors := validateListQuery(query.Page, query.PageSize, query.Keyword, query.SortOrder)
	if query.Status != "" && !ValidCustomerStatus(query.Status) {
		errors["status"] = "must be one of communicating, signing, using, disabled"
	}
	if !containsString([]string{"updated_at", "name", "today_quota", "account_count"}, query.SortBy) {
		errors["sort_by"] = "must be one of updated_at, name, today_quota, account_count"
	}
	return nilIfNoSiteErrors(errors)
}

func (query CustomerListQuery) Offset() int {
	if query.Page < 1 || query.PageSize < 1 {
		return 0
	}
	return (query.Page - 1) * query.PageSize
}

func validateListQuery(page, pageSize int, keyword, sortOrder string) map[string]string {
	errors := map[string]string{}
	if page < 1 {
		errors["p"] = "must be at least 1"
	}
	if pageSize < 1 || pageSize > 100 {
		errors["page_size"] = "must be between 1 and 100"
	}
	if !validSiteString(keyword, 0, 128) {
		errors["keyword"] = "must not exceed 128 Unicode characters"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		errors["sort_order"] = "must be asc or desc"
	}
	return errors
}

type SiteQuotaBreakdown struct {
	SiteID          string  `json:"site_id"`
	SiteName        string  `json:"site_name"`
	Quota           *string `json:"quota"`
	QuotaPerUnit    *string `json:"quota_per_unit"`
	USDExchangeRate *string `json:"usd_exchange_rate"`
	RateSource      string  `json:"rate_source"`
	RateUpdatedAt   *int64  `json:"rate_updated_at"`
	DataStatus      string  `json:"data_status"`
}

type CustomerUsageSummary struct {
	UsageSummary
	SiteBreakdown []SiteQuotaBreakdown `json:"site_breakdown"`
}

type CustomerListItem struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Contact              string               `json:"contact"`
	Remark               string               `json:"remark"`
	ContractAmount       string               `json:"contract_amount"`
	PaymentAmount        string               `json:"payment_amount"`
	Status               string               `json:"status"`
	AccountCount         int                  `json:"account_count"`
	ActiveAccountCount   int                  `json:"active_account_count"`
	ArchivedAccountCount int                  `json:"archived_account_count"`
	SiteCount            int                  `json:"site_count"`
	Today                CustomerUsageSummary `json:"today"`
	Backfill             BackfillSummary      `json:"backfill"`
	UpdatedAt            int64                `json:"updated_at"`
}

type CustomerDetail struct {
	CustomerListItem
	StatisticsPausedAt *int64       `json:"statistics_paused_at"`
	Completeness       Completeness `json:"completeness"`
	CreatedAt          int64        `json:"created_at"`
}
