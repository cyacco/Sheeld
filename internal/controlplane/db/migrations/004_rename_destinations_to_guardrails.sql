-- +goose Up
ALTER TABLE destinations RENAME TO guardrails;
ALTER INDEX idx_destinations_source_id RENAME TO idx_guardrails_source_id;
ALTER TABLE guardrails DROP COLUMN priority;

-- +goose Down
ALTER TABLE guardrails ADD COLUMN priority integer NOT NULL DEFAULT 0;
ALTER INDEX idx_guardrails_source_id RENAME TO idx_destinations_source_id;
ALTER TABLE guardrails RENAME TO destinations;
