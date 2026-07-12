package handler

import (
	"net/http"

	"github.com/cyacco/Sheeld/internal/shared/modelcatalog"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// ModelInfo represents a supported LLM model for the dashboard dropdown.
type ModelInfo struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// ModelsHandler handles requests for model lists.
type ModelsHandler struct{}

// NewModelsHandler creates a new ModelsHandler.
func NewModelsHandler() *ModelsHandler {
	return &ModelsHandler{}
}

// List returns the supported models from the shared catalog, optionally
// filtered by provider. The catalog is also the source of cost prices, so the
// dropdown and cost estimation never drift apart.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	models := modelcatalog.Models(r.URL.Query().Get("provider"))
	out := make([]ModelInfo, len(models))
	for i, m := range models {
		out[i] = ModelInfo{Provider: m.Provider, ID: m.ID}
	}
	response.JSON(w, http.StatusOK, out)
}
