package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ZeroConfigResult holds the results of zero-config installation
type ZeroConfigResult struct {
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	InstallDir      string            `json:"install_dir"`
	Shell           ShellType         `json:"shell"`
	PathConfigured  bool              `json:"path_configured"`
	Completions     bool              `json:"completions_installed"`
	Aliases         map[string]string `json:"aliases"`
	Migrations      []MigrationSource `json:"migrations_available"`
	ExistingInstall bool              `json:"existing_install"`
	RequiresSudo    bool              `json:"requires_sudo"`
	Warnings        []string          `json:"warnings,omitempty"`
}

// ZeroConfigOptions allows customization of zero-config installation
type ZeroConfigOptions struct {
	ForceInstallDir string // Override automatic directory selection
	SkipPath        bool   // Don't modify PATH
	SkipCompletions bool   // Don't install completions
	SkipAliases     bool   // Don't setup aliases
	SkipMigration   bool   // Don't attempt migration
	Quiet           bool   // Suppress output
	Interactive     bool   // Allow interactive prompts
}

// DefaultZeroConfigOptions returns sensible defaults
func DefaultZeroConfigOptions() *ZeroConfigOptions {
	return &ZeroConfigOptions{
		Interactive: false,
	}
}

// DetectOptimalInstallLocation finds the best installation directory
func DetectOptimalInstallLocation() (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("could not detect home directory: %w", err)
	}

	// Priority order for installation directories
	locations := []struct {
		path     string
		inPath   bool
		writable bool
		sudo     bool
	}{
		// User-local directories (preferred - no sudo needed)
		{filepath.Join(home, ".local", "bin"), true, false, false},
		{filepath.Join(home, "bin"), true, false, false},

		// System directories (require sudo on most systems)
		{"/usr/local/bin", true, false, true},
	}

	// Windows-specific paths
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			locations = []struct {
				path     string
				inPath   bool
				writable bool
				sudo     bool
			}{
				{filepath.Join(appData, "automergent", "bin"), false, false, false},
				{filepath.Join(home, "bin"), false, false, false},
			}
		}
	}

	// Check if location is in PATH
	pathEnv := os.Getenv("PATH")
	pathDirs := strings.Split(pathEnv, string(os.PathListSeparator))

	for i := range locations {
		for _, pathDir := range pathDirs {
			if pathDir == locations[i].path {
				locations[i].inPath = true
				break
			}
		}
	}

	// Check writability
	for i := range locations {
		if isDirWritable(locations[i].path) {
			locations[i].writable = true
		}
	}

	// Select best location:
	// 1. Writable and in PATH
	// 2. Writable (we'll add to PATH)
	// 3. In PATH but not writable (needs sudo)

	for _, loc := range locations {
		if loc.writable && loc.inPath {
			return loc.path, false, nil
		}
	}

	for _, loc := range locations {
		if loc.writable {
			return loc.path, false, nil
		}
	}

	// Fall back to ~/.local/bin - create if needed
	localBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0755); err == nil {
		return localBin, false, nil
	}

	// Last resort - /usr/local/bin with sudo
	return "/usr/local/bin", true, nil
}

// RunZeroConfig performs a zero-configuration installation
func RunZeroConfig(opts *ZeroConfigOptions) (*ZeroConfigResult, error) {
	if opts == nil {
		opts = DefaultZeroConfigOptions()
	}

	result := &ZeroConfigResult{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Aliases: make(map[string]string),
	}

	// Detect optimal install location
	var requiresSudo bool
	var err error

	if opts.ForceInstallDir != "" {
		result.InstallDir = opts.ForceInstallDir
		requiresSudo = !isDirWritable(opts.ForceInstallDir)
	} else {
		result.InstallDir, requiresSudo, err = DetectOptimalInstallLocation()
		if err != nil {
			return result, fmt.Errorf("failed to detect install location: %w", err)
		}
	}
	result.RequiresSudo = requiresSudo

	// Detect shell
	result.Shell = DetectShell()

	// Check for existing installation
	existingPath, existingVersion, _ := DetectCurrentInstallation()
	if existingPath != "" {
		result.ExistingInstall = true
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Existing installation found at %s (version: %s)", existingPath, existingVersion))
	}

	// Setup shell integration
	if !opts.SkipPath || !opts.SkipCompletions {
		si, err := NewShellIntegration()
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Shell integration setup failed: %v", err))
		} else {
			// Add to PATH
			if !opts.SkipPath {
				if err := si.AddToPath(result.InstallDir); err != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to add to PATH: %v", err))
				} else {
					result.PathConfigured = true
				}
			}

			// Install completions
			if !opts.SkipCompletions {
				if err := si.InstallCompletions(result.InstallDir); err != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to install completions: %v", err))
				} else {
					result.Completions = true
				}
			}

			// Setup aliases
			if !opts.SkipAliases {
				aliases := SuggestedAliases()
				for alias, command := range aliases {
					fullCommand := filepath.Join(result.InstallDir, command)
					if command == "automergent" {
						result.Aliases[alias] = fullCommand
					}
				}
			}

			// Setup environment
			si.SetupEnvironment()
		}
	}

	// Check for migration sources
	if !opts.SkipMigration {
		sources := DetectMigrationSources()
		for _, src := range sources {
			if src.Detected {
				result.Migrations = append(result.Migrations, src.Source)
			}
		}
	}

	return result, nil
}

