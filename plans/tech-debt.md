# Tech Debt Tracker

Items logged here should be addressed before production. Each item includes context on why it was deferred and what the fix looks like.

---

## Active Items

### 1. Vendor Go Dependencies
- **Location**: `go.mod`, CI workflow
- **Issue**: CI downloads all dependencies on every run, slowing builds
- **Fix**: Run `go mod vendor` and check in the `vendor/` directory; update CI to use `go build -mod=vendor`
- **Risk**: LOW — performance improvement

### 2. OpenAI-Compatible Proxy Endpoint
- **Location**: `internal/dataplane/gateway/`, `internal/dataplane/processor/`
- **Issue**: The proxy wraps LLM responses in a custom `ProxyResult` envelope and returns 403 with full guard internals on rejection, so callers can't point an OpenAI SDK at Sheeld unmodified, rejection status conflates policy with authorization, and guard config details leak to callers
- **Fix**: Serve raw `/v1/chat/completions` responses on pass; return an OpenAI-style error object (400/422, `"type": "guardrail_rejection"`) with a minimal reason + request ID on rejection; full guard results stay in audit logs
- **Risk**: MEDIUM — breaking API change for existing callers

### 3. Per-Guard Error Policy (fail open/closed)
- **Location**: `internal/shared/guard/engine.go`
- **Issue**: A guard *error* (e.g. OpenAI Moderation API outage) is recorded as a failed result, so under the default `all` criteria an external dependency outage blocks all traffic
- **Fix**: Add `on_error: fail_open | fail_closed` to guardrail config; the engine treats errored fail-open guards as passed
- **Risk**: MEDIUM — availability vs safety tradeoff, needs per-guard defaults

### 4. Per-Phase Pass Criteria
- **Location**: `sources` table, `internal/dataplane/processor/`
- **Issue**: One `pass_criteria`/`pass_threshold` pair covers both input and output guard phases, which rarely have the same guard count — an `n_of_m` threshold valid for one phase can be unsatisfiable for the other
- **Fix**: Move criteria/threshold to per-phase settings and validate the threshold against the attached guard count
- **Risk**: MEDIUM — schema migration + API change

### 5. LLM API Key as a Write-Only Sub-Resource
- **Location**: `internal/controlplane/api/handler/source.go`, `openapi.yaml`
- **Issue**: `SourceInput` requires `llm_api_key` on every update, so renaming a source forces re-entering the provider secret
- **Fix**: Split the key into `PUT /v1/sources/{id}/credentials` (write-only) or make it optional-on-update with documented semantics
- **Risk**: LOW — additive API change

### 6. Data-Plane Config Disk Snapshot
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
