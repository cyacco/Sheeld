-- name: CreateTransformer :one
INSERT INTO transformers (organization_id, name, transformer_type, phase, config, enabled)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTransformer :one
SELECT * FROM transformers WHERE id = $1 AND organization_id = $2;

-- name: ListTransformersByOrg :many
SELECT * FROM transformers WHERE organization_id = $1 ORDER BY created_at ASC;

-- name: UpdateTransformer :one
UPDATE transformers
SET name = $3, transformer_type = $4, phase = $5, config = $6, enabled = $7, updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: DeleteTransformer :exec
DELETE FROM transformers WHERE id = $1 AND organization_id = $2;

-- name: AttachTransformerToSource :exec
INSERT INTO source_transformers (source_id, transformer_id, position)
SELECT $1, $2, COALESCE(MAX(position) + 1, 0)
FROM source_transformers WHERE source_id = $1
ON CONFLICT (source_id, transformer_id) DO NOTHING;

-- name: DetachTransformerFromSource :exec
DELETE FROM source_transformers WHERE source_id = $1 AND transformer_id = $2;

-- name: DeleteSourceTransformers :exec
DELETE FROM source_transformers WHERE source_id = $1;

-- name: InsertSourceTransformer :exec
INSERT INTO source_transformers (source_id, transformer_id, position)
VALUES ($1, $2, $3);

-- name: ListTransformersBySource :many
SELECT t.*, st.position FROM transformers t
JOIN source_transformers st ON st.transformer_id = t.id
WHERE st.source_id = $1
ORDER BY st.position, st.transformer_id;

-- name: CountTransformersByOrgAndIDs :one
SELECT COUNT(*) FROM transformers
WHERE organization_id = $1 AND id = ANY($2::uuid[]);

-- name: ListAllEnabledTransformers :many
SELECT id, organization_id, name, transformer_type, phase, config
FROM transformers WHERE enabled = true ORDER BY organization_id, id;

-- name: ListAllSourceTransformers :many
SELECT source_id, transformer_id FROM source_transformers
ORDER BY source_id, position, transformer_id;
