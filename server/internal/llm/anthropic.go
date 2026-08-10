package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Anthropic struct {
	apiKey       string
	defaultModel string
	client       *http.Client
}

func NewAnthropic(apiKey, defaultModel string) *Anthropic {
	return &Anthropic{
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 90 * time.Second},
	}
}

func (a *Anthropic) Name() string         { return "anthropic" }
func (a *Anthropic) DefaultModel() string { return a.defaultModel }

func (a *Anthropic) Complete(ctx context.Context, model string, messages []Message, tools []ToolDef) (*Response, error) {
	if model == "" {
		model = a.defaultModel
	}

	type contentBlock struct {
		Type      string          `json:"type"`
		Text      string          `json:"text,omitempty"`
		ID        string          `json:"id,omitempty"`
		Name      string          `json:"name,omitempty"`
		Input     json.RawMessage `json:"input,omitempty"`
		ToolUseID string          `json:"tool_use_id,omitempty"`
		Content   string          `json:"content,omitempty"`
	}
	type antMsg struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	var systemPrompt string
	var antMsgs []antMsg
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		if m.Role == "tool" {
			antMsgs = append(antMsgs, antMsg{
				Role: "user",
				Content: []contentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
			continue
		}
		if len(m.ToolCalls) > 0 {
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Input,
				})
			}
			antMsgs = append(antMsgs, antMsg{Role: "assistant", Content: blocks})
			continue
		}
		antMsgs = append(antMsgs, antMsg{Role: m.Role, Content: m.Content})
	}

	type antTool struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	var antTools []antTool
	for _, t := range tools {
		antTools = append(antTools, antTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"messages":   antMsgs,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	if len(antTools) > 0 {
		reqBody["tools"] = antTools
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("anthropic parse: %w", err)
	}

	res := &Response{
		TokensIn:  result.Usage.InputTokens,
		TokensOut: result.Usage.OutputTokens,
		Provider:  "anthropic",
		Model:     model,
	}
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			res.Content += block.Text
		case "tool_use":
			res.ToolCalls = append(res.ToolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return res, nil
}
