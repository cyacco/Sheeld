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

const openAIDefaultBaseURL = "https://api.openai.com"
const openAIDefaultTimeout = 10

// OpenAIModerationConfig holds configuration for the OpenAI Moderation guard.
type OpenAIModerationConfig struct {
	// APIKey is the OpenAI API key used to authenticate moderation requests.
	APIKey string `json:"api_key"`

	// Categories is the list of moderation categories to check (e.g. "hate", "violence").
	// If empty, all categories returned by the API are evaluated.
	Categories []string `json:"categories"`

	// Threshold is the score above which a category is considered to have failed (0.0–1.0).
	// Defaults to 0.5 if not set.
	Threshold float64 `json:"threshold"`

	// TimeoutSeconds is the HTTP timeout for calls to the moderation API. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`

	// BaseURL overrides the OpenAI API base URL. Used in tests; leave empty for production.
	BaseURL string `json:"base_url,omitempty"`
}

// OpenAIModerationGuard calls the OpenAI Moderation API to evaluate input text.
type OpenAIModerationGuard struct {
	name   string
	cfg    OpenAIModerationConfig
	client *http.Client
}

// NewOpenAIModerationGuard creates a new OpenAIModerationGuard from configuration.
func NewOpenAIModerationGuard(name string, cfg OpenAIModerationConfig) *OpenAIModerationGuard {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = openAIDefaultTimeout
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.5
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = openAIDefaultBaseURL
	}
	return &OpenAIModerationGuard{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (g *OpenAIModerationGuard) Type() string { return "openai_moderation" }
func (g *OpenAIModerationGuard) Name() string { return g.name }

// openAIModRequest is the request body sent to the OpenAI moderation endpoint.
type openAIModRequest struct {
	Input string `json:"input"`
}

// openAIModResponse is the response from the OpenAI moderation endpoint.
type openAIModResponse struct {
	Results []struct {
		Flagged        bool               `json:"flagged"`
		Categories     map[string]bool    `json:"categories"`
		CategoryScores map[string]float64 `json:"category_scores"`
	} `json:"results"`
}

func (g *OpenAIModerationGuard) Validate(ctx context.Context, input string) (*Result, error) {
	start := time.Now()

	body, err := json.Marshal(openAIModRequest{Input: input})
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: marshal request: %w", err)
	}

	url := strings.TrimRight(g.cfg.BaseURL, "/") + "/v1/moderations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai_moderation: API returned status %d", resp.StatusCode)
	}

	var modResp openAIModResponse
	if err := json.NewDecoder(resp.Body).Decode(&modResp); err != nil {
		return nil, fmt.Errorf("openai_moderation: decode response: %w", err)
	}

	if len(modResp.Results) == 0 {
		return nil, fmt.Errorf("openai_moderation: empty results from API")
	}

	result := modResp.Results[0]
	scores := result.CategoryScores

	// Build the set of categories to evaluate.
	categoriesToCheck := g.cfg.Categories
	if len(categoriesToCheck) == 0 {
		for cat := range scores {
			categoriesToCheck = append(categoriesToCheck, cat)
		}
	}

	type exceeded struct {
		Category string  `json:"category"`
		Score    float64 `json:"score"`
	}
	var exceedances []exceeded
	for _, cat := range categoriesToCheck {
		score, ok := scores[cat]
		if !ok {
			continue
		}
		if score >= g.cfg.Threshold {
			exceedances = append(exceedances, exceeded{Category: cat, Score: score})
		}
	}

	duration := time.Since(start)

	if len(exceedances) > 0 {
		return &Result{
			GuardName: g.name,
			GuardType: g.Type(),
			Passed:    false,
			Message:   "input exceeded moderation threshold",
			Details:   map[string]interface{}{"exceeded_categories": exceedances, "threshold": g.cfg.Threshold},
			Duration:  duration,
		}, nil
	}

	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    true,
		Message:   "input passed moderation",
		Duration:  duration,
	}, nil
}
