package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sheeld/sheeld/internal/crypto"
	"github.com/sheeld/sheeld/internal/db/generated"
)

// CreateSourceParams holds the input for creating a source.
type CreateSourceParams struct {
	Name          string  `json:"name"`
	Route         string  `json:"route"`
	Description   *string `json:"description,omitempty"`
	LLMProvider   string  `json:"llm_provider"`
	LLMModel      string  `json:"llm_model"`
	LLMAPIKey     string  `json:"llm_api_key"`
	PassCriteria  string  `json:"pass_criteria"`
	PassThreshold *int32  `json:"pass_threshold,omitempty"`
	Enabled       bool    `json:"enabled"`
}

// UpdateSourceParams holds the input for updating a source.
type UpdateSourceParams struct {
	Name          string  `json:"name"`
	Route         string  `json:"route"`
	Description   *string `json:"description,omitempty"`
	LLMProvider   string  `json:"llm_provider"`
	LLMModel      string  `json:"llm_model"`
	LLMAPIKey     string  `json:"llm_api_key"`
	PassCriteria  string  `json:"pass_criteria"`
	PassThreshold *int32  `json:"pass_threshold,omitempty"`
	Enabled       bool    `json:"enabled"`
}

// SourceService handles source business logic.
type SourceService struct {
	queries       *generated.Queries
	encryptionKey string
}

// NewSourceService creates a new SourceService.
func NewSourceService(queries *generated.Queries, encryptionKey string) *SourceService {
	return &SourceService{
		queries:       queries,
		encryptionKey: encryptionKey,
	}
}

// Create creates a new source.
func (s *SourceService) Create(ctx context.Context, orgID uuid.UUID, params CreateSourceParams) (generated.Source, error) {
	encryptedKey, err := crypto.Encrypt(params.LLMAPIKey, s.encryptionKey)
	if err != nil {
		return generated.Source{}, fmt.Errorf("encrypting API key: %w", err)
	}

	description := pgtype.Text{}
	if params.Description != nil {
		description = pgtype.Text{String: *params.Description, Valid: true}
	}

	threshold := pgtype.Int4{}
	if params.PassThreshold != nil {
		threshold = pgtype.Int4{Int32: *params.PassThreshold, Valid: true}
	}

	return s.queries.CreateSource(ctx, generated.CreateSourceParams{
		OrganizationID: orgID,
		Name:           params.Name,
		Route:          params.Route,
		Description:    description,
		LlmProvider:    params.LLMProvider,
		LlmModel:       params.LLMModel,
		LlmApiKeyEnc:   encryptedKey,
		PassCriteria:   params.PassCriteria,
		PassThreshold:  threshold,
		Enabled:        params.Enabled,
	})
}

// Get retrieves a source by ID.
func (s *SourceService) Get(ctx context.Context, orgID, sourceID uuid.UUID) (generated.Source, error) {
	return s.queries.GetSource(ctx, generated.GetSourceParams{
		ID:             sourceID,
		OrganizationID: orgID,
	})
}

// GetByRoute retrieves a source by route.
func (s *SourceService) GetByRoute(ctx context.Context, orgID uuid.UUID, route string) (generated.Source, error) {
	return s.queries.GetSourceByRoute(ctx, generated.GetSourceByRouteParams{
		Route:          route,
		OrganizationID: orgID,
	})
}

// List returns all sources for an organization.
func (s *SourceService) List(ctx context.Context, orgID uuid.UUID) ([]generated.Source, error) {
	return s.queries.ListSourcesByOrganization(ctx, orgID)
}

// Update updates a source.
func (s *SourceService) Update(ctx context.Context, orgID, sourceID uuid.UUID, params UpdateSourceParams) (generated.Source, error) {
	encryptedKey, err := crypto.Encrypt(params.LLMAPIKey, s.encryptionKey)
	if err != nil {
		return generated.Source{}, fmt.Errorf("encrypting API key: %w", err)
	}

	description := pgtype.Text{}
	if params.Description != nil {
		description = pgtype.Text{String: *params.Description, Valid: true}
	}

	threshold := pgtype.Int4{}
	if params.PassThreshold != nil {
		threshold = pgtype.Int4{Int32: *params.PassThreshold, Valid: true}
	}

	return s.queries.UpdateSource(ctx, generated.UpdateSourceParams{
		ID:             sourceID,
		OrganizationID: orgID,
		Name:           params.Name,
		Route:          params.Route,
		Description:    description,
		LlmProvider:    params.LLMProvider,
		LlmModel:       params.LLMModel,
		LlmApiKeyEnc:   encryptedKey,
		PassCriteria:   params.PassCriteria,
		PassThreshold:  threshold,
		Enabled:        params.Enabled,
	})
}

// Delete deletes a source.
func (s *SourceService) Delete(ctx context.Context, orgID, sourceID uuid.UUID) error {
	return s.queries.DeleteSource(ctx, generated.DeleteSourceParams{
		ID:             sourceID,
		OrganizationID: orgID,
	})
}

// SourceResponse is the API-friendly representation of a source.
type SourceResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Route         string    `json:"route"`
	Description   *string   `json:"description,omitempty"`
	LLMProvider   string    `json:"llm_provider"`
	LLMModel      string    `json:"llm_model"`
	PassCriteria  string    `json:"pass_criteria"`
	PassThreshold *int32    `json:"pass_threshold,omitempty"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// ToResponse converts a database source to an API response (strips sensitive fields).
func ToSourceResponse(src generated.Source) SourceResponse {
	resp := SourceResponse{
		ID:           src.ID,
		Name:         src.Name,
		Route:        src.Route,
		LLMProvider:  src.LlmProvider,
		LLMModel:     src.LlmModel,
		PassCriteria: src.PassCriteria,
		Enabled:      src.Enabled,
		CreatedAt:    src.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    src.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if src.Description.Valid {
		resp.Description = &src.Description.String
	}
	if src.PassThreshold.Valid {
		resp.PassThreshold = &src.PassThreshold.Int32
	}
	return resp
}

// ToSourceResponses converts a slice of database sources to API responses.
func ToSourceResponses(sources []generated.Source) []SourceResponse {
	responses := make([]SourceResponse, len(sources))
	for i, src := range sources {
		responses[i] = ToSourceResponse(src)
	}
	return responses
}
