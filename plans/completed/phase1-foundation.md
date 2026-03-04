# Phase 1: Foundation — Go Project + Database + Core CRUD API

**Status**: In Progress
**Started**: 2026-02-22

## Goal
Standing Go API server with auth, sources, and destinations CRUD.

## Progress
- [x] Initialize Go module
- [x] Set up project directory structure
- [x] Configure sqlc + goose
- [x] Write database migrations
- [x] Write sqlc queries for all CRUD operations
- [x] Implement config loading (envconfig)
- [x] Domain types
- [ ] Generate sqlc code
- [ ] Build chi router with middleware (logging, CORS, request ID)
- [ ] Implement response helpers
- [ ] Implement auth service (registration, login, JWT, API keys)
- [ ] Implement source service + CRUD handlers
- [ ] Implement destination service + CRUD handlers
- [ ] Set up Docker Compose (Go app + PostgreSQL)
- [ ] Verify: go build, go test, go vet all pass

## Key Files
- `cmd/sheeld/main.go` — entry point
- `internal/config/config.go` — env config
- `internal/domain/types.go` — core types
- `internal/db/migrations/001_initial.sql` — schema
- `internal/db/queries/*.sql` — sqlc queries
- `sqlc.yaml` — sqlc configuration

## Tooling
- chi (HTTP), pgx (DB driver), sqlc (queries), goose (migrations)
- envconfig (config), slog (logging), JWT + bcrypt (auth)
