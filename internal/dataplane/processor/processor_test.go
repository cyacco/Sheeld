package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/alert"
	"github.com/cyacco/Sheeld/internal/dataplane/backendconfig"
	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/llm"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

type captureAudit struct {
	inputText        string
	transforms       *transform.ChainResult
	outputTransforms *transform.ChainResult
	overall          string
	usage            *llm.Usage
	model            string
}

func (c *captureAudit) Record(_, _ uuid.UUID, inputText string, _ map[string]*guard.EngineResult, transforms, outputTransforms *transform.ChainResult, overallResult string, _ int64, usage *llm.Usage, model string) {
	c.inputText = inputText
	c.transforms = transforms
	c.outputTransforms = outputTransforms
	c.overall = overallResult
	c.usage = usage
	c.model = model
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
				InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
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
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), audit, nil)

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

type captureAlerts struct {
	events []alert.Event
}

func (c *captureAlerts) Notify(e alert.Event) {
	c.events = append(c.events, e)
}

func TestProcessor_FiresAlertOnInputRejection(t *testing.T) {
	orgID := uuid.New()
	gid := uuid.New()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Guardrails: []domain.GuardrailConfig{{
				ID: gid, Name: "Blocker", GuardType: domain.GuardTypeBlocklist,
				Phase: domain.GuardPhaseInput, Config: json.RawMessage(`{"words":["attack"]}`),
			}},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
				InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
				GuardrailIDs: []uuid.UUID{gid}, TransformerIDs: []uuid.UUID{},
			}},
		}},
	}
	store := backendconfig.NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer gateway.Close()

	alerts := &captureAlerts{}
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil, alerts)

	// A rejected request fires one alert carrying the failing guard.
	res, err := proc.Execute(context.Background(), orgID, "r", &llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "launch an attack"}},
	})
	if err != nil || res.Status != "rejected" {
		t.Fatalf("expected rejection, got res=%+v err=%v", res, err)
	}
	if len(alerts.events) != 1 {
		t.Fatalf("expected 1 alert event, got %d", len(alerts.events))
	}
	e := alerts.events[0]
	if e.OrganizationID != orgID || e.SourceRoute != "r" || e.Phase != "input" {
		t.Errorf("unexpected event: %+v", e)
	}
	if len(e.FailedGuards) != 1 || e.FailedGuards[0].Name != "Blocker" {
		t.Errorf("unexpected failed guards: %+v", e.FailedGuards)
	}

	// A passing request fires nothing.
	res, err = proc.Execute(context.Background(), orgID, "r", &llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil || res.Status != "pass" {
		t.Fatalf("expected pass, got res=%+v err=%v", res, err)
	}
	if len(alerts.events) != 1 {
		t.Errorf("pass must not fire an alert; got %d events", len(alerts.events))
	}
}

func TestProcessor_CapturesTokenUsage(t *testing.T) {
	orgID := uuid.New()
	store := buildStore(t, orgID)

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	defer gateway.Close()

	audit := &captureAudit{}
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), audit, nil)

	req := &llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "hello there"}}}
	result, err := proc.Execute(context.Background(), orgID, "r", req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass, got %s", result.Status)
	}
	if audit.usage == nil {
		t.Fatal("expected token usage to be captured for audit")
	}
	if audit.usage.PromptTokens != 11 || audit.usage.CompletionTokens != 7 || audit.usage.TotalTokens != 18 {
		t.Errorf("unexpected usage: %+v", audit.usage)
	}
	if audit.model != "gpt-4o-mini" {
		t.Errorf("expected model gpt-4o-mini, got %q", audit.model)
	}
}

func TestProcessor_InputRejectHasNoTokenUsage(t *testing.T) {
	orgID := uuid.New()
	gid := uuid.New()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Guardrails: []domain.GuardrailConfig{{
				ID: gid, Name: "g", GuardType: domain.GuardTypeBlocklist,
				Phase: domain.GuardPhaseInput, Config: json.RawMessage(`{"words":["attack"]}`),
			}},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
				InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
				GuardrailIDs: []uuid.UUID{gid}, TransformerIDs: []uuid.UUID{},
			}},
		}},
	}
	store := backendconfig.NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("LLM must not be called when input guards reject")
	}))
	defer gateway.Close()

	audit := &captureAudit{}
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), audit, nil)

	req := &llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "launch an attack"}}}
	result, err := proc.Execute(context.Background(), orgID, "r", req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "rejected" {
		t.Fatalf("expected rejected, got %s", result.Status)
	}
	if audit.usage != nil {
		t.Errorf("expected no token usage on input reject, got %+v", audit.usage)
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
					InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
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
	proc := NewProcessor(mk(""), guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil, nil)
	result, err := proc.Execute(context.Background(), orgID, "r", req())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected last_message scope to miss history payload, got %s", result.Status)
	}

	// all_messages scope: caught.
	proc = NewProcessor(mk(`,"scope":"all_messages"`), guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil, nil)
	result, err = proc.Execute(context.Background(), orgID, "r", req())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "rejected" {
		t.Fatalf("expected all_messages scope to catch history payload, got %s", result.Status)
	}
}

