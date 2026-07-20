package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

const (
	defaultResourceRetentionTick = time.Minute
	resourceRetentionHour        = 3
	resourceRetentionMinute      = 30
)

type ResourceRetentionCleaner interface {
	Clean(context.Context, int) (service.ResourceRetentionResult, error)
}

type ResourceRetentionSettingsReader interface {
	Load(context.Context) (model.CollectorSettings, error)
}

type ResourceRetentionRuntimeOptions struct {
	Cleaner  ResourceRetentionCleaner
	Settings ResourceRetentionSettingsReader
	Clock    common.Clock
	Tick     time.Duration
	Logf     func(string, ...any)
}

type ResourceRetentionScheduleResult struct {
	Attempted bool
	Completed bool
	Report    service.ResourceRetentionResult
}

type ResourceRetentionRuntime struct {
	cleaner  ResourceRetentionCleaner
	settings ResourceRetentionSettingsReader
	clock    common.Clock
	tick     time.Duration
	logf     func(string, ...any)

	runMu             sync.Mutex
	lastCompletedDate int

	mu      sync.Mutex
	running bool
	ready   bool
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
}

func NewResourceRetentionRuntime(options ResourceRetentionRuntimeOptions) (*ResourceRetentionRuntime, error) {
	if options.Cleaner == nil || options.Settings == nil || options.Clock == nil {
		return nil, fmt.Errorf("resource retention runtime dependencies are required")
	}
	if options.Tick <= 0 {
		options.Tick = defaultResourceRetentionTick
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	return &ResourceRetentionRuntime{
		cleaner: options.Cleaner, settings: options.Settings, clock: options.Clock,
		tick: options.Tick, logf: options.Logf,
	}, nil
}

func (runtime *ResourceRetentionRuntime) RunOnce(ctx context.Context) (ResourceRetentionScheduleResult, error) {
	if runtime == nil || runtime.cleaner == nil || runtime.settings == nil || runtime.clock == nil {
		return ResourceRetentionScheduleResult{}, errors.New("resource retention runtime is not initialized")
	}
	runtime.runMu.Lock()
	defer runtime.runMu.Unlock()
	localNow := runtime.clock.Now().In(beijingLocation)
	dateKey := localNow.Year()*10000 + int(localNow.Month())*100 + localNow.Day()
	if localNow.Hour() < resourceRetentionHour ||
		(localNow.Hour() == resourceRetentionHour && localNow.Minute() < resourceRetentionMinute) ||
		runtime.lastCompletedDate == dateKey {
		return ResourceRetentionScheduleResult{}, nil
	}
	settings, err := runtime.settings.Load(ctx)
	if err != nil {
		return ResourceRetentionScheduleResult{Attempted: true}, fmt.Errorf("load resource retention settings: %w", err)
	}
	if settings.MinuteRetentionDays < 1 || settings.MinuteRetentionDays > 3650 {
		return ResourceRetentionScheduleResult{Attempted: true}, service.ErrResourceRetentionInvalid
	}
	report, err := runtime.cleaner.Clean(ctx, settings.MinuteRetentionDays)
	result := ResourceRetentionScheduleResult{Attempted: true, Completed: report.Complete, Report: report}
	if err != nil {
		return result, err
	}
	if report.Complete {
		runtime.lastCompletedDate = dateKey
	}
	return result, nil
}

func (runtime *ResourceRetentionRuntime) Start(parent context.Context) error {
	if runtime == nil || parent == nil {
		return errors.New("resource retention runtime is not initialized")
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return errors.New("resource retention runtime is already running")
	}
	ctx, cancel := context.WithCancel(parent)
	runtime.running = true
	runtime.ready = true
	runtime.cancel = cancel
	runtime.done = make(chan struct{})
	runtime.runErr = nil
	done := runtime.done
	runtime.mu.Unlock()
	go runtime.run(ctx, done)
	return nil
}

func (runtime *ResourceRetentionRuntime) run(ctx context.Context, done chan struct{}) {
	defer func() {
		runtime.mu.Lock()
		runtime.running = false
		runtime.ready = false
		close(done)
		runtime.mu.Unlock()
	}()
	runtime.runSafely(ctx)
	ticker := runtime.clock.NewTicker(runtime.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			runtime.runSafely(ctx)
		}
	}
}

func (runtime *ResourceRetentionRuntime) runSafely(ctx context.Context) {
	if _, err := runtime.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		runtime.logf("resource retention cleanup failed error_type=%T", err)
	}
}

func (runtime *ResourceRetentionRuntime) Quiesce() error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	runtime.ready = false
	cancel := runtime.cancel
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (runtime *ResourceRetentionRuntime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		return err
	}
	runtime.ready = false
	done := runtime.done
	cancel := runtime.cancel
	runtime.mu.Unlock()
	if cancel == nil {
		cancel = func() {}
	}
	return awaitRuntimeStop(ctx, done, cancel, false, func() error {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.runErr
	})
}

func (runtime *ResourceRetentionRuntime) Ready() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}