// GenerateInstallCommand creates a one-liner install command
func GenerateInstallCommand(version string) string {
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf("curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | sh", Repo)
}

// GenerateUninstallCommand creates an uninstall command
func GenerateUninstallCommand() string {
	return "automergent uninstall"
}

// Uninstall removes Automergent from the system
func Uninstall(keepConfig bool) error {
	// Find installation
	currentPath, _, err := DetectCurrentInstallation()
	if err != nil {
		return fmt.Errorf("failed to detect installation: %w", err)
	}

	if currentPath == "" {
		return fmt.Errorf("automergent installation not found")
	}

	installDir := filepath.Dir(currentPath)

	// Remove binaries
	binaries := []string{"automergent", "owe"}
	for _, bin := range binaries {
		binPath := filepath.Join(installDir, bin)
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}
		os.Remove(binPath)
	}

	// Remove config if not keeping
	if !keepConfig {
		home, _ := os.UserHomeDir()
		configDir := filepath.Join(home, ".automergent")
		if err := os.RemoveAll(configDir); err != nil {
			return fmt.Errorf("failed to remove config directory: %w", err)
		}
	}

	// Try to clean up shell config
	si, err := NewShellIntegration()
	if err == nil {
		cleanupShellConfig(si)
	}

	return nil
}

// cleanupShellConfig removes Automergent entries from shell config
func cleanupShellConfig(si *ShellIntegration) {
	content, err := os.ReadFile(si.ConfigFile)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	skipNext := false

	for _, line := range lines {
		// Skip Automergent-related lines
		if strings.Contains(line, "# Automergent") {
			skipNext = true
			continue
		}
		if skipNext && (strings.Contains(line, "automergent") || strings.Contains(line, "AUTOMERGENT")) {
			skipNext = false
			continue
		}
		skipNext = false
		newLines = append(newLines, line)
	}

	os.WriteFile(si.ConfigFile, []byte(strings.Join(newLines, "\n")), 0644)
}

// PrintZeroConfigSummary prints a summary of the zero-config result
func PrintZeroConfigSummary(result *ZeroConfigResult) string {
	var sb strings.Builder

	sb.WriteString("╭─────────────────────────────────────────────────────────────╮\n")
	sb.WriteString("│                  Automergent Installation Summary                │\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────┤\n")

	sb.WriteString(fmt.Sprintf("│  System: %-50s │\n", fmt.Sprintf("%s/%s", result.OS, result.Arch)))
	sb.WriteString(fmt.Sprintf("│  Install Directory: %-39s │\n", result.InstallDir))
	sb.WriteString(fmt.Sprintf("│  Shell: %-51s │\n", result.Shell))

	pathStatus := "✗ Not configured"
	if result.PathConfigured {
		pathStatus = "✓ Configured"
	}
	sb.WriteString(fmt.Sprintf("│  PATH: %-52s │\n", pathStatus))

	compStatus := "✗ Not installed"
	if result.Completions {
		compStatus = "✓ Installed"
	}
	sb.WriteString(fmt.Sprintf("│  Completions: %-45s │\n", compStatus))

	if result.RequiresSudo {
		sb.WriteString("│  Note: Elevated privileges may be required              │\n")
	}

	if len(result.Migrations) > 0 {
		sb.WriteString("├─────────────────────────────────────────────────────────────┤\n")
		sb.WriteString("│  Migration options available:                              │\n")
		for _, m := range result.Migrations {
			sb.WriteString(fmt.Sprintf("│    • %s%-54s │\n", m, ""))
		}
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("├─────────────────────────────────────────────────────────────┤\n")
		sb.WriteString("│  Warnings:                                                 │\n")
		for _, w := range result.Warnings {
			// Truncate long warnings
			if len(w) > 55 {
				w = w[:52] + "..."
			}
			sb.WriteString(fmt.Sprintf("│    ▲ %-53s │\n", w))
		}
	}

	sb.WriteString("╰─────────────────────────────────────────────────────────────╯\n")

	return sb.String()
}

// QuickInstall performs a minimal installation with sensible defaults
func QuickInstall(progressChan chan float64) error {
	// Get latest version
	version, err := GetLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}

	// Get system info
	sysInfo, err := GetSystemInfo()
	if err != nil {
		return fmt.Errorf("failed to get system info: %w", err)
	}

	// Use optimal location
	installDir, _, err := DetectOptimalInstallLocation()
	if err != nil {
		installDir = sysInfo.DestDir
	}
	sysInfo.DestDir = installDir

	// Ensure directory exists
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// Download binary
	archivePath, err := DownloadBinary(version, sysInfo, progressChan)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer os.Remove(archivePath)

	// Extract
	if err := ExtractBinary(archivePath, installDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Setup binary (create owe symlink)
	if err := SetupBinary(installDir); err != nil {
		// Non-fatal
	}

	// Setup shell integration
	si, err := NewShellIntegration()
	if err == nil {
		si.FullSetup(installDir)
	}

	return nil
}
