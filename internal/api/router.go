package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/sheeld/sheeld/internal/api/handler"
	"github.com/sheeld/sheeld/internal/api/middleware"
	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/config"
	"github.com/sheeld/sheeld/internal/proxy"
	"github.com/sheeld/sheeld/internal/service"
)

// NewRouter creates and configures the chi router with all routes and middleware.
func NewRouter(
	cfg *config.Config,
	authService *service.AuthService,
	sourceService *service.SourceService,
	destinationService *service.DestinationService,
	proxyService *proxy.Proxy,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Sheeld-Status"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	sourceHandler := handler.NewSourceHandler(sourceService)
	destinationHandler := handler.NewDestinationHandler(destinationService)
	proxyHandler := handler.NewProxyHandler(proxyService)

	// API v1
	r.Route("/v1", func(r chi.Router) {
		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)

			// Protected auth routes (JWT)
			r.Group(func(r chi.Router) {
				r.Use(middleware.JWTAuth(authService))
				r.Post("/api-keys", authHandler.CreateAPIKey)
				r.Get("/api-keys", authHandler.ListAPIKeys)
				r.Delete("/api-keys/{id}", authHandler.RevokeAPIKey)
			})
		})

		// Protected source routes (JWT for dashboard)
		r.Route("/sources", func(r chi.Router) {
			r.Use(middleware.JWTAuth(authService))

			r.Post("/", sourceHandler.Create)
			r.Get("/", sourceHandler.List)
			r.Get("/{id}", sourceHandler.Get)
			r.Put("/{id}", sourceHandler.Update)
			r.Delete("/{id}", sourceHandler.Delete)

			// Destination routes (nested under sources)
			r.Route("/{sourceID}/destinations", func(r chi.Router) {
				r.Post("/", destinationHandler.Create)
				r.Get("/", destinationHandler.List)
				r.Get("/{id}", destinationHandler.Get)
				r.Put("/{id}", destinationHandler.Update)
				r.Delete("/{id}", destinationHandler.Delete)
			})
		})

		// Proxy route (API key auth for machine-to-machine)
		r.Route("/proxy", func(r chi.Router) {
			r.Use(middleware.APIKeyAuth(authService))
			r.Post("/{sourceSlug}", proxyHandler.Handle)
		})
	})

	return r
}
