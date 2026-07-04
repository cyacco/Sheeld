-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING *;

-- name: GetOrganization :one
SELECT * FROM organizations
WHERE id = $1;
