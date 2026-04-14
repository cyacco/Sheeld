package llm

import (
	"context"
	"encoding/json"
	"fmt"
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

	client := NewClient(server.URL, 50*time.Millisecond)

	_, err := client.ChatCompletion(context.Background(), "key", &ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClient_StreamChatCompletion_Success(t *testing.T) {
	// Three content chunks + a final chunk carrying finish_reason, then [DONE].
	frames := []string{
		`{"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":", "}}]}`,
		`{"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"world!"},"finish_reason":"stop"}]}`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("expected Accept text/event-stream, got %s", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", got)
		}

		// Verify the outgoing payload had stream=true forced on.
		var sent ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if sent.Stream == nil || !*sent.Stream {
			t.Errorf("expected stream=true to be forced on outgoing request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	var chunkCount int
	resp, err := client.StreamChatCompletion(
		context.Background(),
		"test-key",
		&ChatRequest{
			Model:    "openai/gpt-4o",
			Messages: []Message{{Role: "user", Content: "Hi"}},
		},
		func(_ *ChatResponseChunk) error {
			chunkCount++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunkCount != 3 {
		t.Errorf("expected 3 chunk callbacks, got %d", chunkCount)
	}
	if resp == nil {
		t.Fatal("expected reconstructed response, got nil")
	}
	if resp.ID != "chatcmpl-stream-1" {
		t.Errorf("got id=%q, want chatcmpl-stream-1", resp.ID)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("got object=%q, want chat.completion", resp.Object)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("got model=%q, want gpt-4o", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("got %d choices, want 1", len(resp.Choices))
	}
	if got, want := resp.Choices[0].Message.Content, "Hello, world!"; got != want {
		t.Errorf("got reconstructed content=%q, want %q", got, want)
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("got role=%q, want assistant", resp.Choices[0].Message.Role)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("got finish_reason=%q, want stop", resp.Choices[0].FinishReason)
	}
}

func TestClient_StreamChatCompletion_ContextCancellation(t *testing.T) {
	// Server emits one frame, flushes, then sleeps long enough that the test
	// can cancel ctx mid-stream. The client should bail out promptly.
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		fmt.Fprint(w, `data: {"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Hold the connection open. The client-side ctx cancellation will
		// tear down the request.
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	var sawChunk int32
	errCh := make(chan error, 1)
	go func() {
		_, err := client.StreamChatCompletion(
			ctx,
			"test-key",
			&ChatRequest{
				Model:    "openai/gpt-4o",
				Messages: []Message{{Role: "user", Content: "Hi"}},
			},
			func(_ *ChatResponseChunk) error {
				atomic.StoreInt32(&sawChunk, 1)
				// Cancel ctx on receipt of the first chunk to simulate the
				// caller bailing out mid-stream.
				cancel()
				return nil
			},
		)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from cancelled stream, got nil")
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("StreamChatCompletion did not return promptly after ctx cancel")
	}
	if atomic.LoadInt32(&sawChunk) == 0 {
		t.Error("expected at least one chunk callback before cancellation")
	}
	// Drain the server goroutine so the test doesn't leak.
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
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
