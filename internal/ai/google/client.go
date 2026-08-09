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

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	var contents []geminiContent
	for _, m := range req.Messages {
		role := "user"
		if m.Role == ai.RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.TextContent()}},
		})
	}

	gr := geminiRequest{Contents: contents}
	if req.System != "" {
		gr.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.System}},
		}
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

	text := ""
	if len(gr2.Candidates) > 0 && len(gr2.Candidates[0].Content.Parts) > 0 {
		text = gr2.Candidates[0].Content.Parts[0].Text
	}
	usage := ai.Usage{
		InputTokens:  gr2.UsageMetadata.PromptTokenCount,
		OutputTokens: gr2.UsageMetadata.CandidatesTokenCount,
		TotalTokens:  gr2.UsageMetadata.TotalTokenCount,
	}
	return ai.NewStaticResponse(text, nil, ai.StopReasonEnd, usage), nil
}
