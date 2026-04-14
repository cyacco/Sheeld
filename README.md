# Sheeld

Segment for LLM guardrails — a full LLM proxy that validates input, proxies LLM calls, and validates output.

**Architecture**: User's App → Sheeld API → Input Guards (fan-out) → LLM Provider → Output Guards (fan-out) → Response

Licensed under Apache 2.0.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Node.js 22+](https://nodejs.org/) (only for local dashboard development)

## Quick Start (Docker Compose)

Start the full stack with a single command:

```bash
docker compose up -d --build
```

This starts all four services:

| Service | URL | Description |
|---------|-----|-------------|
| **API** | http://localhost:8080 | Sheeld proxy API |
| **Dashboard** | http://localhost:3000 | Next.js management UI |
| **LiteLLM** | http://localhost:4000 | LLM gateway proxy |
| **PostgreSQL** | localhost:5432 | Database |

Verify everything is running:

```bash
docker compose ps
curl http://localhost:8080/healthz
```

To stop all services:

```bash
docker compose down
```

## Local Development (without Docker)

### 1. Start the database

```bash
docker compose up db -d
```

### 2. Set environment variables

```bash
export SHEELD_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5432/sheeld?sslmode=disable"
export SHEELD_JWT_SECRET="dev-secret"
export SHEELD_ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

### 3. Run the API

```bash
go run ./cmd/sheeld
```

The API will be available at http://localhost:8080.

### 4. Run the dashboard (optional)

```bash
cd web
npm install
npm run dev
```

The dashboard will be available at http://localhost:3000.

## Project Structure

```
sheeld/
├── cmd/sheeld/              # Binary entrypoint
├── internal/
│   ├── api/                 # HTTP handlers + middleware (chi router)
│   │   ├── router.go        # Route definitions
│   │   ├── middleware/       # Auth (JWT + API key), logging, request ID
│   │   └── handler/         # Auth, source, destination handlers
│   ├── config/              # envconfig-based configuration
│   ├── db/
│   │   ├── migrations/      # goose SQL migrations
│   │   ├── queries/         # sqlc .sql files
│   │   └── generated/       # sqlc-generated Go code (DO NOT EDIT)
│   ├── domain/              # Core domain types
│   ├── guard/               # Guardrail engine
│   ├── llm/                 # LLM provider proxy
│   ├── proxy/               # Proxy orchestration
│   └── service/             # Business logic
├── web/                     # Next.js dashboard
├── docker-compose.yaml
├── Dockerfile
├── openapi.yaml             # API specification
└── sqlc.yaml
```

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/healthz` | None | Health check |
| `POST` | `/v1/auth/register` | None | Register a new account |
| `POST` | `/v1/auth/login` | None | Login |
| `CRUD` | `/v1/sources` | JWT | Source management |
| `CRUD` | `/v1/sources/:id/destinations` | JWT | Destination management |
| `POST` | `/v1/proxy/:source_route` | API Key | Main proxy endpoint |

See [`openapi.yaml`](openapi.yaml) for the full API specification.

## Common Commands

```bash
# Build
go build ./...

# Test
go test ./...

# Lint
go vet ./...

# Format
gofmt -w .

# Regenerate sqlc code
~/go/bin/sqlc generate
```

## Configuration

Sheeld is configured via environment variables with the `SHEELD_` prefix:

| Variable | Description | Default |
|----------|-------------|---------|
| `SHEELD_PORT` | API server port | `8080` |
| `SHEELD_DATABASE_URL` | PostgreSQL connection string | required |
| `SHEELD_JWT_SECRET` | Secret for JWT signing | required |
| `SHEELD_ENCRYPTION_KEY` | 64-char hex key for encrypting secrets | required |
| `SHEELD_LLM_GATEWAY_URL` | LiteLLM gateway URL | `http://localhost:4000` |
| `SHEELD_LLM_REQUEST_TIMEOUT` | Timeout for LLM requests | `30s` |
| `SHEELD_CORS_ALLOWED_ORIGINS` | Comma-separated CORS origins | — |
| `SHEELD_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

## Key Technologies

| Tool | Purpose |
|------|---------|
| [chi](https://github.com/go-chi/chi) | HTTP router |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL driver |
| [sqlc](https://sqlc.dev/) | SQL → type-safe Go code generation |
| [goose](https://github.com/pressly/goose) | Database migrations |
| [envconfig](https://github.com/kelseyhightower/envconfig) | Environment variable config |
| [slog](https://pkg.go.dev/log/slog) | Structured logging (stdlib) |
| [Next.js](https://nextjs.org/) | Dashboard frontend |

## License

Apache 2.0
