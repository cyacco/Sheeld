package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/middleware"
	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/llm"
	"github.com/sheeld/sheeld/internal/proxy"
)

// ProxyHandler handles the main proxy endpoint.
type ProxyHandler struct {
	proxy *proxy.Proxy
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
		response.Error(w, http.StatusInternalServerError, err.Error())
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
