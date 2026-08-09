package config

// Config holds the full application configuration.
type Config struct {
	Provider    string
	Model       string
	Mode        string
	Theme       string
	Keybindings string
	Layout      string

	AutoSave           bool
	CheckpointInterval int
	SessionDir         string
	MaxSessions        int

	MaxContextTokens      int
	WarnAtContextFraction float64
	AutoCompressAt        float64
	CompressionKeepRecent int

	MaxAutoReadFileSize int
	MaxTreeFiles        int
	MaxTreeDepth        int
	ExcludePatterns     []string

	NoAnimation bool
	NoColor     bool
	NoTUI       bool
	Quiet       bool
	Verbose     bool

	Security  SecurityConfig
	Tools     map[string]ToolConfig
	LSP       LSPConfig
	MCP       MCPConfig
	Log       LogConfig
	Providers map[string]ProviderConfig

	ContextFiles []string
	SkillsDir    string

	ZeroDataRetention bool
	Telemetry         bool
	NoUpdateCheck     bool

	ProviderFallback []FallbackProvider

	Notifications NotificationConfig
}

// CLIFlags holds flags parsed from the command line.
type CLIFlags struct {
	Provider     string
	Model        string
	Mode         string
	Prompt       string
	Output       string
	NoTUI        bool
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
	Sandbox                string
	BlockedWritePaths      []string
	AllowedWritePaths      []string
	StripEnvVarPatterns    []string
	RequireGitForAutoModes bool
}

// ToolConfig holds per-tool settings.
type ToolConfig struct {
	Enabled              bool
	ConfirmationRequired string
	Timeout              string
	MaxOutputBytes       int
}

// LSPConfig holds LSP server settings.
type LSPConfig struct {
	Enabled        bool
	Servers        map[string]string
	StartupTimeout string
	RequestTimeout string
}

// MCPServer holds configuration for a single MCP server.
type MCPServer struct {
	Type    string
	Command []string
	URL     string
	Env     map[string]string
	Auth    string
	Timeout string
}

// MCPConfig holds MCP server settings.
type MCPConfig struct {
	Servers map[string]MCPServer
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level      string
	File       string
	MaxSize    string
	MaxBackups int
}

// ProviderConfig holds per-provider settings.
type ProviderConfig struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	OrgID        string
	Project      string
	Location     string
}

// FallbackProvider defines a fallback AI provider/model.
type FallbackProvider struct {
	Provider string
	Model    string
}

// NotificationConfig holds notification settings.
type NotificationConfig struct {
	Desktop        bool
	Bell           bool
	ContextWarning bool
}
