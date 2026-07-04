package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
)

// AuditLogHandler handles audit log HTTP requests.
type AuditLogHandler struct {
	queries *generated.Queries
}

// NewAuditLogHandler creates a new AuditLogHandler.
func NewAuditLogHandler(queries *generated.Queries) *AuditLogHandler {
	return &AuditLogHandler{queries: queries}
}

// List handles GET /v1/audit-logs.
func (h *AuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := int32(50)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n)
		}
	}

	offset := int32(0)
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	sourceIDStr := r.URL.Query().Get("source_id")

	if sourceIDStr != "" {
		sourceID, err := uuid.Parse(sourceIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid source_id")
			return
		}
		logs, err := h.queries.ListAuditLogsBySource(r.Context(), generated.ListAuditLogsBySourceParams{
			SourceID:       sourceID,
			OrganizationID: orgID,
			Limit:          limit,
			Offset:         offset,
		})
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "failed to list audit logs")
			return
		}
		response.JSON(w, http.StatusOK, logs)
		return
	}

	logs, err := h.queries.ListAuditLogsByOrganization(r.Context(), generated.ListAuditLogsByOrganizationParams{
		OrganizationID: orgID,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}

	response.JSON(w, http.StatusOK, logs)
}
