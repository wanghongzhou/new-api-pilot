package dto

import (
	"strings"
	"unicode/utf8"
)

type FinanceInventoryQuery struct {
	Page, PageSize               int
	SiteIDs                      []int64
	RemoteID, RemoteUserID       *int64
	Statuses, Providers, Methods []string
	States                       []string
	StartTimestamp, EndTimestamp int64
	Keyword                      string
}

func (q *FinanceInventoryQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.Statuses = normalizeEnumList(q.Statuses)
	q.Providers = uniqueStatisticsStrings(q.Providers)
	q.Methods = uniqueStatisticsStrings(q.Methods)
	q.States = normalizeEnumList(q.States)
	q.Keyword = strings.TrimSpace(q.Keyword)
}
func (q FinanceInventoryQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid pagination"
	}
	if len(q.SiteIDs) > 100 || len(q.Statuses) > 100 || len(q.Providers) > 100 || len(q.Methods) > 100 || len(q.States) > 2 {
		e["filters"] = "too many values"
	}
	if !validEnumList(q.States, "normal", "missing") {
		e["states"] = "contains an invalid state"
	}
	if q.RemoteID != nil && *q.RemoteID <= 0 || q.RemoteUserID != nil && *q.RemoteUserID < 0 {
		e["remote_id"] = "invalid remote id"
	}
	if q.StartTimestamp < 0 || q.EndTimestamp < 0 || q.EndTimestamp > 0 && q.EndTimestamp <= q.StartTimestamp {
		e["range"] = "invalid range"
	}
	if !utf8.ValidString(q.Keyword) || utf8.RuneCountInString(q.Keyword) > 255 {
		e["keyword"] = "invalid keyword"
	}
	return nilIfEmpty(e)
}
func (q FinanceInventoryQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type TopupInventoryItem struct {
	ID              string `json:"id"`
	SiteID          string `json:"site_id"`
	RemoteID        string `json:"remote_id"`
	RemoteUserID    string `json:"remote_user_id"`
	SiteName        string `json:"site_name"`
	Amount          string `json:"amount"`
	Money           string `json:"money"`
	PaymentMethod   string `json:"payment_method"`
	PaymentProvider string `json:"payment_provider"`
	CreateTime      int64  `json:"create_time"`
	CompleteTime    int64  `json:"complete_time"`
	Status          string `json:"status"`
	RemoteState     string `json:"remote_state"`
	MissingCount    int    `json:"missing_count"`
	FirstSeenAt     int64  `json:"first_seen_at"`
	LastSeenAt      *int64 `json:"last_seen_at"`
}

type RedemptionInventoryItem struct {
	ID            string `json:"id"`
	SiteID        string `json:"site_id"`
	RemoteID      string `json:"remote_id"`
	RemoteUserID  string `json:"remote_user_id"`
	SiteName      string `json:"site_name"`
	Name          string `json:"name"`
	Status        int    `json:"status"`
	DerivedStatus string `json:"derived_status"`
	Quota         string `json:"quota"`
	CreatedTime   int64  `json:"created_time"`
	RedeemedTime  int64  `json:"redeemed_time"`
	UsedUserID    string `json:"used_user_id"`
	ExpiredTime   int64  `json:"expired_time"`
	RemoteState   string `json:"remote_state"`
	MissingCount  int    `json:"missing_count"`
	FirstSeenAt   int64  `json:"first_seen_at"`
	LastSeenAt    *int64 `json:"last_seen_at"`
}

type FinanceInventoryPage[T any] struct {
	Items      []T    `json:"items"`
	Total      int64  `json:"total"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}

type FinanceMetric struct {
	Count        string `json:"count"`
	MissingCount string `json:"missing_count"`
	Amount       string `json:"amount,omitempty"`
	Money        string `json:"money,omitempty"`
	Quota        string `json:"quota,omitempty"`
}
type FinanceBreakdown struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	SiteID        string `json:"site_id"`
	SiteName      string `json:"site_name"`
	FinanceMetric
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}
type FinanceStatisticsResponse struct {
	Summary           FinanceMetric      `json:"summary"`
	StatusBreakdown   []FinanceBreakdown `json:"status_breakdown"`
	ProviderBreakdown []FinanceBreakdown `json:"provider_breakdown,omitempty"`
	SiteBreakdown     []FinanceBreakdown `json:"site_breakdown"`
	DataStatus        string             `json:"data_status"`
}
