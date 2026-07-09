package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ChatCompletion_Success(t *testing.T) {
	mockResp := ChatResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4o",
		Choices: []Choice{
			{
				Index:        0,
				Message:      Message{Role: "assistant", Content: "Hello! How can I help you?"},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify body is valid ChatRequest
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if req.Model != "openai/gpt-4o" {
			t.Errorf("expected model openai/gpt-4o, got %s", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	resp, err := client.ChatCompletion(context.Background(), "test-key", &ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("got id=%q, want %q", resp.ID, "chatcmpl-123")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("got %d choices, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello! How can I help you?" {
		t.Errorf("got content=%q, want %q", resp.Choices[0].Message.Content, "Hello! How can I help you?")
	}
	if resp.Usage.TotalTokens != 18 {
		t.Errorf("got total_tokens=%d, want 18", resp.Usage.TotalTokens)
	}
}

func TestClient_ChatCompletion_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}{
				Message: "Invalid API key",
				Type:    "authentication_error",
				Code:    "invalid_api_key",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	_, err := client.ChatCompletion(context.Background(), "bad-key", &ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestClient_ChatCompletion_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, 50*time.Millisecond).WithRetry(0, 0)

	_, err := client.ChatCompletion(context.Background(), "key", &ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExtractInputText(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{
			name: "single user message",
			messages: []Message{
				{Role: "user", Content: "Hello"},
			},
			want: "Hello",
		},
		{
			name: "system + user",
			messages: []Message{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "What is 2+2?"},
			},
			want: "What is 2+2?",
		},
		{
			name: "multi-turn takes last user message",
			messages: []Message{
				{Role: "user", Content: "First question"},
				{Role: "assistant", Content: "First answer"},
				{Role: "user", Content: "Follow-up question"},
			},
			want: "Follow-up question",
		},
		{
			name:     "no user messages",
			messages: []Message{{Role: "system", Content: "You are helpful"}},
			want:     "",
		},
		{
			name:     "empty messages",
			messages: []Message{},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatRequest{Messages: tt.messages}
			got := ExtractInputText(req)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractOutputText(t *testing.T) {
	tests := []struct {
		name    string
		choices []Choice
		want    string
	}{
		{
			name: "single choice",
			choices: []Choice{
				{Message: Message{Role: "assistant", Content: "Hello!"}},
			},
			want: "Hello!",
		},
		{
			name:    "no choices",
			choices: []Choice{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ChatResponse{Choices: tt.choices}
			got := ExtractOutputText(resp)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func okResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
}

func TestClient_RetriesTransientThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okResponse(w)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second).WithRetry(2, time.Millisecond)
	resp, err := client.ChatCompletion(context.Background(), "k", &ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("unexpected content: %q", resp.Choices[0].Message.Content)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second).WithRetry(3, time.Millisecond)
	if _, err := client.ChatCompletion(context.Background(), "k", &ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("4xx must not retry: got %d attempts", got)
	}
}

func TestClient_RetriesExhausted(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second).WithRetry(2, time.Millisecond)
	if _, err := client.ChatCompletion(context.Background(), "k", &ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts (1 + 2 retries), got %d", got)
	}
}

func TestClient_RespectsContextDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	// Long backoff, but a short context deadline: must return promptly, not
	// wait out the backoff.
	client := NewClient(server.URL, 5*time.Second).WithRetry(5, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := client.ChatCompletion(ctx, "k", &ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("expected error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("should abort on context, took %s", elapsed)
	}
}

func TestClient_ChatCompletionAt_OverridesBaseURL(t *testing.T) {
	defaultHit, overrideHit := false, false
	defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultHit = true
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"from default"}}]}`))
	}))
	defer defaultServer.Close()
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		overrideHit = true
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"from override"}}]}`))
	}))
	defer overrideServer.Close()

	client := NewClient(defaultServer.URL, 5*time.Second)

	// Override sends the request to the per-source endpoint, not the default.
	resp, err := client.ChatCompletionAt(context.Background(), overrideServer.URL, "key", &ChatRequest{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !overrideHit || defaultHit {
		t.Fatalf("expected only the override server to be hit (override=%v default=%v)", overrideHit, defaultHit)
	}
	if got := resp.Choices[0].Message.Content; got != "from override" {
		t.Fatalf("unexpected content: %q", got)
	}

	// Empty override falls back to the default endpoint.
	if _, err := client.ChatCompletionAt(context.Background(), "", "key", &ChatRequest{Model: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !defaultHit {
		t.Fatal("expected fallback to hit the default server")
	}
}
