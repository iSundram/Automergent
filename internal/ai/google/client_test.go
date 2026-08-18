package google

import (
	"context"
	"net/http"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"google.golang.org/genai"
)

func TestBuildContents(t *testing.T) {
	messages := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "calculate 2+2"),
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{
				{
					Type: ai.ContentTypeToolCall,
					ToolCall: &ai.ToolCall{
						ID:   "call_1",
						Name: "calculator",
						Args: map[string]any{"expr": "2+2"},
					},
				},
			},
		},
		{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{
				{
					Type: ai.ContentTypeToolResult,
					ToolResult: &ai.ToolResult{
						ToolCallID: "call_1",
						Content:    "4",
					},
				},
			},
		},
	}

	contents := buildContents(messages)

	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	if contents[0].Role != "user" {
		t.Errorf("expected first role user, got %s", contents[0].Role)
	}

	if contents[1].Role != "model" {
		t.Errorf("expected second role model, got %s", contents[1].Role)
	}

	// Tool results are sent back in the 'user' role (Gemini 3 docs).
	if contents[2].Role != "user" {
		t.Errorf("expected third role user, got %s", contents[2].Role)
	}
	if len(contents[2].Parts) != 1 {
		t.Fatalf("expected 1 part in tool result, got %d", len(contents[2].Parts))
	}
	fr := contents[2].Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected function response part")
	}
	if fr.Name != "calculator" {
		t.Errorf("expected tool name calculator, got %s", fr.Name)
	}
	if fr.ID != "call_1" {
		t.Errorf("expected tool call id call_1, got %s", fr.ID)
	}
}

func TestBuildContentsPreservesThoughtParts(t *testing.T) {
	messages := []ai.Message{
		{
			Role: ai.RoleAssistant,
			Metadata: map[string]any{
				"google_parts": []*genai.Part{
					{Text: "hmm", Thought: true, ThoughtSignature: []byte("sig")},
					{Text: "answer"},
				},
			},
		},
	}

	contents := buildContents(messages)

	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	parts := contents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if !parts[0].Thought {
		t.Error("expected first part to be a thought")
	}
	if string(parts[0].ThoughtSignature) != "sig" {
		t.Errorf("expected thought signature to be preserved, got %q", parts[0].ThoughtSignature)
	}
}

func TestSchemaFromJSON(t *testing.T) {
	raw := map[string]any{
		"type":        "object",
		"description": "a calculator",
		"properties": map[string]any{
			"expr": map[string]any{
				"type":        "string",
				"description": "the expression",
				"enum":        []any{"2+2", "3+3"},
			},
			"times": map[string]any{
				"type":    "integer",
				"minimum": int64(1),
				"maximum": int64(10),
			},
			"items": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
		},
		"required": []any{"expr"},
	}

	s := schemaFromJSON(raw)
	if s == nil {
		t.Fatal("expected non-nil schema")
	}
	if s.Type != genai.TypeObject {
		t.Errorf("expected OBJECT type, got %s", s.Type)
	}
	if s.Properties["expr"] == nil || s.Properties["expr"].Type != genai.TypeString {
		t.Error("expected expr property of type STRING")
	}
	if len(s.Properties["expr"].Enum) != 2 || s.Properties["expr"].Enum[0] != "2+2" {
		t.Errorf("expected enum to be preserved, got %v", s.Properties["expr"].Enum)
	}
	if s.Properties["times"].Minimum == nil || *s.Properties["times"].Minimum != 1 {
		t.Error("expected minimum to be set")
	}
	if s.Properties["items"].Items == nil || s.Properties["items"].Items.Type != genai.TypeNumber {
		t.Error("expected nested items schema")
	}
	if len(s.Required) != 1 || s.Required[0] != "expr" {
		t.Errorf("expected required [expr], got %v", s.Required)
	}
}

