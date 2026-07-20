package dto

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type ChannelInventoryQuery struct {
	Page, PageSize                       int
	SiteIDs                              []int64
	Keyword                              string
	Types, Statuses                      []int
	Groups, Tags, States                 []string
	MinBalance, MaxBalance               *string
	MinResponseTimeMS, MaxResponseTimeMS *int64
}

func (q *ChannelInventoryQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.Keyword = strings.TrimSpace(q.Keyword)
	q.Groups = uniqueStatisticsStrings(q.Groups)
	q.Tags = uniqueStatisticsStrings(q.Tags)
	q.States = normalizeEnumList(q.States)
}
func (q ChannelInventoryQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid pagination"
	}
	if len(q.SiteIDs) > 100 || len(q.Types) > 100 || len(q.Statuses) > 20 || len(q.Groups) > 100 || len(q.Tags) > 100 || len(q.States) > 2 {
		e["filters"] = "too many values"
	}
	if !utf8.ValidString(q.Keyword) || utf8.RuneCountInString(q.Keyword) > 255 {
		e["keyword"] = "is invalid"
	}
	if !validEnumList(q.States, "normal", "missing") {
		e["states"] = "contains an invalid state"
	}
	if q.MinResponseTimeMS != nil && q.MaxResponseTimeMS != nil && *q.MinResponseTimeMS > *q.MaxResponseTimeMS {
		e["response_time"] = "minimum must not exceed maximum"
	}
	for _, v := range []*string{q.MinBalance, q.MaxBalance} {
		if v != nil && !regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]{1,10})?$`).MatchString(*v) {
			e["balance"] = "must be a canonical decimal"
		}
	}
	return nilIfEmpty(e)
}
func (q ChannelInventoryQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type ChannelInventoryItem struct {
	ID               string `json:"id"`
	SiteID           string `json:"site_id"`
	SiteName         string `json:"site_name"`
	RemoteChannelID  string `json:"remote_channel_id"`
	Name             string `json:"name"`
	Type             int    `json:"type"`
	Status           int32  `json:"status"`
	TestTime         int64  `json:"test_time"`
	ResponseTimeMS   string `json:"response_time_ms"`
	Balance          string `json:"balance"`
	BalanceUpdatedAt int64  `json:"balance_updated_at"`
	Models           string `json:"models"`
	Group            string `json:"group"`
	UsedQuota        string `json:"used_quota"`
	Priority         string `json:"priority"`
	Weight           string `json:"weight"`
	AutoBan          int    `json:"auto_ban"`
	Tag              string `json:"tag"`
	RemoteState      string `json:"remote_state"`
	MissingCount     int    `json:"missing_count"`
	FirstSeenAt      int64  `json:"first_seen_at"`
	LastSeenAt       *int64 `json:"last_seen_at"`
}
type ChannelInventoryPage struct {
	Items      []ChannelInventoryItem `json:"items"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	DataStatus string                 `json:"data_status"`
	AsOf       *int64                 `json:"as_of"`
}

type ChannelInventoryStatisticsQuery struct {
	StartTimestamp, EndTimestamp int64
	SiteIDs                      []int64
	Types, Statuses              []int
	Groups, Tags                 []string
}

func (q *ChannelInventoryStatisticsQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.Groups = uniqueStatisticsStrings(q.Groups)
	q.Tags = uniqueStatisticsStrings(q.Tags)
}
func (q ChannelInventoryStatisticsQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.StartTimestamp <= 0 || q.EndTimestamp <= q.StartTimestamp || q.StartTimestamp%3600 != 0 || q.EndTimestamp%3600 != 0 || q.EndTimestamp-q.StartTimestamp > 366*24*3600 {
		e["range"] = "must be aligned and at most one year"
	}
	return nilIfEmpty(e)
}

type ChannelInventoryMetric struct {
	ChannelCount      string `json:"channel_count"`
	AvailableCount    string `json:"available_count"`
	UnavailableCount  string `json:"unavailable_count"`
	MissingCount      string `json:"missing_count"`
	BalanceTotal      string `json:"balance_total"`
	UsedQuota         string `json:"used_quota"`
	ResponseTimeAvgMS string `json:"response_time_avg_ms"`
	ResponseTimeMaxMS string `json:"response_time_max_ms"`
	AvailabilityRate  string `json:"availability_rate"`
}
type ChannelInventoryTrendPoint struct {
	BucketStart int64 `json:"bucket_start"`
	BucketEnd   int64 `json:"bucket_end"`
	ChannelInventoryMetric
	DataStatus string `json:"data_status"`
}
type ChannelInventoryBreakdown struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	SiteID        string `json:"site_id"`
	SiteName      string `json:"site_name"`
	ChannelInventoryMetric
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}
type ChannelInventoryStatisticsResponse struct {
	Summary         ChannelInventoryMetric       `json:"summary"`
	Trend           []ChannelInventoryTrendPoint `json:"trend"`
	TypeBreakdown   []ChannelInventoryBreakdown  `json:"type_breakdown"`
	StatusBreakdown []ChannelInventoryBreakdown  `json:"status_breakdown"`
	GroupBreakdown  []ChannelInventoryBreakdown  `json:"group_breakdown"`
	TagBreakdown    []ChannelInventoryBreakdown  `json:"tag_breakdown"`
	SiteBreakdown   []ChannelInventoryBreakdown  `json:"site_breakdown"`
	DataStatus      string                       `json:"data_status"`
}
