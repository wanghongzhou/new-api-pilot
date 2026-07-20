package service

import (
	"context"
	"errors"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

type UpstreamLogRetentionService struct {
	repository *model.UpstreamLogRepository
	clock      common.Clock
}

func NewUpstreamLogRetentionService(repository *model.UpstreamLogRepository, clock common.Clock) (*UpstreamLogRetentionService, error) {
	if repository == nil || clock == nil {
		return nil, errors.New("upstream log retention dependencies are required")
	}
	return &UpstreamLogRetentionService{repository: repository, clock: clock}, nil
}

func (service *UpstreamLogRetentionService) Clean(ctx context.Context) (int64, error) {
	days, err := service.repository.LoadRetentionDays(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := service.clock.Now().Unix() - int64(days)*24*3600
	var total int64
	for {
		deleted, deleteErr := service.repository.DeleteBefore(ctx, cutoff, 1000)
		total += deleted
		if deleteErr != nil || deleted < 1000 {
			return total, deleteErr
		}
	}
}
