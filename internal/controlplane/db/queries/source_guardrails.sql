-- name: AttachGuardrailToSource :exec
INSERT INTO source_guardrails (source_id, guardrail_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DetachGuardrailFromSource :exec
DELETE FROM source_guardrails
WHERE source_id = $1 AND guardrail_id = $2;

-- name: ListSourcesByGuardrail :many
SELECT s.* FROM sources s
JOIN source_guardrails sg ON sg.source_id = s.id
WHERE sg.guardrail_id = $1
ORDER BY s.created_at ASC;
