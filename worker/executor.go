package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

type JobResult struct {
	FetchedRows int64
	WrittenRows int64
}

type JobOutcome struct {
	Result              JobResult
	TransactionMutation model.CollectionTaskWindowMutation
}

type JobExecution struct {
	Claim     model.CollectionTaskClaim
	Window    *model.CollectionRunWindow
	RequestID string
}

type JobHandler interface {
	Execute(context.Context, JobExecution) (JobOutcome, error)
}

type JobHandlerFunc func(context.Context, JobExecution) (JobOutcome, error)

func (function JobHandlerFunc) Execute(ctx context.Context, execution JobExecution) (JobOutcome, error) {
	return function(ctx, execution)
}

type ExecutorOptions struct {
	Repository    *model.CollectionTaskRepository
	Settings      *model.CollectorSettingRepository
	Clock         common.Clock
	Handlers      map[string]JobHandler
	PollInterval  time.Duration
	SliceLimit    time.Duration
	AttemptPolicy model.CollectionTaskAttemptPolicy
	PostCommit    service.PostCommitNotifier
	Metrics       ExecutorMetricsRecorder
}

type Executor struct {
	repository    *model.CollectionTaskRepository
	settings      *model.CollectorSettingRepository
	clock         common.Clock
	pollInterval  time.Duration
	sliceLimit    time.Duration
	attemptPolicy model.CollectionTaskAttemptPolicy
	limiter       *queueLimiter
	bootNonce     string
	postCommit    service.PostCommitNotifier
	metrics       ExecutorMetricsRecorder

	handlersMu sync.RWMutex
	handlers   map[string]JobHandler
	sequence   atomic.Uint64
	active     sync.WaitGroup
}

func NewExecutor(options ExecutorOptions) (*Executor, error) {
	if options.Repository == nil || options.Settings == nil || options.Clock == nil {
		return nil, fmt.Errorf("executor dependencies are required")
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 100 * time.Millisecond
	}
	if options.SliceLimit <= 0 {
		options.SliceLimit = 30 * time.Second
	}
	if options.AttemptPolicy.DefaultMaxAttempts <= 0 && len(options.AttemptPolicy.MaxAttempts) == 0 {
		options.AttemptPolicy = defaultAttemptPolicy()
	}
	bootNonce, err := newWorkerBootNonce()
	if err != nil {
		return nil, err
	}
	executor := &Executor{
		repository: options.Repository, settings: options.Settings, clock: options.Clock,
		pollInterval: options.PollInterval, sliceLimit: options.SliceLimit,
		attemptPolicy: options.AttemptPolicy, limiter: newQueueLimiter(), bootNonce: bootNonce,
		postCommit: options.PostCommit, metrics: options.Metrics,
		handlers: make(map[string]JobHandler),
	}
	for taskType, handler := range options.Handlers {
		if handler != nil {
			executor.handlers[taskType] = handler
		}
	}
	return executor, nil
}

func (executor *Executor) Register(taskType string, handler JobHandler) error {
	if _, valid := QueueForTask(taskType); !valid || handler == nil {
		return model.ErrCollectionRunContract
	}
	executor.handlersMu.Lock()
	defer executor.handlersMu.Unlock()
	executor.handlers[taskType] = handler
	return nil
}

func (executor *Executor) hasHandler(taskType string) bool {
	executor.handlersMu.RLock()
	defer executor.handlersMu.RUnlock()
	return executor.handlers[taskType] != nil
}

