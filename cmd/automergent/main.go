package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/iSundram/Automergent/internal/agent"
	aiPkg "github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	toolsFS "github.com/iSundram/Automergent/internal/tools/filesystem"
	toolsGit "github.com/iSundram/Automergent/internal/tools/git"
	toolsInteraction "github.com/iSundram/Automergent/internal/tools/interaction"
	toolsLSP "github.com/iSundram/Automergent/internal/tools/lsp"
	toolsShell "github.com/iSundram/Automergent/internal/tools/shell"
	toolsWeb "github.com/iSundram/Automergent/internal/tools/web"
	"github.com/iSundram/Automergent/internal/tui"
	"github.com/iSundram/Automergent/internal/version"
)

var flags config.CLIFlags
var cfgFile string

func main() {
	// Graceful shutdown: cancel context on SIGINT / SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "automergent [prompt]",
	Short: "Automergent – AI coding agent for the terminal",
	Long: `Automergent is an AI-powered coding agent that helps you write, edit,
and understand code directly in your terminal.`,
	Version: version.Version,
	Args:    cobra.ArbitraryArgs,
	RunE:    run,
}

func init() {
	cobra.OnInitialize(initConfig)

	f := rootCmd.Flags()
	f.StringVar(&cfgFile, "config", "", "config file (default ~/.automergent/config.yaml)")
	f.StringVarP(&flags.Prompt, "prompt", "p", "", "Non-interactive: run this prompt and exit")
	f.StringVar(&flags.Provider, "provider", "", "AI provider (google)")
	f.StringVarP(&flags.Model, "model", "m", "", "Model name")
	f.StringVar(&flags.Mode, "mode", "", "Approval mode: edit, plan")
	f.StringVar(&flags.Theme, "theme", "", "Color theme: catppuccin, dracula")
	f.StringVar(&flags.Keybindings, "keybindings", "", "Key bindings: default, vim, emacs")
	f.StringVarP(&flags.Output, "output", "o", "text", "Output format: text | json | stream-json")
	f.BoolVar(&flags.NoTUI, "no-tui", false, "Disable TUI, write output to stdout")
	f.BoolVar(&flags.Stdin, "stdin", false, "Read prompt from stdin")
	f.BoolVar(&flags.NoColor, "no-color", false, "Disable color output")
	f.BoolVarP(&flags.Quiet, "quiet", "q", false, "Suppress non-essential output")
	f.BoolVar(&flags.Verbose, "verbose", false, "Enable verbose logging")
	f.BoolVar(&flags.NoAnimation, "no-animation", false, "Disable animations")
	f.StringVarP(&flags.Session, "session", "s", "", "Resume a specific session ID or name")
	f.BoolVar(&flags.NewSession, "new-session", false, "Start a new session")
	f.StringVar(&flags.SessionDir, "session-dir", "", "Session storage directory")
	f.StringSliceVarP(&flags.ContextFiles, "context", "c", nil, "Extra context files to include")
	f.StringSliceVarP(&flags.Files, "file", "f", nil, "Files to include in context")
	f.BoolVar(&flags.NoSandbox, "no-sandbox", false, "Disable OS sandboxing")
	f.StringVar(&flags.Sandbox, "sandbox", "", "Sandbox type: auto | macos | docker | namespaces | off")
	f.StringVar(&flags.APIKey, "api-key", "", "API key (overrides env var)")
	f.StringVar(&flags.BaseURL, "base-url", "", "Custom API base URL")
	f.BoolVar(&flags.NoContext, "no-context", false, "Skip loading AUTOMERGENT.md files")
	f.StringVar(&flags.Layout, "layout", "", "TUI layout: auto | split | single")
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(home + "/.automergent")
	}
	viper.SetEnvPrefix("AUTOMERGENT")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func run(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	if err := decodeConfigFromViper(cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	cfg.ConfigFile = viper.ConfigFileUsed()
	cfg.ApplyFlags(&flags)
	applyProjectDefaults(cfg, cmd)

	// Save config if critical settings were changed via flags to persist as last used.
	if flags.Provider != "" || flags.Model != "" || flags.APIKey != "" || flags.BaseURL != "" {
		_ = cfg.Save()
	}

	// Resolve API keys from environment if not set
	resolveAPIKeysFromEnv(cfg)

	// Build prompt from --prompt flag, trailing args, or --stdin
	prompt := flags.Prompt
	if prompt == "" && len(args) > 0 {
		prompt = strings.Join(args, " ")
	}
	if prompt == "" && flags.Stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		prompt = strings.TrimSpace(string(data))
	}

	// When a prompt is supplied via flag/stdin, default to headless (no-tui)
	if prompt != "" && !cmd.Flags().Changed("no-tui") {
		cfg.NoTUI = true
	}

	// Setup session
	storage, err := session.NewStorage(cfg.SessionDir)
	if err != nil {
		return fmt.Errorf("session storage: %w", err)
	}

	// Prune old sessions in the background to keep disk usage bounded.
	go func() {
		var maxAge time.Duration
		if cfg.MaxSessionAge != "" {
			maxAge, _ = time.ParseDuration(cfg.MaxSessionAge)
		}
		_ = storage.Prune(cfg.MaxSessions, maxAge)
	}()

	var sess *session.Session
	if flags.Session != "" {
		sess, err = storage.Load(flags.Session)
		if err != nil {
			return fmt.Errorf("load session %s: %w", flags.Session, err)
		}
		// Restore session-specific provider/model if not overridden by flags.
		if flags.Provider == "" && sess.Provider != "" {
			cfg.Provider = sess.Provider
		}
		if flags.Model == "" && sess.Model != "" {
			cfg.Model = sess.Model
		}
		// Save resumed session's settings as new default
		_ = cfg.Save()
	} else {
		sess = session.New()
		if prompt != "" {
			sess.Title = prompt
		} else {
			sess.Title = "New conversation"
		}
	}

	// Ensure session reflects the current provider/model.
	sess.Provider = cfg.Provider
	sess.Model = cfg.Model

	// Build tool registry
	reg := tools.NewRegistry()
	reg.Register(&toolsFS.ReadFileTool{})
	reg.Register(&toolsFS.WriteFileTool{})
	reg.Register(&toolsFS.PatchFileTool{})
	reg.Register(&toolsFS.ListDirectoryTool{})
	reg.Register(&toolsFS.GrepTool{})
	reg.Register(toolsShell.NewRunnerTool(0))
	reg.Register(&toolsGit.StatusTool{})
	reg.Register(&toolsGit.DiffTool{})
	reg.Register(&toolsGit.LogTool{})
	reg.Register(toolsWeb.NewFetchTool())
	reg.Register(toolsWeb.NewSearchTool())
	reg.Register(&toolsLSP.DiagnosticsTool{})
	reg.Register(toolsInteraction.NewAskUserTool(func(question string) (string, error) {
		fmt.Fprintf(os.Stdout, "\n[ask_user] %s\n> ", question)
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(answer), nil
	}))
	reg.Register(&toolsInteraction.NotifyTool{})

	// Get AI provider
	provider, err := resolveProvider(cfg)
	if err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	// Build agent
	ag := agent.New(cfg, provider, sess, reg)
	ag.SetSessionPersist(func() { _ = storage.Save(sess) })

	// Save session on exit
	defer func() {
		_ = storage.Save(sess)
	}()

	if cfg.NoTUI {
		return runHeadless(cmd.Context(), ag, sess, prompt)
	}
	return tui.Run(cfg, ag, sess, storage, prompt)
}

