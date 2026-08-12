package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ShellType represents supported shell types
type ShellType string

const (
	ShellBash ShellType = "bash"
	ShellZsh  ShellType = "zsh"
	ShellFish ShellType = "fish"
	ShellPosh ShellType = "powershell"
)

// ShellIntegration handles shell-specific setup
type ShellIntegration struct {
	Shell       ShellType
	ConfigFile  string
	ProfileFile string
	HomeDir     string
}

// DetectShell detects the current shell
func DetectShell() ShellType {
	shell := os.Getenv("SHELL")
	if runtime.GOOS == "windows" {
		// Check for PowerShell
		if os.Getenv("PSModulePath") != "" {
			return ShellPosh
		}
		return ShellPosh // Default to PowerShell on Windows
	}

	switch {
	case strings.Contains(shell, "zsh"):
		return ShellZsh
	case strings.Contains(shell, "fish"):
		return ShellFish
	case strings.Contains(shell, "bash"):
		return ShellBash
	default:
		return ShellBash // Default to bash
	}
}

// NewShellIntegration creates a shell integration for the detected shell
func NewShellIntegration() (*ShellIntegration, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not detect home directory: %w", err)
	}

	shell := DetectShell()
	si := &ShellIntegration{
		Shell:   shell,
		HomeDir: home,
	}

	// Set config files based on shell
	switch shell {
	case ShellZsh:
		zdotdir := os.Getenv("ZDOTDIR")
		if zdotdir == "" {
			zdotdir = home
		}
		si.ConfigFile = filepath.Join(zdotdir, ".zshrc")
		si.ProfileFile = filepath.Join(zdotdir, ".zprofile")
	case ShellBash:
		si.ConfigFile = filepath.Join(home, ".bashrc")
		si.ProfileFile = filepath.Join(home, ".bash_profile")
	case ShellFish:
		si.ConfigFile = filepath.Join(home, ".config", "fish", "config.fish")
	case ShellPosh:
		// PowerShell profile location
		si.ConfigFile = filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	}

	return si, nil
}

// GetPathEntry returns the PATH addition command for this shell
func (si *ShellIntegration) GetPathEntry(binDir string) string {
	switch si.Shell {
	case ShellFish:
		return fmt.Sprintf("set -gx PATH $PATH %s", binDir)
	case ShellPosh:
		return fmt.Sprintf("$env:PATH += \";%s\"", binDir)
	default:
		return fmt.Sprintf("export PATH=\"$PATH:%s\"", binDir)
	}
}

// GetAliasEntry returns the alias command for this shell
func (si *ShellIntegration) GetAliasEntry(alias, command string) string {
	switch si.Shell {
	case ShellFish:
		return fmt.Sprintf("alias %s '%s'", alias, command)
	case ShellPosh:
		return fmt.Sprintf("Set-Alias -Name %s -Value %s", alias, command)
	default:
		return fmt.Sprintf("alias %s='%s'", alias, command)
	}
}

