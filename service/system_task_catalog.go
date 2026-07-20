package service

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type SystemTaskCatalogService struct{ database *gorm.DB }

func NewSystemTaskCatalogService(db *gorm.DB) (*SystemTaskCatalogService, error) {
	if db == nil {
		return nil, errors.New("system task catalog database is required")
	}
	return &SystemTaskCatalogService{database: db}, nil
}
func (s *SystemTaskCatalogService) readSnapshot(ctx context.Context, fn func(*model.SystemTaskRepository) error) error {
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(model.NewSystemTaskRepository(tx)) })
}

func systemTaskProgress(row model.SiteSystemTask) *dto.SystemTaskProgress {
	if row.Total == nil && row.Processed == nil && row.Progress == nil && row.Remaining == nil {
		return nil
	}
	var progress *int
	if row.Progress != nil {
		value := int(*row.Progress)
		progress = &value
	}
	return &dto.SystemTaskProgress{Total: systemTaskInt64String(row.Total), Processed: systemTaskInt64String(row.Processed), Progress: progress, Remaining: systemTaskInt64String(row.Remaining)}
}
func systemTaskResult(row model.SiteSystemTask) *dto.SystemTaskResult {
	if row.DeletedCount == nil && row.Tested == nil && row.Succeeded == nil && row.Failed == nil && row.Disabled == nil && row.Enabled == nil && row.CheckedChannels == nil && row.ChangedChannels == nil && row.DetectedAddModels == nil && row.DetectedRemoveModels == nil && row.FailedChannels == nil && row.AutoAddedModels == nil && row.UnfinishedTasks == nil && row.ChannelsScanned == nil && row.PlatformsScanned == nil && row.NullTasksFailed == nil {
		return nil
	}
	return &dto.SystemTaskResult{DeletedCount: systemTaskInt64String(row.DeletedCount), Tested: systemTaskInt64String(row.Tested), Succeeded: systemTaskInt64String(row.Succeeded), Failed: systemTaskInt64String(row.Failed), Disabled: systemTaskInt64String(row.Disabled), Enabled: systemTaskInt64String(row.Enabled), CheckedChannels: systemTaskInt64String(row.CheckedChannels), ChangedChannels: systemTaskInt64String(row.ChangedChannels), DetectedAddModels: systemTaskInt64String(row.DetectedAddModels), DetectedRemoveModels: systemTaskInt64String(row.DetectedRemoveModels), FailedChannels: systemTaskInt64String(row.FailedChannels), AutoAddedModels: systemTaskInt64String(row.AutoAddedModels), UnfinishedTasks: systemTaskInt64String(row.UnfinishedTasks), ChannelsScanned: systemTaskInt64String(row.ChannelsScanned), PlatformsScanned: systemTaskInt64String(row.PlatformsScanned), NullTasksFailed: systemTaskInt64String(row.NullTasksFailed)}
}
func systemTaskItem(row model.SystemTaskReadRow, status string) dto.SystemTaskItem {
	return dto.SystemTaskItem{ID: strconv.FormatInt(row.ID, 10), SiteID: strconv.FormatInt(row.SiteID, 10), RemoteID: strconv.FormatInt(row.RemoteID, 10), SiteName: row.SiteName, TaskID: row.RemoteTaskID, Type: row.TaskType, Status: row.RemoteStatus, ErrorPresent: row.ErrorPresent, ErrorCode: row.ErrorCode, Progress: systemTaskProgress(row.SiteSystemTask), Result: systemTaskResult(row.SiteSystemTask), RemoteCreatedAt: row.RemoteCreatedAt, RemoteUpdatedAt: row.RemoteUpdatedAt, CollectedAt: row.CollectedAt, DataStatus: status}
}

