package backendconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/shared/domain"
	"github.com/sheeld/sheeld/internal/shared/guard"
)

// ResolvedSource is a source with its guards pre-built and split by phase,
// ready for the processor with no per-request work.
type ResolvedSource struct {
	ID            uuid.UUID
	OrgID         uuid.UUID
	Route         string
	Enabled       bool
	LLMModel      string
	LLMAPIKey     string
	PassCriteria  domain.PassCriteria
	PassThreshold *int
	InputGuards   []guard.Guard
	OutputGuards  []guard.Guard
}

type sourceKey struct {
	orgID uuid.UUID
	route string
}

// Snapshot is one immutable, fully-resolved view of the workspace config.
type Snapshot struct {
	Version string
	apiKeys map[string]uuid.UUID // sha256 hex of raw key → org ID
	sources map[sourceKey]*ResolvedSource
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

// LookupOrgByAPIKey hashes the presented raw API key and returns the owning
// org ID.
func (s *Store) LookupOrgByAPIKey(rawKey string) (uuid.UUID, bool) {
	snap := s.current.Load()
	if snap == nil {
		return uuid.Nil, false
	}
	sum := sha256.Sum256([]byte(rawKey))
	orgID, ok := snap.apiKeys[hex.EncodeToString(sum[:])]
	return orgID, ok
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

// Apply resolves a workspace config into a snapshot (building guards via the
// registry) and atomically swaps it in. On error nothing is swapped and the
// previous snapshot keeps serving.
func (s *Store) Apply(cfg *domain.WorkspaceConfig, registry *guard.Registry) error {
	snap := &Snapshot{
		Version: cfg.Version,
		apiKeys: make(map[string]uuid.UUID),
		sources: make(map[sourceKey]*ResolvedSource),
	}

	for _, org := range cfg.Organizations {
		for _, k := range org.APIKeys {
			snap.apiKeys[k.KeyHash] = org.ID
		}

		guardrailsByID := make(map[uuid.UUID]domain.GuardrailConfig, len(org.Guardrails))
		for _, g := range org.Guardrails {
			guardrailsByID[g.ID] = g
		}

		for _, src := range org.Sources {
			resolved := &ResolvedSource{
				ID:            src.ID,
				OrgID:         org.ID,
				Route:         src.Route,
				Enabled:       src.Enabled,
				LLMModel:      src.LLMModel,
				LLMAPIKey:     src.LLMAPIKey,
				PassCriteria:  src.PassCriteria,
				PassThreshold: src.PassThreshold,
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
				if isFailOpen(gc.Config) {
					g = guard.WithFailOpen(g)
				}
				if gc.Phase == domain.GuardPhaseInput || gc.Phase == domain.GuardPhaseBoth {
					resolved.InputGuards = append(resolved.InputGuards, g)
				}
				if gc.Phase == domain.GuardPhaseOutput || gc.Phase == domain.GuardPhaseBoth {
					resolved.OutputGuards = append(resolved.OutputGuards, g)
				}
			}
			snap.sources[sourceKey{orgID: org.ID, route: src.Route}] = resolved
		}
	}

	s.current.Store(snap)
	return nil
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
