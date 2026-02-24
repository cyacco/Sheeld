-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    organization_id, source_id, input_hash,
    guard_results, overall_result, latency_ms
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAuditLogsByOrganization :many
SELECT * FROM audit_logs
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsBySource :many
SELECT * FROM audit_logs
WHERE source_id = $1 AND organization_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
