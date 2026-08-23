package prompt

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// InitPhaseExecutor executes the initialization phase actions.
type InitPhaseExecutor struct {
	config *PromptConfig
	// OnStart, when set, is invoked just before each init action runs.
	OnStart func(action InitAction)
	// OnAction, when set, is invoked after each init action completes.
	// It receives the action (with Status/Result populated), any error,
	// and the wall-clock duration of the execution.
	OnAction func(action InitAction, execErr error, duration time.Duration)
}

func NewInitPhaseExecutor(config *PromptConfig) *InitPhaseExecutor {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &InitPhaseExecutor{config: config}
}

func (e *InitPhaseExecutor) Execute(ctx context.Context, phase *InitPhase, workingDir string, executor ToolExecutor) (*InitResults, error) {
	results := &InitResults{
		CodeSnippets: make(map[string]string),
		CompletedAt:  time.Now(),
	}

	var allFiles []string
	var allSnippets = make(map[string]string)
	var errors []string

	for i := range phase.Actions {
		action := &phase.Actions[i]
		action.Status = InitActionInProgress
		if e.OnStart != nil {
			e.OnStart(*action)
		}
		started := time.Now()
		result, err := e.executeAction(ctx, action, workingDir, executor)
		duration := time.Since(started)
		if err != nil {
			action.Status = InitActionFailed
			action.Result = err.Error()
			errors = append(errors, fmt.Sprintf("Action %s (%s): %v", action.ID, action.Tool, err))
			if e.OnAction != nil {
				e.OnAction(*action, err, duration)
			}
			continue
		}

		action.Status = InitActionCompleted
		action.Result = result

		if action.Tool == "glob" || action.Tool == "grep" {
			files := e.parseFileResults(result)
			allFiles = append(allFiles, files...)
		}

		if action.Tool == "read" {
			allSnippets[action.Target] = result
		}

		if e.OnAction != nil {
			e.OnAction(*action, nil, duration)
		}
	}

	uniqueFiles := uniqueStrings(allFiles)
	results.FilesFound = uniqueFiles
	results.CodeSnippets = allSnippets
	results.Errors = errors
	results.Summary = e.generateSummary(phase, uniqueFiles, allSnippets, errors)

	phase.Results = results
	return results, nil
}

func (e *InitPhaseExecutor) executeAction(ctx context.Context, action *InitAction, workingDir string, executor ToolExecutor) (string, error) {
	if executor == nil {
		// Return mock results for testing
		switch action.Tool {
		case "glob":
			return "internal/context/manager.go\ninternal/context/transcript.go\n", nil
		case "grep":
			return "internal/context/manager.go:123\ninternal/context/transcript.go:456\n", nil
		case "read":
			return "// mock file content\npackage context\n\nfunc NewManager() *Manager {}\n", nil
		case "bash":
			return "command output\n", nil
		default:
			return "", fmt.Errorf("unknown tool: %s", action.Tool)
		}
	}

	switch action.Tool {
	case "glob":
		return executor.Glob(ctx, action.Target, workingDir)
	case "grep":
		return executor.Grep(ctx, action.Target, workingDir)
	case "read":
		return executor.Read(ctx, action.Target, workingDir)
	case "bash":
		return executor.Bash(ctx, action.Target, workingDir)
	default:
		return "", fmt.Errorf("unknown tool: %s", action.Tool)
	}
}

func (e *InitPhaseExecutor) parseFileResults(result string) []string {
	var files []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Error") {
			files = append(files, line)
		}
	}
	return files
}

func (e *InitPhaseExecutor) generateSummary(phase *InitPhase, files []string, snippets map[string]string, errors []string) string {
	var sb strings.Builder
	sb.WriteString("Init Phase Summary\n")
	sb.WriteString("==================\n")
	sb.WriteString("Goal: " + phase.Goal + "\n\n")
	sb.WriteString(fmt.Sprintf("Actions executed: %d\n", len(phase.Actions)))
	sb.WriteString(fmt.Sprintf("Files found: %d\n", len(files)))
	sb.WriteString(fmt.Sprintf("Files read: %d\n", len(snippets)))
	sb.WriteString(fmt.Sprintf("Errors: %d\n\n", len(errors)))

	if len(files) > 0 {
		sb.WriteString("Key files discovered:\n")
		for i, f := range files {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(files)-10))
				break
			}
			sb.WriteString("  - " + f + "\n")
		}
		sb.WriteString("\n")
	}

	if len(snippets) > 0 {
		sb.WriteString("File contents loaded:\n")
		for path := range snippets {
			sb.WriteString("  - " + path + " (" + fmt.Sprintf("%d chars", len(snippets[path])) + ")\n")
		}
		sb.WriteString("\n")
	}

	if len(errors) > 0 {
		sb.WriteString("Errors encountered:\n")
		for _, err := range errors {
			sb.WriteString("  - " + err + "\n")
		}
	}

	return sb.String()
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

type ToolExecutor interface {
	Glob(ctx context.Context, pattern, workingDir string) (string, error)
	Grep(ctx context.Context, pattern, workingDir string) (string, error)
	Read(ctx context.Context, path, workingDir string) (string, error)
	Bash(ctx context.Context, command, workingDir string) (string, error)
}

// BuildInitPrompt creates a prompt for the init phase execution.
func BuildInitPrompt(phase *InitPhase) *PromptPart {
	var sb strings.Builder

	sb.WriteString("INITIALIZATION PHASE - Exploration Required\n\n")
	sb.WriteString("Goal: ")
	sb.WriteString(phase.Goal)
	sb.WriteString("\n\n")
	sb.WriteString("You need to execute the following exploration actions to understand the codebase:\n\n")

	for i, action := range phase.Actions {
		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, strings.ToUpper(action.Tool), action.Target))
		sb.WriteString("   Reason: ")
		sb.WriteString(action.Reason)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Success Criteria:\n")
	for _, criteria := range phase.SuccessCriteria {
		sb.WriteString("- ")
		sb.WriteString(criteria)
		sb.WriteString("\n")
	}

	sb.WriteString("\nExecute these actions and report findings. Do not implement fixes yet.")

	return &PromptPart{
		Stage:    StageInitialThinking,
		Content:  sb.String(),
		Tools:    ToolSetReadOnly,
		Metadata: map[string]any{"init_phase": phase},
	}
}

// BuildInitResultsPrompt creates a prompt with init phase results for task generation.
func BuildInitResultsPrompt(phase *InitPhase, intentSet *IntentSet) *PromptPart {
	var sb strings.Builder

	sb.WriteString("INITIALIZATION COMPLETE - Ready for Task Generation\n\n")
	sb.WriteString("Exploration Results:\n")
	sb.WriteString(phase.Results.Summary)
	sb.WriteString("\n\n")

	sb.WriteString("Original Intents Identified:\n")
	for _, intent := range intentSet.Intents {
		sb.WriteString(fmt.Sprintf("- %s (priority: %d", intent.Type, intent.Priority))
		if len(intent.Dependencies) > 0 {
			sb.WriteString(", depends on: " + strings.Join(intent.Dependencies, ", "))
		}
		sb.WriteString(")\n")
	}

	sb.WriteString("\nGenerate specific tasks to fulfill these intents based on the exploration results.")

	return &PromptPart{
		Stage:    StageTaskDefinition,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"intent_set": intentSet, "init_results": phase.Results},
	}
}