# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases are cut by pushing a `vX.Y.Z` tag; the `Release` workflow builds and
pushes the container images to GHCR and publishes a GitHub Release. Move items
from `[Unreleased]` into a dated version section as part of cutting a release.

## [Unreleased]

## [0.1.0] - 2026-07-10

Initial public release. Sheeld is a "Segment for LLM guardrails": a
control/data-plane LLM proxy that validates input, proxies the LLM call, and
validates output.

### Added

- **Proxy pipeline**: input transformers → input guards → LLM
  (OpenAI-compatible) → output transformers → output guards. Buffered streaming
  (`"stream": true`) runs the full pipeline before replaying approved SSE.
- **Per-source LLM endpoints**: each source can set `llm_base_url` to send its
  traffic directly to any OpenAI-compatible provider (OpenAI, Anthropic,
  Gemini, Groq, Ollama, vLLM, OpenRouter, or a gateway like LiteLLM); sources
  without one use the data plane's default gateway URL.
- **Full OpenAI schema passthrough**: request/response fields the pipeline
  doesn't act on (`tools`/`tool_calls`, `response_format`, multimodal content
  arrays, `top_p`, `seed`, `logprobs`, …) now round-trip to and from the
  provider unchanged, so function calling and structured outputs work through
  the proxy.
- **Guards**: fan-out engine with `all` / `any` / `n_of_m` pass criteria,
  per-guard `on_error` fail-open/closed, and `scope: all_messages`. Built-in
  types include regex, OpenAI moderation, LLM classifier, Presidio PII
  detection, and generic webhook.
- **Transformers**: sequential input/output rewriters — `regex_replace`,
  `webhook`, and `presidio` (with reversible anonymization via a `deanonymize`
  output transformer).
- **Control/data-plane split**: control plane owns config + auth + dashboard;
  data plane polls workspace config (ETag/304) and serves the proxy with no DB
  on the request path. Optional encrypted on-disk config snapshot as a startup
  fallback.
- **Dashboard**: Next.js app for sources, guardrails, transformers,
  connections wiring, and audit logs.
- **Security hardening**: org-scoped access control, secret redaction in API
  responses, SSRF protection on user-supplied guard/transformer URLs, trimmed
  API-key listings, and control-plane rate limiting.
- **Production hardening**: LLM client retries with exponential backoff on
  transient failures; opt-in audit-log retention/pruning; Prometheus metrics
  on both planes at `/metrics`.
- **Deployment**: Docker Compose stack and a Helm chart (both planes + two
  Postgres instances + web, with Prometheus scrape annotations).
- **Release automation**: tag-triggered workflow publishing images to GHCR and
  creating a GitHub Release.

### Fixed

- Blocklist guard now matches on word boundaries, so multi-word phrases (e.g.
  "ignore previous instructions") are enforced and regex metacharacters in a
  term are treated literally.
- Proxy 500 responses no longer echo internal error detail to clients; the real
  error is logged server-side and correlated via `X-Request-ID`.
- Per-key rate limiters are evicted after an idle period, bounding memory under
  many distinct orgs/IPs.
- The control plane validates `SHEELD_ENCRYPTION_KEY` at startup (hex, 32 bytes)
  instead of failing on the first source write.

Thanks to @kaeawc, whose contributions surfaced these fixes.

[Unreleased]: https://github.com/cyacco/Sheeld/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/cyacco/Sheeld/releases/tag/v0.1.0
