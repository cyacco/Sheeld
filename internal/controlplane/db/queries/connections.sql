-- name: ListSourceGuardrailsByOrg :many
SELECT sg.source_id, sg.guardrail_id
FROM source_guardrails sg
JOIN sources s ON s.id = sg.source_id
WHERE s.organization_id = $1
ORDER BY sg.source_id;
