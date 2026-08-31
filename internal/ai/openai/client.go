package openai

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
	"github.com/iSundram/Automergent/internal/modelsdev"
)

const (
	defaultBaseURL       = "https://api.openai.com/v1"
	defaultModel         = "gpt-5.5"
	modelsCacheTTL       = 5 * time.Minute
	modelsEndpoint       = "/v1/models"
	completionsEndpoint  = "/v1/chat/completions"
)

// Client implements ai.Provider for OpenAI-compatible APIs.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	limit      int
	effort     string

	// Provider-level request defaults
	temperature *float64
	maxTokens   int
	maxRetries  int
	headers     map[string]string

	// Model list cache
	modelsMu       sync.Mutex
	cachedModels   []ai.Model
	modelsCachedAt time.Time

	// Retry observer
	retryMu sync.RWMutex
	onRetry func(ai.RetryInfo)
}

// New creates a new OpenAI-compatible client.
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
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       cfg.APIKey,
		model:        cfg.DefaultModel,
		temperature:  cfg.Temperature,
		maxTokens:    cfg.MaxTokens,
		maxRetries:   cfg.MaxRetries,
		headers:      cfg.Headers,
		effort:       cfg.Effort,
	}
}

func (c *Client) Name() string { return "openai" }

func (c *Client) ContextLimit() int {
	return c.limit
}

func (c *Client) TokenCount(messages []ai.Message) (int, error) {
	// Rough estimate: 4 chars ≈ 1 token for English
	totalChars := 0
	for _, m := range messages {
		for _, part := range m.Content {
			totalChars += len(part.Text)
		}
	}
	return totalChars / 4, nil
}

// SetRetryObserver installs a callback for retried attempts.
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
		Provider:    "openai",
		Model:       c.model,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		Delay:       delay,
	}
	if e := new(automergentErrors.AutomergentError); errors.As(err, &e) {
		info.Code = string(e.Code)
		info.Message = e.Message
		if s, ok := e.Context["status_code"]; ok {
			if sc, ok := s.(int); ok {
				info.Status = fmt.Sprintf("%d", sc)
			}
		}
	} else {
		info.Message = err.Error()
	}
	fn(info)
}

// Models returns the list of available models: live availability from the
// API's model listing enriched with models.dev metadata (names, context
// limits, pricing, capabilities). The listing filter (tool calling, >= 1M
// context) is enforced by the catalog, so a live ID the catalog doesn't know
// — or that fails the filter — is dropped.
func (c *Client) Models(ctx context.Context) ([]ai.Model, error) {
	c.modelsMu.Lock()
	if time.Since(c.modelsCachedAt) < modelsCacheTTL && c.cachedModels != nil {
		models := c.cachedModels
		c.modelsMu.Unlock()
		return models, nil
	}
	c.modelsMu.Unlock()

	catalog := modelsdev.Models(ctx, "openai")
	live, err := c.liveModelIDs(ctx)
	if err != nil {
		// Offline: the catalog alone still answers (snapshot fallback).
		return catalog, nil
	}
	liveSet := make(map[string]bool, len(live))
	for _, id := range live {
		liveSet[id] = true
	}
	var models []ai.Model
	for _, m := range catalog {
		if liveSet[m.ID] || len(live) == 0 {
			models = append(models, m)
		}
	}
	if models == nil {
		models = catalog
	}

	c.modelsMu.Lock()
	c.cachedModels = models
	c.modelsCachedAt = time.Now()
	c.modelsMu.Unlock()
	return models, nil
}

// liveModelIDs fetches the provider's model IDs (availability only — no
// metadata; that comes from the catalog).
func (c *Client) liveModelIDs(ctx context.Context) ([]string, error) {
	url := c.baseURL + modelsEndpoint
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models list failed: %s", resp.Status)
	}

	var data struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var ids []string
	for _, m := range data.Data {
		if m.Object != "model" || m.ID == "" {
			continue
		}
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// Complete sends a completion request to the OpenAI-compatible API.
func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	c.model = string(req.Messages[len(req.Messages)-1].Role)

	// Build request body
	body := map[string]any{
		"model":       c.model,
		"messages":    convertMessages(req.Messages),
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      req.Stream,
	}
	if c.temperature != nil {
		body["temperature"] = *c.temperature
	}
	if c.maxTokens > 0 && req.MaxTokens == 0 {
		body["max_tokens"] = c.maxTokens
	}

	jsonBody, _ := json.Marshal(body)

	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", "Bearer "+c.apiKey)
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
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var text string
	if len(data.Choices) > 0 {
		text = data.Choices[0].Message.Content
	}

	return &completionResponse{
		text: text,
		usage: ai.Usage{
			InputTokens:  data.Usage.PromptTokens,
			OutputTokens: data.Usage.CompletionTokens,
			TotalTokens:  data.Usage.TotalTokens,
		},
	}, nil
}

func convertMessages(messages []ai.Message) []map[string]any {
	var result []map[string]any
	for _, m := range messages {
		for _, part := range m.Content {
			if part.Type == ai.ContentTypeText {
				result = append(result, map[string]any{
					"role":    string(m.Role),
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
				// Rate limited - wait and retry
				if attempt < maxAttempts {
					retryAfter := 1 * time.Second
					if ra := resp.Header.Get("Retry-After"); ra != "" {
						if secs, err := parseRetryAfter(ra); err == nil {
							retryAfter = secs
						}
					}
					time.Sleep(retryAfter)
					resp.Body.Close()
					continue
				}
				return resp, nil
			}
			return resp, nil
		}
		lastErr = err
		if attempt < maxAttempts {
			delay := time.Duration(attempt) * time.Second
			c.notifyRetry(attempt, maxAttempts, lastErr, delay)
			time.Sleep(delay)
		}
	}
	return nil, lastErr
}

func parseRetryAfter(ra string) (time.Duration, error) {
	// Simple parsing - could be extended
	var secs int
	_, err := fmt.Sscanf(ra, "%d", &secs)
	if err == nil && secs > 0 {
		return time.Duration(secs) * time.Second, nil
	}
	return 2 * time.Second, fmt.Errorf("invalid retry-after")
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
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := s.dec.Decode(&chunk); err != nil {
				ch <- ai.Chunk{Done: true}
				return
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				ch <- ai.Chunk{Text: chunk.Choices[0].Delta.Content}
			}
		}
	}()
	return ch
}
func (s *streamResponse) ToolCalls() []ai.ToolCall      { return nil }
func (s *streamResponse) StopReason() ai.StopReason     { return ai.StopReasonEnd }
func (s *streamResponse) Usage() ai.Usage               { return ai.Usage{} }
func (s *streamResponse) GetMetadata() map[string]any   { return nil }