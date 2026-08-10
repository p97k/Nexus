package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI is an OpenAI-compatible chat completions provider. It is also used
// for OpenAI-compatible services such as Groq (different base URL + key).
type OpenAI struct {
	name         string
	apiKey       string
	baseURL      string
	defaultModel string
	client       *http.Client
}

// NewOpenAI returns a provider for the official OpenAI API.
func NewOpenAI(apiKey, defaultModel string) *OpenAI {
	return NewOpenAICompatible("openai", apiKey, "https://api.openai.com/v1", defaultModel)
}

// NewOpenAICompatible returns a provider for any OpenAI-compatible endpoint.
func NewOpenAICompatible(name, apiKey, baseURL, defaultModel string) *OpenAI {
	return &OpenAI{
		name:         name,
		apiKey:       apiKey,
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 90 * time.Second},
	}
}

func (o *OpenAI) Name() string         { return o.name }
func (o *OpenAI) DefaultModel() string { return o.defaultModel }

// oaiToolCall mirrors the OpenAI tool_call object used in both request and response.
type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (o *OpenAI) Complete(ctx context.Context, model string, messages []Message, tools []ToolDef) (*Response, error) {
	if model == "" {
		model = o.defaultModel
	}

	type oaiMsg struct {
		Role       string        `json:"role"`
		Content    any           `json:"content,omitempty"`
		ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
		ToolCallID string        `json:"tool_call_id,omitempty"`
		Name       string        `json:"name,omitempty"`
	}

	var oaiMsgs []oaiMsg
	for _, m := range messages {
		msg := oaiMsg{Role: m.Role, Content: m.Content}
		if m.ToolCallID != "" {
			msg.Role = "tool"
			msg.ToolCallID = m.ToolCallID
			msg.Name = m.ToolName
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: string(tc.Input)},
			})
		}
		oaiMsgs = append(oaiMsgs, msg)
	}

	type oaiFunctionDef struct {
		Type     string `json:"type"`
		Function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	var oaiTools []oaiFunctionDef
	for _, t := range tools {
		fd := oaiFunctionDef{Type: "function"}
		fd.Function.Name = t.Name
		fd.Function.Description = t.Description
		fd.Function.Parameters = t.Parameters
		oaiTools = append(oaiTools, fd)
	}

	reqBody := map[string]any{
		"model":    model,
		"messages": oaiMsgs,
	}
	if len(oaiTools) > 0 {
		reqBody["tools"] = oaiTools
		reqBody["tool_choice"] = "auto"
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", o.name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %d: %s", o.name, resp.StatusCode, raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string        `json:"content"`
				ToolCalls []oaiToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("%s parse: %w", o.name, err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("%s: no choices", o.name)
	}

	msg := result.Choices[0].Message
	res := &Response{
		Content:   msg.Content,
		TokensIn:  result.Usage.PromptTokens,
		TokensOut: result.Usage.CompletionTokens,
		Provider:  o.name,
		Model:     model,
	}
	for _, tc := range msg.ToolCalls {
		res.ToolCalls = append(res.ToolCalls, ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	return res, nil
}
