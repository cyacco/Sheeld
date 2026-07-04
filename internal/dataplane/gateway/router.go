package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/dataplane/config"
	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/middleware"
)

// NewRouter creates the data-plane HTTP router.
func NewRouter(cfg *config.Config, store *backendconfig.Store, proc *processor.Processor) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.MaxBodySize(cfg.MaxBodyBytes))

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

	// Proxy route (API key auth against the in-memory config store)
	proxyHandler := NewProxyHandler(proc)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(APIKeyAuth(store))
		r.Use(rateLimiter.Middleware)
		r.Use(chimiddleware.Timeout(cfg.ProxyTimeout))
		r.Post("/{sourceRoute}", proxyHandler.Handle)
	})

	return r
}
