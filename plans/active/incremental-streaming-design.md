# Incremental Streaming — Design

**Status:** proposed, awaiting sign-off. No code written yet.
**Target:** v0.2.0 flagship.

## Problem

`"stream": true` today is *buffered* streaming: the full pipeline runs, then the
approved response is replayed as SSE ([gateway/stream.go](../../internal/dataplane/gateway/stream.go)).
Safety is perfect — nothing unguarded ever reaches the client — but
time-to-first-token equals total pipeline latency. For a chat UI that is
indistinguishable from not streaming, which disqualifies Sheeld for the most
common LLM product shape.

The goal is to cut TTFT to roughly one guard window while keeping the safety
story honest and explicit about what changes.

## Current state (verified in code)

- `llm.Client` **cannot stream**. `ChatCompletionAt` marshals a request, does one
  round-trip, unmarshals one `ChatResponse` ([llm/client.go:67](../../internal/shared/llm/client.go)).
  Consuming provider SSE is net-new.
- `ChatRequest.Stream` is accepted, then **forced to false** before the provider
  call ([gateway/handler.go:82-83](../../internal/dataplane/gateway/handler.go)); the
  flag only selects SSE replay at the edge.
- `guard.Engine.Run(ctx, guards, input string, cfg)` is stateless over a whole
  string ([guard/engine.go:34](../../internal/shared/guard/engine.go)). It can be
  re-run on progressively longer text with **no new guard interface** — a
  significant simplification. Individual guards need no changes.
- `Processor.Execute` returns a complete `*Result` and writes audit/alerts at the
  end. Streaming needs incremental control flow, so this is the main structural
  change.

## The core tension

Once bytes are flushed to the client they cannot be recalled. So "stream
immediately" and "never show unguarded content" are in direct conflict. Three
possible stances:

1. **Stream freely, kill on violation.** Best TTFT, but violating content is
   already on screen when we detect it. Unacceptable as a guardrail product's
   default — it would make the core promise false.
2. **Guarded release (proposed).** Hold output in a pending buffer; release only
   text that has passed output guards. TTFT = time to fill one window, not the
   whole response. Nothing unguarded is ever released.
3. **Refuse to stream when output guards exist.** Honest, but useless for the
   users who want this.

Option 2 is the design. It preserves the central invariant:

> **No content is released to the client until the output guards have passed on
> it.**

## Design

### Guarded-release loop

```
provider SSE ──▶ pending buffer ──▶ [output guards on cumulative text] ──▶ release to client
```

1. Consume provider deltas, appending to `pending`.
2. When `len(pending) >= windowBytes` (or the provider stream ends), run output
   guards over the **cumulative** released+pending text.
3. Pass → flush `pending` to the client as SSE deltas, move it to `released`.
   Fail → emit termination (below), discard `pending`, never send it.
4. At stream end, write one audit row and fire alerts as today.

Guards evaluate cumulative text, not the isolated window: a blocklist phrase or
a classifier judgment can span a window boundary, and cumulative text is also
what the client will actually have seen.

### The residual exposure, stated plainly

Cumulative re-evaluation means a verdict can flip *late*: windows 1..k−1 passed
and were released, then window k makes the whole text violating. The already
released prefix cannot be recalled.

So incremental streaming's guarantee is **bounded** exposure, not zero:

- Buffered mode: **zero** exposure, TTFT = full pipeline latency.
- Incremental mode: exposure limited to text that passed every output guard at
  release time, TTFT ≈ one window.

This is a genuine safety/latency tradeoff and must be a **per-source opt-in**,
not a global default. Sources that cannot tolerate any exposure keep buffered
mode. The docs must say this in exactly these terms rather than implying
streaming is free.

### Termination protocol

The response is already `200 text/event-stream`, so status codes are unavailable
mid-stream. Use the convention OpenAI SDKs already understand:

