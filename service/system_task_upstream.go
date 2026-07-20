package service

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"new-api-pilot/dto"
)

var upstreamSystemTaskTypes = []string{"log_cleanup", "channel_test", "model_update", "midjourney_poll", "async_task_poll"}

type upstreamSystemTaskWire struct {
	ID        *int64          `json:"id"`
	TaskID    *string         `json:"task_id"`
	Type      *string         `json:"type"`
	Status    *string         `json:"status"`
	Payload   json.RawMessage `json:"payload"`
	State     json.RawMessage `json:"state"`
	Result    json.RawMessage `json:"result"`
	Error     *string         `json:"error"`
	CreatedAt *int64          `json:"created_at"`
	UpdatedAt *int64          `json:"updated_at"`
}

type upstreamNullableSystemTaskResponse struct {
	Task *upstreamSystemTaskWire
}

func (response *upstreamNullableSystemTaskResponse) decodeUpstreamResponse(payload []byte) error {
	var envelope struct {
		Success *bool           `json:"success"`
		Message *string         `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := validateStrictJSONFor(payload, envelope); err != nil || json.Unmarshal(payload, &envelope) != nil || envelope.Success == nil || !*envelope.Success || envelope.Message == nil || len(envelope.Data) == 0 {
		return invalidUpstreamResponse()
	}
	if bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		response.Task = nil
		return nil
	}
	if err := validateStrictJSONFor(envelope.Data, upstreamSystemTaskWire{}); err != nil {
		return invalidUpstreamResponse()
	}
	var task upstreamSystemTaskWire
	if err := json.Unmarshal(envelope.Data, &task); err != nil {
		return invalidUpstreamResponse()
	}
	response.Task = &task
	return nil
}

type upstreamSystemTaskProgressWire struct {
	Total     *int64 `json:"total"`
	Processed *int64 `json:"processed"`
	Progress  *int64 `json:"progress"`
	Remaining *int64 `json:"remaining"`
}
type upstreamLogCleanupPayloadWire struct {
	TargetTimestamp *int64 `json:"target_timestamp"`
	BatchSize       *int64 `json:"batch_size"`
}
type upstreamChannelTestPayloadWire struct {
	Mode   *string `json:"mode"`
	Notify *bool   `json:"notify"`
}
type upstreamModelUpdatePayloadWire struct {
	Manual *bool `json:"manual"`
}
type upstreamLogCleanupResultWire struct {
	DeletedCount *int64 `json:"deleted_count"`
}
type upstreamChannelTestResultWire struct {
	Tested    *int64 `json:"tested"`
	Succeeded *int64 `json:"succeeded"`
	Failed    *int64 `json:"failed"`
	Disabled  *int64 `json:"disabled"`
	Enabled   *int64 `json:"enabled"`
}
type upstreamModelUpdateResultWire struct {
	CheckedChannels      *int64 `json:"checked_channels"`
	ChangedChannels      *int64 `json:"changed_channels"`
	DetectedAddModels    *int64 `json:"detected_add_models"`
	DetectedRemoveModels *int64 `json:"detected_remove_models"`
	FailedChannels       *int64 `json:"failed_channels"`
	AutoAddedModels      *int64 `json:"auto_added_models"`
}
type upstreamMidjourneyResultWire struct {
	UnfinishedTasks *int64 `json:"unfinished_tasks"`
	ChannelsScanned *int64 `json:"channels_scanned"`
	NullTasksFailed *int64 `json:"null_tasks_failed"`
}
type upstreamAsyncPollResultWire struct {
	UnfinishedTasks  *int64 `json:"unfinished_tasks"`
	PlatformsScanned *int64 `json:"platforms_scanned"`
	NullTasksFailed  *int64 `json:"null_tasks_failed"`
}

func decodeOptionalSystemTaskObject(raw json.RawMessage, destination any) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	if err := validateStrictJSONFor(trimmed, destination); err != nil {
		return false
	}
	return json.Unmarshal(trimmed, destination) == nil
}

func nonnegativeSystemTaskValues(values ...*int64) bool {
	for _, value := range values {
		if value != nil && *value < 0 {
			return false
		}
	}
	return true
}

func validateUpstreamSystemTask(wire upstreamSystemTaskWire) (dto.UpstreamSystemTask, error) {
	if wire.ID == nil || wire.TaskID == nil || wire.Type == nil || wire.Status == nil || wire.Error == nil || wire.CreatedAt == nil || wire.UpdatedAt == nil || *wire.ID <= 0 || *wire.CreatedAt < 0 || *wire.UpdatedAt < *wire.CreatedAt || !validUpstreamString(*wire.TaskID, 1, 64) {
		return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
	}
	validType := false
	for _, value := range upstreamSystemTaskTypes {
		if *wire.Type == value {
			validType = true
			break
		}
	}
	if !validType || (*wire.Status != "pending" && *wire.Status != "running" && *wire.Status != "succeeded" && *wire.Status != "failed") {
		return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
	}
	out := dto.UpstreamSystemTask{ID: *wire.ID, TaskID: *wire.TaskID, Type: *wire.Type, Status: *wire.Status, CreatedAt: *wire.CreatedAt, UpdatedAt: *wire.UpdatedAt}
	out.ErrorPresent = *wire.Error != "" || out.Status == "failed"
	if out.ErrorPresent {
		out.ErrorCode = "UPSTREAM_SYSTEM_TASK_FAILED"
	}
	var progress upstreamSystemTaskProgressWire
	if !decodeOptionalSystemTaskObject(wire.State, &progress) || !nonnegativeSystemTaskValues(progress.Total, progress.Processed, progress.Progress, progress.Remaining) || progress.Progress != nil && *progress.Progress > 100 {
		return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
	}
	out.Total, out.Processed, out.Progress, out.Remaining = progress.Total, progress.Processed, progress.Progress, progress.Remaining
	switch out.Type {
	case "log_cleanup":
		var payload upstreamLogCleanupPayloadWire
		var result upstreamLogCleanupResultWire
		if !decodeOptionalSystemTaskObject(wire.Payload, &payload) || !decodeOptionalSystemTaskObject(wire.Result, &result) || !nonnegativeSystemTaskValues(payload.TargetTimestamp, payload.BatchSize, result.DeletedCount) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		out.DeletedCount = result.DeletedCount
	case "channel_test":
		var payload upstreamChannelTestPayloadWire
		var result upstreamChannelTestResultWire
		if !decodeOptionalSystemTaskObject(wire.Payload, &payload) || !decodeOptionalSystemTaskObject(wire.Result, &result) || !nonnegativeSystemTaskValues(result.Tested, result.Succeeded, result.Failed, result.Disabled, result.Enabled) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		if payload.Mode != nil && !validUpstreamString(*payload.Mode, 0, 32) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		out.Tested, out.Succeeded, out.Failed, out.Disabled, out.Enabled = result.Tested, result.Succeeded, result.Failed, result.Disabled, result.Enabled
	case "model_update":
		var payload upstreamModelUpdatePayloadWire
		var result upstreamModelUpdateResultWire
		if !decodeOptionalSystemTaskObject(wire.Payload, &payload) || !decodeOptionalSystemTaskObject(wire.Result, &result) || !nonnegativeSystemTaskValues(result.CheckedChannels, result.ChangedChannels, result.DetectedAddModels, result.DetectedRemoveModels, result.FailedChannels, result.AutoAddedModels) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		out.CheckedChannels, out.ChangedChannels, out.DetectedAddModels, out.DetectedRemoveModels, out.FailedChannels, out.AutoAddedModels = result.CheckedChannels, result.ChangedChannels, result.DetectedAddModels, result.DetectedRemoveModels, result.FailedChannels, result.AutoAddedModels
	case "midjourney_poll":
		var result upstreamMidjourneyResultWire
		if !decodeOptionalSystemTaskObject(wire.Payload, &struct{}{}) || !decodeOptionalSystemTaskObject(wire.Result, &result) || !nonnegativeSystemTaskValues(result.UnfinishedTasks, result.ChannelsScanned, result.NullTasksFailed) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		out.UnfinishedTasks, out.ChannelsScanned, out.NullTasksFailed = result.UnfinishedTasks, result.ChannelsScanned, result.NullTasksFailed
	case "async_task_poll":
		var result upstreamAsyncPollResultWire
		if !decodeOptionalSystemTaskObject(wire.Payload, &struct{}{}) || !decodeOptionalSystemTaskObject(wire.Result, &result) || !nonnegativeSystemTaskValues(result.UnfinishedTasks, result.PlatformsScanned, result.NullTasksFailed) {
			return dto.UpstreamSystemTask{}, invalidUpstreamResponse()
		}
		out.UnfinishedTasks, out.PlatformsScanned, out.NullTasksFailed = result.UnfinishedTasks, result.PlatformsScanned, result.NullTasksFailed
	}
	return out, nil
}

func (client *NewAPIClient) SnapshotSystemTasks(ctx context.Context, requestID string) (dto.UpstreamSystemTaskSnapshot, error) {
	var list []upstreamSystemTaskWire
	query := cloneURLValues(nil)
	query.Set("limit", "100")
	if _, err := client.get(ctx, client.httpClient, "/api/system-task/list", query, requestID+"_list", upstreamAuthManagement, client.requestTimeout, &list, false); err != nil {
		return dto.UpstreamSystemTaskSnapshot{}, err
	}
	if len(list) > 100 {
		return dto.UpstreamSystemTaskSnapshot{}, invalidUpstreamResponse()
	}
	result := dto.UpstreamSystemTaskSnapshot{Items: make([]dto.UpstreamSystemTask, 0, len(list)+5), Truncated: len(list) == 100}
	result.ListObservedCount = int64(len(list))
	seenIDs, seenTaskIDs := map[int64]struct{}{}, map[string]struct{}{}
	previous := int64(0)
	for index, wire := range list {
		item, err := validateUpstreamSystemTask(wire)
		if err != nil {
			return dto.UpstreamSystemTaskSnapshot{}, err
		}
		if _, ok := seenIDs[item.ID]; ok {
			return dto.UpstreamSystemTaskSnapshot{}, invalidUpstreamResponse()
		}
		if _, ok := seenTaskIDs[item.TaskID]; ok {
			return dto.UpstreamSystemTaskSnapshot{}, invalidUpstreamResponse()
		}
		if index > 0 {
			if item.ID >= previous {
				return dto.UpstreamSystemTaskSnapshot{}, invalidUpstreamResponse()
			}
			if previous-item.ID != 1 {
				result.IDGap = true
			}
		}
		seenIDs[item.ID], seenTaskIDs[item.TaskID], previous = struct{}{}, struct{}{}, item.ID
		result.Items = append(result.Items, item)
	}
	for _, taskType := range upstreamSystemTaskTypes {
		currentQuery := cloneURLValues(nil)
		currentQuery.Set("type", taskType)
		var current upstreamNullableSystemTaskResponse
		if _, err := client.get(ctx, client.httpClient, "/api/system-task/current", currentQuery, requestID+"_current_"+taskType, upstreamAuthManagement, client.requestTimeout, &current, false); err != nil {
			result.CurrentFailures = append(result.CurrentFailures, taskType)
			continue
		}
		if current.Task == nil {
			continue
		}
		item, err := validateUpstreamSystemTask(*current.Task)
		if err != nil || item.Type != taskType || item.Status != "pending" && item.Status != "running" {
			result.CurrentFailures = append(result.CurrentFailures, taskType)
			continue
		}
		if _, exists := seenTaskIDs[item.TaskID]; exists {
			continue
		}
		if _, exists := seenIDs[item.ID]; exists {
			return dto.UpstreamSystemTaskSnapshot{}, invalidUpstreamResponse()
		}
		seenIDs[item.ID], seenTaskIDs[item.TaskID] = struct{}{}, struct{}{}
		result.Items = append(result.Items, item)
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].ID > result.Items[j].ID })
	result.Partial = result.Truncated || result.IDGap || len(result.CurrentFailures) > 0
	return result, nil
}

func systemTaskInt64String(value *int64) *string {
	if value == nil {
		return nil
	}
	text := strconv.FormatInt(*value, 10)
	return &text
}
