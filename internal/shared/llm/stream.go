package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cyacco/Sheeld/internal/shared/metrics"
)

// StreamChunk is one decoded chat.completion.chunk SSE event.
type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`

	// Usage is present only on the final usage chunk, which providers emit
	// when stream_options.include_usage is set. Nil on content chunks.
	Usage *Usage `json:"usage,omitempty"`

	// Raw is the chunk's original JSON. Kept so the proxy can re-emit
	// provider fields it doesn't model (tool_call deltas, logprobs, …)
	// without losing them, the same way ChatRequest/ChatResponse preserve
	// unmodeled fields.
	Raw json.RawMessage `json:"-"`
}

// StreamChoice is one choice within a chunk.
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

// StreamDelta is the incremental payload for a choice.
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ChatStream is a forward-only reader over a provider's SSE response, used
// like bufio.Scanner:
//
//	stream, err := client.ChatCompletionStreamAt(ctx, baseURL, key, req)
//	if err != nil { ... }
//	defer stream.Close()
//	for stream.Next() {
//		chunk := stream.Chunk()
//	}
//	if err := stream.Err(); err != nil { ... }
//
// It holds no goroutines: cancellation happens through the request context,
// so an abandoned stream cannot leak. Close must be called to release the
// connection.
type ChatStream struct {
	body io.ReadCloser
	r    *bufio.Reader

	cur  StreamChunk
	err  error
	done bool

	// recorded guards the terminal metric so it fires exactly once.
	recorded bool
}

// Chunk returns the chunk decoded by the most recent Next.
func (s *ChatStream) Chunk() StreamChunk { return s.cur }

// Err returns the first error encountered, if any. io.EOF is not an error:
// a stream that ends with [DONE] or a clean close reports nil.
func (s *ChatStream) Err() error { return s.err }

// Close releases the underlying connection. Safe to call more than once.
func (s *ChatStream) Close() error {
	if s.body == nil {
		return nil
	}
	err := s.body.Close()
	s.body = nil
	s.finish()
	return err
}

// finish records the terminal success/error metric once.
func (s *ChatStream) finish() {
	if s.recorded {
		return
	}
	s.recorded = true
	if s.err != nil {
		metrics.LLMRequests.WithLabelValues("error").Inc()
	} else {
		metrics.LLMRequests.WithLabelValues("success").Inc()
	}
}

// Next advances to the next chunk, reporting false at end of stream or on
// error. Comment lines, event: lines, and blank separators are skipped; a
// multi-line data field is joined with newlines per the SSE spec.
func (s *ChatStream) Next() bool {
	if s.done || s.err != nil {
		return false
	}

	var data []string
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			if len(strings.TrimSpace(line)) == 0 {
				// Clean end of stream: providers may close without [DONE].
				if !errors.Is(err, io.EOF) {
					s.err = fmt.Errorf("reading stream: %w", err)
				}
				s.done = true
				s.finish()
				return false
			}
			// A final line without its trailing newline: process it, then
			// stop on the next call.
			s.done = true
		}

		line = strings.TrimRight(line, "\r\n")

		// Blank line terminates an event.
		if line == "" {
			if len(data) == 0 {
				if s.done {
					s.finish()
					return false
				}
				continue // stray separator
			}
			break
		}
		// Comments and non-data fields carry nothing the pipeline needs.
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if field != "data" {
			continue
		}
		data = append(data, strings.TrimPrefix(value, " "))

		if s.done {
			break
		}
	}

	payload := strings.Join(data, "\n")
	if payload == "[DONE]" {
		s.done = true
		s.finish()
		return false
	}

	var chunk StreamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		s.err = fmt.Errorf("decoding stream chunk: %w", err)
		s.finish()
		return false
	}
	chunk.Raw = json.RawMessage(payload)
	s.cur = chunk

	if chunk.Usage != nil {
		metrics.LLMTokens.WithLabelValues("prompt").Add(float64(chunk.Usage.PromptTokens))
		metrics.LLMTokens.WithLabelValues("completion").Add(float64(chunk.Usage.CompletionTokens))
	}
	return true
}

// ChatCompletionStream opens a streaming chat completion against the client's
// default base URL. See ChatCompletionStreamAt.
func (c *Client) ChatCompletionStream(ctx context.Context, apiKey string, req *ChatRequest) (*ChatStream, error) {
	return c.ChatCompletionStreamAt(ctx, "", apiKey, req)
}

// ChatCompletionStreamAt opens a streaming chat completion against an
// OpenAI-compatible endpoint. The returned stream must be closed by the
// caller.
//
// The outgoing request always sets stream: true and
// stream_options.include_usage: true — without the latter, providers omit
// token usage from streams and audit/cost data would silently go empty. The
// caller's request is not modified.
//
// Retries cover only failures that happen before any response body is read
// (connection errors, HTTP 429/5xx): once chunks are flowing a retry would
// duplicate already-delivered content, so mid-stream failures surface through
// the stream's Err.
func (c *Client) ChatCompletionStreamAt(ctx context.Context, baseURL, apiKey string, req *ChatRequest) (*ChatStream, error) {
	if baseURL == "" {
		baseURL = c.baseURL
	}

	body, err := streamRequestBody(req)
	if err != nil {
		metrics.LLMRequests.WithLabelValues("error").Inc()
		return nil, err
	}

	backoff := c.baseBackoff
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				metrics.LLMRequests.WithLabelValues("error").Inc()
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			metrics.LLMRetries.Inc()
			backoff *= 2
			slog.Warn("retrying LLM gateway stream request",
				"attempt", attempt+1, "max_attempts", c.maxRetries+1, "error", lastErr)
		}

		stream, retryable, err := c.openStream(ctx, baseURL, apiKey, body)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			metrics.LLMRequests.WithLabelValues("error").Inc()
			return nil, err
		}
	}
	metrics.LLMRequests.WithLabelValues("error").Inc()
	return nil, fmt.Errorf("LLM gateway stream request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// openStream performs a single streaming attempt, returning a stream
// positioned before the first chunk. The bool reports whether a failure is
// transient and safe to retry — always true only for pre-body failures.
func (c *Client) openStream(ctx context.Context, baseURL, apiKey string, body []byte) (*ChatStream, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.streamClient().Do(httpReq)
	if err != nil {
		retryable := ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		return nil, retryable, fmt.Errorf("sending stream request to LLM gateway: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, retryable, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, retryable, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return &ChatStream{body: resp.Body, r: bufio.NewReader(resp.Body)}, false, nil
}

// streamRequestBody marshals req with stream and stream_options.include_usage
// forced on, without mutating req.
func streamRequestBody(req *ChatRequest) ([]byte, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	fields["stream"] = json.RawMessage("true")

	// Merge into any caller-supplied stream_options rather than replacing it,
	// so other options survive.
	opts := map[string]json.RawMessage{}
	if existing, ok := fields["stream_options"]; ok {
		_ = json.Unmarshal(existing, &opts)
	}
	opts["include_usage"] = json.RawMessage("true")
	merged, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshaling stream_options: %w", err)
	}
	fields["stream_options"] = merged

	out, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	return out, nil
}
