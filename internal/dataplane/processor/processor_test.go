package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/shared/domain"
	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/transform"
)

type captureAudit struct {
	inputText  string
	transforms *transform.ChainResult
	overall    string
}

func (c *captureAudit) Record(_, _ uuid.UUID, inputText string, _ map[string]*guard.EngineResult, transforms *transform.ChainResult, overallResult string, _ int64) {
	c.inputText = inputText
	c.transforms = transforms
	c.overall = overallResult
}

// buildStore applies a single-source config with a test_replace transformer
// (secret→[REDACTED]) and a blocklist guard blocking "secret".
func buildStore(t *testing.T, orgID uuid.UUID) *backendconfig.Store {
	t.Helper()
	tid, gid := uuid.New(), uuid.New()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Transformers: []domain.TransformerConfig{{
				ID: tid, Name: "redact", TransformerType: "test_replace", Phase: "input",
				Config: json.RawMessage(`{"find":"secret","replace":"[REDACTED]"}`),
			}},
			Guardrails: []domain.GuardrailConfig{{
				ID: gid, Name: "block-secret", GuardType: domain.GuardTypeBlocklist,
				Phase:  domain.GuardPhaseInput,
				Config: json.RawMessage(`{"words":["secret"]}`),
			}},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
				PassCriteria:   domain.PassCriteriaAll,
				GuardrailIDs:   []uuid.UUID{gid},
				TransformerIDs: []uuid.UUID{tid},
			}},
		}},
	}
	tr := transform.NewRegistry()
	tr.Register("test_replace", transform.TestReplaceFactory)
	store := backendconfig.NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), tr); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return store
}

func TestProcessor_TransformsRunBeforeGuardsAndLLM(t *testing.T) {
	orgID := uuid.New()
	store := buildStore(t, orgID)

	// Fake LLM gateway records the request body it receives.
	var llmSaw llm.ChatRequest
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&llmSaw)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer gateway.Close()

	audit := &captureAudit{}
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), audit)

	// "secret" appears in the system prompt AND the last user message. The
	// transformer redacts both; the blocklist guard (which blocks "secret")
	// must therefore PASS, proving guards see post-transform text.
	req := &llm.ChatRequest{Messages: []llm.Message{
		{Role: "system", Content: "the secret is 42"},
		{Role: "user", Content: "tell me the secret"},
	}}
	result, err := proc.Execute(context.Background(), orgID, "r", req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass (guard sees redacted text), got %s", result.Status)
	}

	// LLM received transformed messages — all of them.
	if llmSaw.Messages[0].Content != "the [REDACTED] is 42" ||
		llmSaw.Messages[1].Content != "tell me the [REDACTED]" {
		t.Errorf("LLM did not receive transformed messages: %+v", llmSaw.Messages)
	}

	// Result and audit carry the chain outcome; input hash source is
	// post-transform text.
	if result.Transforms == nil || !result.Transforms.Changed {
		t.Errorf("expected transforms recorded as changed: %+v", result.Transforms)
	}
	if audit.transforms == nil || audit.inputText != "tell me the [REDACTED]" {
		t.Errorf("audit got inputText %q, transforms %+v", audit.inputText, audit.transforms)
	}
}

func TestProcessor_AllMessagesScopeCatchesHistoryInjection(t *testing.T) {
	orgID := uuid.New()
	gid := uuid.New()
	mk := func(scope string) *backendconfig.Store {
		cfg := &domain.WorkspaceConfig{
			Version: "v1",
			Organizations: []domain.OrgConfig{{
				ID: orgID,
				Guardrails: []domain.GuardrailConfig{{
					ID: gid, Name: "g", GuardType: domain.GuardTypeBlocklist,
					Phase:  domain.GuardPhaseInput,
					Config: json.RawMessage(`{"words":["attack"]` + scope + `}`),
				}},
				Sources: []domain.SourceConfig{{
					ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
					PassCriteria: domain.PassCriteriaAll,
					GuardrailIDs: []uuid.UUID{gid}, TransformerIDs: []uuid.UUID{},
				}},
			}},
		}
		store := backendconfig.NewStore()
		if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		return store
	}

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer gateway.Close()

	// Payload hidden in an earlier message; last user message is clean.
	req := func() *llm.ChatRequest {
		return &llm.ChatRequest{Messages: []llm.Message{
			{Role: "user", Content: "here is my attack payload"},
			{Role: "assistant", Content: "noted"},
			{Role: "user", Content: "innocent question"},
		}}
	}

	// Default last_message scope: bypassed.
	proc := NewProcessor(mk(""), guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil)
	result, err := proc.Execute(context.Background(), orgID, "r", req())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected last_message scope to miss history payload, got %s", result.Status)
	}

	// all_messages scope: caught.
	proc = NewProcessor(mk(`,"scope":"all_messages"`), guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil)
	result, err = proc.Execute(context.Background(), orgID, "r", req())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "rejected" {
		t.Fatalf("expected all_messages scope to catch history payload, got %s", result.Status)
	}
}
