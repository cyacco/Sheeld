# Phase 3: LLM Proxy — LiteLLM Gateway + Core Flow

**Status**: In Progress
**Depends on**: Phase 1 (Foundation), Phase 2 (Guardrail Engine)

## Goal
Complete proxy flow — input guardrails → LLM call → output guardrails → response. This is the core value proposition of Sheeld: a single `POST /v1/proxy/:source_slug` call that validates, proxies, and re-validates.

## Key Decision: LiteLLM Gateway
Instead of building per-provider HTTP clients (OpenAI, Anthropic, etc.), Sheeld routes all LLM calls through **LiteLLM** — an LLM gateway that normalizes 100+ providers behind a single OpenAI-compatible API.

**Architecture**:
```
User's App → Sheeld API → Input Guards → LiteLLM Proxy → Any LLM Provider → Output Guards → Response
```

**Benefits**:
- Sheeld only builds ONE LLM client (OpenAI chat completions format)
- LiteLLM handles provider-specific translation (Anthropic messages → OpenAI format, etc.)
- Instant support for 100+ providers: OpenAI, Anthropic, Cohere, Mistral, Ollama, vLLM, Azure, Bedrock, etc.
- Model specified in LiteLLM format: `openai/gpt-4o`, `anthropic/claude-sonnet-4-20250514`, `ollama/llama3`, etc.

**How API keys work**:
- Each source stores its LLM API key (encrypted at rest, Phase 6)
- Sheeld passes the API key per-request to LiteLLM via the `Authorization` header
- LiteLLM forwards it to the appropriate provider

## Tasks

### 1. Add LiteLLM to Docker Compose
- Run `litellm` as a sidecar service
- Expose on an internal port (e.g., `4000`)
- No API key needed between Sheeld → LiteLLM (internal network)
- Config: `SHEELD_LLM_GATEWAY_URL=http://litellm:4000`

### 2. Build OpenAI-compatible LLM client (`internal/llm/`)
```go
type Client struct {
    baseURL    string // LiteLLM gateway URL
    httpClient *http.Client
}

type ChatRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    Temperature *float64  `json:"temperature,omitempty"`
    MaxTokens   *int      `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
    ID      string   `json:"id"`
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}
```
- Single client, talks OpenAI format to LiteLLM
- Passes provider API key via `Authorization: Bearer <key>` header
- Handles errors, timeouts, retries

### 3. Build proxy orchestrator (`internal/proxy/proxy.go`)
The core flow:
1. Look up source by slug + org ID
2. Load enabled destinations for that source
3. Separate destinations into input-phase and output-phase guards
4. Instantiate guards via the Registry
5. **Extract input text** from the chat request messages (last user message)
6. Run input guards (fan-out via Engine)
7. If input fails → return rejection with guard results (no LLM call, no tokens spent)
8. If input passes → call LiteLLM via the LLM client
9. **Extract output text** from the LLM response (assistant message content)
10. Run output guards (fan-out via Engine)
11. If output fails → return rejection with guard results + note that LLM was called
12. If output passes → return the LLM response
13. Write audit log entry regardless of outcome

### 4. Implement `POST /v1/proxy/:source_slug` handler
- Accepts OpenAI chat completions format body
- Authenticated via API key (already wired in Phase 1)
- Returns either the LLM response or a rejection payload
- Includes `X-Sheeld-Status: pass|fail` header

### 5. Wire audit log into proxy flow
- Record: source, org, input hash, per-guard results, overall result, latency
- Uses the `audit_logs` table and `CreateAuditLog` sqlc query from Phase 1

### 6. Tests
- Mock LiteLLM (httptest server returning OpenAI-format responses)
- Test full flow: input pass → LLM call → output pass → response
- Test input rejection (no LLM call made)
- Test output rejection (LLM called but response blocked)
- Test with different pass criteria (all, any, n_of_m)

## Files to Create
- `internal/llm/client.go` — OpenAI-compatible HTTP client for LiteLLM
- `internal/llm/types.go` — ChatRequest, ChatResponse, Message, etc.
- `internal/proxy/proxy.go` — Proxy orchestrator
- `internal/api/handler/proxy.go` — HTTP handler
- `internal/llm/client_test.go` — Client tests with mock server
- `internal/proxy/proxy_test.go` — Orchestrator tests
- Update `docker-compose.yaml` — Add LiteLLM service
- Update `internal/config/config.go` — Add `LLMGatewayURL` field

## API Response Format

### Success (input + output pass)
```json
{
  "status": "pass",
  "llm_response": {
    "id": "chatcmpl-...",
    "choices": [{ "message": { "role": "assistant", "content": "..." } }],
    "usage": { "prompt_tokens": 10, "completion_tokens": 50 }
  },
  "guard_results": {
    "input": { "passed": true, "results": [...] },
    "output": { "passed": true, "results": [...] }
  },
  "latency_ms": 1234
}
```

### Rejection (input fails — no LLM call, tokens saved)
```json
{
  "status": "rejected",
  "phase": "input",
  "guard_results": {
    "input": { "passed": false, "results": [...] }
  },
  "latency_ms": 45
}
```

### Rejection (output fails — LLM was called)
```json
{
  "status": "rejected",
  "phase": "output",
  "guard_results": {
    "input": { "passed": true, "results": [...] },
    "output": { "passed": false, "results": [...] }
  },
  "latency_ms": 2100
}
```

## Source Config Changes
With LiteLLM, the source model field uses LiteLLM format:
- `openai/gpt-4o`
- `anthropic/claude-sonnet-4-20250514`
- `ollama/llama3`
- `azure/gpt-4o` (for Azure OpenAI)

The existing `llm_provider` column becomes less critical since LiteLLM infers the provider from the model string. We keep it for display/filtering purposes.

## Key Considerations
- LiteLLM default timeout: 30s (configurable in Sheeld config)
- Sheeld → LiteLLM is internal (no auth needed on that hop)
- Provider API key goes from source config → Authorization header → LiteLLM → provider
- The proxy handler replaces the placeholder in `router.go` from Phase 1
- Guard phase filtering: destinations have a `phase` field ("input", "output", "both")
