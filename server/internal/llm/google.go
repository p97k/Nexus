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

type Google struct {
	apiKey       string
	defaultModel string
	client       *http.Client
}

func NewGoogle(apiKey, defaultModel string) *Google {
	return &Google{
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 90 * time.Second},
	}
}

func (g *Google) Name() string         { return "google" }
func (g *Google) DefaultModel() string { return g.defaultModel }

func (g *Google) Complete(ctx context.Context, model string, messages []Message, tools []ToolDef) (*Response, error) {
	if model == "" {
		model = g.defaultModel
	}

	type part struct {
		Text             string `json:"text,omitempty"`
		ThoughtSignature string `json:"thoughtSignature,omitempty"`
		FunctionCall     *struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"functionCall,omitempty"`
		FunctionResponse *struct {
			Name     string `json:"name"`
			Response any    `json:"response"`
		} `json:"functionResponse,omitempty"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var system string
	var contents []content
	for i := 0; i < len(messages); i++ {
		m := messages[i]
		if m.Role == "system" {
			system = m.Content
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if m.Role == "tool" {
			// Gemini no longer accepts role "function"; tool results must be
			// role "user" with functionResponse parts. Parallel tool results
			// belong in a single user turn.
			var parts []part
			for j := i; j < len(messages) && messages[j].Role == "tool"; j++ {
				tm := messages[j]
				// Gemini's functionResponse.response must be a JSON object.
				// Tool results that are already objects are passed through;
				// arrays and non-JSON text are wrapped so the payload stays an object.
				var raw any
				var resp any
				if err := json.Unmarshal([]byte(tm.Content), &raw); err != nil {
					resp = map[string]string{"result": tm.Content}
				} else if obj, ok := raw.(map[string]any); ok {
					resp = obj
				} else {
					resp = map[string]any{"result": raw}
				}
				parts = append(parts, part{FunctionResponse: &struct {
					Name     string `json:"name"`
					Response any    `json:"response"`
				}{Name: tm.ToolName, Response: resp}})
				i = j
			}
			contents = append(contents, content{Role: "user", Parts: parts})
			continue
		}
		if len(m.ToolCalls) > 0 {
			var parts []part
			// Thinking models (e.g. gemini-3.5-flash) attach a thoughtSignature
			// to each functionCall part. It must be echoed back verbatim on the
			// next request or the API returns 400 INVALID_ARGUMENT.
			for _, tc := range m.ToolCalls {
				parts = append(parts, part{
					ThoughtSignature: m.ThoughtSignature, // empty string → omitted by omitempty
					FunctionCall: &struct {
						Name string          `json:"name"`
						Args json.RawMessage `json:"args"`
					}{Name: tc.Name, Args: tc.Input},
				})
			}
			contents = append(contents, content{Role: "model", Parts: parts})
			continue
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: m.Content}}})
	}

	reqBody := map[string]any{
		"contents": contents,
	}
	if system != "" {
		reqBody["systemInstruction"] = map[string]any{
			"parts": []part{{Text: system}},
		}
	}
	if len(tools) > 0 {
		type funcDecl struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		var decls []funcDecl
		for _, t := range tools {
			decls = append(decls, funcDecl{t.Name, t.Description, t.Parameters})
		}
		reqBody["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}

	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, g.apiKey,
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text             string `json:"text,omitempty"`
					ThoughtSignature string `json:"thoughtSignature,omitempty"`
					FunctionCall     *struct {
						Name string          `json:"name"`
						Args json.RawMessage `json:"args"`
						ID   string          `json:"id,omitempty"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("google parse: %w", err)
	}
	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("google: no candidates")
	}

	res := &Response{
		TokensIn:  result.UsageMetadata.PromptTokenCount,
		TokensOut: result.UsageMetadata.CandidatesTokenCount,
		Provider:  "google",
		Model:     model,
	}
	for _, p := range result.Candidates[0].Content.Parts {
		if p.ThoughtSignature != "" {
			res.ThoughtSignature = p.ThoughtSignature
		}
		if p.Text != "" {
			res.Content += p.Text
		}
		if p.FunctionCall != nil {
			id := p.FunctionCall.ID
			if id == "" {
				id = p.FunctionCall.Name + "_call"
			}
			res.ToolCalls = append(res.ToolCalls, ToolCall{
				ID:    id,
				Name:  p.FunctionCall.Name,
				Input: p.FunctionCall.Args,
			})
		}
	}
	return res, nil
}
