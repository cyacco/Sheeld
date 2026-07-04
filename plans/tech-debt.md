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

### 3. LLM API Key as a Write-Only Sub-Resource
- **Location**: `internal/controlplane/api/handler/source.go`, `openapi.yaml`
- **Issue**: `SourceInput` requires `llm_api_key` on every update, so renaming a source forces re-entering the provider secret
- **Fix**: Split the key into `PUT /v1/sources/{id}/credentials` (write-only) or make it optional-on-update with documented semantics
- **Risk**: LOW — additive API change

### 4. Data-Plane Config Disk Snapshot
- **Location**: `internal/dataplane/backendconfig/`
- **Issue**: If the data plane restarts while the control plane is down, it serves 503s until the control plane returns (no local config cache survives restarts)
- **Fix**: Persist the last-applied workspace config to disk (encrypted or with keys stripped + refetch) and load it at startup as a fallback
- **Risk**: LOW — resilience improvement; compose restarts both services together today

---

## Resolved Items

### 1. LLM API Key Encryption at Rest
- **Resolved in**: Phase 6 — `internal/controlplane/crypto/aes.go` implements AES-256-GCM, integrated into source service and the workspace-config builder

### 2. No Integration Tests
- **Resolved in**: PR #9 — `internal/integration/integration_test.go` with testcontainers-go, 30 test cases covering all API endpoints (reworked for the control/data plane split)

### 3. Decouple Guardrails from Sources
- **Resolved in**: Migration 006 — guardrails are org-level with a `source_guardrails` many-to-many join

### 4. OpenAI-Compatible Proxy Endpoint
- **Resolved in**: api-fixes branch — proxy returns raw chat completions on pass, 422 OpenAI-style `guardrail_rejection` errors on rejection, with a /chat/completions alias for SDK base_url compatibility

### 5. Per-Guard Error Policy (fail open/closed)
- **Resolved in**: api-fixes branch — `on_error: fail_open` in guardrail config wraps the guard so execution errors count as passed (marked `errored` in audit results); default remains fail_closed