// AddToPath adds the bin directory to PATH in shell config
func (si *ShellIntegration) AddToPath(binDir string) error {
	// Read existing config
	content, err := os.ReadFile(si.ConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already present
	pathEntry := si.GetPathEntry(binDir)
	if strings.Contains(string(content), binDir) {
		return nil // Already configured
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(si.ConfigFile), 0755); err != nil {
		return err
	}

	// Append to config
	f, err := os.OpenFile(si.ConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	marker := "\n# Automergent PATH\n"
	_, err = f.WriteString(marker + pathEntry + "\n")
	return err
}

// InstallCompletions installs shell completions
func (si *ShellIntegration) InstallCompletions(binDir string) error {
	automergent := filepath.Join(binDir, "automergent")
	if runtime.GOOS == "windows" {
		automergent += ".exe"
	}

	// Verify binary exists
	if _, err := os.Stat(automergent); err != nil {
		return nil // Binary not installed yet, skip completions
	}

	switch si.Shell {
	case ShellBash:
		return si.installBashCompletions(automergent)
	case ShellZsh:
		return si.installZshCompletions(automergent)
	case ShellFish:
		return si.installFishCompletions(automergent)
	case ShellPosh:
		return si.installPowerShellCompletions()
	}
	return nil
}

// installBashCompletions installs bash completions
func (si *ShellIntegration) installBashCompletions(automergent string) error {
	completionDir := filepath.Join(si.HomeDir, ".local", "share", "bash-completion", "completions")
	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return err
	}

	completionScript := `# Automergent bash completion
_automergent_completion() {
    local cur prev words cword
    _init_completion || return

    case "${prev}" in
        -c|--config)
            _filedir
            return 0
            ;;
        -p|--project)
            _filedir -d
            return 0
            ;;
    esac

    if [[ ${cur} == -* ]]; then
        COMPREPLY=($(compgen -W "--help --version --config --project --verbose --quiet --json" -- ${cur}))
        return 0
    fi

    COMPREPLY=($(compgen -W "chat ask code review test help version update config" -- ${cur}))
}
complete -F _automergent_completion automergent
complete -F _automergent_completion owe
`
	return os.WriteFile(filepath.Join(completionDir, "automergent"), []byte(completionScript), 0644)
}

// installZshCompletions installs zsh completions
func (si *ShellIntegration) installZshCompletions(automergent string) error {
	completionDir := filepath.Join(si.HomeDir, ".local", "share", "zsh", "site-functions")
	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return err
	}

	completionScript := `#compdef automergent owe
# Automergent zsh completion

_automergent() {
    local -a commands
    commands=(
        'chat:Start an interactive chat session'
        'ask:Ask a one-off question'
        'code:Generate or modify code'
        'review:Review code changes'
        'test:Run tests with AI assistance'
        'help:Show help information'
        'version:Show version information'
        'update:Update Automergent to latest version'
        'config:Manage configuration'
    )

    local -a options
    options=(
        '(-h --help)'{-h,--help}'[Show help]'
        '(-v --version)'{-v,--version}'[Show version]'
        '(-c --config)'{-c,--config}'[Config file]:config file:_files'
        '(-p --project)'{-p,--project}'[Project directory]:project dir:_directories'
        '--verbose[Enable verbose output]'
        '--quiet[Suppress non-essential output]'
        '--json[Output in JSON format]'
    )

    _arguments -C \
        $options \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe -t commands 'automergent command' commands
            ;;
        args)
            case $words[1] in
                config)
                    _arguments \
                        '1:subcommand:(get set list reset)'
                    ;;
            esac
            ;;
    esac
}

_automergent "$@"
`
	return os.WriteFile(filepath.Join(completionDir, "_automergent"), []byte(completionScript), 0644)
}

// installFishCompletions installs fish completions
func (si *ShellIntegration) installFishCompletions(automergent string) error {
	completionDir := filepath.Join(si.HomeDir, ".config", "fish", "completions")
	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return err
	}

	completionScript := `# Automergent fish completion

# Disable file completion by default
complete -c automergent -f
complete -c owe -f

# Commands
complete -c automergent -n __fish_use_subcommand -a chat -d 'Start an interactive chat session'
complete -c automergent -n __fish_use_subcommand -a ask -d 'Ask a one-off question'
complete -c automergent -n __fish_use_subcommand -a code -d 'Generate or modify code'
complete -c automergent -n __fish_use_subcommand -a review -d 'Review code changes'
complete -c automergent -n __fish_use_subcommand -a test -d 'Run tests with AI assistance'
complete -c automergent -n __fish_use_subcommand -a help -d 'Show help information'
complete -c automergent -n __fish_use_subcommand -a version -d 'Show version information'
complete -c automergent -n __fish_use_subcommand -a update -d 'Update Automergent to latest version'
complete -c automergent -n __fish_use_subcommand -a config -d 'Manage configuration'

# Alias owe to automergent
complete -c owe -w automergent

# Global options
complete -c automergent -s h -l help -d 'Show help'
complete -c automergent -s v -l version -d 'Show version'
complete -c automergent -s c -l config -r -d 'Config file'
complete -c automergent -s p -l project -r -d 'Project directory'
complete -c automergent -l verbose -d 'Enable verbose output'
complete -c automergent -l quiet -d 'Suppress non-essential output'
complete -c automergent -l json -d 'Output in JSON format'

# Config subcommands
complete -c automergent -n '__fish_seen_subcommand_from config' -a get -d 'Get a config value'
complete -c automergent -n '__fish_seen_subcommand_from config' -a set -d 'Set a config value'
complete -c automergent -n '__fish_seen_subcommand_from config' -a list -d 'List all config values'
complete -c automergent -n '__fish_seen_subcommand_from config' -a reset -d 'Reset config to defaults'
`

	if err := os.WriteFile(filepath.Join(completionDir, "automergent.fish"), []byte(completionScript), 0644); err != nil {
		return err
	}

	// Also create owe.fish as a simple wrapper
	oweScript := `# owe is an alias for automergent
complete -c owe -w automergent
`
	return os.WriteFile(filepath.Join(completionDir, "owe.fish"), []byte(oweScript), 0644)
}

