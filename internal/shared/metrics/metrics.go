// Package metrics defines the Prometheus collectors used across both planes
// and an HTTP middleware/handler for scraping. Collectors are registered on
// the default registry (alongside the standard Go/process collectors), so a
// process only needs to import this package and mount Handler() at /metrics.
//
// The control plane touches only the HTTP collectors; the data plane also
// records the proxy/guard/LLM/audit/config collectors. Unused collectors sit
// at zero, which is harmless.
package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTPRequests counts handled HTTP requests, labelled by method, the chi
	// route pattern (never the raw path, to keep cardinality bounded), and
	// response status code.
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sheeld_http_requests_total",
		Help: "Total HTTP requests handled, by method, route pattern, and status.",
	}, []string{"method", "route", "status"})

	// HTTPDuration observes request handling latency by method and route.
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sheeld_http_request_duration_seconds",
		Help:    "HTTP request handling latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	// ProxyRequests counts proxy outcomes: status is "pass", "rejected", or
	// "error"; phase is the rejection phase ("input"/"output") or "" otherwise.
	ProxyRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sheeld_proxy_requests_total",
		Help: "Total proxy requests, by status (pass/rejected/error) and rejection phase.",
	}, []string{"status", "phase"})

	// ProxyDuration observes total proxy pipeline latency.
	ProxyDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "sheeld_proxy_request_duration_seconds",
		Help:    "End-to-end proxy pipeline latency in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// GuardDuration observes guard-batch evaluation latency by phase.
	GuardDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sheeld_guard_duration_seconds",
		Help:    "Guard evaluation latency in seconds, by phase (input/output).",
		Buckets: prometheus.DefBuckets,
	}, []string{"phase"})

	// LLMRequests counts LLM gateway calls by outcome ("success"/"error").
	LLMRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sheeld_llm_requests_total",
		Help: "Total LLM gateway requests, by outcome (success/error).",
	}, []string{"outcome"})

	// LLMRetries counts retry attempts made against the LLM gateway.
	LLMRetries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sheeld_llm_retries_total",
		Help: "Total LLM gateway retry attempts after the first try.",
	})

	// AuditDropped counts audit entries dropped because the buffer was full.
	AuditDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sheeld_audit_entries_dropped_total",
		Help: "Audit entries dropped because the async buffer was full.",
	})

	// AuditBatchesDropped counts audit batches dropped after insert retries.
	AuditBatchesDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sheeld_audit_batches_dropped_total",
		Help: "Audit batches dropped after exhausting insert retries.",
	})

	// ConfigLoaded is 1 once a workspace config has been applied, else 0.
	ConfigLoaded = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sheeld_config_loaded",
		Help: "1 if a workspace config snapshot has been applied, else 0.",
	})

	// ConfigLastReload is the Unix timestamp of the last successful config
	// apply; scrape (time() - this) for staleness alerting.
	ConfigLastReload = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sheeld_config_last_reload_timestamp_seconds",
		Help: "Unix timestamp of the last successful workspace-config apply.",
	})
)

var (
	auditDepthOnce sync.Once
	auditDepthFn   atomic.Pointer[func() float64]
)

// RegisterAuditBufferDepth points the audit-buffer-depth gauge at depth(),
// read at scrape time (e.g. the audit writer's buffered channel length). Safe
// to call repeatedly — the gauge is registered once and re-pointed thereafter,
// so creating multiple writers (e.g. in tests) never double-registers.
func RegisterAuditBufferDepth(depth func() float64) {
	auditDepthFn.Store(&depth)
	auditDepthOnce.Do(func() {
		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "sheeld_audit_buffer_depth",
			Help: "Current number of audit entries queued in the async buffer.",
		}, func() float64 {
			if fn := auditDepthFn.Load(); fn != nil {
				return (*fn)()
			}
			return 0
		})
	})
}

// ConfigApplied records a successful workspace-config apply.
func ConfigApplied() {
	ConfigLoaded.Set(1)
	ConfigLastReload.Set(float64(time.Now().Unix()))
}

// Handler returns the Prometheus scrape handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// HTTP is a chi-aware middleware that records request counts and latency.
// It uses the matched route pattern as the route label to keep cardinality
// bounded regardless of path parameters.
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder captures the response status code for the metrics label.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}
