package main

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type orderedRuntime struct {
	name       string
	events     *[]string
	ready      bool
	readyStart bool
	startErr   error
	stopErr    error
	eventsMu   *sync.Mutex
}

func (runtime *orderedRuntime) record(event string) {
	if runtime.eventsMu != nil {
		runtime.eventsMu.Lock()
		defer runtime.eventsMu.Unlock()
	}
	*runtime.events = append(*runtime.events, event)
}

func (runtime *orderedRuntime) Start(context.Context) error {
	runtime.record("start:" + runtime.name)
	if runtime.startErr != nil {
		return runtime.startErr
	}
	runtime.ready = runtime.readyStart
	return nil
}

func (runtime *orderedRuntime) Quiesce() error {
	runtime.record("quiesce:" + runtime.name)
	runtime.ready = false
	return nil
}

func (runtime *orderedRuntime) Stop(context.Context) error {
	runtime.record("stop:" + runtime.name)
	runtime.ready = false
	return runtime.stopErr
}

func (runtime *orderedRuntime) Ready() bool { return runtime.ready }

func TestRuntimeGroupRollsBackStartedComponentsInReverseOrder(t *testing.T) {
	events := []string{}
	want := errors.New("export startup failed")
	collection := &orderedRuntime{name: "collection", events: &events, readyStart: true}
	alerts := &orderedRuntime{name: "alerts", events: &events, readyStart: true}
	exports := &orderedRuntime{name: "exports", events: &events, startErr: want}
	group, err := newRuntimeGroup(collection, alerts, exports)
	if err != nil {
		t.Fatalf("create runtime group: %v", err)
	}
	if err := group.Start(context.Background()); !errors.Is(err, want) {
		t.Fatalf("start error = %v", err)
	}
	wantEvents := []string{
		"start:collection", "start:alerts", "start:exports",
		"quiesce:alerts", "quiesce:collection", "stop:alerts", "stop:collection",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("rollback events = %#v, want %#v", events, wantEvents)
	}
	if group.Ready() {
		t.Fatal("failed runtime group reported ready")
	}
}

func TestRuntimeGroupQuiescesAndStopsInReverseOrder(t *testing.T) {
	events := []string{}
	var eventsMu sync.Mutex
	collection := &orderedRuntime{name: "collection", events: &events, eventsMu: &eventsMu, readyStart: true}
	alerts := &orderedRuntime{name: "alerts", events: &events, eventsMu: &eventsMu, readyStart: true}
	group, err := newRuntimeGroup(collection, alerts)
	if err != nil {
		t.Fatalf("create runtime group: %v", err)
	}
	if err := group.Start(context.Background()); err != nil {
		t.Fatalf("start runtime group: %v", err)
	}
	if !group.Ready() {
		t.Fatal("started runtime group did not report ready")
	}
	if err := group.Quiesce(); err != nil {
		t.Fatalf("quiesce runtime group: %v", err)
	}
	if err := group.Stop(context.Background()); err != nil {
		t.Fatalf("stop runtime group: %v", err)
	}
	wantPrefix := []string{
		"start:collection", "start:alerts",
		"quiesce:alerts", "quiesce:collection",
	}
	if len(events) != 6 || !reflect.DeepEqual(events[:4], wantPrefix) {
		t.Fatalf("shutdown events = %#v, want prefix %#v", events, wantPrefix)
	}
	stops := append([]string(nil), events[4:]...)
	sort.Strings(stops)
	if !reflect.DeepEqual(stops, []string{"stop:alerts", "stop:collection"}) {
		t.Fatalf("shutdown stop events = %#v", stops)
	}
}

func TestRuntimeGroupStopsComponentsConcurrentlyAndHonorsDeadline(t *testing.T) {
	release := make(chan struct{})
	first := &blockingRuntimeLifecycle{started: make(chan struct{}), release: release}
	second := &blockingRuntimeLifecycle{started: make(chan struct{}), release: release}
	group, err := newRuntimeGroup(first, second)
	if err != nil {
		t.Fatalf("create blocking runtime group: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- group.Stop(ctx) }()
	for _, started := range []<-chan struct{}{first.started, second.started} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("runtime group did not stop components concurrently")
		}
	}
	select {
	case err := <-stopDone:
		if !errors.Is(err, errRuntimeGroupStopTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runtime group timeout error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime group ignored its hard deadline")
	}
	close(release)
}

type blockingRuntimeLifecycle struct {
	started chan struct{}
	release <-chan struct{}
}

func (runtime *blockingRuntimeLifecycle) Start(context.Context) error { return nil }
func (runtime *blockingRuntimeLifecycle) Quiesce() error              { return nil }
func (runtime *blockingRuntimeLifecycle) Ready() bool                 { return true }
func (runtime *blockingRuntimeLifecycle) Stop(context.Context) error {
	close(runtime.started)
	<-runtime.release
	return nil
}
