package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/service"
)

// GuardrailHandler handles guardrail-related HTTP requests.
type GuardrailHandler struct {
	guardrailService *service.GuardrailService
}

// NewGuardrailHandler creates a new GuardrailHandler.
func NewGuardrailHandler(guardrailService *service.GuardrailService) *GuardrailHandler {
	return &GuardrailHandler{guardrailService: guardrailService}
}

func parseSourceID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "sourceID"))
}

func parseGuardrailID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}

// Create handles POST /v1/sources/:sourceID/guardrails.
func (h *GuardrailHandler) Create(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	var params service.CreateGuardrailParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if params.Name == "" {
		response.ValidationError(w, "name", "name is required")
		return
	}
	if params.GuardType == "" {
		response.ValidationError(w, "guard_type", "guard type is required")
		return
	}

	// Default phase to "input" if not specified
	if params.Phase == "" {
		params.Phase = "input"
	}

	g, err := h.guardrailService.Create(r.Context(), sourceID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to create guardrail")
		return
	}

	response.JSON(w, http.StatusCreated, service.ToGuardrailResponse(g))
}

// List handles GET /v1/sources/:sourceID/guardrails.
func (h *GuardrailHandler) List(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	guardrails, err := h.guardrailService.List(r.Context(), sourceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list guardrails")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponses(guardrails))
}

// Get handles GET /v1/sources/:sourceID/guardrails/:id.
func (h *GuardrailHandler) Get(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	g, err := h.guardrailService.Get(r.Context(), sourceID, guardrailID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "guardrail not found")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponse(g))
}

// Update handles PUT /v1/sources/:sourceID/guardrails/:id.
func (h *GuardrailHandler) Update(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	var params service.UpdateGuardrailParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	g, err := h.guardrailService.Update(r.Context(), sourceID, guardrailID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to update guardrail")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponse(g))
}

// Delete handles DELETE /v1/sources/:sourceID/guardrails/:id.
func (h *GuardrailHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	if err := h.guardrailService.Delete(r.Context(), sourceID, guardrailID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete guardrail")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
