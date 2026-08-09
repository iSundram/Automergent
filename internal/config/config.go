package config

// Config holds the full application configuration.
type Config struct {
	Provider    string `mapstructure:"provider" yaml:"provider"`
	Model       string `mapstructure:"model" yaml:"model"`
	Mode        string `mapstructure:"mode" yaml:"mode"`
	Theme       string `mapstructure:"theme" yaml:"theme"`
	Keybindings string `mapstructure:"keybindings" yaml:"keybindings"`
	Layout      string `mapstructure:"layout" yaml:"layout"`

	AutoSave           bool   `mapstructure:"autoSave" yaml:"autoSave"`
	CheckpointInterval int    `mapstructure:"checkpointInterval" yaml:"checkpointInterval"`
	SessionDir         string `mapstructure:"sessionDir" yaml:"sessionDir"`
	MaxSessions        int    `mapstructure:"maxSessions" yaml:"maxSessions"`
	MaxSessionAge      string `mapstructure:"maxSessionAge" yaml:"maxSessionAge"`

	MaxContextTokens      int     `mapstructure:"maxContextTokens" yaml:"maxContextTokens"`
	WarnAtContextFraction float64 `mapstructure:"warnAtContextFraction" yaml:"warnAtContextFraction"`
	AutoCompressAt        float64 `mapstructure:"autoCompressAt" yaml:"autoCompressAt"`
	CompressionKeepRecent int     `mapstructure:"compressionKeepRecent" yaml:"compressionKeepRecent"`

	MaxAutoReadFileSize int      `mapstructure:"maxAutoReadFileSize" yaml:"maxAutoReadFileSize"`
	MaxTreeFiles        int      `mapstructure:"maxTreeFiles" yaml:"maxTreeFiles"`
	MaxTreeDepth        int      `mapstructure:"maxTreeDepth" yaml:"maxTreeDepth"`
	ExcludePatterns     []string `mapstructure:"excludePatterns" yaml:"excludePatterns"`

	NoAnimation bool `mapstructure:"noAnimation" yaml:"noAnimation"`
	NoColor     bool `mapstructure:"noColor" yaml:"noColor"`
	NoTUI       bool `mapstructure:"noTui" yaml:"noTui"`
	Quiet       bool `mapstructure:"quiet" yaml:"quiet"`
	Verbose     bool `mapstructure:"verbose" yaml:"verbose"`

	Security  SecurityConfig            `mapstructure:"security" yaml:"security"`
	Tools     map[string]ToolConfig     `mapstructure:"tools" yaml:"tools"`
	LSP       LSPConfig                 `mapstructure:"lsp" yaml:"lsp"`
	MCP       MCPConfig                 `mapstructure:"mcp" yaml:"mcp"`
	Log       LogConfig                 `mapstructure:"log" yaml:"log"`
	Providers map[string]ProviderConfig `mapstructure:"providers" yaml:"providers"`

	ContextFiles []string `mapstructure:"contextFiles" yaml:"contextFiles,omitempty"`
	SkillsDir    string   `mapstructure:"skillsDir" yaml:"skillsDir,omitempty"`

	ZeroDataRetention bool `mapstructure:"zeroDataRetention" yaml:"zeroDataRetention"`
	Telemetry         bool `mapstructure:"telemetry" yaml:"telemetry"`
	NoUpdateCheck     bool `mapstructure:"noUpdateCheck" yaml:"noUpdateCheck"`

	ProviderFallback []FallbackProvider `mapstructure:"providerFallback" yaml:"providerFallback,omitempty"`

	Notifications NotificationConfig `mapstructure:"notifications" yaml:"notifications"`
}

