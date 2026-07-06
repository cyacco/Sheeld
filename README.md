# Sheeld

Segment for LLM guardrails — a full LLM proxy that validates input, proxies LLM calls, and validates output.

**Architecture** (rudder-server style control/data plane split):

```
                       ┌──────────────────────────┐
  Dashboard / users ──▶│ Control Plane (:8080)    │──▶ cp-db (users, orgs, config)
                       │ auth, sources, guardrails │
                       └────────────▲─────────────┘
                                    │ polls workspace config (~5s)
                       ┌────────────┴─────────────┐
  User's App ─────────▶│ Data Plane (:8081)       │──▶ dp-db (audit logs)
   (API key)           │ transforms → input guards │──▶ LiteLLM → LLM provider
                       │ → LLM → output transforms │
                       │ → output guards           │
                       └──────────────────────────┘
```

The proxy pipeline is: request → input transformers (sequential rewrites — "Transformations" in the UI) → input guards (concurrent) → LLM → output transformers (rewrite the response) → output guards (concurrent) → response.

**Built-in guard types**: `blocklist`, `regex`, `openai_moderation`, `guardrails_ai`, `webhook` (your own validation endpoint), `llm_classifier` (a small LLM enforces a plain-language policy), and `presidio` (reject on PII detected by self-hosted Microsoft Presidio). Every guard accepts `on_error: fail_closed (default) | fail_open` and `scope: last_message (default) | all_messages`.

**Built-in transformer types**: `regex_replace` (pattern rules), `webhook` (your own rewrite endpoint), `presidio` (PII anonymization — `mode: redact` replaces entities irreversibly; `mode: reversible` substitutes placeholders the LLM sees), and `deanonymize` (output phase; restores the original values in the response so PII round-trips without ever reaching the LLM provider).

The data plane holds its full config in memory and keeps serving proxy traffic even if the control plane goes down. No control-plane DB access happens on the request path.

