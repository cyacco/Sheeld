package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/service"
)

// DestinationHandler handles destination-related HTTP requests.
type DestinationHandler struct {
	destinationService *service.DestinationService
}

// NewDestinationHandler creates a new DestinationHandler.
func NewDestinationHandler(destinationService *service.DestinationService) *DestinationHandler {
	return &DestinationHandler{destinationService: destinationService}
}

func parseSourceID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "sourceID"))
}

func parseDestinationID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}

// Create handles POST /v1/sources/:sourceID/destinations.
func (h *DestinationHandler) Create(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	var params service.CreateDestinationParams
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

	dest, err := h.destinationService.Create(r.Context(), sourceID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to create destination")
		return
	}

	response.JSON(w, http.StatusCreated, service.ToDestinationResponse(dest))
}

// List handles GET /v1/sources/:sourceID/destinations.
func (h *DestinationHandler) List(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	dests, err := h.destinationService.List(r.Context(), sourceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list destinations")
		return
	}

	response.JSON(w, http.StatusOK, service.ToDestinationResponses(dests))
}

// Get handles GET /v1/sources/:sourceID/destinations/:id.
func (h *DestinationHandler) Get(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	destID, err := parseDestinationID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid destination ID")
		return
	}

	dest, err := h.destinationService.Get(r.Context(), sourceID, destID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "destination not found")
		return
	}

	response.JSON(w, http.StatusOK, service.ToDestinationResponse(dest))
}

// Update handles PUT /v1/sources/:sourceID/destinations/:id.
func (h *DestinationHandler) Update(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	destID, err := parseDestinationID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid destination ID")
		return
	}

	var params service.UpdateDestinationParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	dest, err := h.destinationService.Update(r.Context(), sourceID, destID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to update destination")
		return
	}

	response.JSON(w, http.StatusOK, service.ToDestinationResponse(dest))
}

// Delete handles DELETE /v1/sources/:sourceID/destinations/:id.
func (h *DestinationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSourceID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid source ID")
		return
	}

	destID, err := parseDestinationID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid destination ID")
		return
	}

	if err := h.destinationService.Delete(r.Context(), sourceID, destID); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete destination")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
