package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	testsupport "new-api-pilot/tests/support"
)

type countingLogRetentionCleaner struct {
	calls  atomic.Int64
	called chan struct{}
}

func (cleaner *countingLogRetentionCleaner) Clean(context.Context) (int64, error) {
	cleaner.calls.Add(1)
	select {
	case cleaner.called <- struct{}{}:
	default:
	}
	return 1, nil
}

func TestUpstreamLogRetentionRuntimeRunsDailyAndStops(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(2_100_000_000, 0))
	cleaner := &countingLogRetentionCleaner{called: make(chan struct{}, 1)}
	runtime, err := NewUpstreamLogRetentionRuntime(cleaner, clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil || !runtime.Ready() {
		t.Fatalf("start log retention runtime: %v", err)
	}
	clock.Advance(24 * time.Hour)
	select {
	case <-cleaner.called:
	case <-time.After(time.Second):
		t.Fatal("daily log retention did not run")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopCtx); err != nil || runtime.Ready() || cleaner.calls.Load() != 1 {
		t.Fatalf("stop log retention runtime calls=%d ready=%v err=%v", cleaner.calls.Load(), runtime.Ready(), err)
	}
}
