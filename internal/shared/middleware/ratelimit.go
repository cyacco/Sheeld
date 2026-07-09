package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/cyacco/Sheeld/internal/shared/response"
)

const (
	// limiterIdleTTL is how long an unused per-key limiter is kept before a
	// sweep evicts it. Long enough that active keys are never dropped
	// mid-burst, short enough to bound memory under key churn.
	limiterIdleTTL = 10 * time.Minute
	// limiterSweepInterval is how often idle limiters are swept.
	limiterSweepInterval = 5 * time.Minute
)

// limiterEntry pairs a token-bucket limiter with the last time it was used,
// so idle entries can be evicted.
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter provides per-key rate limiting using an in-memory token bucket.
// Idle keys are periodically evicted so the limiter map doesn't grow without
// bound under many distinct keys (orgs/IPs) over the process lifetime.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	rps      rate.Limit
	burst    int
}

// NewRateLimiter creates a new per-key rate limiter and starts a background
// sweeper that evicts idle entries.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*limiterEntry),
		rps:      rate.Limit(rps),
		burst:    burst,
	}
	go rl.sweepLoop()
	return rl
}

// getLimiter returns the rate limiter for the given key, creating one if
// needed, and records the access time.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.limiters[key]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// sweepLoop periodically removes limiters idle beyond limiterIdleTTL.
func (rl *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(limiterSweepInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.evictIdle(time.Now())
	}
}

// evictIdle removes limiters whose last use is older than limiterIdleTTL
// relative to now. Returns the number evicted.
func (rl *RateLimiter) evictIdle(now time.Time) int {
	cutoff := now.Add(-limiterIdleTTL)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	evicted := 0
	for key, e := range rl.limiters {
		if e.lastSeen.Before(cutoff) {
			delete(rl.limiters, key)
			evicted++
		}
	}
	return evicted
}

// size returns the current number of tracked limiters (test helper).
func (rl *RateLimiter) size() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.limiters)
}

// Middleware returns HTTP middleware that rate-limits based on the authenticated
// organization ID from the request context (set by APIKeyAuth).
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use org ID from context as the rate limit key
		key := "anonymous"
		if orgID := OrgIDFromContext(r.Context()); orgID.String() != "00000000-0000-0000-0000-000000000000" {
			key = orgID.String()
		}

		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			response.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}
