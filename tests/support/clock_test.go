package testsupport

import (
	"testing"
	"time"
)

func TestFakeClockTimerAndTicker(t *testing.T) {
	start := time.Date(2026, time.January, 17, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	clock := NewFakeClock(start)
	timer := clock.NewTimer(90 * time.Second)
	ticker := clock.NewTicker(time.Minute)

	clock.Advance(time.Minute)
	assertTime(t, ticker.C(), start.Add(time.Minute))
	assertNoTime(t, timer.C())

	clock.Advance(30 * time.Second)
	assertTime(t, timer.C(), start.Add(90*time.Second))

	clock.Advance(30 * time.Second)
	assertTime(t, ticker.C(), start.Add(2*time.Minute))
	ticker.Stop()
	clock.Advance(time.Minute)
	assertNoTime(t, ticker.C())
}

func TestFakeClockReset(t *testing.T) {
	start := time.Unix(1768622400, 0)
	clock := NewFakeClock(start)
	timer := clock.NewTimer(time.Minute)

	if !timer.Stop() {
		t.Fatal("active timer Stop returned false")
	}
	if timer.Reset(2 * time.Minute) {
		t.Fatal("stopped timer Reset returned true")
	}
	clock.Advance(2 * time.Minute)
	assertTime(t, timer.C(), start.Add(2*time.Minute))
}

func assertTime(t *testing.T, channel <-chan time.Time, want time.Time) {
	t.Helper()
	select {
	case got := <-channel:
		if !got.Equal(want) {
			t.Fatalf("time = %s, want %s", got, want)
		}
	default:
		t.Fatalf("expected time %s", want)
	}
}

func assertNoTime(t *testing.T, channel <-chan time.Time) {
	t.Helper()
	select {
	case got := <-channel:
		t.Fatalf("unexpected time %s", got)
	default:
	}
}
