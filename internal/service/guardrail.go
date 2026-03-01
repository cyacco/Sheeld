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

// Create creates a new guardrail for a source.
func (s *GuardrailService) Create(ctx context.Context, sourceID uuid.UUID, params CreateGuardrailParams) (generated.Guardrail, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Guardrail{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.CreateGuardrail(ctx, generated.CreateGuardrailParams{
		SourceID:  sourceID,
		Name:      params.Name,
		GuardType: params.GuardType,
		Phase:     params.Phase,
		Config:    configJSON,
		Enabled:   params.Enabled,
	})
}

// Get retrieves a guardrail by ID.
func (s *GuardrailService) Get(ctx context.Context, sourceID, guardrailID uuid.UUID) (generated.Guardrail, error) {
	return s.queries.GetGuardrail(ctx, generated.GetGuardrailParams{
		ID:       guardrailID,
		SourceID: sourceID,
	})
}

// List returns all guardrails for a source.
func (s *GuardrailService) List(ctx context.Context, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListGuardrailsBySource(ctx, sourceID)
}

// ListEnabled returns only enabled guardrails for a source.
func (s *GuardrailService) ListEnabled(ctx context.Context, sourceID uuid.UUID) ([]generated.Guardrail, error) {
	return s.queries.ListEnabledGuardrailsBySource(ctx, sourceID)
}

// Update updates a guardrail.
func (s *GuardrailService) Update(ctx context.Context, sourceID, guardrailID uuid.UUID, params UpdateGuardrailParams) (generated.Guardrail, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Guardrail{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.UpdateGuardrail(ctx, generated.UpdateGuardrailParams{
		ID:        guardrailID,
		SourceID:  sourceID,
		Name:      params.Name,
		GuardType: params.GuardType,
		Phase:     params.Phase,
		Config:    configJSON,
		Enabled:   params.Enabled,
	})
}

// Delete deletes a guardrail.
func (s *GuardrailService) Delete(ctx context.Context, sourceID, guardrailID uuid.UUID) error {
	return s.queries.DeleteGuardrail(ctx, generated.DeleteGuardrailParams{
		ID:       guardrailID,
		SourceID: sourceID,
	})
}

// GuardrailResponse is the API-friendly representation of a guardrail.
type GuardrailResponse struct {
	ID        uuid.UUID              `json:"id"`
	SourceID  uuid.UUID              `json:"source_id"`
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Enabled   bool                   `json:"enabled"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// ToGuardrailResponse converts a database guardrail to an API response.
func ToGuardrailResponse(g generated.Guardrail) GuardrailResponse {
	var config map[string]interface{}
	json.Unmarshal(g.Config, &config)

	return GuardrailResponse{
		ID:        g.ID,
		SourceID:  g.SourceID,
		Name:      g.Name,
		GuardType: g.GuardType,
		Phase:     g.Phase,
		Config:    config,
		Enabled:   g.Enabled,
		CreatedAt: g.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: g.UpdatedAt.Format("2006-01-02T15:04:05Z"),
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
