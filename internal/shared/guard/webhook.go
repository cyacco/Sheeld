package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	webhookDefaultTimeout   = 10
	webhookMaxResponseBytes = 1 << 20 // 1 MiB
)

// WebhookConfig holds configuration for the webhook guard, which validates
// text against an arbitrary user-hosted HTTP endpoint.
type WebhookConfig struct {
	// URL is the endpoint POSTed to on every validation. Must be http(s).
	URL string `json:"url"`

	// Headers are static headers set on every request (e.g. Authorization).
	Headers map[string]string `json:"headers,omitempty"`

	// TimeoutSeconds is the HTTP timeout. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// WebhookGuard calls a user-configured HTTP endpoint to validate text.
//
// Contract: POST {url} with {"input", "phase", "source_route"}; the endpoint
// responds {"passed": bool, "message"?: string, "details"?: object}.
// Connection errors, non-2xx statuses, and malformed responses are returned
// as errors, so the engine's on_error policy (fail_open/fail_closed)
// applies.
type WebhookGuard struct {
	name   string
	cfg    WebhookConfig
	client *http.Client
}

// NewWebhookGuard creates a new WebhookGuard from configuration.
func NewWebhookGuard(name string, cfg WebhookConfig) *WebhookGuard {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = webhookDefaultTimeout
	}
	return &WebhookGuard{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (g *WebhookGuard) Type() string { return "webhook" }
func (g *WebhookGuard) Name() string { return g.name }

// webhookRequest is the body sent to the configured endpoint.
type webhookRequest struct {
	Input       string `json:"input"`
	Phase       string `json:"phase"`
	SourceRoute string `json:"source_route"`
}

// webhookResponse is the expected response. Passed is a pointer so a
// response missing the field is distinguishable from an explicit false.
type webhookResponse struct {
	Passed  *bool                  `json:"passed"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (g *WebhookGuard) Validate(ctx context.Context, input string) (*Result, error) {
	start := time.Now()

	meta, _ := CallMetaFrom(ctx)
	body, err := json.Marshal(webhookRequest{
		Input:       input,
		Phase:       meta.Phase,
		SourceRoute: meta.SourceRoute,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range g.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: calling %s: %w", g.cfg.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("webhook: %s returned status %d", g.cfg.URL, resp.StatusCode)
	}

	var wr webhookResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, webhookMaxResponseBytes)).Decode(&wr); err != nil {
		return nil, fmt.Errorf("webhook: decoding response: %w", err)
	}
	if wr.Passed == nil {
		return nil, fmt.Errorf("webhook: response missing required \"passed\" field")
	}

	message := wr.Message
	if message == "" {
		if *wr.Passed {
			message = "input passed webhook validation"
		} else {
			message = "input rejected by webhook"
		}
	}

	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    *wr.Passed,
		Message:   message,
		Details:   wr.Details,
		Duration:  time.Since(start),
	}, nil
}
