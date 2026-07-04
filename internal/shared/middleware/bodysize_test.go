package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodySize(t *testing.T) {
	tests := []struct {
		name       string
		maxBytes   int64
		bodySize   int
		wantStatus int
	}{
		{
			name:       "body within limit",
			maxBytes:   100,
			bodySize:   50,
			wantStatus: http.StatusOK,
		},
		{
			name:       "body at limit",
			maxBytes:   100,
			bodySize:   100,
			wantStatus: http.StatusOK,
		},
		{
			name:       "body exceeds limit",
			maxBytes:   10,
			bodySize:   100,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := MaxBodySize(tt.maxBytes)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))

			body := strings.Repeat("x", tt.bodySize)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
