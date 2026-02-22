package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/db/generated"
)

// CreateDestinationParams holds the input for creating a destination.
type CreateDestinationParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Priority  int32                  `json:"priority"`
	Enabled   bool                   `json:"enabled"`
}

// UpdateDestinationParams holds the input for updating a destination.
type UpdateDestinationParams struct {
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Priority  int32                  `json:"priority"`
	Enabled   bool                   `json:"enabled"`
}

// DestinationService handles destination business logic.
type DestinationService struct {
	queries *generated.Queries
}

// NewDestinationService creates a new DestinationService.
func NewDestinationService(queries *generated.Queries) *DestinationService {
	return &DestinationService{queries: queries}
}

// Create creates a new destination for a source.
func (s *DestinationService) Create(ctx context.Context, sourceID uuid.UUID, params CreateDestinationParams) (generated.Destination, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Destination{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.CreateDestination(ctx, generated.CreateDestinationParams{
		SourceID:  sourceID,
		Name:      params.Name,
		GuardType: params.GuardType,
		Phase:     params.Phase,
		Config:    configJSON,
		Priority:  params.Priority,
		Enabled:   params.Enabled,
	})
}

// Get retrieves a destination by ID.
func (s *DestinationService) Get(ctx context.Context, sourceID, destID uuid.UUID) (generated.Destination, error) {
	return s.queries.GetDestination(ctx, generated.GetDestinationParams{
		ID:       destID,
		SourceID: sourceID,
	})
}

// List returns all destinations for a source.
func (s *DestinationService) List(ctx context.Context, sourceID uuid.UUID) ([]generated.Destination, error) {
	return s.queries.ListDestinationsBySource(ctx, sourceID)
}

// ListEnabled returns only enabled destinations for a source.
func (s *DestinationService) ListEnabled(ctx context.Context, sourceID uuid.UUID) ([]generated.Destination, error) {
	return s.queries.ListEnabledDestinationsBySource(ctx, sourceID)
}

// Update updates a destination.
func (s *DestinationService) Update(ctx context.Context, sourceID, destID uuid.UUID, params UpdateDestinationParams) (generated.Destination, error) {
	configJSON, err := json.Marshal(params.Config)
	if err != nil {
		return generated.Destination{}, fmt.Errorf("marshaling config: %w", err)
	}

	return s.queries.UpdateDestination(ctx, generated.UpdateDestinationParams{
		ID:        destID,
		SourceID:  sourceID,
		Name:      params.Name,
		GuardType: params.GuardType,
		Phase:     params.Phase,
		Config:    configJSON,
		Priority:  params.Priority,
		Enabled:   params.Enabled,
	})
}

// Delete deletes a destination.
func (s *DestinationService) Delete(ctx context.Context, sourceID, destID uuid.UUID) error {
	return s.queries.DeleteDestination(ctx, generated.DeleteDestinationParams{
		ID:       destID,
		SourceID: sourceID,
	})
}

// DestinationResponse is the API-friendly representation of a destination.
type DestinationResponse struct {
	ID        uuid.UUID              `json:"id"`
	SourceID  uuid.UUID              `json:"source_id"`
	Name      string                 `json:"name"`
	GuardType string                 `json:"guard_type"`
	Phase     string                 `json:"phase"`
	Config    map[string]interface{} `json:"config"`
	Priority  int32                  `json:"priority"`
	Enabled   bool                   `json:"enabled"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// ToDestinationResponse converts a database destination to an API response.
func ToDestinationResponse(dest generated.Destination) DestinationResponse {
	var config map[string]interface{}
	json.Unmarshal(dest.Config, &config)

	return DestinationResponse{
		ID:        dest.ID,
		SourceID:  dest.SourceID,
		Name:      dest.Name,
		GuardType: dest.GuardType,
		Phase:     dest.Phase,
		Config:    config,
		Priority:  dest.Priority,
		Enabled:   dest.Enabled,
		CreatedAt: dest.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: dest.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ToDestinationResponses converts a slice of database destinations to API responses.
func ToDestinationResponses(dests []generated.Destination) []DestinationResponse {
	responses := make([]DestinationResponse, len(dests))
	for i, dest := range dests {
		responses[i] = ToDestinationResponse(dest)
	}
	return responses
}
