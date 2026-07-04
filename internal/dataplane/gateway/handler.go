package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// openAIError is the OpenAI-compatible error envelope, so SDK clients get
// structured errors they already know how to parse.
type openAIError struct {
	Error openAIErrorBody `json:"error"`
}

type openAIErrorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    string  `json:"code,omitempty"`
	Param   *string `json:"param,omitempty"`
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, code, message string) {
	response.JSON(w, status, openAIError{Error: openAIErrorBody{
		Message: message,
		Type:    errType,
		Code:    code,
	}})
}

// ProxyHandler handles the main proxy endpoint. On pass it returns the raw
// OpenAI-format chat completion, so pointing an OpenAI SDK's base_url at
// /v1/proxy/{route} is a drop-in change. On guardrail rejection it returns
// an OpenAI-style error with a minimal reason; full guard results are only
// recorded in audit logs (correlate via X-Request-ID).
type ProxyHandler struct {
	processor *processor.Processor
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(p *processor.Processor) *ProxyHandler {
	return &ProxyHandler{processor: p}
}

// Handle processes POST /v1/proxy/{sourceRoute} and
// POST /v1/proxy/{sourceRoute}/chat/completions.
func (h *ProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.OrgIDFromContext(r.Context())
	if orgID == uuid.Nil {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_request_error", "unauthorized", "unauthorized")
		return
	}

	sourceRoute := chi.URLParam(r, "sourceRoute")
	if sourceRoute == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing_source_route", "missing source route")
		return
	}

	var chatReq llm.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_body", "invalid request body: expected OpenAI chat completions format")
		return
	}

	if len(chatReq.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing_messages", "messages array is required")
		return
	}

	result, err := h.processor.Execute(r.Context(), orgID, sourceRoute, &chatReq)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "proxy_error", err.Error())
		return
	}

	// Set status header for easy programmatic checks
	w.Header().Set("X-Sheeld-Status", result.Status)

	if result.Status == "rejected" {
		writeOpenAIError(w, http.StatusUnprocessableEntity, "guardrail_rejection",
			result.Phase+"_rejected",
			fmt.Sprintf("request rejected by %s guardrails; see audit logs for details", result.Phase))
		return
	}

	response.JSON(w, http.StatusOK, result.LLMResponse)
}
