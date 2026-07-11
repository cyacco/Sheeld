package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRateLimiter(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(10, 10)
		handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		orgID := uuid.New()
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			ctx := context.WithValue(req.Context(), OrgIDKey, orgID)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("request %d: got status %d, want %d", i, rec.Code, http.StatusOK)
			}
		}
	})

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		rl := NewRateLimiter(1, 2) // 1 RPS, burst of 2
		handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		orgID := uuid.New()
		var rejected bool
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			ctx := context.WithValue(req.Context(), OrgIDKey, orgID)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code == http.StatusTooManyRequests {
				rejected = true

				// Verify Retry-After header
				if rec.Header().Get("Retry-After") != "1" {
					t.Error("missing Retry-After header")
				}

				// Verify error body
				var body map[string]string
				json.NewDecoder(rec.Body).Decode(&body)
				if body["error"] != "rate limit exceeded" {
					t.Errorf("got error %q, want %q", body["error"], "rate limit exceeded")
				}
				break
			}
		}

		if !rejected {
			t.Error("expected at least one request to be rejected")
		}
	})

	t.Run("separate limits per org", func(t *testing.T) {
		rl := NewRateLimiter(1, 2)
		handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		orgA := uuid.New()
		orgB := uuid.New()

		// Exhaust org A's burst
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			ctx := context.WithValue(req.Context(), OrgIDKey, orgA)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}

		// Org B should still be allowed
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		ctx := context.WithValue(req.Context(), OrgIDKey, orgB)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("org B should not be rate limited, got status %d", rec.Code)
		}
	})
}

func TestRateLimiter_MiddlewareWith(t *testing.T) {
	// keyFn returns a fixed key with per-key limits from the request header,
	// so each subtest can drive different limits without wiring real auth.
	keyFn := func(r *http.Request) (string, float64, int) {
		key := r.Header.Get("X-Key")
		var rps float64
		var burst int
		if v := r.Header.Get("X-Burst"); v != "" {
			// small burst values only; parse by hand to avoid strconv noise
			burst = int(v[0] - '0')
			rps = 1
		}
		return key, rps, burst
	}

	send := func(h http.Handler, key, burst string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Key", key)
		if burst != "" {
			req.Header.Set("X-Burst", burst)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("per-key override caps that key independently", func(t *testing.T) {
		// Default is generous (100 rps); key "tight" overrides to burst 2.
		rl := NewRateLimiter(100, 100)
		h := rl.MiddlewareWith(keyFn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// "tight" (burst 2) is rejected after 2 requests.
		if c := send(h, "tight", "2"); c != http.StatusOK {
			t.Fatalf("req 1: got %d", c)
		}
		if c := send(h, "tight", "2"); c != http.StatusOK {
			t.Fatalf("req 2: got %d", c)
		}
		if c := send(h, "tight", "2"); c != http.StatusTooManyRequests {
			t.Fatalf("req 3: expected 429, got %d", c)
		}

		// A different key with no override uses the generous default.
		if c := send(h, "loose", ""); c != http.StatusOK {
			t.Fatalf("loose key should use default limit, got %d", c)
		}
	})
}

func TestRateLimiter_UpdatesLimitsOnChange(t *testing.T) {
	rl := NewRateLimiter(1, 2)

	// Create with burst 1; the same key is then re-fetched with burst 5.
	l1 := rl.getLimiter("k", 1, 1)
	if l1.Burst() != 1 {
		t.Fatalf("expected burst 1, got %d", l1.Burst())
	}
	l2 := rl.getLimiter("k", 2, 5)
	if l1 != l2 {
		t.Fatal("expected the same limiter instance to be reused")
	}
	if l2.Burst() != 5 || l2.Limit() != 2 {
		t.Fatalf("expected updated limits (rps=2, burst=5), got rps=%v burst=%d", l2.Limit(), l2.Burst())
	}
}

func TestRateLimiter_EvictsIdleLimiters(t *testing.T) {
	rl := NewRateLimiter(100, 200)

	// Create limiters for three distinct keys.
	rl.getLimiter("org-a", rl.rps, rl.burst)
	rl.getLimiter("org-b", rl.rps, rl.burst)
	rl.getLimiter("org-c", rl.rps, rl.burst)
	if got := rl.size(); got != 3 {
		t.Fatalf("expected 3 limiters, got %d", got)
	}

	// Nothing is idle yet: a sweep at "now" evicts nothing.
	if n := rl.evictIdle(time.Now()); n != 0 {
		t.Fatalf("expected 0 evictions, got %d", n)
	}

	// Simulate one key staying active by touching it, then run a sweep far
	// enough in the future that the untouched keys are past the idle TTL.
	rl.getLimiter("org-a", rl.rps, rl.burst)
	future := time.Now().Add(limiterIdleTTL + time.Minute)
	// org-a was just touched (lastSeen ~= now), so it survives a sweep whose
	// cutoff is future-TTL ~= now+1m... adjust: touch org-a to the future.
	rl.mu.Lock()
	rl.limiters["org-a"].lastSeen = future
	rl.mu.Unlock()

	if n := rl.evictIdle(future); n != 2 {
		t.Fatalf("expected 2 idle limiters evicted, got %d", n)
	}
	if got := rl.size(); got != 1 {
		t.Fatalf("expected 1 limiter remaining, got %d", got)
	}
}
