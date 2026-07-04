package middleware

import (
	"net/http"

	"github.com/sheeld/sheeld/internal/shared/response"
)

// MaxBodySize returns middleware that limits the request body size.
// If the body exceeds maxBytes, the server returns 413 Request Entity Too Large.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// HandleMaxBytesError checks if an error is from exceeding MaxBytesReader and writes 413.
// Returns true if the error was handled.
func HandleMaxBytesError(w http.ResponseWriter, err error) bool {
	if err != nil && err.Error() == "http: request body too large" {
		response.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
		return true
	}
	return false
}
