package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// ProxyHandler handles the main proxy endpoint.
type ProxyHandler struct {
	processor *processor.Processor
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(p *processor.Processor) *ProxyHandler {
	return &ProxyHandler{processor: p}
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

	result, err := h.processor.Execute(r.Context(), orgID, sourceRoute, &chatReq)
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
