package dto

type LocalRankingQuery struct {
	Period  string
	SiteIDs []int64
}

func (q *LocalRankingQuery) Normalize() { q.SiteIDs = uniquePositiveIDs(q.SiteIDs) }
func (q LocalRankingQuery) Validate() map[string]string {
	e := map[string]string{}
	switch q.Period {
	case "today", "week", "month", "year":
	default:
		e["period"] = "invalid"
	}
	if len(q.SiteIDs) > 100 {
		e["site_ids"] = "too many"
	}
	return nilIfEmpty(e)
}

type RankingHistoryPoint struct {
	DimensionID string `json:"dimension_id"`
	BucketStart int64  `json:"bucket_start"`
	TokenUsed   string `json:"token_used"`
}
type RankingSiteBreakdown struct {
	DimensionID string `json:"dimension_id"`
	SiteID      string `json:"site_id"`
	SiteName    string `json:"site_name"`
	TokenUsed   string `json:"token_used"`
	DataStatus  string `json:"data_status"`
	AsOf        *int64 `json:"as_of"`
}
type LocalRankingItem struct {
	DimensionID   string  `json:"dimension_id"`
	DimensionName string  `json:"dimension_name"`
	TokenUsed     string  `json:"token_used"`
	RequestCount  string  `json:"request_count"`
	Quota         string  `json:"quota"`
	Share         string  `json:"share"`
	Growth        *string `json:"growth"`
	Rank          int     `json:"rank"`
}
type LocalRankingResponse struct {
	Period         string                 `json:"period"`
	StartTimestamp int64                  `json:"start_timestamp"`
	EndTimestamp   int64                  `json:"end_timestamp"`
	Items          []LocalRankingItem     `json:"items"`
	Movers         []LocalRankingItem     `json:"movers"`
	Droppers       []LocalRankingItem     `json:"droppers"`
	History        []RankingHistoryPoint  `json:"history"`
	SiteBreakdown  []RankingSiteBreakdown `json:"site_breakdown"`
	DataStatus     string                 `json:"data_status"`
	AsOf           *int64                 `json:"as_of"`
}
