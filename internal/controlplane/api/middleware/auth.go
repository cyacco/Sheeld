package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/cyacco/Sheeld/internal/controlplane/service"
	sharedmw "github.com/cyacco/Sheeld/internal/shared/middleware"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// Context keys and FromContext helpers live in shared/middleware so both
// planes can extract identity without importing control-plane packages.
const (
	UserIDKey = sharedmw.UserIDKey
	OrgIDKey  = sharedmw.OrgIDKey
)

// JWTAuth validates JWT tokens from the Authorization header (for dashboard users).
func JWTAuth(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				response.Error(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			claims, err := authSvc.ValidateToken(parts[1])
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, OrgIDKey, claims.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth validates API keys from the Authorization header (for machine-to-machine).
func APIKeyAuth(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				response.Error(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			orgID, err := authSvc.ValidateAPIKey(r.Context(), parts[1])
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), OrgIDKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
