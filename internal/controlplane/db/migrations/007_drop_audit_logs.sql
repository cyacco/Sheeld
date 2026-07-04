-- +goose Up

-- Audit logs now live in the data plane's database (see
-- internal/dataplane/db/migrations). To keep existing rows, dump them
-- before applying this migration:
--   pg_dump -t audit_logs <control-plane-db-url> | psql <data-plane-db-url>
DROP TABLE IF EXISTS audit_logs;

-- +goose Down

CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    source_id       UUID NOT NULL REFERENCES sources(id),
    input_hash      TEXT,
    guard_results   JSONB NOT NULL,
    overall_result  TEXT NOT NULL,
    latency_ms      INT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_organization_id ON audit_logs(organization_id);
CREATE INDEX idx_audit_logs_source_id ON audit_logs(source_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
