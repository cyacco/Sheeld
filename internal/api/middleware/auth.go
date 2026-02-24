package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/response"
	"github.com/sheeld/sheeld/internal/service"
)

type authContextKey string

const (
	// UserIDKey stores the authenticated user's ID in context.
	UserIDKey authContextKey = "user_id"
	// OrgIDKey stores the authenticated user's organization ID in context.
	OrgIDKey authContextKey = "org_id"
)

// OrgIDFromContext extracts the organization ID from the request context.
func OrgIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(OrgIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(UserIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

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