func (executor *Executor) Run(admissionCtx context.Context, executionCtx context.Context) error {
	if admissionCtx == nil || executionCtx == nil {
		return fmt.Errorf("executor contexts are required")
	}
	if err := executor.dispatch(admissionCtx, executionCtx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	ticker := executor.clock.NewTicker(executor.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-admissionCtx.Done():
			executor.active.Wait()
			return nil
		case <-ticker.C():
			if err := executor.dispatch(admissionCtx, executionCtx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (executor *Executor) dispatch(admissionCtx context.Context, executionCtx context.Context) error {
	settings, err := executor.settings.Load(admissionCtx)
	if err != nil {
		return err
	}
	if err := executor.dispatchShared(admissionCtx, executionCtx, settings); err != nil {
		return err
	}
	for _, queue := range independentQueueOrder {
		maximum := queueConcurrency(queue, settings)
		for executor.limiter.tryAcquire(queue, maximum) {
			taskTypes := executor.registeredTaskTypes(queue)
			if len(taskTypes) == 0 {
				executor.limiter.release(queue)
				break
			}
			now := executor.clock.Now().Unix()
			requestID := executor.nextRequestID(now)
			claim, claimErr := executor.repository.ClaimNext(admissionCtx, model.CollectionTaskClaimOptions{
				TaskTypes: taskTypes, Now: now, RequestID: requestID, MaxWindow: 24, ScanLimit: 64,
			})
			if claimErr != nil {
				executor.limiter.release(queue)
				if errors.Is(claimErr, model.ErrCollectionTaskUnavailable) || errors.Is(claimErr, context.Canceled) {
					break
				}
				return claimErr
			}
			executor.recordClaim(queue, claim.Run.TaskType)
			executor.active.Add(1)
			go func(queue QueueKind, claim model.CollectionTaskClaim) {
				defer executor.active.Done()
				defer executor.limiter.release(queue)
				executor.executeClaim(executionCtx, claim)
			}(queue, claim)
		}
	}
	return nil
}

func (executor *Executor) dispatchShared(
	admissionCtx context.Context,
	executionCtx context.Context,
	settings model.CollectorSettings,
) error {
	initialPriority := constant.CollectionPriorityInitialBackfill
	for executor.limiter.tryAcquire(QueueInitialBackfill, initialBackfillConcurrency) {
		now := executor.clock.Now().Unix()
		claim, err := executor.repository.ClaimNext(admissionCtx, model.CollectionTaskClaimOptions{
			TaskTypes: []string{constant.TaskTypeUsageBackfill}, Priority: &initialPriority,
			Now: now, RequestID: executor.nextRequestID(now), MaxWindow: 2048, ScanLimit: 64,
		})
		if err != nil {
			executor.limiter.release(QueueInitialBackfill)
			if errors.Is(err, model.ErrCollectionTaskUnavailable) || errors.Is(err, context.Canceled) {
				break
			}
			return err
		}
		executor.recordClaim(QueueBackfill, claim.Run.TaskType)
		executor.active.Add(1)
		go func(claim model.CollectionTaskClaim) {
			defer executor.active.Done()
			defer executor.limiter.release(QueueInitialBackfill)
			executor.executeClaim(executionCtx, claim)
		}(claim)
	}

	for {
		taskTypes := executor.registeredSharedTaskTypes(settings)
		if len(taskTypes) == 0 {
			return nil
		}
		now := executor.clock.Now().Unix()
		claim, err := executor.repository.ClaimNext(admissionCtx, model.CollectionTaskClaimOptions{
			TaskTypes: taskTypes, ExcludePriority: &initialPriority,
			Now: now, RequestID: executor.nextRequestID(now), MaxWindow: 24, ScanLimit: 64,
		})
		if err != nil {
			if errors.Is(err, model.ErrCollectionTaskUnavailable) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		queue, valid := QueueForTask(claim.Run.TaskType)
		if valid {
			executor.recordClaim(queue, claim.Run.TaskType)
		} else {
			executor.recordClaim(QueueKind("other"), claim.Run.TaskType)
		}
		capacityKey := queueCapacityKey(queue)
		if !valid || !sharedCollectionQueue(queue) || !executor.limiter.tryAcquire(capacityKey, queueConcurrency(queue, settings)) {
			return fmt.Errorf("shared collection queue capacity changed during claim")
		}
		executor.active.Add(1)
		go func(queue QueueKind, capacityKey QueueKind, claim model.CollectionTaskClaim) {
			defer executor.active.Done()
			defer executor.limiter.release(capacityKey)
			executor.executeClaim(executionCtx, claim)
		}(queue, capacityKey, claim)
	}
}

func (executor *Executor) registeredTaskTypes(queue QueueKind) []string {
	executor.handlersMu.RLock()
	defer executor.handlersMu.RUnlock()
	taskTypes := make([]string, 0, len(executor.handlers))
	for taskType := range executor.handlers {
		candidate, valid := QueueForTask(taskType)
		if valid && candidate == queue {
			taskTypes = append(taskTypes, taskType)
		}
	}
	// The database applies priority and stable FIFO ordering. Sorting the filter
	// keeps generated SQL deterministic for tests and query plans.
	sortStrings(taskTypes)
	return taskTypes
}

func (executor *Executor) registeredSharedTaskTypes(settings model.CollectorSettings) []string {
	executor.handlersMu.RLock()
	defer executor.handlersMu.RUnlock()
	taskTypes := make([]string, 0, len(executor.handlers))
	for taskType := range executor.handlers {
		queue, valid := QueueForTask(taskType)
		if !valid || !sharedCollectionQueue(queue) ||
			executor.limiter.count(queueCapacityKey(queue)) >= queueConcurrency(queue, settings) {
			continue
		}
		taskTypes = append(taskTypes, taskType)
	}
	sortStrings(taskTypes)
	return taskTypes
}

func (executor *Executor) executeClaim(ctx context.Context, claim model.CollectionTaskClaim) {
	heartbeatContext, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go executor.heartbeatLoop(heartbeatContext, claim)
	if ctx.Err() != nil {
		return
	}

	handler := executor.handler(claim.Run.TaskType)
	if handler == nil {
		if _, err := executor.repository.ReleaseClaim(context.Background(), claim, executor.clock.Now().Unix()); err == nil {
			executor.recordOutcome(claim.Run.TaskType, model.CollectionTaskStatusPending, "released")
		}
		return
	}
	if len(claim.Windows) == 0 {
		executor.executeNonWindow(ctx, claim, handler)
		return
	}
	if claim.Run.TaskType == constant.TaskTypeUsageBackfill &&
		claim.Run.Priority == constant.CollectionPriorityInitialBackfill {
		executor.executeInitialBackfillClaim(ctx, claim, handler)
		return
	}
	startedAt := executor.clock.Now()
	lastCommitAt := claim.Run.UpdatedAt
	for index := range claim.Windows {
		window := claim.Windows[index]
		if ctx.Err() != nil {
			return
		}
		if executor.clock.Now().Sub(startedAt) >= executor.sliceLimit {
			for remaining := index; remaining < len(claim.Windows); remaining++ {
				pending := claim.Windows[remaining]
				commitAt := monotonicWorkerCommitTime(executor.clock.Now().Unix(), lastCommitAt, pending.UpdatedAt)
				lastCommitAt = commitAt
				next := commitAt
				_, err := executor.repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
					RunID: claim.Run.ID, RequestID: claim.RequestID, Now: commitAt,
					Window: model.CollectionTaskWindowResult{
						WindowID: pending.ID, AttemptCount: pending.AttemptCount,
						Status: model.CollectionTaskStatusPending, NextRetryAt: &next,
					},
				})
				if err != nil {
					return
				}
				executor.recordOutcome(claim.Run.TaskType, model.CollectionTaskStatusPending, "retry")
			}
			return
		}
		jobOutcome, executionErr := handler.Execute(ctx, JobExecution{
			Claim: claim, Window: &window, RequestID: claim.RequestID,
		})
		if ctx.Err() != nil {
			return
		}
		windowOutcome := executor.windowOutcome(claim.Run.TaskType, window, jobOutcome.Result, executionErr)
		commitAt := monotonicWorkerCommitTime(executor.clock.Now().Unix(), lastCommitAt, window.UpdatedAt)
		lastCommitAt = commitAt
		completedRun, commitErr := executor.repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
			RunID: claim.Run.ID, RequestID: claim.RequestID, Now: commitAt,
			Window: windowOutcome, Mutation: jobOutcome.TransactionMutation,
		})
		if commitErr != nil {
			return
		}
		executor.recordOutcome(claim.Run.TaskType, windowOutcome.Status, "")
		executor.notifyWindowAfterCommitAsync(claim, window, completedRun, commitAt, jobOutcome.TransactionMutation != nil)
	}
}

func (executor *Executor) executeInitialBackfillClaim(
	ctx context.Context,
	claim model.CollectionTaskClaim,
	handler JobHandler,
) {
	limit := initialBackfillConcurrency
	if limit > len(claim.Windows) {
		limit = len(claim.Windows)
	}
	semaphore := make(chan struct{}, limit)
	var waitGroup sync.WaitGroup

	for index := range claim.Windows {
		window := claim.Windows[index]
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
		waitGroup.Add(1)
		go func(window model.CollectionRunWindow) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			if ctx.Err() != nil {
				return
			}
			windowContext, cancel := context.WithTimeout(ctx, initialBackfillWindowTimeout)
			jobOutcome, executionErr := handler.Execute(windowContext, JobExecution{
				Claim: claim, Window: &window, RequestID: claim.RequestID,
			})
			cancel()
			if ctx.Err() != nil {
				return
			}
			windowOutcome := executor.windowOutcome(claim.Run.TaskType, window, jobOutcome.Result, executionErr)
			commitAt := executor.clock.Now().Unix()
			completedRun, commitErr := executor.repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
				RunID: claim.Run.ID, RequestID: claim.RequestID, Now: commitAt,
				Window: windowOutcome, Mutation: jobOutcome.TransactionMutation,
			})
			if commitErr != nil {
				return
			}
			executor.recordOutcome(claim.Run.TaskType, windowOutcome.Status, "")
			executor.notifyWindowAfterCommitAsync(claim, window, completedRun, commitAt, jobOutcome.TransactionMutation != nil)
		}(window)
	}
	waitGroup.Wait()
}

