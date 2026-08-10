package tools

import (
	"context"
	"encoding/json"
)

// Tool is a single callable function an agent can invoke.
type Tool struct {
	Name        string
	Description string
	// Parameters is a JSON Schema object describing the tool's input.
	Parameters json.RawMessage
	// ReadOnly indicates the tool has no side effects.
	// Write tools require an ActionProposal and PM approval.
	ReadOnly bool
	Execute  func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// Adapter is the interface each project plugin must implement.
type Adapter interface {
	// AdapterID returns the unique ID used in the projects table.
	AdapterID() string
	// Tools returns all tools this adapter provides.
	Tools() []*Tool
}

// Registry holds all adapters and tools.
type Registry struct {
	adapters map[string]Adapter
	byName   map[string]*Tool
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
		byName:   make(map[string]*Tool),
	}
}

func (r *Registry) Register(a Adapter) {
	r.adapters[a.AdapterID()] = a
	for _, t := range a.Tools() {
		r.byName[t.Name] = t
	}
}

func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

func (r *Registry) ToolsForAdapter(adapterID string) []*Tool {
	a, ok := r.adapters[adapterID]
	if !ok {
		return nil
	}
	return a.Tools()
}
