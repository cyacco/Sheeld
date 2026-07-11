package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// WorkspaceConfig is the full configuration payload served by the control
// plane and polled by data planes. It carries plaintext LLM API keys over
// the token-authenticated channel — it must never be logged.
type WorkspaceConfig struct {
	// Version is the sha256 of the canonical payload (excluding Version and
	// GeneratedAt), used as the ETag for cheap polling.
	Version       string      `json:"version"`
	GeneratedAt   time.Time   `json:"generated_at"`
	Organizations []OrgConfig `json:"organizations"`
}

// OrgConfig holds one organization's proxy-relevant configuration.
type OrgConfig struct {
	ID           uuid.UUID           `json:"id"`
	APIKeys      []APIKeyConfig      `json:"api_keys"`
	Sources      []SourceConfig      `json:"sources"`
	Guardrails   []GuardrailConfig   `json:"guardrails"`
	Transformers []TransformerConfig `json:"transformers"`
}

// APIKeyConfig carries the hash of an active API key and its optional
// per-key rate limits; the data plane authenticates proxy calls by hashing
// the presented key and matching. A nil limit means "use the data plane's
// default rate limit".
type APIKeyConfig struct {
	KeyHash string `json:"key_hash"`
	// RateLimitRPS / RateLimitBurst override the data plane's default rate
	// limit for requests authenticated with this key; nil = use the default.
	RateLimitRPS   *float64 `json:"rate_limit_rps,omitempty"`
	RateLimitBurst *int     `json:"rate_limit_burst,omitempty"`
}

// SourceConfig is a source as seen by the data plane.
type SourceConfig struct {
	ID                  uuid.UUID    `json:"id"`
	Route               string       `json:"route"`
	Enabled             bool         `json:"enabled"`
	LLMModel            string       `json:"llm_model"`
	LLMAPIKey           string       `json:"llm_api_key"`            // plaintext, decrypted by the control plane
	LLMBaseURL          string       `json:"llm_base_url,omitempty"` // per-source OpenAI-compatible endpoint; empty = data plane's gateway
	InputPassCriteria   PassCriteria `json:"input_pass_criteria"`
	InputPassThreshold  *int         `json:"input_pass_threshold,omitempty"`
	OutputPassCriteria  PassCriteria `json:"output_pass_criteria"`
	OutputPassThreshold *int         `json:"output_pass_threshold,omitempty"`
	GuardrailIDs        []uuid.UUID  `json:"guardrail_ids"`

	// TransformerIDs is ORDERED: the data plane runs transformers in this
	// sequence (source_transformers.position order).
	TransformerIDs []uuid.UUID `json:"transformer_ids"`
}

// TransformerConfig is a transformer as seen by the data plane. Only
// enabled transformers are included in the payload.
type TransformerConfig struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	TransformerType string          `json:"transformer_type"`
	Phase           string          `json:"phase"`
	Config          json.RawMessage `json:"config"`
}

// GuardrailConfig is a guardrail as seen by the data plane. Only enabled
// guardrails are included in the payload.
type GuardrailConfig struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	GuardType GuardType       `json:"guard_type"`
	Phase     GuardPhase      `json:"phase"`
	Config    json.RawMessage `json:"config"`
}
