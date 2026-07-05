package transform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
)

const (
	webhookDefaultTimeout   = 10
	webhookMaxResponseBytes = 1 << 20 // 1 MiB
)

// WebhookConfig holds configuration for the webhook transformer, which
// rewrites messages via an arbitrary user-hosted HTTP endpoint.
type WebhookConfig struct {
	// URL is the endpoint POSTed to on every request. Must be http(s).
	URL string `json:"url"`

	// Headers are static headers set on every request (e.g. Authorization).
	Headers map[string]string `json:"headers,omitempty"`

	// TimeoutSeconds is the HTTP timeout. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// WebhookTransformer calls a user-configured HTTP endpoint to rewrite the
// messages array.
//
// Contract: POST {url} with {"messages": [{"role","content"}...], "phase",
// "source_route"}; the endpoint responds {"messages": [...]} — the full
// rewritten array, which replaces the request's messages. Connection errors,
// non-2xx statuses, and responses missing "messages" are returned as errors,
// so the on_error policy (fail_open/fail_closed) applies.
type WebhookTransformer struct {
	name   string
	cfg    WebhookConfig
	client *http.Client
}

// NewWebhookTransformer creates a new WebhookTransformer from configuration.
func NewWebhookTransformer(name string, cfg WebhookConfig) *WebhookTransformer {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = webhookDefaultTimeout
	}
	return &WebhookTransformer{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (t *WebhookTransformer) Name() string { return t.name }
func (t *WebhookTransformer) Type() string { return "webhook" }

// webhookRequest is the body sent to the configured endpoint.
type webhookRequest struct {
	Messages    []llm.Message `json:"messages"`
	Phase       string        `json:"phase"`
	SourceRoute string        `json:"source_route"`
}

// webhookResponse is the expected response. Messages is required; a response
// without it is an error rather than a silent no-op.
type webhookResponse struct {
	Messages []llm.Message `json:"messages"`
}

func (t *WebhookTransformer) Transform(ctx context.Context, msgs []llm.Message) ([]llm.Message, error) {
	meta, _ := guard.CallMetaFrom(ctx)
	body, err := json.Marshal(webhookRequest{
		Messages:    msgs,
		Phase:       meta.Phase,
		SourceRoute: meta.SourceRoute,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: calling %s: %w", t.cfg.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("webhook: %s returned status %d", t.cfg.URL, resp.StatusCode)
	}

	var wr webhookResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, webhookMaxResponseBytes)).Decode(&wr); err != nil {
		return nil, fmt.Errorf("webhook: decoding response: %w", err)
	}
	if wr.Messages == nil {
		return nil, fmt.Errorf("webhook: response missing required \"messages\" field")
	}
	return wr.Messages, nil
}
