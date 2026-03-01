package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sheeld/sheeld/internal/db"
	"github.com/sheeld/sheeld/internal/provider"
	"github.com/sheeld/sheeld/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	dbURL := os.Getenv("SHEELD_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("SHEELD_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	slog.Info("connected to database")

	// Run migrations
	if err := db.RunMigrations(ctx, pool); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	slog.Info("database migrations applied")

	// Sync OpenAI
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		slog.Info("fetching OpenAI models...")
		models, err := provider.FetchOpenAIModels(ctx, apiKey)
		if err != nil {
			return fmt.Errorf("fetching OpenAI models: %w", err)
		}
		if err := service.SyncProvider(ctx, pool, "openai", models); err != nil {
			return fmt.Errorf("syncing OpenAI models: %w", err)
		}
		slog.Info("synced OpenAI models", "count", len(models))
	} else {
		slog.Warn("OPENAI_API_KEY not set, skipping OpenAI")
	}

	// Sync Anthropic
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		slog.Info("fetching Anthropic models...")
		models, err := provider.FetchAnthropicModels(ctx, apiKey)
		if err != nil {
			return fmt.Errorf("fetching Anthropic models: %w", err)
		}
		if err := service.SyncProvider(ctx, pool, "anthropic", models); err != nil {
			return fmt.Errorf("syncing Anthropic models: %w", err)
		}
		slog.Info("synced Anthropic models", "count", len(models))
	} else {
		slog.Warn("ANTHROPIC_API_KEY not set, skipping Anthropic")
	}

	slog.Info("model sync complete")
	return nil
}
