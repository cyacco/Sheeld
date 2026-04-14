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

	t.Run("evicts idle entries", func(t *testing.T) {
		rl := &RateLimiter{
			rps:          10,
			burst:        10,
			IdleTTL:      50 * time.Millisecond,
			CleanupEvery: 20 * time.Millisecond,
			stopCh:       make(chan struct{}),
		}
		go rl.cleanupLoop()
		defer rl.Stop()

		// Touch a key so an entry exists.
		rl.getLimiter("org-a")
		if got := rl.size(); got != 1 {
			t.Fatalf("size after first touch = %d, want 1", got)
		}

		// Wait long enough for at least one cleanup tick after the TTL elapses.
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			if rl.size() == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("entry was not evicted after idle TTL; size = %d", rl.size())
	})
}
