package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/transform"
)

// CreateTransformerParams holds the input for creating a transformer.
// Enabled is a pointer to distinguish an omitted field from explicit false.
type CreateTransformerParams struct {
	Name            string                 `json:"name"`
	TransformerType string                 `json:"transformer_type"`
	Phase           string                 `json:"phase"`
	Config          map[string]interface{} `json:"config"`
	Enabled         *bool                  `json:"enabled"`
}

// UpdateTransformerParams holds the input for updating a transformer.
type UpdateTransformerParams = CreateTransformerParams

// TransformerService handles transformer business logic. The registry is
// used to validate transformer_type server-side.
type TransformerService struct {
	queries  *generated.Queries
	pool     *pgxpool.Pool
	registry *transform.Registry
}

// NewTransformerService creates a new TransformerService.
func NewTransformerService(queries *generated.Queries, pool *pgxpool.Pool, registry *transform.Registry) *TransformerService {
	return &TransformerService{queries: queries, pool: pool, registry: registry}
}

// validate applies defaults and validates type/phase. Returns the resolved
// phase, enabled flag, and marshaled config.
func (s *TransformerService) validate(params CreateTransformerParams) (string, bool, []byte, error) {
	if !s.registry.Has(params.TransformerType) {
		return "", false, nil, fmt.Errorf("unknown transformer_type: %q", params.TransformerType)
	}
	phase := params.Phase
	if phase == "" {
		phase = "input"
	}
	if phase != "input" {
		return "", false, nil, fmt.Errorf("phase must be \"input\" (only input transformers are supported)")
	}
	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}
	config := params.Config
	if config == nil {
		config = map[string]interface{}{}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", false, nil, fmt.Errorf("marshaling config: %w", err)
	}
	return phase, enabled, configJSON, nil
}

// Create creates a new org-level transformer.
func (s *TransformerService) Create(ctx context.Context, orgID uuid.UUID, params CreateTransformerParams) (generated.Transformer, error) {
	phase, enabled, configJSON, err := s.validate(params)
	if err != nil {
		return generated.Transformer{}, err
	}
	return s.queries.CreateTransformer(ctx, generated.CreateTransformerParams{
		OrganizationID:  orgID,
		Name:            params.Name,
		TransformerType: params.TransformerType,
		Phase:           phase,
		Config:          configJSON,
		Enabled:         enabled,
	})
}

// Get retrieves a transformer by ID, scoped to an organization.
func (s *TransformerService) Get(ctx context.Context, orgID, id uuid.UUID) (generated.Transformer, error) {
	return s.queries.GetTransformer(ctx, generated.GetTransformerParams{ID: id, OrganizationID: orgID})
}

// List returns all transformers for an organization.
func (s *TransformerService) List(ctx context.Context, orgID uuid.UUID) ([]generated.Transformer, error) {
	return s.queries.ListTransformersByOrg(ctx, orgID)
}

// Update updates a transformer, scoped to an organization.
func (s *TransformerService) Update(ctx context.Context, orgID, id uuid.UUID, params UpdateTransformerParams) (generated.Transformer, error) {
	phase, enabled, configJSON, err := s.validate(params)
	if err != nil {
		return generated.Transformer{}, err
	}
	return s.queries.UpdateTransformer(ctx, generated.UpdateTransformerParams{
		ID:              id,
		OrganizationID:  orgID,
		Name:            params.Name,
		TransformerType: params.TransformerType,
		Phase:           phase,
		Config:          configJSON,
		Enabled:         enabled,
	})
}

// Delete deletes a transformer, scoped to an organization.
func (s *TransformerService) Delete(ctx context.Context, orgID, id uuid.UUID) error {
	return s.queries.DeleteTransformer(ctx, generated.DeleteTransformerParams{ID: id, OrganizationID: orgID})
}

// AttachToSource appends a transformer to a source's chain, validating org
// ownership of the transformer.
func (s *TransformerService) AttachToSource(ctx context.Context, orgID, transformerID, sourceID uuid.UUID) error {
	if _, err := s.Get(ctx, orgID, transformerID); err != nil {
		return fmt.Errorf("transformer not found: %w", err)
	}
	return s.queries.AttachTransformerToSource(ctx, generated.AttachTransformerToSourceParams{
		SourceID:      sourceID,
		TransformerID: transformerID,
	})
}

// DetachFromSource detaches a transformer from a source.
func (s *TransformerService) DetachFromSource(ctx context.Context, transformerID, sourceID uuid.UUID) error {
	return s.queries.DetachTransformerFromSource(ctx, generated.DetachTransformerFromSourceParams{
		SourceID:      sourceID,
		TransformerID: transformerID,
	})
}

// ListSources returns all sources attached to a transformer.
func (s *TransformerService) ListSources(ctx context.Context, transformerID uuid.UUID) ([]generated.Source, error) {
	return s.queries.ListSourcesByTransformer(ctx, transformerID)
}

// ListBySource returns a source's transformers in chain order.
func (s *TransformerService) ListBySource(ctx context.Context, sourceID uuid.UUID) ([]generated.ListTransformersBySourceRow, error) {
	return s.queries.ListTransformersBySource(ctx, sourceID)
}

// SetSourceTransformers replaces a source's transformer chain with the given
// ordered IDs (positions 0..n-1) in one transaction. Every ID must belong to
// the organization.
func (s *TransformerService) SetSourceTransformers(ctx context.Context, orgID, sourceID uuid.UUID, ids []uuid.UUID) error {
	if len(uniqueUUIDs(ids)) != len(ids) {
		return fmt.Errorf("duplicate transformer ids")
	}
	count, err := s.queries.CountTransformersByOrgAndIDs(ctx, generated.CountTransformersByOrgAndIDsParams{
		OrganizationID: orgID,
		Column2:        ids,
	})
	if err != nil {
		return fmt.Errorf("validating transformer ids: %w", err)
	}
	if int(count) != len(ids) {
		return fmt.Errorf("one or more transformer ids not found in organization")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.queries.WithTx(tx)
	if err := q.DeleteSourceTransformers(ctx, sourceID); err != nil {
		return fmt.Errorf("clearing source transformers: %w", err)
	}
	for i, id := range ids {
		if err := q.InsertSourceTransformer(ctx, generated.InsertSourceTransformerParams{
			SourceID:      sourceID,
			TransformerID: id,
			Position:      int32(i),
		}); err != nil {
			return fmt.Errorf("inserting source transformer: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := ids[:0:0]
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
