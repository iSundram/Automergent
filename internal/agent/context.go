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

	sb.WriteString(fmt.Sprintf("You are Automergent %s, an AI coding agent for the terminal.\n\n", version.Version))
	sb.WriteString("You help users with coding tasks including reading, writing, and modifying code files, ")
	sb.WriteString("running commands, and answering programming questions.\n\n")

	sb.WriteString(fmt.Sprintf("Mode: %s\n", cfg.Mode))
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

	sb.WriteString("Always be concise, precise, and helpful. ")
	sb.WriteString("Use the provided tools to interact with the filesystem and run commands. ")
	sb.WriteString("Never make up file contents — always read files before editing them.")

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
