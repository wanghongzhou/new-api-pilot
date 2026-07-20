package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"new-api-pilot/common"
)

type UpstreamLogRetentionCleaner interface {
	Clean(context.Context) (int64, error)
}

type UpstreamLogRetentionRuntime struct {
	cleaner UpstreamLogRetentionCleaner
	clock   common.Clock
	mu      sync.Mutex
	running bool
	ready   bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewUpstreamLogRetentionRuntime(cleaner UpstreamLogRetentionCleaner, clock common.Clock) (*UpstreamLogRetentionRuntime, error) {
	if cleaner == nil || clock == nil {
		return nil, errors.New("upstream log retention runtime dependencies are required")
	}
	return &UpstreamLogRetentionRuntime{cleaner: cleaner, clock: clock}, nil
}

func (runtime *UpstreamLogRetentionRuntime) Start(parent context.Context) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.running {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	runtime.cancel, runtime.done, runtime.running, runtime.ready = cancel, make(chan struct{}), true, true
	ticker := runtime.clock.NewTicker(24 * time.Hour)
	go func() {
		defer close(runtime.done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				cleanCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				_, _ = runtime.cleaner.Clean(cleanCtx)
				cancel()
			}
		}
	}()
	return nil
}

func (runtime *UpstreamLogRetentionRuntime) Quiesce() error {
	runtime.mu.Lock()
	runtime.ready = false
	runtime.mu.Unlock()
	return nil
}
func (runtime *UpstreamLogRetentionRuntime) Ready() bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}
func (runtime *UpstreamLogRetentionRuntime) Stop(ctx context.Context) error {
	runtime.mu.Lock()
	cancel, done := runtime.cancel, runtime.done
	runtime.ready = false
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	runtime.mu.Lock()
	runtime.running = false
	runtime.mu.Unlock()
	return nil
}
