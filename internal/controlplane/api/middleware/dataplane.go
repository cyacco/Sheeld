package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/sheeld/sheeld/internal/shared/response"
)

// DataPlaneAuth authenticates data-plane requests using a static shared
// token, compared in constant time. If no token is configured the endpoint
// is disabled entirely.
func DataPlaneAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				response.Error(w, http.StatusServiceUnavailable, "data plane endpoint not configured")
				return
			}

			authHeader := r.Header.Get("Authorization")
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				response.Error(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) != 1 {
				response.Error(w, http.StatusUnauthorized, "invalid data plane token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
