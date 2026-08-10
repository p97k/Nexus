package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"nexus/internal/db"
	"nexus/internal/domain"
	"nexus/internal/llm"
	"nexus/internal/tools"

	"github.com/google/uuid"
)

// EventSink receives real-time events during a run.
type EventSink interface {
	Send(event domain.StreamEvent)
}

// Runner orchestrates a single agent run: user message → LLM → tools → LLM → …
type Runner struct {
	store    *db.Store
	llmR     *llm.Router
	toolReg  *tools.Registry
	fallback []llm.Candidate
}

func NewRunner(store *db.Store, llmR *llm.Router, toolReg *tools.Registry, fallbackModels []string) *Runner {
	return &Runner{
		store:    store,
		llmR:     llmR,
		toolReg:  toolReg,
		fallback: parseFallback(fallbackModels),
	}
}

// parseFallback converts "provider:model" strings into candidates.
func parseFallback(raw []string) []llm.Candidate {
	var out []llm.Candidate
	for _, s := range raw {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, llm.Candidate{Provider: strings.TrimSpace(parts[0]), Model: strings.TrimSpace(parts[1])})
	}
	return out
}

// Free-tier sibling models per provider, tried in order after the configured
// model fails (e.g. quota exhausted). These keep the demo running without a
// paid plan: Google Gemini has a free tier, Groq has a free tier.
var googleFreeModels = []string{
	"gemini-3.5-flash",
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-2.5-flash-lite",
	"gemini-2.0-flash-lite",
}

var groqFreeModels = []string{
	"llama-3.3-70b-versatile",
	"llama-3.1-8b-instant",
	"gpt-oss-20b",
}

// fallbackCandidates builds the ordered (provider, model) chain tried for each
// LLM step: explicit config → this run's last-used model → agent defaults →
// free-tier sibling models → every registered provider's default model.
func (r *Runner) fallbackCandidates(run *domain.Run, agent *domain.Agent) []llm.Candidate {
	var chain []llm.Candidate
	seen := map[string]bool{}
	add := func(p, m string) {
		if p == "" {
			return
		}
		key := p + "/" + m
		if seen[key] {
			return
		}
		seen[key] = true
		chain = append(chain, llm.Candidate{Provider: p, Model: m})
	}

	for _, c := range r.fallback {
		add(c.Provider, c.Model)
	}
	add(run.Provider, run.Model)
	add(agent.DefaultProvider, agent.DefaultModel)
	for _, m := range googleFreeModels {
		add("google", m)
	}
	for _, m := range groqFreeModels {
		add("groq", m)
	}
	for _, p := range r.llmR.Providers() {
		add(p, r.llmR.DefaultModelFor(p))
	}
	return chain
}

// RunOptions configures a single execution pass.
type RunOptions struct {
	Run     *domain.Run
	Agent   *domain.Agent
	UserMsg string // new user message
	Sink    EventSink
}

