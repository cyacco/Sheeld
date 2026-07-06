package workspaceconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/crypto"
	"github.com/sheeld/sheeld/internal/shared/domain"
)

// Builder assembles the workspace-config payload served to data planes.
type Builder struct {
	queries       *generated.Queries
	encryptionKey string
}

// NewBuilder creates a workspace-config builder.
func NewBuilder(queries *generated.Queries, encryptionKey string) *Builder {
	return &Builder{queries: queries, encryptionKey: encryptionKey}
}

// Build assembles the full config for all organizations, decrypting LLM API
// keys. The returned config's Version is the sha256 of the payload with
// Version and GeneratedAt zeroed, so identical config yields identical
// versions across rebuilds.
func (b *Builder) Build(ctx context.Context) (*domain.WorkspaceConfig, error) {
	orgIDs, err := b.queries.ListAllOrganizationIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	apiKeys, err := b.queries.ListAllActiveAPIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	sources, err := b.queries.ListAllSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	guardrails, err := b.queries.ListAllEnabledGuardrails(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing guardrails: %w", err)
	}
	attachments, err := b.queries.ListAllSourceGuardrails(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing source guardrails: %w", err)
	}
	transformers, err := b.queries.ListAllEnabledTransformers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing transformers: %w", err)
	}
	// Ordered by (source_id, position, transformer_id): appending preserves
	// the per-source chain order the data plane must run.
	transformerAttachments, err := b.queries.ListAllSourceTransformers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing source transformers: %w", err)
	}

	guardrailsBySource := make(map[uuid.UUID][]uuid.UUID)
	for _, a := range attachments {
		guardrailsBySource[a.SourceID] = append(guardrailsBySource[a.SourceID], a.GuardrailID)
	}

	transformersBySource := make(map[uuid.UUID][]uuid.UUID)
	for _, a := range transformerAttachments {
		transformersBySource[a.SourceID] = append(transformersBySource[a.SourceID], a.TransformerID)
	}

	orgs := make(map[uuid.UUID]*domain.OrgConfig, len(orgIDs))
	ordered := make([]uuid.UUID, 0, len(orgIDs))
	for _, id := range orgIDs {
		orgs[id] = &domain.OrgConfig{
			ID:           id,
			APIKeys:      []domain.APIKeyConfig{},
			Sources:      []domain.SourceConfig{},
			Guardrails:   []domain.GuardrailConfig{},
			Transformers: []domain.TransformerConfig{},
		}
		ordered = append(ordered, id)
	}

	for _, k := range apiKeys {
		if org, ok := orgs[k.OrganizationID]; ok {
			org.APIKeys = append(org.APIKeys, domain.APIKeyConfig{KeyHash: k.KeyHash})
		}
	}

	for _, s := range sources {
		org, ok := orgs[s.OrganizationID]
		if !ok {
			continue
		}
		apiKey, err := crypto.Decrypt(s.LlmApiKeyEnc, b.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting LLM key for source %s: %w", s.ID, err)
		}
		src := domain.SourceConfig{
			ID:                 s.ID,
			Route:              s.Route,
			Enabled:            s.Enabled,
			LLMModel:           s.LlmModel,
			LLMAPIKey:          apiKey,
			InputPassCriteria:  domain.PassCriteria(s.InputPassCriteria),
			OutputPassCriteria: domain.PassCriteria(s.OutputPassCriteria),
			GuardrailIDs:       guardrailsBySource[s.ID],
		}
		if src.GuardrailIDs == nil {
			src.GuardrailIDs = []uuid.UUID{}
		}
		src.TransformerIDs = transformersBySource[s.ID]
		if src.TransformerIDs == nil {
			src.TransformerIDs = []uuid.UUID{}
		}
		if s.InputPassThreshold.Valid {
			t := int(s.InputPassThreshold.Int32)
			src.InputPassThreshold = &t
		}
		if s.OutputPassThreshold.Valid {
			t := int(s.OutputPassThreshold.Int32)
			src.OutputPassThreshold = &t
		}
		org.Sources = append(org.Sources, src)
	}

	for _, g := range guardrails {
		if org, ok := orgs[g.OrganizationID]; ok {
			org.Guardrails = append(org.Guardrails, domain.GuardrailConfig{
				ID:        g.ID,
				Name:      g.Name,
				GuardType: domain.GuardType(g.GuardType),
				Phase:     domain.GuardPhase(g.Phase),
				Config:    g.Config,
			})
		}
	}

	for _, t := range transformers {
		if org, ok := orgs[t.OrganizationID]; ok {
			org.Transformers = append(org.Transformers, domain.TransformerConfig{
				ID:              t.ID,
				Name:            t.Name,
				TransformerType: t.TransformerType,
				Phase:           t.Phase,
				Config:          t.Config,
			})
		}
	}

	cfg := &domain.WorkspaceConfig{
		Organizations: make([]domain.OrgConfig, 0, len(ordered)),
	}
	for _, id := range ordered {
		cfg.Organizations = append(cfg.Organizations, *orgs[id])
	}

	version, err := computeVersion(cfg)
	if err != nil {
		return nil, fmt.Errorf("computing config version: %w", err)
	}
	cfg.Version = version
	cfg.GeneratedAt = time.Now().UTC()
	return cfg, nil
}

// computeVersion hashes the payload with Version and GeneratedAt zeroed.
func computeVersion(cfg *domain.WorkspaceConfig) (string, error) {
	canonical := *cfg
	canonical.Version = ""
	canonical.GeneratedAt = time.Time{}
	data, err := json.Marshal(&canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
