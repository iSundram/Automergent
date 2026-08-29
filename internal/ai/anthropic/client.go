package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
)

const (
	defaultBaseURL = "https://api.anthropic.com/v1"
	defaultModel   = "claude-3-5-sonnet-20241022"
	modelsCacheTTL = 5 * time.Minute
)

var anthropicModels = []ai.Model{
	{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", ContextLimit: 200000, InputPrice: 3, OutputPrice: 15},
	{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ContextLimit: 200000, InputPrice: 1, OutputPrice: 5},
	{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", ContextLimit: 200000, InputPrice: 15, OutputPrice: 75},
	{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet", ContextLimit: 200000, InputPrice: 3, OutputPrice: 15},
	{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku", ContextLimit: 200000, InputPrice: 0.25, OutputPrice: 1.25},
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	limit      int

	temperature *float64
	maxTokens   int
	maxRetries  int
	headers     map[string]string

	retryMu sync.RWMutex
	onRetry func(ai.RetryInfo)
}

func New(cfg ai.ProviderConfig) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.TimeoutSeconds > 0 {
		httpClient.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return &Client{
		httpClient:   httpClient,
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:       cfg.APIKey,
		model:        cfg.DefaultModel,
		temperature:  cfg.Temperature,
		maxTokens:    cfg.MaxTokens,
		maxRetries:   cfg.MaxRetries,
		headers:      cfg.Headers,
	}
}

func (c *Client) Name() string { return "anthropic" }

func (c *Client) ContextLimit() int { return c.limit }

func (c *Client) TokenCount(messages []ai.Message) (int, error) {
	totalChars := 0
	for _, m := range messages {
		for _, part := range m.Content {
			totalChars += len(part.Text)
		}
	}
	return totalChars / 4, nil
}

func (c *Client) SetRetryObserver(fn func(ai.RetryInfo)) {
	c.retryMu.Lock()
	c.onRetry = fn
	c.retryMu.Unlock()
}

func (c *Client) retryObserver() func(ai.RetryInfo) {
	c.retryMu.RLock()
	defer c.retryMu.RUnlock()
	return c.onRetry
}

func (c *Client) notifyRetry(attempt, maxAttempts int, err error, delay time.Duration) {
	fn := c.retryObserver()
	if fn == nil {
		return
	}
	info := ai.RetryInfo{
		Provider:    "anthropic",
		Model:       c.model,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		Delay:       delay,
	}
	if e := new(automergentErrors.AutomergentError); errors.As(err, &e) {
		info.Code = string(e.Code)
		info.Message = e.Message
	} else {
		info.Message = err.Error()
	}
	fn(info)
}

func (c *Client) Models(ctx context.Context) ([]ai.Model, error) {
	// Return curated list since Anthropic doesn't have a public models endpoint
	return anthropicModels, nil
}

func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	c.model = string(req.Messages[len(req.Messages)-1].Role)

	messages := convertMessages(req.Messages)
	body := map[string]any{
		"model":       c.model,
		"max_tokens":  req.MaxTokens,
		"messages":    messages,
		"temperature": req.Temperature,
		"stream":      req.Stream,
	}
	if req.MaxTokens == 0 && c.maxTokens > 0 {
		body["max_tokens"] = c.maxTokens
	}
	if c.temperature != nil {
		body["temperature"] = *c.temperature
	}

	jsonBody, _ := json.Marshal(body)

	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("x-api-key", c.apiKey)
	reqHTTP.Header.Set("anthropic-version", "2023-06-01")
	for k, v := range c.headers {
		reqHTTP.Header.Set(k, v)
	}

	resp, err := c.doWithRetry(ctx, reqHTTP)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("completion failed: %s", resp.Status)
	}

	if req.Stream {
		return &streamResponse{resp: resp}, nil
	}

	var data struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var text string
	for _, c := range data.Content {
		text += c.Text
	}

	return &completionResponse{
		text: text,
		usage: ai.Usage{
			InputTokens:  data.Usage.InputTokens,
			OutputTokens: data.Usage.OutputTokens,
			TotalTokens:  data.Usage.InputTokens + data.Usage.OutputTokens,
		},
	}, nil
}

func convertMessages(messages []ai.Message) []map[string]any {
	var result []map[string]any
	for _, m := range messages {
		role := "user"
		if m.Role == ai.RoleAssistant {
			role = "assistant"
		} else if m.Role == ai.RoleSystem {
			role = "user" // Anthropic doesn't have system role, prepend to first user message
		}
		for _, part := range m.Content {
			if part.Type == ai.ContentTypeText {
				result = append(result, map[string]any{
					"role":    role,
					"content": part.Text,
				})
			}
		}
	}
	return result
}

func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	maxAttempts := c.maxRetries
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err == nil && (resp.StatusCode < 500 || resp.StatusCode == 429) {
			if resp.StatusCode == 429 {
				if attempt < 5 {
					time.Sleep(time.Duration(attempt) * time.Second)
					resp.Body.Close()
					continue
				}
				return resp, nil
			}
			return resp, nil
		}
		lastErr = err
		if attempt < 5 {
			delay := time.Duration(attempt) * time.Second
			c.notifyRetry(attempt, 5, lastErr, delay)
			time.Sleep(delay)
		}
	}
	return nil, lastErr
}

type completionResponse struct {
	text  string
	usage ai.Usage
}

func (r *completionResponse) Stream() <-chan ai.Chunk {
	ch := make(chan ai.Chunk, 1)
	ch <- ai.Chunk{Text: r.text, Done: true}
	close(ch)
	return ch
}
func (r *completionResponse) ToolCalls() []ai.ToolCall      { return nil }
func (r *completionResponse) StopReason() ai.StopReason     { return ai.StopReasonEnd }
func (r *completionResponse) Usage() ai.Usage               { return r.usage }
func (r *completionResponse) GetMetadata() map[string]any   { return nil }

type streamResponse struct {
	resp *http.Response
	dec  *json.Decoder
}

func (s *streamResponse) Stream() <-chan ai.Chunk {
	ch := make(chan ai.Chunk)
	go func() {
		defer close(ch)
		s.dec = json.NewDecoder(s.resp.Body)
		for {
			var chunk struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := s.dec.Decode(&chunk); err != nil {
				ch <- ai.Chunk{Done: true}
				return
			}
			if chunk.Type == "content_block_delta" && chunk.Delta.Text != "" {
				ch <- ai.Chunk{Text: chunk.Delta.Text}
			}
		}
	}()
	return ch
}
func (s *streamResponse) ToolCalls() []ai.ToolCall      { return nil }
func (s *streamResponse) StopReason() ai.StopReason     { return ai.StopReasonEnd }
func (s *streamResponse) Usage() ai.Usage               { return ai.Usage{} }
func (s *streamResponse) GetMetadata() map[string]any   { return nil }