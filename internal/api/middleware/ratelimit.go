package middleware

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/sheeld/sheeld/internal/api/response"
)

const (
	defaultIdleTTL      = 10 * time.Minute
	defaultCleanupEvery = 1 * time.Minute
)

// limiterEntry wraps a rate.Limiter with a last-access timestamp used for
// idle eviction. lastAccess is stored as unix nanoseconds and updated
// atomically so reads on the hot path don't need to hold a lock.
type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess atomic.Int64
}

// RateLimiter provides per-key rate limiting using an in-memory token bucket.
// Idle entries are evicted by a background goroutine to bound memory growth.
type RateLimiter struct {
	limiters sync.Map
	rps      rate.Limit
	burst    int

	// IdleTTL is how long an entry can sit unused before being evicted.
	// Exposed as a field so tests can set short values.
	IdleTTL time.Duration
	// CleanupEvery is the interval at which the cleanup goroutine runs.
	// Exposed as a field so tests can set short values.
	CleanupEvery time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewRateLimiter creates a new per-key rate limiter and starts a background
// goroutine that evicts idle entries. Call Stop to shut down the goroutine.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rps:          rate.Limit(rps),
		burst:        burst,
		IdleTTL:      defaultIdleTTL,
		CleanupEvery: defaultCleanupEvery,
		stopCh:       make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Stop halts the background cleanup goroutine. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

// cleanupLoop periodically evicts idle entries until Stop is called.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.CleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.evictIdle()
		}
	}
}

// evictIdle walks the map and removes entries whose last access is older
// than IdleTTL.
func (rl *RateLimiter) evictIdle() {
	cutoff := time.Now().Add(-rl.IdleTTL).UnixNano()
	rl.limiters.Range(func(key, value any) bool {
		entry := value.(*limiterEntry)
		if entry.lastAccess.Load() < cutoff {
			rl.limiters.Delete(key)
		}
		return true
	})
}

// getLimiter returns the rate limiter for the given key, creating one if needed.
// It also bumps the entry's last-access timestamp.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	now := time.Now().UnixNano()
	if v, ok := rl.limiters.Load(key); ok {
		entry := v.(*limiterEntry)
		entry.lastAccess.Store(now)
		return entry.limiter
	}
	entry := &limiterEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
	entry.lastAccess.Store(now)
	actual, loaded := rl.limiters.LoadOrStore(key, entry)
	stored := actual.(*limiterEntry)
	if loaded {
		stored.lastAccess.Store(now)
	}
	return stored.limiter
}

// size returns the number of entries currently held. Intended for tests.
func (rl *RateLimiter) size() int {
	n := 0
	rl.limiters.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
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
