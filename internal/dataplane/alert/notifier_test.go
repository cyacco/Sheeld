package alert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/backendconfig"
	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

// storeWith builds a config store whose org has the given alert webhooks.
func storeWith(t *testing.T, orgID uuid.UUID, webhooks []domain.AlertWebhookConfig) *backendconfig.Store {
	t.Helper()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID:            orgID,
			AlertWebhooks: webhooks,
		}},
	}
	store := backendconfig.NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return store
}

type capture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *capture) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		c.mu.Lock()
		c.bodies = append(c.bodies, append([]byte(nil), buf[:n]...))
		c.mu.Unlock()
	}
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestNotifier_DeliversJSONPayload(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	orgID := uuid.New()
	store := storeWith(t, orgID, []domain.AlertWebhookConfig{
		{ID: uuid.New(), URL: srv.URL, PayloadFormat: "json"},
	})

	n := NewNotifier(store)
	n.Notify(Event{
		OrganizationID: orgID,
		SourceRoute:    "chat",
		Phase:          "input",
		RequestID:      "req-1",
		FailedGuards:   []FailedGuard{{Name: "Blocklist", Type: "blocklist", Message: "blocked word"}},
	})

	waitFor(t, func() bool { return cap.count() == 1 })

	var got struct {
		Type string `json:"type"`
		Event
	}
	if err := json.Unmarshal(cap.bodies[0], &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "guard_rejection" || got.SourceRoute != "chat" || got.Phase != "input" {
		t.Errorf("unexpected payload: %+v", got)
	}
	if len(got.FailedGuards) != 1 || got.FailedGuards[0].Name != "Blocklist" {
		t.Errorf("unexpected failed guards: %+v", got.FailedGuards)
	}
}

func TestNotifier_SlackFormat(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	orgID := uuid.New()
	store := storeWith(t, orgID, []domain.AlertWebhookConfig{
		{ID: uuid.New(), URL: srv.URL, PayloadFormat: "slack"},
	})

	n := NewNotifier(store)
	n.Notify(Event{OrganizationID: orgID, SourceRoute: "chat", Phase: "output"})

	waitFor(t, func() bool { return cap.count() == 1 })

	var got map[string]string
	if err := json.Unmarshal(cap.bodies[0], &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["text"] == "" {
		t.Errorf("expected slack text payload, got %q", string(cap.bodies[0]))
	}
}

func TestNotifier_RateCapSuppresses(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	orgID := uuid.New()
	store := storeWith(t, orgID, []domain.AlertWebhookConfig{
		{ID: uuid.New(), URL: srv.URL, PayloadFormat: "json"},
	})

	n := NewNotifier(store)
	for i := 0; i < alertBurst+5; i++ {
		n.Notify(Event{OrganizationID: orgID, SourceRoute: "chat", Phase: "input"})
	}

	waitFor(t, func() bool { return cap.count() == alertBurst })
	// Give any stray deliveries a moment, then confirm the cap held.
	time.Sleep(100 * time.Millisecond)
	if got := cap.count(); got != alertBurst {
		t.Errorf("expected %d deliveries (burst cap), got %d", alertBurst, got)
	}
}

func TestNotifier_NoWebhooksNoDelivery(t *testing.T) {
	store := storeWith(t, uuid.New(), nil)
	n := NewNotifier(store)
	// Must not panic or block for an org with no webhooks (or unknown org).
	n.Notify(Event{OrganizationID: uuid.New(), SourceRoute: "chat", Phase: "input"})
}
