package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ExistingTool represents a detected existing CLI tool
type ExistingTool struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Conflict    bool   `json:"conflict,omitempty"`
}

// Dependency represents a required system dependency
type Dependency struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	MinVersion  string `json:"min_version,omitempty"`
	InstallHint string `json:"install_hint,omitempty"`
}

// DetectionResult holds all detection results
type DetectionResult struct {
	ExistingTools  []ExistingTool `json:"existing_tools"`
	Dependencies   []Dependency   `json:"dependencies"`
	CurrentVersion string         `json:"current_version,omitempty"`
	Conflicts      []string       `json:"conflicts,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// KnownTools is a list of CLI tools we should detect
var KnownTools = []string{
	"claude",
	"claude-code",
	"aider",
	"cursor",
	"copilot",
	"cody",
	"continue",
	"tabby",
	"codegpt",
}

// DetectExistingTools scans for existing CLI tools that might conflict
func DetectExistingTools() []ExistingTool {
	var tools []ExistingTool

	for _, name := range KnownTools {
		if path, err := exec.LookPath(name); err == nil {
			tool := ExistingTool{
				Name: name,
				Path: path,
			}

			// Try to get version
			tool.Version = getToolVersion(name, path)
			tool.Description = getToolDescription(name)

			// Check for conflicts with automergent
			if name == "owe" || name == "automergent" {
				tool.Conflict = true
			}

			tools = append(tools, tool)
		}
	}

	// Check for automergent itself
	if path, err := exec.LookPath("automergent"); err == nil {
		tools = append(tools, ExistingTool{
			Name:     "automergent",
			Path:     path,
			Version:  getToolVersion("automergent", path),
			Conflict: true,
		})
	}
	if path, err := exec.LookPath("owe"); err == nil {
		tools = append(tools, ExistingTool{
			Name:     "owe",
			Path:     path,
			Version:  getToolVersion("owe", path),
			Conflict: true,
		})
	}

	return tools
}

// getToolVersion attempts to get the version of a tool
func getToolVersion(name, path string) string {
	// Try common version flags
	for _, flag := range []string{"--version", "-v", "version"} {
		cmd := exec.Command(path, flag)
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			// Extract first line, clean it up
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				version := strings.TrimSpace(lines[0])
				// Limit length
				if len(version) > 50 {
					version = version[:50] + "..."
				}
				return version
			}
		}
	}
	return ""
}

// getToolDescription provides descriptions for known tools
func getToolDescription(name string) string {
	descriptions := map[string]string{
		"claude":      "Anthropic's Claude CLI",
		"claude-code": "Claude Code Assistant",
		"aider":       "AI pair programming in terminal",
		"cursor":      "AI-first code editor",
		"copilot":     "GitHub Copilot CLI",
		"cody":        "Sourcegraph's AI assistant",
		"continue":    "Open-source AI code assistant",
		"tabby":       "Self-hosted AI coding assistant",
		"codegpt":     "CLI for various LLMs",
	}
	return descriptions[name]
}

// DetectDependencies checks for required system dependencies
func DetectDependencies() []Dependency {
	deps := []Dependency{
		{
			Name:        "git",
			Required:    false, // Optional but recommended
			InstallHint: getInstallHint("git"),
		},
		{
			Name:        "curl",
			Required:    false,
			InstallHint: getInstallHint("curl"),
		},
	}

	for i := range deps {
		if path, err := exec.LookPath(deps[i].Name); err == nil {
			deps[i].Installed = true
			deps[i].Version = getToolVersion(deps[i].Name, path)
		}
	}

	return deps
}

// getInstallHint returns OS-specific install hints
func getInstallHint(pkg string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("brew install %s", pkg)
	case "linux":
		// Try to detect package manager
		if _, err := exec.LookPath("apt"); err == nil {
			return fmt.Sprintf("sudo apt install %s", pkg)
		}
		if _, err := exec.LookPath("dnf"); err == nil {
			return fmt.Sprintf("sudo dnf install %s", pkg)
		}
		if _, err := exec.LookPath("pacman"); err == nil {
			return fmt.Sprintf("sudo pacman -S %s", pkg)
		}
		if _, err := exec.LookPath("apk"); err == nil {
			return fmt.Sprintf("sudo apk add %s", pkg)
		}
		return fmt.Sprintf("Install %s using your package manager", pkg)
	case "windows":
		return fmt.Sprintf("winget install %s", pkg)
	default:
		return fmt.Sprintf("Install %s", pkg)
	}
}

// DetectCurrentInstallation checks for existing Automergent installation
func DetectCurrentInstallation() (string, string, error) {
	// Check standard locations
	locations := getInstallLocations()

	for _, loc := range locations {
		binaryPath := filepath.Join(loc, "automergent")
		if runtime.GOOS == "windows" {
			binaryPath += ".exe"
		}

		if _, err := os.Stat(binaryPath); err == nil {
			version := getToolVersion("automergent", binaryPath)
			return binaryPath, version, nil
		}
	}

	// Check PATH
	if path, err := exec.LookPath("automergent"); err == nil {
		version := getToolVersion("automergent", path)
		return path, version, nil
	}

	return "", "", nil
}

// getInstallLocations returns possible installation directories
func getInstallLocations() []string {
	home, _ := os.UserHomeDir()
	locations := []string{
		"/usr/local/bin",
		"/usr/bin",
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
	}

	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			locations = append(locations, filepath.Join(appData, "automergent", "bin"))
		}
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			locations = append(locations, filepath.Join(localAppData, "automergent", "bin"))
		}
	}

	return locations
}

// RunDetection performs full system detection
func RunDetection() *DetectionResult {
	result := &DetectionResult{
		ExistingTools: DetectExistingTools(),
		Dependencies:  DetectDependencies(),
	}

	// Check for current installation
	path, version, _ := DetectCurrentInstallation()
	if path != "" {
		result.CurrentVersion = version
	}

	// Check for conflicts
	for _, tool := range result.ExistingTools {
		if tool.Conflict {
			result.Conflicts = append(result.Conflicts,
				fmt.Sprintf("%s at %s (version: %s)", tool.Name, tool.Path, tool.Version))
		}
	}

	// Add warnings
	for _, dep := range result.Dependencies {
		if dep.Required && !dep.Installed {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Required dependency '%s' is not installed. %s", dep.Name, dep.InstallHint))
		}
	}

	return result
}

// DetectionResultJSON returns detection results as JSON
func DetectionResultJSON() (string, error) {
	result := RunDetection()
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
