package common

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"
)

var ErrRuntimeNotInitialized = errors.New("runtime initialization has not completed")

const defaultReadinessTimeout = 2 * time.Second

type ReadinessCheck func(context.Context) error

type readinessCheckRun struct {
	done   chan struct{}
	failed bool
}

type Readiness struct {
	mutex          sync.RWMutex
	initialized    bool
	schedulerReady bool
	checks         map[string]ReadinessCheck
	inFlight       map[string]*readinessCheckRun
}

func NewReadiness() *Readiness {
	return &Readiness{
		checks:   make(map[string]ReadinessCheck),
		inFlight: make(map[string]*readinessCheckRun),
	}
}

func (readiness *Readiness) AddCheck(name string, check ReadinessCheck) {
	readiness.mutex.Lock()
	defer readiness.mutex.Unlock()
	readiness.checks[name] = check
}

func (readiness *Readiness) SetInitialized(initialized bool) {
	readiness.mutex.Lock()
	defer readiness.mutex.Unlock()
	readiness.initialized = initialized
}

func (readiness *Readiness) SetSchedulerReady(ready bool) {
	readiness.mutex.Lock()
	defer readiness.mutex.Unlock()
	readiness.schedulerReady = ready
}

func (readiness *Readiness) Check(ctx context.Context) []string {
	ctx, cancel := readinessWaitContext(ctx)
	defer cancel()

	failures := make(map[string]struct{})
	addFailure := func(name string) {
		failures[name] = struct{}{}
	}

	readiness.mutex.Lock()
	initialized := readiness.initialized
	schedulerReady := readiness.schedulerReady
	if !initialized {
		addFailure("runtime")
	}
	if !schedulerReady {
		addFailure("scheduler")
	}

	if ctx.Err() != nil {
		for name := range readiness.checks {
			addFailure(name)
		}
		readiness.mutex.Unlock()
		return sortedReadinessFailures(failures)
	}

	pending := make(map[string]*readinessCheckRun, len(readiness.checks))
	for name, check := range readiness.checks {
		run, running := readiness.inFlight[name]
		if !running {
			run = &readinessCheckRun{done: make(chan struct{})}
			readiness.inFlight[name] = run
			checkContext, cancelCheck := readinessCheckContext(ctx)
			go readiness.runCheck(name, check, checkContext, cancelCheck, run)
		}
		pending[name] = run
	}
	readiness.mutex.Unlock()
	readiness.collectCheckFailures(ctx, pending, addFailure)
	return sortedReadinessFailures(failures)
}

func readinessWaitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultReadinessTimeout)
}

func readinessCheckContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, _ := ctx.Deadline()
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

func (readiness *Readiness) collectCheckFailures(ctx context.Context, pending map[string]*readinessCheckRun, addFailure func(string)) {
	cases := make([]reflect.SelectCase, 1, len(pending)+1)
	cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
	names := make([]string, 1, len(pending)+1)
	runs := make([]*readinessCheckRun, 1, len(pending)+1)
	for name, run := range pending {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(run.done)})
		names = append(names, name)
		runs = append(runs, run)
	}

	for remaining := len(pending); remaining > 0; {
		selected, _, _ := reflect.Select(cases)
		if selected == 0 {
			// An arbitrary readiness check cannot be forcibly stopped. Treat
			// unfinished checks as failed rather than serving stale readiness.
			for index := 1; index < len(cases); index++ {
				if cases[index].Chan.IsValid() {
					addFailure(names[index])
				}
			}
			return
		}

		if runs[selected].failed {
			addFailure(names[selected])
		}
		cases[selected].Chan = reflect.Value{}
		remaining--
	}
}

func (readiness *Readiness) runCheck(
	name string,
	check ReadinessCheck,
	ctx context.Context,
	cancel context.CancelFunc,
	run *readinessCheckRun,
) {
	defer cancel()
	failed := readinessCheckFailed(ctx, check)
	readiness.mutex.Lock()
	run.failed = failed
	if readiness.inFlight[name] == run {
		delete(readiness.inFlight, name)
	}
	close(run.done)
	readiness.mutex.Unlock()
}

func readinessCheckFailed(ctx context.Context, check ReadinessCheck) (failed bool) {
	if check == nil {
		return true
	}
	defer func() {
		if recover() != nil {
			failed = true
		}
	}()
	return check(ctx) != nil
}

func sortedReadinessFailures(failures map[string]struct{}) []string {
	result := make([]string, 0, len(failures))
	for name := range failures {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
