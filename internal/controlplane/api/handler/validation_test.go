package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/shared/middleware"
)

// These tests cover the request-validation paths that reject before any
// service/DB call, so they run with a nil service. The happy paths are
// exercised end-to-end in internal/integration.

func doJSON(t *testing.T, h http.HandlerFunc, body string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAuthHandler_RegisterValidation(t *testing.T) {
	h := NewAuthHandler(nil)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed json", `{`, http.StatusBadRequest},
		{"missing org_name", `{"email":"a@b.com","password":"longenough1"}`, http.StatusBadRequest},
		{"missing email", `{"org_name":"Acme","password":"longenough1"}`, http.StatusBadRequest},
		{"short password", `{"org_name":"Acme","email":"a@b.com","password":"short"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJSON(t, h.Register, tt.body, nil)
			if rec.Code != tt.want {
				t.Errorf("got %d, want %d (body: %s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestAuthHandler_LoginValidation(t *testing.T) {
	h := NewAuthHandler(nil)
	for _, tt := range []struct {
		name string
		body string
	}{
		{"malformed json", `not json`},
		{"missing fields", `{"email":""}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJSON(t, h.Login, tt.body, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400", rec.Code)
			}
		})
	}
}

func TestSourceHandler_CreateValidation(t *testing.T) {
	h := NewSourceHandler(nil)
	withOrg := context.WithValue(context.Background(), middleware.OrgIDKey, uuid.New())

	t.Run("no org context is unauthorized", func(t *testing.T) {
		rec := doJSON(t, h.Create, `{"name":"x"}`, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rec.Code)
		}
	})

	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{`},
		{"missing name", `{"route":"r","llm_provider":"openai","llm_model":"m","llm_api_key":"k"}`},
		{"missing route", `{"name":"n","llm_provider":"openai","llm_model":"m","llm_api_key":"k"}`},
		{"missing provider", `{"name":"n","route":"r","llm_model":"m","llm_api_key":"k"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJSON(t, h.Create, tt.body, withOrg)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}
