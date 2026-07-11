package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/controlplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/guard"
)

// CreateGuardrailParams holds the input for creating a guardrail. Enabled is
// a pointer to distinguish an omitted field from an explicit false.
type CreateGuardrailParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Enabled   *bool                  `json:"enabled"`
}

// UpdateGuardrailParams holds the input for updating a guardrail.
type UpdateGuardrailParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Enabled   *bool                  `json:"enabled"`
}

// guardrailDefaults applies the documented defaults for omitted optional
// fields: enabled=true, phase="input".
func guardrailDefaults(phase string, enabled *bool) (string, bool) {
	if phase == "" {
		phase = "input"
	}
	e := true
	if enabled != nil {
		e = *enabled
	}
	return phase, e
}

// GuardrailService handles guardrail business logic.
type GuardrailService struct {
	queries  *generated.Queries
	registry *guard.Registry
}

// NewGuardrailService creates a new GuardrailService.
func NewGuardrailService(queries *generated.Queries, registry *guard.Registry) *GuardrailService {
	return &GuardrailService{queries: queries, registry: registry}
}

// validateConfig instantiates the guard through the registry so an unknown
// guard_type or an invalid config (bad regex, missing URL) surfaces here
// instead of at data-plane resolution time.
func (s *GuardrailService) validateConfig(name, guardType string, config map[string]interface{}) ([]byte, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	if _, err := s.registry.Create(guardType, name, configJSON); err != nil {
		return nil, err
	}
	return configJSON, nil
}

// Create creates a new org-level guardrail.
func (s *GuardrailService) Create(ctx context.Context, orgID uuid.UUID, params CreateGuardrailParams) (generated.Guardrail, error) {
	configJSON, err := s.validateConfig(params.Name, params.GuardType, params.Config)
	if err != nil {
		return generated.Guardrail{}, err
	}

	phase, enabled := guardrailDefaults(params.Phase, params.Enabled)

	return s.queries.CreateGuardrail(ctx, generated.CreateGuardrailParams{
		OrganizationID: orgID,
		Name:           params.Name,
		GuardType:      params.GuardType,
		Phase:          phase,
		Config:         configJSON,
		Enabled:        enabled,
	})
}

// Get retrieves a guardrail by ID, scoped to an organization.
func (s *GuardrailService) Get(ctx context.Context, orgID, guardrailID uuid.UUID) (generated.Guardrail, error) {
	return s.queries.GetGuardrail(ctx, generated.GetGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
	})
}

// Test builds the guard from a stored guardrail's config and runs it against
// sample text, returning the guard result without involving the data plane or
// any source. Org-scoped via Get. Network-backed guards (webhook, presidio,
// llm_classifier) make their real calls, so an unreachable dependency surfaces
// as an error here.
func (s *GuardrailService) Test(ctx context.Context, orgID, guardrailID uuid.UUID, input string) (*guard.Result, error) {
	g, err := s.Get(ctx, orgID, guardrailID)
	if err != nil {
		return nil, fmt.Errorf("guardrail not found: %w", err)
	}
	built, err := s.registry.Create(g.GuardType, g.Name, g.Config)
	if err != nil {
		return nil, fmt.Errorf("building guard: %w", err)
	}
	// Guards read optional CallMeta; mark this as a test invocation.
	ctx = guard.WithCallMeta(ctx, guard.CallMeta{Phase: "test"})
	return built.Validate(ctx, input)
}

// List returns all guardrails for an organization.
func (s *GuardrailService) List(ctx context.Context, orgID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListGuardrailsByOrg(ctx, orgID)
}

// ListBySource returns all guardrails attached to a source, validating org
// ownership of the source first.
func (s *GuardrailService) ListBySource(ctx context.Context, orgID, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	if _, err := s.queries.GetSource(ctx, generated.GetSourceParams{ID: sourceID, OrganizationID: orgID}); err != nil {
		return nil, fmt.Errorf("source not found: %w", err)
	}
	return s.queries.ListGuardrailsBySource(ctx, sourceID)
}

// ListEnabledBySource returns only enabled guardrails attached to a source.
func (s *GuardrailService) ListEnabledBySource(ctx context.Context, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListEnabledGuardrailsBySource(ctx, sourceID)
}

