package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

type alertEvaluationRunnerFunc func(context.Context) (service.AlertScanResult, error)

func (function alertEvaluationRunnerFunc) RunOnce(ctx context.Context) (service.AlertScanResult, error) {
	return function(ctx)
}

type alertDeliveryRunnerFunc func(context.Context) error

func (function alertDeliveryRunnerFunc) Run(ctx context.Context) error { return function(ctx) }

func (function alertDeliveryRunnerFunc) Recover(context.Context) error { return nil }

type tickerRecordingClock struct {
	*testsupport.FakeClock
	tickerCreations atomic.Int32
}

func (clock *tickerRecordingClock) NewTicker(duration time.Duration) common.Ticker {
	clock.tickerCreations.Add(1)
	return clock.FakeClock.NewTicker(duration)
}

func TestAlertRuntimeRunsStartupAndFiveMinuteEvaluation(t *testing.T) {
	clock := &tickerRecordingClock{FakeClock: testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))}
	calls := make(chan int, 4)
	var count atomic.Int32
	evaluator := alertEvaluationRunnerFunc(func(context.Context) (service.AlertScanResult, error) {
		current := int(count.Add(1))
		calls <- current
		return service.AlertScanResult{EvaluationCount: 17}, nil
	})
	deliveryStarted := make(chan struct{})
	deliveries := alertDeliveryRunnerFunc(func(ctx context.Context) error {
		close(deliveryStarted)
		<-ctx.Done()
		return nil
	})
	runtime, err := NewAlertRuntime(AlertRuntimeOptions{Evaluator: evaluator, Deliveries: deliveries, Clock: clock})
	if err != nil {
		t.Fatalf("create alert runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("start alert runtime: %v", err)
	}
	if tickerCreations := clock.tickerCreations.Load(); tickerCreations != 1 {
		t.Fatalf("evaluation tickers created before Start returned = %d, want 1", tickerCreations)
	}
	waitAlertRuntimeCall(t, calls, 1)
	select {
	case <-deliveryStarted:
	case <-time.After(time.Second):
		t.Fatal("delivery worker did not start")
	}
	clock.Advance(5*time.Minute - time.Second)
	select {
	case call := <-calls:
		t.Fatalf("early evaluation call %d", call)
	default:
	}
	clock.Advance(time.Second)
	waitAlertRuntimeCall(t, calls, 2)
	stopContext, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopContext); err != nil {
		t.Fatalf("stop alert runtime: %v", err)
	}
}

func TestAlertRuntimeStopsBeforeDeliveryWhenStartupEvaluationFails(t *testing.T) {
	want := errors.New("snapshot failed")
	deliveryStarted := false
	runtime, err := NewAlertRuntime(AlertRuntimeOptions{
		Evaluator: alertEvaluationRunnerFunc(func(context.Context) (service.AlertScanResult, error) {
			return service.AlertScanResult{}, want
		}),
		Deliveries: alertDeliveryRunnerFunc(func(context.Context) error {
			deliveryStarted = true
			return nil
		}),
		Clock: testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
	})
	if err != nil {
		t.Fatalf("create alert runtime: %v", err)
	}
	if err := runtime.Run(context.Background()); !errors.Is(err, want) {
		t.Fatalf("runtime error = %v", err)
	}
	if deliveryStarted {
		t.Fatal("delivery worker started after failed recovery scan")
	}
}

type recoverableAlertDeliveryRunner struct {
	recoverErr error
	runCalled  bool
}

func (runner *recoverableAlertDeliveryRunner) Recover(context.Context) error {
	return runner.recoverErr
}

func (runner *recoverableAlertDeliveryRunner) Run(context.Context) error {
	runner.runCalled = true
	return nil
}

func TestAlertRuntimeDoesNotBecomeReadyWhenDeliveryRecoveryFails(t *testing.T) {
	want := errors.New("delivery recovery failed")
	deliveries := &recoverableAlertDeliveryRunner{recoverErr: want}
	runtime, err := NewAlertRuntime(AlertRuntimeOptions{
		Evaluator: alertEvaluationRunnerFunc(func(context.Context) (service.AlertScanResult, error) {
			return service.AlertScanResult{}, nil
		}),
		Deliveries: deliveries,
		Clock:      testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
	})
	if err != nil {
		t.Fatalf("create alert runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); !errors.Is(err, want) {
		t.Fatalf("start error = %v", err)
	}
	if runtime.Ready() || deliveries.runCalled {
		t.Fatalf("failed recovery ready=%t run_called=%t", runtime.Ready(), deliveries.runCalled)
	}
}

type orderedAlertDeliveryRunner struct {
	events  chan string
	started chan struct{}
}

func (runner *orderedAlertDeliveryRunner) Recover(context.Context) error {
	runner.events <- "recover"
	return nil
}

func (runner *orderedAlertDeliveryRunner) Run(ctx context.Context) error {
	runner.events <- "delivery"
	close(runner.started)
	<-ctx.Done()
	return nil
}

func TestAlertRuntimeScansThenRecoversBeforeStartingDelivery(t *testing.T) {
	events := make(chan string, 3)
	deliveries := &orderedAlertDeliveryRunner{events: events, started: make(chan struct{})}
	runtime, err := NewAlertRuntime(AlertRuntimeOptions{
		Evaluator: alertEvaluationRunnerFunc(func(context.Context) (service.AlertScanResult, error) {
			events <- "scan"
			return service.AlertScanResult{}, nil
		}),
		Deliveries: deliveries,
		Clock:      testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
	})
	if err != nil {
		t.Fatalf("create ordered alert runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start ordered alert runtime: %v", err)
	}
	select {
	case <-deliveries.started:
	case <-time.After(time.Second):
		t.Fatal("delivery runner did not start")
	}
	got := []string{<-events, <-events, <-events}
	want := []string{"scan", "recover", "delivery"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("startup events = %#v, want %#v", got, want)
		}
	}
	if !runtime.Ready() {
		t.Fatal("alert runtime did not report ready after recovery")
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Stop(stopContext); err != nil {
		t.Fatalf("stop ordered alert runtime: %v", err)
	}
}

func TestNewAlertRuntimeRequiresDependencies(t *testing.T) {
	if runtime, err := NewAlertRuntime(AlertRuntimeOptions{}); err == nil || runtime != nil {
		t.Fatalf("runtime=%#v error=%v", runtime, err)
	}
}

func TestAlertRuntimeStopHonorsDeadlineAndComponentCanFinishLater(t *testing.T) {
	canceled := make(chan struct{})
	release := make(chan struct{})
	runtime, err := NewAlertRuntime(AlertRuntimeOptions{
		Evaluator: alertEvaluationRunnerFunc(func(context.Context) (service.AlertScanResult, error) {
			return service.AlertScanResult{}, nil
		}),
		Deliveries: alertDeliveryRunnerFunc(func(ctx context.Context) error {
			<-ctx.Done()
			close(canceled)
			<-release
			return ctx.Err()
		}),
		Clock: testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
	})
	if err != nil {
		t.Fatalf("create blocking alert runtime: %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start blocking alert runtime: %v", err)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- runtime.Stop(stopContext) }()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("alert component was not canceled")
	}
	select {
	case err := <-stopDone:
		if !errors.Is(err, ErrRuntimeStopTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("alert runtime stop error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("alert runtime ignored its hard deadline")
	}
	close(release)
	runtime.mu.Lock()
	done := runtime.done
	runtime.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("alert component did not finish after release")
	}
}

func waitAlertRuntimeCall(t *testing.T, calls <-chan int, want int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("evaluation call = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("evaluation call %d did not arrive", want)
	}
}
