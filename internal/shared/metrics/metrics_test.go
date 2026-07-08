package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHTTPMiddlewareRecordsRoutePattern(t *testing.T) {
	r := chi.NewRouter()
	r.Use(HTTP)
	r.Get("/things/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(http.MethodGet, "/things/abc123", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	// Scrape and confirm the request was counted under the route pattern
	// (not the raw path), with the returned status.
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `sheeld_http_requests_total{method="GET",route="/things/{id}",status="418"}`) {
		t.Fatalf("expected counter for route pattern with status 418, got:\n%s", body)
	}
	if strings.Contains(body, "abc123") {
		t.Fatalf("raw path leaked into metrics labels:\n%s", body)
	}
}

func TestConfigAppliedSetsGauges(t *testing.T) {
	ConfigApplied()

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, "sheeld_config_loaded 1") {
		t.Fatalf("expected config_loaded gauge set to 1, got:\n%s", body)
	}
	if !strings.Contains(body, "sheeld_config_last_reload_timestamp_seconds") {
		t.Fatalf("expected last_reload gauge to be present, got:\n%s", body)
	}
}

func TestAuditBufferDepthGaugeReadsFunc(t *testing.T) {
	depth := 7.0
	RegisterAuditBufferDepth(func() float64 { return depth })
	// Re-pointing must not panic on a second registration.
	RegisterAuditBufferDepth(func() float64 { return depth })

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "sheeld_audit_buffer_depth 7") {
		t.Fatalf("expected audit_buffer_depth 7, got:\n%s", rec.Body.String())
	}
}
