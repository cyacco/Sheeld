package backendconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/shared/domain"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

// ResolvedSource is a source with its guards pre-built and split by phase,
// ready for the processor with no per-request work.
type ResolvedSource struct {
	ID                  uuid.UUID
	OrgID               uuid.UUID
	Route               string
	Enabled             bool
	LLMModel            string
	LLMAPIKey           string
	LLMBaseURL          string
	InputPassCriteria   domain.PassCriteria
	InputPassThreshold  *int
	OutputPassCriteria  domain.PassCriteria
	OutputPassThreshold *int
	InputGuards         []guard.Guard
	OutputGuards        []guard.Guard

	// InputTransformers run sequentially, in this order, on the request
	// before input guards; OutputTransformers run on the LLM response
	// before output guards. Both preserve the source chain's order.
	InputTransformers  []transform.Transformer
	OutputTransformers []transform.Transformer
}

type sourceKey struct {
	orgID uuid.UUID
	route string
}

// AuthInfo is the result of authenticating a proxy request against an API
// key: the owning org plus the key's optional per-key rate limits. A limit
// of 0 means "use the data plane's default".
type AuthInfo struct {
	OrgID          uuid.UUID
	KeyHash        string
	RateLimitRPS   float64
	RateLimitBurst int
}

// Snapshot is one immutable, fully-resolved view of the workspace config.
type Snapshot struct {
	Version string
	apiKeys map[string]AuthInfo // sha256 hex of raw key → auth info
	sources map[sourceKey]*ResolvedSource
	alerts  map[uuid.UUID][]domain.AlertWebhookConfig // org → enabled alert webhooks
}

// Store holds the current config snapshot, swapped atomically by the poller.
type Store struct {
	current atomic.Pointer[Snapshot]
}

// NewStore creates an empty store. Loaded() is false until the first apply.
func NewStore() *Store {
	return &Store{}
}

// Loaded reports whether a config snapshot has been applied.
func (s *Store) Loaded() bool {
	return s.current.Load() != nil
}

// Version returns the current config version, or "" if not loaded.
func (s *Store) Version() string {
	if snap := s.current.Load(); snap != nil {
		return snap.Version
	}
	return ""
}

// LookupAPIKey hashes the presented raw API key and returns the auth info
// (owning org + per-key rate limits) for it.
func (s *Store) LookupAPIKey(rawKey string) (AuthInfo, bool) {
	snap := s.current.Load()
	if snap == nil {
		return AuthInfo{}, false
	}
	sum := sha256.Sum256([]byte(rawKey))
	info, ok := snap.apiKeys[hex.EncodeToString(sum[:])]
	return info, ok
}

// LookupSource returns the resolved source for an org + route.
func (s *Store) LookupSource(orgID uuid.UUID, route string) (*ResolvedSource, bool) {
	snap := s.current.Load()
	if snap == nil {
		return nil, false
	}
	src, ok := snap.sources[sourceKey{orgID: orgID, route: route}]
	return src, ok
}

// AlertWebhooks returns the org's enabled rejection-alert webhooks.
func (s *Store) AlertWebhooks(orgID uuid.UUID) []domain.AlertWebhookConfig {
	snap := s.current.Load()
	if snap == nil {
		return nil
	}
	return snap.alerts[orgID]
}

