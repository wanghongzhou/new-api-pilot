package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"new-api-pilot/common"
)

type UpstreamRequestClass uint8

const (
	UpstreamRequestRoutine UpstreamRequestClass = iota
	UpstreamRequestBulk
)

type upstreamRequestClassContextKey struct{}

func WithUpstreamRequestClass(ctx context.Context, class UpstreamRequestClass) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, upstreamRequestClassContextKey{}, class)
}

func upstreamRequestClassFromContext(ctx context.Context) UpstreamRequestClass {
	if ctx != nil {
		if class, ok := ctx.Value(upstreamRequestClassContextKey{}).(UpstreamRequestClass); ok && class == UpstreamRequestBulk {
			return class
		}
	}
	return UpstreamRequestRoutine
}

type UpstreamGovernor interface {
	Acquire(context.Context, string, UpstreamRequestClass) (func(), error)
	ObserveRateLimit(string, time.Duration, bool)
	Reconfigure(UpstreamGovernorOptions) error
}

type UpstreamGovernorOptions struct {
	Requests    int
	Window      time.Duration
	MaxInFlight int
	Clock       common.Clock
}

type upstreamGovernor struct {
	config   atomic.Pointer[upstreamGovernorConfig]
	clock    common.Clock
	mu       sync.Mutex
	origins  map[string]*upstreamOriginGovernor
	sequence atomic.Uint64
}

type upstreamGovernorConfig struct {
	interval    time.Duration
	window      time.Duration
	maxInFlight int
}

type upstreamAcquireRequest struct {
	id      uint64
	class   UpstreamRequestClass
	granted chan struct{}
}

type upstreamCancelRequest struct {
	id   uint64
	done chan struct{}
}

type upstreamCooldownRequest struct {
	until time.Time
	done  chan struct{}
}

type upstreamReloadRequest struct{ done chan struct{} }

type upstreamOriginGovernor struct {
	parent   *upstreamGovernor
	enqueue  chan upstreamAcquireRequest
	cancel   chan upstreamCancelRequest
	release  chan uint64
	cooldown chan upstreamCooldownRequest
	reload   chan upstreamReloadRequest
}

func NewUpstreamGovernor(options UpstreamGovernorOptions) (UpstreamGovernor, error) {
	config, err := normalizeUpstreamGovernorOptions(options)
	if err != nil {
		return nil, err
	}
	governor := &upstreamGovernor{clock: options.Clock, origins: make(map[string]*upstreamOriginGovernor)}
	governor.config.Store(config)
	return governor, nil
}

func normalizeUpstreamGovernorOptions(options UpstreamGovernorOptions) (*upstreamGovernorConfig, error) {
	if options.Requests == 0 {
		options.Requests = 300
	}
	if options.Window == 0 {
		options.Window = 180 * time.Second
	}
	if options.MaxInFlight == 0 {
		options.MaxInFlight = 4
	}
	if options.Requests < 0 || options.Window < 0 || options.MaxInFlight < 0 || options.Clock == nil {
		return nil, errors.New("upstream governor options are invalid")
	}
	interval := options.Window / time.Duration(options.Requests)
	if interval < 10*time.Millisecond {
		return nil, errors.New("upstream governor interval must be at least 10ms")
	}
	return &upstreamGovernorConfig{interval: interval, window: options.Window, maxInFlight: options.MaxInFlight}, nil
}

func (governor *upstreamGovernor) Reconfigure(options UpstreamGovernorOptions) error {
	if governor == nil {
		return errors.New("upstream governor is required")
	}
	options.Clock = governor.clock
	config, err := normalizeUpstreamGovernorOptions(options)
	if err != nil {
		return err
	}
	governor.config.Store(config)
	governor.mu.Lock()
	states := make([]*upstreamOriginGovernor, 0, len(governor.origins))
	for _, state := range governor.origins {
		states = append(states, state)
	}
	governor.mu.Unlock()
	for _, state := range states {
		request := upstreamReloadRequest{done: make(chan struct{})}
		state.reload <- request
		<-request.done
	}
	return nil
}

func (governor *upstreamGovernor) origin(origin string) *upstreamOriginGovernor {
	governor.mu.Lock()
	defer governor.mu.Unlock()
	state := governor.origins[origin]
	if state != nil {
		return state
	}
	state = &upstreamOriginGovernor{
		parent: governor, enqueue: make(chan upstreamAcquireRequest), cancel: make(chan upstreamCancelRequest),
		release: make(chan uint64), cooldown: make(chan upstreamCooldownRequest), reload: make(chan upstreamReloadRequest),
	}
	governor.origins[origin] = state
	go state.run()
	return state
}

