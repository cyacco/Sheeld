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
│   │   ├── handler/         # auth, source, destination handlers
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
│   └── service/             # Business logic (auth, source, destination)
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
- `destinations` — guardrail instances attached to sources (JSONB config)
- `audit_logs` — request history with per-guard results

## API Endpoints

- `POST /v1/auth/register` | `POST /v1/auth/login` — Auth
- `CRUD /v1/sources` — Source management (JWT auth)
- `CRUD /v1/sources/:id/destinations` — Destination management (JWT auth)
- `POST /v1/proxy/:source_slug` — Main proxy endpoint (API key auth)
- `GET /healthz` — Health check

## Git Workflow

- Branch from `main` for new features/fixes
- Use descriptive commit messages
- PRs require passing `go build`, `go test`, `go vet` before merging
