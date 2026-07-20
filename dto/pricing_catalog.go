package dto

type UpstreamPricingItem struct {
	ModelName, VendorKey, Description, Icon, Tags, OwnerBy, ModelRatio, ModelPrice, CompletionRatio string
	CacheRatio, CreateCacheRatio, ImageRatio, AudioRatio, AudioCompletionRatio                      *string
	VendorID, QuotaType                                                                             int64
	RootVisible                                                                                     bool
	EnableGroups, SupportedEndpointTypes                                                            []string
}

type UpstreamPricingGroup struct {
	Name, Description string
	Ratio             *string
	RootVisible       bool
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

type PricingCatalogQuery struct {
	Page, PageSize int
	SiteIDs        []int64
	States         []string
	Keyword        string
	Group          string
}

func (q *PricingCatalogQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.States = normalizeEnumList(q.States)
}

func (q PricingCatalogQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid"
	}
	for _, state := range q.States {
		if state != "normal" && state != "missing" {
			e["states"] = "invalid"
		}
	}
	if len(q.Keyword) > 255 || len(q.Group) > 128 {
		e["keyword"] = "invalid"
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
	VendorKey              string   `json:"vendor_key"`
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
	EnableGroups           []string `json:"enable_groups"`
	SupportedEndpointTypes []string `json:"supported_endpoint_types"`
	PricingVersion         string   `json:"pricing_version"`
	RootVisible            bool     `json:"root_visible"`
	RemoteState            string   `json:"remote_state"`
	MissingCount           int      `json:"missing_count"`
	CollectedAt            int64    `json:"collected_at"`
	DataStatus             string   `json:"data_status"`
}

type PricingCatalogPageResponse struct {
	Items         []PricingCatalogItem          `json:"items"`
	Total         int64                         `json:"total"`
	Page          int                           `json:"page"`
	PageSize      int                           `json:"page_size"`
	DataStatus    string                        `json:"data_status"`
	AsOf          *int64                        `json:"as_of"`
	SiteBreakdown []PricingCatalogSiteBreakdown `json:"site_breakdown"`
}

type PricingGroupItem struct {
	ID           string  `json:"id"`
	SiteID       string  `json:"site_id"`
	SiteName     string  `json:"site_name"`
	Name         string  `json:"name"`
	Ratio        *string `json:"ratio"`
	Description  string  `json:"description"`
	RootVisible  bool    `json:"root_visible"`
	RemoteState  string  `json:"remote_state"`
	MissingCount int     `json:"missing_count"`
	CollectedAt  int64   `json:"collected_at"`
	DataStatus   string  `json:"data_status"`
}

type PricingGroupPageResponse struct {
	Items         []PricingGroupItem            `json:"items"`
	Total         int64                         `json:"total"`
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
	Total                 string                              `json:"total"`
	Missing               string                              `json:"missing"`
	GroupTotal            string                              `json:"group_total"`
	DataStatus            string                              `json:"data_status"`
	SiteBreakdown         []PricingCatalogSiteBreakdown       `json:"site_breakdown"`
	VendorBreakdown       []PricingVendorBreakdown            `json:"vendor_breakdown"`
	GroupBreakdown        []PricingModelGroupBreakdown        `json:"group_breakdown"`
	GroupCatalogBreakdown []GroupCatalogAvailabilityBreakdown `json:"group_catalog_breakdown"`
}
type PricingVendorBreakdown struct {
	VendorKey string `json:"vendor_key"`
	VendorID  string `json:"vendor_id"`
	Total     string `json:"total"`
	Missing   string `json:"missing"`
}
type PricingModelGroupBreakdown struct {
	GroupName  string `json:"group_name"`
	ModelCount string `json:"model_count"`
}
type GroupCatalogAvailabilityBreakdown struct {
	RootVisible    bool   `json:"root_visible"`
	RatioAvailable bool   `json:"ratio_available"`
	Count          string `json:"count"`
}
