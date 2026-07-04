-- +goose Up

DROP TABLE IF EXISTS models;

-- +goose Down

CREATE TABLE models (
    provider   TEXT NOT NULL,
    id         TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, id)
);

CREATE INDEX idx_models_provider ON models(provider);
