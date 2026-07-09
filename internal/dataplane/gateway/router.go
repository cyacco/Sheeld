package gateway

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/sheeld/sheeld/internal/dataplane/auditstore"
	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/dataplane/config"
	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/metrics"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// NewRouter creates the data-plane HTTP router.
func NewRouter(cfg *config.Config, store *backendconfig.Store, proc *processor.Processor, auditHandler *auditstore.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.MaxBodySize(cfg.MaxBodyBytes))
	r.Use(metrics.HTTP)

	// Prometheus scrape endpoint.
	r.Handle("/metrics", metrics.Handler())

	// Health check reflects config state: not ready until the first
	// workspace config has been applied.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !store.Loaded() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"error","config":"absent"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","config":"loaded","version":"` + store.Version() + `"}`))
	})

	// Internal routes for the control plane (same shared static token the
	// data plane uses to fetch config)
	r.Route("/v1/internal", func(r chi.Router) {
		r.Use(staticTokenAuth(cfg.Token))
		r.Get("/audit-logs", auditHandler.List)
	})

	// Proxy route (API key auth against the in-memory config store)
	proxyHandler := NewProxyHandler(proc)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(APIKeyAuth(store))
		r.Use(rateLimiter.Middleware)
		r.Use(chimiddleware.Timeout(cfg.ProxyTimeout))
		r.Post("/{sourceRoute}", proxyHandler.Handle)
		// Drop-in for OpenAI SDKs: set base_url to .../v1/proxy/{route}
		// and the SDK appends /chat/completions itself.
		r.Post("/{sourceRoute}/chat/completions", proxyHandler.Handle)
	})

	return r
}

// staticTokenAuth authenticates control-plane requests with the shared
// static token, compared in constant time.
func staticTokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") ||
				subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) != 1 {
				response.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
