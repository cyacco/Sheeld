# Sheeld

## Project Overview

Sheeld is a "Segment for LLM guardrails" — a full LLM proxy that validates input, proxies LLM calls, and validates output. Licensed under Apache 2.0.

**Architecture** (rudder-server style control/data plane split):
- **Control plane** (`cmd/control-plane`, :8080): user auth, source/guardrail CRUD, dashboard backend, workspace-config endpoint. Owns cp-db (users, orgs, config).
- **Data plane** (`cmd/sheeld-server`, :8081): the proxy. Polls the control plane for workspace config (~5s, ETag), holds it in memory, runs input guards → LLM (any OpenAI-compatible endpoint; per-source llm_base_url or the default gateway) → output guards. Owns dp-db (audit logs). No control-plane or DB access on the request path.
- The config payload carries plaintext LLM keys (control plane decrypts before serving) — never log it; TLS between planes outside compose.

## Development Setup

```bash
# Build
go build ./...

# Test
go test ./...

# Run locally (requires PostgreSQL)
docker compose up cp-db dp-db -d

# Control plane
export SHEELD_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5432/sheeld?sslmode=disable"
export SHEELD_JWT_SECRET="dev-secret"
export SHEELD_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
export SHEELD_DATAPLANE_TOKEN="dev-dataplane-token"
export SHEELD_DATAPLANE_URL="http://localhost:8081"
go run ./cmd/control-plane

# Data plane (second shell)
export SHEELD_DP_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5433/sheeld?sslmode=disable"
export SHEELD_DP_CONTROL_PLANE_URL="http://localhost:8080"
export SHEELD_DP_ALLOW_INSECURE_CP=true
export SHEELD_DP_TOKEN="dev-dataplane-token"
go run ./cmd/sheeld-server

# Run full stack
docker compose up
```

## Repository Structure

```
sheeld/
├── cmd/
│   ├── control-plane/       # Control-plane entrypoint
│   └── sheeld-server/       # Data-plane entrypoint
├── internal/
│   ├── controlplane/
│   │   ├── api/             # chi router, CRUD handlers, JWT/DP-token middleware
│   │   ├── service/         # Business logic (auth, source, guardrail)
│   │   ├── db/              # goose migrations + sqlc (DO NOT EDIT generated/)
│   │   ├── crypto/          # AES-256-GCM for LLM keys at rest
│   │   ├── config/          # envconfig (SHEELD_ prefix)
│   │   └── workspaceconfig/ # Builds + serves the config payload (ETag/304)
│   ├── dataplane/
│   │   ├── gateway/         # HTTP layer: in-memory API-key auth, proxy route
│   │   ├── processor/       # Proxy stages: input guards → LLM → output guards
│   │   ├── backendconfig/   # Config poller + atomic in-memory store
│   │   ├── auditstore/      # Async batched audit writer + query handler
│   │   ├── db/              # goose migrations + sqlc for audit logs
│   │   └── config/          # envconfig (SHEELD_DP_ prefix)
│   └── shared/
│       ├── guard/           # Guard engine + implementations (fan-out)
│       ├── transform/       # Transformer pipeline (sequential input rewriters)
│       ├── llm/             # OpenAI-compatible chat-completions client
│       ├── domain/          # Core domain + workspace-config types
│       ├── middleware/      # request ID, logging, rate limit, body size
│       └── response/        # JSON response helpers
├── plans/                   # Implementation plans
│   ├── active/              # Current phase plans
│   ├── completed/           # Finished phase plans
│   └── tech-debt.md         # Tech debt tracker
├── web/                     # Next.js dashboard (Phase 5)
├── docker-compose.yaml
├── Dockerfile
└── sqlc.yaml
```

## Common Commands

| Command | Description |
|---------|-------------|
| `go build ./...` | Build all packages |
| `go test ./...` | Run all tests |
| `go vet ./...` | Run static analysis |
| `gofmt -w .` | Format all code |
| `~/go/bin/sqlc generate` | Regenerate sqlc code after query changes (both planes) |
| `go test -tags integration ./internal/integration/` | Integration tests (requires Docker) |
| `docker compose up` | Start full stack (both planes + DBs + mock-llm + web) |
| `docker compose up cp-db dp-db -d` | Start only the databases |

## Key Tooling

| Tool | Purpose |
|------|---------|
| **chi** | HTTP router (lightweight, idiomatic) |
| **pgx** | PostgreSQL driver |
| **sqlc** | SQL → type-safe Go code generation |
| **goose** | Database migrations |
| **envconfig** | Environment variable config (SHEELD_ control plane, SHEELD_DP_ data plane) |
| **slog** | Structured logging (stdlib) |

