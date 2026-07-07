# Tech Debt Tracker

Items logged here should be addressed before production. Each item includes context on why it was deferred and what the fix looks like.

---

## Active Items

### 1. Vendor Go Dependencies
- **Location**: `go.mod`, CI workflow
- **Issue**: CI downloads all dependencies on every run, slowing builds
- **Fix**: Run `go mod vendor` and check in the `vendor/` directory; update CI to use `go build -mod=vendor`
- **Risk**: LOW — performance improvement


---

## Resolved Items

### Security & Tenancy Hardening
- **Resolved in**: security-tenancy-hardening branch — fixed cross-org IDOR on guardrail/transformer detach & list (org ownership now validated, 404 on mismatch); redacted provider secrets (`api_key`, `headers`, `*_key`) from config in API responses with round-trip-safe update; added SSRF protection (`urlpolicy.ValidatePublicHTTPURL` rejects private/loopback/link-local, opt-out `*_ALLOW_PRIVATE_GUARD_URLS`); trimmed API-key list response to non-secret fields; added per-org rate limiting to control-plane JWT routes

### Per-Phase Pass Criteria
- **Resolved in**: per-phase-pass-criteria branch — migration 010 renames to `input_pass_criteria`/`input_pass_threshold` and adds `output_pass_criteria`/`output_pass_threshold` (existing values copied to both phases); processor evaluates each phase with its own criteria; API/UI expose both pairs; `n_of_m` validates threshold >= 1 per phase (422). Validating thresholds against live attachment counts was deliberately skipped — attachments change after source creation, and an unsatisfiable `n_of_m` fails closed, which is the safe behavior

### LLM API Key Optional on Update
- **Resolved**: the handler/service already kept the stored key on empty update (PR #31); the ops-papercuts branch fixed the stale openapi spec (`llm_api_key` no longer in `required`, semantics documented)

### Data-Plane Config Disk Snapshot
- **Resolved in**: ops-papercuts branch — opt-in via `SHEELD_DP_CONFIG_SNAPSHOT_PATH` + `SHEELD_DP_CONFIG_SNAPSHOT_KEY`; each applied config is AES-256-GCM encrypted and atomically written 0600; `WaitForInitial` falls back to the snapshot when the control plane is unreachable at startup

### Render `transforms` Audit Key in Events UI
- **Resolved in**: presidio-guard-audit-ui branch — expanded audit rows show input/output transformation chains (per-step changed/errored/skipped badges and durations) in pipeline order alongside guard results

### Guard Config Validation at Create Time
- **Resolved in**: guard-config-validation branch — `GuardrailService` now instantiates the guard through `guard.Registry` on create/update; unknown types and invalid configs return 422 (parity with transformers)

### 1. LLM API Key Encryption at Rest
- **Resolved in**: Phase 6 — `internal/controlplane/crypto/aes.go` implements AES-256-GCM, integrated into source service and the workspace-config builder

### 2. No Integration Tests
- **Resolved in**: PR #9 — `internal/integration/integration_test.go` with testcontainers-go, 30 test cases covering all API endpoints (reworked for the control/data plane split)

### 3. Decouple Guardrails from Sources
- **Resolved in**: Migration 006 — guardrails are org-level with a `source_guardrails` many-to-many join

### 4. OpenAI-Compatible Proxy Endpoint
- **Resolved in**: api-fixes branch — proxy returns raw chat completions on pass, 422 OpenAI-style `guardrail_rejection` errors on rejection, with a /chat/completions alias for SDK base_url compatibility

### 5. Per-Guard Error Policy (fail open/closed)
- **Resolved in**: api-fixes branch — `on_error: fail_open` in guardrail config wraps the guard so execution errors count as passed (marked `errored` in audit results); default remains fail_closed. The webhook-guard branch later removed guardrails_ai's redundant bespoke `fail_open` field in favor of this mechanism.

### 6. Self-Hosted Integrations Limited to guardrails.ai
- **Resolved in**: webhook-guard branch — generic `webhook` guard type POSTs to any http(s) endpoint with a documented contract and optional static auth headers
