package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sheeld/sheeld/internal/controlplane/api/handler"
	cpmw "github.com/sheeld/sheeld/internal/controlplane/api/middleware"
	"github.com/sheeld/sheeld/internal/controlplane/config"
	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/controlplane/service"
	"github.com/sheeld/sheeld/internal/controlplane/workspaceconfig"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// NewRouter creates and configures the chi router with all routes and middleware.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	authService *service.AuthService,
	sourceService *service.SourceService,
	guardrailService *service.GuardrailService,
	queries *generated.Queries,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.MaxBodySize(cfg.MaxBodyBytes))
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

	// Serve OpenAPI spec
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "openapi.yaml")
	})

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	sourceHandler := handler.NewSourceHandler(sourceService)
	guardrailHandler := handler.NewGuardrailHandler(guardrailService)
	auditLogHandler := handler.NewAuditLogHandler(queries)
	modelsHandler := handler.NewModelsHandler()

	// API v1
	r.Route("/v1", func(r chi.Router) {
		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)

			// Protected auth routes (JWT)
			r.Group(func(r chi.Router) {
				r.Use(cpmw.JWTAuth(authService))
				r.Post("/refresh", authHandler.Refresh)
				r.Post("/api-keys", authHandler.CreateAPIKey)
				r.Get("/api-keys", authHandler.ListAPIKeys)
				r.Delete("/api-keys/{id}", authHandler.RevokeAPIKey)
			})
		})

		// Models list (JWT for dashboard)
		r.Group(func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Get("/models", modelsHandler.List)
		})

		// Protected source routes (JWT for dashboard)
		r.Route("/sources", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))

			r.Post("/", sourceHandler.Create)
			r.Get("/", sourceHandler.List)
			r.Get("/{id}", sourceHandler.Get)
			r.Put("/{id}", sourceHandler.Update)
			r.Delete("/{id}", sourceHandler.Delete)

			// List guardrails for a source
			r.Get("/{sourceID}/guardrails", guardrailHandler.ListBySource)
		})

		// Guardrail routes (org-scoped via JWT)
		r.Route("/guardrails", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))

			r.Post("/", guardrailHandler.Create)
			r.Get("/", guardrailHandler.List)
			r.Get("/{id}", guardrailHandler.Get)
			r.Put("/{id}", guardrailHandler.Update)
			r.Delete("/{id}", guardrailHandler.Delete)

			// Attach/detach/list sources for a guardrail
			r.Post("/{id}/sources", guardrailHandler.AttachToSource)
			r.Get("/{id}/sources", guardrailHandler.ListSources)
			r.Delete("/{id}/sources/{sourceID}", guardrailHandler.DetachFromSource)
		})

		// Audit log routes (JWT for dashboard)
		r.Route("/audit-logs", func(r chi.Router) {
			r.Use(cpmw.JWTAuth(authService))
			r.Get("/", auditLogHandler.List)
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
