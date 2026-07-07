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
	"github.com/sheeld/sheeld/internal/shared/urlpolicy"
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

	// SSRF policy for user-supplied guard/transformer URLs, applied when the
	// data plane builds guards from config.
	urlpolicy.SetAllowPrivate(cfg.AllowPrivateGuardURLs)

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
	transformRegistry := transform.NewRegistry()
	store := backendconfig.NewStore()
	poller := backendconfig.NewPoller(cfg.ControlPlaneURL, cfg.Token, cfg.PollInterval, store, guardRegistry, transformRegistry)
	if cfg.ConfigSnapshotPath != "" {
		if cfg.ConfigSnapshotKey == "" {
			slog.Error("SHEELD_DP_CONFIG_SNAPSHOT_KEY is required when SHEELD_DP_CONFIG_SNAPSHOT_PATH is set (snapshots are always encrypted)")
			os.Exit(1)
		}
		poller.WithSnapshotter(backendconfig.NewSnapshotter(cfg.ConfigSnapshotPath, cfg.ConfigSnapshotKey))
		slog.Info("config disk snapshot enabled", "path", cfg.ConfigSnapshotPath)
	}

	slog.Info("fetching initial workspace config", "control_plane", cfg.ControlPlaneURL)
	if err := poller.WaitForInitial(ctx, cfg.StartupTimeout); err != nil {
		slog.Warn("starting without workspace config; proxy will return 503 until the control plane is reachable", "error", err)
	}
	go poller.Run(ctx)

	// Processor: guards + LLM client, config from the in-memory store
	guardEngine := guard.NewEngine(guardRegistry)
	llmClient := llm.NewClient(cfg.LLMGatewayURL, cfg.LLMRequestTimeout).
		WithRetry(cfg.LLMMaxRetries, cfg.LLMRetryBackoff)
	proc := processor.NewProcessor(store, guardEngine, llmClient, auditWriter)
	slog.Info("LLM gateway configured",
		"url", cfg.LLMGatewayURL, "timeout", cfg.LLMRequestTimeout,
		"max_retries", cfg.LLMMaxRetries, "retry_backoff", cfg.LLMRetryBackoff)

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
