package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/modelsdev"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"google.golang.org/genai"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel   = "gemini-3.6-flash"

	// backendAIStudio is the Gemini API (AI Studio); backendVertex is Vertex
	// AI (Google Cloud). These strings appear in user config.
	backendAIStudio = "aistudio"
	backendVertex   = "vertex"

	// modelsCacheTTL bounds how long the live model list is reused before
	// the next Models call hits the API again.
	modelsCacheTTL = 5 * time.Minute
)

// isGemini25 returns true if the model is a Gemini 2.5 series model.
func isGemini25(model string) bool {
	return strings.HasPrefix(model, "gemini-2.5")
}

// isGemini3 returns true if the model is a Gemini 3.x series model.
func isGemini3(model string) bool {
	return strings.HasPrefix(model, "gemini-3")
}

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
	effort  string
	backend string

	// Provider-level request defaults and transport tuning.
	temperature *float64
	maxTokens   int
	maxRetries  int

	// Live model list cache, refreshed on demand by LiveModels.
	modelsMu       sync.Mutex
	cachedModels   []ai.Model
	modelsCachedAt time.Time

	// onRetry is notified of each retried API attempt. Guarded by retryMu
	// because Complete runs on the agent goroutine while the observer is
	// installed from the UI goroutine on provider switch.
	retryMu sync.RWMutex
	onRetry func(ai.RetryInfo)
}

// SetRetryObserver installs a callback notified of every retried API attempt.
// Implements ai.RetryObserver.
func (c *Client) SetRetryObserver(fn func(ai.RetryInfo)) {
	c.retryMu.Lock()
	c.onRetry = fn
	c.retryMu.Unlock()
}

// retryObserver returns the installed observer, if any.
func (c *Client) retryObserver() func(ai.RetryInfo) {
	c.retryMu.RLock()
	defer c.retryMu.RUnlock()
	return c.onRetry
}

// notifyRetry reports one failed attempt to the observer. It classifies the
// error through the same AutomergentError codes mapError produces, so the UI
// shows the same code the error log records.
func (c *Client) notifyRetry(attempt, maxAttempts int, err error, delay time.Duration) {
	fn := c.retryObserver()
	if fn == nil {
		return
	}
	info := ai.RetryInfo{
		Provider:    "google",
		Model:       c.model,
		Message:     err.Error(),
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		Delay:       delay,
	}
	var oce *automergentErrors.AutomergentError
	if errors.As(err, &oce) && oce != nil {
		info.Code = string(oce.Code)
		info.Message = oce.Message
		if status, ok := oce.Context["status_code"]; ok {
			info.Status = fmt.Sprint(status)
		}
	}
	fn(info)
}

func New(cfg ai.ProviderConfig) *Client {
	model := cfg.DefaultModel
	if model == "" {
		model = defaultModel
	}

	// Backend selection: explicit config wins; otherwise a configured
	// project+location implies Vertex AI (Google Cloud), and anything else is
	// the Gemini API (AI Studio).
	backend := cfg.Backend
	if backend == "" {
		if cfg.Project != "" && cfg.Location != "" {
			backend = backendVertex
		} else {
			backend = backendAIStudio
		}
	}

	cc := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		HTTPClient: cfg.HTTPClient,
	}
	if cc.HTTPClient == nil && cfg.TimeoutSeconds > 0 {
		cc.HTTPClient = &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
	}

	// Extra headers ride along for custom endpoints (gateways, proxies with
	// their own auth schemes) on either backend.
	httpOpts := genai.HTTPOptions{}
	if len(cfg.Headers) > 0 {
		h := http.Header{}
		for k, v := range cfg.Headers {
			h.Set(k, v)
		}
		httpOpts.Headers = h
	}

	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}

	if backend == backendVertex {
		cc.Backend = genai.BackendVertexAI
		cc.Project = cfg.Project
		cc.Location = cfg.Location
		// Vertex derives its endpoint from the location; baseURL is kept only
		// for error messages and status output.
		if cfg.Location != "" {
			base = fmt.Sprintf("https://%s-aiplatform.googleapis.com", cfg.Location)
		} else {
			base = "https://aiplatform.googleapis.com"
		}
	} else {
		if cfg.Backend == backendAIStudio {
			// An explicit backend choice overrides the SDK's own
			// GOOGLE_GENAI_USE_VERTEXAI environment inference.
			cc.Backend = genai.BackendGeminiAPI
		}
		if cfg.BaseURL != "" {
			// The genai SDK appends its own API version (v1beta) to the base
			// URL, so a config base URL that already ends in /v1beta must have
			// that segment stripped, otherwise requests go to
			// .../v1beta/v1beta/... and fail forever.
			httpOpts.BaseURL = stripAPIVersion(cfg.BaseURL)
		}
	}
	if httpOpts.BaseURL != "" || httpOpts.Headers != nil {
		cc.HTTPOptions = httpOpts
	}

	c := &Client{
		model:       model,
		limit:       maxContextTokens,
		baseURL:     base,
		effort:      cfg.Effort,
		backend:     backend,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
		maxRetries:  cfg.MaxRetries,
	}
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

