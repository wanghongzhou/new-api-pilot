package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/service"
)

const defaultAlertEvaluationInterval = 5 * time.Minute

type AlertEvaluationRunner interface {
	RunOnce(context.Context) (service.AlertScanResult, error)
}

type AlertDeliveryRunner interface {
	Recover(context.Context) error
	Run(context.Context) error
}

type AlertRuntimeOptions struct {
	Evaluator          AlertEvaluationRunner
	Deliveries         AlertDeliveryRunner
	Clock              common.Clock
	EvaluationInterval time.Duration
	Metrics            RuntimeMetricsRecorder
}

type AlertRuntime struct {
	evaluator          AlertEvaluationRunner
	deliveries         AlertDeliveryRunner
	clock              common.Clock
	evaluationInterval time.Duration
	metrics            RuntimeMetricsRecorder

	mu      sync.Mutex
	running bool
	ready   bool
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
}

func NewAlertRuntime(options AlertRuntimeOptions) (*AlertRuntime, error) {
	if options.Evaluator == nil || options.Deliveries == nil || options.Clock == nil {
		return nil, fmt.Errorf("alert runtime dependencies are required")
	}
	if options.EvaluationInterval <= 0 {
		options.EvaluationInterval = defaultAlertEvaluationInterval
	}
	return &AlertRuntime{
		evaluator: options.Evaluator, deliveries: options.Deliveries, clock: options.Clock,
		evaluationInterval: options.EvaluationInterval, metrics: options.Metrics,
	}, nil
}

func (runtime *AlertRuntime) RunOnce(ctx context.Context) (service.AlertScanResult, error) {
	return runtime.evaluator.RunOnce(ctx)
}

func (runtime *AlertRuntime) Start(parent context.Context) error {
	if runtime == nil || parent == nil {
		return errors.New("alert runtime is not initialized")
	}
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("alert", false) })
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return errors.New("alert runtime is already running")
	}
	runtime.running = true
	runtime.ready = false
	runtime.runErr = nil
	runtime.done = make(chan struct{})
	done := runtime.done
	runtime.mu.Unlock()

	if _, err := runtime.evaluator.RunOnce(parent); err != nil {
		runtime.failStart(done, err)
		return err
	}
	if err := runtime.deliveries.Recover(parent); err != nil {
		runtime.failStart(done, err)
		return err
	}

	ctx, cancel := context.WithCancel(parent)
	runtime.mu.Lock()
	runtime.cancel = cancel
	runtime.ready = true
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.IncrementRuntimeRecovery("alert", "success")
		runtime.metrics.SetRuntimeReady("alert", true)
	})

	componentErrors := make(chan error, 2)
	var components sync.WaitGroup
	evaluationTicker := runtime.clock.NewTicker(runtime.evaluationInterval)
	runComponent := func(name string, run func() error) {
		components.Add(1)
		go func() {
			defer components.Done()
			err := run()
			if err == nil && ctx.Err() == nil {
				err = fmt.Errorf("alert runtime component %s stopped unexpectedly", name)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				select {
				case componentErrors <- err:
				default:
				}
			}
			cancel()
		}()
	}
	runComponent("evaluation", func() error {
		return runtime.runEvaluations(ctx, evaluationTicker)
	})
	runComponent("delivery", func() error { return runtime.deliveries.Run(ctx) })
	go func() {
		components.Wait()
		close(componentErrors)
		var runErr error
		for err := range componentErrors {
			if runErr == nil {
				runErr = err
			}
		}
		runtime.mu.Lock()
		runtime.ready = false
		runtime.running = false
		runtime.runErr = runErr
		close(done)
		runtime.mu.Unlock()
		recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("alert", false) })
	}()
	return nil
}

func (runtime *AlertRuntime) failStart(done chan struct{}, err error) {
	runtime.mu.Lock()
	runtime.ready = false
	runtime.running = false
	runtime.runErr = err
	close(done)
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.IncrementRuntimeRecovery("alert", "failure")
		runtime.metrics.SetRuntimeReady("alert", false)
	})
}

func (runtime *AlertRuntime) Quiesce() error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	runtime.ready = false
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		return err
	}
	cancel := runtime.cancel
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("alert", false) })
	if cancel != nil {
		cancel()
	}
	return nil
}

func (runtime *AlertRuntime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		return err
	}
	done := runtime.done
	cancel := runtime.cancel
	runtime.ready = false
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("alert", false) })
	if cancel == nil {
		cancel = func() {}
	}
	return awaitRuntimeStop(ctx, done, cancel, false, func() error {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		err := runtime.runErr
		return err
	})
}

func (runtime *AlertRuntime) Ready() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}

func (runtime *AlertRuntime) Run(parent context.Context) error {
	if err := runtime.Start(parent); err != nil {
		return err
	}
	runtime.mu.Lock()
	done := runtime.done
	runtime.mu.Unlock()
	<-done
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.runErr
}

func (runtime *AlertRuntime) runEvaluations(ctx context.Context, ticker common.Ticker) error {
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if _, err := runtime.evaluator.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}
