package ai

import (
	"context"
	"net/http"
	"time"
)

// Role identifies who authored a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType describes the kind of content in a message part.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeThought    ContentType = "thought"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolCall   ContentType = "tool_call"
	ContentTypeToolResult ContentType = "tool_result"
)

// StopReason describes why the model stopped generating.
type StopReason string

const (
	StopReasonEnd     StopReason = "end"
	StopReasonTools   StopReason = "tool_calls"
	StopReasonLength  StopReason = "length"
	StopReasonStopped StopReason = "stopped"
)

// Provider is the interface that every AI backend must satisfy.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Models(ctx context.Context) ([]Model, error)
	TokenCount(messages []Message) (int, error)
	ContextLimit() int
}

// CompletionRequest is the input to the provider.
type CompletionRequest struct {
	Messages    []Message
	Tools       []ToolSchema
	System      string
	Temperature float64
	MaxTokens   int
	Stream      bool
	// Thinking enables extended thinking for supported models (Gemini).
	// When enabled, the model uses additional tokens to reason before responding.
	Thinking *ThinkingConfig
}

// Validate checks request-level protocol constraints.
func (r CompletionRequest) Validate() error {
	return ValidateMessageSequence(r.Messages)
}

// ThinkingConfig controls extended thinking behavior.
type ThinkingConfig struct {
	// Type should be "enabled" to activate extended thinking
	Type string
	// BudgetTokens is the maximum tokens allocated for thinking (Gemini 2.5 only).
	// Range: 0-32768 (2.5 Pro), 0-24576 (2.5 Flash). Use -1 for dynamic.
	BudgetTokens int
	// ThinkingLevel specifies the reasoning effort level for Gemini 3+ models.
	// Valid values: "minimal", "low", "medium", "high".
	ThinkingLevel string
	// Stream enables streaming thought chunks when supported by the provider.
	Stream bool
	// IncludeThoughts includes thought summaries in the streaming response.
	IncludeThoughts bool
	// Effort specifies the reasoning/thinking effort level (legacy, maps to ThinkingLevel).
	Effort string
}

// CompletionResponse is returned by a provider.
type CompletionResponse interface {
	Stream() <-chan Chunk
	ToolCalls() []ToolCall
	StopReason() StopReason
	Usage() Usage
	GetMetadata() map[string]any
}

// Message is a single turn in the conversation.
type Message struct {
	Role     Role
	Content  []ContentPart
	Metadata map[string]any
}

// NewTextMessage is a convenience constructor.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role:    role,
		Content: []ContentPart{{Type: ContentTypeText, Text: text}},
	}
}

// ContentPart is one segment of a message.
type ContentPart struct {
	Type       ContentType
	Text       string
	Thought    string
	ImageURL   string
	ToolCall   *ToolCall
	ToolResult *ToolResult
}

// ToolCall represents a function invocation requested by the model.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult holds the outcome of a tool invocation.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolSchema describes a tool the model can call.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Chunk is a streaming token from the model.
type Chunk struct {
	Text      string
	Thought   string // Extended thinking content (Gemini)
	ToolCalls []ToolCall
	Done      bool
	Error     error
}

// Usage describes token usage for a completion.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CacheHits    int
}

// Model describes an AI model offered by a provider.
type Model struct {
	ID           string
	Name         string
	ContextLimit int
	InputPrice   float64
	OutputPrice  float64
	// OutputLimit is the model's maximum output tokens (0 = unknown).
	OutputLimit int
	// Reasoning marks thinking-capable models (effort control applies).
	Reasoning bool
	// Efforts lists the reasoning effort levels the model accepts
	// ({"low", "medium", "high"}), from the models.dev catalog. Empty means
	// the model either doesn't reason or exposes no effort control.
	Efforts []string
	// Attachment marks models that accept file/image attachments.
	Attachment bool
	// Knowledge is the model's knowledge cutoff ("2026-03"), when known.
	Knowledge string
	// Released is the model's release date ("2026-06-29"), when known.
	Released string
	// CacheReadPrice / CacheWritePrice are the per-1M-token prompt cache
	// prices (USD) when the provider bills cache traffic separately.
	CacheReadPrice  float64
	CacheWritePrice float64
}

// ProviderConfig holds provider-level credentials and defaults.
type ProviderConfig struct {
	APIKey             string
	BaseURL            string
	DefaultModel       string
	OrgID              string
	Project            string
	Location           string
	// Backend selects between provider backends when the provider ships more
	// than one (Google: "aistudio" for the Gemini API, "vertex" for Vertex AI).
	// Empty means the provider chooses (Google: project+location implies
	// Vertex, otherwise the Gemini API).
	Backend            string
	Effort             string
	ThinkingLevel      string
	HTTPClient         *http.Client
	PromptCacheEnabled *bool
	// Headers are extra HTTP headers sent with every request (gateways,
	// proxies, custom auth schemes for custom endpoints).
	Headers map[string]string
	// Temperature and MaxTokens are provider-level defaults applied when the
	// completion request leaves them at zero.
	Temperature *float64
	MaxTokens   int
	// TimeoutSeconds bounds each API call when > 0.
	TimeoutSeconds int
	// MaxRetries caps the in-provider retry attempts when > 0.
	MaxRetries int
}

// Event is a generic event emitted during a completion.
type Event struct {
	Type    string
	Payload any
}

// RetryInfo describes one retry attempt against a provider API. Providers that
// retry internally report each attempt through a RetryObserver so the UI can
// show that a request is being retried rather than appearing to hang.
type RetryInfo struct {
	// Provider and Model identify what was being called.
	Provider string
	Model    string
	// Code is the classified error code, e.g. "RATE_LIMITED".
	Code string
	// Status is the transport status when there was one, e.g. "429".
	Status string
	// Message is the provider's error text. Callers must treat this as
	// untrusted: it can embed request URLs, so sanitize before display.
	Message string
	// Attempt is the 1-based attempt that just failed; MaxAttempts is the
	// policy's ceiling. Attempt == MaxAttempts means no further retry follows.
	Attempt     int
	MaxAttempts int
	// Delay is how long the provider will wait before the next attempt.
	Delay time.Duration
}

// Retriable reports whether another attempt follows this one.
func (r RetryInfo) Retriable() bool { return r.Attempt < r.MaxAttempts }

// RetryObserver is implemented by providers that retry API calls internally
// and can report those attempts to a caller.
//
// Wrapping providers (caching, debug, and any future decorator) MUST forward
// this to the wrapped provider. A wrapper that omits it makes the interface
// assertion fail silently and retries become invisible again.
type RetryObserver interface {
	SetRetryObserver(func(RetryInfo))
}
