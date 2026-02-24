package domain

import (
	"time"

	"github.com/google/uuid"
)

// Organization represents a tenant in the system.
type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents an authenticated user within an organization.
type User struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// APIKey represents a machine-to-machine API key.
type APIKey struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Name           string     `json:"name"`
	KeyHash        string     `json:"-"`
	KeyPrefix      string     `json:"key_prefix"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// PassCriteria defines how guardrail results are evaluated.
type PassCriteria string

const (
	PassCriteriaAll  PassCriteria = "all"
	PassCriteriaAny  PassCriteria = "any"
	PassCriteriaNofM PassCriteria = "n_of_m"
)

// Source represents a named entry point for LLM requests.
type Source struct {
	ID             uuid.UUID    `json:"id"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	Name           string       `json:"name"`
	Slug           string       `json:"slug"`
	Description    *string      `json:"description,omitempty"`
	LLMProvider    string       `json:"llm_provider"`
	LLMModel       string       `json:"llm_model"`
	LLMAPIKeyEnc   string       `json:"-"`
	PassCriteria   PassCriteria `json:"pass_criteria"`
	PassThreshold  *int         `json:"pass_threshold,omitempty"`
	Enabled        bool         `json:"enabled"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// GuardType identifies the type of guardrail.
type GuardType string

const (
	GuardTypeBlocklist    GuardType = "blocklist"
	GuardTypeRegex        GuardType = "regex"
	GuardTypeOpenAIMod    GuardType = "openai_moderation"
	GuardTypeGuardrailsAI GuardType = "guardrails_ai"
)

// GuardPhase defines when a guardrail runs.
type GuardPhase string

const (
	GuardPhaseInput  GuardPhase = "input"
	GuardPhaseOutput GuardPhase = "output"
	GuardPhaseBoth   GuardPhase = "both"
)

// Destination represents a guardrail instance attached to a source.
type Destination struct {
	ID        uuid.UUID              `json:"id"`
	SourceID  uuid.UUID              `json:"source_id"`
	Name      string                 `json:"name"`
	GuardType GuardType              `json:"guard_type"`
	Phase     GuardPhase             `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Priority  int                    `json:"priority"`
	Enabled   bool                   `json:"enabled"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// AuditLog records the result of a proxy request.
type AuditLog struct {
	ID             uuid.UUID              `json:"id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
	SourceID       uuid.UUID              `json:"source_id"`
	InputHash      *string                `json:"input_hash,omitempty"`
	GuardResults   map[string]interface{} `json:"guard_results"`
	OverallResult  string                 `json:"overall_result"`
	LatencyMs      int                    `json:"latency_ms"`
	CreatedAt      time.Time              `json:"created_at"`
}
