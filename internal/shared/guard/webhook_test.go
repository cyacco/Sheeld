package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func webhookServer(t *testing.T, handler func(w http.ResponseWriter, body webhookRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body webhookRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, body)
	}))
}

func TestWebhookGuard_Pass(t *testing.T) {
	var gotBody webhookRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"passed": true}`))
	}))
	defer srv.Close()

	g := NewWebhookGuard("hook", WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer secret"},
	})

	ctx := WithCallMeta(context.Background(), CallMeta{Phase: "input", SourceRoute: "chat"})
	result, err := g.Validate(ctx, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected pass")
	}
	if gotBody.Input != "hello" || gotBody.Phase != "input" || gotBody.SourceRoute != "chat" {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
}

func TestWebhookGuard_Fail_PropagatesMessageAndDetails(t *testing.T) {
	srv := webhookServer(t, func(w http.ResponseWriter, _ webhookRequest) {
		w.Write([]byte(`{"passed": false, "message": "PII detected", "details": {"entities": ["SSN"]}}`))
	})
	defer srv.Close()

	g := NewWebhookGuard("hook", WebhookConfig{URL: srv.URL})
	result, err := g.Validate(context.Background(), "my ssn is 123-45-6789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail")
	}
	if result.Message != "PII detected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if result.Details["entities"] == nil {
		t.Error("expected details passed through")
	}
}

func TestWebhookGuard_MissingCallMeta(t *testing.T) {
	var gotBody webhookRequest
	srv := webhookServer(t, func(w http.ResponseWriter, body webhookRequest) {
		gotBody = body
		w.Write([]byte(`{"passed": true}`))
	})
	defer srv.Close()

	g := NewWebhookGuard("hook", WebhookConfig{URL: srv.URL})
	if _, err := g.Validate(context.Background(), "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody.Phase != "" || gotBody.SourceRoute != "" {
		t.Errorf("expected empty meta fields, got %+v", gotBody)
	}
}

func TestWebhookGuard_Errors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"non-2xx status", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
		{"malformed JSON", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{not json`))
		}},
		{"missing passed field", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"message": "no verdict"}`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			g := NewWebhookGuard("hook", WebhookConfig{URL: srv.URL})
			if _, err := g.Validate(context.Background(), "x"); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestWebhookGuard_ConnectionRefused(t *testing.T) {
	g := NewWebhookGuard("hook", WebhookConfig{URL: "http://127.0.0.1:19998", TimeoutSeconds: 1})
	if _, err := g.Validate(context.Background(), "x"); err == nil {
		t.Error("expected error on connection failure")
	}
}

func TestWebhookGuard_TimeoutHonored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{"passed": true}`))
	}))
	defer srv.Close()

	g := NewWebhookGuard("hook", WebhookConfig{URL: srv.URL, TimeoutSeconds: 1})
	start := time.Now()
	_, err := g.Validate(context.Background(), "x")
	if err == nil {
		t.Error("expected timeout error")
	}
	if time.Since(start) > 1800*time.Millisecond {
		t.Error("timeout not honored")
	}
}

func TestWebhookFactory_Validation(t *testing.T) {
	r := NewRegistry()
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{"valid", `{"url": "https://example.com/validate"}`, ""},
		{"missing url", `{}`, "url is required"},
		{"bad scheme", `{"url": "ftp://example.com"}`, "http(s)"},
		{"not a url", `{"url": "::bogus::"}`, "http(s)"},
		{"invalid json", `{`, "invalid webhook config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Create("webhook", "w", json.RawMessage(tt.config))
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
