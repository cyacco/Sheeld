package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sheeld/sheeld/internal/provider"
)

// SyncProvider replaces all models for a given provider within a transaction.
func SyncProvider(ctx context.Context, pool *pgxpool.Pool, providerName string, models []provider.Model) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing models for this provider
	if _, err := tx.Exec(ctx, "DELETE FROM models WHERE provider = $1", providerName); err != nil {
		return fmt.Errorf("deleting models for %s: %w", providerName, err)
	}

	// Insert fresh models
	for _, m := range models {
		if _, err := tx.Exec(ctx, "INSERT INTO models (provider, id) VALUES ($1, $2) ON CONFLICT DO NOTHING", m.Provider, m.ID); err != nil {
			return fmt.Errorf("inserting model %s: %w", m.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
