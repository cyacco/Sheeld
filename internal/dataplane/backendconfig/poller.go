package backendconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/metrics"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

// Poller periodically fetches the workspace config from the control plane
// and applies it to the store.
type Poller struct {
	url               string
	token             string
	interval          time.Duration
	store             *Store
	registry          *guard.Registry
	transformRegistry *transform.Registry
	client            *http.Client

	// snap, when non-nil, persists each applied config and serves as a
	// startup fallback while the control plane is unreachable.
	snap *Snapshotter

	lastETag string
}

// NewPoller creates a poller against the control plane's workspace-config
// endpoint.
func NewPoller(controlPlaneURL, token string, interval time.Duration, store *Store, registry *guard.Registry, transformRegistry *transform.Registry) *Poller {
	return &Poller{
		url:               controlPlaneURL + "/v1/internal/workspace-config",
		token:             token,
		interval:          interval,
		store:             store,
		registry:          registry,
		transformRegistry: transformRegistry,
		client:            &http.Client{Timeout: 15 * time.Second},
	}
}

// WithSnapshotter enables disk snapshots of applied configs.
func (p *Poller) WithSnapshotter(snap *Snapshotter) *Poller {
	p.snap = snap
	return p
}

// FetchOnce fetches and applies the config a single time. A 304 is a no-op
// success.
func (p *Poller) FetchOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if p.lastETag != "" {
		req.Header.Set("If-None-Match", p.lastETag)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching workspace config: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil
	case http.StatusOK:
		// Never log the body — it contains plaintext LLM keys.
		var cfg domain.WorkspaceConfig
		if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
			return fmt.Errorf("decoding workspace config: %w", err)
		}
		if err := p.store.Apply(&cfg, p.registry, p.transformRegistry); err != nil {
			return fmt.Errorf("applying workspace config: %w", err)
		}
		metrics.ConfigApplied()
		p.lastETag = resp.Header.Get("ETag")
		slog.Info("workspace config applied", "version", cfg.Version, "organizations", len(cfg.Organizations))
		if p.snap != nil {
			if err := p.snap.Save(&cfg); err != nil {
				// Snapshot failures never affect serving.
				slog.Warn("failed to persist config snapshot", "error", err)
			}
		}
		return nil
	default:
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("workspace config fetch: unexpected status %d", resp.StatusCode)
	}
}

// WaitForInitial blocks until the first successful fetch or the timeout
// elapses. Returns an error on timeout; the caller may still start serving
// (the store reports Loaded() == false).
func (p *Poller) WaitForInitial(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	backoff := time.Second
	for {
		if err := p.FetchOnce(ctx); err == nil {
			return nil
		} else {
			slog.Warn("initial workspace config fetch failed", "error", err)
		}
		if time.Now().After(deadline) {
			if p.snap != nil {
				if cfg, err := p.snap.Load(); err == nil {
					if err := p.store.Apply(cfg, p.registry, p.transformRegistry); err == nil {
						metrics.ConfigApplied()
						slog.Warn("control plane unreachable; serving from disk config snapshot",
							"version", cfg.Version)
						return nil
					}
				} else {
					slog.Warn("no usable config snapshot", "error", err)
				}
			}
			return fmt.Errorf("no workspace config after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 8*time.Second {
			backoff *= 2
		}
	}
}

// Run polls until the context is cancelled. Errors keep the previous
// snapshot serving, with exponential backoff capped at 6x the poll interval.
func (p *Poller) Run(ctx context.Context) {
	wait := p.interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		if err := p.FetchOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("workspace config poll failed; keeping previous config", "error", err)
			wait = min(wait*2, 6*p.interval)
		} else {
			wait = p.interval
		}
	}
}
