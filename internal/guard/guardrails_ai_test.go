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
		FailOpen:  false, // fail-closed (default)
	})

	_, err := g.Validate(context.Background(), "test")
	if err == nil {
		t.Error("expected error in fail-closed mode, got nil")
	}
}

func TestGuardrailsAIGuard_ServerError_FailOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL: srv.URL,
		GuardName: "my-guard",
		FailOpen:  true,
	})

	result, err := g.Validate(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error in fail-open mode: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass in fail-open mode when server errors")
	}
}

func TestGuardrailsAIGuard_ConnectionFailure_FailOpen(t *testing.T) {
	// Point at a port nothing is listening on.
	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL:      "http://127.0.0.1:19999",
		GuardName:      "my-guard",
		FailOpen:       true,
		TimeoutSeconds: 1,
	})

	result, err := g.Validate(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error in fail-open mode: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass in fail-open mode on connection failure")
	}
	if _, ok := result.Details["error"]; !ok {
		t.Error("expected error detail to be populated")
	}
}

func TestGuardrailsAIGuard_ConnectionFailure_FailClosed(t *testing.T) {
	g := NewGuardrailsAIGuard("gr", GuardrailsAIConfig{
		ServerURL:      "http://127.0.0.1:19999",
		GuardName:      "my-guard",
		FailOpen:       false,
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
