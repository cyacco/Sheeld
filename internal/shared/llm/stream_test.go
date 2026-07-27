package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// sseServer serves a fixed SSE body, capturing the request body it received.
func sseServer(t *testing.T, events string) (*httptest.Server, *[]byte) {
	t.Helper()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, events)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func testClient(baseURL string) *Client {
	return NewClient(baseURL, 2*time.Second).WithRetry(2, time.Millisecond)
}

func simpleReq() *ChatRequest {
	return &ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}
}

// collect drains a stream, returning concatenated content, the last
// finish_reason, and any usage seen.
func collect(t *testing.T, s *ChatStream) (content, finish string, usage *Usage) {
	t.Helper()
	var b strings.Builder
	for s.Next() {
		c := s.Chunk()
		for _, ch := range c.Choices {
			b.WriteString(ch.Delta.Content)
			if ch.FinishReason != "" {
				finish = ch.FinishReason
			}
		}
		if c.Usage != nil {
			usage = c.Usage
		}
	}
	return b.String(), finish, usage
}

func TestChatCompletionStreamAt_HappyPath(t *testing.T) {
	events := `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":", world"},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}

data: [DONE]

`
	srv, _ := sseServer(t, events)
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	content, finish, usage := collect(t, stream)
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if content != "Hello, world" {
		t.Errorf("content = %q, want %q", content, "Hello, world")
	}
	if finish != "stop" {
		t.Errorf("finish_reason = %q, want stop", finish)
	}
	if usage == nil {
		t.Fatal("expected usage from the final chunk, got nil")
	}
	if usage.PromptTokens != 7 || usage.CompletionTokens != 3 || usage.TotalTokens != 10 {
		t.Errorf("usage = %+v, want 7/3/10", *usage)
	}
}

// Without include_usage providers omit usage entirely, which would silently
// empty the audit log's token fields and under-report cost.
func TestChatCompletionStreamAt_ForcesStreamAndIncludeUsage(t *testing.T) {
	srv, body := sseServer(t, "data: [DONE]\n\n")
	req := simpleReq()
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	stream.Close()

	var sent map[string]any
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("decoding sent body: %v", err)
	}
	if sent["stream"] != true {
		t.Errorf("stream = %v, want true", sent["stream"])
	}
	opts, ok := sent["stream_options"].(map[string]any)
	if !ok {
		t.Fatalf("stream_options missing or wrong shape: %v", sent["stream_options"])
	}
	if opts["include_usage"] != true {
		t.Errorf("include_usage = %v, want true", opts["include_usage"])
	}
	// The caller's request must be untouched.
	if req.Stream {
		t.Error("caller's request was mutated: Stream is now true")
	}
}

func TestChatCompletionStreamAt_PreservesCallerStreamOptions(t *testing.T) {
	srv, body := sseServer(t, "data: [DONE]\n\n")
	req := simpleReq()
	if err := json.Unmarshal([]byte(`{"model":"m","messages":[],"stream_options":{"custom":42}}`), req); err != nil {
		t.Fatalf("seeding request: %v", err)
	}
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	stream.Close()

	var sent struct {
		StreamOptions map[string]any `json:"stream_options"`
	}
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("decoding sent body: %v", err)
	}
	if sent.StreamOptions["include_usage"] != true {
		t.Errorf("include_usage = %v, want true", sent.StreamOptions["include_usage"])
	}
	if sent.StreamOptions["custom"] != float64(42) {
		t.Errorf("caller's stream_options.custom lost: %v", sent.StreamOptions["custom"])
	}
}

// Raw preserves fields the pipeline doesn't model, so later phases can re-emit
// tool-call deltas faithfully instead of dropping them.
func TestChatCompletionStreamAt_PreservesRawChunk(t *testing.T) {
	events := `data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"f"}}]},"finish_reason":null}]}

data: [DONE]

`
	srv, _ := sseServer(t, events)
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	if !stream.Next() {
		t.Fatalf("expected a chunk, err = %v", stream.Err())
	}
	if !strings.Contains(string(stream.Chunk().Raw), "tool_calls") {
		t.Errorf("Raw lost unmodeled fields: %s", stream.Chunk().Raw)
	}
}

