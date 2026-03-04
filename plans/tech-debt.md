# Tech Debt Tracker

Items logged here should be addressed before production. Each item includes context on why it was deferred and what the fix looks like.

---

## Active Items

### 1. Vendor Go Dependencies
- **Location**: `go.mod`, CI workflow
- **Issue**: CI downloads all dependencies on every run, slowing builds
- **Fix**: Run `go mod vendor` and check in the `vendor/` directory; update CI to use `go build -mod=vendor`
- **Risk**: LOW — performance improvement

### 2. Decouple Guardrails from Sources
- **Location**: `internal/db/`, `internal/api/handler/`, `internal/service/guardrail.go`
- **Issue**: Guardrails are nested under sources (each guardrail belongs to exactly one source), preventing reuse across multiple sources
- **Fix**: Make guardrails a standalone resource with a many-to-many join table to sources
- **Risk**: MEDIUM — schema migration required, API breaking change

---

## Resolved Items

### 1. LLM API Key Encryption at Rest
- **Resolved in**: Phase 6 — `internal/crypto/aes.go` implements AES-256-GCM, integrated into source service and proxy

### 2. No Integration Tests
- **Resolved in**: PR #9 — `internal/integration/integration_test.go` with testcontainers-go, 30 test cases covering all API endpoints
