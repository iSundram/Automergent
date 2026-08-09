package main

import (
	"context"
	"fmt"
	"os"

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
	toolsShell "github.com/iSundram/Automergent/internal/tools/shell"
	toolsWeb "github.com/iSundram/Automergent/internal/tools/web"
	"github.com/iSundram/Automergent/internal/tui"
	"github.com/iSundram/Automergent/internal/version"
)

var flags config.CLIFlags
var cfgFile string

func main() {
	if err := rootCmd.Execute(); err != nil {
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
	f.StringVarP(&flags.Provider, "provider", "p", "", "AI provider (google)")
	f.StringVarP(&flags.Model, "model", "m", "", "Model name")
	f.StringVar(&flags.Mode, "mode", "", "Approval mode: suggest, auto-edit, full-auto, plan")
	f.StringVar(&flags.Theme, "theme", "", "Color theme: catppuccin, dracula")
	f.StringVar(&flags.Keybindings, "keybindings", "", "Key bindings: default, vim, emacs")
	f.StringVarP(&flags.Output, "output", "o", "", "Output file for non-TUI mode")
	f.BoolVar(&flags.NoTUI, "no-tui", false, "Disable TUI, write output to stdout")
	f.BoolVar(&flags.NoColor, "no-color", false, "Disable color output")
	f.BoolVar(&flags.Quiet, "quiet", false, "Suppress non-essential output")
	f.BoolVar(&flags.Verbose, "verbose", false, "Enable verbose logging")
	f.BoolVar(&flags.NoAnimation, "no-animation", false, "Disable animations")
	f.StringVar(&flags.Session, "session", "", "Resume a specific session ID")
	f.BoolVar(&flags.NewSession, "new-session", false, "Start a new session")
	f.StringVar(&flags.SessionDir, "session-dir", "", "Session storage directory")
	f.StringSliceVar(&flags.ContextFiles, "context", nil, "Extra context files to include")
	f.StringSliceVar(&flags.Files, "files", nil, "Files to include in context")
	f.BoolVar(&flags.NoSandbox, "no-sandbox", false, "Disable OS sandboxing")
	f.StringVar(&flags.Sandbox, "sandbox", "", "Sandbox type")
	f.StringVar(&flags.APIKey, "api-key", "", "API key (overrides env var)")
	f.StringVar(&flags.BaseURL, "base-url", "", "Custom API base URL")
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(home + "/.automergent")
		viper.AddConfigPath(".")
	}
	viper.SetEnvPrefix("AUTOMERGENT")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func run(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	cfg.ApplyFlags(&flags)

	// Resolve API keys from environment if not set
	resolveAPIKeysFromEnv(cfg)

	// Build prompt from trailing args
	prompt := ""
	if len(args) > 0 {
		prompt = args[0]
		for _, a := range args[1:] {
			prompt += " " + a
		}
	}

	// Setup session
	storage, err := session.NewStorage(cfg.SessionDir)
	if err != nil {
		return fmt.Errorf("session storage: %w", err)
	}

	var sess *session.Session
	if flags.Session != "" {
		sess, err = storage.Load(flags.Session)
		if err != nil {
			return fmt.Errorf("load session %s: %w", flags.Session, err)
		}
	} else {
		sess = session.New()
		if prompt != "" {
			sess.Title = prompt
		} else {
			sess.Title = "New conversation"
		}
	}

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
	reg.Register(toolsInteraction.NewAskUserTool(nil))
	reg.Register(&toolsInteraction.NotifyTool{})

	// Get AI provider
	provider, err := resolveProvider(cfg)
	if err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	// Build agent
	ag := agent.New(cfg, provider, sess, reg)

	// Save session on exit
	defer func() {
		_ = storage.Save(sess)
	}()

	if cfg.NoTUI {
		return runHeadless(cmd.Context(), ag, sess, prompt)
	}
	return tui.Run(cfg, ag, sess, prompt)
}

func runHeadless(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string) error {
	if prompt == "" {
		return fmt.Errorf("prompt required in no-tui mode")
	}
	// Drain events in background
	go func() {
		for ev := range ag.Events() {
			if ev.Type == agent.EventToken {
				if tok, ok := ev.Payload.(string); ok {
					fmt.Print(tok)
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
		"google": "GOOGLE_API_KEY",
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.ProviderConfig{}
	}
	for provider, envVar := range envMap {
		if val := os.Getenv(envVar); val != "" {
			pc := cfg.Providers[provider]
			if pc.APIKey == "" {
				pc.APIKey = val
				cfg.Providers[provider] = pc
			}
		}
	}
}
