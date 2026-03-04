-- +goose Up
-- Drop source_id column and its index from guardrails
ALTER TABLE guardrails DROP COLUMN source_id;

-- Add organization_id to guardrails
ALTER TABLE guardrails ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);
CREATE INDEX idx_guardrails_organization_id ON guardrails(organization_id);

-- Create many-to-many join table
CREATE TABLE source_guardrails (
    source_id    UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    guardrail_id UUID NOT NULL REFERENCES guardrails(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, guardrail_id)
);

-- +goose Down
DROP TABLE IF EXISTS source_guardrails;

DROP INDEX IF EXISTS idx_guardrails_organization_id;
ALTER TABLE guardrails DROP COLUMN organization_id;

ALTER TABLE guardrails ADD COLUMN source_id UUID NOT NULL REFERENCES sources(id);
CREATE INDEX idx_guardrails_source_id ON guardrails(source_id);
