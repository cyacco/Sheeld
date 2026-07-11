package backendconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/transform"
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
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
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

func TestApplyCarriesPerKeyRateLimits(t *testing.T) {
	orgID := uuid.New()
	rps := 42.0
	burst := 7
	rawWithLimits := "shld_withlimits"
	rawDefault := "shld_default"
	hash := func(raw string) string {
		sum := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(sum[:])
	}

	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			APIKeys: []domain.APIKeyConfig{
				{KeyHash: hash(rawWithLimits), RateLimitRPS: &rps, RateLimitBurst: &burst},
				{KeyHash: hash(rawDefault)},
			},
		}},
	}

	store := NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	info, ok := store.LookupAPIKey(rawWithLimits)
	if !ok {
		t.Fatal("key with limits not found")
	}
	if info.OrgID != orgID || info.RateLimitRPS != rps || info.RateLimitBurst != burst {
		t.Fatalf("unexpected auth info: %+v", info)
	}

	def, ok := store.LookupAPIKey(rawDefault)
	if !ok {
		t.Fatal("default key not found")
	}
	if def.RateLimitRPS != 0 || def.RateLimitBurst != 0 {
		t.Fatalf("expected zero (default) limits, got rps=%v burst=%d", def.RateLimitRPS, def.RateLimitBurst)
	}

	if _, ok := store.LookupAPIKey("shld_nope"); ok {
		t.Fatal("unknown key should not resolve")
	}
}

func TestApplyTransformerOrdering(t *testing.T) {
	orgID := uuid.New()
	t1, t2 := uuid.New(), uuid.New()

	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Transformers: []domain.TransformerConfig{
				{ID: t2, Name: "second", TransformerType: "test_replace", Phase: "input", Config: json.RawMessage(`{"find":"b","replace":"c"}`)},
				{ID: t1, Name: "first", TransformerType: "test_replace", Phase: "input", Config: json.RawMessage(`{"find":"a","replace":"b"}`)},
			},
			Sources: []domain.SourceConfig{{
				ID:             uuid.New(),
				Route:          "r",
				Enabled:        true,
				GuardrailIDs:   []uuid.UUID{},
				TransformerIDs: []uuid.UUID{t1, t2}, // chain order, not payload order
			}},
		}},
	}

	tr := transform.NewRegistry()
	tr.Register("test_replace", transform.TestReplaceFactory)

	store := NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), tr); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	src, ok := store.LookupSource(orgID, "r")
	if !ok {
		t.Fatal("source not found")
	}
	if len(src.InputTransformers) != 2 {
		t.Fatalf("expected 2 input transformers, got %d", len(src.InputTransformers))
	}
	if src.InputTransformers[0].Name() != "first" || src.InputTransformers[1].Name() != "second" {
		t.Errorf("chain order not preserved: %s, %s", src.InputTransformers[0].Name(), src.InputTransformers[1].Name())
	}
}

func TestApplyTransformerPhaseSplit(t *testing.T) {
	orgID := uuid.New()
	tIn, tOut := uuid.New(), uuid.New()

	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Transformers: []domain.TransformerConfig{
				{ID: tIn, Name: "in", TransformerType: "test_replace", Phase: "input", Config: json.RawMessage(`{"find":"a","replace":"b"}`)},
				{ID: tOut, Name: "out", TransformerType: "test_replace", Phase: "output", Config: json.RawMessage(`{"find":"b","replace":"c"}`)},
			},
			Sources: []domain.SourceConfig{{
				ID:             uuid.New(),
				Route:          "r",
				Enabled:        true,
				GuardrailIDs:   []uuid.UUID{},
				TransformerIDs: []uuid.UUID{tIn, tOut},
			}},
		}},
	}

	tr := transform.NewRegistry()
	tr.Register("test_replace", transform.TestReplaceFactory)

	store := NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), tr); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	src, ok := store.LookupSource(orgID, "r")
	if !ok {
		t.Fatal("source not found")
	}
	if len(src.InputTransformers) != 1 || src.InputTransformers[0].Name() != "in" {
		t.Errorf("input chain wrong: %+v", src.InputTransformers)
	}
	if len(src.OutputTransformers) != 1 || src.OutputTransformers[0].Name() != "out" {
		t.Errorf("output chain wrong: %+v", src.OutputTransformers)
	}
}

func TestApplyScopeWrapping(t *testing.T) {
	orgID := uuid.New()
	gid := uuid.New()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Guardrails: []domain.GuardrailConfig{{
				ID: gid, Name: "g", GuardType: domain.GuardTypeBlocklist,
				Phase:  domain.GuardPhaseInput,
				Config: json.RawMessage(`{"words":["x"],"scope":"all_messages"}`),
			}},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true,
				GuardrailIDs:   []uuid.UUID{gid},
				TransformerIDs: []uuid.UUID{},
			}},
		}},
	}
	store := NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	src, _ := store.LookupSource(orgID, "r")
	// The scoped guard validates the serialized conversation, so a blocked
	// word only present in an earlier message must be caught.
	ctx := guard.WithCallMeta(context.Background(), guard.CallMeta{
		Phase:           "input",
		AllMessagesText: "user: x in history\nuser: clean last message",
	})
	res, err := src.InputGuards[0].Validate(ctx, "clean last message")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Passed {
		t.Error("expected all_messages-scoped guard to catch blocked word in history")
	}
}
