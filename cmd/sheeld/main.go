package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sheeld/sheeld/internal/api"
	"github.com/sheeld/sheeld/internal/config"
	"github.com/sheeld/sheeld/internal/crypto"
	"github.com/sheeld/sheeld/internal/db"
	"github.com/sheeld/sheeld/internal/db/generated"
	"github.com/sheeld/sheeld/internal/guard"
	"github.com/sheeld/sheeld/internal/llm"
	"github.com/sheeld/sheeld/internal/proxy"
	"github.com/sheeld/sheeld/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Validate encryption key at startup so misconfiguration fails fast
	// instead of surfacing on the first encrypt/decrypt call.
	if err := crypto.ValidateKey(cfg.EncryptionKey); err != nil {
		return fmt.Errorf("validating encryption key: %w", err)
	}

	// Set up structured logging
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Connect to database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	slog.Info("connected to database")

	// Run database migrations
	if err := db.RunMigrations(ctx, pool); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database migrations applied")

	// Initialize dependencies
	queries := generated.New(pool)

	authService := service.NewAuthService(queries, cfg.JWTSecret, cfg.JWTExpiration)
	sourceService := service.NewSourceService(queries, cfg.EncryptionKey)
	guardrailService := service.NewGuardrailService(queries)

	// Initialize guardrail engine and LLM client
	guardRegistry := guard.NewRegistry()
	guardEngine := guard.NewEngine(guardRegistry)
	llmClient := llm.NewClient(cfg.LLMGatewayURL, cfg.LLMRequestTimeout)
	proxyService := proxy.NewProxy(queries, guardEngine, llmClient, cfg.EncryptionKey)

	slog.Info("LLM gateway configured", "url", cfg.LLMGatewayURL, "timeout", cfg.LLMRequestTimeout)

	// Build HTTP router
	router := api.NewRouter(cfg, pool, authService, sourceService, guardrailService, proxyService, queries)

	// Start HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Run server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		slog.Info("shutting down server")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
