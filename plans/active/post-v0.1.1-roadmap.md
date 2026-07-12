# Post-v0.1.1 Roadmap

## Context

v0.1.1 shipped 2026-07-11: guard dry-run + shadow mode, per-API-key rate
limits, token/model capture, and the analytics dashboard. The engine and ops
story are solid; what remains falls into three buckets — closing the honesty
gaps, deepening the money story, and making Sheeld adoptable by teams.
Sequenced by leverage, cheapest-first within each tier.

## Tier 1 — Highest leverage

- **Rejection alerting (webhook/Slack)** — **SHIPPED**: org-level alert
  webhooks (CRUD /v1/alerts + dashboard Alerts page); the DP POSTs
  generic-JSON or Slack payloads on guard rejections, async and rate-capped
  per webhook, with a `sheeld_alerts_sent_total{outcome}` metric and the
  same SSRF policy as guard URLs. Verified live (UI-created webhook fired on
  a blocked request).
- **Audit filtering + pagination** — **SHIPPED** (status + date filters, keyset
  cursor pagination on GET /v1/audit-logs; dashboard Result/From/To controls).
  Verified live. Remaining: guard-type filter (needs JSONB querying into
  guard_results) and pagination on the sources/guardrails/transformers lists.
- **True incremental streaming** (flagship for v0.2.0) — buffered streaming
  means TTFT = full pipeline latency, disqualifying for chat UX. Chunk-level
  output guarding (sliding-window evaluation, kill the stream on violation).
  Hard part is semantics (what happens to already-streamed tokens on a late
  rejection?), not plumbing. Write a design doc before code.

## Tier 2 — Money and scale story

- **Dollar-cost estimation** — **SHIPPED**: a shared priced model catalog
  (`internal/shared/modelcatalog`) backs both the `/v1/models` dropdown and
  analytics cost estimation; per-model + total estimated USD cost from captured
  tokens (prefix-matched to prices), unpriced models flagged as "—". Verified
  live. Follow-ups: per-org price overrides (custom/self-hosted rates) and a
  periodic price-refresh process.
- **Durable quotas** — Redis-backed rolling request/token caps: optional
  counter store, lease batching for the fast path, explicit fail-open/closed
  switch. Design agreed in principle (see PR #65 discussion); build when
  someone needs billing ceilings.
- **Multi-user orgs + roles + invitations** — one registration = one user is
  fine for evaluation, disqualifying for team adoption. Meaty: invitations,
  role checks across every handler.

## Observability

Already in place: Prometheus `/metrics` on both planes (request + guard latency
histograms, audit-buffer depth/drop counter, config-version + staleness gauge,
`sheeld_llm_tokens_total{kind}`, `sheeld_alerts_sent_total{outcome}`), slog with
`X-Request-ID` correlation, the analytics dashboard, per-guard results +
durations in audit logs, and rejection alerting. Gaps, cheapest-first:

- **Shipped Grafana dashboard + example alert rules** — self-hosters currently
  get a raw `/metrics` endpoint and nothing to consume it. Ship a dashboard
  JSON and Prometheus alert examples (audit drops > 0, config staleness > 60s,
  p95 proxy latency, LLM error rate) under `deploy/` or the Helm chart. Cheap,
  high leverage.
- **`sheeld_guard_errors_total{guard_type}` counter** — guards that repeatedly
  `on_error` fail-open are invisible outside audit JSON. Small add.
- **OpenTelemetry tracing** — spans per pipeline stage (transformers, each
  guard, LLM call) so slow requests decompose without reading audit JSON.
  Medium lift; defer until latency questions come from real users.

## Tier 3 — Ecosystem and polish

- **Audit export / SIEM forwarding** — CSV/JSON export endpoint; enterprise
  checkbox, cheap.
- **Non-bearer provider auth** — Azure OpenAI `api-key` header as a
  per-source auth-style field.
- **Published latency benchmark** — reproducible script + "Sheeld adds
  ~X ms p50" in the README.
- **Guard SDK story** — documented webhook-guard contract + starter repo
  (Python/TS) to make "write your own guard" first-class; OSS growth
  flywheel.

## Housekeeping (opportunistic)

- Bump Node-20-deprecated GitHub Actions (checkout@v4, docker/*) before
  GitHub drops them.
- `wires.tsx` exhaustive-deps lint warning.
- Grow the vitest seed suite alongside feature work.

## Sequencing

Alerting → audit filtering → dollar-cost all shipped. Next: the streaming
design doc as the v0.2.0 flagship, with the Grafana dashboard + alert rules
and the guard-error counter as cheap observability wins to slot in between.
Multi-user when real users ask.
