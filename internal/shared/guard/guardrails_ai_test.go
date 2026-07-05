package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func guardrailsHandler(t *testing.T, guardName string, pass bool, message string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/guards/" + guardName + "/validate"
		if r.URL.Path != expectedPath {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(guardrailsAIResponse{
			ValidationPassed: pass,
			Message:          message,
		})
	}
}

func TestGuardrailsAIGuard_Pass(t *testing.T) {
	srv := httptest.NewServer(guardrailsHandler(t, "my-guard", true, "ok"))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "my-guard",
	})

	result, err := g.Validate(context.Background(), "safe input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got fail: %s", result.Message)
	}
}

func TestGuardrailsAIGuard_Fail(t *testing.T) {
	srv := httptest.NewServer(guardrailsHandler(t, "my-guard", false, "input violates policy"))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "my-guard",
	})

	result, err := g.Validate(context.Background(), "bad input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail, got pass")
	}
	if result.Message != "input violates policy" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestGuardrailsAIGuard_ServerError_FailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "my-guard",
	})

	_, err := g.Validate(context.Background(), "test")
	if err == nil {
		t.Error("expected error in fail-closed mode, got nil")
	}
}

func TestGuardrailsAIGuard_ServerError_FailOpenViaEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "my-guard",
	})

	// Fail-open now lives in the engine via the generic on_error policy.
	engine := NewEngine(NewRegistry())
	res, err := engine.Run(context.Background(), []Guard{WithFailOpen(g)}, "test", EvalConfig{Criteria: CriteriaAll})
	if err != nil {
		t.Fatalf("unexpected engine error: %v", err)
	}
	if !res.Passed {
		t.Error("expected fail-open errored guard to pass overall")
	}
	if res.Results[0].Details["errored"] != true {
		t.Error("expected result marked as errored")
	}
}

func TestGuardrailsAIGuard_ConnectionFailure_ReturnsError(t *testing.T) {
	// Point at a port nothing is listening on.
	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL:      "http://127.0.0.1:19999",
		GuardName:      "my-guard",
		TimeoutSeconds: 1,
	})

	_, err := g.Validate(context.Background(), "test")
	if err == nil {
		t.Error("expected error on connection failure (fail-open is now an engine policy)")
	}
}

func TestGuardrailsAIGuard_ConnectionFailure_FailClosed(t *testing.T) {
	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL:      "http://127.0.0.1:19999",
		GuardName:      "my-guard",
		TimeoutSeconds: 1,
	})

	_, err := g.Validate(context.Background(), "test")
	if err == nil {
		t.Error("expected error in fail-closed mode on connection failure")
	}
}

func TestGuardrailsAIGuard_CorrectPathAndMethod(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/guards/test-guard/validate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(guardrailsAIResponse{ValidationPassed: true})
	}))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "test-guard",
	})

	if _, err := g.Validate(context.Background(), "input"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was never called")
	}
}
