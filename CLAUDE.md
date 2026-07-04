# Sheeld

## Project Overview

Sheeld is a "Segment for LLM guardrails" — a full LLM proxy that validates input, proxies LLM calls, and validates output. Licensed under Apache 2.0.

**Architecture**: User's App → Sheeld API → Input Guards (fan-out) → LLM Provider → Output Guards (fan-out) → Response

## Development Setup

```bash
# Build
go build ./...

# Test
go test ./...

# Run locally (requires PostgreSQL)
docker-compose up db -d
export SHEELD_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5432/sheeld?sslmode=disable"
export SHEELD_JWT_SECRET="dev-secret"
export SHEELD_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
go run ./cmd/sheeld

# Run full stack
docker-compose up
```

## Repository Structure

```
sheeld/
├── cmd/sheeld/              # Binary entrypoint
├── internal/
│   ├── api/                 # HTTP handlers + middleware (chi router)
│   │   ├── router.go        # Route definitions
│   │   ├── middleware/       # auth (JWT + API key), logging, request ID
│   │   ├── handler/         # auth, source, guardrail handlers
│   │   └── response/        # JSON response helpers
│   ├── config/              # envconfig-based configuration
│   ├── db/
│   │   ├── migrations/      # goose SQL migrations
│   │   ├── queries/         # sqlc .sql files
│   │   └── generated/       # sqlc-generated Go code (DO NOT EDIT)
│   ├── domain/              # Core domain types
│   ├── guard/               # Guardrail engine (Phase 2)
│   ├── llm/                 # LLM provider proxy (Phase 3)
│   ├── proxy/               # Proxy orchestration (Phase 3)
│   └── service/             # Business logic (auth, source, guardrail)
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
| `~/go/bin/sqlc generate` | Regenerate sqlc code after query changes |
| `docker-compose up` | Start full stack (API + PostgreSQL) |
| `docker-compose up db -d` | Start only PostgreSQL |

## Key Tooling

| Tool | Purpose |
|------|---------|
| **chi** | HTTP router (lightweight, idiomatic) |
| **pgx** | PostgreSQL driver |
| **sqlc** | SQL → type-safe Go code generation |
| **goose** | Database migrations |
| **envconfig** | Environment variable config (SHEELD_ prefix) |
| **slog** | Structured logging (stdlib) |

## Code Style

- Follow standard Go conventions and `gofmt` formatting
- Use `go vet` to catch common mistakes
- Write table-driven tests where applicable
- Keep packages focused and cohesive
- sqlc generated code in `internal/db/generated/` is auto-generated — never edit manually

## Database

PostgreSQL with goose migrations in `internal/db/migrations/`. Tables:
- `organizations` — multi-tenant orgs
- `users` — org members
- `api_keys` — machine-to-machine auth (SHA-256 hashed)
- `sources` — named entry points (e.g., "feedback", "chat")
- `guardrails` — guardrail instances attached to sources (JSONB config)
- `audit_logs` — request history with per-guard results

## API Endpoints

- `POST /v1/auth/register` | `POST /v1/auth/login` — Auth
- `CRUD /v1/sources` — Source management (JWT auth)
- `CRUD /v1/sources/:id/guardrails` — Guardrail management (JWT auth)
- `POST /v1/proxy/:source_route` — Main proxy endpoint (API key auth)
- `GET /healthz` — Health check

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
