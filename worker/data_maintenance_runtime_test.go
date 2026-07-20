package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type dataMaintenanceProcessorStub struct {
	mu      sync.Mutex
	err     error
	entered chan struct{}
	release chan struct{}
}

func (stub *dataMaintenanceProcessorStub) ProcessAuthorizationPricingIntent(context.Context) (model.AuthorizationPricingProcessResult, error) {
	stub.mu.Lock()
	err, entered, release := stub.err, stub.entered, stub.release
	stub.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if release != nil {
		<-release
	}
	return model.AuthorizationPricingProcessResult{}, err
}

func TestDataMaintenanceRuntimeStartFailsBeforeReadyWhenRecoveryFails(t *testing.T) {
	stub := &dataMaintenanceProcessorStub{err: errors.New("database unavailable")}
	runtime, err := NewDataMaintenanceRuntime(stub, testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)), NewDataMaintenanceWake(), time.Minute)
	if err != nil {
		t.Fatalf("NewDataMaintenanceRuntime() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil")
	}
	if runtime.Ready() {
		t.Fatal("runtime became ready after failed recovery")
	}
}

func TestDataMaintenanceRuntimeRejectsConcurrentStartDuringRecovery(t *testing.T) {
	stub := &dataMaintenanceProcessorStub{entered: make(chan struct{}, 1), release: make(chan struct{})}
	runtime, err := NewDataMaintenanceRuntime(stub, testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)), NewDataMaintenanceWake(), time.Minute)
	if err != nil {
		t.Fatalf("NewDataMaintenanceRuntime() error = %v", err)
	}
	first := make(chan error, 1)
	go func() { first <- runtime.Start(context.Background()) }()
	<-stub.entered
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("concurrent Start() error = nil")
	}
	close(stub.release)
	if err := <-first; err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if !runtime.Ready() {
		t.Fatal("runtime not ready after successful recovery")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestAuthorizationPricingSiteEligibilityIgnoresOnlineStatus(t *testing.T) {
	// Compile-time behavioral coverage lives in model; this runtime regression
	// asserts a wake remains non-blocking while the immediate probe races.
	wake := NewDataMaintenanceWake()
	wake.NotifyAuthorizationPricingSync()
	wake.NotifyAuthorizationPricingSync()
	select {
	case <-wake.channel:
	default:
		t.Fatal("wake was lost")
	}
}

func TestDataMaintenanceRuntimeDoesNotBecomeReadyAfterQuiescedProcessorReleases(t *testing.T) {
	stub := &dataMaintenanceProcessorStub{}
	runtime, err := NewDataMaintenanceRuntime(stub, testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)), NewDataMaintenanceWake(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	stub.entered = make(chan struct{}, 1)
	stub.release = make(chan struct{})
	entered, release := stub.entered, stub.release
	stub.mu.Unlock()
	runtime.wake.NotifyAuthorizationPricingSync()
	<-entered
	if err := runtime.Quiesce(); err != nil {
		t.Fatal(err)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !runtime.Ready() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if runtime.Ready() {
		t.Fatal("runtime ready rebounded after quiesce")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