## Code Style

- Follow standard Go conventions and `gofmt` formatting
- Use `go vet` to catch common mistakes
- Write table-driven tests where applicable
- Keep packages focused and cohesive
- sqlc generated code in `internal/{controlplane,dataplane}/db/generated/` is auto-generated — never edit manually

## Database

Two PostgreSQL databases, each with its own goose migrations:

**Control plane** (`internal/controlplane/db/migrations/`):
- `organizations` — multi-tenant orgs
- `users` — org members
- `api_keys` — machine-to-machine auth (SHA-256 hashed); optional per-key `rate_limit_rps` / `rate_limit_burst` overrides (NULL = data-plane default)
- `sources` — named entry points (e.g., "feedback", "chat")
- `guardrails` — org-level guardrail instances (JSONB config)
- `source_guardrails` — many-to-many attachment
- `transformers` — org-level message rewriters, input and output phases ("Transformations" in UI copy; `transformers` in API/DB)
- `source_transformers` — ordered attachment (position column = chain order)
- `alert_webhooks` — org-level rejection-alert destinations (url + payload_format json/slack)

**Data plane** (`internal/dataplane/db/migrations/`, separate goose version table):
- `audit_logs` — request history with per-guard results and LLM token usage (`prompt_tokens`/`completion_tokens`/`total_tokens`/`model`, NULL when no LLM call was made); no FKs, org/source ids are opaque

## API Endpoints

**Control plane (:8080)**
- `POST /v1/auth/register` | `POST /v1/auth/login` — Auth
- `POST/GET/DELETE /v1/auth/api-keys` — API-key management; create accepts optional per-key rate limits (JWT auth)
- `CRUD /v1/sources` — Source management (JWT auth)
- `CRUD /v1/guardrails` — Guardrail management + attachment; `POST /v1/guardrails/:id/test` dry-runs a guard against sample text (JWT auth)
- `CRUD /v1/transformers` — Transformer management; PUT /v1/sources/:id/transformers replaces the ordered chain (JWT auth)
- `GET /v1/audit-logs` — Audit logs (filters: status/from/to; keyset pagination via before/before_id), proxied from the data plane (JWT auth)
- `GET /v1/analytics` — Aggregated usage (requests, tokens, estimated cost by model/source), proxied from the data plane (JWT auth)
- `GET /v1/models` — Supported model list for the dropdown, served from the shared priced catalog (`internal/shared/modelcatalog`, prices in prices.json — also used for analytics cost estimation) (JWT auth)
- `CRUD /v1/alerts` — Rejection-alert webhooks; DP POSTs JSON/Slack payloads on guard rejections, async + rate-capped (JWT auth)
- `GET /v1/internal/workspace-config` — Config payload for data planes (DP token)
- `GET /healthz` — Health check

Proxy pipeline: input transformers (sequential, whole messages array, never reject; on_error fail_closed aborts) → input guards → LLM → output transformers (rewrite the response) → output guards. Built-in transformer types: `regex_replace`, `webhook`, `presidio` (self-hosted PII redaction; `mode: reversible` + a `deanonymize` output transformer restore originals in the response via per-request `transform.State`). Guards accept `scope: all_messages` to validate full history and `mode: shadow` to run monitor-only (recorded in audit as `shadow: true`, never blocks). Audit `guard_results` JSONB reserves the keys `transforms` (input chain) and `output_transforms` (output chain); `input_hash` is post-transform. The proxy rate-limits per API key (in-memory, per-replica), honoring per-key overrides from the config.

**Data plane (:8081)**
- `POST /v1/proxy/:source_route` — Main proxy endpoint (API key auth; `"stream": true` = buffered streaming — full pipeline first, then SSE replay)
- `GET /v1/internal/audit-logs` — Audit queries for the control plane (DP token)
- `GET /v1/internal/analytics` — Aggregated usage queries for the control plane (DP token)
- `GET /healthz` — Health + config version

## Git Workflow

- Branch from `main` for new features/fixes
- Use descriptive commit messages
- PRs require passing `go build`, `go test`, `go vet` before merging

## Behavioral Guidelines

Behavioral guidelines to reduce common LLM coding mistakes. Bias toward caution over speed. For trivial tasks, use judgment.

### 1. Think Before Coding

Don't assume. Don't hide confusion. Surface tradeoffs.

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

Touch only what you must. Clean up only your own mess.

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

Define success criteria. Loop until verified.

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.
