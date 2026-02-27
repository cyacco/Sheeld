# Sheeld — Master Implementation Plan

## Overview
Sheeld is a "Segment for LLM guardrails" — a full LLM proxy. Users configure sources and attach guardrail destinations. Sheeld validates input → proxies the LLM call → validates output → returns the result.

## Phases

| Phase | Name | Status | Description |
|-------|------|--------|-------------|
| 1 | Foundation | ✅ Complete | Go project, database, core CRUD API, auth |
| 2 | Guardrail Engine | ✅ Complete | Guard interface, blocklist, regex, fan-out executor |
| 3 | LLM Proxy | ✅ Complete | LiteLLM gateway, proxy orchestrator, full flow |
| 4 | External Integrations | ⏳ Pending | OpenAI Moderation, guardrails.ai |
| 5 | Web Dashboard | ⏳ Pending | Next.js + Tailwind + shadcn/ui |
| 6 | Production Readiness | ⏳ Pending | Docker, CI/CD, monitoring, security |

## Architecture
```
User's App → Sheeld API → Input Guards (fan-out) → LLM Provider → Output Guards (fan-out) → Response
```

## Key Decisions
- **Full proxy**: input validation → LLM call → output validation
- **Configurable fan-out**: all/any/N-of-M pass criteria
- **SaaS-first**: multi-tenant, self-hosted later
- **sqlc + pgx**: type-safe SQL, no ORM
- **chi**: lightweight idiomatic HTTP router
- **LiteLLM**: LLM gateway for 100+ providers via single OpenAI-compatible client