func (executor *Executor) notifyWindowAfterCommit(
	claim model.CollectionTaskClaim,
	window model.CollectionRunWindow,
	completedRun model.CollectionRun,
	committedAt int64,
	hasMutation bool,
) {
	if executor.postCommit == nil {
		return
	}
	ctx := context.Background()
	if hasMutation {
		identity, err := executor.repository.CommittedCollectionWindowAlertIdentity(ctx, window.SiteID, window.HourTS)
		if err != nil {
			log.Printf(
				"alert post-commit identity load failed source=collection_window site_id=%d hour_ts=%d error_type=%T",
				window.SiteID, window.HourTS, err,
			)
		} else {
			executor.postCommit.NotifyAfterCommit(ctx, service.AlertPostCommitTrigger{
				Source: service.AlertSampleSourceWindow, RowID: identity.ID, HourTS: identity.HourTS,
				ObservedAt: identity.UpdatedAt, Window: service.AlertWindowSourceCollection,
			})
		}
	}
	switch claim.Run.TaskType {
	case constant.TaskTypeUsageValidation:
		executor.postCommit.NotifyAfterCommit(ctx, service.AlertPostCommitTrigger{
			Source: service.AlertSampleSourceWindow, RowID: window.ID, HourTS: window.HourTS,
			ObservedAt: committedAt, Window: service.AlertWindowSourceValidation,
		})
	case constant.TaskTypeUsageBackfill:
		executor.postCommit.NotifyAfterCommit(ctx, service.AlertPostCommitTrigger{
			Source: service.AlertSampleSourceWindow, RowID: completedRun.ID,
			ObservedAt: completedRun.UpdatedAt, Window: service.AlertWindowSourceBackfill,
		})
	case constant.TaskTypeAccountRebuild:
		if completedRun.Status == model.CollectionTaskStatusSuccess {
			executor.postCommit.NotifyAfterCommit(ctx, service.AlertPostCommitTrigger{
				Source: service.AlertSampleSourceLifecycle, ScopeType: "account",
				ScopeID: claim.Run.TargetID, ObservedAt: committedAt,
			})
		}
	case constant.TaskTypeCustomerRebuild:
		if completedRun.Status == model.CollectionTaskStatusSuccess {
			executor.postCommit.NotifyAfterCommit(ctx, service.AlertPostCommitTrigger{
				Source: service.AlertSampleSourceLifecycle, ScopeType: "customer",
				ScopeID: claim.Run.TargetID, ObservedAt: committedAt,
			})
		}
	}
}

