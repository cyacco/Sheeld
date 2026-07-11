package auditstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/llm"
)

// fakeWriterQueries records CreateAuditLog calls and can be told to fail.
type fakeWriterQueries struct {
	mu      sync.Mutex
	inserts []generated.CreateAuditLogParams
	failN   int // fail the first failN calls, then succeed
	calls   int
}

func (f *fakeWriterQueries) CreateAuditLog(_ context.Context, arg generated.CreateAuditLogParams) (generated.AuditLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failN {
		return generated.AuditLog{}, context.DeadlineExceeded
	}
	f.inserts = append(f.inserts, arg)
	return generated.AuditLog{}, nil
}

func (f *fakeWriterQueries) recorded() []generated.CreateAuditLogParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]generated.CreateAuditLogParams(nil), f.inserts...)
}

// waitFor polls until cond() or the deadline, so we don't race the async flush.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestWriter_RecordHashesInputAndFlushes(t *testing.T) {
	fake := &fakeWriterQueries{}
	w := NewWriter(fake)
	defer w.Close()

	orgID, srcID := uuid.New(), uuid.New()
	w.Record(orgID, srcID, "secret prompt text", nil, nil, nil, "pass", 42,
		&llm.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, "gpt-4o-mini")

	waitFor(t, func() bool { return len(fake.recorded()) == 1 })

	got := fake.recorded()[0]
	if got.OrganizationID != orgID || got.SourceID != srcID {
		t.Errorf("ids mismatch: %+v", got)
	}
	if got.OverallResult != "pass" || got.LatencyMs != 42 {
		t.Errorf("result/latency mismatch: %+v", got)
	}
	if got.PromptTokens.Int32 != 11 || got.CompletionTokens.Int32 != 7 || got.TotalTokens.Int32 != 18 {
		t.Errorf("token usage mismatch: %+v", got)
	}
	if got.Model.String != "gpt-4o-mini" {
		t.Errorf("model mismatch: got %q", got.Model.String)
	}
	// Raw input must never be stored — only its SHA-256 hash.
	sum := sha256.Sum256([]byte("secret prompt text"))
	if got.InputHash.String != hex.EncodeToString(sum[:]) {
		t.Errorf("input hash mismatch: got %q", got.InputHash.String)
	}
	if got.InputHash.String == "secret prompt text" {
		t.Error("raw input leaked into audit log")
	}
}

func TestWriter_FlushRetriesThenSucceeds(t *testing.T) {
	// First insert attempt fails; the writer retries the batch and succeeds.
	fake := &fakeWriterQueries{failN: 1}
	w := NewWriter(fake)
	defer w.Close()

	w.Record(uuid.New(), uuid.New(), "x", nil, nil, nil, "pass", 1, nil, "")
	waitFor(t, func() bool { return len(fake.recorded()) == 1 })
}

func TestWriter_CloseDrainsBufferedEntries(t *testing.T) {
	fake := &fakeWriterQueries{}
	w := NewWriter(fake)

	for i := range 5 {
		w.Record(uuid.New(), uuid.New(), "x", nil, nil, nil, "pass", int64(i), nil, "")
	}
	// Close flushes remaining buffered entries before returning.
	w.Close()
	if got := len(fake.recorded()); got != 5 {
		t.Errorf("expected 5 entries flushed on close, got %d", got)
	}
}

func TestWriter_RecordNeverBlocksWhenBufferFull(t *testing.T) {
	// A writer whose consumer can't drain (queries block) must still not block
	// callers once the buffer fills — excess entries are dropped.
	block := make(chan struct{})
	fake := &blockingQueries{gate: block}
	w := NewWriter(fake)
	defer func() { close(block); w.Close() }()

	done := make(chan struct{})
	go func() {
		for range bufferSize + batchSize + 100 {
			w.Record(uuid.New(), uuid.New(), "x", nil, nil, nil, "pass", 1, nil, "")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Record blocked when buffer was full")
	}
}

type blockingQueries struct{ gate chan struct{} }

func (b *blockingQueries) CreateAuditLog(_ context.Context, _ generated.CreateAuditLogParams) (generated.AuditLog, error) {
	<-b.gate
	return generated.AuditLog{}, nil
}
