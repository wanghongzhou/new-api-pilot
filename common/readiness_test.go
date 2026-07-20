package common

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadinessCheckFailsClosedAtDeadlineForUncooperativeCheck(t *testing.T) {
	readiness := NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	readiness.AddCheck("database", func(context.Context) error {
		close(started)
		<-release
		close(finished)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	failures := readiness.Check(ctx)
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("Check() waited for an uncooperative check: %s", elapsed)
	}
	if !reflect.DeepEqual(failures, []string{"database"}) {
		t.Fatalf("Check() failures = %v, want [database]", failures)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("readiness check did not start")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("uncooperative readiness check did not finish after release")
	}
}

func TestReadinessCheckUsesDefaultDeadlineWithoutCallerDeadline(t *testing.T) {
	readiness := NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)
	finished := make(chan struct{})
	readiness.AddCheck("database", func(ctx context.Context) error {
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	})

	startedAt := time.Now()
	failures := readiness.Check(context.Background())
	if elapsed := time.Since(startedAt); elapsed < defaultReadinessTimeout-200*time.Millisecond || elapsed > defaultReadinessTimeout+time.Second {
		t.Fatalf("Check() without deadline took %s, want about %s", elapsed, defaultReadinessTimeout)
	}
	if !reflect.DeepEqual(failures, []string{"database"}) {
		t.Fatalf("Check() failures = %v, want [database]", failures)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("default-deadline readiness check did not finish")
	}
}

func TestReadinessCheckDoesNotAccumulateUncooperativeWorkers(t *testing.T) {
	readiness := NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)

	var starts atomic.Int32
	var startedOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	readiness.AddCheck("database", func(context.Context) error {
		starts.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		return nil
	})

	const requests = 16
	startRequests := make(chan struct{})
	results := make(chan []string, requests)
	var wait sync.WaitGroup
	for range requests {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-startRequests
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			results <- readiness.Check(ctx)
		}()
	}
	close(startRequests)
	wait.Wait()
	close(results)

	for failures := range results {
		if !reflect.DeepEqual(failures, []string{"database"}) {
			t.Fatalf("Check() failures = %v, want [database]", failures)
		}
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("readiness check did not start")
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("uncooperative readiness check started %d times, want 1", got)
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		readiness.mutex.RLock()
		_, running := readiness.inFlight["database"]
		readiness.mutex.RUnlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("readiness check remained in flight after release")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	failures := readiness.Check(ctx)
	cancel()
	if len(failures) != 0 {
		t.Fatalf("readiness retry failures = %v, want none", failures)
	}
	if got := starts.Load(); got != 2 {
		t.Fatalf("readiness check started %d times after retry, want 2", got)
	}
}

func TestReadinessCheckSharesSuccessfulInFlightResult(t *testing.T) {
	readiness := NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)

	var starts atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	readiness.AddCheck("database", func(context.Context) error {
		starts.Add(1)
		close(started)
		<-release
		return nil
	})

	first := make(chan []string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		first <- readiness.Check(ctx)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first readiness check did not start")
	}

	second := make(chan []string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		second <- readiness.Check(ctx)
	}()
	select {
	case failures := <-second:
		t.Fatalf("concurrent readiness check returned before the shared result: %v", failures)
	case <-time.After(25 * time.Millisecond):
	}

	close(release)
	for name, results := range map[string]chan []string{"first": first, "second": second} {
		select {
		case failures := <-results:
			if len(failures) != 0 {
				t.Fatalf("%s readiness failures = %v, want none", name, failures)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s readiness check did not finish", name)
		}
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("successful readiness check started %d times, want 1", got)
	}
}

func TestReadinessCheckDoesNotShareCallerCancellation(t *testing.T) {
	readiness := NewReadiness()
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)

	started := make(chan struct{})
	release := make(chan struct{})
	readiness.AddCheck("database", func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	firstContext, cancelFirst := context.WithTimeout(context.Background(), time.Second)
	defer cancelFirst()
	first := make(chan []string, 1)
	go func() {
		first <- readiness.Check(firstContext)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first readiness check did not start")
	}

	second := make(chan []string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		second <- readiness.Check(ctx)
	}()
	cancelFirst()
	select {
	case failures := <-first:
		if !reflect.DeepEqual(failures, []string{"database"}) {
			t.Fatalf("cancelled readiness failures = %v, want [database]", failures)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled readiness check did not return")
	}
	select {
	case failures := <-second:
		t.Fatalf("second readiness check inherited caller cancellation: %v", failures)
	case <-time.After(25 * time.Millisecond):
	}

	close(release)
	select {
	case failures := <-second:
		if len(failures) != 0 {
			t.Fatalf("second readiness failures = %v, want none", failures)
		}
	case <-time.After(time.Second):
		t.Fatal("second readiness check did not finish")
	}
}

func TestReadinessCheckRecoversPanicsAndSortsFailures(t *testing.T) {
	readiness := NewReadiness()
	readiness.AddCheck("zeta", func(context.Context) error { panic("probe failed") })
	readiness.AddCheck("alpha", nil)

	failures := readiness.Check(context.Background())
	if !reflect.DeepEqual(failures, []string{"alpha", "runtime", "scheduler", "zeta"}) {
		t.Fatalf("Check() failures = %v", failures)
	}
}
