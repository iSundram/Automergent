package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"google.golang.org/genai"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel   = "gemini-3.6-flash"
)

// Client implements ai.Provider for Google Gemini, backed by the official
// Google GenAI SDK (google.golang.org/genai).
//
// Two backends are supported:
//   - Gemini API: when only an API key is configured (or nothing at all, in
//     which case the SDK falls back to environment variables).
//   - Vertex AI (Google Cloud): when a project and location are configured.
//     Credentials come from Application Default Credentials or an API key.
type Client struct {
	client  *genai.Client
	initErr error
	model   string
	limit   int
	baseURL string
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

	cc := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		HTTPClient: cfg.HTTPClient,
	}

	// Vertex AI (Google Cloud) is used when a project and location are set.
	// This satisfies the Google Cloud infrastructure requirement: requests go
	// to {location}-aiplatform.googleapis.com instead of the plain Gemini API.
	if cfg.Project != "" && cfg.Location != "" {
		cc.Backend = genai.BackendVertexAI
		cc.Project = cfg.Project
		cc.Location = cfg.Location
	} else if base != "" {
		// The genai SDK appends its own API version (v1beta) to the base URL,
		// so a config base URL that already ends in /v1beta must have that
		// segment stripped, otherwise requests go to .../v1beta/v1beta/... and
		// fail forever.
		cc.HTTPOptions = genai.HTTPOptions{BaseURL: stripAPIVersion(base)}
	}

	c := &Client{model: model, limit: 1000000, baseURL: base}
	client, err := genai.NewClient(context.Background(), cc)
	if err != nil {
		c.initErr = err
		return c
	}
	c.client = client
	return c
}

func (c *Client) Name() string      { return "google" }
func (c *Client) ContextLimit() int { return c.limit }

// stripAPIVersion removes a trailing API version segment (e.g. /v1beta) from a
// base URL, because the genai SDK always appends its own API version to the
// base URL it is given.
func stripAPIVersion(base string) string {
	for _, v := range []string{"/v1beta1", "/v1alpha1", "/v1beta", "/v1alpha", "/v1"} {
		if strings.HasSuffix(base, v) {
			return strings.TrimSuffix(base, v)
		}
	}
	return base
}

// isEmptyPart reports whether a genai.Part carries no meaningful content.
// The Gemini SSE stream sometimes includes a final event with an empty text
// part (text:"") and finishReason=STOP; such parts must not be added to
// rawParts because they cause the next request to fail with a 400 error.
func isEmptyPart(p *genai.Part) bool {
	if p.Text != "" {
		return false
	}
	if p.Thought {
		return false
	}
	if p.FunctionCall != nil {
		return false
	}
	if p.FunctionResponse != nil {
		return false
	}
	if p.FileData != nil {
		return false
	}
	if p.InlineData != nil {
		return false
	}
	if p.CodeExecutionResult != nil {
		return false
	}
	if p.ExecutableCode != nil {
		return false
	}
	if p.VideoMetadata != nil {
		return false
	}
	if p.AudioTranscription != nil {
		return false
	}
	if len(p.ThoughtSignature) > 0 {
		return false
	}
	if p.ToolCall != nil {
		return false
	}
	if p.ToolResponse != nil {
		return false
	}
	if p.PartMetadata != nil {
		return false
	}
	if p.MediaResolution != nil {
		return false
	}
	return true
}

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

