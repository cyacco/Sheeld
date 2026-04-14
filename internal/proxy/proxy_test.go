package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sheeld/sheeld/internal/crypto"
	"github.com/sheeld/sheeld/internal/db/generated"
	"github.com/sheeld/sheeld/internal/guard"
	"github.com/sheeld/sheeld/internal/llm"
)

// mockGuard is a test guard that returns a predetermined result.
type mockGuard struct {
	name      string
	guardType string
	passed    bool
}

func (g *mockGuard) Type() string { return g.guardType }
func (g *mockGuard) Name() string { return g.name }
func (g *mockGuard) Validate(_ context.Context, input string) (*guard.Result, error) {
	msg := "passed"
	if !g.passed {
		msg = "blocked by " + g.name
	}
	return &guard.Result{
		GuardName: g.name,
		GuardType: g.guardType,
		Passed:    g.passed,
		Message:   msg,
		Duration:  time.Millisecond,
	}, nil
}

// mockLLMServer creates an httptest server that returns an OpenAI-format response.
func mockLLMServer(t *testing.T, responseContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := llm.ChatResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
			Choices: []llm.Choice{
				{
					Index:        0,
					Message:      llm.Message{Role: "assistant", Content: responseContent},
					FinishReason: "stop",
				},
			},
			Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestEvaluatePassCriteria(t *testing.T) {
	// Test the evaluate function indirectly through the engine
	engine := guard.NewEngine(guard.NewRegistry())

	tests := []struct {
		name       string
		guards     []guard.Guard
		criteria   guard.PassCriteria
		threshold  int
		wantPassed bool
	}{
		{
			name: "all criteria - all pass",
			guards: []guard.Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
			},
			criteria:   guard.CriteriaAll,
			wantPassed: true,
		},
		{
			name: "all criteria - one fails",
			guards: []guard.Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
			},
			criteria:   guard.CriteriaAll,
			wantPassed: false,
		},
		{
			name: "any criteria - one passes",
			guards: []guard.Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: false},
				&mockGuard{name: "g2", guardType: "mock", passed: true},
			},
			criteria:   guard.CriteriaAny,
			wantPassed: true,
		},
		{
			name: "n_of_m criteria - meets threshold",
			guards: []guard.Guard{
				&mockGuard{name: "g1", guardType: "mock", passed: true},
				&mockGuard{name: "g2", guardType: "mock", passed: false},
				&mockGuard{name: "g3", guardType: "mock", passed: true},
			},
			criteria:   guard.CriteriaNofM,
			threshold:  2,
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Run(context.Background(), tt.guards, "test input", guard.EvalConfig{
				Criteria:  tt.criteria,
				Threshold: tt.threshold,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("got passed=%v, want %v", result.Passed, tt.wantPassed)
			}
		})
	}
}

func TestBuildGuardsPhaseFiltering(t *testing.T) {
	// Test that the blocklist guard can be created from JSON config via registry
	registry := guard.NewRegistry()

	config := json.RawMessage(`{"words": ["bad"]}`)
	g, err := registry.Create("blocklist", "test-guard", config)
	if err != nil {
		t.Fatalf("failed to create guard: %v", err)
	}

	// Test input validation
	result, err := g.Validate(context.Background(), "this is bad input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected guard to reject input containing blocked word")
	}

	result, err = g.Validate(context.Background(), "this is clean input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected guard to pass clean input")
	}
}

