package auditstore

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
)

// fakePrunerQueries simulates batched deletes: it reports `remaining` rows are
// older than the cutoff and deletes up to the batch limit per call.
type fakePrunerQueries struct {
	mu        sync.Mutex
	remaining int64
	calls     int
	err       error
}

func (f *fakePrunerQueries) DeleteAuditLogsBefore(_ context.Context, arg generated.DeleteAuditLogsBeforeParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	n := arg.Limit
	if f.remaining < int64(n) {
		n = int32(f.remaining)
	}
	f.remaining -= int64(n)
	return int64(n), nil
}

func TestPruner_DisabledIsNoOp(t *testing.T) {
	fake := &fakePrunerQueries{remaining: 10_000}
	p := NewPruner(fake, 0, time.Minute) // retention <= 0 disables
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Run should return immediately regardless
	p.Run(ctx)
	if fake.calls != 0 {
		t.Errorf("disabled pruner should never query, got %d calls", fake.calls)
	}
}

func TestPruner_DeletesInBatches(t *testing.T) {
	// 2500 expired rows with a 1000-row batch → 3 delete calls (1000,1000,500).
	fake := &fakePrunerQueries{remaining: 2500}
	p := NewPruner(fake, time.Hour, time.Hour)
	p.pruneOnce(context.Background())
	if fake.remaining != 0 {
		t.Errorf("expected all rows pruned, %d remain", fake.remaining)
	}
	if fake.calls != 3 {
		t.Errorf("expected 3 batched deletes, got %d", fake.calls)
	}
}

func TestPruner_StopsOnError(t *testing.T) {
	fake := &fakePrunerQueries{remaining: 5000, err: errors.New("db down")}
	p := NewPruner(fake, time.Hour, time.Hour)
	p.pruneOnce(context.Background())
	// One failed call, then it bails rather than spinning.
	if fake.calls != 1 {
		t.Errorf("expected prune to stop after the first error, got %d calls", fake.calls)
	}
}

func TestPruner_RunSweepsImmediatelyThenStops(t *testing.T) {
	fake := &fakePrunerQueries{remaining: 500}
	p := NewPruner(fake, time.Hour, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p.Run(ctx) // runs one sweep immediately, then blocks on the ticker until ctx done
	if fake.remaining != 0 {
		t.Errorf("expected immediate sweep to prune rows, %d remain", fake.remaining)
	}
}
