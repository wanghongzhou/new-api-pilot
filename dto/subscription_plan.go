package dto

type UpstreamSubscriptionPlan struct {
	ID                                                   int64
	Title, Subtitle, PriceAmount, Currency, DurationUnit string
	DurationValue                                        int
	CustomSeconds                                        int64
	Enabled                                              bool
	SortOrder                                            int
	TotalAmount                                          int64
	QuotaResetPeriod                                     string
	QuotaResetCustomSeconds, CreatedAt, UpdatedAt        int64
}
type UpstreamSubscriptionPlanSnapshot struct{ Items []UpstreamSubscriptionPlan }
type SubscriptionPlanQuery struct {
	Page, PageSize int
	SiteIDs        []int64
	States         []string
	Enabled        *bool
	Keyword        string
}

func (q *SubscriptionPlanQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.States = normalizeEnumList(q.States)
}
func (q SubscriptionPlanQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid"
	}
	for _, s := range q.States {
		if s != "normal" && s != "missing" {
			e["states"] = "invalid"
		}
	}
	return nilIfEmpty(e)
}
func (q SubscriptionPlanQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type SubscriptionPlanItem struct {
	ID                      string `json:"id"`
	SiteID                  string `json:"site_id"`
	RemoteID                string `json:"remote_id"`
	SiteName                string `json:"site_name"`
	Title                   string `json:"title"`
	Subtitle                string `json:"subtitle"`
	PriceAmount             string `json:"price_amount"`
	Currency                string `json:"currency"`
	DurationUnit            string `json:"duration_unit"`
	DurationValue           int    `json:"duration_value"`
	CustomSeconds           string `json:"custom_seconds"`
	Enabled                 bool   `json:"enabled"`
	SortOrder               int    `json:"sort_order"`
	TotalAmount             string `json:"total_amount"`
	QuotaResetPeriod        string `json:"quota_reset_period"`
	QuotaResetCustomSeconds string `json:"quota_reset_custom_seconds"`
	CreatedAt               int64  `json:"created_at"`
	UpdatedAt               int64  `json:"updated_at"`
	RemoteState             string `json:"remote_state"`
	MissingCount            int    `json:"missing_count"`
	DataStatus              string `json:"data_status"`
}
type SubscriptionPlanPageResponse struct {
	Items      []SubscriptionPlanItem `json:"items"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	DataStatus string                 `json:"data_status"`
}
type SubscriptionPlanBreakdown struct {
	SiteID     string `json:"site_id"`
	SiteName   string `json:"site_name"`
	Total      string `json:"total"`
	Enabled    string `json:"enabled"`
	Disabled   string `json:"disabled"`
	Missing    string `json:"missing"`
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}
type SubscriptionPlanStatistics struct {
	Total         string                      `json:"total"`
	Enabled       string                      `json:"enabled"`
	Disabled      string                      `json:"disabled"`
	Missing       string                      `json:"missing"`
	DataStatus    string                      `json:"data_status"`
	SiteBreakdown []SubscriptionPlanBreakdown `json:"site_breakdown"`
}
