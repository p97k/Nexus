# Nexus — Request Flow & Edge Cases

Visual walkthrough of one user prompt, end to end: from the moment the user
presses Enter in the chat UI until the final AI answer (and every failure mode
in between).

**What is "an agent" here?** A Nexus *agent* is a persona (system prompt +
allowed tools + model defaults) fixed for the whole run. When something fails,
Nexus does **not** switch agents — it switches **LLM model/provider** through a
dedicated fallback chain (see [§4](#4-llm-fallback-chain--switching-models)).
The run always keeps the same agent.

Components (see `ARCHITECTURE.md` for the full picture):

| Abbrev | Component | Code |
|---|---|---|
| `W` | Next.js chat page | `web/src/app/(app)/chat/page.tsx` |
| `A` | Go API (Gin) | `server/internal/api/server.go` |
| `S` | SSE sink (per-run fan-out) | `server/internal/api/sse.go` |
| `R` | Agent runner | `server/internal/agent/runner.go` |
| `D` | Nexus PostgreSQL | `server/internal/db/store.go` |
| `L` | LLM router + providers | `server/internal/llm/*.go` |
| `T` | Cashback tools | `server/internal/adapters/offline_cashback/adapter.go` |
| `CashDB` | Cashback PostgreSQL | optional, read-only |

---

## 1. End-to-end flow (happy path)

```mermaid
sequenceDiagram
    autonumber
    participant U as User (browser)
    participant W as Next.js chat page
    participant A as Go API (Gin)
    participant S as SSE Sink
    participant R as Agent Runner
    participant D as Nexus PostgreSQL
    participant L as LLM provider(s)
    participant T as Cashback tools
    participant CashDB as Cashback DB

    U->>W: type prompt + Enter
    W->>W: add optimistic "user" bubble<br/>(streaming = true)

    W->>A: POST /api/runs {agent_id}
    A->>D: INSERT run (status=pending)
    A-->>W: 200 {run}

    W->>A: GET /api/runs/:id/stream?token=JWT
    Note over W,A: SSE opened BEFORE sending the message so no early event is missed
    A->>S: sink.Subscribe() → client channel

    W->>A: POST /api/runs/:id/messages {content}
    A-->>W: 202 Accepted (handler returns immediately)

    A->>R: go runner.Execute (ctx timeout 5 min)
    R->>D: run = running
    R->>D: save user message
    R-->>S: SSE [message] user
    S-->>W: onmessage → replace optimistic bubble

    Note over R,T: build tool defs = adapter "offline-cashback" tools ∩ agent.AllowedTools

    loop step 0 .. maxSteps (default 15)
        R->>L: CompleteWithFallback(candidates, conversation, tools)
        L-->>R: text and/or tool_calls

        R->>D: save assistant message (+ tokens, provider/model)
        R-->>S: SSE [message] assistant

        alt no tool calls
            R->>R: break loop → run is finished
        else one or more tool calls
            loop each tool call
                R->>T: executeTool
                alt read-only tool
                    T->>CashDB: parameterized SELECT
                    CashDB-->>T: rows
                    T-->>R: JSON result
                    R->>D: save ToolCallRecord
                    R-->>S: SSE [tool_call] + SSE [message]
                else write tool
                    R->>D: create ActionProposal (status=pending)
                    R-->>S: SSE [tool_call] + SSE [message] "pending_approval"
                end
                R->>R: append tool result to conversation (LLM sees it next step)
            end
        end
    end

    R->>D: run = completed
    R-->>S: SSE [done] {run_id, step_count, tokens_in, tokens_out}
    S-->>W: onDone → streaming = false
    W-->>U: render final answer
```

Key files: `server.go:262` (sendMessage), `runner.go:116` (Execute),
`sse.go:66` (getOrCreateSink).

---

## 2. Frontend orchestration

```mermaid
flowchart TD
    A[User presses Enter] --> B{run already created?}
    B -- No, first message --> C[POST /api/runs]
    C --> D[open SSE stream]
    D --> E[POST message → 202]
    B -- Yes, follow-up message --> D

    E --> F[streaming = true]
    F --> G[watchdog poll every 3s:<br/>GET run + GET messages]

    G --> H{SSE event type}
    H -- message --> I[append message;<br/>user msg replaces optimistic bubble]
    H -- tool_call --> J[render tool indicator under last user msg]
    H -- done --> K[streaming = false]
    H -- error --> L[error banner]

    G --> M{SSE connection dropped?}
    M -- reconnects ≤ 8 times --> N[reconnect with capped backoff 1.5s → 20s]
    M -- 8 attempts exceeded --> O[error: Stream connection lost]
    M -- watchdog sees run completed/failed --> P[stop stream, reconcile from REST]
```

Frontend guarantees (from `chat/page.tsx` and `lib/api.ts`):

1. **No missed events:** SSE opens before the message POST.
2. **No double-send:** `sendingRef` guards double Enter in the same tick; the
   send button is disabled while `loading || streaming`.
3. **Optimistic UI:** temporary bubble is replaced once the persisted user
   message arrives over SSE (matched by content + `optimistic-` id prefix).
4. **Drop tolerance:** SSE reconnect with capped exponential backoff; a 3 s
   watchdog polls run status + messages and reconciles the transcript even if
   SSE is gone.
5. **Auth expiry:** any `401` clears `localStorage` and redirects to `/login`.

---

## 3. Agent loop decision tree (backend)

```mermaid
flowchart TD
    START[POST message → 202, goroutine starts] --> RUNNING[run = running]
    RUNNING --> LOAD[load prior messages<br/>assemble system prompt + conversation]
    LOAD --> LOOP{step < maxSteps?}
    LOOP -- no --> DONE[run = completed<br/>SSE done]
    LOOP -- yes -->     TIMEOUT{"ctx canceled?<br/>(5 min per turn)"}
    TIMEOUT -- yes --> FAIL1[run = failed<br/>SSE error: run timed out]
    TIMEOUT -- no --> LLM[CompleteWithFallback]
    LLM -- all candidates failed --> FAIL2[run = failed<br/>save ⚠ assistant message with error<br/>SSE error]
    LLM -- ok --> SAVE[save assistant message<br/>accumulate tokens<br/>update run.provider/model]
    SAVE --> CALLS{tool_calls returned?}
    CALLS -- none --> DONE
    CALLS -- some --> EXEC[for each tool call: executeTool]
    EXEC --> APPEND[append tool results to conversation]
    APPEND --> LOOP
```

`runner.go:196` — the loop; `runner.go:207` — step counter; `runner.go:214`
— LLM failure path; `runner.go:197` — timeout path; `runner.go:263` — normal
completion.

---

## 4. LLM fallback chain (switching models when one fails)

This is the answer to *"one agent fails, how does it switch?"* — it doesn't
switch agents, it walks an ordered list of `(provider, model)` candidates.

### 4.1 Candidate order

Built fresh on **every step** by `fallbackCandidates` (`runner.go:74`),
deduplicated by `provider/model`:

```mermaid
flowchart LR
    subgraph chain [ordered candidate list, first success wins]
        c1[1. LLM_FALLBACK_MODELS<br/>explicit list from env]
        c2["2. run's last-used provider/model<br/>(what answered the previous step)"]
        c3[3. agent's default provider/model]
        c4[4. Google free tier ×5<br/>gemini-3.5-flash, -lite, 3.1, 2.5, 2.0]
        c5[5. Groq free tier ×3<br/>llama-3.3-70b, llama-3.1-8b, gpt-oss-20b]
        c6[6. every registered provider's default model]
    end
    c1 --> c2 --> c3 --> c4 --> c5 --> c6
```

Candidates whose provider is **not registered** (no API key) or whose model is
empty are skipped. Position 2 is why a run that failed over to Groq keeps using
Groq on later steps instead of bouncing back to the broken primary.

### 4.2 Failure walk

```mermaid
flowchart TD
    START[try next candidate] --> TRY[provider.Complete<br/>translated to that vendor's API]
    TRY -- success (HTTP 200 + parseable) --> WINNER[return response<br/>runner stores actual provider/model<br/>step continues normally]
    TRY -- failure: quota exhausted /<br/>network / 4xx-5xx / parse error --> LOG[slog.Warn candidate failed]
    LOG --> MORE{candidates remain?}
    MORE -- yes --> START
    MORE -- no --> ERR[return combined error: all models failed<br/>→ runner marks run failed + SSE error]
```

`router.go:66` (`CompleteWithFallback`) never returns a partial result — it is
all-or-nothing per step. Per-step semantics: a *model* failure is handled by
this chain; a *tool* failure is **not** (see §5).

### 4.3 Why switching vendors works

The conversation is provider-neutral (`llm.Message` / `llm.ToolDef`). Each
adapter (`openai.go`, `anthropic.go`, `google.go`) translates it:

| Concern | OpenAI-compatible | Anthropic | Google Gemini |
|---|---|---|---|
| System prompt | `system` message | `system` field | `systemInstruction` |
| Tool schema | `tools[].function` | `tools[].input_schema` | `tools[].functionDeclarations` |
| Tool results | `tool` role message | `tool_result` content block | `functionResponse` part |
| Assistant w/ calls | `tool_calls` array | `tool_use` blocks | `functionCall` parts |
| Thinking models | — | — | `thoughtSignature` echoed back verbatim (stored in `packToolCalls`) |

The Google thought signature survives the DB round-trip
(`runner.go:328` `packToolCalls`), so a multi-turn Gemini tool conversation can
be reconstructed even after a restart or a provider switch back to Gemini.

---

## 5. Tool execution: read vs write (human-in-the-loop)

```mermaid
flowchart TD
    EXEC[executeTool] --> GET{known tool?}
    GET -- unknown --> FEED1[error JSON fed back to LLM as tool message]
    GET -- known --> RO{ReadOnly flag?}

    RO -- true, read-only --> RUN1[execute immediately]
    RUN1 --> REC[record ToolCallRecord:<br/>input, output, duration_ms, error]
    REC --> SSE1[SSE tool_call + message]

    RO -- false, write tool --> PROP[create ActionProposal<br/>status=pending, params stored]
    PROP --> SSE2[SSE tool_call + message:<br/>status=pending_approval<br/>proposal_id]
    SSE2 --> FEED2[pending_approval JSON fed back to LLM<br/>run continues; proposal waits for a human]
```

**Key point:** a tool *error* is not fatal. The error string is wrapped in
`{"error": ...}` and returned to the LLM as a normal tool result, so the model
can correct itself on the next step (`runner.go:279`).

Write tools never touch the Cashback DB from the runner. Only `unlock_in_progress`
is a write tool today (`adapter.go:614`). Read tools are cached in-memory by
hash of name+input (`adapter.go:62`); errors are never cached.

### 5.1 Dynamic SQL guard (edge case)

`query_cashback_db` runs model-authored SQL, filtered by `sanitizeSQL`
(`adapter.go:445`):

- must start with `SELECT` / `WITH`, no `;`, no write/DDL keywords
- FROM/JOIN tables must be in the whitelist (`users_card*`)
- LIMIT auto-appended if missing; result capped at 500 rows; 20 s query timeout

Violations become tool errors fed back to the model — the run keeps going.

---

## 6. Proposal lifecycle (approval/rejection)

```mermaid
stateDiagram-v2
    [*] --> pending: write tool invoked during a run
    pending --> approved: POST /proposals/:id/approve<br/>(atomic claim wins)
    pending --> rejected: POST /proposals/:id/reject<br/>(audit written)
    approved --> executed: tool executes OK (audit written)
    approved --> failed: tool not found or execution error (audit written)

    note right of pending
      Claim is atomic (UPDATE ... WHERE status='pending').
      Losing concurrent approval gets HTTP 409.
    end note
```

```mermaid
sequenceDiagram
    autonumber
    participant W as Proposals page
    participant A as Go API
    participant D as Nexus PostgreSQL
    participant T as Tool registry

    W->>A: POST /proposals/:id/approve
    A->>D: ClaimProposal (atomic UPDATE pending→approved)
    D-->>A: claimed?
    alt not claimed
        A-->>W: 409 "proposal already acted on"
    else claimed
        A->>T: look up tool by proposal.tool_name
        alt tool missing
            A->>D: status = failed
            A-->>W: 422 "tool not found"
        else tool runs
            A->>T: tool.Execute(proposal.params)
            T-->>A: result or error
            alt error
                A->>D: status = failed (+ error in result)
                A-->>W: 500 error
            else success
                A->>D: status = executed (+ result)
                A->>D: audit log "approve_proposal"
                A-->>W: 200 {status: executed, result}
            end
        end
    end
```

`server.go:352` (approve), `server.go:391` (reject), `store.go:354` (claim).

---

## 7. Run state machine

```mermaid
stateDiagram-v2
    [*] --> pending: POST /api/runs (agent + provider/model selected)
    pending --> running: first message POST → goroutine starts
    running --> completed: final LLM turn with no tool calls
    running --> failed: all LLM candidates failed
    running --> failed: ctx timeout / cancellation
    running --> failed: tool execution error on read path

    note right of running
      Edge: an early DB error (load/save) returns before any terminal
      state is written → run can be left "running" (known gap, §8).
    end note
```

---

## 8. Edge-case catalog

| # | Situation | Behavior | Where |
|---|---|---|---|
| 1 | Primary LLM quota/network/parse error | Next fallback candidate tried, warning logged | `router.go:66`, `runner.go:209` |
| 2 | **Every** LLM candidate fails | Run → `failed`; ⚠ assistant message saved; SSE `error` | `runner.go:214` |
| 3 | Per-turn 5 min timeout / context cancel | Run → `failed`; SSE `error "run timed out"` | `runner.go:197`, `server.go:288` |
| 4 | Provider HTTP call hangs | 90 s client timeout per provider | each `llm/*.go` |
| 5 | LLM asks for unknown tool | Error JSON returned as tool result; run continues | `runner.go:363` |
| 6 | Read tool DB error / Cashback DB not configured | `{"error": ...}` fed back to model; run continues | `adapter.go:53` |
| 7 | Model-written SQL is unsafe | `query_cashback_db` rejects it as a tool error | `adapter.go:445` |
| 8 | Write tool requested | Proposal created (pending); model gets `pending_approval`; **no DB write** | `runner.go:378` |
| 9 | Concurrent approvals of same proposal | Atomic claim: one winner, loser gets 409 | `store.go:354` |
| 10 | Proposal tool missing / execution error | Proposal → `failed` + audit; HTTP 422/500 | `server.go:376` |
| 11 | SSE connection drops | Reconnect ≤ 8× w/ backoff; 3 s watchdog reconciles transcript | `api.ts:145`, `chat/page.tsx:71` |
| 12 | Slow SSE consumer | Events silently dropped (buffered chan 32) — run still persists | `sse.go:53` |
| 13 | JWT expired / invalid | Frontend clears auth → redirect `/login` | `api.ts:31` |
| 14 | Same run sent twice concurrently | **Not guarded** — interleaved runners possible (gap) | `server.go:262` |
| 15 | Process restart mid-run | Active goroutine lost; run stays `running` forever (gap) | `server.go:288` |
| 16 | Run detail page load | REST fetch of persisted messages (SSE has no replay) | `api.ts:106` |
| 17 | Run/proposal owned by another user | **No resource authorization** — any authenticated user can read/act (gap) | `server.go:244` |
| 18 | Google thinking model tool turn | `thoughtSignature` persisted & echoed to avoid 400 | `runner.go:328`, `google.go:97` |
| 19 | First turn (no run yet) | Frontend creates run, opens SSE, then posts message | `chat/page.tsx:119` |

Gaps #14–#17 are documented known limitations (see `ARCHITECTURE.md` §8).

---

## 9. Timeouts & limits reference

| Bound | Value | Set by |
|---|---|---|
| Whole turn (message → response) | 5 min | `server.go:288` |
| Provider HTTP call | 90 s | `llm/openai.go:36`, `anthropic.go:23`, `google.go:24` |
| Ad-hoc `query_cashback_db` | 20 s | `adapter.go:569` |
| Max agent steps per turn | `agent.MaxSteps` (default 15) | `runner.go:191` |
| Max rows returned by dynamic SQL | 500 | `adapter.go:583` |
| Adapter read-tool cache TTL | `CASHBACK_CACHE_TTL` (default 60 s) | `config.go:79` |
| SSE heartbeat | every 15 s (`: ping`) | `server.go:319` |
| SSE reconnect | max 8 attempts, 1.5 s→20 s backoff | `api.ts:142` |
| Watchdog poll while streaming | every 3 s | `chat/page.tsx:91` |
