-- name: CreateDestination :one
INSERT INTO destinations (source_id, name, guard_type, phase, config, priority, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetDestination :one
SELECT * FROM destinations
WHERE id = $1 AND source_id = $2;

-- name: ListDestinationsBySource :many
SELECT * FROM destinations
WHERE source_id = $1
ORDER BY priority ASC, created_at ASC;

-- name: ListEnabledDestinationsBySource :many
SELECT * FROM destinations
WHERE source_id = $1 AND enabled = true
ORDER BY priority ASC, created_at ASC;

-- name: UpdateDestination :one
UPDATE destinations
SET
    name = $3,
    guard_type = $4,
    phase = $5,
    config = $6,
    priority = $7,
    enabled = $8,
    updated_at = now()
WHERE id = $1 AND source_id = $2
RETURNING *;

-- name: DeleteDestination :exec
DELETE FROM destinations
WHERE id = $1 AND source_id = $2;