func (executor *Executor) notifyWindowAfterCommitAsync(
	claim model.CollectionTaskClaim,
	window model.CollectionRunWindow,
	completedRun model.CollectionRun,
	committedAt int64,
	hasMutation bool,
) {
	if executor.postCommit == nil {
		return
	}
	go executor.notifyWindowAfterCommit(claim, window, completedRun, committedAt, hasMutation)
}

func monotonicWorkerCommitTime(now int64, previous ...int64) int64 {
	for _, value := range previous {
		if now <= value {
			now = value + 1
		}
	}
	return now
}

func (executor *Executor) executeNonWindow(ctx context.Context, claim model.CollectionTaskClaim, handler JobHandler) {
	outcome, err := handler.Execute(ctx, JobExecution{Claim: claim, RequestID: claim.RequestID})
	if ctx.Err() != nil {
		return
	}
	result := outcome.Result
	now := executor.clock.Now().Unix()
	request := model.CollectionTaskCommitRequest{
		RunID: claim.Run.ID, RequestID: claim.RequestID, Now: now,
		FetchedRows: result.FetchedRows, WrittenRows: result.WrittenRows,
	}
	if err == nil {
		request.RunStatus = model.CollectionTaskStatusSuccess
	} else if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		next := now
		request.RunStatus = model.CollectionTaskStatusPending
		request.NextAttemptAt = &next
	} else {
		attempt := claim.Run.RetryCount
		retryable, delay, code := retryDecision(claim.Run.TaskType, attempt, err, executor.attemptPolicy)
		request.ErrorCode = code
		if retryable {
			next := executor.clock.Now().Add(delay).Unix()
			request.RunStatus = model.CollectionTaskStatusPending
			request.NextAttemptAt = &next
		} else {
			request.RunStatus = model.CollectionTaskStatusFailed
		}
	}
	if _, commitErr := executor.repository.CommitClaim(context.Background(), request); commitErr == nil {
		executor.recordOutcome(claim.Run.TaskType, request.RunStatus, "")
	}
}

