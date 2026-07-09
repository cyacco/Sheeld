package backendconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

const testSnapshotKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testConfig(orgID uuid.UUID) *domain.WorkspaceConfig {
	return &domain.WorkspaceConfig{
		Version: "snap-v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMAPIKey: "sk-secret",
				GuardrailIDs: []uuid.UUID{}, TransformerIDs: []uuid.UUID{},
			}},
		}},
	}
}

func TestSnapshotterRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.snap")
	snap := NewSnapshotter(path, testSnapshotKey)
	orgID := uuid.New()

	if err := snap.Save(testConfig(orgID)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File is 0600 and does not contain the plaintext key.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("snapshot perms = %o, want 600", perm)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sk-secret") {
		t.Error("snapshot contains plaintext LLM key")
	}

	loaded, err := snap.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != "snap-v1" || loaded.Organizations[0].Sources[0].LLMAPIKey != "sk-secret" {
		t.Errorf("round trip mismatch: %+v", loaded)
	}
}

func TestSnapshotterLoadErrors(t *testing.T) {
	dir := t.TempDir()
	snap := NewSnapshotter(filepath.Join(dir, "missing.snap"), testSnapshotKey)
	if _, err := snap.Load(); err == nil {
		t.Error("expected error for missing snapshot")
	}

	// Wrong key must fail, not return garbage.
	path := filepath.Join(dir, "config.snap")
	if err := NewSnapshotter(path, testSnapshotKey).Save(testConfig(uuid.New())); err != nil {
		t.Fatal(err)
	}
	wrongKey := strings.Repeat("ff", 32)
	if _, err := NewSnapshotter(path, wrongKey).Load(); err == nil {
		t.Error("expected error with wrong key")
	}
}

func TestWaitForInitialFallsBackToSnapshot(t *testing.T) {
	// Control plane that always fails.
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer cp.Close()

	path := filepath.Join(t.TempDir(), "config.snap")
	snap := NewSnapshotter(path, testSnapshotKey)
	orgID := uuid.New()
	if err := snap.Save(testConfig(orgID)); err != nil {
		t.Fatal(err)
	}

	store := NewStore()
	poller := NewPoller(cp.URL, "tok", time.Hour, store, guard.NewRegistry(), transform.NewRegistry()).
		WithSnapshotter(snap)

	if err := poller.WaitForInitial(context.Background(), 1*time.Millisecond); err != nil {
		t.Fatalf("expected snapshot fallback to succeed, got %v", err)
	}
	if !store.Loaded() || store.Version() != "snap-v1" {
		t.Errorf("store not serving snapshot config: loaded=%v version=%q", store.Loaded(), store.Version())
	}
	src, ok := store.LookupSource(orgID, "r")
	if !ok || src.LLMAPIKey != "sk-secret" {
		t.Errorf("source not resolved from snapshot: %+v", src)
	}
}

func TestWaitForInitialNoSnapshotStillFails(t *testing.T) {
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer cp.Close()

	store := NewStore()
	poller := NewPoller(cp.URL, "tok", time.Hour, store, guard.NewRegistry(), transform.NewRegistry())
	if err := poller.WaitForInitial(context.Background(), 1*time.Millisecond); err == nil {
		t.Error("expected timeout error without snapshot")
	}
}
