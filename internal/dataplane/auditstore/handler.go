package auditstore

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// handlerQueries is the subset of generated queries the read handler needs,
// so tests can inject a fake. *generated.Queries satisfies it.
type handlerQueries interface {
	ListAuditLogs(ctx context.Context, arg generated.ListAuditLogsParams) ([]generated.AuditLog, error)
	AuditSummary(ctx context.Context, arg generated.AuditSummaryParams) (generated.AuditSummaryRow, error)
	AuditDailySeries(ctx context.Context, arg generated.AuditDailySeriesParams) ([]generated.AuditDailySeriesRow, error)
	AuditByModel(ctx context.Context, arg generated.AuditByModelParams) ([]generated.AuditByModelRow, error)
	AuditBySource(ctx context.Context, arg generated.AuditBySourceParams) ([]generated.AuditBySourceRow, error)
}

// Handler serves audit-log queries to the control plane.
type Handler struct {
	queries handlerQueries
}

// NewHandler creates an audit-log query handler.
func NewHandler(queries handlerQueries) *Handler {
	return &Handler{queries: queries}
}

// List handles GET /v1/internal/audit-logs with optional filters
// (source_id, status, from, to) and keyset pagination (before, before_id).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	orgID, err := uuid.Parse(q.Get("org_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid or missing org_id")
		return
	}

	params := generated.ListAuditLogsParams{
		OrganizationID: orgID,
		LimitCount:     50,
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			params.LimitCount = int32(n)
		}
	}

	if v := q.Get("source_id"); v != "" {
		sourceID, err := uuid.Parse(v)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid source_id")
			return
		}
		params.SourceID = pgtype.UUID{Bytes: sourceID, Valid: true}
	}

	if v := q.Get("status"); v != "" {
		if v != "pass" && v != "fail" {
			response.Error(w, http.StatusBadRequest, "status must be \"pass\" or \"fail\"")
			return
		}
		params.Status = pgtype.Text{String: v, Valid: true}
	}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid from (want RFC3339)")
			return
		}
		params.FromTime = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid to (want RFC3339)")
			return
		}
		params.ToTime = pgtype.Timestamptz{Time: t, Valid: true}
	}

	// Keyset cursor: both parts are required together to page to older rows.
	before, beforeID := q.Get("before"), q.Get("before_id")
	if before != "" || beforeID != "" {
		if before == "" || beforeID == "" {
			response.Error(w, http.StatusBadRequest, "before and before_id must be provided together")
			return
		}
		t, err := time.Parse(time.RFC3339Nano, before)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid before cursor (want RFC3339)")
			return
		}
		id, err := uuid.Parse(beforeID)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid before_id cursor")
			return
		}
		params.BeforeTime = pgtype.Timestamptz{Time: t, Valid: true}
		params.BeforeID = pgtype.UUID{Bytes: id, Valid: true}
	}

	logs, err := h.queries.ListAuditLogs(r.Context(), params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	response.JSON(w, http.StatusOK, logs)
}
