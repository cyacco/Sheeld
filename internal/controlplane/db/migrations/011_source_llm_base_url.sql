-- +goose Up
-- Optional per-source LLM endpoint: when set, the data plane sends this
-- source's traffic directly to that OpenAI-compatible base URL instead of
-- the globally configured gateway.
ALTER TABLE sources ADD COLUMN llm_base_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sources DROP COLUMN llm_base_url;
