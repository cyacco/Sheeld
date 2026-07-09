package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/crypto"
	"github.com/sheeld/sheeld/internal/shared/urlpolicy"
)

// CreateSourceParams holds the input for creating a source. Enabled is a
// pointer to distinguish an omitted field from an explicit false.
type CreateSourceParams struct {
	Name                string  `json:"name"`
	Route               string  `json:"route"`
	Description         *string `json:"description,omitempty"`
	LLMProvider         string  `json:"llm_provider"`
	LLMModel            string  `json:"llm_model"`
	LLMAPIKey           string  `json:"llm_api_key"`
	LLMBaseURL          string  `json:"llm_base_url,omitempty"`
	InputPassCriteria   string  `json:"input_pass_criteria"`
	InputPassThreshold  *int32  `json:"input_pass_threshold,omitempty"`
	OutputPassCriteria  string  `json:"output_pass_criteria"`
	OutputPassThreshold *int32  `json:"output_pass_threshold,omitempty"`
	Enabled             *bool   `json:"enabled"`
}

// UpdateSourceParams holds the input for updating a source.
type UpdateSourceParams struct {
	Name                string  `json:"name"`
	Route               string  `json:"route"`
	Description         *string `json:"description,omitempty"`
	LLMProvider         string  `json:"llm_provider"`
	LLMModel            string  `json:"llm_model"`
	LLMAPIKey           string  `json:"llm_api_key"`
	LLMBaseURL          string  `json:"llm_base_url,omitempty"`
	InputPassCriteria   string  `json:"input_pass_criteria"`
	InputPassThreshold  *int32  `json:"input_pass_threshold,omitempty"`
	OutputPassCriteria  string  `json:"output_pass_criteria"`
	OutputPassThreshold *int32  `json:"output_pass_threshold,omitempty"`
	Enabled             *bool   `json:"enabled"`
}

// validateCriteria applies the "all" default and checks one phase's
// criteria/threshold pair: n_of_m needs a threshold >= 1.
func validateCriteria(phase, criteria string, threshold *int32) (string, error) {
	if criteria == "" {
		criteria = "all"
	}
	switch criteria {
	case "all", "any":
	case "n_of_m":
		if threshold == nil || *threshold < 1 {
			return "", fmt.Errorf("%s_pass_threshold must be >= 1 when %s_pass_criteria is n_of_m", phase, phase)
		}
	default:
		return "", fmt.Errorf("%s_pass_criteria must be one of all, any, n_of_m", phase)
	}
	return criteria, nil
}

func toInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
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
	if err := validateLLMBaseURL(params.LLMBaseURL); err != nil {
		return generated.Source{}, err
	}
	encryptedKey, err := crypto.Encrypt(params.LLMAPIKey, s.encryptionKey)
	if err != nil {
		return generated.Source{}, fmt.Errorf("encrypting API key: %w", err)
	}

	description := pgtype.Text{}
	if params.Description != nil {
		description = pgtype.Text{String: *params.Description, Valid: true}
	}

	inputCriteria, err := validateCriteria("input", params.InputPassCriteria, params.InputPassThreshold)
	if err != nil {
		return generated.Source{}, err
	}
	outputCriteria, err := validateCriteria("output", params.OutputPassCriteria, params.OutputPassThreshold)
	if err != nil {
		return generated.Source{}, err
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	return s.queries.CreateSource(ctx, generated.CreateSourceParams{
		OrganizationID:      orgID,
		Name:                params.Name,
		Route:               params.Route,
		Description:         description,
		LlmProvider:         params.LLMProvider,
		LlmModel:            params.LLMModel,
		LlmApiKeyEnc:        encryptedKey,
		LlmBaseUrl:          params.LLMBaseURL,
		InputPassCriteria:   inputCriteria,
		InputPassThreshold:  toInt4(params.InputPassThreshold),
		OutputPassCriteria:  outputCriteria,
		OutputPassThreshold: toInt4(params.OutputPassThreshold),
		Enabled:             enabled,
	})
}