// installPowerShellCompletions installs PowerShell completions
func (si *ShellIntegration) installPowerShellCompletions() error {
	completionScript := `
# Automergent PowerShell completion
$AutomergentCommands = @(
    @{Name='chat'; Description='Start an interactive chat session'},
    @{Name='ask'; Description='Ask a one-off question'},
    @{Name='code'; Description='Generate or modify code'},
    @{Name='review'; Description='Review code changes'},
    @{Name='test'; Description='Run tests with AI assistance'},
    @{Name='help'; Description='Show help information'},
    @{Name='version'; Description='Show version information'},
    @{Name='update'; Description='Update Automergent to latest version'},
    @{Name='config'; Description='Manage configuration'}
)

Register-ArgumentCompleter -CommandName @('automergent', 'owe') -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    
    $AutomergentCommands | Where-Object { $_.Name -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new(
            $_.Name,
            $_.Name,
            'ParameterValue',
            $_.Description
        )
    }
}
`

	// Ensure profile directory exists
	if err := os.MkdirAll(filepath.Dir(si.ConfigFile), 0755); err != nil {
		return err
	}

	// Read existing profile
	content, err := os.ReadFile(si.ConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already present
	if strings.Contains(string(content), "Automergent PowerShell completion") {
		return nil
	}

	// Append to profile
	f, err := os.OpenFile(si.ConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n" + completionScript)
	return err
}

// SetupAlias creates shell aliases
func (si *ShellIntegration) SetupAlias(alias, target string) error {
	// Read existing config
	content, err := os.ReadFile(si.ConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if alias already exists
	aliasEntry := si.GetAliasEntry(alias, target)
	if strings.Contains(string(content), fmt.Sprintf("alias %s", alias)) ||
		strings.Contains(string(content), fmt.Sprintf("Set-Alias -Name %s", alias)) {
		return nil // Already configured
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(si.ConfigFile), 0755); err != nil {
		return err
	}

	// Append to config
	f, err := os.OpenFile(si.ConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	marker := "\n# Automergent Aliases\n"
	_, err = f.WriteString(marker + aliasEntry + "\n")
	return err
}

// SuggestedAliases returns suggested aliases for Automergent
func SuggestedAliases() map[string]string {
	return map[string]string{
		"ow":  "automergent",
		"oai": "automergent ask",
		"och": "automergent chat",
		"ocr": "automergent code review",
	}
}

// GetEnvironmentSetup returns environment variables to set
func (si *ShellIntegration) GetEnvironmentSetup() map[string]string {
	return map[string]string{
		"AUTOMERGENT_HOME": filepath.Join(si.HomeDir, ".automergent"),
	}
}

// SetupEnvironment configures environment variables
func (si *ShellIntegration) SetupEnvironment() error {
	envVars := si.GetEnvironmentSetup()

	// Read existing config
	content, err := os.ReadFile(si.ConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already configured
	if strings.Contains(string(content), "AUTOMERGENT_HOME") {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(si.ConfigFile), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(si.ConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = f.WriteString("\n# Automergent Environment\n")
	for key, value := range envVars {
		var line string
		switch si.Shell {
		case ShellFish:
			line = fmt.Sprintf("set -gx %s '%s'\n", key, value)
		case ShellPosh:
			line = fmt.Sprintf("$env:%s = '%s'\n", key, value)
		default:
			line = fmt.Sprintf("export %s='%s'\n", key, value)
		}
		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}

	return nil
}

// FullSetup performs complete shell integration
func (si *ShellIntegration) FullSetup(binDir string) error {
	// Add to PATH if needed
	if err := si.AddToPath(binDir); err != nil {
		return fmt.Errorf("failed to add to PATH: %w", err)
	}

	// Install completions
	if err := si.InstallCompletions(binDir); err != nil {
		return fmt.Errorf("failed to install completions: %w", err)
	}

	// Setup environment
	if err := si.SetupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	return nil
}
