-- +goose Up
-- Org-level rejection alert webhooks: the data plane POSTs to url whenever a
-- request is rejected by guards. payload_format selects the body shape
-- ("json" = generic JSON, "slack" = Slack-compatible {"text": ...}).
CREATE TABLE alert_webhooks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name            TEXT NOT NULL,
    url             TEXT NOT NULL,
    payload_format  TEXT NOT NULL DEFAULT 'json',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_alert_webhooks_organization_id ON alert_webhooks(organization_id);

-- +goose Down
DROP TABLE IF EXISTS alert_webhooks;