func TestFunctionDeclarations(t *testing.T) {
	tools := functionDeclarations([]ai.ToolSchema{
		{
			Name:        "calculator",
			Description: "do math",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expr": map[string]any{"type": "string"},
				},
			},
		},
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	decls := tools[0].FunctionDeclarations
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(decls))
	}
	if decls[0].Name != "calculator" {
		t.Errorf("expected name calculator, got %s", decls[0].Name)
	}
	if decls[0].Parameters == nil || decls[0].Parameters.Type != genai.TypeObject {
		t.Error("expected object parameters")
	}
}

func TestFunctionDeclarationsEmpty(t *testing.T) {
	if tools := functionDeclarations(nil); len(tools) != 0 {
		t.Errorf("expected no tools, got %d", len(tools))
	}
}

func TestStaticResponse(t *testing.T) {
	c := New(ai.ProviderConfig{APIKey: "test-key"})
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "reasoning", Thought: true, ThoughtSignature: []byte("s1")},
						{Text: "hello"},
						{
							FunctionCall: &genai.FunctionCall{
								Name: "calculator",
								Args: map[string]any{"expr": "2+2"},
								ID:   "fc_1",
							},
						},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}

	res := c.buildStaticResponse(resp)

	if res.StopReason() != ai.StopReasonTools {
		t.Errorf("expected StopReasonTools, got %s", res.StopReason())
	}
	calls := res.ToolCalls()
	if len(calls) != 1 || calls[0].ID != "fc_1" || calls[0].Name != "calculator" {
		t.Errorf("expected 1 tool call with id fc_1, got %+v", calls)
	}
	if u := res.Usage(); u.InputTokens != 10 || u.OutputTokens != 5 || u.TotalTokens != 15 {
		t.Errorf("unexpected usage: %+v", u)
	}

	md := res.GetMetadata()
	rawParts, ok := md["google_parts"].([]*genai.Part)
	if !ok || len(rawParts) != 3 {
		t.Fatalf("expected 3 raw parts in metadata, got %v", md)
	}
	if !rawParts[0].Thought || string(rawParts[0].ThoughtSignature) != "s1" {
		t.Error("expected thought signature preserved in metadata")
	}

	var chunks []ai.Chunk
	for ch := range res.Stream() {
		chunks = append(chunks, ch)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Thought != "reasoning" {
		t.Errorf("expected thought chunk, got %q", chunks[0].Thought)
	}
	if chunks[1].Text != "hello" {
		t.Errorf("expected text chunk, got %q", chunks[1].Text)
	}
	if !chunks[2].Done {
		t.Error("expected final done chunk")
	}
}

func TestMapError(t *testing.T) {
	c := New(ai.ProviderConfig{APIKey: "test-key"})

	err := c.mapError(genai.APIError{
		Code:    http.StatusTooManyRequests,
		Message: "rate limit exceeded",
		Status:  "RESOURCE_EXHAUSTED",
	})
	var oce *automergentErrors.AutomergentError
	if !errorsAs(err, &oce) {
		t.Fatalf("expected OweError, got %T", err)
	}
	if oce.Code != automergentErrors.CodeRateLimited {
		t.Errorf("expected rate limited, got %s", oce.Code)
	}

	err = c.mapError(genai.APIError{
		Code:    http.StatusTooManyRequests,
		Message: "quota exceeded for model",
	})
	errorsAs(err, &oce)
	if oce.Code != automergentErrors.CodeQuotaExceeded {
		t.Errorf("expected quota exceeded, got %s", oce.Code)
	}

	err = c.mapError(genai.APIError{Code: http.StatusUnauthorized, Message: "bad key"})
	errorsAs(err, &oce)
	if oce.Code != automergentErrors.CodeUnauthorized {
		t.Errorf("expected unauthorized, got %s", oce.Code)
	}

	err = c.mapError(genai.APIError{Code: http.StatusServiceUnavailable, Message: "busy"})
	errorsAs(err, &oce)
	if oce.Code != automergentErrors.CodeServiceUnavailable {
		t.Errorf("expected service unavailable, got %s", oce.Code)
	}

	err = c.mapError(genai.APIError{Code: http.StatusForbidden, Message: "nope"})
	errorsAs(err, &oce)
	if oce.Code != automergentErrors.CodeForbidden {
		t.Errorf("expected forbidden, got %s", oce.Code)
	}

	plain := c.mapError(automergentErrors.New(automergentErrors.CodeInternal, "boom"))
	if plain == nil {
		t.Fatal("expected non-nil error")
	}
}

