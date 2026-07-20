package testsupport

import (
	"sync"
	"time"

	"new-api-pilot/common"
)

var _ common.Clock = (*FakeClock)(nil)

type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  map[*fakeTimer]struct{}
	tickers map[*fakeTicker]struct{}
}

func NewFakeClock(now time.Time) *FakeClock {
	return &FakeClock{
		now:     now,
		timers:  make(map[*fakeTimer]struct{}),
		tickers: make(map[*fakeTicker]struct{}),
	}
}

func (clock *FakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *FakeClock) NewTimer(duration time.Duration) common.Timer {
	if duration < 0 {
		duration = 0
	}

	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &fakeTimer{
		clock:    clock,
		channel:  make(chan time.Time, 1),
		deadline: clock.now.Add(duration),
		active:   true,
	}
	clock.timers[timer] = struct{}{}
	return timer
}

func (clock *FakeClock) NewTicker(duration time.Duration) common.Ticker {
	if duration <= 0 {
		panic("testsupport: non-positive ticker duration")
	}

	clock.mu.Lock()
	defer clock.mu.Unlock()
	ticker := &fakeTicker{
		clock:    clock,
		channel:  make(chan time.Time, 1),
		interval: duration,
		next:     clock.now.Add(duration),
		active:   true,
	}
	clock.tickers[ticker] = struct{}{}
	return ticker
}

func (clock *FakeClock) Advance(duration time.Duration) {
	if duration < 0 {
		panic("testsupport: cannot move clock backwards")
	}

	clock.mu.Lock()
	defer clock.mu.Unlock()
	target := clock.now.Add(duration)

	for {
		next, found := clock.nextEventAtOrBefore(target)
		if !found {
			clock.now = target
			return
		}

		clock.now = next
		for timer := range clock.timers {
			if timer.active && !timer.deadline.After(next) {
				timer.active = false
				nonBlockingSend(timer.channel, timer.deadline)
			}
		}
		for ticker := range clock.tickers {
			if !ticker.active {
				continue
			}
			for !ticker.next.After(next) {
				nonBlockingSend(ticker.channel, ticker.next)
				ticker.next = ticker.next.Add(ticker.interval)
			}
		}
	}
}

func (clock *FakeClock) Set(now time.Time) {
	clock.mu.Lock()
	current := clock.now
	clock.mu.Unlock()
	if now.Before(current) {
		panic("testsupport: cannot move clock backwards")
	}
	clock.Advance(now.Sub(current))
}

func (clock *FakeClock) nextEventAtOrBefore(target time.Time) (time.Time, bool) {
	var next time.Time
	found := false
	for timer := range clock.timers {
		if !timer.active || timer.deadline.After(target) {
			continue
		}
		if !found || timer.deadline.Before(next) {
			next = timer.deadline
			found = true
		}
	}
	for ticker := range clock.tickers {
		if !ticker.active || ticker.next.After(target) {
			continue
		}
		if !found || ticker.next.Before(next) {
			next = ticker.next
			found = true
		}
	}
	return next, found
}

func nonBlockingSend(channel chan time.Time, value time.Time) {
	select {
	case channel <- value:
	default:
	}
}

type fakeTimer struct {
	clock    *FakeClock
	channel  chan time.Time
	deadline time.Time
	active   bool
}

func (timer *fakeTimer) C() <-chan time.Time {
	return timer.channel
}

func (timer *fakeTimer) Stop() bool {
	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	wasActive := timer.active
	timer.active = false
	return wasActive
}

func (timer *fakeTimer) Reset(duration time.Duration) bool {
	if duration < 0 {
		duration = 0
	}

	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	wasActive := timer.active
	select {
	case <-timer.channel:
	default:
	}
	timer.deadline = timer.clock.now.Add(duration)
	timer.active = true
	return wasActive
}

type fakeTicker struct {
	clock    *FakeClock
	channel  chan time.Time
	interval time.Duration
	next     time.Time
	active   bool
}

func (ticker *fakeTicker) C() <-chan time.Time {
	return ticker.channel
}

func (ticker *fakeTicker) Stop() {
	ticker.clock.mu.Lock()
	defer ticker.clock.mu.Unlock()
	ticker.active = false
}
