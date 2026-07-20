package service

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"math/big"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"strconv"
	"strings"
)

type UpstreamTaskService struct{ database *gorm.DB }

func NewUpstreamTaskService(db *gorm.DB) (*UpstreamTaskService, error) {
	if db == nil {
		return nil, errors.New("upstream task database is required")
	}
	return &UpstreamTaskService{database: db}, nil
}
func (s *UpstreamTaskService) readSnapshot(ctx context.Context, read func(*model.UpstreamTaskRepository) error) error {
	if s == nil || s.database == nil || read == nil {
		return errors.New("upstream task snapshot dependencies are required")
	}
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return read(model.NewUpstreamTaskRepository(tx))
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
func (s *UpstreamTaskService) List(ctx context.Context, q dto.UpstreamTaskQuery) (dto.UpstreamTaskPageResponse, error) {
	q.Normalize()
	if s == nil || q.Validate() != nil {
		return dto.UpstreamTaskPageResponse{}, ErrStatisticsInvalid
	}
	var rows []model.UpstreamTaskReadRow
	var summary []model.UpstreamTaskMetricRow
	var total int64
	var collectionStatus string
	if err := s.readSnapshot(ctx, func(repository *model.UpstreamTaskRepository) error {
		var err error
		rows, total, err = repository.List(ctx, q)
		if err != nil {
			return err
		}
		summary, err = repository.Metrics(ctx, q, "summary")
		if err != nil {
			return err
		}
		_, collectionStatus, err = repository.CollectionStatuses(ctx, q.SiteIDs)
		return err
	}); err != nil {
		return dto.UpstreamTaskPageResponse{}, err
	}
	items := make([]dto.UpstreamTaskItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.UpstreamTaskItem{ID: strconv.FormatInt(r.ID, 10), SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, RemoteID: strconv.FormatInt(r.RemoteID, 10), CreatedAt: r.RemoteCreatedAt, UpdatedAt: r.RemoteUpdatedAt, TaskID: r.TaskID, Platform: r.Platform, UserID: strconv.FormatInt(r.RemoteUserID, 10), Group: r.RemoteGroup, ChannelID: strconv.FormatInt(r.RemoteChannelID, 10), Quota: strconv.FormatInt(r.Quota, 10), Action: r.Action, Status: r.RemoteStatus, SubmitTime: r.SubmitTime, StartTime: r.StartTime, FinishTime: r.FinishTime, Progress: r.Progress, Properties: dto.UpstreamTaskProperties{Model: r.ModelName}, FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt})
	}
	var asOf *int64
	if len(summary) == 1 {
		asOf = summary[0].AsOf
	} else if len(summary) != 0 {
		return dto.UpstreamTaskPageResponse{}, model.ErrStatisticsReadContract
	}
	return dto.UpstreamTaskPageResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize, DataStatus: collectionStatus, AsOf: asOf}, nil
}
func preciseRatio(n, d int64) *string {
	if d <= 0 {
		return nil
	}
	value := strings.TrimRight(strings.TrimRight(new(big.Rat).SetFrac(big.NewInt(n), big.NewInt(d)).FloatString(10), "0"), ".")
	return &value
}
func taskMetric(r model.UpstreamTaskMetricRow) dto.UpstreamTaskMetric {
	return dto.UpstreamTaskMetric{Total: strconv.FormatInt(r.Total, 10), Queued: strconv.FormatInt(r.Queued, 10), Running: strconv.FormatInt(r.Running, 10), Success: strconv.FormatInt(r.Success, 10), Failure: strconv.FormatInt(r.Failure, 10), SuccessRate: preciseRatio(r.Success, r.Success+r.Failure), AvgQueueSeconds: preciseRatio(r.QueueSum, r.QueueCount), AvgRunSeconds: preciseRatio(r.RunSum, r.RunCount), AvgTotalSeconds: preciseRatio(r.TotalSum, r.TotalCount)}
}
func taskBreakdown(rows []model.UpstreamTaskMetricRow, overall string, statuses map[int64]string) []dto.UpstreamTaskBreakdown {
	out := make([]dto.UpstreamTaskBreakdown, 0, len(rows))
	for _, r := range rows {
		status := overall
		if r.SiteID > 0 {
			if value, ok := statuses[r.SiteID]; ok {
				status = value
			} else {
				status = "pending"
			}
		}
		out = append(out, dto.UpstreamTaskBreakdown{DimensionID: r.DimensionID, DimensionName: r.DimensionName, SiteID: strconv.FormatInt(r.SiteID, 10), SiteName: r.SiteName, UpstreamTaskMetric: taskMetric(r), DataStatus: status, AsOf: r.AsOf})
	}
	return out
}
func (s *UpstreamTaskService) Statistics(ctx context.Context, q dto.UpstreamTaskQuery) (dto.UpstreamTaskStatisticsResponse, error) {
	q.Normalize()
	q.Page, q.PageSize = 1, 1
	if s == nil || q.Validate() != nil {
		return dto.UpstreamTaskStatisticsResponse{}, ErrStatisticsInvalid
	}
	var summary, statuses, platforms, actions, models, sites []model.UpstreamTaskMetricRow
	var collectionStatuses map[int64]string
	var overall string
	if err := s.readSnapshot(ctx, func(repository *model.UpstreamTaskRepository) error {
		queries := []struct {
			dimension string
			rows      *[]model.UpstreamTaskMetricRow
		}{{"summary", &summary}, {"status", &statuses}, {"platform", &platforms}, {"action", &actions}, {"model", &models}, {"site", &sites}}
		for _, query := range queries {
			var err error
			*query.rows, err = repository.Metrics(ctx, q, query.dimension)
			if err != nil {
				return err
			}
		}
		var err error
		collectionStatuses, overall, err = repository.CollectionStatuses(ctx, q.SiteIDs)
		return err
	}); err != nil {
		return dto.UpstreamTaskStatisticsResponse{}, err
	}
	out := dto.UpstreamTaskStatisticsResponse{StatusBreakdown: taskBreakdown(statuses, overall, nil), PlatformBreakdown: taskBreakdown(platforms, overall, nil), ActionBreakdown: taskBreakdown(actions, overall, nil), ModelBreakdown: taskBreakdown(models, overall, nil), SiteBreakdown: taskBreakdown(sites, overall, collectionStatuses), DataStatus: overall}
	if len(summary) > 0 {
		out.Summary = taskMetric(summary[0])
	} else {
		out.Summary = taskMetric(model.UpstreamTaskMetricRow{})
	}
	return out, nil
}