func TestProcessor_OutputTransformsRewriteResponseBeforeOutputGuards(t *testing.T) {
	orgID := uuid.New()
	tid, gid := uuid.New(), uuid.New()
	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Transformers: []domain.TransformerConfig{{
				ID: tid, Name: "redact-out", TransformerType: "test_replace", Phase: "output",
				Config: json.RawMessage(`{"find":"secret","replace":"[REDACTED]"}`),
			}},
			Guardrails: []domain.GuardrailConfig{{
				ID: gid, Name: "block-secret-out", GuardType: domain.GuardTypeBlocklist,
				Phase:  domain.GuardPhaseOutput,
				Config: json.RawMessage(`{"words":["secret"]}`),
			}},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
				InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
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

	// LLM leaks "secret" in its response. The output transformer redacts it,
	// so the output blocklist guard (blocking "secret") must PASS — proving
	// output guards see the post-transform response.
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"the secret is 42"},"finish_reason":"stop"}]}`))
	}))
	defer gateway.Close()

	audit := &captureAudit{}
	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), audit, nil)

	result, err := proc.Execute(context.Background(), orgID, "r", &llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass (output guard sees redacted response), got %s", result.Status)
	}
	if got := result.LLMResponse.Choices[0].Message.Content; got != "the [REDACTED] is 42" {
		t.Errorf("client response not transformed: %q", got)
	}
	if result.OutputTransforms == nil || !result.OutputTransforms.Changed {
		t.Errorf("expected output transforms recorded as changed: %+v", result.OutputTransforms)
	}
	if result.Transforms != nil {
		t.Errorf("input chain should be empty, got %+v", result.Transforms)
	}
	if audit.outputTransforms == nil || !audit.outputTransforms.Changed {
		t.Errorf("audit missing output transforms: %+v", audit.outputTransforms)
	}
}

func TestProcessor_ReversibleAnonymizationRoundTrip(t *testing.T) {
	orgID := uuid.New()
	tIn, tOut := uuid.New(), uuid.New()

	// Fake analyzer: flags "Alice" as PERSON.
	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if i := indexOf(req.Text, "Alice"); i >= 0 {
			w.Write([]byte(`[{"entity_type":"PERSON","start":` + itoa(i) + `,"end":` + itoa(i+5) + `,"score":0.9}]`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer analyzer.Close()

	cfg := &domain.WorkspaceConfig{
		Version: "v1",
		Organizations: []domain.OrgConfig{{
			ID: orgID,
			Transformers: []domain.TransformerConfig{
				{ID: tIn, Name: "anon", TransformerType: "presidio", Phase: "input",
					Config: json.RawMessage(`{"analyzer_url":"` + analyzer.URL + `","mode":"reversible"}`)},
				{ID: tOut, Name: "dean", TransformerType: "deanonymize", Phase: "output",
					Config: json.RawMessage(`{}`)},
			},
			Sources: []domain.SourceConfig{{
				ID: uuid.New(), Route: "r", Enabled: true, LLMModel: "m", LLMAPIKey: "k",
				InputPassCriteria: domain.PassCriteriaAll, OutputPassCriteria: domain.PassCriteriaAll,
				GuardrailIDs:   []uuid.UUID{},
				TransformerIDs: []uuid.UUID{tIn, tOut},
			}},
		}},
	}
	store := backendconfig.NewStore()
	if err := store.Apply(cfg, guard.NewRegistry(), transform.NewRegistry()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Fake LLM echoes the placeholder it received.
	var llmSaw llm.ChatRequest
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&llmSaw)
		last := llmSaw.Messages[len(llmSaw.Messages)-1].Content
		resp := map[string]interface{}{
			"id": "x", "object": "chat.completion",
			"choices": []map[string]interface{}{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": "Re: " + last},
				"finish_reason": "stop",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer gateway.Close()

	proc := NewProcessor(store, guard.NewEngine(guard.NewRegistry()), llm.NewClient(gateway.URL, 0), nil, nil)
	result, err := proc.Execute(context.Background(), orgID, "r", &llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "tell Alice hi"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// LLM never saw the real name.
	if got := llmSaw.Messages[0].Content; got != "tell <PERSON_1> hi" {
		t.Errorf("LLM saw %q, want placeholder", got)
	}
	// Client gets the real name restored.
	if got := result.LLMResponse.Choices[0].Message.Content; got != "Re: tell Alice hi" {
		t.Errorf("client got %q, want restored name", got)
	}
	if result.OutputTransforms == nil || !result.OutputTransforms.Changed {
		t.Errorf("output chain should record the restoration: %+v", result.OutputTransforms)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
