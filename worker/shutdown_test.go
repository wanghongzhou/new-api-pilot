package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAwaitRuntimeStopCancelsBeforeHardDeadlineAndDoesNotWaitPastIt(t *testing.T) {
	done := make(chan struct{})
	canceled := make(chan struct{})
	var cancelOnce sync.Once
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	err := awaitRuntimeStop(ctx, done, func() {
		cancelOnce.Do(func() { close(canceled) })
	}, true, func() error { return nil })
	elapsed := time.Since(startedAt)
	if !errors.Is(err, ErrRuntimeStopTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runtime stop error = %v", err)
	}
	if elapsed < 75*time.Millisecond || elapsed > 400*time.Millisecond {
		t.Fatalf("runtime stop elapsed = %v", elapsed)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("execution was not canceled before the hard deadline")
	}
	close(done)
}