// validateLLMBaseURL checks an optional per-source LLM endpoint. Empty means
// "use the data plane's configured gateway". Non-empty URLs go through the
// shared SSRF policy: the data plane dials this URL directly.
func validateLLMBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	return urlpolicy.ValidatePublicHTTPURL(raw, "llm_base_url")
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

// Update updates a source. An empty LLMAPIKey keeps the stored key rather
// than overwriting it.
func (s *SourceService) Update(ctx context.Context, orgID, sourceID uuid.UUID, params UpdateSourceParams) (generated.Source, error) {
	if err := validateLLMBaseURL(params.LLMBaseURL); err != nil {
		return generated.Source{}, err
	}
	var encryptedKey string
	if params.LLMAPIKey == "" {
		existing, err := s.Get(ctx, orgID, sourceID)
		if err != nil {
			return generated.Source{}, fmt.Errorf("loading source: %w", err)
		}
		encryptedKey = existing.LlmApiKeyEnc
	} else {
		var err error
		encryptedKey, err = crypto.Encrypt(params.LLMAPIKey, s.encryptionKey)
		if err != nil {
			return generated.Source{}, fmt.Errorf("encrypting API key: %w", err)
		}
	}

	description := pgtype.Text{}
	if params.Description != nil {
		description = pgtype.Text{String: *params.Description, Valid: true}
	}

	inputCriteria, err := validateCriteria("input", params.InputPassCriteria, params.InputPassThreshold)
	if err != nil {
		return generated.Source{}, err
	}
	outputCriteria, err := validateCriteria("output", params.OutputPassCriteria, params.OutputPassThreshold)
	if err != nil {
		return generated.Source{}, err
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	return s.queries.UpdateSource(ctx, generated.UpdateSourceParams{
		ID:                  sourceID,
		OrganizationID:      orgID,
		Name:                params.Name,
		Route:               params.Route,
		Description:         description,
		LlmProvider:         params.LLMProvider,
		LlmModel:            params.LLMModel,
		LlmApiKeyEnc:        encryptedKey,
		LlmBaseUrl:          params.LLMBaseURL,
		InputPassCriteria:   inputCriteria,
		InputPassThreshold:  toInt4(params.InputPassThreshold),
		OutputPassCriteria:  outputCriteria,
		OutputPassThreshold: toInt4(params.OutputPassThreshold),
		Enabled:             enabled,
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
	ID                  uuid.UUID `json:"id"`
	Name                string    `json:"name"`
	Route               string    `json:"route"`
	Description         *string   `json:"description,omitempty"`
	LLMProvider         string    `json:"llm_provider"`
	LLMModel            string    `json:"llm_model"`
	LLMBaseURL          string    `json:"llm_base_url,omitempty"`
	InputPassCriteria   string    `json:"input_pass_criteria"`
	InputPassThreshold  *int32    `json:"input_pass_threshold,omitempty"`
	OutputPassCriteria  string    `json:"output_pass_criteria"`
	OutputPassThreshold *int32    `json:"output_pass_threshold,omitempty"`
	Enabled             bool      `json:"enabled"`
	CreatedAt           string    `json:"created_at"`
	UpdatedAt           string    `json:"updated_at"`
}

// ToResponse converts a database source to an API response (strips sensitive fields).
func ToSourceResponse(src generated.Source) SourceResponse {
	resp := SourceResponse{
		ID:                 src.ID,
		Name:               src.Name,
		Route:              src.Route,
		LLMProvider:        src.LlmProvider,
		LLMModel:           src.LlmModel,
		LLMBaseURL:         src.LlmBaseUrl,
		InputPassCriteria:  src.InputPassCriteria,
		OutputPassCriteria: src.OutputPassCriteria,
		Enabled:            src.Enabled,
		CreatedAt:          src.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          src.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if src.Description.Valid {
		resp.Description = &src.Description.String
	}
	if src.InputPassThreshold.Valid {
		resp.InputPassThreshold = &src.InputPassThreshold.Int32
	}
	if src.OutputPassThreshold.Valid {
		resp.OutputPassThreshold = &src.OutputPassThreshold.Int32
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
