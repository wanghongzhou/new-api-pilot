package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

type resourceRetentionSettingsStub struct {
	settings model.CollectorSettings
	err      error
	calls    int
}

func (reader *resourceRetentionSettingsStub) Load(context.Context) (model.CollectorSettings, error) {
	reader.calls++
	return reader.settings, reader.err
}

type resourceRetentionCleanerStub struct {
	mu      sync.Mutex
	reports []service.ResourceRetentionResult
	errors  []error
	days    []int
	started chan struct{}
	release chan struct{}
}

func (cleaner *resourceRetentionCleanerStub) Clean(ctx context.Context, days int) (service.ResourceRetentionResult, error) {
	cleaner.mu.Lock()
	cleaner.days = append(cleaner.days, days)
	var report service.ResourceRetentionResult
	if len(cleaner.reports) > 0 {
		report = cleaner.reports[0]
		cleaner.reports = cleaner.reports[1:]
	}
	var err error
	if len(cleaner.errors) > 0 {
		err = cleaner.errors[0]
		cleaner.errors = cleaner.errors[1:]
	}
	started, release := cleaner.started, cleaner.release
	cleaner.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return service.ResourceRetentionResult{}, ctx.Err()
		}
	}
	return report, err
}

func (cleaner *resourceRetentionCleanerStub) callDays() []int {
	cleaner.mu.Lock()
	defer cleaner.mu.Unlock()
	return append([]int(nil), cleaner.days...)
}

func TestResourceRetentionRuntimeSchedulesAt0330RetriesAndDeduplicatesDay(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	clock := testsupport.NewFakeClock(time.Date(2026, time.January, 17, 3, 29, 0, 0, location))
	settings := &resourceRetentionSettingsStub{settings: model.CollectorSettings{MinuteRetentionDays: 90}}
	cleaner := &resourceRetentionCleanerStub{reports: []service.ResourceRetentionResult{
		{RetentionDays: 90, Complete: false},
		{RetentionDays: 90, Complete: true},
		{RetentionDays: 90, Complete: true},
	}}
	runtime, err := NewResourceRetentionRuntime(ResourceRetentionRuntimeOptions{
		Cleaner: cleaner, Settings: settings, Clock: clock, Logf: func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("create resource retention runtime: %v", err)
	}
	result, err := runtime.RunOnce(context.Background())
	if err != nil || result.Attempted || settings.calls != 0 {
		t.Fatalf("pre-schedule result=%#v settings=%d err=%v", result, settings.calls, err)
	}
	clock.Advance(time.Minute)
	first, err := runtime.RunOnce(context.Background())
	if err != nil || !first.Attempted || first.Completed {
		t.Fatalf("first scheduled result=%#v err=%v", first, err)
	}
	second, err := runtime.RunOnce(context.Background())
	if err != nil || !second.Attempted || !second.Completed {
		t.Fatalf("retry result=%#v err=%v", second, err)
	}
	deduplicated, err := runtime.RunOnce(context.Background())
	if err != nil || deduplicated.Attempted {
		t.Fatalf("same-day result=%#v err=%v", deduplicated, err)
	}
	clock.Advance(24 * time.Hour)
	nextDay, err := runtime.RunOnce(context.Background())
	if err != nil || !nextDay.Attempted || !nextDay.Completed {
		t.Fatalf("next-day result=%#v err=%v", nextDay, err)
	}
	if got := cleaner.callDays(); len(got) != 3 || got[0] != 90 || got[1] != 90 || got[2] != 90 {
		t.Fatalf("retention cleaner days = %#v", got)
	}
}

func TestResourceRetentionRuntimeFailsSafeForInvalidSettingsAndRetriesErrors(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	clock := testsupport.NewFakeClock(time.Date(2026, time.January, 17, 3, 30, 0, 0, location))
	settings := &resourceRetentionSettingsStub{settings: model.CollectorSettings{MinuteRetentionDays: 0}}
	cleaner := &resourceRetentionCleanerStub{}
	runtime, err := NewResourceRetentionRuntime(ResourceRetentionRuntimeOptions{
		Cleaner: cleaner, Settings: settings, Clock: clock, Logf: func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("create resource retention runtime: %v", err)
	}
	result, runErr := runtime.RunOnce(context.Background())
	if !errors.Is(runErr, service.ErrResourceRetentionInvalid) || !result.Attempted || len(cleaner.callDays()) != 0 {
		t.Fatalf("invalid setting result=%#v calls=%#v err=%v", result, cleaner.callDays(), runErr)
	}
	settings.settings.MinuteRetentionDays = 90
	cleaner.errors = []error{errors.New("injected retention failure")}
	if _, runErr = runtime.RunOnce(context.Background()); runErr == nil {
		t.Fatal("injected cleanup failure was accepted")
	}
	cleaner.reports = []service.ResourceRetentionResult{{RetentionDays: 90, Complete: true}}
	result, runErr = runtime.RunOnce(context.Background())
	if runErr != nil || !result.Completed {
		t.Fatalf("cleanup retry result=%#v err=%v", result, runErr)
	}
}

func TestResourceRetentionRuntimeStartDoesNotBlockCollectionAdmission(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	clock := testsupport.NewFakeClock(time.Date(2026, time.January, 17, 3, 30, 0, 0, location))
	settings := &resourceRetentionSettingsStub{settings: model.CollectorSettings{MinuteRetentionDays: 90}}
	cleaner := &resourceRetentionCleanerStub{
		reports: []service.ResourceRetentionResult{{RetentionDays: 90, Complete: true}},
		started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	runtime, err := NewResourceRetentionRuntime(ResourceRetentionRuntimeOptions{
		Cleaner: cleaner, Settings: settings, Clock: clock, Logf: func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("create resource retention runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startedAt := time.Now()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("start resource retention runtime: %v", err)
	}
	if time.Since(startedAt) > 100*time.Millisecond || !runtime.Ready() {
		t.Fatalf("retention runtime blocked start or was not ready")
	}
	select {
	case <-cleaner.started:
	case <-time.After(time.Second):
		t.Fatal("background cleanup did not start")
	}
	if !runtime.Ready() {
		t.Fatal("blocking cleanup changed runtime readiness")
	}
	close(cleaner.release)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopCtx); err != nil {
		t.Fatalf("stop resource retention runtime: %v", err)
	}
}
