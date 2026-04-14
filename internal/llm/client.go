package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an OpenAI-compatible HTTP client that talks to LiteLLM.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new LLM client pointed at the given gateway URL.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ChatCompletion sends a chat completion request through the LLM gateway.
// The apiKey is the provider API key (e.g., OpenAI key, Anthropic key) which
// gets forwarded by LiteLLM to the appropriate provider.
func (c *Client) ChatCompletion(ctx context.Context, apiKey string, req *ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request to LLM gateway: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &chatResp, nil
}

// StreamChatCompletion sends a streaming chat completion request to the LLM
// gateway. It forces stream=true on the outgoing request, parses the SSE
// frames, invokes onChunk for each parsed chunk, and accumulates the chunks
// into a fully reconstructed *ChatResponse so downstream output guards can
// still evaluate the complete assistant content.
//
// The callback may return an error to abort the stream. Context cancellation
// also aborts the stream promptly.
//
// NOTE: This method does NOT forward SSE frames to any client by itself. The
// proxy layer is expected to buffer the stream, run output guards on the
// reconstructed response, and only then replay the frames to the end user.
// Real progressive streaming (frames forwarded as they arrive, with output
// guards racing) is a follow-up.
func (c *Client) StreamChatCompletion(
	ctx context.Context,
	apiKey string,
	req *ChatRequest,
	onChunk func(*ChatResponseChunk) error,
) (*ChatResponse, error) {
	// Force stream=true on the outgoing request without mutating the
	// caller's struct in a surprising way: we copy the req shallowly first.
	streamReq := *req
	streamTrue := true
	streamReq.Stream = &streamTrue

	body, err := json.Marshal(&streamReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request to LLM gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Reconstructed response. We pull the static fields (ID, Model, etc.)
	// from the first chunk that supplies them; we accumulate Delta.Content
	// per choice index into the final Message.Content.
	var (
		reconstructed ChatResponse
		// contentByIndex accumulates streamed text per choice index.
		contentByIndex = map[int]*strings.Builder{}
		// finishByIndex captures the final finish_reason per choice.
		finishByIndex = map[int]string{}
		// roleByIndex captures the role from the first delta that supplies
		// it (typically the very first chunk).
		roleByIndex = map[int]string{}
	)

	scanner := bufio.NewScanner(resp.Body)
	// SSE frames can be larger than the default 64KB scanner buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		// Honor ctx cancellation between frames.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("stream cancelled: %w", err)
		}

		line := scanner.Text()
		if line == "" {
			// Frame separator.
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			// Comments/keepalives/event lines — ignore for now.
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk ChatResponseChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return nil, fmt.Errorf("decoding stream chunk: %w", err)
		}

		// Capture top-level metadata from the first chunk that has it.
		if reconstructed.ID == "" && chunk.ID != "" {
			reconstructed.ID = chunk.ID
		}
		if reconstructed.Object == "" && chunk.Object != "" {
			// Streaming chunks come back as "chat.completion.chunk"; the
			// reconstructed (non-streaming) response should advertise the
			// non-streaming object type.
			reconstructed.Object = "chat.completion"
		}
		if reconstructed.Created == 0 && chunk.Created != 0 {
			reconstructed.Created = chunk.Created
		}
		if reconstructed.Model == "" && chunk.Model != "" {
			reconstructed.Model = chunk.Model
		}

		for _, ch := range chunk.Choices {
			if _, ok := contentByIndex[ch.Index]; !ok {
				contentByIndex[ch.Index] = &strings.Builder{}
			}
			contentByIndex[ch.Index].WriteString(ch.Delta.Content)
			if ch.Delta.Role != "" {
				if _, ok := roleByIndex[ch.Index]; !ok {
					roleByIndex[ch.Index] = ch.Delta.Role
				}
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				finishByIndex[ch.Index] = *ch.FinishReason
			}
		}

		if err := onChunk(&chunk); err != nil {
			return nil, fmt.Errorf("stream callback error: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		// If context died mid-read, prefer that error message.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("stream cancelled: %w", ctxErr)
		}
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	// Materialize choices in deterministic index order.
	maxIdx := -1
	for idx := range contentByIndex {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	for i := 0; i <= maxIdx; i++ {
		role := roleByIndex[i]
		if role == "" {
			role = "assistant"
		}
		var content string
		if b, ok := contentByIndex[i]; ok {
			content = b.String()
		}
		reconstructed.Choices = append(reconstructed.Choices, Choice{
			Index:        i,
			Message:      Message{Role: role, Content: content},
			FinishReason: finishByIndex[i],
		})
	}

	return &reconstructed, nil
}
