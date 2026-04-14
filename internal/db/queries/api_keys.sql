-- name: CreateAPIKey :one
INSERT INTO api_keys (organization_id, name, key_hash, key_prefix)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = $1 AND revoked_at IS NULL;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys
WHERE key_prefix = $1 AND revoked_at IS NULL
LIMIT 1;

-- name: ListAPIKeysByOrganization :many
SELECT * FROM api_keys
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys
SET revoked_at = now()
WHERE id = $1 AND organization_id = $2;
