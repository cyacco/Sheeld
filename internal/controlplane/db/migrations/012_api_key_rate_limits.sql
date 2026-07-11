-- +goose Up
-- Optional per-API-key rate limits. NULL means "use the data plane's default".
ALTER TABLE api_keys ADD COLUMN rate_limit_rps DOUBLE PRECISION;
ALTER TABLE api_keys ADD COLUMN rate_limit_burst INTEGER;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN rate_limit_burst;
ALTER TABLE api_keys DROP COLUMN rate_limit_rps;
