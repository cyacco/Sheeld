package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// Connection is one source↔guardrail attachment.
type Connection struct {
	SourceID    uuid.UUID `json:"source_id"`
	GuardrailID uuid.UUID `json:"guardrail_id"`
}

// ConnectionsHandler serves the org's full attachment list for the
// dashboard's connections view.
type ConnectionsHandler struct {
	queries *generated.Queries
}

// NewConnectionsHandler creates a ConnectionsHandler.
func NewConnectionsHandler(queries *generated.Queries) *ConnectionsHandler {
	return &ConnectionsHandler{queries: queries}
}

// List handles GET /v1/connections.
func (h *ConnectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.queries.ListSourceGuardrailsByOrg(r.Context(), orgID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list connections")
		return
	}

	connections := make([]Connection, len(rows))
	for i, row := range rows {
		connections[i] = Connection{SourceID: row.SourceID, GuardrailID: row.GuardrailID}
	}
	response.JSON(w, http.StatusOK, connections)
}
