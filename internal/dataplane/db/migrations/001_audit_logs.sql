-- +goose Up

-- Audit log (request history). Organization and source ids are opaque
-- references to control-plane entities — no FKs, the config is the source
-- of truth.
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    source_id       UUID NOT NULL,
    input_hash      TEXT,
    guard_results   JSONB NOT NULL,
    overall_result  TEXT NOT NULL,
    latency_ms      INT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_organization_id ON audit_logs(organization_id);
CREATE INDEX idx_audit_logs_source_id ON audit_logs(source_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);

-- +goose Down

DROP TABLE IF EXISTS audit_logs;