func applyProjectDefaults(cfg *config.Config, cmd *cobra.Command) {
	if cmd.Flags().Changed("session-dir") {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	cfg.SessionDir = filepath.Join(home, ".automergent", "sessions")
	_ = os.MkdirAll(filepath.Join(home, ".automergent"), 0o700)
}

func runHeadless(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string) error {
	if prompt == "" {
		return fmt.Errorf("prompt required in no-tui mode")
	}
	// Forward events to stdout
	go func() {
		for ev := range ag.Events() {
			switch ev.Type {
			case agent.EventToken:
				if tok, ok := ev.Payload.(string); ok {
					fmt.Print(tok)
				}
			case agent.EventToolCall:
				if te, ok := ev.Payload.(agent.ToolCallEvent); ok {
					fmt.Fprintf(os.Stderr, "\n[tool:start] %s id=%s\n", te.Name, te.ID)
				} else if tc, ok := ev.Payload.(aiPkg.ToolCall); ok {
					fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", tc.Name)
				}
			case agent.EventToolDone:
				if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
					state := "ok"
					if td.Result.IsError {
						state = "error"
					}
					fmt.Fprintf(os.Stderr, "\n[tool:%s] %s id=%s dur=%s\n", state, td.Name, td.ID, td.Duration.Round(time.Millisecond))
				}
			case agent.EventDone:
				fmt.Println()
			case agent.EventError:
				if err, ok := ev.Payload.(error); ok {
					fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				}
			}
		}
	}()
	return ag.Run(ctx, prompt)
}

func resolveProvider(cfg *config.Config) (aiPkg.Provider, error) {
	pc := cfg.Providers[cfg.Provider]
	aiCfg := aiPkg.ProviderConfig{
		APIKey:       pc.APIKey,
		BaseURL:      pc.BaseURL,
		DefaultModel: cfg.Model,
		OrgID:        pc.OrgID,
	}

	switch cfg.Provider {
	case "google":
		return googleProvider.New(aiCfg), nil
	default:
		// Check global registry
		if p, ok := aiPkg.Get(cfg.Provider); ok {
			return p, nil
		}
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

func resolveAPIKeysFromEnv(cfg *config.Config) {
	envMap := map[string]string{
		"google": "GEMINI_API_KEY",
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.ProviderConfig{}
	}
	for provider, envVar := range envMap {
		if val := os.Getenv(envVar); val != "" {
			pc, exists := cfg.Providers[provider]
			// Only set from env if not already present in config (manual set)
			if !exists || pc.APIKey == "" {
				pc.APIKey = val
				cfg.Providers[provider] = pc
			}
		}
	}
}

func decodeConfigFromViper(cfg *config.Config) error {
	// Unmarshal directly into the config struct using viper's built-in support.
	// This handles mapstructure tags correctly.
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("viper unmarshal: %w", err)
	}
	return nil
}
