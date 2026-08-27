package mcp

import (
	"context"
	"encoding/json"
	"time"
)

// ServerStatus represents the health status of an MCP server.
type ServerStatus string

const (
	StatusHealthy      ServerStatus = "healthy"
	StatusUnhealthy    ServerStatus = "unhealthy"
	StatusConnecting   ServerStatus = "connecting"
	StatusDisconnected ServerStatus = "disconnected"
	StatusUnknown      ServerStatus = "unknown"
)

// TransportType specifies how to communicate with the MCP server.
type TransportType string

const (
	TransportHTTP  TransportType = "http"
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
)

// ServerConfig defines configuration for a single MCP server.
type ServerConfig struct {
	Name        string             `json:"name" yaml:"name"`
	URL         string             `json:"url,omitempty" yaml:"url,omitempty"`
	Command     string             `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string           `json:"args,omitempty" yaml:"args,omitempty"`
	Env         map[string]string  `json:"env,omitempty" yaml:"env,omitempty"`
	Transport   TransportType      `json:"transport" yaml:"transport"`
	AuthToken   string             `json:"auth_token,omitempty" yaml:"auth_token,omitempty"`
	Priority    int                `json:"priority" yaml:"priority"`                       // Higher = preferred
	Namespace   string             `json:"namespace,omitempty" yaml:"namespace,omitempty"` // Tool namespace prefix
	Enabled     bool               `json:"enabled" yaml:"enabled"`
	Timeout     time.Duration      `json:"timeout" yaml:"timeout"`
	RetryCount  int                `json:"retry_count" yaml:"retry_count"`
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty" yaml:"health_check,omitempty"`
}

// HealthCheckConfig configures health monitoring for a server.
type HealthCheckConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled"`
	Interval time.Duration `json:"interval" yaml:"interval"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`
}

// ServerInfo contains runtime information about a connected server.
type ServerInfo struct {
	Config       *ServerConfig   `json:"config"`
	Status       ServerStatus    `json:"status"`
	ConnectedAt  *time.Time      `json:"connected_at,omitempty"`
	LastPing     *time.Time      `json:"last_ping,omitempty"`
	LastError    string          `json:"last_error,omitempty"`
	Version      string          `json:"version,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	Tools        []ToolInfo      `json:"tools"`
	Resources    []ResourceInfo  `json:"resources,omitempty"`
	Prompts      []PromptInfo    `json:"prompts,omitempty"`
	Latency      time.Duration   `json:"latency"`
	RequestCount int64           `json:"request_count"`
	ErrorCount   int64           `json:"error_count"`
}

const (
	ProtocolVersion20241105 = "2024-11-05"
	ProtocolVersion20250326 = "2025-03-26"
	ProtocolVersion20250618 = "2025-06-18"
)

// ToolAnnotations describes server-provided hints about a tool's behavior.
type ToolAnnotations struct {
	Title            string `json:"title,omitempty"`
	ReadOnlyHint     bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint  bool   `json:"destructiveHint,omitempty"`
	IdempotentHint   bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint    bool   `json:"openWorldHint,omitempty"`
}

// ToolInfo describes a tool provided by an MCP server.
type ToolInfo struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"inputSchema"`
	Server        string          `json:"server"`        // Source server name
	QualifiedName string          `json:"qualifiedName"` // namespace/name
	Priority      int             `json:"priority"`
	Annotations   *ToolAnnotations `json:"annotations,omitempty"`
}

// ResourceInfo describes a resource provided by an MCP server.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Server      string `json:"server"`
}

// PromptInfo describes a prompt template from an MCP server.
type PromptInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Server      string           `json:"server"`
}

// PromptArgument defines an argument for a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptResult is the result of getting a prompt from a server.
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage is a single message in a prompt result.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content ContentBlock `json:"content"`
}

// ToolCall represents a request to execute a tool.
type ToolCall struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"arguments"`
	Server string         `json:"server,omitempty"` // Specific server, or empty for auto-routing
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
	Server  string         `json:"server"`
	Latency time.Duration  `json:"latency"`
}

// ContentBlock represents a piece of content in a tool result.
type ContentBlock struct {
	Type     string `json:"type"` // "text", "image", "resource"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// OrchestratorConfig configures the multi-server orchestrator.
type OrchestratorConfig struct {
	Servers           []ServerConfig      `json:"servers" yaml:"servers"`
	DefaultTimeout    time.Duration       `json:"default_timeout" yaml:"default_timeout"`
	MaxConcurrent     int                 `json:"max_concurrent" yaml:"max_concurrent"`
	EnableDiscovery   bool                `json:"enable_discovery" yaml:"enable_discovery"`
	DiscoveryInterval time.Duration       `json:"discovery_interval" yaml:"discovery_interval"`
	LoadBalancing     LoadBalanceStrategy `json:"load_balancing" yaml:"load_balancing"`
	FailoverEnabled   bool                `json:"failover_enabled" yaml:"failover_enabled"`
	CacheEnabled      bool                `json:"cache_enabled" yaml:"cache_enabled"`
	CacheTTL          time.Duration       `json:"cache_ttl" yaml:"cache_ttl"`
}

// LoadBalanceStrategy defines how requests are distributed across servers.
type LoadBalanceStrategy string

const (
	LoadBalanceRoundRobin   LoadBalanceStrategy = "round_robin"
	LoadBalanceLeastLatency LoadBalanceStrategy = "least_latency"
	LoadBalancePriority     LoadBalanceStrategy = "priority"
	LoadBalanceRandom       LoadBalanceStrategy = "random"
)

// ConfigChangeEvent represents a configuration change for hot-reload.
type ConfigChangeEvent struct {
	Type      ConfigChangeType `json:"type"`
	Server    string           `json:"server,omitempty"`
	OldConfig *ServerConfig    `json:"old_config,omitempty"`
	NewConfig *ServerConfig    `json:"new_config,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// ConfigChangeType specifies the type of configuration change.
type ConfigChangeType string

const (
	ConfigAdded   ConfigChangeType = "added"
	ConfigUpdated ConfigChangeType = "updated"
	ConfigRemoved ConfigChangeType = "removed"
)

// ServerEvent represents an event from a server (status change, error, etc).
type ServerEvent struct {
	Type      ServerEventType `json:"type"`
	Server    string          `json:"server"`
	Status    ServerStatus    `json:"status,omitempty"`
	Error     string          `json:"error,omitempty"`
	Message   string          `json:"message,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// ServerEventType specifies the type of server event.
type ServerEventType string

const (
	EventConnected    ServerEventType = "connected"
	EventDisconnected ServerEventType = "disconnected"
	EventHealthChange ServerEventType = "health_change"
	EventToolsUpdated ServerEventType = "tools_updated"
	EventError        ServerEventType = "error"
)

// Transport defines the interface for MCP communication transports.
type Transport interface {
	// Connect establishes connection to the server.
	Connect(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// Send sends a request and returns the response.
	Send(ctx context.Context, method string, params any) (json.RawMessage, error)

	// IsConnected returns true if the transport is connected.
	IsConnected() bool
}

// EventHandler is called when server events occur.
type EventHandler func(event ServerEvent)

// ConfigChangeHandler is called when configuration changes are detected.
type ConfigChangeHandler func(event ConfigChangeEvent)