// Apply resolves a workspace config into a snapshot (building guards via the
// registry) and atomically swaps it in. On error nothing is swapped and the
// previous snapshot keeps serving.
func (s *Store) Apply(cfg *domain.WorkspaceConfig, registry *guard.Registry, transformRegistry *transform.Registry) error {
	snap := &Snapshot{
		Version: cfg.Version,
		apiKeys: make(map[string]AuthInfo),
		sources: make(map[sourceKey]*ResolvedSource),
		alerts:  make(map[uuid.UUID][]domain.AlertWebhookConfig),
	}

	for _, org := range cfg.Organizations {
		if len(org.AlertWebhooks) > 0 {
			snap.alerts[org.ID] = org.AlertWebhooks
		}
		for _, k := range org.APIKeys {
			info := AuthInfo{OrgID: org.ID, KeyHash: k.KeyHash}
			if k.RateLimitRPS != nil {
				info.RateLimitRPS = *k.RateLimitRPS
			}
			if k.RateLimitBurst != nil {
				info.RateLimitBurst = *k.RateLimitBurst
			}
			snap.apiKeys[k.KeyHash] = info
		}

		guardrailsByID := make(map[uuid.UUID]domain.GuardrailConfig, len(org.Guardrails))
		for _, g := range org.Guardrails {
			guardrailsByID[g.ID] = g
		}

		transformersByID := make(map[uuid.UUID]domain.TransformerConfig, len(org.Transformers))
		for _, t := range org.Transformers {
			transformersByID[t.ID] = t
		}

		for _, src := range org.Sources {
			resolved := &ResolvedSource{
				ID:                  src.ID,
				OrgID:               org.ID,
				Route:               src.Route,
				Enabled:             src.Enabled,
				LLMModel:            src.LLMModel,
				LLMAPIKey:           src.LLMAPIKey,
				LLMBaseURL:          src.LLMBaseURL,
				InputPassCriteria:   src.InputPassCriteria,
				InputPassThreshold:  src.InputPassThreshold,
				OutputPassCriteria:  src.OutputPassCriteria,
				OutputPassThreshold: src.OutputPassThreshold,
			}
			for _, gid := range src.GuardrailIDs {
				gc, ok := guardrailsByID[gid]
				if !ok {
					// Disabled or deleted guardrail still attached — skip.
					continue
				}
				g, err := registry.Create(string(gc.GuardType), gc.Name, gc.Config)
				if err != nil {
					return fmt.Errorf("building guard %q for source %q: %w", gc.Name, src.Route, err)
				}
				if readScope(gc.Config) == "all_messages" {
					g = guard.WithScopeAllMessages(g)
				}
				if isFailOpen(gc.Config) {
					g = guard.WithFailOpen(g)
				}
				// Shadow (monitor-only): the guard runs and is audited but
				// never blocks. Wrap order is not significant — the engine
				// detects markers through the wrapper chain.
				if isShadow(gc.Config) {
					g = guard.WithShadow(g)
				}
				if gc.Phase == domain.GuardPhaseInput || gc.Phase == domain.GuardPhaseBoth {
					resolved.InputGuards = append(resolved.InputGuards, g)
				}
				if gc.Phase == domain.GuardPhaseOutput || gc.Phase == domain.GuardPhaseBoth {
					resolved.OutputGuards = append(resolved.OutputGuards, g)
				}
			}
			for _, tid := range src.TransformerIDs {
				tc, ok := transformersByID[tid]
				if !ok {
					// Disabled or deleted transformer still attached — skip.
					continue
				}
				tr, err := transformRegistry.Create(tc.TransformerType, tc.Name, tc.Config)
				if err != nil {
					return fmt.Errorf("building transformer %q for source %q: %w", tc.Name, src.Route, err)
				}
				if isFailOpen(tc.Config) {
					tr = transform.WithFailOpen(tr)
				}
				switch tc.Phase {
				case "input":
					resolved.InputTransformers = append(resolved.InputTransformers, tr)
				case "output":
					resolved.OutputTransformers = append(resolved.OutputTransformers, tr)
				default:
					// Unknown phase from a newer control plane — skip.
				}
			}

			snap.sources[sourceKey{orgID: org.ID, route: src.Route}] = resolved
		}
	}

	s.current.Store(snap)
	return nil
}

// readScope reads the optional scope field from a guardrail's config:
// "last_message" (default) or "all_messages".
func readScope(config json.RawMessage) string {
	var c struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return ""
	}
	return c.Scope
}

// isFailOpen reads the optional on_error field from a guardrail's config.
// Guards fail closed by default; "fail_open" means execution errors count
// as passed.
func isFailOpen(config json.RawMessage) bool {
	var c struct {
		OnError string `json:"on_error"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return false
	}
	return c.OnError == "fail_open"
}

// isShadow reads the optional mode field: "shadow" runs the guard monitor-only
// (recorded but never blocking); anything else (default "enforce") enforces.
func isShadow(config json.RawMessage) bool {
	var c struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return false
	}
	return c.Mode == "shadow"
}
