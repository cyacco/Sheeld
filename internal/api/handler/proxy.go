package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/middleware"
	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/llm"
	"github.com/sheeld/sheeld/internal/proxy"
)

// proxyExecutor is the minimal interface the handler needs from a proxy.
// Defined here so the handler can be tested with a fake.
type proxyExecutor interface {
	Execute(ctx context.Context, orgID uuid.UUID, sourceRoute string, chatReq *llm.ChatRequest) (*proxy.ProxyResult, error)
}

// ProxyHandler handles the main proxy endpoint.
type ProxyHandler struct {
	proxy proxyExecutor
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(p *proxy.Proxy) *ProxyHandler {
	return &ProxyHandler{proxy: p}
}

// Handle processes POST /v1/proxy/:sourceRoute.
func (h *ProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sourceRoute := chi.URLParam(r, "sourceRoute")
	if sourceRoute == "" {
		response.Error(w, http.StatusBadRequest, "missing source route")
		return
	}

	var chatReq llm.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body: expected OpenAI chat completions format")
		return
	}

	if len(chatReq.Messages) == 0 {
		response.Error(w, http.StatusBadRequest, "messages array is required")
		return
	}

	result, err := h.proxy.Execute(r.Context(), orgID, sourceRoute, &chatReq)
	if err != nil {
		// Log full error server-side with request context, but never leak
		// internal error details (e.g. "sql: no rows", "crypto/cipher: ...")
		// to clients — that would be an information disclosure and
		// enumeration oracle. Return a generic message instead.
		reqID, _ := r.Context().Value(middleware.RequestIDKey).(string)
		slog.ErrorContext(r.Context(), "proxy execute failed",
			"request_id", reqID,
			"source", sourceRoute,
			"error", err,
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Set status header for easy programmatic checks
	w.Header().Set("X-Sheeld-Status", result.Status)

	status := http.StatusOK
	if result.Status == "rejected" {
		status = http.StatusForbidden
	}

	response.JSON(w, status, result)
}
