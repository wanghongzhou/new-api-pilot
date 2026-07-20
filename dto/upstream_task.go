package dto

import (
	"strings"
	"unicode/utf8"
)

type UpstreamTaskProperties struct {
	Model string `json:"model"`
}
type UpstreamTask struct {
	ID, CreatedAt, UpdatedAt          int64
	TaskID, Platform                  string
	UserID                            int64
	Group                             string
	ChannelID, Quota                  int64
	Action, Status                    string
	SubmitTime, StartTime, FinishTime int64
	Progress                          string
	Properties                        UpstreamTaskProperties
}
type UpstreamTaskPage struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int64
	Items    []UpstreamTask
}
type UpstreamTaskSnapshot struct{ Items []UpstreamTask }

type UpstreamTaskQuery struct {
	Page, PageSize                               int
	SiteIDs                                      []int64
	RemoteID, RemoteUserID, RemoteChannelID      *int64
	TaskID                                       string
	Platforms, Groups, Actions, Statuses, Models []string
	StartTimestamp, EndTimestamp                 int64
}

func (q *UpstreamTaskQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.TaskID = strings.TrimSpace(q.TaskID)
	q.Platforms = uniqueStatisticsStrings(q.Platforms)
	q.Groups = uniqueStatisticsStrings(q.Groups)
	q.Actions = uniqueStatisticsStrings(q.Actions)
	q.Statuses = normalizeEnumList(q.Statuses)
	q.Models = uniqueStatisticsStrings(q.Models)
}
func (q UpstreamTaskQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid pagination"
	}
	if len(q.SiteIDs) > 100 || len(q.Platforms) > 100 || len(q.Groups) > 100 || len(q.Actions) > 100 || len(q.Statuses) > 20 || len(q.Models) > 100 {
		e["filters"] = "too many values"
	}
	for _, status := range q.Statuses {
		switch status {
		case "NOT_START", "SUBMITTED", "QUEUED", "IN_PROGRESS", "FAILURE", "SUCCESS", "UNKNOWN":
		default:
			e["statuses"] = "invalid status"
		}
	}
	if q.RemoteID != nil && *q.RemoteID <= 0 || q.RemoteUserID != nil && *q.RemoteUserID < 0 || q.RemoteChannelID != nil && *q.RemoteChannelID < 0 {
		e["remote_id"] = "invalid remote id"
	}
	if !utf8.ValidString(q.TaskID) || len(q.TaskID) > 191 {
		e["task_id"] = "invalid"
	}
	if q.StartTimestamp < 0 || q.EndTimestamp < 0 || q.EndTimestamp > 0 && q.EndTimestamp <= q.StartTimestamp {
		e["range"] = "invalid"
	}
	return nilIfEmpty(e)
}
func (q UpstreamTaskQuery) Offset() int {
	o, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return o
}

type UpstreamTaskItem struct {
	ID          string                 `json:"id"`
	SiteID      string                 `json:"site_id"`
	SiteName    string                 `json:"site_name"`
	RemoteID    string                 `json:"remote_id"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
	TaskID      string                 `json:"task_id"`
	Platform    string                 `json:"platform"`
	UserID      string                 `json:"user_id"`
	Group       string                 `json:"group"`
	ChannelID   string                 `json:"channel_id"`
	Quota       string                 `json:"quota"`
	Action      string                 `json:"action"`
	Status      string                 `json:"status"`
	SubmitTime  int64                  `json:"submit_time"`
	StartTime   int64                  `json:"start_time"`
	FinishTime  int64                  `json:"finish_time"`
	Progress    string                 `json:"progress"`
	Properties  UpstreamTaskProperties `json:"properties"`
	FirstSeenAt int64                  `json:"first_seen_at"`
	LastSeenAt  int64                  `json:"last_seen_at"`
}
type UpstreamTaskPageResponse struct {
	Items      []UpstreamTaskItem `json:"items"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	DataStatus string             `json:"data_status"`
	AsOf       *int64             `json:"as_of"`
}
type UpstreamTaskMetric struct {
	Total           string  `json:"total"`
	Queued          string  `json:"queued"`
	Running         string  `json:"running"`
	Success         string  `json:"success"`
	Failure         string  `json:"failure"`
	SuccessRate     *string `json:"success_rate"`
	AvgQueueSeconds *string `json:"avg_queue_seconds"`
	AvgRunSeconds   *string `json:"avg_run_seconds"`
	AvgTotalSeconds *string `json:"avg_total_seconds"`
}
type UpstreamTaskBreakdown struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	SiteID        string `json:"site_id"`
	SiteName      string `json:"site_name"`
	UpstreamTaskMetric
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}
type UpstreamTaskStatisticsResponse struct {
	Summary           UpstreamTaskMetric      `json:"summary"`
	StatusBreakdown   []UpstreamTaskBreakdown `json:"status_breakdown"`
	PlatformBreakdown []UpstreamTaskBreakdown `json:"platform_breakdown"`
	ActionBreakdown   []UpstreamTaskBreakdown `json:"action_breakdown"`
	ModelBreakdown    []UpstreamTaskBreakdown `json:"model_breakdown"`
	SiteBreakdown     []UpstreamTaskBreakdown `json:"site_breakdown"`
	DataStatus        string                  `json:"data_status"`
}
