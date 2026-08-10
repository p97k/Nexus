-- Nexus platform schema

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── Users ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'pm',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Projects ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS projects (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    description  TEXT,
    adapter_id   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Agents ───────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS agents (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    description      TEXT,
    system_prompt    TEXT NOT NULL,
    default_mode     TEXT NOT NULL DEFAULT 'auto',
    default_provider TEXT NOT NULL DEFAULT 'anthropic',
    default_model    TEXT NOT NULL DEFAULT 'claude-sonnet-4-5',
    allowed_tools    JSONB NOT NULL DEFAULT '[]',
    max_steps        INT NOT NULL DEFAULT 15,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Runs ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS runs (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id TEXT NOT NULL REFERENCES projects(id),
    agent_id   TEXT NOT NULL REFERENCES agents(id),
    user_id    TEXT NOT NULL REFERENCES users(id),
    title      TEXT NOT NULL DEFAULT 'Untitled investigation',
    status     TEXT NOT NULL DEFAULT 'pending',
    mode       TEXT NOT NULL DEFAULT 'auto',
    provider   TEXT NOT NULL DEFAULT '',
    model      TEXT NOT NULL DEFAULT '',
    step_count INT NOT NULL DEFAULT 0,
    tokens_in  INT NOT NULL DEFAULT 0,
    tokens_out INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_runs_user_id ON runs(user_id);
CREATE INDEX IF NOT EXISTS idx_runs_project_id ON runs(project_id);

-- ─── Messages ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS messages (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    content      TEXT NOT NULL DEFAULT '',
    tool_calls   JSONB,
    tool_call_id TEXT,
    tool_name    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_run_id ON messages(run_id);

-- ─── Tool Call Records ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tool_call_records (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    message_id  TEXT NOT NULL,
    tool_name   TEXT NOT NULL,
    input       JSONB NOT NULL DEFAULT '{}',
    output      JSONB,
    duration_ms INT NOT NULL DEFAULT 0,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_run_id ON tool_call_records(run_id);

-- ─── Action Proposals ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS action_proposals (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    tool_name   TEXT NOT NULL,
    description TEXT NOT NULL,
    params      JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'pending',
    acted_by    TEXT,
    acted_at    TIMESTAMPTZ,
    result      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proposals_status ON action_proposals(status);

-- ─── Audit Logs ───────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_logs (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id     TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
