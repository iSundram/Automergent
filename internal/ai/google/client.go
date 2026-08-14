package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

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
	ID   string         `json:"id,omitempty"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"`
	ThoughtSignature string              `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionRes  `json:"functionResponse,omitempty"`
}

type geminiFunctionRes struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
	ID       string         `json:"id,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiTool struct {
	FunctionDeclarations []map[string]any `json:"functionDeclarations,omitempty"`
	GoogleSearch         *struct{}        `json:"googleSearch,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig *struct {
		Mode string `json:"mode,omitempty"`
	} `json:"functionCallingConfig,omitempty"`
	IncludeServerSideToolInvocations bool `json:"includeServerSideToolInvocations,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent   `json:"contents"`
	SystemInstruction *geminiContent    `json:"systemInstruction,omitempty"`
	Tools             []geminiTool      `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig `json:"toolConfig,omitempty"`
	GenerationConfig  *geminiGenConfig  `json:"generationConfig,omitempty"`
}

type geminiGenConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
	ThinkingConfig   *struct {
		IncludeThoughts bool   `json:"includeThoughts,omitempty"`
		ThinkingLevel   string `json:"thinkingLevel,omitempty"`  // "minimal", "low", "medium", "high"
		ThinkingBudget  int    `json:"thinkingBudget,omitempty"` // Gemini 2.5 models
	} `json:"thinkingConfig,omitempty"`
}

type geminiResponsePart struct {
	Text             string `json:"text,omitempty"`
	Thought          bool   `json:"thought,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
	FunctionCall     *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
		ID   string          `json:"id,omitempty"`
	} `json:"functionCall,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Role  string               `json:"role"`
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
			// For model messages, we try to find the original parts if we stored them in metadata,
			// or we rebuild them from the current message content.
			var parts []geminiPart
			if partsRaw, ok := m.Metadata["google_parts"]; ok {
				if b, err := json.Marshal(partsRaw); err == nil {
					// Note: Errors unmarshaling metadata are not fatal - we can rebuild from Content
					_ = json.Unmarshal(b, &parts)
				}
			}

			if len(parts) == 0 {
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
								FunctionCall: &geminiFunctionCall{
									Name: p.ToolCall.Name,
									Args: args,
									ID:   p.ToolCall.ID,
								},
							})
						}
					}
				}
			}

			if len(parts) == 0 {
				continue
			}
			out = append(out, geminiContent{Role: "model", Parts: parts})

		case ai.RoleTool:
			var parts []geminiPart
			for _, p := range m.Content {
				if p.Type != ai.ContentTypeToolResult || p.ToolResult == nil {
					continue
				}
				name := toolNameForID(messages, i, p.ToolResult.ToolCallID)
				parts = append(parts, geminiPart{
					FunctionResponse: &geminiFunctionRes{
						Name: name,
						ID:   p.ToolResult.ToolCallID,
						Response: map[string]any{
							"result": p.ToolResult.Content,
						},
					},
				})
			}
			if len(parts) > 0 {
				// According to Gemini 3 documentation, tool results are provided in 'user' role.
				out = append(out, geminiContent{Role: "user", Parts: parts})
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
		gr.ToolConfig = &geminiToolConfig{
			FunctionCallingConfig: &struct {
				Mode string `json:"mode,omitempty"`
			}{Mode: "AUTO"},
		}
	}

	// Configure generation settings including thinking
	genConfig := &geminiGenConfig{}
	if req.MaxTokens > 0 {
		genConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		genConfig.Temperature = &temp
	}
	// Enable thinking mode if configured
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		tc := &struct {
			IncludeThoughts bool   `json:"includeThoughts,omitempty"`
			ThinkingLevel   string `json:"thinkingLevel,omitempty"`
			ThinkingBudget  int    `json:"thinkingBudget,omitempty"`
		}{IncludeThoughts: true}

		// Gemini 2.5 uses thinkingBudget; Gemini 3 uses thinkingLevel.
		if strings.HasPrefix(c.model, "gemini-2.5") {
			if req.Thinking.BudgetTokens != 0 {
				tc.ThinkingBudget = req.Thinking.BudgetTokens
			} else {
				tc.ThinkingBudget = -1 // Dynamic thinking budget.
			}
		} else {
			tc.ThinkingLevel = "high"
		}

		genConfig.ThinkingConfig = tc
	}
	gr.GenerationConfig = genConfig

	body, err := json.Marshal(gr)
	if err != nil {
		return nil, err
	}

	method := "generateContent"
	if req.Stream {
		method = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s?key=%s", c.baseURL, c.model, method, c.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google: status %d: %s", resp.StatusCode, data)
	}

	if !req.Stream {
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var gr2 geminiResponse
		if err := json.Unmarshal(data, &gr2); err != nil {
			return nil, err
		}

		var text string
		var thought string
		var toolCalls []ai.ToolCall
		var rawParts []geminiPart

		if len(gr2.Candidates) > 0 {
			for _, part := range gr2.Candidates[0].Content.Parts {
				// Skip parts that are completely empty to avoid 400 error.
				// If it only has thoughtSignature, it MUST still have some data field (like text).
				if part.Text == "" && part.FunctionCall == nil && part.ThoughtSignature != "" {
					// Add a placeholder space to satisfy "oneof field 'data' must have one initialized field"
					part.Text = " "
				}
				if part.Text == "" && part.FunctionCall == nil && part.ThoughtSignature == "" {
					continue
				}

				// Parse function call args if present
				var parsedArgs map[string]any
				if part.FunctionCall != nil && len(part.FunctionCall.Args) > 0 {
					if err := json.Unmarshal(part.FunctionCall.Args, &parsedArgs); err != nil {
						return nil, fmt.Errorf("google: invalid function call args: %w", err)
					}
				}

				// Save for next turn
				rawParts = append(rawParts, geminiPart{
					Text:             part.Text,
					Thought:          part.Thought,
					ThoughtSignature: part.ThoughtSignature,
					FunctionCall: func() *geminiFunctionCall {
						if part.FunctionCall == nil {
							return nil
						}
						argsCopy := parsedArgs
						if argsCopy == nil {
							argsCopy = map[string]any{}
						}
						return &geminiFunctionCall{Name: part.FunctionCall.Name, Args: argsCopy}
					}(),
				})
				if part.Thought {
					thought += part.Text
				} else if part.Text != "" {
					text += part.Text
				}
				if part.FunctionCall != nil {
					args := parsedArgs
					if args == nil {
						args = map[string]any{}
					}
					// Use model-provided ID if available
					id := part.FunctionCall.ID
					if id == "" {
						id = fmt.Sprintf("gemini_%d", len(toolCalls))
					}
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

		res := ai.NewStaticResponse(text, thought, toolCalls, ai.StopReasonEnd, usage)
		if len(rawParts) > 0 {
			res.SetMetadata(map[string]any{"google_parts": rawParts})
		}

		if len(toolCalls) > 0 {
			res.SetStopReason(ai.StopReasonTools)
		} else if len(gr2.Candidates) > 0 {
			switch gr2.Candidates[0].FinishReason {
			case "MAX_TOKENS":
				res.SetStopReason(ai.StopReasonLength)
			case "STOP", "":
				res.SetStopReason(ai.StopReasonEnd)
			}
		}

		return res, nil
	}

	// Streaming logic: Gemini returns a JSON array of objects during streamGenerateContent
	ch := make(chan ai.Chunk, 128)

	// Protect shared state with mutex
	var mu sync.Mutex
	toolCalls := []ai.ToolCall{}
	stopReason := ai.StopReasonEnd
	usage := ai.Usage{}
	rawParts := []geminiPart{}

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		dec := json.NewDecoder(resp.Body)
		// Read the opening '['
		t, err := dec.Token()
		if err != nil {
			ch <- ai.Chunk{Error: fmt.Errorf("stream start: %w", err)}
			return
		}
		if t != json.Delim('[') {
			ch <- ai.Chunk{Error: fmt.Errorf("expected '[' at start of stream, got %v", t)}
			return
		}

		for dec.More() {
			var gr2 geminiResponse
			if err := dec.Decode(&gr2); err != nil {
				ch <- ai.Chunk{Error: fmt.Errorf("stream decode: %w", err)}
				return
			}

			if gr2.UsageMetadata.TotalTokenCount > 0 {
				usage = ai.Usage{
					InputTokens:  gr2.UsageMetadata.PromptTokenCount,
					OutputTokens: gr2.UsageMetadata.CandidatesTokenCount,
					TotalTokens:  gr2.UsageMetadata.TotalTokenCount,
				}
			}

			if len(gr2.Candidates) > 0 {
				cand := gr2.Candidates[0]
				if cand.FinishReason != "" && stopReason != ai.StopReasonTools {
					switch cand.FinishReason {
					case "MAX_TOKENS":
						stopReason = ai.StopReasonLength
					case "STOP":
						stopReason = ai.StopReasonEnd
					}
				}

				for _, part := range cand.Content.Parts {
					// Skip parts that are completely empty to avoid 400 error.
					if part.Text == "" && part.FunctionCall == nil && part.ThoughtSignature != "" {
						part.Text = " "
					}
					if part.Text == "" && part.FunctionCall == nil && part.ThoughtSignature == "" {
						continue
					}

					// Save raw parts for context preservation (protected by mutex)
					mu.Lock()
					rawParts = append(rawParts, geminiPart{
						Text:             part.Text,
						Thought:          part.Thought,
						ThoughtSignature: part.ThoughtSignature,
						FunctionCall: func() *geminiFunctionCall {
							if part.FunctionCall == nil {
								return nil
							}
							var args map[string]any
							if err := json.Unmarshal(part.FunctionCall.Args, &args); err != nil {
								// Send error chunk
								ch <- ai.Chunk{
									Error: fmt.Errorf("google: parsing tool args for %s: %w", part.FunctionCall.Name, err),
									Done:  true,
								}
								return nil
							}
							return &geminiFunctionCall{Name: part.FunctionCall.Name, Args: args}
						}(),
					})
					mu.Unlock()

					chunk := ai.Chunk{}
					if part.Thought {
						chunk.Thought = part.Text
					} else if part.Text != "" {
						chunk.Text = part.Text
					}

					if part.FunctionCall != nil {
						var args map[string]any
						if len(part.FunctionCall.Args) > 0 {
							if err := json.Unmarshal(part.FunctionCall.Args, &args); err != nil {
								ch <- ai.Chunk{
									Error: fmt.Errorf("google: invalid tool arguments for %s: %w", part.FunctionCall.Name, err),
									Done:  true,
								}
								mu.Unlock()
								return
							}
						}
						if args == nil {
							args = map[string]any{}
						}

						mu.Lock()
						id := part.FunctionCall.ID
						if id == "" {
							id = fmt.Sprintf("gemini_%d", len(toolCalls))
						}
						tc := ai.ToolCall{
							ID:   id,
							Name: part.FunctionCall.Name,
							Args: args,
						}
						toolCalls = append(toolCalls, tc)
						stopReason = ai.StopReasonTools
						mu.Unlock()

						chunk.ToolCalls = append(chunk.ToolCalls, tc)
					}
					ch <- chunk
				}
			}
		}
		ch <- ai.Chunk{Done: true}
	}()

	res := ai.NewChannelResponse(ch, ai.StopReasonEnd, ai.Usage{})
	return &geminiStreamResponse{
		res:        res,
		toolCalls:  &toolCalls,
		stopReason: &stopReason,
		usage:      &usage,
		rawParts:   &rawParts,
		mu:         &mu,
	}, nil
}

type geminiStreamResponse struct {
	res        *ai.ChannelResponse
	toolCalls  *[]ai.ToolCall
	stopReason *ai.StopReason
	usage      *ai.Usage
	rawParts   *[]geminiPart
	mu         *sync.Mutex
}

func (r *geminiStreamResponse) Stream() <-chan ai.Chunk { return r.res.Stream() }
func (r *geminiStreamResponse) ToolCalls() []ai.ToolCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.toolCalls
}
func (r *geminiStreamResponse) StopReason() ai.StopReason {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.stopReason
}
func (r *geminiStreamResponse) Usage() ai.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.usage
}
func (r *geminiStreamResponse) GetMetadata() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return map[string]any{"google_parts": *r.rawParts}
}
