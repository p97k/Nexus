package llm

import (
	"context"
	"encoding/json"
)

// ToolDef is the universal tool definition passed to any provider.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// Message is the normalized chat message.
type Message struct {
	Role       string     `json:"role"` // "system"|"user"|"assistant"|"tool"
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	// ThoughtSignature is Google Gemini-specific: thinking models attach an
	// opaque signature to tool-call turns that must be echoed back verbatim
	// on the next request, otherwise the API returns 400.
	ThoughtSignature string `json:"-"`
}

// ToolCall is a normalized LLM-requested tool invocation.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Response is the normalized LLM completion response.
type Response struct {
	Content          string     `json:"content"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	TokensIn         int        `json:"tokens_in"`
	TokensOut        int        `json:"tokens_out"`
	Provider         string     `json:"provider"`
	Model            string     `json:"model"`
	ThoughtSignature string     `json:"-"` // see Message.ThoughtSignature
}

// Provider is the interface every LLM adapter must implement.
type Provider interface {
	Name() string
	DefaultModel() string
	Complete(ctx context.Context, model string, messages []Message, tools []ToolDef) (*Response, error)
}
