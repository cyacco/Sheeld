-- name: ListAllOrganizationIDs :many
SELECT id FROM organizations ORDER BY id;

-- name: ListAllActiveAPIKeys :many
SELECT organization_id, key_hash FROM api_keys WHERE revoked_at IS NULL ORDER BY organization_id;

-- name: ListAllSources :many
SELECT id, organization_id, route, llm_model, llm_api_key_enc, input_pass_criteria, input_pass_threshold, output_pass_criteria, output_pass_threshold, enabled
FROM sources ORDER BY organization_id, route;

-- name: ListAllEnabledGuardrails :many
SELECT id, organization_id, name, guard_type, phase, config, enabled
FROM guardrails WHERE enabled = true ORDER BY organization_id, id;

-- name: ListAllSourceGuardrails :many
SELECT source_id, guardrail_id FROM source_guardrails ORDER BY source_id;