// CLIFlags holds flags parsed from the command line.
type CLIFlags struct {
	Provider     string
	Model        string
	Mode         string
	Prompt       string
	Output       string
	NoTUI        bool
	Stdin        bool
	Theme        string
	Keybindings  string
	ConfigFile   string
	NoColor      bool
	Session      string
	NewSession   bool
	SessionDir   string
	ContextFiles []string
	Images       []string
	Files        []string
	NoContext    bool
	NoSandbox    bool
	Sandbox      string
	Quiet        bool
	Verbose      bool
	NoAnimation  bool
	Layout       string
	BaseURL      string
	APIKey       string
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	Sandbox                string   `mapstructure:"sandbox" yaml:"sandbox"`
	BlockedWritePaths      []string `mapstructure:"blockedWritePaths" yaml:"blockedWritePaths,omitempty"`
	AllowedWritePaths      []string `mapstructure:"allowedWritePaths" yaml:"allowedWritePaths,omitempty"`
	StripEnvVarPatterns    []string `mapstructure:"stripEnvVarPatterns" yaml:"stripEnvVarPatterns,omitempty"`
	RequireGitForAutoModes bool     `mapstructure:"requireGitForAutoModes" yaml:"requireGitForAutoModes"`
}

// ToolConfig holds per-tool settings.
type ToolConfig struct {
	Enabled              bool   `mapstructure:"enabled" yaml:"enabled"`
	ConfirmationRequired string `mapstructure:"confirmationRequired" yaml:"confirmationRequired"`
	Timeout              string `mapstructure:"timeout" yaml:"timeout,omitempty"`
	MaxOutputBytes       int    `mapstructure:"maxOutputBytes" yaml:"maxOutputBytes,omitempty"`
}

// LSPConfig holds LSP server settings.
type LSPConfig struct {
	Enabled        bool              `mapstructure:"enabled" yaml:"enabled"`
	Servers        map[string]string `mapstructure:"servers" yaml:"servers,omitempty"`
	StartupTimeout string            `mapstructure:"startupTimeout" yaml:"startupTimeout,omitempty"`
	RequestTimeout string            `mapstructure:"requestTimeout" yaml:"requestTimeout,omitempty"`
}

// MCPServer holds configuration for a single MCP server.
type MCPServer struct {
	Type    string            `mapstructure:"type" yaml:"type"`
	Command []string          `mapstructure:"command" yaml:"command,omitempty"`
	URL     string            `mapstructure:"url" yaml:"url,omitempty"`
	Env     map[string]string `mapstructure:"env" yaml:"env,omitempty"`
	Auth    string            `mapstructure:"auth" yaml:"auth,omitempty"`
	Timeout string            `mapstructure:"timeout" yaml:"timeout,omitempty"`
}

// MCPConfig holds MCP server settings.
type MCPConfig struct {
	Servers map[string]MCPServer `mapstructure:"servers" yaml:"servers,omitempty"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level      string `mapstructure:"level" yaml:"level"`
	File       string `mapstructure:"file" yaml:"file"`
	MaxSize    string `mapstructure:"maxSize" yaml:"maxSize,omitempty"`
	MaxBackups int    `mapstructure:"maxBackups" yaml:"maxBackups,omitempty"`
}

// ProviderConfig holds per-provider settings.
type ProviderConfig struct {
	APIKey       string                 `mapstructure:"apiKey" yaml:"apiKey,omitempty"`
	BaseURL      string                 `mapstructure:"baseUrl" yaml:"baseUrl,omitempty"`
	DefaultModel string                 `mapstructure:"defaultModel" yaml:"defaultModel,omitempty"`
	OrgID        string                 `mapstructure:"orgId" yaml:"orgId,omitempty"`
	Project      string                 `mapstructure:"project" yaml:"project,omitempty"`
	Location     string                 `mapstructure:"location" yaml:"location,omitempty"`
	Models       map[string]ModelConfig `mapstructure:"models" yaml:"models,omitempty"`
}

// ModelConfig holds model-scoped provider settings.
type ModelConfig struct {
	APIKey  string `mapstructure:"apiKey" yaml:"apiKey,omitempty"`
	BaseURL string `mapstructure:"baseUrl" yaml:"baseUrl,omitempty"`
}

// FallbackProvider defines a fallback AI provider/model.
type FallbackProvider struct {
	Provider string `mapstructure:"provider" yaml:"provider"`
	Model    string `mapstructure:"model" yaml:"model"`
}

// NotificationConfig holds notification settings.
type NotificationConfig struct {
	Desktop        bool `mapstructure:"desktop" yaml:"desktop"`
	Bell           bool `mapstructure:"bell" yaml:"bell"`
	ContextWarning bool `mapstructure:"contextWarning" yaml:"contextWarning"`
}