// Execute runs the agent loop for a single user message.
// It saves all messages + tool call records to the DB and emits SSE events.
func (r *Runner) Execute(ctx context.Context, opts RunOptions) error {
	run := opts.Run
	agent := opts.Agent

	run.Status = domain.RunStatusRunning
	r.store.UpdateRun(ctx, run)

	// Build the tool list for this agent (adapter_id matches the offline_cashback constant)
	// We look up by the offline_cashback adapter ID; in future this comes from the project record.
	adapterTools := r.toolReg.ToolsForAdapter("offline-cashback")
	var allowedSet map[string]bool
	if len(agent.AllowedTools) > 0 {
		allowedSet = make(map[string]bool)
		for _, n := range agent.AllowedTools {
			allowedSet[n] = true
		}
	}

	var toolDefs []llm.ToolDef
	for _, t := range adapterTools {
		if allowedSet != nil && !allowedSet[t.Name] {
			continue
		}
		toolDefs = append(toolDefs, llm.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	// Load previous messages for context
	prevMsgs, err := r.store.GetMessages(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}

	// Assemble LLM conversation
	var conversation []llm.Message
	conversation = append(conversation, llm.Message{
		Role:    "system",
		Content: agent.SystemPrompt,
	})
	for _, m := range prevMsgs {
		lm := llm.Message{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
		}
		if len(m.ToolCalls) > 0 {
			lm.ToolCalls, lm.ThoughtSignature = unpackToolCalls(m.ToolCalls)
		}
		conversation = append(conversation, lm)
	}

	// Save and emit user message
	userMsg := &domain.Message{
		ID:      uuid.NewString(),
		RunID:   run.ID,
		Role:    domain.MessageRoleUser,
		Content: opts.UserMsg,
	}
	if err := r.store.SaveMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	if opts.Sink != nil {
		opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventMessage, Payload: userMsg})
	}

	conversation = append(conversation, llm.Message{
		Role:    "user",
		Content: opts.UserMsg,
	})

	// Agent loop
	maxSteps := agent.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 15
	}

	for step := 0; step < maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			run.Status = domain.RunStatusFailed
			r.store.UpdateRun(ctx, run)
			if opts.Sink != nil {
				opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventError, Payload: map[string]string{
					"error": "run timed out",
				}})
			}
			return fmt.Errorf("run %s: %w", run.ID, err)
		}
		run.StepCount++

		llmResp, err := r.llmR.CompleteWithFallback(ctx, r.fallbackCandidates(run, agent), conversation, toolDefs)
		if err != nil {
			run.Status = domain.RunStatusFailed
			r.store.UpdateRun(ctx, run)

			// Save the error as an assistant message so the UI shows it
			errMsg := &domain.Message{
				ID:      uuid.NewString(),
				RunID:   run.ID,
				Role:    domain.MessageRoleAssistant,
				Content: fmt.Sprintf("⚠️ Agent error: %s", err.Error()),
			}
			r.store.SaveMessage(ctx, errMsg)
			if opts.Sink != nil {
				opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventMessage, Payload: errMsg})
				opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventError, Payload: map[string]string{
					"error": err.Error(),
				}})
			}
			return fmt.Errorf("llm complete step %d: %w", step, err)
		}

		run.TokensIn += llmResp.TokensIn
		run.TokensOut += llmResp.TokensOut
		run.Provider = llmResp.Provider
		run.Model = llmResp.Model

		// The fallback chain may have answered from a different model than the
		// agent's configured one — surface that in the log.
		if llmResp.Provider != agent.DefaultProvider || llmResp.Model != agent.DefaultModel {
			slog.Info("llm fallback used", "run", run.ID, "step", step,
				"configured", agent.DefaultProvider+"/"+agent.DefaultModel,
				"used", llmResp.Provider+"/"+llmResp.Model)
		}

		// Build and save assistant message
		toolCallsJSON := packToolCalls(llmResp.ToolCalls, llmResp.ThoughtSignature)
		assistantMsg := &domain.Message{
			ID:        uuid.NewString(),
			RunID:     run.ID,
			Role:      domain.MessageRoleAssistant,
			Content:   llmResp.Content,
			ToolCalls: toolCallsJSON,
		}
		r.store.SaveMessage(ctx, assistantMsg)
		if opts.Sink != nil {
			opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventMessage, Payload: assistantMsg})
		}

		// No tool calls → we're done
		if len(llmResp.ToolCalls) == 0 {
			break
		}

		// Add assistant turn to conversation (include thought signature for Google)
		conversation = append(conversation, llm.Message{
			Role:             "assistant",
			Content:          llmResp.Content,
			ToolCalls:        llmResp.ToolCalls,
			ThoughtSignature: llmResp.ThoughtSignature,
		})

		// Execute each tool call
		for _, tc := range llmResp.ToolCalls {
			toolResult, toolErr := r.executeTool(ctx, run, agent, assistantMsg.ID, tc, opts.Sink)

			var resultContent string
			if toolErr != nil {
				resultContent = fmt.Sprintf(`{"error": %q}`, toolErr.Error())
			} else {
				resultContent = string(toolResult)
			}

			toolMsg := &domain.Message{
				ID:         uuid.NewString(),
				RunID:      run.ID,
				Role:       domain.MessageRoleTool,
				Content:    resultContent,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			}
			r.store.SaveMessage(ctx, toolMsg)
			if opts.Sink != nil {
				opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventMessage, Payload: toolMsg})
			}

			conversation = append(conversation, llm.Message{
				Role:       "tool",
				Content:    resultContent,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			})
		}
	}

	run.Status = domain.RunStatusCompleted
	r.store.UpdateRun(ctx, run)

	if opts.Sink != nil {
		opts.Sink.Send(domain.StreamEvent{Type: domain.StreamEventDone, Payload: map[string]any{
			"run_id":     run.ID,
			"step_count": run.StepCount,
			"tokens_in":  run.TokensIn,
			"tokens_out": run.TokensOut,
		}})
	}
	return nil
}

