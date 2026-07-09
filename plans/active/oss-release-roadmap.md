# Open-Source Release Roadmap

## Context

The guardrail/transformer engine is feature-complete, security-hardened (PR #45),
and Helm-deployable (PR #46). What remains before a credible public release is
release engineering and operability, not core features. This roadmap sequences
that work. Assessment done via codebase review on 2026-07-07.

## M1 — "Can exist publicly" (blockers)  — IN PROGRESS
- Governance files: CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, issue/PR templates,
  CODEOWNERS; fill LICENSE copyright. **(this PR)**
- **Deferred (user decision pending):** Go module path is `github.com/sheeld/sheeld`
  but the repo is `github.com/cyacco/Sheeld` — `go get` / external imports fail.
  Resolve by renaming the module to match the repo, or moving to a `sheeld` GitHub
  org. 144 import references across 132 files; one-shot rewrite once decided.

## M2 — "Can be trusted & run" (production hardening + release plumbing)
- Prometheus metrics on both planes — **SHIPPED**: `/metrics` endpoint on CP and
  DP (unauthenticated, network-scoped like the rest of ops); shared
  `internal/shared/metrics` package with HTTP request count/latency (by chi route
  pattern), proxy request count/latency by status+phase, guard-batch latency by
  phase, LLM request outcome + retry counters, audit buffer-depth gauge + entry/
  batch drop counters, and config loaded/last-reload gauges for staleness alerting.
- Audit-log retention — **SHIPPED**: opt-in background pruner (batched deletes,
  disabled by default so audit history is never silently discarded);
  SHEELD_DP_AUDIT_RETENTION / SHEELD_DP_AUDIT_PRUNE_INTERVAL.
- LLM client resilience — **SHIPPED**: retry with exponential backoff on
  transient failures (connection errors, HTTP 429/5xx), never on deterministic
  4xx; context-aware backoff; tunable via SHEELD_DP_LLM_MAX_RETRIES /
  SHEELD_DP_LLM_RETRY_BACKOFF.
- Release workflow — **SHIPPED**: `.github/workflows/release.yml` triggers on a
  `vX.Y.Z` tag, builds & pushes the three images (`sheeld-api`, `sheeld-server`,
  `sheeld-web`) to GHCR via docker/metadata + build-push actions, and publishes a
  GitHub Release with auto-generated notes (prereleases auto-detected from a `-`
  in the tag). CHANGELOG.md added (Keep a Changelog); Helm image repos point at
  `ghcr.io/cyacco/*`. Cutting `v0.1.0` is now just pushing the tag.

## M3 — "Attracts users" (polish)
- README rework: value-prop lede, badges (CI/license/release), a 60-second
  copy-paste quickstart ending in a working guarded LLM call, a screenshot.
- `docker compose up` demo that works end-to-end out of the box.
- Fill test gaps: HTTP handler layer, `auditstore`, dashboard have no unit tests.

## Post-launch (feature milestones)
- Multi-user-per-org + roles + invitations (today one registration = one user).
- Analytics dashboard (needs token/cost capture in audit logs first).
- Guard dry-run: POST /v1/guardrails/{id}/test + a dashboard "test against sample"
  button.
- Pagination on sources/guardrails/transformers list endpoints; richer audit-log
  filtering.
