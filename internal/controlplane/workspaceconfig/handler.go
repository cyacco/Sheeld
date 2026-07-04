package workspaceconfig

import (
	"log/slog"
	"net/http"

	"github.com/sheeld/sheeld/internal/controlplane/api/response"
)

// Handler serves the workspace-config payload to data planes.
type Handler struct {
	builder *Builder
}

// NewHandler creates a workspace-config handler.
func NewHandler(builder *Builder) *Handler {
	return &Handler{builder: builder}
}

// Get builds and returns the current workspace config. It sets the config
// version as the ETag and honors If-None-Match with a 304. The payload
// contains plaintext LLM keys and must never be logged.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.builder.Build(r.Context())
	if err != nil {
		slog.Error("building workspace config", "error", err)
		response.Error(w, http.StatusInternalServerError, "failed to build workspace config")
		return
	}

	etag := `"` + cfg.Version + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	response.JSON(w, http.StatusOK, cfg)
}