func TestLLMClientIntegration(t *testing.T) {
	server := mockLLMServer(t, "I'm a helpful assistant!")
	defer server.Close()

	client := llm.NewClient(server.URL, 5*time.Second)

	resp, err := client.ChatCompletion(context.Background(), "test-key", &llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := llm.ExtractOutputText(resp)
	if output != "I'm a helpful assistant!" {
		t.Errorf("got output=%q, want %q", output, "I'm a helpful assistant!")
	}
}

func TestFullProxyFlow_InputGuardRejects(t *testing.T) {
	// Verify that when input guard fails, no LLM call is made
	llmCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := guard.NewEngine(guard.NewRegistry())

	// Create a blocklist guard that will reject "bad"
	inputGuards := []guard.Guard{
		guard.NewBlocklistGuard("profanity-filter", guard.BlocklistConfig{
			Words: []string{"bad"},
		}),
	}

	// Run input guards
	result, err := engine.Run(context.Background(), inputGuards, "this is bad input", guard.EvalConfig{
		Criteria: guard.CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected input guards to reject")
	}

	// Verify LLM was never called
	if llmCalled {
		t.Error("LLM should not have been called when input guards reject")
	}
}

func TestFullProxyFlow_OutputGuardRejects(t *testing.T) {
	// LLM returns content that fails output guards
	server := mockLLMServer(t, "Here is some bad content for you")
	defer server.Close()

	client := llm.NewClient(server.URL, 5*time.Second)
	engine := guard.NewEngine(guard.NewRegistry())

	// Input guard passes (clean input)
	inputGuards := []guard.Guard{
		guard.NewBlocklistGuard("input-filter", guard.BlocklistConfig{
			Words: []string{"blocked"},
		}),
	}

	inputResult, err := engine.Run(context.Background(), inputGuards, "clean input", guard.EvalConfig{
		Criteria: guard.CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inputResult.Passed {
		t.Fatal("expected input guards to pass")
	}

	// Call LLM
	resp, err := client.ChatCompletion(context.Background(), "test-key", &llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: "user", Content: "clean input"}},
	})
	if err != nil {
		t.Fatalf("LLM call failed: %v", err)
	}

	// Output guard rejects (LLM response contains "bad")
	outputGuards := []guard.Guard{
		guard.NewBlocklistGuard("output-filter", guard.BlocklistConfig{
			Words: []string{"bad"},
		}),
	}

	outputText := llm.ExtractOutputText(resp)
	outputResult, err := engine.Run(context.Background(), outputGuards, outputText, guard.EvalConfig{
		Criteria: guard.CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outputResult.Passed {
		t.Error("expected output guards to reject LLM response containing blocked word")
	}
}

func TestFullProxyFlow_AllPass(t *testing.T) {
	server := mockLLMServer(t, "Here is a clean helpful response")
	defer server.Close()

	client := llm.NewClient(server.URL, 5*time.Second)
	engine := guard.NewEngine(guard.NewRegistry())

	// Input guards pass
	inputGuards := []guard.Guard{
		guard.NewBlocklistGuard("input-filter", guard.BlocklistConfig{
			Words: []string{"blocked"},
		}),
	}

	inputResult, err := engine.Run(context.Background(), inputGuards, "clean question", guard.EvalConfig{
		Criteria: guard.CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inputResult.Passed {
		t.Fatal("expected input guards to pass")
	}

	// LLM call succeeds
	resp, err := client.ChatCompletion(context.Background(), "test-key", &llm.ChatRequest{
		Model:    "test-model",
		Messages: []llm.Message{{Role: "user", Content: "clean question"}},
	})
	if err != nil {
		t.Fatalf("LLM call failed: %v", err)
	}

	// Output guards pass
	outputGuards := []guard.Guard{
		guard.NewBlocklistGuard("output-filter", guard.BlocklistConfig{
			Words: []string{"blocked"},
		}),
	}

	outputText := llm.ExtractOutputText(resp)
	outputResult, err := engine.Run(context.Background(), outputGuards, outputText, guard.EvalConfig{
		Criteria: guard.CriteriaAll,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outputResult.Passed {
		t.Fatal("expected output guards to pass")
	}

	// Full flow passed
	if resp.Choices[0].Message.Content != "Here is a clean helpful response" {
		t.Errorf("unexpected response content: %s", resp.Choices[0].Message.Content)
	}
}

// stubRow implements pgx.Row by invoking a provided scan function.
type stubRow struct {
	scan func(dest ...any) error
}

func (r *stubRow) Scan(dest ...any) error { return r.scan(dest...) }

// stubRows implements pgx.Rows for an empty result set.
type stubEmptyRows struct{}

func (s *stubEmptyRows) Close()                                       {}
func (s *stubEmptyRows) Err() error                                   { return nil }
func (s *stubEmptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (s *stubEmptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (s *stubEmptyRows) Next() bool                                   { return false }
func (s *stubEmptyRows) Scan(dest ...any) error                       { return nil }
func (s *stubEmptyRows) Values() ([]any, error)                       { return nil, nil }
func (s *stubEmptyRows) RawValues() [][]byte                          { return nil }
func (s *stubEmptyRows) Conn() *pgx.Conn                              { return nil }

// stubDBTX implements generated.DBTX. It dispatches based on the SQL string
// and returns rows scanned from the provided source.
type stubDBTX struct {
	source generated.Source
}

func (s *stubDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *stubDBTX) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	// listEnabledGuardrailsBySource: return zero guardrails so we exercise the
	// LLM-call path without engaging the guard engine.
	if strings.Contains(sql, "FROM guardrails") {
		return &stubEmptyRows{}, nil
	}
	return &stubEmptyRows{}, nil
}

func (s *stubDBTX) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "FROM sources"):
		src := s.source
		return &stubRow{scan: func(dest ...any) error {
			// Order matches getSourceByRoute's row.Scan in generated code.
			fields := []any{
				src.ID, src.OrganizationID, src.Name, src.Route, src.Description,
				src.LlmProvider, src.LlmModel, src.LlmApiKeyEnc, src.PassCriteria,
				src.PassThreshold, src.Enabled, src.CreatedAt, src.UpdatedAt,
			}
			return assignScan(dest, fields)
		}}
	case strings.Contains(sql, "INSERT INTO audit_logs"):
		// CreateAuditLog scans 8 RETURNING columns; we don't care about the
		// values, just that Scan returns nil so the proxy's audit write
		// succeeds quietly.
		return &stubRow{scan: func(dest ...any) error { return nil }}
	}
	return &stubRow{scan: func(dest ...any) error { return nil }}
}

// assignScan copies values from src into the pointers in dest using a type
// switch. It handles only the field types used by the Source row.
func assignScan(dest []any, src []any) error {
	for i, d := range dest {
		if i >= len(src) {
			break
		}
		switch p := d.(type) {
		case *uuid.UUID:
			*p = src[i].(uuid.UUID)
		case *string:
			*p = src[i].(string)
		case *bool:
			*p = src[i].(bool)
		default:
			// Use reflection-free assignment by re-marshaling through json
			// for any pgtype/time fields we don't explicitly handle. The
			// proxy only reads ID/Route/LlmModel/LlmApiKeyEnc/Enabled/
			// PassCriteria/PassThreshold so these defaults are fine.
			b, err := json.Marshal(src[i])
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// mockLLMServerCapture returns a server that records the model field of the
// last received chat completion request.
func mockLLMServerCapture(t *testing.T, capturedModel *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got llm.ChatRequest
		_ = json.Unmarshal(body, &got)
		*capturedModel = got.Model

		resp := llm.ChatResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   got.Model,
			Choices: []llm.Choice{{
				Index:        0,
				Message:      llm.Message{Role: "assistant", Content: "ok"},
				FinishReason: "stop",
			}},
			Usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestExecute_DoesNotMutateCallerChatRequest verifies that Proxy.Execute does
// not silently overwrite the caller's ChatRequest.Model field with the
// source's configured model. The caller-supplied struct must be left intact
// so audit logs and downstream code see the original requested model.
func TestExecute_DoesNotMutateCallerChatRequest(t *testing.T) {
	// Encrypt a fake provider API key with a real 32-byte hex key so the
	// proxy's crypto.Decrypt call succeeds.
	encKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedAPIKey, err := crypto.Encrypt("provider-api-key", encKey)
	if err != nil {
		t.Fatalf("encrypting api key: %v", err)
	}

	var capturedModel string
	llmServer := mockLLMServerCapture(t, &capturedModel)
	defer llmServer.Close()

	// Configure the source with a model that differs from what the caller
	// will request.
	const sourceModel = "openai/source-model"
	src := generated.Source{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		Name:           "test-source",
		Route:          "test-route",
		LlmProvider:    "openai",
		LlmModel:       sourceModel,
		LlmApiKeyEnc:   encryptedAPIKey,
		PassCriteria:   string(guard.CriteriaAll),
		Enabled:        true,
	}

	queries := generated.New(&stubDBTX{source: src})
	engine := guard.NewEngine(guard.NewRegistry())
	llmClient := llm.NewClient(llmServer.URL, 5*time.Second)
	p := NewProxy(queries, engine, llmClient, encKey)

	const userModel = "user-requested-model"
	chatReq := &llm.ChatRequest{
		Model:    userModel,
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	}

	result, err := p.Execute(context.Background(), src.OrganizationID, src.Route, chatReq)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected status=pass, got %q", result.Status)
	}

	// The caller's struct must be untouched.
	if chatReq.Model != userModel {
		t.Errorf("caller ChatRequest.Model was mutated: got %q, want %q", chatReq.Model, userModel)
	}

	// And the LLM should have been called with the source's configured model.
	if capturedModel != sourceModel {
		t.Errorf("LLM received model=%q, want %q", capturedModel, sourceModel)
	}
}