// packToolCalls serialises tool calls for DB storage, embedding an optional
// thought signature (used by Google Gemini thinking models) so it survives the
// round-trip and can be echoed back on the next LLM request.
//
// Format when signature present: {"_ts":"<sig>","calls":[...]}
// Format otherwise:              [...] (plain array, backward-compatible)
func packToolCalls(calls []llm.ToolCall, thoughtSignature string) json.RawMessage {
	if thoughtSignature == "" {
		b, _ := json.Marshal(calls)
		return b
	}
	b, _ := json.Marshal(struct {
		TS    string         `json:"_ts"`
		Calls []llm.ToolCall `json:"calls"`
	}{TS: thoughtSignature, Calls: calls})
	return b
}

// unpackToolCalls is the inverse of packToolCalls.
func unpackToolCalls(raw json.RawMessage) (calls []llm.ToolCall, thoughtSignature string) {
	// Try the wrapper format first.
	var wrapper struct {
		TS    string         `json:"_ts"`
		Calls []llm.ToolCall `json:"calls"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Calls != nil {
		return wrapper.Calls, wrapper.TS
	}
	// Fall back to plain array (old data or non-thinking models).
	json.Unmarshal(raw, &calls)
	return calls, ""
}

func (r *Runner) executeTool(
	ctx context.Context,
	run *domain.Run,
	agent *domain.Agent,
	msgID string,
	tc llm.ToolCall,
	sink EventSink,
) (json.RawMessage, error) {
	tool, ok := r.toolReg.Get(tc.Name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	// Emit tool call event
	if sink != nil {
		sink.Send(domain.StreamEvent{Type: domain.StreamEventToolCall, Payload: map[string]any{
			"tool_name": tc.Name,
			"input":     tc.Input,
			"read_only": tool.ReadOnly,
		}})
	}

	// Write tools: create an ActionProposal and return pending notice
	if !tool.ReadOnly {
		proposal := &domain.ActionProposal{
			ID:          uuid.NewString(),
			RunID:       run.ID,
			ProjectID:   agent.ProjectID,
			ToolName:    tc.Name,
			Description: fmt.Sprintf("Agent requested write tool '%s'", tc.Name),
			Params:      tc.Input,
			Status:      domain.ProposalStatusPending,
		}
		if err := r.store.CreateProposal(ctx, proposal); err != nil {
			slog.Error("create proposal", "err", err)
		}
		return json.Marshal(map[string]any{
			"status":      "pending_approval",
			"proposal_id": proposal.ID,
			"message":     "This action requires PM approval. The proposal has been created.",
		})
	}

	// Execute read-only tool
	start := time.Now()
	result, err := tool.Execute(ctx, tc.Input)
	durationMs := int(time.Since(start).Milliseconds())

	record := &domain.ToolCallRecord{
		ID:         uuid.NewString(),
		RunID:      run.ID,
		MessageID:  msgID,
		ToolName:   tc.Name,
		Input:      tc.Input,
		Output:     result,
		DurationMs: durationMs,
	}
	if err != nil {
		record.Error = err.Error()
	}
	r.store.SaveToolCall(ctx, record)

	return result, err
}
