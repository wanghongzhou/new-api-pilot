package service

import (
	"sync"
	"time"

	"new-api-pilot/common"
)

const (
	loginFailureWindow = 10 * time.Minute
	loginBlockDuration = 15 * time.Minute
	loginFailureLimit  = 5
)

type LoginRateLimitedError struct {
	RetryAfter time.Duration
}

func (err LoginRateLimitedError) Error() string {
	return "login rate limit exceeded"
}

type loginAttempt struct {
	failures     []time.Time
	blockedUntil time.Time
}

type LoginLimiter struct {
	mutex    sync.Mutex
	attempts map[string]loginAttempt
	clock    common.Clock
}

func NewLoginLimiter(clock common.Clock) *LoginLimiter {
	if clock == nil {
		clock = common.SystemClock{}
	}
	return &LoginLimiter{attempts: make(map[string]loginAttempt), clock: clock}
}

func (limiter *LoginLimiter) Check(key string) error {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock.Now()
	attempt := limiter.attempts[key]
	if attempt.blockedUntil.After(now) {
		return LoginRateLimitedError{RetryAfter: attempt.blockedUntil.Sub(now)}
	}
	attempt.failures = recentFailures(attempt.failures, now)
	if len(attempt.failures) >= loginFailureLimit {
		attempt.blockedUntil = now.Add(loginBlockDuration)
		limiter.attempts[key] = attempt
		return LoginRateLimitedError{RetryAfter: loginBlockDuration}
	}
	if len(attempt.failures) == 0 {
		delete(limiter.attempts, key)
	} else {
		attempt.blockedUntil = time.Time{}
		limiter.attempts[key] = attempt
	}
	return nil
}

func (limiter *LoginLimiter) RecordFailure(key string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock.Now()
	attempt := limiter.attempts[key]
	attempt.failures = append(recentFailures(attempt.failures, now), now)
	limiter.attempts[key] = attempt
}

func (limiter *LoginLimiter) Reset(key string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	delete(limiter.attempts, key)
}

func recentFailures(failures []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-loginFailureWindow)
	result := failures[:0]
	for _, failure := range failures {
		if !failure.Before(cutoff) {
			result = append(result, failure)
		}
	}
	return result
}
