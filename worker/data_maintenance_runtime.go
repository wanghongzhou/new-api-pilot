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

const defaultDataMaintenanceTick = time.Minute
const (
	startupAuthorizationIntentLimit = 20
	runtimeAuthorizationIntentLimit = 100
)

type DataMaintenanceProcessor interface {
	ProcessAuthorizationPricingIntent(context.Context) (model.AuthorizationPricingProcessResult, error)
}

type scheduledDataMaintenanceProcessor interface {
	RunScheduledMaintenance(context.Context) (service.ScheduledDataMaintenanceResult, error)
}

// DataMaintenanceWake is shared by SiteService and the managed runtime. Notify
// is deliberately non-blocking: the durable intent remains the source of truth.
type DataMaintenanceWake struct{ channel chan struct{} }

func NewDataMaintenanceWake() *DataMaintenanceWake {
	return &DataMaintenanceWake{channel: make(chan struct{}, 1)}
}

func (wake *DataMaintenanceWake) NotifyAuthorizationPricingSync() {
	if wake == nil {
		return
	}
	select {
	case wake.channel <- struct{}{}:
	default:
	}
}

type DataMaintenanceRuntime struct {
	processor DataMaintenanceProcessor
	clock     common.Clock
	tick      time.Duration
	wake      *DataMaintenanceWake
	logf      func(string, ...any)
	runMu     sync.Mutex
	mu        sync.Mutex
	running   bool
	ready     bool
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewDataMaintenanceRuntime(processor DataMaintenanceProcessor, clock common.Clock, wake *DataMaintenanceWake, tick time.Duration) (*DataMaintenanceRuntime, error) {
	if processor == nil || clock == nil || wake == nil {
		return nil, fmt.Errorf("data maintenance runtime dependencies are required")
	}
	if tick <= 0 {
		tick = defaultDataMaintenanceTick
	}
	return &DataMaintenanceRuntime{processor: processor, clock: clock, wake: wake, tick: tick, logf: log.Printf}, nil
}

func (runtime *DataMaintenanceRuntime) RunOnce(ctx context.Context) error {
	return runtime.runOnce(ctx, runtimeAuthorizationIntentLimit)
}

func (runtime *DataMaintenanceRuntime) runOnce(ctx context.Context, authorizationLimit int) error {
	if runtime == nil || runtime.processor == nil {
		return errors.New("data maintenance runtime is not initialized")
	}
	runtime.runMu.Lock()
	defer runtime.runMu.Unlock()
	for processed := 0; processed < authorizationLimit; processed++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := runtime.processor.ProcessAuthorizationPricingIntent(ctx)
		if err != nil {
			return err
		}
		if !result.Attempted {
			break
		}
	}
	if scheduled, ok := runtime.processor.(scheduledDataMaintenanceProcessor); ok {
		_, err := scheduled.RunScheduledMaintenance(ctx)
		return err
	}
	return nil
}

func (runtime *DataMaintenanceRuntime) Start(parent context.Context) error {
	if runtime == nil || parent == nil {
		return errors.New("data maintenance runtime is not initialized")
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return errors.New("data maintenance runtime is already running")
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	// Reserve the complete lifecycle before recovery so concurrent Start/Stop
	// and Quiesce can cancel and await the synchronous probe.
	runtime.running, runtime.ready = true, false
	runtime.cancel, runtime.done = cancel, done
	runtime.mu.Unlock()
	// Recovery is synchronous so Ready never reports true before durable state
	// has been read successfully at least once.
	if err := runtime.runOnce(ctx, startupAuthorizationIntentLimit); err != nil {
		runtime.mu.Lock()
		if runtime.done == done {
			runtime.running, runtime.ready, runtime.cancel, runtime.done = false, false, nil, nil
			close(done)
		}
		runtime.mu.Unlock()
		cancel()
		return fmt.Errorf("recover data maintenance state: %w", err)
	}
	if err := ctx.Err(); err != nil {
		runtime.mu.Lock()
		if runtime.done == done {
			runtime.running, runtime.ready, runtime.cancel, runtime.done = false, false, nil, nil
			close(done)
		}
		runtime.mu.Unlock()
		return fmt.Errorf("recover data maintenance state: %w", err)
	}
	runtime.mu.Lock()
	if runtime.done != done || ctx.Err() != nil {
		runtime.mu.Unlock()
		cancel()
		return errors.New("data maintenance runtime start was canceled")
	}
	runtime.ready = true
	runtime.mu.Unlock()
	go runtime.run(ctx, done)
	return nil
}

func (runtime *DataMaintenanceRuntime) run(ctx context.Context, done chan struct{}) {
	defer func() {
		runtime.mu.Lock()
		runtime.running, runtime.ready = false, false
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
		case <-runtime.wake.channel:
			runtime.runSafely(ctx)
		}
	}
}

func (runtime *DataMaintenanceRuntime) runSafely(ctx context.Context) {
	err := runtime.RunOnce(ctx)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		runtime.mu.Lock()
		runtime.ready = false
		runtime.mu.Unlock()
		if !errors.Is(err, context.Canceled) {
			runtime.logf("data maintenance failed error_type=%T", err)
		}
		return
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.ready = true
	}
	runtime.mu.Unlock()
}

func (runtime *DataMaintenanceRuntime) Quiesce() error {
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

func (runtime *DataMaintenanceRuntime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		runtime.mu.Unlock()
		return nil
	}
	done, cancel := runtime.done, runtime.cancel
	runtime.ready = false
	runtime.mu.Unlock()
	if cancel == nil {
		cancel = func() {}
	}
	return awaitRuntimeStop(ctx, done, cancel, false, func() error { return nil })
}

func (runtime *DataMaintenanceRuntime) Ready() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}
