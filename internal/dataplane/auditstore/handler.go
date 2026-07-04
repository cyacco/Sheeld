package auditstore

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/dataplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// Handler serves audit-log queries to the control plane.
type Handler struct {
	queries *generated.Queries
}

// NewHandler creates an audit-log query handler.
func NewHandler(queries *generated.Queries) *Handler {
	return &Handler{queries: queries}
}

// List handles GET /v1/internal/audit-logs?org_id=&source_id=&limit=&offset=.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(r.URL.Query().Get("org_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid or missing org_id")
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

	if sourceIDStr := r.URL.Query().Get("source_id"); sourceIDStr != "" {
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
