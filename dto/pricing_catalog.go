package dto

import (
	"strings"
	"unicode/utf8"
)

type UpstreamPricingItem struct {
	ModelName, VendorName, Description, Icon, Tags, OwnerBy, ModelRatio, ModelPrice, CompletionRatio string
	BillingMode, BillingExpr, PricingSource                                                          string
	CacheRatio, CreateCacheRatio, ImageRatio, AudioRatio, AudioCompletionRatio                       *string
	VendorID, QuotaType                                                                              int64
	AbilityAvailable                                                                                 bool
	EnableGroups, SupportedEndpointTypes                                                             []string
}

type UpstreamPricingGroup struct {
	Name, Description                    string
	Ratio, TopupRatio                    *string
	UserSelectable, DefaultUseAutoGroup  bool
	AutoPriority                         *int
	OutgoingOverrides, IncomingOverrides map[string]string
	VisibleToGroups                      map[string]string
	HiddenFromGroups                     []string
}

type UpstreamPricingSnapshot struct {
	PricingVersion string
	Items          []UpstreamPricingItem
	Groups         []UpstreamPricingGroup
}
type UpstreamPricingGroupSnapshot struct{ Groups []UpstreamPricingGroup }
type UpstreamPricingOnlySnapshot struct {
	PricingVersion string
	Items          []UpstreamPricingItem
	Groups         []UpstreamPricingGroup
}

type UpstreamPricingConfiguration struct {
	ModelPrice, ModelRatio, CompletionRatio, CacheRatio, CreateCacheRatio map[string]string
	ImageRatio, AudioRatio, AudioCompletionRatio                          map[string]string
	BillingMode, BillingExpr                                              map[string]string
	GroupRatio, TopupGroupRatio                                           map[string]string
	UserUsableGroups                                                      map[string]string
	GroupGroupRatio                                                       map[string]map[string]string
	AutoGroups                                                            []string
	DefaultUseAutoGroup                                                   bool
	GroupSpecialUsableGroup                                               map[string]map[string]string
}

type PricingCatalogQuery struct {
	Page, PageSize int
	SiteIDs        []int64
	States         []string
	Keyword        string
	Group          string
	BillingMode    string
}

func (q *PricingCatalogQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.States = normalizeEnumList(q.States)
	q.Keyword = strings.TrimSpace(q.Keyword)
	q.Group = strings.TrimSpace(q.Group)
	q.BillingMode = strings.TrimSpace(q.BillingMode)
}

func (q PricingCatalogQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid"
	}
	if len(q.SiteIDs) > 100 || len(q.States) > 2 {
		e["filters"] = "invalid"
	}
	for _, state := range q.States {
		if state != "normal" && state != "missing" {
			e["states"] = "invalid"
		}
	}
	if !utf8.ValidString(q.Keyword) || len(q.Keyword) > 255 {
		e["keyword"] = "must be valid UTF-8 and at most 255 bytes"
	}
	if !utf8.ValidString(q.Group) || len(q.Group) > 128 {
		e["group"] = "must be valid UTF-8 and at most 128 bytes"
	}
	if q.BillingMode != "" && q.BillingMode != "token" && q.BillingMode != "fixed" && q.BillingMode != "tiered_expr" {
		e["billing_mode"] = "invalid"
	}
	return nilIfEmpty(e)
}

func (q PricingCatalogQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return offset
}

type PricingCatalogItem struct {
	ID                     string   `json:"id"`
	SiteID                 string   `json:"site_id"`
	VendorID               string   `json:"vendor_id"`
	QuotaType              string   `json:"quota_type"`
	SiteName               string   `json:"site_name"`
	ModelName              string   `json:"model_name"`
	VendorName             string   `json:"vendor_name"`
	Description            string   `json:"description"`
	Icon                   string   `json:"icon"`
	Tags                   string   `json:"tags"`
	OwnerBy                string   `json:"owner_by"`
	ModelRatio             string   `json:"model_ratio"`
	ModelPrice             string   `json:"model_price"`
	CompletionRatio        string   `json:"completion_ratio"`
	CacheRatio             *string  `json:"cache_ratio"`
	CreateCacheRatio       *string  `json:"create_cache_ratio"`
	ImageRatio             *string  `json:"image_ratio"`
	AudioRatio             *string  `json:"audio_ratio"`
	AudioCompletionRatio   *string  `json:"audio_completion_ratio"`
	BillingMode            string   `json:"billing_mode"`
	BillingExpr            string   `json:"billing_expr"`
	PricingSource          string   `json:"pricing_source"`
	AbilityAvailable       bool     `json:"ability_available"`
	EnableGroups           []string `json:"enable_groups"`
	SupportedEndpointTypes []string `json:"supported_endpoint_types"`
	PricingVersion         string   `json:"pricing_version"`
	RemoteState            string   `json:"remote_state"`
	MissingCount           int      `json:"missing_count"`
	CollectedAt            int64    `json:"collected_at"`
	DataStatus             string   `json:"data_status"`
}

