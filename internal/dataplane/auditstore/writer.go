package auditstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/metrics"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

const (
	bufferSize    = 1024
	batchSize     = 100
	flushInterval = time.Second
)

type entry struct {
	orgID         uuid.UUID
	sourceID      uuid.UUID
	inputHash     string
	guardResults  json.RawMessage
	overallResult string
	latencyMs     int64
}

// writerQueries is the subset of generated queries the writer needs, so tests
// can inject a fake. *generated.Queries satisfies it.
type writerQueries interface {
	CreateAuditLog(ctx context.Context, arg generated.CreateAuditLogParams) (generated.AuditLog, error)
}

// Writer records audit entries asynchronously: a buffered channel feeds a
// single consumer goroutine doing batched inserts. If the buffer is full
// entries are dropped with an error log — audit logging never blocks or
// fails the request path.
type Writer struct {
	queries writerQueries
	ch      chan entry
	done    chan struct{}
	once    sync.Once
}

// NewWriter creates and starts an audit writer.
func NewWriter(queries writerQueries) *Writer {
	w := &Writer{
		queries: queries,
		ch:      make(chan entry, bufferSize),
		done:    make(chan struct{}),
	}
	metrics.RegisterAuditBufferDepth(func() float64 { return float64(len(w.ch)) })
	go w.run()
	return w
}

// Record implements processor.AuditSink. It hashes the input (raw content is
// never stored) and enqueues the entry without blocking.
//
// inputText is the POST-transform last user message: the audit artifact is
// "what the guards evaluated / what the LLM received", and pre-transform
// text was never sent anywhere. The transformer chain outcomes are stored in
// the guard_results JSONB under the reserved keys "transforms" (input chain)
// and "output_transforms" (output chain).
func (w *Writer) Record(orgID, sourceID uuid.UUID, inputText string, guardResults map[string]*guard.EngineResult, transforms, outputTransforms *transform.ChainResult, overallResult string, latencyMs int64) {
	hash := sha256.Sum256([]byte(inputText))

	blob := make(map[string]interface{}, len(guardResults)+2)
	for phase, res := range guardResults {
		blob[phase] = res
	}
	if transforms != nil {
		blob["transforms"] = transforms
	}
	if outputTransforms != nil {
		blob["output_transforms"] = outputTransforms
	}
	resultsJSON, err := json.Marshal(blob)
	if err != nil {
		slog.Error("failed to marshal guard results for audit log", "error", err)
		return
	}

	e := entry{
		orgID:         orgID,
		sourceID:      sourceID,
		inputHash:     hex.EncodeToString(hash[:]),
		guardResults:  resultsJSON,
		overallResult: overallResult,
		latencyMs:     latencyMs,
	}

	select {
	case w.ch <- e:
	default:
		metrics.AuditDropped.Inc()
		slog.Error("audit buffer full; dropping entry", "source_id", sourceID)
	}
}

// Close stops the writer and drains buffered entries with a short deadline.
// Call during graceful shutdown.
func (w *Writer) Close() {
	w.once.Do(func() {
		close(w.ch)
		select {
		case <-w.done:
		case <-time.After(5 * time.Second):
			slog.Error("audit writer drain timed out")
		}
	})
}

func (w *Writer) run() {
	defer close(w.done)

	batch := make([]entry, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-w.ch:
			if !ok {
				w.flush(batch)
				return
			}
			batch = append(batch, e)
			if len(batch) >= batchSize {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// flush inserts a batch, retrying once with backoff before dropping.
func (w *Writer) flush(batch []entry) {
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		var err error
		for _, e := range batch {
			_, err = w.queries.CreateAuditLog(ctx, generated.CreateAuditLogParams{
				OrganizationID: e.orgID,
				SourceID:       e.sourceID,
				InputHash:      pgtype.Text{String: e.inputHash, Valid: true},
				GuardResults:   e.guardResults,
				OverallResult:  e.overallResult,
				LatencyMs:      int32(e.latencyMs),
			})
			if err != nil {
				break
			}
		}
		if err == nil {
			return
		}
		slog.Warn("audit batch insert failed", "error", err, "attempt", attempt+1, "entries", len(batch))
	}
	metrics.AuditBatchesDropped.Inc()
	slog.Error("dropping audit batch after retries", "entries", len(batch))
}
