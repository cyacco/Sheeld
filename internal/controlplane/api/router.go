package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyacco/Sheeld/internal/controlplane/api/handler"
	cpmw "github.com/cyacco/Sheeld/internal/controlplane/api/middleware"
	"github.com/cyacco/Sheeld/internal/controlplane/config"
	"github.com/cyacco/Sheeld/internal/controlplane/db/generated"
	"github.com/cyacco/Sheeld/internal/controlplane/service"
	"github.com/cyacco/Sheeld/internal/controlplane/workspaceconfig"
	"github.com/cyacco/Sheeld/internal/shared/metrics"
	"github.com/cyacco/Sheeld/internal/shared/middleware"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// NewRouter creates and configures the chi router with all routes and middleware.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	authService *service.AuthService,
	sourceService *service.SourceService,
	guardrailService *service.GuardrailService,
	transformerService *service.TransformerService,
	queries *generated.Queries,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.MaxBodySize(cfg.MaxBodyBytes))
	r.Use(metrics.HTTP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Sheeld-Status"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check with DB ping
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			response.JSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "error",
				"db":     "disconnected",
			})
			return
		}
		response.JSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"db":     "connected",
		})
	})

	// Prometheus scrape endpoint.
	r.Handle("/metrics", metrics.Handler())

	// Serve OpenAPI spec
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "openapi.yaml")
	})

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	sourceHandler := handler.NewSourceHandler(sourceService)
	guardrailHandler := handler.NewGuardrailHandler(guardrailService)
	transformerHandler := handler.NewTransformerHandler(transformerService)
	auditLogHandler := handler.NewAuditLogHandler(cfg.DataPlaneURL, cfg.DataPlaneToken)
	alertHandler := handler.NewAlertHandler(service.NewAlertService(queries))
	modelsHandler := handler.NewModelsHandler()

	// Per-org rate limiter for authenticated control-plane routes. Keys on
	// the org ID set by JWTAuth, so it is applied after auth in each group.
	cpRateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)

	// API v1
	r.Route("/v1", func(r chi.Router) {
		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)

			// Protected auth routes (JWT)
			r.Group(func(r chi.Router) {
				r.Use(cpmw.JWTAuth(authService))
				r.Use(cpRateLimiter.Middleware)
				r.Post("/refresh", authHandler.Refresh)
				r.Post("/api-keys", authHandler.CreateAPIKey)
				r.Get("/api-keys", authHandler.ListAPIKeys)
				r.Delete("/api-keys/{id}", authHandler.RevokeAPIKey)
			})
		})

		// Models list (JWT for dashboard)
		r.Group(func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)
			r.Get("/models", modelsHandler.List)
		})

		// Protected source routes (JWT for dashboard)
		r.Route("/sources", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)

			r.Post("/", sourceHandler.Create)
			r.Get("/", sourceHandler.List)
			r.Get("/{id}", sourceHandler.Get)
			r.Put("/{id}", sourceHandler.Update)
			r.Delete("/{id}", sourceHandler.Delete)

			// List guardrails for a source
			r.Get("/{sourceID}/guardrails", guardrailHandler.ListBySource)

			// Transformer chain for a source (ordered)
			r.Get("/{sourceID}/transformers", transformerHandler.ListBySource)
			r.Put("/{sourceID}/transformers", transformerHandler.SetSourceTransformers)
		})

		// Guardrail routes (org-scoped via JWT)
		r.Route("/guardrails", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)

			r.Post("/", guardrailHandler.Create)
			r.Get("/", guardrailHandler.List)
			r.Get("/{id}", guardrailHandler.Get)
			r.Put("/{id}", guardrailHandler.Update)
			r.Delete("/{id}", guardrailHandler.Delete)

			// Dry-run: test a guard against sample text.
			r.Post("/{id}/test", guardrailHandler.Test)

			// Attach/detach/list sources for a guardrail
			r.Post("/{id}/sources", guardrailHandler.AttachToSource)
			r.Get("/{id}/sources", guardrailHandler.ListSources)
			r.Delete("/{id}/sources/{sourceID}", guardrailHandler.DetachFromSource)
		})

		// Connections list for the dashboard wiring view (JWT)
		r.Group(func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)
			connectionsHandler := handler.NewConnectionsHandler(queries)
			r.Get("/connections", connectionsHandler.List)
		})

		// Transformer routes (org-scoped via JWT)
		r.Route("/transformers", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)

			r.Post("/", transformerHandler.Create)
			r.Get("/", transformerHandler.List)
			r.Get("/{id}", transformerHandler.Get)
			r.Put("/{id}", transformerHandler.Update)
			r.Delete("/{id}", transformerHandler.Delete)

			r.Get("/{id}/sources", transformerHandler.ListSources)
			r.Post("/{id}/sources", transformerHandler.AttachToSource)
			r.Delete("/{id}/sources/{sourceID}", transformerHandler.DetachFromSource)
		})

		// Audit log routes (JWT for dashboard)
		r.Route("/audit-logs", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)
			r.Get("/", auditLogHandler.List)
		})

		// Analytics (aggregated usage) routes (JWT for dashboard)
		r.Route("/analytics", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)
			r.Get("/", auditLogHandler.Analytics)
		})

		// Rejection-alert webhook routes (JWT for dashboard)
		r.Route("/alerts", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Use(cpRateLimiter.Middleware)
			r.Post("/", alertHandler.Create)
			r.Get("/", alertHandler.List)
			r.Put("/{id}", alertHandler.Update)
			r.Delete("/{id}", alertHandler.Delete)
		})

		// Internal routes for data planes (static shared token)
		r.Route("/internal", func(r chi.Router) {
			r.Use(cpmw.DataPlaneAuth(cfg.DataPlaneToken))
			wcHandler := workspaceconfig.NewHandler(workspaceconfig.NewBuilder(queries, cfg.EncryptionKey))
			r.Get("/workspace-config", wcHandler.Get)
		})

	})

	return r
}
