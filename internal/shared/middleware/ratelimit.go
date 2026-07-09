package middleware

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"github.com/cyacco/Sheeld/internal/shared/response"
)

// RateLimiter provides per-key rate limiting using an in-memory token bucket.
type RateLimiter struct {
	limiters sync.Map
	rps      rate.Limit
	burst    int
}

// NewRateLimiter creates a new per-key rate limiter.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		rps:   rate.Limit(rps),
		burst: burst,
	}
}

// getLimiter returns the rate limiter for the given key, creating one if needed.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	if v, ok := rl.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rl.rps, rl.burst)
	actual, _ := rl.limiters.LoadOrStore(key, limiter)
	return actual.(*rate.Limiter)
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