func (executor *Executor) recordClaim(queue QueueKind, taskType string) {
	recordWorkerMetric(func() {
		executor.metrics.IncrementTaskAttempt(taskType, "claimed")
		executor.metrics.IncrementCollectionEvent(string(queue), "claim", "success")
	})
}

func (executor *Executor) recordOutcome(taskType, status, taskResult string) {
	queue, valid := QueueForTask(taskType)
	if !valid {
		queue = QueueKind("other")
	}
	event, collectionResult := "completion", "failed"
	if taskResult == "" {
		switch status {
		case model.CollectionTaskStatusSuccess:
			taskResult, collectionResult = "success", "success"
		case model.CollectionTaskStatusPending:
			taskResult, event, collectionResult = "retry", "retry", "pending"
		case model.CollectionTaskStatusFailed, model.CollectionTaskStatusUnavailable:
			taskResult, collectionResult = "failed", "failed"
		default:
			taskResult, collectionResult = "lost", "lost"
		}
	} else if taskResult == "released" || taskResult == "retry" {
		// Cooperative slice yields and handler releases remain pending work;
		// they are not persisted collection failures.
		event, collectionResult = "retry", "pending"
	}
	recordWorkerMetric(func() {
		executor.metrics.IncrementTaskAttempt(taskType, taskResult)
		executor.metrics.IncrementCollectionEvent(string(queue), event, collectionResult)
	})
}

func (executor *Executor) windowOutcome(
	taskType string,
	window model.CollectionRunWindow,
	result JobResult,
	cause error,
) model.CollectionTaskWindowResult {
	outcome := model.CollectionTaskWindowResult{
		WindowID: window.ID, AttemptCount: window.AttemptCount,
		FetchedRows: result.FetchedRows, WrittenRows: result.WrittenRows,
	}
	if cause == nil {
		outcome.Status = model.CollectionTaskStatusSuccess
		return outcome
	}
	if errors.Is(cause, context.Canceled) {
		next := executor.clock.Now().Unix()
		outcome.Status = model.CollectionTaskStatusPending
		outcome.NextRetryAt = &next
		return outcome
	}
	retryable, delay, code := retryDecision(taskType, window.AttemptCount, cause, executor.attemptPolicy)
	outcome.ErrorCode = code
	var executionError *TaskExecutionError
	if errors.As(cause, &executionError) {
		outcome.ErrorParams = append([]byte(nil), executionError.Params...)
	}
	if retryable {
		next := executor.clock.Now().Add(delay).Unix()
		outcome.Status = model.CollectionTaskStatusPending
		outcome.NextRetryAt = &next
		return outcome
	}
	outcome.Status = model.CollectionTaskStatusFailed
	return outcome
}

func (executor *Executor) heartbeatLoop(ctx context.Context, claim model.CollectionTaskClaim) {
	ticker := executor.clock.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if err := executor.repository.Heartbeat(
				ctx, claim.Run.ID, claim.RequestID, executor.clock.Now().Unix(),
			); err != nil {
				return
			}
		}
	}
}

func (executor *Executor) handler(taskType string) JobHandler {
	executor.handlersMu.RLock()
	defer executor.handlersMu.RUnlock()
	return executor.handlers[taskType]
}

func (executor *Executor) nextRequestID(now int64) string {
	sequence := executor.sequence.Add(1)
	return "wrk_" + executor.bootNonce + "_" + strconv.FormatUint(sequence, 36)
}

func newWorkerBootNonce() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate worker boot nonce: %w", err)
	}
	return hex.EncodeToString(nonce[:]), nil
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for current := index; current > 0 && values[current] < values[current-1]; current-- {
			values[current], values[current-1] = values[current-1], values[current]
		}
	}
}
