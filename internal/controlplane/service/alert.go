package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/controlplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/urlpolicy"
)

// AlertService manages org-level rejection-alert webhooks.
type AlertService struct {
	queries *generated.Queries
}

// NewAlertService creates a new AlertService.
func NewAlertService(queries *generated.Queries) *AlertService {
	return &AlertService{queries: queries}
}

// AlertWebhookParams are the caller-supplied fields for create/update.
type AlertWebhookParams struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	PayloadFormat string `json:"payload_format"`
	Enabled       *bool  `json:"enabled"`
}

func (p *AlertWebhookParams) validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if err := urlpolicy.ValidatePublicHTTPURL(p.URL, "url"); err != nil {
		return err
	}
	switch p.PayloadFormat {
	case "":
		p.PayloadFormat = "json"
	case "json", "slack":
	default:
		return fmt.Errorf("payload_format must be \"json\" or \"slack\"")
	}
	return nil
}

// Create validates and stores a new alert webhook for the org.
func (s *AlertService) Create(ctx context.Context, orgID uuid.UUID, p AlertWebhookParams) (*generated.AlertWebhook, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	w, err := s.queries.CreateAlertWebhook(ctx, generated.CreateAlertWebhookParams{
		OrganizationID: orgID,
		Name:           p.Name,
		Url:            p.URL,
		PayloadFormat:  p.PayloadFormat,
		Enabled:        enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("creating alert webhook: %w", err)
	}
	return &w, nil
}

// List returns the org's alert webhooks.
func (s *AlertService) List(ctx context.Context, orgID uuid.UUID) ([]generated.AlertWebhook, error) {
	return s.queries.ListAlertWebhooksByOrganization(ctx, orgID)
}

// Update validates and replaces an alert webhook's fields (org-scoped).
func (s *AlertService) Update(ctx context.Context, orgID, id uuid.UUID, p AlertWebhookParams) (*generated.AlertWebhook, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	existing, err := s.queries.GetAlertWebhook(ctx, generated.GetAlertWebhookParams{ID: id, OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("alert webhook not found")
	}
	enabled := existing.Enabled
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	w, err := s.queries.UpdateAlertWebhook(ctx, generated.UpdateAlertWebhookParams{
		ID:             id,
		OrganizationID: orgID,
		Name:           p.Name,
		Url:            p.URL,
		PayloadFormat:  p.PayloadFormat,
		Enabled:        enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("updating alert webhook: %w", err)
	}
	return &w, nil
}

// Delete removes an alert webhook (org-scoped).
func (s *AlertService) Delete(ctx context.Context, orgID, id uuid.UUID) error {
	return s.queries.DeleteAlertWebhook(ctx, generated.DeleteAlertWebhookParams{ID: id, OrganizationID: orgID})
}
