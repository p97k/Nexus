package llm

import (
	"context"
	"fmt"
	"strings"
)

// Candidate is a (provider, model) pair tried in order during a run.
type Candidate struct {
	Provider string
	Model    string
}

func (c Candidate) Key() string { return c.Provider + "/" + c.Model }

// Router holds all registered providers and dispatches to the right one.
type Router struct {
	providers map[string]Provider
}

func NewRouter() *Router {
	return &Router{providers: make(map[string]Provider)}
}

func (r *Router) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Router) Has(providerName string) bool {
	_, ok := r.providers[providerName]
	return ok
}

func (r *Router) Providers() []string {
	keys := make([]string, 0, len(r.providers))
	for k := range r.providers {
		keys = append(keys, k)
	}
	return keys
}

// DefaultModelFor returns the provider's default model, or "" if unknown.
func (r *Router) DefaultModelFor(provider string) string {
	if p, ok := r.providers[provider]; ok {
		return p.DefaultModel()
	}
	return ""
}

func (r *Router) Complete(ctx context.Context, provider, model string, messages []Message, tools []ToolDef) (*Response, error) {
	p, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", provider)
	}
	if model == "" {
		model = p.DefaultModel()
	}
	return p.Complete(ctx, model, messages, tools)
}

// CompleteWithFallback tries each candidate in order and returns the first
// successful response. It never returns a partial result: only a success or a
// combined error after every candidate has failed.
func (r *Router) CompleteWithFallback(ctx context.Context, candidates []Candidate, messages []Message, tools []ToolDef) (*Response, error) {
	var errs []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if c.Provider == "" {
			continue
		}
		if c.Model == "" {
			c.Model = r.DefaultModelFor(c.Provider)
		}
		if c.Model == "" {
			errs = append(errs, c.Provider+"/<unset>")
			continue
		}
		if seen[c.Key()] {
			continue
		}
		seen[c.Key()] = true

		resp, err := r.Complete(ctx, c.Provider, c.Model, messages, tools)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		errs = append(errs, c.Key()+": "+err.Error())
	}
	return nil, fmt.Errorf("all models failed: %s", strings.Join(errs, "; "))
}
