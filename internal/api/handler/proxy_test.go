package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/api/middleware"
	"github.com/sheeld/sheeld/internal/llm"
	"github.com/sheeld/sheeld/internal/proxy"
)

// fakeProxyExecutor is a stub proxyExecutor for tests.
type fakeProxyExecutor struct {
	err error
}

func (f *fakeProxyExecutor) Execute(_ context.Context, _ uuid.UUID, _ string, _ *llm.ChatRequest) (*proxy.ProxyResult, error) {
	return nil, f.err
}

// newTestRequest builds a POST /v1/proxy/:sourceRoute request with auth context
// and a chi route param set, ready to hand to ProxyHandler.Handle.
func newTestRequest(t *testing.T, sourceRoute string, body any) *http.Request {
	t.Helper()

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		t.Fatalf("encode body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/"+sourceRoute, buf)

	// Inject the chi URL param the handler reads via chi.URLParam.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sourceRoute", sourceRoute)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	// Inject auth + request id context the handler expects.
	ctx = context.WithValue(ctx, middleware.OrgIDKey, uuid.New())
	ctx = context.WithValue(ctx, middleware.RequestIDKey, "test-req-id")

	return req.WithContext(ctx)
}

// TestHandle_InternalErrorDoesNotLeakDetails verifies that when the underlying
// proxy returns a wrapped internal error (e.g. "source not found: sql: no rows
// in result set" or "decrypting API key: crypto/cipher: ciphertext too short"),
// the handler responds with a generic "internal server error" message and never
// surfaces internal tokens like "sql", "crypto", or "decrypting" to the client.
func TestHandle_InternalErrorDoesNotLeakDetails(t *testing.T) {
	leakyErrors := []error{
		fmt.Errorf("source not found: %w", errors.New("sql: no rows in result set")),
		fmt.Errorf("decrypting API key: %w", errors.New("crypto/cipher: ciphertext too short")),
		fmt.Errorf("loading guardrails: %w", errors.New("sql: connection refused")),
	}

	for _, leaky := range leakyErrors {
		t.Run(leaky.Error(), func(t *testing.T) {
			h := &ProxyHandler{proxy: &fakeProxyExecutor{err: leaky}}

			req := newTestRequest(t, "feedback", map[string]any{
				"messages": []map[string]string{
					{"role": "user", "content": "hello"},
				},
			})
			rec := httptest.NewRecorder()

			h.Handle(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d want %d", rec.Code, http.StatusInternalServerError)
			}

			body := rec.Body.String()
			lower := strings.ToLower(body)

			for _, forbidden := range []string{"sql", "crypto", "decrypting", "no rows", "ciphertext"} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("response body leaks internal token %q: %s", forbidden, body)
				}
			}

			if !strings.Contains(lower, "internal server error") {
				t.Errorf("response body should contain a generic error message, got: %s", body)
			}

			// Sanity: the body should be a JSON object with an "error" field.
			var parsed map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if parsed["error"] == "" {
				t.Errorf("response missing error field: %s", body)
			}
		})
	}
}

// TestHandle_ValidationErrorsUnchanged makes sure the sanitization of 500s
// did not accidentally swallow user-facing 4xx validation messages.
func TestHandle_ValidationErrorsUnchanged(t *testing.T) {
	h := &ProxyHandler{proxy: &fakeProxyExecutor{err: errors.New("should not be called")}}

	req := newTestRequest(t, "feedback", map[string]any{
		"messages": []map[string]string{}, // empty -> 400
	})
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "messages array is required") {
		t.Errorf("expected validation message, got: %s", rec.Body.String())
	}
}