Licensed under Apache 2.0.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Node.js 22+](https://nodejs.org/) (only for local dashboard development)

## Quick Start (Docker Compose)

Copy the example env file and fill in your secrets:

```bash
cp .env.example .env
# Edit .env — at minimum, generate a real encryption key:
#   openssl rand -hex 32
```

Then start the full stack:

```bash
docker compose up -d --build
```

This starts six services:

| Service | URL | Description |
|---------|-----|-------------|
| **control-plane** | http://localhost:8080 | Config API, auth, dashboard backend |
| **sheeld-server** | http://localhost:8081 | Data plane: proxy + guard engine |
| **Dashboard** | http://localhost:3000 | Next.js management UI |
| **LiteLLM** | http://localhost:4000 | LLM gateway proxy |
| **cp-db** | localhost:5432 | Control-plane PostgreSQL (users, orgs, config) |
| **dp-db** | localhost:5433 | Data-plane PostgreSQL (audit logs) |

Verify everything is running:

```bash
docker compose ps
curl http://localhost:8080/healthz   # control plane
curl http://localhost:8081/healthz   # data plane (includes config version)
```

To stop all services:

```bash
docker compose down
```

## Local Development (without Docker)

### 1. Start the databases

```bash
docker compose up cp-db dp-db -d
```

### 2. Run the control plane

```bash
export SHEELD_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5432/sheeld?sslmode=disable"
export SHEELD_JWT_SECRET="dev-secret"
export SHEELD_ENCRYPTION_KEY=$(openssl rand -hex 32)
export SHEELD_DATAPLANE_TOKEN="dev-dataplane-token"
export SHEELD_DATAPLANE_URL="http://localhost:8081"
go run ./cmd/control-plane
```

### 3. Run the data plane

```bash
export SHEELD_DP_DATABASE_URL="postgres://sheeld:sheeld_dev@localhost:5433/sheeld?sslmode=disable"
export SHEELD_DP_CONTROL_PLANE_URL="http://localhost:8080"
export SHEELD_DP_ALLOW_INSECURE_CP=true   # plain HTTP is for local dev only
export SHEELD_DP_TOKEN="dev-dataplane-token"
go run ./cmd/sheeld-server
```

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
├── cmd/
│   ├── control-plane/       # Control-plane entrypoint
│   └── sheeld-server/       # Data-plane entrypoint
├── internal/
│   ├── controlplane/
│   │   ├── api/             # chi router, CRUD handlers, auth middleware
│   │   ├── service/         # Business logic (auth, source, guardrail)
│   │   ├── db/              # goose migrations + sqlc (users, orgs, config)
│   │   ├── crypto/          # AES-256-GCM for LLM keys at rest
│   │   └── workspaceconfig/ # Builds + serves the config payload
│   ├── dataplane/
│   │   ├── gateway/         # HTTP layer: in-memory API-key auth, proxy route
│   │   ├── processor/       # Transforms → input guards → LLM → output transforms → guards
│   │   ├── backendconfig/   # Config poller + atomic in-memory store
│   │   ├── auditstore/      # Async batched audit writer + query API
│   │   └── db/              # goose migrations + sqlc (audit logs)
│   └── shared/              # guard engine, LLM client, domain types, middleware
├── web/                     # Next.js dashboard
├── docker-compose.yaml
├── Dockerfile               # Multi-stage; targets: control-plane, sheeld-server
├── openapi.yaml             # API specification
└── sqlc.yaml
```

## API Endpoints

### Control plane (:8080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/healthz` | None | Health check |
| `POST` | `/v1/auth/register` | None | Register a new account |
| `POST` | `/v1/auth/login` | None | Login |
| `CRUD` | `/v1/sources` | JWT | Source management |
| `CRUD` | `/v1/guardrails` | JWT | Guardrail management + source attachment |
| `CRUD` | `/v1/transformers` | JWT | Transformer management; `PUT /v1/sources/:id/transformers` reorders a source's chain |
| `GET` | `/v1/audit-logs` | JWT | Audit logs (proxied from the data plane) |
| `GET` | `/v1/internal/workspace-config` | DP token | Config payload for data planes |

### Data plane (:8081)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/healthz` | None | Health + config version |
| `POST` | `/v1/proxy/:source_route` | API Key | Main proxy endpoint (OpenAI-compatible) |
| `POST` | `/v1/proxy/:source_route/chat/completions` | API Key | Alias for OpenAI SDK base_url compatibility |
| `GET` | `/v1/internal/audit-logs` | DP token | Audit-log queries for the control plane |

The proxy is a drop-in OpenAI replacement: point your SDK's `base_url` at `http://<data-plane>/v1/proxy/<route>` with your Sheeld API key and the response is a raw chat completion. Guardrail rejections return HTTP 422 with an OpenAI-style error (`"type": "guardrail_rejection"`); full guard results are in the audit logs, correlated by the `X-Request-ID` response header.

`"stream": true` is supported as **buffered streaming**: guards and transformers always evaluate the complete response first, then the approved text is replayed as standard SSE chunks. Time-to-first-token therefore equals the full pipeline latency — the safety semantics are identical to non-streaming, and rejections surface as the usual 422 before any stream starts.

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8081/v1/proxy/feedback", api_key="shld_...")
resp = client.chat.completions.create(model="ignored", messages=[{"role": "user", "content": "Hi"}])
```

See [`openapi.yaml`](openapi.yaml) for the full API specification.

## Common Commands

```bash
# Build
go build ./...

# Test
go test ./...

# Integration tests (requires Docker)
go test -tags integration ./internal/integration/

# Lint
go vet ./...

# Format
gofmt -w .

# Regenerate sqlc code (both planes)
~/go/bin/sqlc generate
```

## Configuration

### Control plane (`SHEELD_` prefix)

| Variable | Description | Default |
|----------|-------------|---------|
| `SHEELD_PORT` | Server port | `8080` |
| `SHEELD_DATABASE_URL` | PostgreSQL connection string | required |
| `SHEELD_JWT_SECRET` | Secret for JWT signing | required |
| `SHEELD_ENCRYPTION_KEY` | 64-char hex key for encrypting LLM keys at rest | required |
| `SHEELD_DATAPLANE_TOKEN` | Static token authenticating data planes | — (disables data-plane endpoints) |
| `SHEELD_DATAPLANE_URL` | Data-plane base URL for audit-log queries | — (disables audit queries) |
| `SHEELD_CORS_ALLOWED_ORIGINS` | Comma-separated CORS origins | `http://localhost:3000` |
| `SHEELD_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

### Data plane (`SHEELD_DP_` prefix)

| Variable | Description | Default |
|----------|-------------|---------|
| `SHEELD_DP_PORT` | Server port | `8081` |
| `SHEELD_DP_DATABASE_URL` | PostgreSQL connection string (audit logs) | required |
| `SHEELD_DP_CONTROL_PLANE_URL` | Control-plane base URL | required |
| `SHEELD_DP_TOKEN` | Shared static token (matches `SHEELD_DATAPLANE_TOKEN`) | required |
| `SHEELD_DP_ALLOW_INSECURE_CP` | Allow http:// control-plane URL (local dev only) | `false` |
| `SHEELD_DP_POLL_INTERVAL` | Workspace-config poll interval | `5s` |
| `SHEELD_DP_STARTUP_TIMEOUT` | Max wait for initial config at startup | `60s` |
| `SHEELD_DP_LLM_GATEWAY_URL` | LiteLLM gateway URL | `http://localhost:4000` |
| `SHEELD_DP_LLM_REQUEST_TIMEOUT` | Timeout for LLM requests | `30s` |
| `SHEELD_DP_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

## Operational notes

- **Secrets in the config channel**: the workspace-config payload contains plaintext LLM API keys (decrypted by the control plane, rudder-style). The channel is token-authenticated; use TLS between planes in any real deployment — plain HTTP is acceptable only inside the compose network, and the data plane refuses http:// URLs unless `SHEELD_DP_ALLOW_INSECURE_CP=true`. Never log the payload.
- **Config propagation lag**: config changes (including API-key revocations) take up to one poll interval (~5s) to reach data planes.
- **Control-plane outages**: data planes keep serving proxy traffic from the in-memory config; only config changes and audit-log dashboard queries are unavailable until it returns.
- **Migrations**: each binary runs its own goose migrations at startup — fine for single replicas; run migrations as a separate step when deploying multiple replicas.
- **Rate limits are per data-plane replica** (in-memory), so effective limits scale with replica count.
- **Migrating from the single-binary layout**: audit logs moved to the data-plane DB. To keep existing rows, dump them before the control plane applies migration 007: `pg_dump -t audit_logs <cp-db-url> | psql <dp-db-url>`.

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
