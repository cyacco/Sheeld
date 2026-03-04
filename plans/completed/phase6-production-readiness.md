# Phase 6: Production Readiness

**Status**: Pending
**Depends on**: All previous phases

## Goal
Make Sheeld deployable, observable, and secure for production use.

## Tasks

### 1. LLM API Key Encryption at Rest (Tech Debt #1)
- Implement AES-256-GCM encryption/decryption in `internal/service/source.go`
- Use `SHEELD_ENCRYPTION_KEY` (hex-encoded 32 bytes)
- Encrypt on source Create/Update, decrypt on proxy read
- Migrate any existing plaintext keys

### 2. Multi-stage Dockerfile
- Already exists from Phase 1 — review and harden
- Ensure non-root user, minimal base image (alpine)
- Add health check instruction

### 3. Docker Compose — full stack
- Go API + PostgreSQL + Next.js dashboard
- Nginx reverse proxy (API on `/api`, dashboard on `/`)
- Volume mounts for DB persistence
- Environment variable templates

### 4. Health check endpoint
- `GET /healthz` already exists — enhance with DB connectivity check
- Return `{"status": "ok", "db": "connected"}` or appropriate error

### 5. Structured logging with request tracing
- Ensure all log entries include request ID
- Add trace ID propagation for proxy → guard → LLM calls
- Log guard execution times, LLM latency, overall latency

### 6. Rate limiting middleware
- Per-API-key rate limiting using token bucket or sliding window
- Configurable via environment variables (e.g., `SHEELD_RATE_LIMIT_RPS=100`)
- Return `429 Too Many Requests` with `Retry-After` header
- Consider using Redis for distributed rate limiting (or in-memory for single-instance)

### 7. Graceful shutdown
- Already implemented in `main.go` — verify it drains in-flight proxy requests
- Ensure DB connections are properly closed
- Configurable shutdown timeout

### 8. Input size limits + request timeouts
- Max request body size middleware (e.g., 1MB default)
- Per-route timeouts (proxy route: 60s, CRUD routes: 10s)
- Guard execution timeout (configurable per destination)

### 9. CI pipeline (GitHub Actions)
- Workflow: lint → test → build → Docker image
- Run on push to main and PRs
- Steps:
  - `gofmt -l .` (fail if not formatted)
  - `go vet ./...`
  - `go test ./...`
  - `go build ./...`
  - `docker build .`
  - For dashboard: `npm ci && npm run build`

### 10. OpenAPI spec generation
- Generate OpenAPI 3.0 spec from route definitions
- Options: hand-written YAML or use `swaggo/swag` annotations
- Serve at `/v1/docs` or `/openapi.yaml`

## Files to Create/Modify
- `internal/service/crypto.go` — AES-256-GCM encrypt/decrypt helpers
- `internal/api/middleware/ratelimit.go` — Rate limiting middleware
- `internal/api/middleware/bodysize.go` — Request body size limiter
- `.github/workflows/ci.yml` — GitHub Actions CI pipeline
- `nginx.conf` — Reverse proxy config (if using Nginx)
- Update `docker-compose.yaml` — Add dashboard + proxy services
- Update `Dockerfile` — Harden for production
- `openapi.yaml` — API specification

## Key Considerations
- Encryption key rotation strategy (future: support multiple keys with key ID)
- Rate limiting should be tested under load
- CI should run the full test suite including guard tests
- Consider adding `golangci-lint` for comprehensive linting
- Dashboard build should be part of the Docker image or served separately
