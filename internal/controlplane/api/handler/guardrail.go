package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/controlplane/api/response"
	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/controlplane/service"
)

// GuardrailHandler handles guardrail-related HTTP requests.
type GuardrailHandler struct {
	guardrailService *service.GuardrailService
}

// NewGuardrailHandler creates a new GuardrailHandler.
func NewGuardrailHandler(guardrailService *service.GuardrailService) *GuardrailHandler {
	return &GuardrailHandler{guardrailService: guardrailService}
}

func parseGuardrailID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}

func parseSourceIDParam(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "sourceID"))
}

// Create handles POST /v1/guardrails.
func (h *GuardrailHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

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

	g, err := h.guardrailService.Create(r.Context(), orgID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to create guardrail")
		return
	}

	response.JSON(w, http.StatusCreated, service.ToGuardrailResponse(g))
}

// List handles GET /v1/guardrails.
func (h *GuardrailHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

	guardrails, err := h.guardrailService.List(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list guardrails")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponses(guardrails))
}

// Get handles GET /v1/guardrails/:id.
func (h *GuardrailHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	g, err := h.guardrailService.Get(r.Context(), orgID, guardrailID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "guardrail not found")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponse(g))
}

// Update handles PUT /v1/guardrails/:id.
func (h *GuardrailHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

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

	g, err := h.guardrailService.Update(r.Context(), orgID, guardrailID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to update guardrail")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponse(g))
}

// Delete handles DELETE /v1/guardrails/:id.
func (h *GuardrailHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	if err := h.guardrailService.Delete(r.Context(), orgID, guardrailID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete guardrail")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AttachToSource handles POST /v1/guardrails/:id/sources.
func (h *GuardrailHandler) AttachToSource(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())

	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	var body struct {
		SourceID string `json:"source_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sourceID, err := uuid.Parse(body.SourceID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source_id")
		return
	}

	if err := h.guardrailService.AttachToSource(r.Context(), orgID, guardrailID, sourceID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to attach guardrail to source")
		return
	}

	response.JSON(w, http.StatusCreated, map[string]string{"status": "attached"})
}

// DetachFromSource handles DELETE /v1/guardrails/:id/sources/:sourceID.
func (h *GuardrailHandler) DetachFromSource(w http.ResponseWriter, r *http.Request) {
	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	sourceID, err := parseSourceIDParam(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	if err := h.guardrailService.DetachFromSource(r.Context(), guardrailID, sourceID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to detach guardrail from source")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListSources handles GET /v1/guardrails/:id/sources.
func (h *GuardrailHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	guardrailID, err := parseGuardrailID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid guardrail ID")
		return
	}

	sources, err := h.guardrailService.ListSources(r.Context(), guardrailID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list sources")
		return
	}

	response.JSON(w, http.StatusOK, toSourceSummaries(sources))
}

// ListBySource handles GET /v1/sources/:sourceID/guardrails.
func (h *GuardrailHandler) ListBySource(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceIDParam(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	guardrails, err := h.guardrailService.ListBySource(r.Context(), sourceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list guardrails")
		return
	}

	response.JSON(w, http.StatusOK, service.ToGuardrailResponses(guardrails))
}

// toSourceSummaries converts database sources to a minimal API response (no sensitive fields).
func toSourceSummaries(sources []generated.Source) []map[string]interface{} {
	result := make([]map[string]interface{}, len(sources))
	for i, s := range sources {
		result[i] = map[string]interface{}{
			"id":    s.ID,
			"name":  s.Name,
			"route": s.Route,
		}
	}
	return result
}
