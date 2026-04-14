-- name: CreateGuardrail :one
INSERT INTO guardrails (organization_id, name, guard_type, phase, config, enabled)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetGuardrail :one
SELECT * FROM guardrails
WHERE id = $1 AND organization_id = $2;

-- name: ListGuardrailsByOrg :many
SELECT * FROM guardrails
WHERE organization_id = $1
ORDER BY created_at ASC;

-- name: ListGuardrailsBySource :many
SELECT g.* FROM guardrails g
JOIN source_guardrails sg ON sg.guardrail_id = g.id
WHERE sg.source_id = $1
ORDER BY g.created_at ASC;

-- name: ListEnabledGuardrailsBySource :many
SELECT g.* FROM guardrails g
JOIN source_guardrails sg ON sg.guardrail_id = g.id
WHERE sg.source_id = $1 AND g.organization_id = $2 AND g.enabled = true
ORDER BY g.created_at ASC;

-- name: UpdateGuardrail :one
UPDATE guardrails
SET
    name = $3,
    guard_type = $4,
    phase = $5,
    config = $6,
    enabled = $7,
    updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: DeleteGuardrail :exec
DELETE FROM guardrails
WHERE id = $1 AND organization_id = $2;