func (governor *upstreamGovernor) Acquire(ctx context.Context, origin string, class UpstreamRequestClass) (func(), error) {
	if governor == nil || origin == "" || ctx == nil {
		return nil, errors.New("upstream governor acquire arguments are invalid")
	}
	state := governor.origin(origin)
	request := upstreamAcquireRequest{id: governor.sequence.Add(1), class: class, granted: make(chan struct{})}
	select {
	case state.enqueue <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-request.granted:
		var once sync.Once
		return func() { once.Do(func() { state.release <- request.id }) }, nil
	case <-ctx.Done():
		canceled := upstreamCancelRequest{id: request.id, done: make(chan struct{})}
		state.cancel <- canceled
		<-canceled.done
		return nil, ctx.Err()
	}
}

func (governor *upstreamGovernor) ObserveRateLimit(origin string, retryAfter time.Duration, hasRetryAfter bool) {
	if governor == nil || origin == "" {
		return
	}
	delay := governor.config.Load().window
	if hasRetryAfter && retryAfter > 0 {
		delay = retryAfter
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	until := governor.clock.Now().Add(delay)
	state := governor.origin(origin)
	request := upstreamCooldownRequest{until: until, done: make(chan struct{})}
	state.cooldown <- request
	<-request.done
}

func (state *upstreamOriginGovernor) run() {
	var routine, bulk []upstreamAcquireRequest
	granted := make(map[uint64]struct{})
	inFlight := 0
	nextAllowed := time.Time{}
	lastGrantedAt := time.Time{}
	cooldownUntil := time.Time{}
	fairnessIndex := 0
	timer := state.parent.clock.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C():
		default:
		}
	}
	var timerC <-chan time.Time

	remove := func(queue []upstreamAcquireRequest, id uint64) ([]upstreamAcquireRequest, bool) {
		for index := range queue {
			if queue[index].id == id {
				return append(queue[:index], queue[index+1:]...), true
			}
		}
		return queue, false
	}
	pick := func() (upstreamAcquireRequest, bool) {
		if len(routine) == 0 && len(bulk) == 0 {
			return upstreamAcquireRequest{}, false
		}
		if len(routine) == 0 {
			item := bulk[0]
			bulk = bulk[1:]
			return item, true
		}
		if len(bulk) == 0 {
			item := routine[0]
			routine = routine[1:]
			return item, true
		}
		useBulk := fairnessIndex == 3
		fairnessIndex = (fairnessIndex + 1) % 4
		if useBulk {
			item := bulk[0]
			bulk = bulk[1:]
			return item, true
		}
		item := routine[0]
		routine = routine[1:]
		return item, true
	}
	resetTimer := func(delay time.Duration) {
		if delay < 0 {
			delay = 0
		}
		if !timer.Stop() {
			select {
			case <-timer.C():
			default:
			}
		}
		timer.Reset(delay)
		timerC = timer.C()
	}
	dispatch := func() {
		timerC = nil
		config := state.parent.config.Load()
		if inFlight >= config.maxInFlight || (len(routine) == 0 && len(bulk) == 0) {
			return
		}
		now := state.parent.clock.Now()
		readyAt := nextAllowed
		if cooldownUntil.After(readyAt) {
			readyAt = cooldownUntil
		}
		if readyAt.After(now) {
			resetTimer(readyAt.Sub(now))
			return
		}
		request, ok := pick()
		if !ok {
			return
		}
		inFlight++
		granted[request.id] = struct{}{}
		lastGrantedAt = now
		nextAllowed = now.Add(config.interval)
		close(request.granted)
		if inFlight < config.maxInFlight && (len(routine) > 0 || len(bulk) > 0) {
			resetTimer(config.interval)
		}
	}

	for {
		dispatch()
		select {
		case request := <-state.enqueue:
			if request.class == UpstreamRequestBulk {
				bulk = append(bulk, request)
			} else {
				routine = append(routine, request)
			}
		case canceled := <-state.cancel:
			var removed bool
			routine, removed = remove(routine, canceled.id)
			if !removed {
				bulk, removed = remove(bulk, canceled.id)
			}
			if !removed {
				if _, exists := granted[canceled.id]; exists {
					delete(granted, canceled.id)
					inFlight--
				}
			}
			close(canceled.done)
		case id := <-state.release:
			if _, exists := granted[id]; exists {
				delete(granted, id)
				inFlight--
			}
		case request := <-state.cooldown:
			if request.until.After(cooldownUntil) {
				cooldownUntil = request.until
			}
			close(request.done)
		case request := <-state.reload:
			// Wake the dispatcher so a lower concurrency cap takes effect before
			// another grant, while preserving queues, in-flight work and cooldown.
			if !lastGrantedAt.IsZero() {
				candidate := lastGrantedAt.Add(state.parent.config.Load().interval)
				if candidate.After(nextAllowed) {
					nextAllowed = candidate
				}
			}
			close(request.done)
		case <-timerC:
			timerC = nil
		}
	}
}
