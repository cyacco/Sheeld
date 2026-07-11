-- +goose Up
-- Token usage + model for each proxied request. NULL when no LLM call was made
-- (e.g. a request rejected by input guards before reaching the provider).
ALTER TABLE audit_logs ADD COLUMN prompt_tokens INT;
ALTER TABLE audit_logs ADD COLUMN completion_tokens INT;
ALTER TABLE audit_logs ADD COLUMN total_tokens INT;
ALTER TABLE audit_logs ADD COLUMN model TEXT;

-- +goose Down
ALTER TABLE audit_logs DROP COLUMN model;
ALTER TABLE audit_logs DROP COLUMN total_tokens;
ALTER TABLE audit_logs DROP COLUMN completion_tokens;
ALTER TABLE audit_logs DROP COLUMN prompt_tokens;
