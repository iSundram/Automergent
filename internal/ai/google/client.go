package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/iSundram/Automergent/internal/ai"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel   = "gemini-3.6-flash"
)

// Client implements ai.Provider for Google Gemini.
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	model      string
	limit      int
}

func New(cfg ai.ProviderConfig) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	model := cfg.DefaultModel
	if model == "" {
		model = defaultModel
	}
	return &Client{
		httpClient: &http.Client{},
		apiKey:     cfg.APIKey,
		baseURL:    base,
		model:      model,
		limit:      1000000,
	}
}

func (c *Client) Name() string      { return "google" }
func (c *Client) ContextLimit() int { return c.limit }

func (c *Client) Models(_ context.Context) ([]ai.Model, error) {
	return []ai.Model{
		{ID: "gemini-3.6-flash", Name: "Gemini 3.6 Flash", ContextLimit: 1048576},
		{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", ContextLimit: 1048576},
		{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", ContextLimit: 1048576},
		{ID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash-Lite", ContextLimit: 1048576},
		{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview", ContextLimit: 2097152},
		{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", ContextLimit: 1048576},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextLimit: 2097152},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextLimit: 1048576},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash-Lite", ContextLimit: 1048576},
		{ID: "gemini-2.5-flash-preview", Name: "Gemini 2.5 Flash Preview", ContextLimit: 1048576},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextLimit: 1048576},
		{ID: "gemini-2.0-flash-001", Name: "Gemini 2.0 Flash", ContextLimit: 1048576},
		{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash-Lite", ContextLimit: 1048576},
		{ID: "gemini-2.0-flash-lite-001", Name: "Gemini 2.0 Flash-Lite", ContextLimit: 1048576},
	}, nil
}

func (c *Client) TokenCount(messages []ai.Message) (int, error) {
	return ai.ApproximateTokenCount(messages), nil
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *struct {
		Name     string         `json:"name"`
		Response map[string]any `json:"response"`
	} `json:"functionResponse,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiTool struct {
	FunctionDeclarations []map[string]any `json:"functionDeclarations"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	ToolConfig        *struct {
		FunctionCallingConfig struct {
			Mode string `json:"mode"`
		} `json:"functionCallingConfig"`
	} `json:"toolConfig,omitempty"`
}

type geminiResponsePart struct {
	Text         string `json:"text"`
	FunctionCall *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiResponsePart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func toolNameForID(messages []ai.Message, toolIdx int, callID string) string {
	for j := toolIdx - 1; j >= 0; j-- {
		if messages[j].Role != ai.RoleAssistant {
			continue
		}
		for _, p := range messages[j].Content {
			if p.Type == ai.ContentTypeToolCall && p.ToolCall != nil && p.ToolCall.ID == callID {
				return p.ToolCall.Name
			}
		}
	}
	return "tool"
}

func buildGeminiContents(messages []ai.Message) []geminiContent {
	var out []geminiContent
	for i, m := range messages {
		switch m.Role {
		case ai.RoleUser:
			t := m.TextContent()
			if t == "" {
				continue
			}
			out = append(out, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: t}},
			})
		case ai.RoleAssistant:
			var parts []geminiPart
			for _, p := range m.Content {
				switch p.Type {
				case ai.ContentTypeText:
					if p.Text != "" {
						parts = append(parts, geminiPart{Text: p.Text})
					}
				case ai.ContentTypeToolCall:
					if p.ToolCall != nil {
						args := p.ToolCall.Args
						if args == nil {
							args = map[string]any{}
						}
						parts = append(parts, geminiPart{
							FunctionCall: &geminiFunctionCall{Name: p.ToolCall.Name, Args: args},
						})
					}
				}
			}
			if len(parts) == 0 {
				continue
			}
			out = append(out, geminiContent{Role: "model", Parts: parts})
		case ai.RoleTool:
			for _, p := range m.Content {
				if p.Type != ai.ContentTypeToolResult || p.ToolResult == nil {
					continue
				}
				name := toolNameForID(messages, i, p.ToolResult.ToolCallID)
				out = append(out, geminiContent{
					Role: "user",
					Parts: []geminiPart{{
						FunctionResponse: &struct {
							Name     string         `json:"name"`
							Response map[string]any `json:"response"`
						}{
							Name: name,
							Response: map[string]any{
								"result": p.ToolResult.Content,
							},
						},
					}},
				})
			}
		}
	}
	return out
}

func functionDeclarations(schemas []ai.ToolSchema) []map[string]any {
	if len(schemas) == 0 {
		return nil
	}
	decls := make([]map[string]any, 0, len(schemas))
	for _, s := range schemas {
		decls = append(decls, map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"parameters":  s.Parameters,
		})
	}
	return decls
}

func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	contents := buildGeminiContents(req.Messages)
	if len(contents) == 0 {
		contents = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: " "}}}}
	}

	gr := geminiRequest{Contents: contents}
	if req.System != "" {
		gr.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.System}},
		}
	}
	if decls := functionDeclarations(req.Tools); len(decls) > 0 {
		gr.Tools = []geminiTool{{FunctionDeclarations: decls}}
		gr.ToolConfig = &struct {
			FunctionCallingConfig struct {
				Mode string `json:"mode"`
			} `json:"functionCallingConfig"`
		}{}
		gr.ToolConfig.FunctionCallingConfig.Mode = "AUTO"
	}

	body, err := json.Marshal(gr)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: status %d: %s", resp.StatusCode, data)
	}

	var gr2 geminiResponse
	if err := json.Unmarshal(data, &gr2); err != nil {
		return nil, err
	}

	var text string
	var toolCalls []ai.ToolCall
	if len(gr2.Candidates) > 0 {
		for _, part := range gr2.Candidates[0].Content.Parts {
			if part.Text != "" {
				text += part.Text
			}
			if part.FunctionCall != nil {
				var args map[string]any
				if len(part.FunctionCall.Args) > 0 {
					_ = json.Unmarshal(part.FunctionCall.Args, &args)
				}
				if args == nil {
					args = map[string]any{}
				}
				id := fmt.Sprintf("gemini_%d", len(toolCalls))
				toolCalls = append(toolCalls, ai.ToolCall{
					ID:   id,
					Name: part.FunctionCall.Name,
					Args: args,
				})
			}
		}
	}

	usage := ai.Usage{
		InputTokens:  gr2.UsageMetadata.PromptTokenCount,
		OutputTokens: gr2.UsageMetadata.CandidatesTokenCount,
		TotalTokens:  gr2.UsageMetadata.TotalTokenCount,
	}

	stop := ai.StopReasonEnd
	if len(toolCalls) > 0 {
		stop = ai.StopReasonTools
	} else if len(gr2.Candidates) > 0 {
		switch gr2.Candidates[0].FinishReason {
		case "MAX_TOKENS":
			stop = ai.StopReasonLength
		case "STOP", "":
			stop = ai.StopReasonEnd
		}
	}

	return ai.NewStaticResponse(text, toolCalls, stop, usage), nil
}
