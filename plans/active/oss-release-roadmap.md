# Open-Source Release Roadmap

## Context

The guardrail/transformer engine is feature-complete, security-hardened (PR #45),
and Helm-deployable (PR #46). What remains before a credible public release is
release engineering and operability, not core features. This roadmap sequences
that work. Assessment done via codebase review on 2026-07-07.

## M1 — "Can exist publicly" (blockers)  — IN PROGRESS
- Governance files: CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, issue/PR templates,
  CODEOWNERS; fill LICENSE copyright. **(this PR)**
- Go module path — **SHIPPED**: renamed `github.com/sheeld/sheeld` →
  `github.com/cyacco/Sheeld` to match the repo, so `go get` / external imports
  work. Mechanical rewrite across all Go files + go.mod; build/vet/test +
  integration green.

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
- README rework — **SHIPPED**: value-prop lede ("Segment for LLM guardrails" +
  four differentiators), CI/release/license/Go badges, dashboard connections
  screenshot up top and audit-log screenshot in the quickstart
  (docs/images/), quickstart already ends in a working guarded call.
- `docker compose up` demo that works end-to-end out of the box — **SHIPPED**:
  LiteLLM replaced by a 15MB Go mock provider (cmd/mock-llm, OpenAI-compatible)
  so the full pipeline runs with no provider key and no Python sidecar; README
  quickstart now ends in a copy-paste guarded call (pass → 200, blocked → 422).
  Verified live against the built stack. LiteLLM remains supported as a
  bring-your-own gateway (any OpenAI-compatible base URL works).
- Fill test gaps — **PARTIAL**: `auditstore` now unit-tested (writer batching/
  drop/retry/drain + pruner batched-delete/disabled/error paths; 0→65%) via
  small query interfaces for fakes; CP handler validation/error paths covered
  with a nil-service pattern (happy paths stay in integration). Dashboard test
  harness — **SHIPPED**: vitest + Testing Library wired up in `web/`
  (`npm test` / `npm run test:run`) with initial coverage of the guard-type
  catalog, guardrail-form draft mapping, and a Badge render test.

## Pre-v0.1.0 must-fix (found 2026-07-08 review)
- **OpenAI API-surface passthrough** — **SHIPPED**: `llm.ChatRequest`/`Message`/
  `ChatResponse`/`Choice` now capture unmodeled fields as raw JSON and re-emit
  them, so `tools`/`tool_calls`, `response_format`, `top_p`/`seed`/`logprobs`/
  `stop`/`stream_options`, and message `tool_calls`/`name`/`refusal` round-trip
  to and from the provider. Multimodal content arrays decode (text extracted for
  guards, raw preserved for the provider); `content: null` is preserved. Unit +
  integration tests prove tools reach the provider and tool_calls return.
  Known limitation: a transformer that rewrites a multimodal message collapses
  it to text (drops non-text parts) — acceptable for v0.1.0.
- **Triage the external contributor PR queue** (#17–#29 from @kaeawc) — **DONE**:
  verified each against current main. Adapted four still-valid fixes (crediting
  the contributor): #18 proxy error-detail leak, #21 word-boundary/phrase
  blocklist, #22 rate-limiter idle eviction, #23 startup encryption-key
  validation. Closed as superseded: #20 (async audit), #24 (revoked keys), #25
  (org-scoping), #27 (const-time/hashed lookup), #29 (→ #55 passthrough). Closed
  as already-handled: #17 (engine sets duration on error), #26 (per-request
  struct, not shared). Deferred to post-launch: #28 (streaming client → conflicts
  with buffered-guard model), #19 (guard cancellation → breaks full audit
  results).
- Module-path rename `github.com/sheeld/sheeld` → repo path — **SHIPPED** (see M1).

## Post-launch (feature milestones)
- Multi-user-per-org + roles + invitations (today one registration = one user).
- Analytics dashboard (needs token/cost capture in audit logs first).
- Guard dry-run — **SHIPPED**: POST /v1/guardrails/{id}/test runs a guard against
  sample text (org-scoped, bounded timeout for network guards) and a "Test" tab on
  the guardrail detail page shows pass/reject + details, without touching live
  traffic or the audit trail. Verified live in the dashboard.
- Pagination on sources/guardrails/transformers list endpoints; richer audit-log
  filtering.
- True incremental streaming: chunk-level output guarding so TTFT isn't full
  pipeline latency (today streaming is buffered — the honest gap in our story).
- Guard shadow/monitor mode — **SHIPPED**: a guard with `mode: shadow` runs and
  is recorded in the audit log (result marked `shadow: true`) but never counts
  toward the accept/reject decision, so a guard can be trialed on live traffic
  before enforcing. Marker detection is wrap-order-independent (Unwrap chain);
  dashboard has a Mode control on the guard form and a "shadow" badge in the
  audit log. Verified live (request tripping a shadow guard → pass, would-be
  fail visible in audit).
- Per-API-key rate limits/quotas (limiter is currently global per replica).
- Rejection alerting (webhook/Slack on guard failures); audit-log export
  (CSV/JSON, SIEM forwarding).
- Non-bearer provider auth (e.g. Azure `api-key` header) for direct connections.
- Published latency benchmark ("Sheeld adds ~X ms p50") for the README.
