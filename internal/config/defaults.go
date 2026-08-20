package config

import (
	"os"
	"path/filepath"
)

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	sessionDir := filepath.Join(home, ".automergent", "sessions")
	skillsDir := filepath.Join(home, ".automergent", "skills")

	return &Config{
		Provider:    "google",
		Model:       "gemini-3.6-flash",
		Mode:        "edit",
		Output:      "text",
		Theme:       "modern",
		Keybindings: "default",
		Layout:      "default",

		AutoSave:           true,
		CheckpointInterval: 5,
		SessionDir:         sessionDir,
		MaxSessions:        100,

		MaxContextTokens:      128000,
		WarnAtContextFraction: 0.8,
		AutoCompressAt:        0.9,
		CompressionKeepRecent: 10,

		MaxAutoReadFileSize: 512 * 1024,
		MaxTreeFiles:        1000,
		MaxTreeDepth:        10,
		ExcludePatterns: []string{
			".git", "node_modules", ".venv", "__pycache__",
			"*.pyc", "*.o", "*.a", "vendor",
		},

		NoAnimation:          false,
		NoColor:              false,
		NoTUI:                false,
		Quiet:                false,
		Verbose:              false,
ReasoningPreAnalysis: false,
	PromptSystemEnabled:  true,
	Debug:                DefaultDebugConfig(),

	Security: SecurityConfig{
			Sandbox:                "auto",
			RequireGitForAutoModes: true,
			BlockedWritePaths: []string{
				".git/**",
				"~/.ssh/**",
				"~/.gnupg/**",
				"~/.aws/**",
				"~/.kube/**",
			},
			StripEnvVarPatterns: []string{
				"*_SECRET", "*_PASSWORD", "*_TOKEN", "*_KEY",
				"AWS_*", "GEMINI_*",
				"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
				"http_proxy", "https_proxy", "all_proxy", "no_proxy",
			},
		},

		Tools: map[string]ToolConfig{
			"shell": {
				Enabled:              true,
				ConfirmationRequired: "always",
				Timeout:              "30s",
				MaxOutputBytes:       1024 * 1024,
			},
			"filesystem": {
				Enabled:              true,
				ConfirmationRequired: "write",
				MaxOutputBytes:       512 * 1024,
			},
			"git": {
				Enabled:              true,
				ConfirmationRequired: "never",
			},
			"web": {
				Enabled:              true,
				ConfirmationRequired: "never",
				Timeout:              "15s",
			},
		},

		LSP: LSPConfig{
			Enabled:        true,
			Servers:        map[string]string{},
			StartupTimeout: "10s",
			RequestTimeout: "5s",
		},

		MCP: MCPConfig{
			Servers: map[string]MCPServer{},
		},

		Log: LogConfig{
			Level:      "warn",
			File:       filepath.Join(home, ".automergent", "automergent.log"),
			MaxSize:    "50MB",
			MaxBackups: 3,
		},

		Providers: map[string]ProviderConfig{},

		SkillsDir: skillsDir,

		ZeroDataRetention: false,
		Telemetry:         true,
		NoUpdateCheck:     false,

		Notifications: NotificationConfig{
			Desktop:        false,
			Bell:           true,
			ContextWarning: true,
		},
		Coordinator: CoordinatorConfig{
			Enabled:          false, // Opt-in: set to true to enable multi-agent coordination
			WorkersPerRole:   map[string]int{"researcher": 2, "coder": 2, "reviewer": 1, "tester": 1, "documenter": 1},
			DefaultTimeout:   "5m",
			MaxRetries:       3,
			QualityThreshold: 0.7,
			ConsensusThreshold: 2,
			ResourceLimits: CoordinatorResourceLimits{
				MaxTokensPerTask:   100000,
				MaxConcurrentTasks: 5,
				MaxMemoryMB:        512,
				RateLimitPerMinute: 60,
			},
		},
		Diagnostics: DiagnosticsConfig{
			Enabled:          true,
			ShowInRead:       true,
			BlockOnError:     true,
			BlockOnWarning:   false,
			MaxFileSizeBytes: 1 << 20, // 1 MB
			CacheDurationSec: 30,
		},
		Cache: CacheConfig{
			Prompt: PromptCacheConfig{
				Enabled: true,
			},
		},
		Git: GitConfig{
			CoAuthor:        "ask", // "always", "never", "ask"
			CoAuthorTrailer: "Co-authored-by: Automergent <automergent-bot@users.noreply.github.com>",
		},
	}
}

// ApplyFlags merges CLI flags into the config.
func (c *Config) ApplyFlags(f *CLIFlags) {
	if f.Provider != "" {
		c.Provider = f.Provider
	}
	if f.Model != "" {
		c.Model = f.Model
	}
	if f.Mode != "" {
		c.Mode = f.Mode
	}
	if f.Output != "" {
		c.Output = f.Output
	}
	if f.Theme != "" {
		c.Theme = f.Theme
	}
	if f.Keybindings != "" {
		c.Keybindings = f.Keybindings
	}
	if f.Layout != "" {
		c.Layout = f.Layout
	}
	if f.SessionDir != "" {
		c.SessionDir = f.SessionDir
	}
	if f.NoTUI {
		c.NoTUI = true
	}
	if f.NoColor {
		c.NoColor = true
	}
	if f.Quiet {
		c.Quiet = true
	}
	if f.Verbose {
		c.Verbose = true
	}
	if f.NoAnimation {
		c.NoAnimation = true
	}
	if f.NoSandbox {
		c.Security.Sandbox = "off"
	}
	if f.Sandbox != "" {
		c.Security.Sandbox = f.Sandbox
	}
	if len(f.ContextFiles) > 0 {
		c.ContextFiles = append(c.ContextFiles, f.ContextFiles...)
	}
	if f.APIKey != "" {
		if c.Providers == nil {
			c.Providers = map[string]ProviderConfig{}
		}
		pc := c.Providers[c.Provider]
		pc.APIKey = f.APIKey
		c.Providers[c.Provider] = pc
	}
	if f.BaseURL != "" {
		if c.Providers == nil {
			c.Providers = map[string]ProviderConfig{}
		}
		pc := c.Providers[c.Provider]
		pc.BaseURL = f.BaseURL
		c.Providers[c.Provider] = pc
	}
}