// buildContents converts the conversation into genai contents. Assistant turns
// first try to restore the original model parts (including thought signatures)
// from the "google_parts" metadata captured at response time; otherwise they are
// rebuilt from the normalized message content.
func buildContents(messages []ai.Message) []*genai.Content {
	var out []*genai.Content
	for i, m := range messages {
		switch m.Role {
		case ai.RoleUser:
			t := m.TextContent()
			if t == "" {
				continue
			}
			out = append(out, genai.NewContentFromText(t, genai.RoleUser))

		case ai.RoleAssistant:
			var parts []*genai.Part
			if partsRaw, ok := m.Metadata["google_parts"]; ok {
				if b, err := json.Marshal(partsRaw); err == nil {
					var stored []*genai.Part
					// Note: Errors unmarshaling metadata are not fatal - we can rebuild from Content
					if json.Unmarshal(b, &stored) == nil {
						parts = stored
					}
				}
			}

			if len(parts) == 0 {
				for _, p := range m.Content {
					switch p.Type {
					case ai.ContentTypeText:
						if p.Text != "" {
							parts = append(parts, genai.NewPartFromText(p.Text))
						}
					case ai.ContentTypeToolCall:
						if p.ToolCall != nil {
							args := p.ToolCall.Args
							if args == nil {
								args = map[string]any{}
							}
							parts = append(parts, &genai.Part{
								FunctionCall: &genai.FunctionCall{
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
			out = append(out, &genai.Content{Role: string(genai.RoleModel), Parts: parts})

		case ai.RoleTool:
			var parts []*genai.Part
			for _, p := range m.Content {
				if p.Type != ai.ContentTypeToolResult || p.ToolResult == nil {
					continue
				}
				name := toolNameForID(messages, i, p.ToolResult.ToolCallID)
				parts = append(parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
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
				out = append(out, &genai.Content{Role: string(genai.RoleUser), Parts: parts})
			}
		}
	}
	return out
}

// schemaFromJSON converts a raw JSON schema object (as produced by the tool
// registry) into a genai.Schema for function declarations.
func schemaFromJSON(raw map[string]any) *genai.Schema {
	if raw == nil {
		return nil
	}
	s := &genai.Schema{}
	if t, ok := raw["type"].(string); ok {
		s.Type = genai.Type(strings.ToUpper(t))
	}
	if d, ok := raw["description"].(string); ok {
		s.Description = d
	}
	if f, ok := raw["format"].(string); ok {
		s.Format = f
	}
	if def, ok := raw["default"]; ok {
		s.Default = def
	}
	switch s.Type {
	case "OBJECT":
		if props, ok := raw["properties"].(map[string]any); ok {
			s.Properties = make(map[string]*genai.Schema, len(props))
			for name, v := range props {
				if pm, ok := v.(map[string]any); ok {
					s.Properties[name] = schemaFromJSON(pm)
				}
			}
		}
		if req, ok := raw["required"].([]any); ok {
			for _, r := range req {
				if rs, ok := r.(string); ok {
					s.Required = append(s.Required, rs)
				}
			}
		}
	case "ARRAY":
		if items, ok := raw["items"].(map[string]any); ok {
			s.Items = schemaFromJSON(items)
		}
		if max, ok := raw["maxItems"].(int); ok {
			v := int64(max)
			s.MaxItems = &v
		}
	case "STRING":
		if max, ok := numberValue(raw["maxLength"]); ok {
			v := int64(max)
			s.MaxLength = &v
		}
	case "INTEGER", "NUMBER":
		if min, ok := numberValue(raw["minimum"]); ok {
			v := float64(min)
			s.Minimum = &v
		}
		if max, ok := numberValue(raw["maximum"]); ok {
			v := float64(max)
			s.Maximum = &v
		}
	}
	if en, ok := raw["enum"].([]any); ok {
		for _, e := range en {
			if es, ok := e.(string); ok {
				s.Enum = append(s.Enum, es)
			}
		}
	}
	if anyOf, ok := raw["anyOf"].([]any); ok {
		for _, a := range anyOf {
			if am, ok := a.(map[string]any); ok {
				s.AnyOf = append(s.AnyOf, schemaFromJSON(am))
			}
		}
	}
	return s
}

// numberValue extracts a numeric value from a raw JSON schema field, which may
// arrive as int, int64, float64 (typical after JSON decoding) or json.Number.
func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

func functionDeclarations(schemas []ai.ToolSchema) []*genai.Tool {
	if len(schemas) == 0 {
		return nil
	}
	decls := make([]*genai.FunctionDeclaration, 0, len(schemas))
	for _, s := range schemas {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  schemaFromJSON(s.Parameters),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

func buildGenerateContentConfig(c *Client, req ai.CompletionRequest) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}
	if req.System != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: req.System}}}
	}
	if tools := functionDeclarations(req.Tools); len(tools) > 0 {
		config.Tools = tools
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if req.Temperature > 0 {
		temp := float32(req.Temperature)
		config.Temperature = &temp
	}
	// Enable thinking mode if configured
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		tc := &genai.ThinkingConfig{IncludeThoughts: true}

		// Gemini 2.5 uses thinkingBudget; Gemini 3 uses thinkingLevel.
		if strings.HasPrefix(c.model, "gemini-2.5") {
			if req.Thinking.BudgetTokens != 0 {
				budget := int32(req.Thinking.BudgetTokens)
				tc.ThinkingBudget = &budget
			} else {
				budget := int32(-1) // Dynamic thinking budget.
				tc.ThinkingBudget = &budget
			}
		} else {
			tc.ThinkingLevel = "high"
		}

		config.ThinkingConfig = tc
	}
	return config
}

func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	if c.initErr != nil {
		return nil, fmt.Errorf("google: client initialization failed: %w", c.initErr)
	}
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("google: invalid message sequence: %w", err)
	}
	contents := buildContents(req.Messages)
	if len(contents) == 0 {
		contents = []*genai.Content{genai.NewContentFromText(" ", genai.RoleUser)}
	}
	config := buildGenerateContentConfig(c, req)

	// Wrap the request in a retry loop
	policy := automergentErrors.AggressiveRetryPolicy()
	policy.MaxAttempts = 10

	if !req.Stream {
		resp, retryResult := automergentErrors.RetryWithValue(ctx, policy, func() (*genai.GenerateContentResponse, error) {
			resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
			if err != nil {
				log.Printf("google: attempt error: %v", err)
				return nil, c.mapError(err)
			}
			return resp, nil
		})
		if !retryResult.Successful {
			return nil, retryResult.LastError
		}
		return c.buildStaticResponse(resp), nil
	}

	// Streaming: the SDK returns a lazy iterator; the HTTP request is only sent
	// on the first iteration. Retry only until the first response arrives, so a
	// mid-stream failure is never retried (avoiding duplicated output). The
	// iterator is consumed through iter.Pull2 so that a failed attempt can be
	// retried with a fresh request, while a successful attempt is drained
	// exactly once (re-ranging a push iterator re-runs its body over the
	// already-consumed stream, which can yield garbage responses).
	first, retryResult := automergentErrors.RetryWithValue(ctx, policy, func() (firstStreamResult, error) {
		stream := c.client.Models.GenerateContentStream(ctx, c.model, contents, config)
		pull, stop := iter.Pull2(stream)
		resp, err, ok := pull()
		if err != nil {
			stop()
			log.Printf("google: stream attempt error: %v", err)
			return firstStreamResult{}, c.mapError(err)
		}
		if !ok {
			stop()
			return firstStreamResult{}, nil
		}
		return firstStreamResult{pull: pull, stop: stop, first: resp}, nil
	})
	if !retryResult.Successful {
		return nil, retryResult.LastError
	}

	ch := make(chan ai.Chunk, 128)
	// Protect shared state with mutex
	var mu sync.Mutex
	toolCalls := []ai.ToolCall{}
	stopReason := ai.StopReasonEnd
	usage := ai.Usage{}
	rawParts := []*genai.Part{}
	// receivedFinish tracks whether the model already delivered its final
	// response (finish reason or usage accounting). A transport error after
	// that point is just the server closing the SSE connection cleanly and
	// must not be reported as a failure.
	receivedFinish := false

	go func() {
		defer close(ch)
		defer first.stop()

		emit := func(resp *genai.GenerateContentResponse) {
			if resp.UsageMetadata != nil && resp.UsageMetadata.TotalTokenCount > 0 {
				receivedFinish = true
				usage = ai.Usage{
					InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
					OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
					TotalTokens:  int(resp.UsageMetadata.TotalTokenCount),
				}
			}

			if len(resp.Candidates) == 0 {
				return
			}
			cand := resp.Candidates[0]
			if cand.FinishReason != "" && stopReason != ai.StopReasonTools {
				receivedFinish = true
				switch cand.FinishReason {
				case genai.FinishReasonMaxTokens:
					stopReason = ai.StopReasonLength
				case genai.FinishReasonStop:
					stopReason = ai.StopReasonEnd
				}
			}

			if cand.Content == nil {
				return
			}
			for _, part := range cand.Content.Parts {
				if part == nil {
					continue
				}
				if isEmptyPart(part) {
					continue
				}

				// Save raw parts for context preservation (protected by mutex)
				mu.Lock()
				rawParts = append(rawParts, part)
				mu.Unlock()

				chunk := ai.Chunk{}
				if part.Thought {
					chunk.Thought = part.Text
				} else if part.Text != "" {
					chunk.Text = part.Text
				}

				if part.FunctionCall != nil {
					args := part.FunctionCall.Args
					if args == nil {
						args = map[string]any{}
					}

					id := part.FunctionCall.ID
					if id == "" {
						mu.Lock()
						id = fmt.Sprintf("gemini_%d", len(toolCalls))
						mu.Unlock()
					}
					tc := ai.ToolCall{
						ID:   id,
						Name: part.FunctionCall.Name,
						Args: args,
					}

					mu.Lock()
					toolCalls = append(toolCalls, tc)
					stopReason = ai.StopReasonTools
					mu.Unlock()

					chunk.ToolCalls = append(chunk.ToolCalls, tc)
				}
				ch <- chunk
			}
		}

		if first.first != nil {
			emit(first.first)
		}
		for {
			resp, err, ok := first.pull()
			if err != nil {
				if receivedFinish {
					break
				}
				ch <- ai.Chunk{Error: c.mapError(err), Done: true}
				return
			}
			if !ok {
				break
			}
			emit(resp)
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

// firstStreamResult carries the first streamed response together with a pull
// function over the same iterator, so the remaining chunks can be drained
// without re-sending the request or re-running the iterator body.
type firstStreamResult struct {
	pull  func() (*genai.GenerateContentResponse, error, bool)
	stop  func()
	first *genai.GenerateContentResponse
}

func (c *Client) buildStaticResponse(resp *genai.GenerateContentResponse) ai.CompletionResponse {
	var text, thought string
	var toolCalls []ai.ToolCall
	var rawParts []*genai.Part

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part == nil {
				continue
			}
			rawParts = append(rawParts, part)
			if part.Thought {
				thought += part.Text
			} else if part.Text != "" {
				text += part.Text
			}
			if part.FunctionCall != nil {
				args := part.FunctionCall.Args
				if args == nil {
					args = map[string]any{}
				}
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

	usage := ai.Usage{}
	if resp.UsageMetadata != nil {
		usage = ai.Usage{
			InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:  int(resp.UsageMetadata.TotalTokenCount),
		}
	}

	stop := ai.StopReasonEnd
	if len(toolCalls) > 0 {
		stop = ai.StopReasonTools
	} else if len(resp.Candidates) > 0 {
		switch resp.Candidates[0].FinishReason {
		case genai.FinishReasonMaxTokens:
			stop = ai.StopReasonLength
		case genai.FinishReasonStop, "":
			stop = ai.StopReasonEnd
		}
	}

	res := ai.NewStaticResponse(text, thought, toolCalls, stop, usage)
	if len(rawParts) > 0 {
		res.SetMetadata(map[string]any{"google_parts": rawParts})
	}
	return res
}

type geminiStreamResponse struct {
	res        *ai.ChannelResponse
	toolCalls  *[]ai.ToolCall
	stopReason *ai.StopReason
	usage      *ai.Usage
	rawParts   *[]*genai.Part
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

// mapError converts SDK errors into the project's error taxonomy.
func (c *Client) mapError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) || func() bool {
		var ptrErr *genai.APIError
		if errors.As(err, &ptrErr) && ptrErr != nil {
			apiErr = *ptrErr
			return true
		}
		return false
	}() {
		msg := apiErr.Message
		if msg == "" {
			msg = fmt.Sprintf("Google API error (status %d)", apiErr.Code)
		}

		code := automergentErrors.CodeHTTPError
		switch apiErr.Code {
		case http.StatusTooManyRequests:
			code = automergentErrors.CodeRateLimited
			if strings.Contains(strings.ToLower(msg), "quota") {
				code = automergentErrors.CodeQuotaExceeded
			}
		case http.StatusUnauthorized:
			code = automergentErrors.CodeUnauthorized
		case http.StatusForbidden:
			code = automergentErrors.CodeForbidden
		case http.StatusServiceUnavailable:
			code = automergentErrors.CodeServiceUnavailable
		case http.StatusInternalServerError:
			code = automergentErrors.CodeServerError
		}

		oce := automergentErrors.New(code, msg).
			WithResource(c.baseURL).
			WithContext("status_code", apiErr.Code).
			WithContext("google_status", apiErr.Status)

		if apiErr.Code == http.StatusTooManyRequests || apiErr.Code >= http.StatusInternalServerError {
			oce.WithRetry(30 * time.Second)
		}
		return oce
	}

	return automergentErrors.NewConnectionError(c.baseURL, err)
}
