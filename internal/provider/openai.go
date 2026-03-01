package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Model represents an LLM model from a provider.
type Model struct {
	ID       string
	Provider string
}

var openAIChatPrefixes = []string{"gpt-4", "gpt-3.5", "o1", "o3", "o4", "chatgpt"}

type openAIResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// FetchOpenAIModels fetches chat models from the OpenAI API.
func FetchOpenAIModels(ctx context.Context, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var models []Model
	for _, m := range result.Data {
		if isChatModel(m.ID) {
			models = append(models, Model{ID: m.ID, Provider: "openai"})
		}
	}

	return models, nil
}

func isChatModel(id string) bool {
	for _, prefix := range openAIChatPrefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}
