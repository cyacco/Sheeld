-- name: CreateAlertWebhook :one
INSERT INTO alert_webhooks (organization_id, name, url, payload_format, enabled)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAlertWebhooksByOrganization :many
SELECT * FROM alert_webhooks
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: GetAlertWebhook :one
SELECT * FROM alert_webhooks
WHERE id = $1 AND organization_id = $2;

-- name: UpdateAlertWebhook :one
UPDATE alert_webhooks
SET name = $3, url = $4, payload_format = $5, enabled = $6, updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: DeleteAlertWebhook :exec
DELETE FROM alert_webhooks
WHERE id = $1 AND organization_id = $2;

-- name: ListAllEnabledAlertWebhooks :many
SELECT organization_id, id, url, payload_format FROM alert_webhooks
WHERE enabled = true
ORDER BY organization_id;