func TestChatCompletionStreamAt_SSEFraming(t *testing.T) {
	tests := []struct {
		name   string
		events string
		want   string
	}{
		{
			name:   "skips comments and event lines",
			events: ": ping\nevent: message\ndata: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: [DONE]\n\n",
			want:   "a",
		},
		{
			name:   "joins multi-line data fields",
			events: "data: {\"choices\":[{\"delta\":\ndata: {\"content\":\"b\"}}]}\n\ndata: [DONE]\n\n",
			want:   "b",
		},
		{
			name:   "tolerates absent trailing DONE",
			events: "data: {\"choices\":[{\"delta\":{\"content\":\"c\"}}]}\n\n",
			want:   "c",
		},
		{
			name:   "tolerates a final line without a newline",
			events: "data: {\"choices\":[{\"delta\":{\"content\":\"d\"}}]}",
			want:   "d",
		},
		{
			name:   "handles CRLF line endings",
			events: "data: {\"choices\":[{\"delta\":{\"content\":\"e\"}}]}\r\n\r\ndata: [DONE]\r\n\r\n",
			want:   "e",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := sseServer(t, tt.events)
			stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
			if err != nil {
				t.Fatalf("open stream: %v", err)
			}
			defer stream.Close()

			got, _, _ := collect(t, stream)
			if err := stream.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}
			if got != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatCompletionStreamAt_MalformedChunk(t *testing.T) {
	srv, _ := sseServer(t, "data: {not json}\n\n")
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	if stream.Next() {
		t.Error("Next returned true on a malformed chunk")
	}
	if stream.Err() == nil {
		t.Error("expected a decode error, got nil")
	}
}

func TestChatCompletionStreamAt_ErrorStatuses(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		wantAttempts int32
		wantErrText  string
	}{
		{
			name:         "400 is not retried",
			status:       http.StatusBadRequest,
			body:         `{"error":{"message":"bad model"}}`,
			wantAttempts: 1,
			wantErrText:  "bad model",
		},
		{
			name:         "500 is retried up to the limit",
			status:       http.StatusInternalServerError,
			body:         `{"error":{"message":"boom"}}`,
			wantAttempts: 3,
			wantErrText:  "boom",
		},
		{
			name:         "429 is retried up to the limit",
			status:       http.StatusTooManyRequests,
			body:         `{"error":{"message":"slow down"}}`,
			wantAttempts: 3,
			wantErrText:  "slow down",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attempts, 1)
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			_, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErrText)
			}
			if got := atomic.LoadInt32(&attempts); got != tt.wantAttempts {
				t.Errorf("attempts = %d, want %d", got, tt.wantAttempts)
			}
		})
	}
}

// A transient failure before any data is safe to retry, unlike a mid-stream
// failure which would duplicate delivered content.
func TestChatCompletionStreamAt_RetrySucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	got, _, _ := collect(t, stream)
	if got != "ok" {
		t.Errorf("content = %q, want %q", got, "ok")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

// A client that goes away must not leave the upstream stream running; the
// request context is the cancellation path.
func TestChatCompletionStreamAt_ContextCancelMidStream(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(ctx, "", "k", simpleReq())
	if err != nil {
		cancel()
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	if !stream.Next() {
		cancel()
		t.Fatalf("expected the first chunk, err = %v", stream.Err())
	}
	cancel()

	if stream.Next() {
		t.Error("Next returned true after cancellation")
	}
	if stream.Err() == nil {
		t.Error("expected an error after cancellation, got nil")
	}
}

func TestChatStream_CloseIsIdempotent(t *testing.T) {
	srv, _ := sseServer(t, "data: [DONE]\n\n")
	stream, err := testClient(srv.URL).ChatCompletionStreamAt(context.Background(), "", "k", simpleReq())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
