package dto

type UpstreamModelMeta struct {
	ID, VendorID, CreatedTime, UpdatedTime int64
	ModelName, Description, Icon, Tags     string
	Status, SyncOfficial, NameRule         int
}
type UpstreamModelMetaPage struct {
	Page, PageSize int
	Total          int64
	Items          []UpstreamModelMeta
}
type UpstreamModelMetaSnapshot struct {
	Total, MaxID int64
	Items        []UpstreamModelMeta
}

type ModelCatalogQuery struct {
	Page, PageSize int
	SiteIDs        []int64
	VendorID       *int64
	Statuses       []int
	SyncOfficial   []int
	Keyword        string
}

func (q *ModelCatalogQuery) Normalize() { q.SiteIDs = uniquePositiveIDs(q.SiteIDs) }
func (q ModelCatalogQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid pagination"
	}
	if len(q.SiteIDs) > 100 || len(q.Statuses) > 2 || len(q.SyncOfficial) > 2 || len(q.Keyword) > 128 {
		e["filters"] = "invalid filters"
	}
	if q.VendorID != nil && *q.VendorID < 0 {
		e["vendor_id"] = "invalid"
	}
	for _, value := range append(append([]int{}, q.Statuses...), q.SyncOfficial...) {
		if value != 0 && value != 1 {
			e["statuses"] = "must be 0 or 1"
		}
	}
	return nilIfEmpty(e)
}
func (q ModelCatalogQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type ModelCatalogItem struct {
	ID              string `json:"id"`
	SiteID          string `json:"site_id"`
	RemoteID        string `json:"remote_id"`
	SiteName        string `json:"site_name"`
	ModelName       string `json:"model_name"`
	Description     string `json:"description"`
	Icon            string `json:"icon"`
	Tags            string `json:"tags"`
	VendorID        string `json:"vendor_id"`
	Status          int    `json:"status"`
	SyncOfficial    int    `json:"sync_official"`
	NameRule        int    `json:"name_rule"`
	CreatedTime     int64  `json:"created_time"`
	UpdatedTime     int64  `json:"updated_time"`
	CoveredChannels string `json:"covered_channels"`
	CoveredGroups   string `json:"covered_groups"`
	DataStatus      string `json:"data_status"`
}
type ModelCatalogPageResponse struct {
	Items      []ModelCatalogItem `json:"items"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	DataStatus string             `json:"data_status"`
}
type ModelCoverageBreakdown struct {
	DimensionID        string `json:"dimension_id"`
	DimensionName      string `json:"dimension_name"`
	SiteID             string `json:"site_id"`
	SiteName           string `json:"site_name"`
	CatalogModels      string `json:"catalog_models"`
	ExactCoveredModels string `json:"exact_covered_models"`
	ExactMissingModels string `json:"exact_missing_models"`
	ChannelMappings    string `json:"channel_mappings"`
	DataStatus         string `json:"data_status"`
	AsOf               *int64 `json:"as_of"`
}
type ModelCoverageResponse struct {
	CatalogModels      string                   `json:"catalog_models"`
	ExactCoveredModels string                   `json:"exact_covered_models"`
	ExactMissingModels string                   `json:"exact_missing_models"`
	ChannelMappings    string                   `json:"channel_mappings"`
	DataStatus         string                   `json:"data_status"`
	SiteBreakdown      []ModelCoverageBreakdown `json:"site_breakdown"`
	VendorBreakdown    []ModelCoverageBreakdown `json:"vendor_breakdown"`
	StatusBreakdown    []ModelCoverageBreakdown `json:"status_breakdown"`
}
type MissingModelItem struct {
	SiteID          string `json:"site_id"`
	SiteName        string `json:"site_name"`
	RemoteChannelID string `json:"remote_channel_id"`
	ChannelName     string `json:"channel_name"`
	ModelName       string `json:"model_name"`
	Group           string `json:"group"`
	DataStatus      string `json:"data_status"`
	AsOf            *int64 `json:"as_of"`
}
type MissingModelPageResponse struct {
	Items      []MissingModelItem `json:"items"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	DataStatus string             `json:"data_status"`
	AsOf       *int64             `json:"as_of"`
}
