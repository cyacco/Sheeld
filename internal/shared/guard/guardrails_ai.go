package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const guardrailsAIDefaultTimeout = 10

// GuardrailsAIConfig holds configuration for the guardrails.ai guard.
type GuardrailsAIConfig struct {
	// ServerURL is the base URL of the user-hosted guardrails.ai server
	// (e.g. "http://guardrails:8000").
	ServerURL string `json:"server_url"`

	// GuardName is the name of the guard to invoke on the remote server.
	GuardName string `json:"guard_name"`

	// TimeoutSeconds is the HTTP timeout for calls to the guardrails.ai server. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// GuardrailsAIGuard calls a user-hosted guardrails.ai server to validate input text.
type GuardrailsAIGuard struct {
	name   string
	cfg    GuardrailsAIConfig
	client *http.Client
}

// NewGuardrailsAIGuard creates a new GuardrailsAIGuard from configuration.
func NewGuardrailsAIGuard(name string, cfg GuardrailsAIConfig) *GuardrailsAIGuard {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = guardrailsAIDefaultTimeout
	}
	return &GuardrailsAIGuard{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (g *GuardrailsAIGuard) Type() string { return "guardrails_ai" }
func (g *GuardrailsAIGuard) Name() string { return g.name }

// guardrailsAIRequest is the body sent to POST /guards/{guard_name}/validate.
type guardrailsAIRequest struct {
	Input string `json:"input"`
}

// guardrailsAIResponse is the response from the guardrails.ai validation endpoint.
type guardrailsAIResponse struct {
	ValidationPassed bool   `json:"validation_passed"`
	Message          string `json:"message,omitempty"`
	Error            string `json:"error,omitempty"`
}

func (g *GuardrailsAIGuard) Validate(ctx context.Context, input string) (*Result, error) {
	start := time.Now()

	body, err := json.Marshal(guardrailsAIRequest{Input: input})
	if err != nil {
		return nil, fmt.Errorf("guardrails_ai: marshal request: %w", err)
	}

	url := strings.TrimRight(g.cfg.ServerURL, "/") + "/guards/" + g.cfg.GuardName + "/validate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("guardrails_ai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("guardrails_ai: connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guardrails_ai: server returned status %d", resp.StatusCode)
	}

	var grResp guardrailsAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&grResp); err != nil {
		return nil, fmt.Errorf("guardrails_ai: decode response: %w", err)
	}

	duration := time.Since(start)

	if !grResp.ValidationPassed {
		msg := grResp.Message
		if msg == "" {
			msg = "input failed guardrails.ai validation"
		}
		details := map[string]interface{}{"guard_name": g.cfg.GuardName}
		if grResp.Error != "" {
			details["error"] = grResp.Error
		}
		return &Result{
			GuardName: g.name,
			GuardType: g.Type(),
			Passed:    false,
			Message:   msg,
			Details:   details,
			Duration:  duration,
		}, nil
	}

	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    true,
		Message:   "input passed guardrails.ai validation",
		Duration:  duration,
	}, nil
}
