package handler

import (
	"net/http"

	"github.com/sheeld/sheeld/internal/api/response"
)

// ModelInfo represents a curated LLM model available for use.
type ModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

var curatedModels = []ModelInfo{
	// OpenAI
	{ID: "gpt-4o", Provider: "openai"},
	{ID: "gpt-4o-mini", Provider: "openai"},
	{ID: "gpt-4-turbo", Provider: "openai"},
	{ID: "gpt-4", Provider: "openai"},
	{ID: "gpt-3.5-turbo", Provider: "openai"},
	{ID: "o1", Provider: "openai"},
	{ID: "o1-mini", Provider: "openai"},
	{ID: "o3-mini", Provider: "openai"},
	// Anthropic
	{ID: "claude-sonnet-4-20250514", Provider: "anthropic"},
	{ID: "claude-haiku-4-5-20251001", Provider: "anthropic"},
	{ID: "claude-3-5-sonnet-20241022", Provider: "anthropic"},
	{ID: "claude-3-5-haiku-20241022", Provider: "anthropic"},
	{ID: "claude-3-opus-20240229", Provider: "anthropic"},
}

// ModelsHandler handles requests for curated model lists.
type ModelsHandler struct{}

// NewModelsHandler creates a new ModelsHandler.
func NewModelsHandler() *ModelsHandler {
	return &ModelsHandler{}
}

// List returns the curated model list, optionally filtered by provider.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")

	if provider == "" {
		response.JSON(w, http.StatusOK, curatedModels)
		return
	}

	var filtered []ModelInfo
	for _, m := range curatedModels {
		if m.Provider == provider {
			filtered = append(filtered, m)
		}
	}

	response.JSON(w, http.StatusOK, filtered)
}
