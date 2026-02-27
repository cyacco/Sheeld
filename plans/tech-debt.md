# Tech Debt Tracker

Items logged here should be addressed before production. Each item includes context on why it was deferred and what the fix looks like.

---

## Active Items

### 1. LLM API Key Encryption at Rest
- **Location**: `internal/service/source.go` (Create + Update methods)
- **Issue**: LLM API keys are stored in plaintext in the `sources.llm_api_key_enc` column
- **Fix**: Implement AES-256-GCM encryption using `SHEELD_ENCRYPTION_KEY` before storing, decrypt when reading for proxy use
- **Target**: Phase 6 (Production Readiness)
- **Risk**: HIGH — must be resolved before any production deployment

### 2. No Integration Tests
- **Location**: Project-wide
- **Issue**: No tests exist yet. Phase 1 was focused on getting the structure right.
- **Fix**: Add testcontainers-go based integration tests for all handlers and services
- **Target**: End of Phase 1 or Phase 2
- **Risk**: MEDIUM — tests should be added before the codebase grows much larger

---

## Resolved Items

_None yet._
