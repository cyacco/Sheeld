package handler

import (
	"log/slog"
	"net/http"

	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/db/generated"
)

// ModelsHandler handles requests for model lists.
type ModelsHandler struct {
	queries *generated.Queries
}

// NewModelsHandler creates a new ModelsHandler backed by the database.
func NewModelsHandler(queries *generated.Queries) *ModelsHandler {
	return &ModelsHandler{queries: queries}
}

// List returns all models from the database.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	models, err := h.queries.ListModels(r.Context())
	if err != nil {
		slog.Error("listing models", "error", err)
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list models"})
		return
	}

	response.JSON(w, http.StatusOK, models)
}
