package dto

type FastTaskHistoryItem struct {
	SiteID string `json:"site_id"`
	TaskType string `json:"task_type"`
	StartedAt int64 `json:"started_at"`
	FinishedAt int64 `json:"finished_at"`
	Status string `json:"status"`
	DurationMS int64 `json:"duration_ms"`
	Error string `json:"error,omitempty"`
	RequestID string `json:"request_id"`
}
