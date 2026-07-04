package gateway

import (
	"context"
	"net/http"
	"strings"

	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/shared/middleware"
	"github.com/sheeld/sheeld/internal/shared/response"
)

// APIKeyAuth validates API keys against the in-memory config store — no
// database access on the request path. Returns 503 until a config snapshot
// has been loaded.
func APIKeyAuth(store *backendconfig.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !store.Loaded() {
				response.Error(w, http.StatusServiceUnavailable, "config not loaded")
				return
			}

			authHeader := r.Header.Get("Authorization")
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				response.Error(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			orgID, ok := store.LookupOrgByAPIKey(parts[1])
			if !ok {
				response.Error(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), middleware.OrgIDKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
