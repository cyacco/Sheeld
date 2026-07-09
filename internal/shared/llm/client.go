package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sheeld/sheeld/internal/shared/metrics"
)

// Client is an HTTP client for OpenAI-compatible chat-completions endpoints
// (a provider directly, or a gateway such as LiteLLM).
type Client struct {
	baseURL    string
	httpClient *http.Client

	// maxRetries is the number of retries after the first attempt (so
	// maxRetries=2 means up to 3 attempts). Retries apply only to transient
	// failures: connection errors, HTTP 429, and HTTP 5xx.
	maxRetries  int
	baseBackoff time.Duration
}

// NewClient creates a new LLM client pointed at the given gateway URL. By
// default it retries transient failures twice; tune with WithRetry.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries:  2,
		baseBackoff: 200 * time.Millisecond,
	}
}

// WithRetry configures transient-failure retries. maxRetries is retries after
// the first attempt (0 disables retrying); baseBackoff is the first delay,
// doubling each retry. Returns the client for chaining.
func (c *Client) WithRetry(maxRetries int, baseBackoff time.Duration) *Client {
	if maxRetries < 0 {
		maxRetries = 0
	}
	c.maxRetries = maxRetries
	if baseBackoff > 0 {
		c.baseBackoff = baseBackoff
	}
	return c
}

// ChatCompletion sends a chat completion request to the client's default
// base URL. See ChatCompletionAt.
func (c *Client) ChatCompletion(ctx context.Context, apiKey string, req *ChatRequest) (*ChatResponse, error) {
	return c.ChatCompletionAt(ctx, "", apiKey, req)
}

// ChatCompletionAt sends a chat completion request to an OpenAI-compatible
// endpoint, retrying transient failures with exponential backoff. baseURL
// overrides the client's default endpoint (a provider directly, or a gateway);
// empty means use the default. The apiKey is sent as the bearer token.
func (c *Client) ChatCompletionAt(ctx context.Context, baseURL, apiKey string, req *ChatRequest) (resp *ChatResponse, err error) {
	defer func() {
		if err != nil {
			metrics.LLMRequests.WithLabelValues("error").Inc()
		} else {
			metrics.LLMRequests.WithLabelValues("success").Inc()
		}
	}()

	if baseURL == "" {
		baseURL = c.baseURL
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	backoff := c.baseBackoff
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retrying, honoring context cancellation.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			metrics.LLMRetries.Inc()
			backoff *= 2
			slog.Warn("retrying LLM gateway request",
				"attempt", attempt+1, "max_attempts", c.maxRetries+1, "error", lastErr)
		}

		resp, retryable, err := c.doOnce(ctx, baseURL, apiKey, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Don't burn a backoff sleep if the error isn't worth retrying or
		// the context is already done.
		if !retryable || ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("LLM gateway request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// doOnce performs a single attempt. The bool reports whether the error (if
// any) is transient and worth retrying.
func (c *Client) doOnce(ctx context.Context, baseURL, apiKey string, body []byte) (*ChatResponse, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Connection-level failures are transient unless the caller's context
		// was cancelled or timed out.
		retryable := ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		return nil, retryable, fmt.Errorf("sending request to LLM gateway: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 429 and 5xx are transient; other 4xx are deterministic.
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, retryable, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, retryable, fmt.Errorf("LLM gateway error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, false, fmt.Errorf("decoding response: %w", err)
	}
	return &chatResp, false, nil
}
