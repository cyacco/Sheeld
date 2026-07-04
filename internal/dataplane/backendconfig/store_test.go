package backendconfig

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/domain"
	"github.com/sheeld/sheeld/internal/shared/guard"
)

func TestApplyFailOpenWiring(t *testing.T) {
	orgID := uuid.New()
	failOpenID := uuid.New()
	failClosedID := uuid.New()

	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Guardrails: []domain.GuardrailConfig{
				{
					ID:        failOpenID,
					Name:      "open",
					GuardType: domain.GuardTypeBlocklist,
					Phase:     domain.GuardPhaseInput,
					Config:    json.RawMessage(`{"words":["x"],"on_error":"fail_open"}`),
				},
				{
					ID:        failClosedID,
					Name:      "closed",
					GuardType: domain.GuardTypeBlocklist,
					Phase:     domain.GuardPhaseInput,
					Config:    json.RawMessage(`{"words":["x"]}`),
				},
			},
			Sources: []domain.SourceConfig{{
				ID:           uuid.New(),
				Route:        "r",
				Enabled:      true,
				GuardrailIDs: []uuid.UUID{failOpenID, failClosedID},
			}},
		}},
	}

	store := NewStore()
	if err := store.Apply(cfg, guard.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	src, ok := store.LookupSource(orgID, "r")
	if !ok {
		t.Fatal("source not found after apply")
	}
	if len(src.InputGuards) != 2 {
		t.Fatalf("expected 2 input guards, got %d", len(src.InputGuards))
	}

	if _, ok := src.InputGuards[0].(guard.FailOpenGuard); !ok {
		t.Error("guard with on_error=fail_open should be wrapped as FailOpenGuard")
	}
	if _, ok := src.InputGuards[1].(guard.FailOpenGuard); ok {
		t.Error("guard without on_error must not be fail-open")
	}
}
