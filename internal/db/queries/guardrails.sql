-- name: CreateGuardrail :one
INSERT INTO guardrails (source_id, name, guard_type, phase, config, enabled)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetGuardrail :one
SELECT * FROM guardrails
WHERE id = $1 AND source_id = $2;

-- name: ListGuardrailsBySource :many
SELECT * FROM guardrails
WHERE source_id = $1
ORDER BY created_at ASC;

-- name: ListEnabledGuardrailsBySource :many
SELECT * FROM guardrails
WHERE source_id = $1 AND enabled = true
ORDER BY created_at ASC;

-- name: UpdateGuardrail :one
UPDATE guardrails
SET
    name = $3,
    guard_type = $4,
    phase = $5,
    config = $6,
    enabled = $7,
    updated_at = now()
WHERE id = $1 AND source_id = $2
RETURNING *;

-- name: DeleteGuardrail :exec
DELETE FROM guardrails
WHERE id = $1 AND source_id = $2;