// Backend reports which Google backend is in force: "aistudio" (Gemini API)
// or "vertex" (Vertex AI).
func (c *Client) Backend() string { return c.backend }

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

// curatedModels is the offline fallback and enrichment source for the live
// model list: display names, context limits and pricing for known models. New
// curated entries that the API does not list yet remain selectable.
var curatedModels = []ai.Model{
	{ID: "gemini-3.6-flash", Name: "Gemini 3.6 Flash", ContextLimit: 1048576},
	{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", ContextLimit: 1048576},
	{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", ContextLimit: 1048576},
	{ID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash-Lite", ContextLimit: 1048576},
	{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview", ContextLimit: 1048576},
	{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", ContextLimit: 1048576},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextLimit: 1048576},
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextLimit: 1048576},
	{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash-Lite", ContextLimit: 1048576},
	{ID: "gemini-2.5-flash-preview", Name: "Gemini 2.5 Flash Preview", ContextLimit: 1048576},
	{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextLimit: 1048576},
	{ID: "gemini-2.0-flash-001", Name: "Gemini 2.0 Flash", ContextLimit: 1048576},
	{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash-Lite", ContextLimit: 1048576},
	{ID: "gemini-2.0-flash-lite-001", Name: "Gemini 2.0 Flash-Lite", ContextLimit: 1048576},
}

// Models returns the provider's model list. It serves the live list from
// cache when fresh, refreshes from the models.list API otherwise, and falls
// back to the curated static list when the API is unreachable — so the
// palette and /model list never come up empty on a flaky network.
func (c *Client) Models(ctx context.Context) ([]ai.Model, error) {
	if cached := c.cachedModelList(); cached != nil {
		return cached, nil
	}
	models, err := c.LiveModels(ctx)
	if err != nil {
		// Offline: the models.dev catalog (disk cache or embedded snapshot)
		// is richer than the curated list; curated stays beneath it.
		if catalog := modelsdev.Models(ctx, "google"); len(catalog) > 0 {
			return catalog, nil
		}
		return append([]ai.Model{}, curatedModels...), nil
	}
	return models, nil
}

// LiveModels enumerates models through the models.list API (works for both
// AI Studio and Vertex AI backends), enriches them with curated metadata and
// refreshes the cache. Unlike Models it reports errors instead of falling
// back, which is what /provider test needs to detect broken credentials.
func (c *Client) LiveModels(ctx context.Context) ([]ai.Model, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var live []ai.Model
	for m, err := range c.client.Models.All(ctx) {
		if err != nil {
			return nil, c.mapError(err)
		}
		if model, ok := convertListedModel(m); ok {
			live = append(live, model)
		}
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("google: model listing returned no usable models")
	}
	merged := mergeCuratedModels(live)

	c.modelsMu.Lock()
	c.cachedModels = merged
	c.modelsCachedAt = time.Now()
	c.modelsMu.Unlock()
	return merged, nil
}

// InvalidateModelsCache forces the next Models call to hit the API. Used by
// /model refresh.
func (c *Client) InvalidateModelsCache() {
	c.modelsMu.Lock()
	c.modelsCachedAt = time.Time{}
	c.modelsMu.Unlock()
}

func (c *Client) cachedModelList() []ai.Model {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()
	if c.cachedModels == nil || time.Since(c.modelsCachedAt) > modelsCacheTTL {
		return nil
	}
	return append([]ai.Model{}, c.cachedModels...)
}

// convertListedModel maps an API model resource to ai.Model. Names arrive as
// "models/gemini-2.0-flash" (AI Studio) or
// "projects/…/publishers/google/models/gemini-…" (Vertex); the model ID is the
// last path segment. Non-generative resources (embeddings, tuned artifacts)
// are dropped.
func convertListedModel(m *genai.Model) (ai.Model, bool) {
	id := m.Name
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	if id == "" {
		return ai.Model{}, false
	}
	if len(m.SupportedActions) > 0 {
		usable := false
		for _, a := range m.SupportedActions {
			if a == "generateContent" {
				usable = true
				break
			}
		}
		if !usable {
			return ai.Model{}, false
		}
	} else if !strings.Contains(id, "gemini") {
		return ai.Model{}, false
	}
	name := m.DisplayName
	if name == "" {
		name = id
	}
	return ai.Model{ID: id, Name: name, ContextLimit: clampContextLimit(int(m.InputTokenLimit))}, true
}

// maxContextTokens is the platform ceiling on any model's advertised window.
// Providers list multi-million-token limits for some models; beyond 1M the
// estimation drift between our counting and the provider's real counting
// compounds to the point the context ladder misfires, so everything is
// clamped here.
const maxContextTokens = 1_048_576

func clampContextLimit(n int) int {
	if n <= 0 {
		return n
	}
	if n > maxContextTokens {
		return maxContextTokens
	}
	return n
}

// mergeCuratedModels enriches live entries with curated names, context limits
// (only when the API omitted them) and pricing, and appends curated models
// the API does not list (e.g. brand-new previews) so they stay selectable.
func mergeCuratedModels(live []ai.Model) []ai.Model {
	byID := make(map[string]int, len(live))
	for i, m := range live {
		byID[m.ID] = i
	}
	merge := func(extra []ai.Model, authoritative bool) {
		for _, m := range extra {
			if i, ok := byID[m.ID]; ok {
				if live[i].Name == live[i].ID {
					live[i].Name = m.Name
				}
				if live[i].ContextLimit == 0 {
					live[i].ContextLimit = m.ContextLimit
				}
				if m.OutputLimit > 0 {
					live[i].OutputLimit = m.OutputLimit
				}
				// Catalog pricing is authoritative when present; curated
				// entries fill gaps only.
				if authoritative || (live[i].InputPrice == 0 && live[i].OutputPrice == 0) {
					live[i].InputPrice = m.InputPrice
					live[i].OutputPrice = m.OutputPrice
				}
				if m.Reasoning {
					live[i].Reasoning = true
				}
				if m.Attachment {
					live[i].Attachment = true
				}
				if m.Knowledge != "" {
					live[i].Knowledge = m.Knowledge
				}
			} else {
				live = append(live, m)
				byID[m.ID] = len(live) - 1
			}
		}
	}
	// The models.dev community catalog first (rich metadata), then the
	// curated static list for entries the catalog has not caught up with.
	merge(modelsdev.Models(context.Background(), "google"), true)
	merge(curatedModels, false)
	return live
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

		case ai.RoleSystem:
			// Mid-conversation system messages (compaction summaries, stall
			// nudges, goal reminders) must NOT be dropped: they sit between
			// assistant turns, and removing them made two model turns
			// adjacent — the API rejects that with INVALID_INPUT. Render
			// them as user-role text, merged into the previous user turn
			// when there is one so they cannot create runs of user contents.
			t := m.TextContent()
			if t == "" {
				continue
			}
			if n := len(out); n > 0 && out[n-1].Role == string(genai.RoleUser) && len(out[n-1].Parts) == 1 && out[n-1].Parts[0].Text != "" {
				out[n-1].Parts[0].Text += "\n\n" + t
				continue
			}
			if i == 0 {
				// A leading system message is the platform context; the
				// System field usually carries it, so skip a duplicate.
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

	// Belt-and-braces alternation: coalesce adjacent model contents into
	// single turns. The agent merges consecutive assistants before the
	// request, but metadata-rebuilt parts and resumed sessions can still
	// produce runs; the API rejects any adjacent model turns.
	coalesced := out[:0]
	for _, c := range out {
		if n := len(coalesced); n > 0 && c.Role == string(genai.RoleModel) &&
			coalesced[n-1].Role == string(genai.RoleModel) {
			coalesced[n-1].Parts = append(coalesced[n-1].Parts, c.Parts...)
			continue
		}
		coalesced = append(coalesced, c)
	}
	out = coalesced

	// Gemini requires the last content to be from the user role.
	// Strip any trailing model-role entries to avoid a 400 error:
	// "Requests ending with a model turn are not supported."
	// Only strip when a user-role content remains so we don't produce an
	// empty slice for single-assistant-message edge cases.
	for len(out) > 1 && out[len(out)-1].Role == string(genai.RoleModel) {
		out = out[:len(out)-1]
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
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens > 0 {
		config.MaxOutputTokens = int32(maxTokens) //nolint:gosec // bounded by provider limits
	}
	temperature := req.Temperature
	if temperature == 0 && c.temperature != nil {
		temperature = *c.temperature
	}
	if temperature > 0 {
		temp := float32(temperature)
		config.Temperature = &temp
	}
	// Enable thinking mode if configured
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		tc := &genai.ThinkingConfig{IncludeThoughts: req.Thinking.IncludeThoughts}

		// Gemini 2.5 uses thinkingBudget; Gemini 3 uses thinkingLevel.
		if isGemini25(c.model) {
			if req.Thinking.BudgetTokens != 0 {
				budget := int32(req.Thinking.BudgetTokens)
				tc.ThinkingBudget = &budget
			} else {
				budget := int32(-1) // Dynamic thinking budget.
				tc.ThinkingBudget = &budget
			}
		} else if isGemini3(c.model) {
			// Gemini 3.x uses thinking_level
			level := req.Thinking.ThinkingLevel
			if level == "" {
				level = req.Thinking.Effort // fallback to legacy Effort field
			}
			if level == "" {
				level = c.effort // fallback to client config
			}
			if level == "" {
				level = "high" // Default for Gemini 3
			}
			switch strings.ToLower(level) {
			case "minimal":
				tc.ThinkingLevel = genai.ThinkingLevelMinimal
			case "low":
				tc.ThinkingLevel = genai.ThinkingLevelLow
			case "medium":
				tc.ThinkingLevel = genai.ThinkingLevelMedium
			case "high":
				tc.ThinkingLevel = genai.ThinkingLevelHigh
			default:
				tc.ThinkingLevel = genai.ThinkingLevelHigh
			}
		} else {
			// Legacy: map Effort to thinkingLevel for older models
			effort := req.Thinking.Effort
			if effort == "" {
				effort = c.effort
			}
			switch strings.ToLower(effort) {
			case "minimal":
				tc.ThinkingLevel = genai.ThinkingLevelMinimal
			case "low":
				tc.ThinkingLevel = genai.ThinkingLevelLow
			case "medium":
				tc.ThinkingLevel = genai.ThinkingLevelMedium
			case "high":
				tc.ThinkingLevel = genai.ThinkingLevelHigh
			default:
				tc.ThinkingLevel = genai.ThinkingLevelHigh
			}
		}

		config.ThinkingConfig = tc
	}
	return config
}

func (c *Client) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	// Both pre-flight failures are classified rather than returned as bare
	// fmt.Errorf: they reach the UI through the same EventError path as API
	// failures, and the error log shows a code for every entry.
	if c.initErr != nil {
		return nil, automergentErrors.Wrap(c.initErr, automergentErrors.CodeConfigInvalid,
			"google: client initialization failed").
			WithResource(c.baseURL).
			WithSuggestion("Check the provider API key and base URL (/api-key, /base-url)")
	}
	// Repair consecutive assistant messages that can appear after an
	// interrupted agentic turn or autocompact rewrite. Must run before
	// Validate() which rejects them as a hard error.
	req.Messages = ai.MergeConsecutiveAssistantMessages(req.Messages)
	if err := req.Validate(); err != nil {
		return nil, automergentErrors.Wrap(err, automergentErrors.CodeInvalidInput,
			"google: invalid message sequence")
	}
	contents := buildContents(req.Messages)
	if len(contents) == 0 {
		contents = []*genai.Content{genai.NewContentFromText(" ", genai.RoleUser)}
	}
	config := buildGenerateContentConfig(c, req)

	// Wrap the request in a retry loop
	policy := automergentErrors.AggressiveRetryPolicy()
	if c.maxRetries > 0 {
		policy.MaxAttempts = c.maxRetries
	} else {
		policy.MaxAttempts = 10
	}
	// Report each retried attempt so the UI can show "retrying (3/10)" rather
	// than appearing to hang for the duration of the backoff.
	maxAttempts := policy.MaxAttempts
	policy.OnRetry = func(attempt int, err error, delay time.Duration) {
		c.notifyRetry(attempt, maxAttempts, err, delay)
	}

	if !req.Stream {
		resp, retryResult := automergentErrors.RetryWithValue(ctx, policy, func() (*genai.GenerateContentResponse, error) {
			resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
			if err != nil {
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
		case http.StatusBadRequest:
			// The API reports oversized prompts as a plain 400; surface them
			// as CONTEXT_TOO_LONG so the agent loop can reactively compact
			// and retry instead of failing the turn.
			if isContextLengthMessage(msg) {
				code = automergentErrors.CodeContextTooLong
			}
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

// isContextLengthMessage matches the phrasing providers use when a request
// exceeds the model's context window.
func isContextLengthMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range []string{
		"context length", "context window", "too many tokens",
		"exceeds the maximum number of tokens", "input tokens exceed",
		"token limit", "request too large",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