// Update updates a guardrail, scoped to an organization.
func (s *GuardrailService) Update(ctx context.Context, orgID, guardrailID uuid.UUID, params UpdateGuardrailParams) (generated.Guardrail, error) {
	// Preserve stored secrets when the client resubmits a redacted config
	// (loaded from a sanitized GET response).
	if existing, err := s.Get(ctx, orgID, guardrailID); err == nil {
		var storedConfig map[string]interface{}
		json.Unmarshal(existing.Config, &storedConfig)
		params.Config = PreserveRedactedSecrets(params.Config, storedConfig)
	}

	configJSON, err := s.validateConfig(params.Name, params.GuardType, params.Config)
	if err != nil {
		return generated.Guardrail{}, err
	}

	phase, enabled := guardrailDefaults(params.Phase, params.Enabled)

	return s.queries.UpdateGuardrail(ctx, generated.UpdateGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
		Name:           params.Name,
		GuardType:      params.GuardType,
		Phase:          phase,
		Config:         configJSON,
		Enabled:        enabled,
	})
}

// Delete deletes a guardrail, scoped to an organization.
func (s *GuardrailService) Delete(ctx context.Context, orgID, guardrailID uuid.UUID) error {
	return s.queries.DeleteGuardrail(ctx, generated.DeleteGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
	})
}

// AttachToSource attaches a guardrail to a source, validating org ownership.
func (s *GuardrailService) AttachToSource(ctx context.Context, orgID, guardrailID, sourceID uuid.UUID) error {
	// Validate guardrail belongs to org
	_, err := s.queries.GetGuardrail(ctx, generated.GetGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
	})
	if err != nil {
		return fmt.Errorf("guardrail not found: %w", err)
	}

	return s.queries.AttachGuardrailToSource(ctx, generated.AttachGuardrailToSourceParams{
		SourceID:    sourceID,
		GuardrailID: guardrailID,
	})
}

// DetachFromSource detaches a guardrail from a source, validating org
// ownership of the guardrail first.
func (s *GuardrailService) DetachFromSource(ctx context.Context, orgID, guardrailID, sourceID uuid.UUID) error {
	if _, err := s.Get(ctx, orgID, guardrailID); err != nil {
		return fmt.Errorf("guardrail not found: %w", err)
	}
	return s.queries.DetachGuardrailFromSource(ctx, generated.DetachGuardrailFromSourceParams{
		SourceID:    sourceID,
		GuardrailID: guardrailID,
	})
}

// ListSources returns all sources attached to a guardrail, validating org
// ownership of the guardrail first.
func (s *GuardrailService) ListSources(ctx context.Context, orgID, guardrailID uuid.UUID) ([]generated.Source, error) {
	if _, err := s.Get(ctx, orgID, guardrailID); err != nil {
		return nil, fmt.Errorf("guardrail not found: %w", err)
	}
	return s.queries.ListSourcesByGuardrail(ctx, guardrailID)
}

// GuardrailResponse is the API-friendly representation of a guardrail.
type GuardrailResponse struct {
	ID             uuid.UUID              `json:"id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
	Name           string                 `json:"name"`
	GuardType      string                 `json:"guard_type"`
	Phase          string                 `json:"phase"`
	Config         map[string]interface{} `json:"config"`
	Enabled        bool                   `json:"enabled"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// ToGuardrailResponse converts a database guardrail to an API response.
func ToGuardrailResponse(g generated.Guardrail) GuardrailResponse {
	var config map[string]interface{}
	json.Unmarshal(g.Config, &config)
	config = SanitizeConfig(config)

	return GuardrailResponse{
		ID:             g.ID,
		OrganizationID: g.OrganizationID,
		Name:           g.Name,
		GuardType:      g.GuardType,
		Phase:          g.Phase,
		Config:         config,
		Enabled:        g.Enabled,
		CreatedAt:      g.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      g.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ToGuardrailResponses converts a slice of database guardrails to API responses.
func ToGuardrailResponses(guardrails []generated.Guardrail) []GuardrailResponse {
	responses := make([]GuardrailResponse, len(guardrails))
	for i, g := range guardrails {
		responses[i] = ToGuardrailResponse(g)
	}
	return responses
}
