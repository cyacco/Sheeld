package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/controlplane/api/response"
	"github.com/sheeld/sheeld/internal/controlplane/service"
)

// SourceHandler handles source-related HTTP requests.
type SourceHandler struct {
	sourceService *service.SourceService
}

// NewSourceHandler creates a new SourceHandler.
func NewSourceHandler(sourceService *service.SourceService) *SourceHandler {
	return &SourceHandler{sourceService: sourceService}
}

// Create handles POST /v1/sources.
func (h *SourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var params service.CreateSourceParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if params.Name == "" {
		response.ValidationError(w, "name", "name is required")
		return
	}
	if params.Route == "" {
		response.ValidationError(w, "route", "route is required")
		return
	}
	if params.LLMProvider == "" {
		response.ValidationError(w, "llm_provider", "LLM provider is required")
		return
	}
	if params.LLMModel == "" {
		response.ValidationError(w, "llm_model", "LLM model is required")
		return
	}
	if params.LLMAPIKey == "" {
		response.ValidationError(w, "llm_api_key", "LLM API key is required")
		return
	}

	src, err := h.sourceService.Create(r.Context(), orgID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to create source")
		return
	}

	response.JSON(w, http.StatusCreated, service.ToSourceResponse(src))
}

// List handles GET /v1/sources.
func (h *SourceHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sources, err := h.sourceService.List(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list sources")
		return
	}

	response.JSON(w, http.StatusOK, service.ToSourceResponses(sources))
}

// Get handles GET /v1/sources/:id.
func (h *SourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	src, err := h.sourceService.Get(r.Context(), orgID, sourceID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "source not found")
		return
	}

	response.JSON(w, http.StatusOK, service.ToSourceResponse(src))
}

// Update handles PUT /v1/sources/:id.
func (h *SourceHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	var params service.UpdateSourceParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	src, err := h.sourceService.Update(r.Context(), orgID, sourceID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to update source")
		return
	}

	response.JSON(w, http.StatusOK, service.ToSourceResponse(src))
}

// Delete handles DELETE /v1/sources/:id.
func (h *SourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	if err := h.sourceService.Delete(r.Context(), orgID, sourceID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete source")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
