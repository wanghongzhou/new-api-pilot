package dto

type UpstreamSystemTask struct {
	ID, CreatedAt, UpdatedAt                                                  int64
	TaskID, Type, Status                                                      string
	ErrorPresent                                                              bool
	ErrorCode                                                                 string
	Total, Processed, Progress, Remaining                                     *int64
	DeletedCount                                                              *int64
	Tested, Succeeded, Failed, Disabled, Enabled                              *int64
	CheckedChannels, ChangedChannels, DetectedAddModels, DetectedRemoveModels *int64
	FailedChannels, AutoAddedModels                                           *int64
	UnfinishedTasks, ChannelsScanned, PlatformsScanned, NullTasksFailed       *int64
}

type UpstreamSystemTaskSnapshot struct {
	Items             []UpstreamSystemTask
	Partial           bool
	Truncated         bool
	IDGap             bool
	ListObservedCount int64
	CurrentFailures   []string
}

type SystemTaskQuery struct {
	Page, PageSize           int
	SiteIDs                  []int64
	Types, Statuses          []string
	ErrorPresent             *bool
	CreatedStart, CreatedEnd int64
}

func (q *SystemTaskQuery) Normalize() {
	q.SiteIDs = uniquePositiveIDs(q.SiteIDs)
	q.Types = normalizeEnumList(q.Types)
	q.Statuses = normalizeEnumList(q.Statuses)
}

func (q SystemTaskQuery) Validate() map[string]string {
	e := map[string]string{}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || !statisticsPaginationValid(q.Page, q.PageSize) {
		e["p"] = "invalid"
	}
	allowedTypes := map[string]bool{"log_cleanup": true, "channel_test": true, "model_update": true, "midjourney_poll": true, "async_task_poll": true}
	for _, value := range q.Types {
		if !allowedTypes[value] {
			e["types"] = "invalid"
		}
	}
	allowedStatuses := map[string]bool{"pending": true, "running": true, "succeeded": true, "failed": true}
	for _, value := range q.Statuses {
		if !allowedStatuses[value] {
			e["statuses"] = "invalid"
		}
	}
	if q.CreatedStart < 0 || q.CreatedEnd < 0 || q.CreatedEnd > 0 && q.CreatedStart >= q.CreatedEnd {
		e["created_range"] = "invalid"
	}
	return nilIfEmpty(e)
}

func (q SystemTaskQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(q.Page, q.PageSize)
	return offset
}

type SystemTaskProgress struct {
	Total     *string `json:"total"`
	Processed *string `json:"processed"`
	Progress  *int    `json:"progress"`
	Remaining *string `json:"remaining"`
}

type SystemTaskResult struct {
	DeletedCount         *string `json:"deleted_count"`
	Tested               *string `json:"tested"`
	Succeeded            *string `json:"succeeded"`
	Failed               *string `json:"failed"`
	Disabled             *string `json:"disabled"`
	Enabled              *string `json:"enabled"`
	CheckedChannels      *string `json:"checked_channels"`
	ChangedChannels      *string `json:"changed_channels"`
	DetectedAddModels    *string `json:"detected_add_models"`
	DetectedRemoveModels *string `json:"detected_remove_models"`
	FailedChannels       *string `json:"failed_channels"`
	AutoAddedModels      *string `json:"auto_added_models"`
	UnfinishedTasks      *string `json:"unfinished_tasks"`
	ChannelsScanned      *string `json:"channels_scanned"`
	PlatformsScanned     *string `json:"platforms_scanned"`
	NullTasksFailed      *string `json:"null_tasks_failed"`
}

type SystemTaskItem struct {
	ID              string              `json:"id"`
	SiteID          string              `json:"site_id"`
	RemoteID        string              `json:"remote_id"`
	SiteName        string              `json:"site_name"`
	TaskID          string              `json:"task_id"`
	Type            string              `json:"type"`
	Status          string              `json:"status"`
	ErrorPresent    bool                `json:"error_present"`
	ErrorCode       string              `json:"error_code"`
	Progress        *SystemTaskProgress `json:"progress"`
	Result          *SystemTaskResult   `json:"result"`
	RemoteCreatedAt int64               `json:"remote_created_at"`
	RemoteUpdatedAt int64               `json:"remote_updated_at"`
	CollectedAt     int64               `json:"collected_at"`
	DataStatus      string              `json:"data_status"`
}

type SystemTaskPageResponse struct {
	Items            []SystemTaskItem `json:"items"`
	Total            string           `json:"total"`
	Page             int              `json:"page"`
	PageSize         int              `json:"page_size"`
	DataStatus       string           `json:"data_status"`
	AsOf             *int64           `json:"as_of"`
	Truncated        bool             `json:"truncated"`
	TruncationReason *string          `json:"truncation_reason"`
	SourceLimit      string           `json:"source_limit"`
	ObservedCount    string           `json:"observed_count"`
}

type SystemTaskMetric struct {
	Total        string `json:"total"`
	Active       string `json:"active"`
	Succeeded    string `json:"succeeded"`
	Failed       string `json:"failed"`
	ErrorPresent string `json:"error_present"`
}

type SystemTaskBreakdown struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	SiteID        string `json:"site_id"`
	SiteName      string `json:"site_name"`
	SystemTaskMetric
	DataStatus string `json:"data_status"`
	AsOf       *int64 `json:"as_of"`
}

type SystemTaskStatisticsResponse struct {
	Summary          SystemTaskMetric      `json:"summary"`
	TypeBreakdown    []SystemTaskBreakdown `json:"type_breakdown"`
	StatusBreakdown  []SystemTaskBreakdown `json:"status_breakdown"`
	SiteBreakdown    []SystemTaskBreakdown `json:"site_breakdown"`
	DataStatus       string                `json:"data_status"`
	AsOf             *int64                `json:"as_of"`
	Truncated        bool                  `json:"truncated"`
	TruncationReason *string               `json:"truncation_reason"`
	SourceLimit      string                `json:"source_limit"`
	ObservedCount    string                `json:"observed_count"`
}
