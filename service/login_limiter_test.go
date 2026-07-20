package service_test

import (
	"errors"
	"testing"
	"time"

	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestLoginLimiterBlocksSixthAttemptAndExpires(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	limiter := service.NewLoginLimiter(clock)
	const key = "127.0.0.1:admin"
	for attempt := 1; attempt <= 5; attempt++ {
		if err := limiter.Check(key); err != nil {
			t.Fatalf("attempt %d unexpectedly blocked: %v", attempt, err)
		}
		limiter.RecordFailure(key)
	}
	err := limiter.Check(key)
	var limited service.LoginRateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 15*time.Minute {
		t.Fatalf("sixth attempt error = %#v", err)
	}
	clock.Advance(15 * time.Minute)
	if err := limiter.Check(key); err != nil {
		t.Fatalf("attempt after block expiry = %v", err)
	}
}

func TestLoginLimiterResetClearsFailures(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Now())
	limiter := service.NewLoginLimiter(clock)
	limiter.RecordFailure("key")
	limiter.Reset("key")
	if err := limiter.Check("key"); err != nil {
		t.Fatalf("Check() after Reset() error = %v", err)
	}
}
