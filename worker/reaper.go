package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

const collectionTaskLeaseTimeout = 5 * time.Minute

type ReaperOptions struct {
	Repository    *model.CollectionTaskRepository
	Clock         common.Clock
	Interval      time.Duration
	LeaseTimeout  time.Duration
	AttemptPolicy model.CollectionTaskAttemptPolicy
	Metrics       ReaperMetricsRecorder
}

type Reaper struct {
	repository    *model.CollectionTaskRepository
	clock         common.Clock
	interval      time.Duration
	leaseTimeout  time.Duration
	attemptPolicy model.CollectionTaskAttemptPolicy
	metrics       ReaperMetricsRecorder
}

func NewReaper(options ReaperOptions) (*Reaper, error) {
	if options.Repository == nil || options.Clock == nil {
		return nil, fmt.Errorf("reaper dependencies are required")
	}
	if options.Interval <= 0 {
		options.Interval = time.Minute
	}
	if options.LeaseTimeout <= 0 {
		options.LeaseTimeout = collectionTaskLeaseTimeout
	}
	if options.AttemptPolicy.DefaultMaxAttempts <= 0 && len(options.AttemptPolicy.MaxAttempts) == 0 {
		options.AttemptPolicy = defaultAttemptPolicy()
	}
	return &Reaper{
		repository: options.Repository, clock: options.Clock, interval: options.Interval,
		leaseTimeout: options.LeaseTimeout, attemptPolicy: options.AttemptPolicy, metrics: options.Metrics,
	}, nil
}

func (reaper *Reaper) Takeover(ctx context.Context) (int, error) {
	count, err := reaper.repository.RecoverRunning(ctx, reaper.clock.Now().Unix(), nil, reaper.attemptPolicy)
	reaper.recordRecovery(count, err)
	return count, err
}

func (reaper *Reaper) RunOnce(ctx context.Context) (int, error) {
	cutoff := reaper.clock.Now().Add(-reaper.leaseTimeout).Unix()
	count, err := reaper.repository.RecoverRunning(ctx, reaper.clock.Now().Unix(), &cutoff, reaper.attemptPolicy)
	reaper.recordRecovery(count, err)
	return count, err
}

func (reaper *Reaper) recordRecovery(count int, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	recordWorkerMetric(func() { reaper.metrics.IncrementRuntimeRecovery("collection", result) })
	if err == nil && count > 0 {
		recordWorkerMetric(func() {
			reaper.metrics.AddCollectionEvents("other", "takeover", "success", float64(count))
		})
	}
}

func (reaper *Reaper) Run(ctx context.Context) error {
	ticker := reaper.clock.NewTicker(reaper.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if _, err := reaper.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}