type PricingCatalogPageResponse struct {
	Items         []PricingCatalogItem          `json:"items"`
	Total         string                        `json:"total"`
	Page          int                           `json:"page"`
	PageSize      int                           `json:"page_size"`
	DataStatus    string                        `json:"data_status"`
	AsOf          *int64                        `json:"as_of"`
	SiteBreakdown []PricingCatalogSiteBreakdown `json:"site_breakdown"`
}

type PricingGroupItem struct {
	ID                  string            `json:"id"`
	SiteID              string            `json:"site_id"`
	SiteName            string            `json:"site_name"`
	Name                string            `json:"name"`
	Ratio               *string           `json:"ratio"`
	TopupRatio          *string           `json:"topup_ratio"`
	Description         string            `json:"description"`
	UserSelectable      bool              `json:"user_selectable"`
	DefaultUseAutoGroup bool              `json:"default_use_auto_group"`
	AutoPriority        *int              `json:"auto_priority"`
	OutgoingOverrides   map[string]string `json:"outgoing_overrides"`
	IncomingOverrides   map[string]string `json:"incoming_overrides"`
	VisibleToGroups     map[string]string `json:"visible_to_groups"`
	HiddenFromGroups    []string          `json:"hidden_from_groups"`
	RemoteState         string            `json:"remote_state"`
	MissingCount        int               `json:"missing_count"`
	ActivePricingCount  string            `json:"active_pricing_count"`
	MissingPricingCount string            `json:"missing_pricing_count"`
	ModelNames          []string          `json:"model_names"`
	MissingModelNames   []string          `json:"missing_model_names"`
	CollectedAt         int64             `json:"collected_at"`
	DataStatus          string            `json:"data_status"`
}

type PricingGroupPageResponse struct {
	Items         []PricingGroupItem            `json:"items"`
	Total         string                        `json:"total"`
	Page          int                           `json:"page"`
	PageSize      int                           `json:"page_size"`
	DataStatus    string                        `json:"data_status"`
	AsOf          *int64                        `json:"as_of"`
	SiteBreakdown []PricingCatalogSiteBreakdown `json:"site_breakdown"`
}

type PricingCatalogSiteBreakdown struct {
	SiteID     string `json:"site_id"`
	Total      string `json:"total"`
	Missing    string `json:"missing"`
	SiteName   string `json:"site_name"`
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}

type PricingCatalogStatistics struct {
	SiteCount      string                       `json:"site_count"`
	PricingActive  string                       `json:"pricing_active"`
	PricingMissing string                       `json:"pricing_missing"`
	GroupActive    string                       `json:"group_active"`
	GroupMissing   string                       `json:"group_missing"`
	DataStatus     string                       `json:"data_status"`
	Sites          []PricingCatalogSiteOverview `json:"sites"`
}
type PricingCatalogSiteOverview struct {
	SiteID            string `json:"site_id"`
	SiteName          string `json:"site_name"`
	PricingActive     string `json:"pricing_active"`
	PricingMissing    string `json:"pricing_missing"`
	GroupActive       string `json:"group_active"`
	GroupMissing      string `json:"group_missing"`
	PricingDataStatus string `json:"pricing_data_status"`
	GroupDataStatus   string `json:"group_data_status"`
	PricingAsOf       *int64 `json:"pricing_as_of"`
	GroupAsOf         *int64 `json:"group_as_of"`
}
