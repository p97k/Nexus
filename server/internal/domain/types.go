package domain

import (
	"encoding/json"
	"time"
)

// ─── Users ────────────────────────────────────────────────────────────────────

type Role string

const (
	RoleAdmin Role = "admin"
	RoleOps   Role = "ops"
	RolePM    Role = "pm"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── Projects ─────────────────────────────────────────────────────────────────

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"` // e.g. "offline-cashback"
	Description string    `json:"description"`
	AdapterID   string    `json:"adapter_id"` // which adapter handles tools
	CreatedAt   time.Time `json:"created_at"`
}

// ─── Agents ───────────────────────────────────────────────────────────────────

type AgentMode string

const (
	AgentModeAuto   AgentMode = "auto"
	AgentModeManual AgentMode = "manual"
)

type Agent struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	SystemPrompt    string    `json:"system_prompt"`
	DefaultMode     AgentMode `json:"default_mode"`
	DefaultProvider string    `json:"default_provider"` // "openai"|"anthropic"|"google"
	DefaultModel    string    `json:"default_model"`
	AllowedTools    []string  `json:"allowed_tools"`
	MaxSteps        int       `json:"max_steps"`
	CreatedAt       time.Time `json:"created_at"`
}

// ─── Runs ─────────────────────────────────────────────────────────────────────

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type Run struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Status    RunStatus `json:"status"`
	Mode      string    `json:"mode"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	StepCount int       `json:"step_count"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── Messages ─────────────────────────────────────────────────────────────────

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

type Message struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	Role       MessageRole     `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ─── Tool Calls ───────────────────────────────────────────────────────────────

type ToolCallRecord struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	MessageID  string          `json:"message_id"`
	ToolName   string          `json:"tool_name"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	DurationMs int             `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ─── Action Proposals ─────────────────────────────────────────────────────────

type ProposalStatus string

const (
	ProposalStatusPending  ProposalStatus = "pending"
	ProposalStatusApproved ProposalStatus = "approved"
	ProposalStatusRejected ProposalStatus = "rejected"
	ProposalStatusExecuted ProposalStatus = "executed"
	ProposalStatusFailed   ProposalStatus = "failed"
)

type ActionProposal struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	ProjectID   string          `json:"project_id"`
	ToolName    string          `json:"tool_name"`
	Description string          `json:"description"`
	Params      json.RawMessage `json:"params"`
	Status      ProposalStatus  `json:"status"`
	ActedBy     string          `json:"acted_by,omitempty"`
	ActedAt     *time.Time      `json:"acted_at,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ─── Audit Logs ───────────────────────────────────────────────────────────────

type AuditLog struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Action    string          `json:"action"`
	Resource  string          `json:"resource"`
	ResourceID string         `json:"resource_id"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// ─── SSE Events ───────────────────────────────────────────────────────────────

type StreamEventType string

const (
	StreamEventMessage  StreamEventType = "message"
	StreamEventToolCall StreamEventType = "tool_call"
	StreamEventDone     StreamEventType = "done"
	StreamEventError    StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Payload any             `json:"payload"`
}
