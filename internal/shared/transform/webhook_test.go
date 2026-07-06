package transform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
)

func TestWebhookTransform(t *testing.T) {
	var gotBody webhookRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		msgs := make([]llm.Message, len(gotBody.Messages))
		copy(msgs, gotBody.Messages)
		for i := range msgs {
			msgs[i].Content = strings.ReplaceAll(msgs[i].Content, "secret", "[MASKED]")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": msgs})
	}))
	defer srv.Close()

	tr := NewWebhookTransformer("hook", WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer tok"},
	})

	ctx := guard.WithCallMeta(context.Background(), guard.CallMeta{Phase: "input", SourceRoute: "chat"})
	out, err := tr.Transform(ctx, []llm.Message{{Role: "user", Content: "the secret is 42"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out[0].Content, "the [MASKED] is 42"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody.Phase != "input" || gotBody.SourceRoute != "chat" {
		t.Errorf("meta = %+v", gotBody)
	}
}

func TestWebhookTransformErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"non-2xx", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }},
		{"malformed json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("nope")) }},
		{"missing messages", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			tr := NewWebhookTransformer("hook", WebhookConfig{URL: srv.URL})
			if _, err := tr.Transform(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestWebhookFactoryValidation(t *testing.T) {
	if _, err := webhookFactory("t", json.RawMessage(`{}`)); err == nil {
		t.Error("missing url accepted")
	}
	if _, err := webhookFactory("t", json.RawMessage(`{"url":"ftp://x"}`)); err == nil {
		t.Error("non-http url accepted")
	}
	if _, err := webhookFactory("t", json.RawMessage(`{"url":"https://example.com/x"}`)); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}
