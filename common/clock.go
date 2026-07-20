package common

import "time"

// Clock isolates wall-clock access so scheduling and time-bound business rules
// can be exercised against deterministic time in tests.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
	NewTicker(time.Duration) Ticker
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

func (SystemClock) NewTimer(duration time.Duration) Timer {
	return systemTimer{timer: time.NewTimer(duration)}
}

func (SystemClock) NewTicker(duration time.Duration) Ticker {
	return systemTicker{ticker: time.NewTicker(duration)}
}

type systemTimer struct {
	timer *time.Timer
}

func (timer systemTimer) C() <-chan time.Time {
	return timer.timer.C
}

func (timer systemTimer) Stop() bool {
	return timer.timer.Stop()
}

func (timer systemTimer) Reset(duration time.Duration) bool {
	return timer.timer.Reset(duration)
}

type systemTicker struct {
	ticker *time.Ticker
}

func (ticker systemTicker) C() <-chan time.Time {
	return ticker.ticker.C
}

func (ticker systemTicker) Stop() {
	ticker.ticker.Stop()
}