func (s *SystemTaskCatalogService) List(ctx context.Context, q dto.SystemTaskQuery) (dto.SystemTaskPageResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.SystemTaskPageResponse{}, ErrStatisticsInvalid
	}
	var rows []model.SystemTaskReadRow
	var total int64
	var statuses map[int64]model.SystemTaskSiteCollectionState
	var overall string
	var asOf *int64
	err := s.readSnapshot(ctx, func(r *model.SystemTaskRepository) error {
		var e error
		statuses, overall, asOf, e = r.CollectionStatuses(ctx, q.SiteIDs)
		if e != nil {
			return e
		}
		rows, total, e = r.List(ctx, q)
		return e
	})
	if err != nil {
		return dto.SystemTaskPageResponse{}, err
	}
	items := make([]dto.SystemTaskItem, 0, len(rows))
	for _, row := range rows {
		status := statuses[row.SiteID].DataStatus
		if status == "" {
			status = "pending"
		}
		items = append(items, systemTaskItem(row, status))
	}
	truncated, reason, observed := systemTaskCompleteness(statuses)
	return dto.SystemTaskPageResponse{Items: items, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, DataStatus: overall, AsOf: asOf, Truncated: truncated, TruncationReason: reason, SourceLimit: "100", ObservedCount: strconv.FormatInt(observed, 10)}, nil
}
func systemTaskMetric(row model.SystemTaskMetricRow) dto.SystemTaskMetric {
	return dto.SystemTaskMetric{Total: strconv.FormatInt(row.Total, 10), Active: strconv.FormatInt(row.Active, 10), Succeeded: strconv.FormatInt(row.Succeeded, 10), Failed: strconv.FormatInt(row.Failed, 10), ErrorPresent: strconv.FormatInt(row.ErrorPresent, 10)}
}
func systemTaskBreakdowns(rows []model.SystemTaskMetricRow, statuses map[int64]model.SystemTaskSiteCollectionState, overall string) []dto.SystemTaskBreakdown {
	out := make([]dto.SystemTaskBreakdown, 0, len(rows))
	for _, row := range rows {
		status := overall
		if row.SiteID > 0 {
			status = statuses[row.SiteID].DataStatus
			if status == "" {
				status = "pending"
			}
		}
		out = append(out, dto.SystemTaskBreakdown{DimensionID: row.DimensionID, DimensionName: row.DimensionName, SiteID: func() string {
			if row.SiteID == 0 {
				return ""
			}
			return strconv.FormatInt(row.SiteID, 10)
		}(), SiteName: row.SiteName, SystemTaskMetric: systemTaskMetric(row), DataStatus: status, AsOf: row.AsOf})
	}
	return out
}
func systemTaskCompleteness(states map[int64]model.SystemTaskSiteCollectionState) (bool, *string, int64) {
	truncated, idGap := false, false
	var observed int64
	for _, state := range states {
		truncated = truncated || state.Truncated
		idGap = idGap || state.IDGap
		observed += state.ObservedCount
	}
	var reason *string
	value := ""
	switch {
	case truncated && idGap:
		value = "source_limit_and_id_gap"
	case truncated:
		value = "source_limit"
	case idGap:
		value = "id_gap"
	}
	if value != "" {
		reason = &value
	}
	return truncated || idGap, reason, observed
}
func (s *SystemTaskCatalogService) Statistics(ctx context.Context, q dto.SystemTaskQuery) (dto.SystemTaskStatisticsResponse, error) {
	q.Normalize()
	if q.Validate() != nil {
		return dto.SystemTaskStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summary, types, statusRows, sites []model.SystemTaskMetricRow
	var statuses map[int64]model.SystemTaskSiteCollectionState
	var overall string
	var asOf *int64
	err := s.readSnapshot(ctx, func(r *model.SystemTaskRepository) error {
		var e error
		statuses, overall, asOf, e = r.CollectionStatuses(ctx, q.SiteIDs)
		if e != nil {
			return e
		}
		for _, spec := range []struct {
			dim    string
			target *[]model.SystemTaskMetricRow
		}{{"summary", &summary}, {"type", &types}, {"status", &statusRows}, {"site", &sites}} {
			*spec.target, e = r.Metrics(ctx, q, spec.dim)
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return dto.SystemTaskStatisticsResponse{}, err
	}
	truncated, reason, observed := systemTaskCompleteness(statuses)
	out := dto.SystemTaskStatisticsResponse{TypeBreakdown: systemTaskBreakdowns(types, nil, overall), StatusBreakdown: systemTaskBreakdowns(statusRows, nil, overall), SiteBreakdown: systemTaskBreakdowns(sites, statuses, overall), DataStatus: overall, AsOf: asOf, Truncated: truncated, TruncationReason: reason, SourceLimit: "100", ObservedCount: strconv.FormatInt(observed, 10)}
	if len(summary) > 0 {
		out.Summary = systemTaskMetric(summary[0])
	} else {
		out.Summary = systemTaskMetric(model.SystemTaskMetricRow{})
	}
	return out, nil
}
