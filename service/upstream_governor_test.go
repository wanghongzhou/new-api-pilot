package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"new-api-pilot/tests/support"
)

func TestUpstreamGovernorSpacesRequestsAndHonorsCooldown(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	governor, err := NewUpstreamGovernor(UpstreamGovernorOptions{Requests: 10, Window: time.Second, MaxInFlight: 2, Clock: clock})
	if err != nil {
		t.Fatalf("create governor: %v", err)
	}
	firstRelease, err := governor.Acquire(context.Background(), "http://upstream.local", UpstreamRequestBulk)
	if err != nil {
		t.Fatalf("acquire first: %v", err)
	}
	firstRelease()
	second := make(chan error, 1)
	go acquireForGovernorTest(governor, context.Background(), UpstreamRequestBulk, second)
	assertGovernorBlocked(t, second)
	clock.Advance(99 * time.Millisecond)
	assertGovernorBlocked(t, second)
	clock.Advance(time.Millisecond)
	if err := waitGovernorResult(t, second); err != nil {
		t.Fatalf("acquire spaced request: %v", err)
	}

	governor.ObserveRateLimit("http://upstream.local", 0, false)
	third := make(chan error, 1)
	go acquireForGovernorTest(governor, context.Background(), UpstreamRequestRoutine, third)
	clock.Advance(999 * time.Millisecond)
	assertGovernorBlocked(t, third)
	clock.Advance(time.Millisecond)
	if err := waitGovernorResult(t, third); err != nil {
		t.Fatalf("acquire after cooldown: %v", err)
	}
}

func TestUpstreamGovernorCanceledWaiterDoesNotLeakCapacity(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	governor, err := NewUpstreamGovernor(UpstreamGovernorOptions{Requests: 10, Window: time.Second, MaxInFlight: 1, Clock: clock})
	if err != nil {
		t.Fatalf("create governor: %v", err)
	}
	firstRelease, err := governor.Acquire(context.Background(), "http://upstream.local", UpstreamRequestRoutine)
	if err != nil {
		t.Fatalf("acquire first: %v", err)
	}
	waitContext, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go acquireForGovernorTest(governor, waitContext, UpstreamRequestBulk, canceled)
	cancel()
	if err := waitGovernorResult(t, canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquire error = %v", err)
	}
	third := make(chan error, 1)
	go acquireForGovernorTest(governor, context.Background(), UpstreamRequestRoutine, third)
	firstRelease()
	clock.Advance(100 * time.Millisecond)
	if err := waitGovernorResult(t, third); err != nil {
		t.Fatalf("acquire after canceled waiter: %v", err)
	}
}

func acquireForGovernorTest(governor UpstreamGovernor, ctx context.Context, class UpstreamRequestClass, result chan<- error) {
	release, err := governor.Acquire(ctx, "http://upstream.local", class)
	if err == nil {
		release()
	}
	result <- err
}

func assertGovernorBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("request unexpectedly completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitGovernorResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for governor")
		return nil
	}
}
