package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

type RuntimeOptions struct {
	Repository        *model.CollectionTaskRepository
	Settings          *model.CollectorSettingRepository
	Clock             common.Clock
	SiteJobs          SitePeriodicJobRunner
	UsageCollector    UsageHourCollector
	LogCollector      UpstreamLogCollector
	Handlers          map[string]JobHandler
	PollInterval      time.Duration
	SchedulerTick     time.Duration
	AttemptPolicy     model.CollectionTaskAttemptPolicy
	RequiredTaskTypes []string
	PostCommit        service.PostCommitNotifier
	Metrics           WorkerMetricsRecorder
	MetricsRepository *model.OperationalMetricsRepository
	MetricsDatabase   *sql.DB
	ExportDir         string
	MetricsInterval   time.Duration
	MetricsTimeout    time.Duration
	FastTaskHistory   *common.RedisStore
}

type Runtime struct {
	executor          *Executor
	scheduler         *Scheduler
	materializer      *Materializer
	reaper            *Reaper
	metricsSampler    *MetricsSampler
	metrics           RuntimeMetricsRecorder
	requiredTaskTypes []string

	mu              sync.Mutex
	running         bool
	ready           bool
	cancelAdmission context.CancelFunc
	cancelExecution context.CancelFunc
	done            chan struct{}
	runErr          error
}

func NewRuntime(options RuntimeOptions) (*Runtime, error) {
	if options.Repository == nil || options.Settings == nil || options.Clock == nil {
		return nil, fmt.Errorf("runtime dependencies are required")
	}
	handlers := make(map[string]JobHandler, len(options.Handlers)+8)
	for taskType, handler := range SiteJobHandlers(options.SiteJobs) {
		handlers[taskType] = handler
	}
	for taskType, handler := range UsageJobHandlers(options.UsageCollector) {
		handlers[taskType] = handler
	}
	for taskType, handler := range LogJobHandlers(options.LogCollector) {
		handlers[taskType] = handler
	}
	for taskType, handler := range options.Handlers {
		if handler != nil {
			handlers[taskType] = handler
		}
	}
	seenRequired := make(map[string]struct{}, len(options.RequiredTaskTypes))
	for _, taskType := range options.RequiredTaskTypes {
		if !constant.ValidCollectionTaskType(taskType) {
			return nil, fmt.Errorf("invalid required task type %q", taskType)
		}
		if _, duplicate := seenRequired[taskType]; duplicate {
			return nil, fmt.Errorf("duplicate required task type %q", taskType)
		}
		seenRequired[taskType] = struct{}{}
	}
	executor, err := NewExecutor(ExecutorOptions{
		Repository: options.Repository, Settings: options.Settings, Clock: options.Clock,
		Handlers: handlers, PollInterval: options.PollInterval, AttemptPolicy: options.AttemptPolicy,
		PostCommit: options.PostCommit, Metrics: options.Metrics,
	})
	if err != nil {
		return nil, err
	}
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: options.Repository, Settings: options.Settings, Clock: options.Clock, Tick: options.SchedulerTick,
		SiteJobs: options.SiteJobs, Metrics: options.Metrics,
		FastTaskHistory: options.FastTaskHistory,
	})
	if err != nil {
		return nil, err
	}
	materializer, err := NewMaterializer(MaterializerOptions{Repository: options.Repository, Clock: options.Clock})
	if err != nil {
		return nil, err
	}
	reaper, err := NewReaper(ReaperOptions{
		Repository: options.Repository, Clock: options.Clock, AttemptPolicy: options.AttemptPolicy,
		Metrics: options.Metrics,
	})
	if err != nil {
		return nil, err
	}
	var metricsSampler *MetricsSampler
	if options.MetricsRepository != nil || options.MetricsDatabase != nil {
		if options.Metrics == nil || options.MetricsRepository == nil || options.MetricsDatabase == nil {
			return nil, fmt.Errorf("runtime metrics sampler dependencies are required")
		}
		metricsSampler, err = NewMetricsSampler(MetricsSamplerOptions{
			Snapshotter: options.MetricsRepository, Database: options.MetricsDatabase,
			Recorder: options.Metrics, Clock: options.Clock, ExportDir: options.ExportDir,
			Interval: options.MetricsInterval, Timeout: options.MetricsTimeout,
		})
		if err != nil {
			return nil, err
		}
	}
	return &Runtime{
		executor: executor, scheduler: scheduler, materializer: materializer, reaper: reaper,
		metricsSampler: metricsSampler, metrics: options.Metrics,
		requiredTaskTypes: append([]string(nil), options.RequiredTaskTypes...),
	}, nil
}

