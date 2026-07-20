package dto

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	LogCollectionPending     = "pending"
	LogCollectionComplete    = "complete"
	LogCollectionPartial     = "partial"
	LogCollectionUnavailable = "unavailable"
	LogCollectionDisabled    = "disabled"
)

type UpstreamLogRow struct {
	ID                int64
	UserID            int64
	CreatedAt         int64
	Type              int
	Content           string
	Username          string
	TokenName         string
	ModelName         string
	Quota             int64
	PromptTokens      int64
	CompletionTokens  int64
	UseTimeSeconds    int64
	IsStream          bool
	ChannelID         int64
	TokenID           int64
	UseGroup          string
	IP                string
	RequestID         string
	UpstreamRequestID string
}

type UpstreamLogPage struct {
	Page     int
	PageSize int
	Total    int64
	Items    []UpstreamLogRow
}

type LogQuery struct {
	Page              int
	PageSize          int
	SiteIDs           []int64
	Type              *int
	StartTimestamp    int64
	EndTimestamp      int64
	Username          string
	ModelName         string
	TokenName         string
	ChannelID         *int64
	UseGroup          string
	RequestID         string
	UpstreamRequestID string
}

func (query *LogQuery) Normalize() {
	query.SiteIDs = uniquePositiveIDs(query.SiteIDs)
	query.Username = strings.TrimSpace(query.Username)
	query.ModelName = strings.TrimSpace(query.ModelName)
	query.TokenName = strings.TrimSpace(query.TokenName)
	query.UseGroup = strings.TrimSpace(query.UseGroup)
	query.RequestID = strings.TrimSpace(query.RequestID)
	query.UpstreamRequestID = strings.TrimSpace(query.UpstreamRequestID)
}

func (query LogQuery) Validate() map[string]string {
	errors := map[string]string{}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 || !statisticsPaginationValid(query.Page, query.PageSize) {
		errors["p"] = "invalid pagination"
	}
	if query.StartTimestamp <= 0 || query.EndTimestamp <= query.StartTimestamp || query.EndTimestamp-query.StartTimestamp > 31*24*3600 {
		errors["range"] = "must be a positive range of at most 31 days"
	}
	if len(query.SiteIDs) > 100 {
		errors["site_ids"] = "must contain at most 100 IDs"
	}
	if query.Type != nil && (*query.Type < 0 || *query.Type > 7) {
		errors["type"] = "must be between 0 and 7"
	}
	if query.ChannelID != nil && *query.ChannelID < 0 {
		errors["channel_id"] = "must be non-negative"
	}
	for field, value := range map[string]string{
		"username": query.Username, "model_name": query.ModelName, "token_name": query.TokenName,
		"group": query.UseGroup, "request_id": query.RequestID, "upstream_request_id": query.UpstreamRequestID,
	} {
		limit := 255
		if field == "group" || field == "upstream_request_id" {
			limit = 128
		} else if field == "request_id" {
			limit = 64
		}
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
			errors[field] = "is too long or invalid UTF-8"
		}
	}
	return nilIfEmpty(errors)
}

func (query LogQuery) Offset() int {
	offset, _ := statisticsPaginationOffset(query.Page, query.PageSize)
	return offset
}

type LogItem struct {
	ID                string `json:"id"`
	SiteID            string `json:"site_id"`
	SiteName          string `json:"site_name"`
	CreatedAt         int64  `json:"created_at"`
	Type              int    `json:"type"`
	RemoteUserID      string `json:"remote_user_id"`
	Username          string `json:"username"`
	ModelName         string `json:"model_name"`
	TokenID           string `json:"token_id"`
	TokenName         string `json:"token_name"`
	ChannelID         string `json:"channel_id"`
	UseGroup          string `json:"group"`
	RequestID         string `json:"request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	Quota             string `json:"quota"`
	PromptTokens      string `json:"prompt_tokens"`
	CompletionTokens  string `json:"completion_tokens"`
	UseTimeSeconds    string `json:"use_time_seconds"`
	IsStream          bool   `json:"is_stream"`
	Content           string `json:"content"`
	IP                string `json:"ip"`
}

type LogResponse struct {
	Items      []LogItem `json:"items"`
	Total      int64     `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	DataStatus string    `json:"data_status"`
	AsOf       *int64    `json:"as_of"`
}

func Int64String(value int64) string { return strconv.FormatInt(value, 10) }
