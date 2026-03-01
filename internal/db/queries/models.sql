-- name: ListModels :many
SELECT provider, id FROM models
ORDER BY provider, id;

-- name: UpsertModel :exec
INSERT INTO models (provider, id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeleteModelsByProvider :exec
DELETE FROM models
WHERE provider = $1;
