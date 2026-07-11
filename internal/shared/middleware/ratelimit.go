package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
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
// needed with the given rps/burst, and records the access time. If the key's
// limits changed since the limiter was created (e.g. a per-key override was
// edited), the existing limiter is updated in place.
func (rl *RateLimiter) getLimiter(key string, rps rate.Limit, burst int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.limiters[key]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(rps, burst)}
		rl.limiters[key] = e
	} else {
		if e.limiter.Limit() != rps {
			e.limiter.SetLimit(rps)
		}
		if e.limiter.Burst() != burst {
			e.limiter.SetBurst(burst)
		}
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

// KeyFunc derives the rate-limit key and per-key limits for a request. A
// returned rps or burst <= 0 means "use the limiter's configured default".
type KeyFunc func(r *http.Request) (key string, rps float64, burst int)

// MiddlewareWith returns rate-limiting middleware that derives the bucket key
// and (optionally overridden) limits per request via keyFn.
func (rl *RateLimiter) MiddlewareWith(keyFn KeyFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, rps, burst := keyFn(r)
			limit := rl.rps
			if rps > 0 {
				limit = rate.Limit(rps)
			}
			if burst <= 0 {
				burst = rl.burst
			}

			limiter := rl.getLimiter(key, limit, burst)
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				response.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Middleware rate-limits by the authenticated organization ID from the request
// context (set by JWT/API-key auth), using the limiter's default limits.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return rl.MiddlewareWith(func(r *http.Request) (string, float64, int) {
		key := "anonymous"
		if orgID := OrgIDFromContext(r.Context()); orgID != uuid.Nil {
			key = orgID.String()
		}
		return key, 0, 0
	})(next)
}
