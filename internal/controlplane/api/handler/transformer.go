package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/controlplane/service"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// TransformerHandler handles transformer HTTP requests.
type TransformerHandler struct {
	transformerService *service.TransformerService
}

// NewTransformerHandler creates a new TransformerHandler.
func NewTransformerHandler(s *service.TransformerService) *TransformerHandler {
	return &TransformerHandler{transformerService: s}
}

// TransformerResponse is the API-friendly representation of a transformer.
type TransformerResponse struct {
	ID              uuid.UUID              `json:"id"`
	OrganizationID  uuid.UUID              `json:"organization_id"`
	Name            string                 `json:"name"`
	TransformerType string                 `json:"transformer_type"`
	Phase           string                 `json:"phase"`
	Config          map[string]interface{} `json:"config"`
	Enabled         bool                   `json:"enabled"`
	Position        *int32                 `json:"position,omitempty"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
}

func toTransformerResponse(t generated.Transformer) TransformerResponse {
	var config map[string]interface{}
	json.Unmarshal(t.Config, &config)
	return TransformerResponse{
		ID:              t.ID,
		OrganizationID:  t.OrganizationID,
		Name:            t.Name,
		TransformerType: t.TransformerType,
		Phase:           t.Phase,
		Config:          config,
		Enabled:         t.Enabled,
		CreatedAt:       t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *TransformerHandler) orgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return orgID, true
}

// Create handles POST /v1/transformers.
func (h *TransformerHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	var params service.CreateTransformerParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if params.Name == "" {
		response.ValidationError(w, "name", "name is required")
		return
	}
	if params.TransformerType == "" {
		response.ValidationError(w, "transformer_type", "transformer type is required")
		return
	}
	t, err := h.transformerService.Create(r.Context(), orgID, params)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, toTransformerResponse(t))
}

// List handles GET /v1/transformers.
func (h *TransformerHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	transformers, err := h.transformerService.List(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list transformers")
		return
	}
	out := make([]TransformerResponse, len(transformers))
	for i, t := range transformers {
		out[i] = toTransformerResponse(t)
	}
	response.JSON(w, http.StatusOK, out)
}

// Get handles GET /v1/transformers/{id}.
func (h *TransformerHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid transformer ID")
		return
	}
	t, err := h.transformerService.Get(r.Context(), orgID, id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "transformer not found")
		return
	}
	response.JSON(w, http.StatusOK, toTransformerResponse(t))
}

// Update handles PUT /v1/transformers/{id}.
func (h *TransformerHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid transformer ID")
		return
	}
	var params service.UpdateTransformerParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, err := h.transformerService.Update(r.Context(), orgID, id, params)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, toTransformerResponse(t))
}

// Delete handles DELETE /v1/transformers/{id}.
func (h *TransformerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid transformer ID")
		return
	}
	if err := h.transformerService.Delete(r.Context(), orgID, id); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete transformer")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// AttachToSource handles POST /v1/transformers/{id}/sources.
func (h *TransformerHandler) AttachToSource(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid transformer ID")
		return
	}
	var body struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SourceID == uuid.Nil {
		response.Error(w, http.StatusBadRequest, "source_id is required")
		return
	}
	if err := h.transformerService.AttachToSource(r.Context(), orgID, id, body.SourceID); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, map[string]string{"status": "attached"})
}

// DetachFromSource handles DELETE /v1/transformers/{id}/sources/{sourceID}.
func (h *TransformerHandler) DetachFromSource(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.orgID(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid transformer ID")
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "sourceID"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}
	if err := h.transformerService.DetachFromSource(r.Context(), id, sourceID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to detach transformer")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListBySource handles GET /v1/sources/{sourceID}/transformers.
func (h *TransformerHandler) ListBySource(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.orgID(w, r); !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "sourceID"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}
	rows, err := h.transformerService.ListBySource(r.Context(), sourceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list transformers")
		return
	}
	out := make([]TransformerResponse, len(rows))
	for i, row := range rows {
		resp := toTransformerResponse(generated.Transformer{
			ID:              row.ID,
			OrganizationID:  row.OrganizationID,
			Name:            row.Name,
			TransformerType: row.TransformerType,
			Phase:           row.Phase,
			Config:          row.Config,
			Enabled:         row.Enabled,
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
		})
		pos := row.Position
		resp.Position = &pos
		out[i] = resp
	}
	response.JSON(w, http.StatusOK, out)
}

// SetSourceTransformers handles PUT /v1/sources/{sourceID}/transformers —
// replaces the source's transformer chain with the given ordered ids.
func (h *TransformerHandler) SetSourceTransformers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgID(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "sourceID"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}
	var body struct {
		TransformerIDs []uuid.UUID `json:"transformer_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.transformerService.SetSourceTransformers(r.Context(), orgID, sourceID, body.TransformerIDs); err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
