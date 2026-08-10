# Nexus — Agentic Operations Platform

Multi-project AI agent platform for engineering and product-manager operations.

## Architecture

```
web/           Next.js 15 PM/Ops UI   (TypeScript + Tailwind)
server/        Go agent backend        (Gin + pgx + Redis)
  cmd/nexus    HTTP server entry point
  cmd/migrate  DB migration runner
  cmd/seed     Seed default data
  internal/
    config/    Environment config
    db/        PostgreSQL layer + migrations
    auth/      JWT authentication
    domain/    Core types (Run, Message, Proposal…)
    llm/       Multi-provider LLM adapters
    tools/     Tool registry + base interfaces
    adapters/  Per-project tool implementations
      offline_cashback/
    agent/     Agent orchestration loop
    api/       HTTP handlers + SSE streaming
```

## Quick start

```bash
cp .env.example .env          # fill in API keys + DB URLs
make setup                    # start infra, migrate, seed
make run-server               # terminal 1 → :8080
make run-web                  # terminal 2 → :3000
```

Default admin credentials (seeded):
- Email: `admin@nexus.local`
- Password: `nexus_admin_2024`

## Supported LLM providers

| Provider  | Key env var          | Default model           |
|-----------|----------------------|-------------------------|
| OpenAI    | `OPENAI_API_KEY`     | `gpt-4.1`               |
| Anthropic | `ANTHROPIC_API_KEY`  | `claude-sonnet-4-5`     |
| Google    | `GOOGLE_API_KEY`     | `gemini-2.5-pro`        |

Set any subset. The router skips providers with empty keys.

## Adding a new project adapter

1. Create `server/internal/adapters/<project>/adapter.go`
2. Implement `tools.Adapter` interface (`Name()`, `Tools()`, `Execute()`)
3. Register in `server/internal/adapters/registry.go`
4. Add a `Project` row in the database (or seed)

## API overview

```
POST /api/auth/login
GET  /api/auth/me

GET  /api/projects
GET  /api/agents

POST /api/runs                    start a run
GET  /api/runs                    list runs
GET  /api/runs/:id                run detail
GET  /api/runs/:id/stream         SSE: live message stream
POST /api/runs/:id/messages       send user message

GET  /api/proposals               pending action proposals
POST /api/proposals/:id/approve
POST /api/proposals/:id/reject

GET  /api/dashboard/cashback      bank health summary
```
