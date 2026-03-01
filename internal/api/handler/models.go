package handler

import (
	"net/http"

	"github.com/sheeld/sheeld/internal/api/response"
)

// ModelInfo represents a supported LLM model.
type ModelInfo struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

var hardcodedModels = []ModelInfo{
	// Anthropic
	{Provider: "anthropic", ID: "claude-sonnet-4-5-20250514"},
	{Provider: "anthropic", ID: "claude-haiku-4-5-20251001"},
	{Provider: "anthropic", ID: "claude-opus-4-20250514"},
	// OpenAI
	{Provider: "openai", ID: "gpt-4o"},
	{Provider: "openai", ID: "gpt-4o-mini"},
	{Provider: "openai", ID: "gpt-4.1"},
	{Provider: "openai", ID: "gpt-4.1-mini"},
	{Provider: "openai", ID: "gpt-4.1-nano"},
	{Provider: "openai", ID: "o3"},
	{Provider: "openai", ID: "o3-mini"},
	{Provider: "openai", ID: "o4-mini"},
}

// ModelsHandler handles requests for model lists.
type ModelsHandler struct{}

// NewModelsHandler creates a new ModelsHandler.
func NewModelsHandler() *ModelsHandler {
	return &ModelsHandler{}
}

// List returns all supported models, optionally filtered by provider.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")

	if provider == "" {
		response.JSON(w, http.StatusOK, hardcodedModels)
		return
	}

	var filtered []ModelInfo
	for _, m := range hardcodedModels {
		if m.Provider == provider {
			filtered = append(filtered, m)
		}
	}
	response.JSON(w, http.StatusOK, filtered)
}
