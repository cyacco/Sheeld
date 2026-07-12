package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/controlplane/service"
	"github.com/cyacco/Sheeld/internal/shared/middleware"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// AlertHandler exposes CRUD for org-level rejection-alert webhooks.
type AlertHandler struct {
	alertService *service.AlertService
}

// NewAlertHandler creates a new AlertHandler.
func NewAlertHandler(alertService *service.AlertService) *AlertHandler {
	return &AlertHandler{alertService: alertService}
}

// Create handles POST /v1/alerts.
func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var params service.AlertWebhookParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wh, err := h.alertService.Create(r.Context(), orgID, params)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, wh)
}

// List handles GET /v1/alerts.
func (h *AlertHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	whs, err := h.alertService.List(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list alert webhooks")
		return
	}
	response.JSON(w, http.StatusOK, whs)
}

// Update handles PUT /v1/alerts/{id}.
func (h *AlertHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var params service.AlertWebhookParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wh, err := h.alertService.Update(r.Context(), orgID, id, params)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, wh)
}

// Delete handles DELETE /v1/alerts/{id}.
func (h *AlertHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.alertService.Delete(r.Context(), orgID, id); err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to delete alert webhook")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
