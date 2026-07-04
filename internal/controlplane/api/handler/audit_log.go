package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// AuditLogHandler proxies audit-log queries to the data plane, which owns
// the audit database. The caller's org ID is injected server-side so a
// dashboard user can only see their own logs.
type AuditLogHandler struct {
	dataPlaneURL string
	token        string
	client       *http.Client
}

// NewAuditLogHandler creates a new AuditLogHandler forwarding to the data
// plane at dataPlaneURL using the shared static token.
func NewAuditLogHandler(dataPlaneURL, token string) *AuditLogHandler {
	return &AuditLogHandler{
		dataPlaneURL: dataPlaneURL,
		token:        token,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// List handles GET /v1/audit-logs.
func (h *AuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if h.dataPlaneURL == "" {
		response.Error(w, http.StatusServiceUnavailable, "data plane not configured")
		return
	}

	q := url.Values{}
	q.Set("org_id", orgID.String())
	for _, param := range []string{"source_id", "limit", "offset"} {
		if v := r.URL.Query().Get(param); v != "" {
			q.Set(param, v)
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		h.dataPlaneURL+"/v1/internal/audit-logs?"+q.Encode(), nil)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to build audit query")
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		slog.Error("audit-log query to data plane failed", "error", err)
		response.Error(w, http.StatusBadGateway, "audit log query failed")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
