-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    organization_id, source_id, input_hash,
    guard_results, overall_result, latency_ms,
    prompt_tokens, completion_tokens, total_tokens, model
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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

-- name: AuditSummary :one
-- Totals over a window for one org.
SELECT
    COUNT(*)::bigint AS total_requests,
    COUNT(*) FILTER (WHERE overall_result = 'pass')::bigint AS passed,
    COALESCE(SUM(prompt_tokens), 0)::bigint AS prompt_tokens,
    COALESCE(SUM(completion_tokens), 0)::bigint AS completion_tokens,
    COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens
FROM audit_logs
WHERE organization_id = $1 AND created_at >= $2;

-- name: AuditDailySeries :many
-- Per-day request and token totals for the usage chart.
SELECT
    date_trunc('day', created_at)::timestamptz AS day,
    COUNT(*)::bigint AS requests,
    COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens
FROM audit_logs
WHERE organization_id = $1 AND created_at >= $2
GROUP BY day
ORDER BY day;

-- name: AuditByModel :many
-- Request and token totals grouped by model (rows with a recorded model).
SELECT
    model,
    COUNT(*)::bigint AS requests,
    COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens
FROM audit_logs
WHERE organization_id = $1 AND created_at >= $2 AND model IS NOT NULL
GROUP BY model
ORDER BY total_tokens DESC;

-- name: AuditBySource :many
-- Request, rejection, and token totals grouped by source.
SELECT
    source_id,
    COUNT(*)::bigint AS requests,
    COUNT(*) FILTER (WHERE overall_result <> 'pass')::bigint AS rejected,
    COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens
FROM audit_logs
WHERE organization_id = $1 AND created_at >= $2
GROUP BY source_id
ORDER BY requests DESC;

-- name: DeleteAuditLogsBefore :execrows
-- Deletes up to $2 audit rows older than $1. Batched via ctid so a large
-- backlog is cleared incrementally instead of one table-locking statement.
DELETE FROM audit_logs
WHERE ctid IN (
    SELECT al.ctid FROM audit_logs al
    WHERE al.created_at < $1
    LIMIT $2
);
