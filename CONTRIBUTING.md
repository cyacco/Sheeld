# Contributing to Sheeld

Thanks for your interest in improving Sheeld! This guide covers how to get set
up, the checks your change must pass, and how to propose it.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting set up

Prerequisites: Go 1.25+, Docker + Docker Compose, and Node.js 22+ (only for the
dashboard). See the [README](README.md) for the full local-development walkthrough.

```bash
# Start the databases
docker compose up cp-db dp-db -d

# Run the control plane and data plane in separate shells (see README for env)
go run ./cmd/control-plane
go run ./cmd/sheeld-server
```

## Architecture at a glance

Sheeld is a control-plane / data-plane split (rudder-server style):

- **Control plane** (`cmd/control-plane`, `internal/controlplane`) — auth, CRUD,
  the dashboard backend, and the workspace-config endpoint. Owns cp-db.
- **Data plane** (`cmd/sheeld-server`, `internal/dataplane`) — the proxy. Polls
  the control plane for config, runs input guards → LLM → output guards, and
  writes audit logs to dp-db. No control-plane or DB access on the request path.
- **Shared** (`internal/shared`) — the guard engine, transformer pipeline, LLM
  client, and domain types used by both planes.

`CLAUDE.md` has a deeper tour of the layout and conventions.

## Before you open a pull request

Every change must pass the same checks CI runs:

```bash
gofmt -l .                                   # must print nothing
go vet ./...
go build ./...
go test ./...
go test -tags integration ./internal/integration/   # requires Docker
```

For dashboard changes (`web/`):

```bash
cd web && npm ci && npm run build   # includes typecheck + lint
```

If you change SQL queries, regenerate the typed code (never edit
`internal/**/db/generated/` by hand):

```bash
~/go/bin/sqlc generate
```

## Conventions

- **Format and vet** everything (`gofmt`, `go vet`). CI fails on unformatted code.
- **Tests** — add or update tests for behavior changes. Table-driven tests are
  the norm; see `internal/shared/guard` for examples. Cross-cutting behavior gets
  an integration test in `internal/integration`.
- **Migrations** are goose files under `internal/{controlplane,dataplane}/db/migrations`;
  they run automatically at startup. Add matching `-- +goose Down` sections.
- **Security-sensitive surfaces**: the workspace-config payload carries plaintext
  LLM keys — never log it. Guard/transformer configs may hold secrets — never
  return them unredacted in an API response (see `service.SanitizeConfig`).
- **Keep changes surgical** — touch only what the change requires, and match the
  surrounding style.

## Pull request process

1. Branch from `main`.
2. Make your change with tests and passing local checks.
3. Open a PR against `main` with a clear description of *what* and *why*. Fill in
   the PR template.
4. CI must be green (Go, Dashboard, Docker Build, Helm Lint) before review.

Small, focused PRs are reviewed fastest. If you're planning something large, open
an issue first so we can align on the approach.

## Reporting bugs and requesting features

Use the [issue templates](https://github.com/cyacco/Sheeld/issues/new/choose). For
**security vulnerabilities, do not open a public issue** — follow
[SECURITY.md](SECURITY.md).
