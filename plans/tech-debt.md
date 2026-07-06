# Tech Debt Tracker

Items logged here should be addressed before production. Each item includes context on why it was deferred and what the fix looks like.

---

## Active Items

### 1. Vendor Go Dependencies
- **Location**: `go.mod`, CI workflow
- **Issue**: CI downloads all dependencies on every run, slowing builds
- **Fix**: Run `go mod vendor` and check in the `vendor/` directory; update CI to use `go build -mod=vendor`
- **Risk**: LOW — performance improvement

### 2. Per-Phase Pass Criteria
- **Location**: `sources` table, `internal/dataplane/processor/`
- **Issue**: One `pass_criteria`/`pass_threshold` pair covers both input and output guard phases, which rarely have the same guard count — an `n_of_m` threshold valid for one phase can be unsatisfiable for the other
- **Fix**: Move criteria/threshold to per-phase settings and validate the threshold against the attached guard count
- **Risk**: MEDIUM — schema migration + API change


---

## Resolved Items

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
