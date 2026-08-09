package ai

import "context"

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
}

// CompletionResponse is returned by a provider.
type CompletionResponse interface {
	Stream() <-chan Chunk
	ToolCalls() []ToolCall
	StopReason() StopReason
	Usage() Usage
}

// Message is a single turn in the conversation.
type Message struct {
	Role    Role
	Content []ContentPart
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
}

// ProviderConfig holds provider-level credentials and defaults.
type ProviderConfig struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	OrgID        string
	Project      string
	Location     string
}

// Event is a generic event emitted during a completion.
type Event struct {
	Type    string
	Payload any
}