func errorsAs(err error, target **automergentErrors.AutomergentError) bool {
	o, ok := err.(*automergentErrors.AutomergentError)
	if ok {
		*target = o
	}
	return ok
}

func TestNewVertexBackend(t *testing.T) {
	// An HTTPClient skips Application Default Credentials detection, keeping
	// the test hermetic (auth is delegated to the provided client).
	c := New(ai.ProviderConfig{Project: "my-project", Location: "us-central1", HTTPClient: &http.Client{}})
	if c.initErr != nil {
		t.Fatalf("expected no init error for vertex backend, got %v", c.initErr)
	}
	if c.client == nil {
		t.Fatal("expected client to be initialized")
	}
	if cc := c.client.ClientConfig(); cc.Backend != genai.BackendVertexAI {
		t.Errorf("expected vertex backend, got %v", cc.Backend)
	}
}

func TestNewGeminiBackend(t *testing.T) {
	c := New(ai.ProviderConfig{APIKey: "test-key"})
	if c.initErr != nil {
		t.Fatalf("expected no init error, got %v", c.initErr)
	}
	if cc := c.client.ClientConfig(); cc.Backend != genai.BackendGeminiAPI {
		t.Errorf("expected gemini api backend, got %v", cc.Backend)
	}
}

func TestNewMissingAuth(t *testing.T) {
	c := New(ai.ProviderConfig{})
	if c.initErr == nil {
		t.Fatal("expected init error without API key")
	}
	if _, err := c.Complete(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{ai.NewTextMessage(ai.RoleUser, "hi")},
	}); err == nil {
		t.Fatal("expected error from Complete with uninitialized client")
	}
}

func TestBuildGenerateContentConfigThinkingEffort(t *testing.T) {
	tests := []struct {
		name          string
		clientEffort  string
		reqEffort     string
		model         string
		expectedLevel genai.ThinkingLevel
	}{
		{
			name:          "default high for gemini 3",
			model:         "gemini-3.6-flash",
			expectedLevel: genai.ThinkingLevelHigh,
		},
		{
			name:          "request minimal effort",
			model:         "gemini-3.6-flash",
			reqEffort:     "minimal",
			expectedLevel: genai.ThinkingLevelMinimal,
		},
		{
			name:          "request low effort",
			model:         "gemini-3.6-flash",
			reqEffort:     "low",
			expectedLevel: genai.ThinkingLevelLow,
		},
		{
			name:          "client default medium effort",
			model:         "gemini-3.6-flash",
			clientEffort:  "medium",
			expectedLevel: genai.ThinkingLevelMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{model: tt.model, effort: tt.clientEffort}
			req := ai.CompletionRequest{
				Messages: []ai.Message{ai.NewTextMessage(ai.RoleUser, "test")},
				Thinking: &ai.ThinkingConfig{
					Type:   "enabled",
					Effort: tt.reqEffort,
				},
			}
			cfg := buildGenerateContentConfig(c, req)
			if cfg.ThinkingConfig == nil {
				t.Fatal("expected ThinkingConfig to be set")
			}
			if cfg.ThinkingConfig.ThinkingLevel != tt.expectedLevel {
				t.Errorf("expected ThinkingLevel %v, got %v", tt.expectedLevel, cfg.ThinkingConfig.ThinkingLevel)
			}
		})
	}
}
