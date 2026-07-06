# Roadmap: Guardrail & Transformer Capabilities

Gap analysis after PR #35 (built-in transformer types + Transformations UI).
Priority order reflects user value vs. build cost. Each item should get its
own plan before implementation.

## 1. Output-phase transformers — SHIPPED

Implemented in the `output-transformers` branch/PR: migration 009 relaxes
the phase CHECK to `('input','output')`; the data plane splits each
source's chain into Input/OutputTransformers preserving order; the
processor runs the output chain on the LLM response (all choices) before
output guards; audit reserves `output_transforms`; UI gains a phase
selector and badges. Ordering decision: transform-then-validate, so output
guards see the text the client actually receives.

Original notes:

The strongest redaction case is often the *response*: the LLM echoing PII
from context, leaking system-prompt contents, or emitting markup/links to
strip. v1 transformers are input-only, but the design anticipated this:

- `transformers.phase` column exists with a `CHECK (phase = 'input')` —
  relax the constraint and accept `output` in service validation.
- Run a second chain in the processor after output guards (or before —
  decide ordering: redact-then-validate vs validate-then-redact).
- Phase already flows through `guard.CallMeta`; webhook and presidio
  transformer types work unchanged.
- Audit: `transforms` key becomes phase-keyed like guard results (or add
  `output_transforms`) — pick one and document the reserved keys.
- UI: phase selector in the wizard/form; source tab shows both chains.

## 2. `llm_classifier` guard — SHIPPED

Implemented in the `llm-classifier-guard` branch/PR: config {base_url,
api_key?, model, instructions, timeout_seconds}; strict JSON verdict
protocol ({"flagged", "reason"}) with fence-tolerant parsing; unparseable
verdicts and endpoint errors follow on_error. Works on both phases and
with scope: all_messages.

Original notes:

Send text + a rubric prompt to a cheap model ("is this prompt injection /
off-topic / a competitor mention?") and pass/fail on its verdict. Covers the
long tail of policies no regex or moderation API handles.

- Reuse the LiteLLM client (`internal/shared/llm`).
- Config: model, provider credentials, rubric prompt, pass condition
  (expected label / yes-no), timeout, on_error.
- Careful defaults: short timeout, fail_open guidance in docs (a slow
  classifier doubles proxy latency).
- Works for both phases via the existing guard engine; `scope:
  all_messages` applies.

## 3. Presidio as a guard (detect-and-reject) — SHIPPED

Implemented alongside the audit-UI item: guard type `presidio` (config:
analyzer_url, language, entities, score_threshold default 0.5); rejects on
any detection at/above threshold; detections (types + positions, never the
matched text) land in audit details.

Original notes:

Same self-hosted `/analyze` endpoint the presidio transformer calls, but
reject-on-detection instead of rewrite — "block any request containing a
credit card number" is a different policy than "mask it."

- New guard type `presidio`; config: analyzer_url, entities, score
  threshold, language.
- HTTP client/config conventions already exist in the transformer — keep
  the shapes aligned.

## 4. Reversible anonymization (deanonymize on output)

Replace entities with placeholders on input, restore the real values in the
response. Presidio supports this, but it needs per-request state shared
between the input and output stages — real architectural work (a request-
scoped entity map carried through the processor). The natural endgame of
the transformer design; wait for demand before building.

## 5. Streaming responses

The proxy buffers the full LLM response today; output guards and
transformers fundamentally conflict with token streaming. Options, in
increasing complexity:

1. Document the tradeoff (status quo).
2. "Buffered streaming": run output guards/transforms on the complete
   response, then stream the approved text to the client (client-perceived
   streaming UX, unchanged safety semantics).
3. True incremental scanning (chunked guards) — large effort, weaker
   guarantees; not planned.

## Deferred / covered by webhook types

LLM Guard, Private AI, philterd, token truncation, prompt-injection
stripping — all reachable today via the generic webhook guard/transformer.
Add native types only when configuration pain or demand justifies it.

## Small fixes (tracked in tech-debt.md)

- Guard config validation at create time (parity with transformers).
- Render the `transforms` audit key in the Events UI.
