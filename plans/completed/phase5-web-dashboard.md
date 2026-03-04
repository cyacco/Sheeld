# Phase 5: Web Dashboard

**Status**: Pending
**Depends on**: Phase 1 (Foundation API), Phase 3 (LLM Proxy for stats)

## Goal
Basic Next.js web dashboard for managing sources and destinations visually. Users can configure their guardrail pipeline without writing API calls.

## Tech Stack
- **Next.js** (App Router) — React framework
- **Tailwind CSS** — Utility-first styling
- **shadcn/ui** — Component library (built on Radix + Tailwind)
- **Recharts** — Charts for analytics/stats

## Tasks

### 1. Initialize Next.js project
- Create `web/` directory with Next.js, Tailwind, shadcn/ui
- Configure API client to talk to the Go backend
- Set up JWT auth flow (store token in httpOnly cookie or localStorage)

### 2. Auth pages
- `/login` — Email + password form → `POST /v1/auth/login`
- `/register` — Org name + email + password → `POST /v1/auth/register`
- Auth context/provider for protected routes
- Redirect unauthenticated users to login

### 3. Dashboard home
- `/dashboard` — List all sources with status badges (enabled/disabled)
- Show destination count per source
- Quick stats: total requests, pass rate (from audit logs)
- "Create Source" button → source creation form

### 4. Source detail page
- `/dashboard/sources/:id` — Full source config
- Edit source fields (name, slug, LLM provider, model, pass criteria)
- List attached destinations with enable/disable toggles
- "Add Destination" button → destination creation form

### 5. Destination config forms
Type-specific configuration UIs:
- **Blocklist**: Textarea for words (one per line), block/allow mode toggle
- **Regex**: Pattern input with live preview/test, block/require mode
- **OpenAI Moderation**: Category checkboxes, threshold slider
- **guardrails.ai**: Server URL, guard name, timeout

### 6. API Key management
- `/dashboard/api-keys` — List keys (showing prefix only), create new, revoke
- Show the full key only once at creation time (with copy button)

### 7. Audit log viewer
- `/dashboard/audit-logs` — Paginated list of proxy requests
- Filters: source, date range, pass/fail status
- Expandable rows showing per-guard results

### 8. Stats / charts
- Pass/fail rate over time (line chart)
- Per-source breakdown (bar chart)
- Average latency trends
- Guard-level failure breakdown

### 9. API client library
- `web/src/lib/api.ts` — Typed fetch wrapper for all Sheeld API endpoints
- Handles JWT refresh, error responses
- Type definitions matching Go API responses

## Files to Create
```
web/
├── package.json
├── next.config.js
├── tailwind.config.js
├── tsconfig.json
├── src/
│   ├── app/
│   │   ├── layout.tsx          # Root layout with auth provider
│   │   ├── page.tsx            # Landing → redirect to dashboard
│   │   ├── login/page.tsx
│   │   ├── register/page.tsx
│   │   └── dashboard/
│   │       ├── layout.tsx      # Dashboard layout with sidebar
│   │       ├── page.tsx        # Source list
│   │       ├── sources/[id]/page.tsx
│   │       ├── api-keys/page.tsx
│   │       └── audit-logs/page.tsx
│   ├── components/
│   │   ├── source-card.tsx
│   │   ├── destination-form.tsx
│   │   ├── guard-config/       # Per-type config components
│   │   │   ├── blocklist.tsx
│   │   │   ├── regex.tsx
│   │   │   ├── openai-mod.tsx
│   │   │   └── guardrails-ai.tsx
│   │   └── ui/                 # shadcn/ui components
│   └── lib/
│       ├── api.ts              # API client
│       ├── auth.tsx            # Auth context/provider
│       └── types.ts            # TypeScript type definitions
```

## Key Considerations
- CORS is already configured in the Go API (Phase 1) for `http://localhost:3000`
- JWT token stored client-side — consider httpOnly cookies for production
- Dashboard is a separate process from the Go API (different port)
- Docker Compose in Phase 6 will serve both behind a reverse proxy
