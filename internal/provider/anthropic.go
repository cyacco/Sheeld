package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type anthropicResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore bool    `json:"has_more"`
	LastID  *string `json:"last_id"`
}

// FetchAnthropicModels fetches models from the Anthropic API, handling pagination.
func FetchAnthropicModels(ctx context.Context, apiKey string) ([]Model, error) {
	var models []Model
	var afterID string

	for {
		url := "https://api.anthropic.com/v1/models?limit=100"
		if afterID != "" {
			url += "&after_id=" + afterID
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching models: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Anthropic API returned status %d", resp.StatusCode)
		}

		var result anthropicResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		for _, m := range result.Data {
			models = append(models, Model{ID: m.ID, Provider: "anthropic"})
		}

		if !result.HasMore || result.LastID == nil {
			break
		}
		afterID = *result.LastID
	}

	return models, nil
}
