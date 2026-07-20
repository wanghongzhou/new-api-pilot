package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

type MaterializerOptions struct {
	Repository *model.CollectionTaskRepository
	Clock      common.Clock
	Interval   time.Duration
	BatchSize  int
	ScanLimit  int
}

type Materializer struct {
	repository *model.CollectionTaskRepository
	clock      common.Clock
	interval   time.Duration
	batchSize  int
	scanLimit  int
}

func NewMaterializer(options MaterializerOptions) (*Materializer, error) {
	if options.Repository == nil || options.Clock == nil {
		return nil, fmt.Errorf("materializer dependencies are required")
	}
	if options.Interval <= 0 {
		options.Interval = time.Minute
	}
	if options.BatchSize <= 0 || options.BatchSize > 1000 {
		options.BatchSize = 1000
	}
	if options.ScanLimit <= 0 {
		options.ScanLimit = 100
	}
	return &Materializer{
		repository: options.Repository, clock: options.Clock, interval: options.Interval,
		batchSize: options.BatchSize, scanLimit: options.ScanLimit,
	}, nil
}

func (materializer *Materializer) RunOnce(ctx context.Context) (int, error) {
	ids, err := materializer.repository.PendingMaterializationRunIDs(ctx, materializer.scanLimit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return completed, ctx.Err()
		}
		_, err := materializer.repository.MaterializeRunWindows(ctx, id, materializer.clock.Now().Unix(), materializer.batchSize)
		if err != nil {
			if errors.Is(err, model.ErrCollectionTaskClaimLost) || errors.Is(err, model.ErrSiteRunConfigChanged) {
				continue
			}
			return completed, err
		}
		completed++
	}
	return completed, nil
}

func (materializer *Materializer) Run(ctx context.Context) error {
	ticker := materializer.clock.NewTicker(materializer.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if _, err := materializer.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}
