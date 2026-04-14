package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/db/generated"
)

// CreateGuardrailParams holds the input for creating a guardrail.
type CreateGuardrailParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Enabled   bool                   `json:"enabled"`
}

// UpdateGuardrailParams holds the input for updating a guardrail.
type UpdateGuardrailParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Enabled   bool                   `json:"enabled"`
}

// GuardrailService handles guardrail business logic.
type GuardrailService struct {
	queries *generated.Queries
}

// NewGuardrailService creates a new GuardrailService.
func NewGuardrailService(queries *generated.Queries) *GuardrailService {
	return &GuardrailService{queries: queries}
}

// Create creates a new org-level guardrail.
func (s *GuardrailService) Create(ctx context.Context, orgID uuid.UUID, params CreateGuardrailParams) (generated.Guardrail, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Guardrail{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.CreateGuardrail(ctx, generated.CreateGuardrailParams{
		OrganizationID: orgID,
		Name:           params.Name,
		GuardType:      params.GuardType,
		Phase:          params.Phase,
		Config:         configJSON,
		Enabled:        params.Enabled,
	})
}

// Get retrieves a guardrail by ID, scoped to an organization.
func (s *GuardrailService) Get(ctx context.Context, orgID, guardrailID uuid.UUID) (generated.Guardrail, error) {
	return s.queries.GetGuardrail(ctx, generated.GetGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
	})
}

// List returns all guardrails for an organization.
func (s *GuardrailService) List(ctx context.Context, orgID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListGuardrailsByOrg(ctx, orgID)
}

// ListBySource returns all guardrails attached to a source.
func (s *GuardrailService) ListBySource(ctx context.Context, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListGuardrailsBySource(ctx, sourceID)
}

// ListEnabledBySource returns only enabled guardrails attached to a source,
// scoped to an organization for defense in depth.
func (s *GuardrailService) ListEnabledBySource(ctx context.Context, orgID, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListEnabledGuardrailsBySource(ctx, generated.ListEnabledGuardrailsBySourceParams{
		SourceID:       sourceID,
		OrganizationID: orgID,
	})
}

// Update updates a guardrail, scoped to an organization.
func (s *GuardrailService) Update(ctx context.Context, orgID, guardrailID uuid.UUID, params UpdateGuardrailParams) (generated.Guardrail, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Guardrail{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.UpdateGuardrail(ctx, generated.UpdateGuardrailParams{
		ID:             guardrailID,
		OrganizationID: orgID,
		Name:           params.Name,
		GuardType:      params.GuardType,
		Phase:          params.Phase,
		Config:         configJSON,
		Enabled:        params.Enabled,
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

// DetachFromSource detaches a guardrail from a source.
func (s *GuardrailService) DetachFromSource(ctx context.Context, guardrailID, sourceID uuid.UUID) error {
	return s.queries.DetachGuardrailFromSource(ctx, generated.DetachGuardrailFromSourceParams{
		SourceID:    sourceID,
		GuardrailID: guardrailID,
	})
}

// ListSources returns all sources attached to a guardrail.
func (s *GuardrailService) ListSources(ctx context.Context, guardrailID uuid.UUID) ([]generated.Source, error) {
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
