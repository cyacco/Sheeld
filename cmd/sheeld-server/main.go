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

	"github.com/sheeld/sheeld/internal/dataplane/auditstore"
	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/dataplane/config"
	"github.com/sheeld/sheeld/internal/dataplane/db"
	"github.com/sheeld/sheeld/internal/dataplane/db/generated"
	"github.com/sheeld/sheeld/internal/dataplane/gateway"
	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/transform"
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

	// Connect to the data-plane database (audit logs)
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	slog.Info("connected to database")

	if err := db.RunMigrations(ctx, pool); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database migrations applied")

	queries := generated.New(pool)
	auditWriter := auditstore.NewWriter(queries)
	defer auditWriter.Close()

	// Config store + poller
	guardRegistry := guard.NewRegistry()
	// No built-in transformer types in v1; first real types land in a
	// follow-up and register here.
	transformRegistry := transform.NewRegistry()
	store := backendconfig.NewStore()
	poller := backendconfig.NewPoller(cfg.ControlPlaneURL, cfg.Token, cfg.PollInterval, store, guardRegistry, transformRegistry)

	slog.Info("fetching initial workspace config", "control_plane", cfg.ControlPlaneURL)
	if err := poller.WaitForInitial(ctx, cfg.StartupTimeout); err != nil {
		slog.Warn("starting without workspace config; proxy will return 503 until the control plane is reachable", "error", err)
	}
	go poller.Run(ctx)

	// Processor: guards + LLM client, config from the in-memory store
	guardEngine := guard.NewEngine(guardRegistry)
	llmClient := llm.NewClient(cfg.LLMGatewayURL, cfg.LLMRequestTimeout)
	proc := processor.NewProcessor(store, guardEngine, llmClient, auditWriter)
	slog.Info("LLM gateway configured", "url", cfg.LLMGatewayURL, "timeout", cfg.LLMRequestTimeout)

	// Build HTTP router
	router := gateway.NewRouter(cfg, store, proc, auditstore.NewHandler(queries))

	// Start HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting data-plane server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down server")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