- emit a chunk with `finish_reason: "content_filter"` — standard OpenAI value,
  handled by existing SDKs with no Sheeld-specific code
- then a Sheeld error event carrying the phase, for clients that want detail
- then `data: [DONE]` so well-behaved SSE parsers close cleanly

`X-Sheeld-Status` is a header, so it is already committed by the time we stream;
it will read `pass` even on a mid-stream rejection. Worth noting in docs — the
authoritative signal for streams is `finish_reason`.

### Configuration

Per-source, defaulting to today's behavior so nothing changes for existing users:

| field | values | default |
|---|---|---|
| `stream_mode` | `buffered` \| `incremental` | `buffered` |
| `stream_window_bytes` | int | 256 |

Guard cadence is driven by `stream_window_bytes`. This matters for cost as much
as latency: a moderation or LLM-classifier guard fires once per window, so a
window of 32 would multiply provider guard calls. 256 bytes is a starting point
to be tuned against real traffic, not a researched constant.

### Interactions that must be handled

- **Output transformers.** `transform.ApplyAll` rewrites whole messages, and
  presidio `deanonymize` needs complete text. Incremental + output transformers
  is not safely composable in v1: when a source has output transformers,
  **fall back to buffered** and log why. Prevents a class of subtle corruption.
- **Usage/cost.** Providers omit usage from streams unless asked
  (`stream_options: {include_usage: true}`). Without it, `prompt_tokens` /
  `completion_tokens` go NULL and the new cost estimation silently
  under-reports. Must set it and parse the final usage chunk.
- **Client disconnect.** Must cancel the provider request via context, or a
  closed browser tab leaks an upstream stream and its tokens.
- **Guard errors mid-stream.** Reuse the existing per-guard `on_error`
  fail-open/fail-closed policy; fail-closed terminates the stream.
- **Shadow guards.** Must never block a release — same rule as non-streaming.
- **`n > 1` choices.** Multiple choices interleave in streams. v1 supports
  `n == 1` for incremental and falls back to buffered otherwise.

## Phasing

1. **Provider streaming in `llm.Client`** — `ChatCompletionStreamAt` yielding
   parsed deltas over a channel/iterator, with `include_usage`. Unit-testable
   against a fake SSE server, no pipeline changes.
2. **Guarded-release engine** — the buffer/window/release loop as a standalone,
   heavily unit-tested component. Pure logic, no HTTP.
3. **Processor + gateway wiring** — incremental path, fallbacks (transformers,
   `n > 1`), audit/alerts at stream end.
4. **Config plumbing** — migration, workspace-config payload, dashboard controls.
5. **Docs** — README streaming section rewritten around the two modes and the
   bounded-exposure wording.

Each phase is a reviewable PR; 1 and 2 carry the real risk and are pure-logic
testable.

## Testing

- Guarded release: violation in first window (nothing released), in a later
  window (prefix released, remainder withheld), never (full passthrough).
- Cumulative semantics: a blocklist phrase straddling a window boundary is still
  caught.
- Fallbacks: output transformers present → buffered; `n > 1` → buffered.
- Usage: final chunk parsed, audit row and cost match the non-streaming path for
  identical content.
- Disconnect: client abort cancels the upstream request.
- Integration: a real rejection mid-stream against mock-llm, asserting
  `finish_reason: "content_filter"` and that the violating text never appears in
  the response body.

## Open questions for sign-off

1. **Is bounded exposure acceptable at all**, given the opt-in? If not, this
   feature cannot exist and buffered mode is the honest final answer.
2. **Default window size** — 256 bytes is a guess. Tune later, or measure first?
3. **`stream_mode` per source, or per request** (an `X-Sheeld-Stream-Mode`
   header / body field)? Per-source is safer and is what this proposes; per
   request is more flexible but lets a client opt itself into weaker safety.
4. **Should `n > 1` and output transformers fall back silently, or 400?**
   Proposal is silent fallback plus a log line; an error is more honest but
   breaks working requests.
