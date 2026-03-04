# Phase 4: External Guardrail Integrations

**Status**: Pending
**Depends on**: Phase 2 (Guardrail Engine), Phase 3 (LLM Proxy)

## Goal
Integrate external guardrail services — OpenAI Moderation API and guardrails.ai — into the guard system. These register as new factory types in the existing Registry.

## Tasks

### 1. Implement OpenAI Moderation Guard
- Calls `POST https://api.openai.com/v1/moderations`
- Sends the input text to OpenAI's moderation endpoint
- Checks response categories against user-configured thresholds
- Config format:
  ```json
  {
    "api_key": "sk-...",
    "categories": ["hate", "violence", "sexual"],
    "threshold": 0.7
  }
  ```
- Pass if all configured category scores are below the threshold
- Fail with details showing which categories exceeded the threshold

### 2. Implement guardrails.ai Guard
- Calls a user-hosted guardrails.ai HTTP server (started via `guardrails start`)
- `POST /guards/{guard_name}/validate` with the input text
- Config format:
  ```json
  {
    "server_url": "http://guardrails:8000",
    "guard_name": "my-guard",
    "timeout_seconds": 10
  }
  ```
- Pass/fail based on the guardrails.ai server response
- Must handle connection errors gracefully (configurable: fail-open vs fail-closed)

### 3. Register in the guard Registry
- Add `openai_moderation` and `guardrails_ai` factory functions
- Register them in `NewRegistry()` alongside blocklist and regex

### 4. Integration tests
- Use `httptest.Server` to mock external APIs
- Test OpenAI moderation: pass, fail (threshold exceeded), API error
- Test guardrails.ai: pass, fail, connection timeout, server error
- Test fail-open vs fail-closed behavior

## Files to Create
- `internal/guard/openai_mod.go` — OpenAI Moderation guard
- `internal/guard/openai_mod_test.go` — Tests with mock HTTP server
- `internal/guard/guardrails_ai.go` — guardrails.ai guard
- `internal/guard/guardrails_ai_test.go` — Tests with mock HTTP server
- Update `internal/guard/registry.go` — Register new types

## Key Considerations
- OpenAI moderation API key may differ from the LLM API key (stored in destination config, not source)
- guardrails.ai is self-hosted by the user — Sheeld just calls the HTTP API
- Both guards need configurable HTTP timeouts
- Consider fail-open (treat error as pass) vs fail-closed (treat error as fail) per destination
