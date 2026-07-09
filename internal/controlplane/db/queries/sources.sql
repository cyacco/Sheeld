-- name: CreateSource :one
INSERT INTO sources (
    organization_id, name, route, description,
    llm_provider, llm_model, llm_api_key_enc, llm_base_url,
    input_pass_criteria, input_pass_threshold,
    output_pass_criteria, output_pass_threshold, enabled
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetSource :one
SELECT * FROM sources
WHERE id = $1 AND organization_id = $2;

-- name: GetSourceByRoute :one
SELECT * FROM sources
WHERE route = $1 AND organization_id = $2;

-- name: ListSourcesByOrganization :many
SELECT * FROM sources
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: UpdateSource :one
UPDATE sources
SET
    name = $3,
    route = $4,
    description = $5,
    llm_provider = $6,
    llm_model = $7,
    llm_api_key_enc = $8,
    llm_base_url = $9,
    input_pass_criteria = $10,
    input_pass_threshold = $11,
    output_pass_criteria = $12,
    output_pass_threshold = $13,
    enabled = $14,
    updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: DeleteSource :exec
DELETE FROM sources
WHERE id = $1 AND organization_id = $2;
