package auditstore

import (
	"context"
	"log/slog"
	"time"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
)

// pruneBatchSize bounds each DELETE so clearing a large backlog doesn't lock
// the table in one long-running statement.
const pruneBatchSize = 1000

// prunerQueries is the subset of generated queries the pruner needs, so tests
// can inject a fake. *generated.Queries satisfies it.
type prunerQueries interface {
	DeleteAuditLogsBefore(ctx context.Context, arg generated.DeleteAuditLogsBeforeParams) (int64, error)
}

// Pruner periodically deletes audit rows older than the retention window.
// Retention is opt-in: with retention <= 0 the pruner does nothing, so audit
// history is never silently discarded.
type Pruner struct {
	queries   prunerQueries
	retention time.Duration
	interval  time.Duration
}

// NewPruner creates an audit-log pruner. retention is how long rows are kept;
// interval is how often the sweep runs.
func NewPruner(queries prunerQueries, retention, interval time.Duration) *Pruner {
	return &Pruner{queries: queries, retention: retention, interval: interval}
}

// Run sweeps on a ticker until the context is cancelled. It runs one sweep
// immediately at startup so a restart doesn't wait a full interval. A no-op
// when retention is disabled.
func (p *Pruner) Run(ctx context.Context) {
	if p.retention <= 0 {
		slog.Info("audit-log retention disabled; rows are kept indefinitely")
		return
	}
	slog.Info("audit-log retention enabled", "retention", p.retention, "interval", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		p.pruneOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pruneOnce deletes everything older than the retention cutoff, in batches.
func (p *Pruner) pruneOnce(ctx context.Context) {
	cutoff := time.Now().Add(-p.retention)
	var total int64
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := p.queries.DeleteAuditLogsBefore(ctx, generated.DeleteAuditLogsBeforeParams{
			CreatedAt: cutoff,
			Limit:     pruneBatchSize,
		})
		if err != nil {
			slog.Error("audit-log prune failed", "error", err, "deleted_so_far", total)
			return
		}
		total += n
		if n < pruneBatchSize {
			break
		}
	}
	if total > 0 {
		slog.Info("pruned expired audit logs", "deleted", total, "older_than", cutoff)
	}
}