// Start performs durable recovery before reporting ready. It deliberately does
// not mutate the HTTP readiness coordinator; composition owns that boundary.
func (runtime *Runtime) Start(parent context.Context) error {
	if runtime == nil || parent == nil {
		return fmt.Errorf("worker runtime is not initialized")
	}
	recordWorkerMetric(func() {
		runtime.metrics.SetRuntimeReady("collection", false)
		runtime.metrics.SetRuntimeReady("scheduler", false)
	})
	for _, taskType := range runtime.requiredTaskTypes {
		if !runtime.executor.hasHandler(taskType) {
			return fmt.Errorf("required task handler %q is not registered", taskType)
		}
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return fmt.Errorf("worker runtime is already running")
	}
	admissionCtx, cancelAdmission := context.WithCancel(parent)
	executionCtx, cancelExecution := context.WithCancel(parent)
	runtime.running = true
	runtime.ready = false
	runtime.cancelAdmission = cancelAdmission
	runtime.cancelExecution = cancelExecution
	runtime.done = make(chan struct{})
	runtime.runErr = nil
	done := runtime.done
	runtime.mu.Unlock()

	if _, err := runtime.materializer.RunOnce(admissionCtx); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}
	if _, err := runtime.reaper.Takeover(admissionCtx); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}
	if err := runtime.scheduler.Startup(admissionCtx); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}

	runtime.mu.Lock()
	runtime.ready = true
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.IncrementRuntimeRecovery("collection", "success")
		runtime.metrics.SetRuntimeReady("collection", true)
		runtime.metrics.SetRuntimeReady("scheduler", true)
	})
	var components sync.WaitGroup
	runs := []func() error{
		func() error { return runtime.executor.Run(admissionCtx, executionCtx) },
		func() error { return runtime.scheduler.Run(admissionCtx) },
		func() error { return runtime.materializer.Run(admissionCtx) },
		func() error { return runtime.reaper.Run(admissionCtx) },
	}
	if runtime.metricsSampler != nil {
		runs = append(runs, func() error { return runtime.metricsSampler.Run(admissionCtx) })
	}
	componentErrors := make(chan error, len(runs))
	for _, run := range runs {
		components.Add(1)
		go func(run func() error) {
			defer components.Done()
			if err := run(); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case componentErrors <- err:
				default:
				}
				cancelAdmission()
				cancelExecution()
			}
		}(run)
	}
	go func() {
		components.Wait()
		cancelExecution()
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
		recordWorkerMetric(func() {
			runtime.metrics.SetRuntimeReady("collection", false)
			runtime.metrics.SetRuntimeReady("scheduler", false)
		})
	}()
	return nil
}

func (runtime *Runtime) failStart(
	cancelAdmission context.CancelFunc,
	cancelExecution context.CancelFunc,
	done chan struct{},
	err error,
) {
	cancelAdmission()
	cancelExecution()
	runtime.scheduler.shutdownFastTasks()
	runtime.mu.Lock()
	runtime.ready = false
	runtime.running = false
	runtime.runErr = err
	close(done)
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.IncrementRuntimeRecovery("collection", "failure")
		runtime.metrics.SetRuntimeReady("collection", false)
		runtime.metrics.SetRuntimeReady("scheduler", false)
	})
}

func (runtime *Runtime) Quiesce() error {
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
	cancel := runtime.cancelAdmission
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.SetRuntimeReady("collection", false)
		runtime.metrics.SetRuntimeReady("scheduler", false)
	})
	cancel()
	return nil
}

func (runtime *Runtime) Stop(ctx context.Context) error {
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
	cancelAdmission := runtime.cancelAdmission
	cancelExecution := runtime.cancelExecution
	done := runtime.done
	runtime.mu.Unlock()
	recordWorkerMetric(func() {
		runtime.metrics.SetRuntimeReady("collection", false)
		runtime.metrics.SetRuntimeReady("scheduler", false)
	})
	cancelAdmission()
	return awaitRuntimeStop(ctx, done, cancelExecution, true, func() error {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		err := runtime.runErr
		return err
	})
}

func (runtime *Runtime) Ready() bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}
