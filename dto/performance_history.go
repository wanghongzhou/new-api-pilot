package dto

import (
	"strings"
	"unicode/utf8"
)

type PerformanceHistoryQuery struct {
	Page, PageSize               int
	StartTimestamp, EndTimestamp int64
	SiteIDs                      []int64
	ModelNames, Groups           []string
	SnapshotAt                   int64
}

func (q *PerformanceHistoryQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.ModelNames = uniqueStatisticsStrings(q.ModelNames)
	q.Groups = uniqueStatisticsStrings(q.Groups)
}
func (q PerformanceHistoryQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid pagination"
	}
	if q.StartTimestamp <= 0 || q.EndTimestamp <= q.StartTimestamp || q.EndTimestamp-q.StartTimestamp > 366*24*3600 || q.StartTimestamp%3600 != 0 || q.EndTimestamp%3600 != 0 {
		e["range"] = "invalid range"
	}
	if len(q.SiteIDs) > 100 || len(q.ModelNames) > 100 || len(q.Groups) > 100 {
		e["filters"] = "too many values"
	}
	for _, v := range append(append([]string{}, q.ModelNames...), q.Groups...) {
		if !utf8.ValidString(v) || len(strings.TrimSpace(v)) > 255 {
			e["filters"] = "invalid text"
		}
	}
	if q.SnapshotAt < 0 {
		e["snapshot_at"] = "invalid snapshot"
	}
	return nilIfEmpty(e)
}
func (q PerformanceHistoryQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type PerformanceCounterSet struct {
	RequestCount   *string `json:"request_count"`
	SuccessCount   *string `json:"success_count"`
	TotalLatencyMS *string `json:"total_latency_ms"`
	TTFTSumMS      *string `json:"ttft_sum_ms"`
	TTFTCount      *string `json:"ttft_count"`
	OutputTokens   *string `json:"output_tokens"`
	GenerationMS   *string `json:"generation_ms"`
}
type PerformanceHistoryItem struct {
	ID           string `json:"id"`
	SiteID       string `json:"site_id"`
	SiteName     string `json:"site_name"`
	ModelName    string `json:"model_name"`
	Group        string `json:"group"`
	BucketStart  int64  `json:"bucket_start"`
	SeriesSchema string `json:"series_schema"`
	MetricSource string `json:"metric_source"`
	AvgTTFTMS    string `json:"avg_ttft_ms"`
	AvgLatencyMS string `json:"avg_latency_ms"`
	SuccessRate  string `json:"success_rate"`
	AvgTPS       string `json:"avg_tps"`
	PerformanceCounterSet
	CollectedAt int64 `json:"collected_at"`
}
type PerformanceHistoryPage struct {
	Items        []PerformanceHistoryItem `json:"items"`
	Total        string                   `json:"total"`
	Page         int                      `json:"page"`
	PageSize     int                      `json:"page_size"`
	DataStatus   string                   `json:"data_status"`
	AsOf         *int64                   `json:"as_of"`
	Completeness PerformanceCompleteness  `json:"completeness"`
}
type PerformanceWeightedMetric struct {
	SuccessRate  *string `json:"success_rate"`
	AvgLatencyMS *string `json:"avg_latency_ms"`
	AvgTTFTMS    *string `json:"avg_ttft_ms"`
	AvgTPS       *string `json:"avg_tps"`
	RequestCount *string `json:"request_count"`
}
type PerformanceDimensionBreakdown struct {
	Dimension string `json:"dimension"`
	PerformanceWeightedMetric
}
type PerformanceHistoryStatisticsResponse struct {
	Summary           PerformanceWeightedMetric       `json:"summary"`
	Trend             []PerformanceHistoryItem        `json:"trend"`
	ModelBreakdown    []PerformanceDimensionBreakdown `json:"model_breakdown"`
	GroupBreakdown    []PerformanceDimensionBreakdown `json:"group_breakdown"`
	SiteBreakdown     []PerformanceHistoryItem        `json:"site_breakdown"`
	AggregationStatus string                          `json:"aggregation_status"`
	DataStatus        string                          `json:"data_status"`
	AsOf              *int64                          `json:"as_of"`
	Completeness      PerformanceCompleteness         `json:"completeness"`
	UnavailableReason string                          `json:"unavailable_reason,omitempty"`
}

type PerformanceCompleteness struct {
	DataStatus           string `json:"data_status"`
	SuccessfulSiteCount  int64  `json:"successful_site_count"`
	UnavailableSiteCount int64  `json:"unavailable_site_count"`
	ExpectedSiteCount    int64  `json:"expected_site_count"`
}
