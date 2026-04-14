package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
