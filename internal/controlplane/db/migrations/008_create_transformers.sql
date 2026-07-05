-- +goose Up

-- Transformers ("Transformations" in the UI): sequential input rewriters
-- that run before input guards and the LLM call. Separate entity from
-- guardrails, mirroring rudderstack's Sources/Destinations/Transformations.
CREATE TABLE transformers (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    transformer_type TEXT NOT NULL,
    -- Input-only in v1; the column exists for forward compatibility.
    phase            TEXT NOT NULL DEFAULT 'input' CHECK (phase = 'input'),
    config           JSONB NOT NULL DEFAULT '{}',
    enabled          BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transformers_org ON transformers(organization_id);

-- Per-source ordered attachment. Position is 0-based; ties broken by
-- transformer_id. Reorder rewrites all rows for a source transactionally.
CREATE TABLE source_transformers (
    source_id      UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    transformer_id UUID NOT NULL REFERENCES transformers(id) ON DELETE CASCADE,
    position       INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, transformer_id)
);

CREATE INDEX idx_source_transformers_source ON source_transformers(source_id, position);

-- +goose Down

DROP TABLE IF EXISTS source_transformers;
DROP TABLE IF EXISTS transformers;
