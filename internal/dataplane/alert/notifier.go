// Package alert delivers rejection alerts to org-configured webhooks. All
// delivery is asynchronous and rate-capped per webhook — a rejection storm
// must never flood the destination or slow the proxy path.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/cyacco/Sheeld/internal/dataplane/backendconfig"
	"github.com/cyacco/Sheeld/internal/shared/metrics"
)

const (
	deliveryTimeout = 5 * time.Second
	// Per-webhook cap: sustained 1 alert / 10s with a burst of 3. Beyond
	// that, alerts are suppressed (counted, not queued) — the audit log
	// remains the complete record.
	alertsPerSecond = 0.1
	alertBurst      = 3
)

// Event describes one guard rejection for alerting.
type Event struct {
	OrganizationID uuid.UUID     `json:"organization_id"`
	SourceRoute    string        `json:"source_route"`
	Phase          string        `json:"phase"` // "input" | "output"
	RequestID      string        `json:"request_id,omitempty"`
	FailedGuards   []FailedGuard `json:"failed_guards"`
	Timestamp      time.Time     `json:"timestamp"`
}

// FailedGuard is one failing guard within a rejection event.
type FailedGuard struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Notifier resolves an org's alert webhooks from the config store and posts
// rejection events to them.
type Notifier struct {
	store  *backendconfig.Store
	client *http.Client

	mu       sync.Mutex
	limiters map[uuid.UUID]*rate.Limiter // webhook ID → limiter
}

// NewNotifier creates a Notifier reading webhook config from store.
func NewNotifier(store *backendconfig.Store) *Notifier {
	return &Notifier{
		store:    store,
		client:   &http.Client{Timeout: deliveryTimeout},
		limiters: make(map[uuid.UUID]*rate.Limiter),
	}
}

// Notify posts the event to each of the org's enabled alert webhooks,
// asynchronously. It never blocks the caller.
func (n *Notifier) Notify(event Event) {
	webhooks := n.store.AlertWebhooks(event.OrganizationID)
	if len(webhooks) == 0 {
		return
	}
	event.Timestamp = event.Timestamp.UTC()

	for _, wh := range webhooks {
		if !n.allow(wh.ID) {
			metrics.AlertsSent.WithLabelValues("suppressed").Inc()
			continue
		}
		go n.deliver(wh.URL, wh.PayloadFormat, event)
	}
}

// allow reports whether the per-webhook rate cap admits another alert.
func (n *Notifier) allow(id uuid.UUID) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	l, ok := n.limiters[id]
	if !ok {
		l = rate.NewLimiter(rate.Limit(alertsPerSecond), alertBurst)
		n.limiters[id] = l
	}
	return l.Allow()
}

func (n *Notifier) deliver(url, format string, event Event) {
	body, err := marshalPayload(format, event)
	if err != nil {
		metrics.AlertsSent.WithLabelValues("error").Inc()
		slog.Error("marshaling alert payload", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		metrics.AlertsSent.WithLabelValues("error").Inc()
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		metrics.AlertsSent.WithLabelValues("error").Inc()
		slog.Warn("alert webhook delivery failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		metrics.AlertsSent.WithLabelValues("error").Inc()
		slog.Warn("alert webhook returned non-2xx", "status", resp.StatusCode)
		return
	}
	metrics.AlertsSent.WithLabelValues("sent").Inc()
}

// marshalPayload renders the event for the webhook's payload format.
func marshalPayload(format string, event Event) ([]byte, error) {
	if format == "slack" {
		return json.Marshal(map[string]string{"text": slackText(event)})
	}
	// Generic JSON: the event itself, wrapped with a type for consumers.
	return json.Marshal(struct {
		Type string `json:"type"`
		Event
	}{Type: "guard_rejection", Event: event})
}

func slackText(event Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, ":no_entry: Sheeld rejected a request on source `%s` (%s phase)", event.SourceRoute, event.Phase)
	for _, g := range event.FailedGuards {
		fmt.Fprintf(&b, "\n• *%s* (%s): %s", g.Name, g.Type, g.Message)
	}
	if event.RequestID != "" {
		fmt.Fprintf(&b, "\nrequest_id: `%s`", event.RequestID)
	}
	return b.String()
}
