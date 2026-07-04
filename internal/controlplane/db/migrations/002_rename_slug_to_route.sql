-- +goose Up

ALTER TABLE sources RENAME COLUMN slug TO route;
DROP INDEX idx_sources_slug;
CREATE INDEX idx_sources_route ON sources(organization_id, route);
ALTER TABLE sources DROP CONSTRAINT sources_organization_id_slug_key;
ALTER TABLE sources ADD CONSTRAINT sources_organization_id_route_key UNIQUE(organization_id, route);

-- +goose Down

ALTER TABLE sources DROP CONSTRAINT sources_organization_id_route_key;
ALTER TABLE sources ADD CONSTRAINT sources_organization_id_slug_key UNIQUE(organization_id, slug);
DROP INDEX idx_sources_route;
CREATE INDEX idx_sources_slug ON sources(organization_id, slug);
ALTER TABLE sources RENAME COLUMN route TO slug;
