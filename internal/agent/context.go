package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/version"
)

// buildSystemPrompt constructs the system prompt for the AI.
func buildSystemPrompt(cfg *config.Config) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are Automergent %s, an expert AI coding agent for the terminal. ", version.Version))
	sb.WriteString("You are highly capable, precise, and proactive. ")
	sb.WriteString("You help users with all coding tasks: reading, writing, refactoring, debugging, ")
	sb.WriteString("testing, documenting, and explaining code across any language or framework.\n\n")

	sb.WriteString("## Capabilities\n")
	sb.WriteString("- Read and write files with read_file, write_file, patch_file\n")
	sb.WriteString("- Search codebases with grep\n")
	sb.WriteString("- Execute shell commands with run_command\n")
	sb.WriteString("- Browse directories with list_directory\n")
	sb.WriteString("- View git history, status, diffs with git tools\n")
	sb.WriteString("- Fetch web pages and search the web\n\n")

	sb.WriteString("## Behavior Guidelines\n")
	sb.WriteString("- Always read files before editing them — never guess at content\n")
	sb.WriteString("- Make precise, surgical changes; avoid unnecessary rewrites\n")
	sb.WriteString("- Explain what you're doing and why when making changes\n")
	sb.WriteString("- Run tests after making changes when a test suite exists\n")
	sb.WriteString("- Prefer using existing tools/libraries over introducing new dependencies\n")
	sb.WriteString("- If you encounter an error, diagnose it carefully before trying again\n\n")

	sb.WriteString(fmt.Sprintf("## Approval Mode: %s\n", cfg.Mode))
	modeDesc := modeDescription(cfg.Mode)
	if modeDesc != "" {
		sb.WriteString(modeDesc + "\n")
	}
	sb.WriteString("\n")

	// Add working directory context
	cwd, err := os.Getwd()
	if err == nil {
		sb.WriteString(fmt.Sprintf("Working directory: %s\n\n", cwd))
	}

	// Load AUTOMERGENT.md if present
	automergentMD := loadContextFile(cwd)
	if automergentMD != "" {
		sb.WriteString("## Project Context (AUTOMERGENT.md)\n")
		sb.WriteString(automergentMD)
		sb.WriteString("\n\n")
	}

	// Load extra context files
	for _, p := range cfg.ContextFiles {
		data, err := os.ReadFile(p)
		if err == nil {
			sb.WriteString(fmt.Sprintf("## %s\n", filepath.Base(p)))
			sb.Write(data)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func modeDescription(mode string) string {
	switch mode {
	case "suggest":
		return "In suggest mode, propose changes but ask for confirmation before writing files or running commands."
	case "auto-edit":
		return "In auto-edit mode, automatically read and write files, but ask for confirmation before running shell commands."
	case "full-auto":
		return "In full-auto mode, automatically perform all actions without asking for confirmation."
	case "plan":
		return "In plan mode, create a detailed plan of the changes to be made, then ask for approval before executing."
	default:
		return ""
	}
}

func loadContextFile(dir string) string {
	for _, name := range []string{"AUTOMERGENT.md", ".automergent.md"} {
		current := dir
		for {
			path := filepath.Join(current, name)
			data, err := os.ReadFile(path)
			if err == nil {
				return string(data)
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return ""
}
