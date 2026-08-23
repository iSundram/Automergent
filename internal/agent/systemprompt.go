package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/version"
)

// buildUnifiedSystemPrompt assembles THE system prompt for the agent.
// This is the single source of truth: there is no assistant/coder split and
// no legacy fallback path. Static engineering sections are composed with
// dynamic per-turn blocks produced by the prep pipeline (intents, init phase,
// task plan) so every turn gets one coherent persona and contract.
func (a *Agent) buildUnifiedSystemPrompt(ctx context.Context, provider ai.Provider) string {
	var sb strings.Builder

	// --- Identity (single persona) ---
	sb.WriteString("# Identity\n")
	sb.WriteString(fmt.Sprintf("You are Automergent %s, a senior lead software engineer and autonomous agent. "+
		"You take full responsibility for the technical integrity, security, and maintainability of the workspace. "+
		"You plan, implement, verify, and communicate — all in one conversation.\n\n", version.Version))

	// --- Prep pipeline results: pre-explored context ---
	if initResults := a.promptSystem.GetInitResults(); initResults != nil && len(initResults.FilesFound) > 0 {
		sb.WriteString("## Pre-Explored Context (prep phase ALREADY executed — discovery is DONE)\n")
		sb.WriteString("IMPORTANT: The codebase was already explored for this request. Do NOT call glob, list_directory, or broad search tools — they are disabled for this request.\n")
		sb.WriteString("START by reading the most relevant files below with read_file.\n\n")
		sb.WriteString(fmt.Sprintf("Discovered files (%d):\n", len(initResults.FilesFound)))
		for i, f := range initResults.FilesFound {
			if i >= 30 {
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(initResults.FilesFound)-30))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		if len(initResults.CodeSnippets) > 0 {
			sb.WriteString("\nContents already loaded (no need to re-read):\n")
			for path := range initResults.CodeSnippets {
				sb.WriteString(fmt.Sprintf("- %s\n", path))
			}
		}
		if len(initResults.Errors) > 0 {
			sb.WriteString(fmt.Sprintf("\n(prep phase had %d failed actions — ignore those targets)\n", len(initResults.Errors)))
		}
		sb.WriteString("\n")
	}

	// --- Prep pipeline results: intents, tasks, working areas, context needs ---
	if intentSet := a.promptSystem.GetCurrentIntentSet(); intentSet != nil {
		sb.WriteString(fmt.Sprintf("## Current Request\n%s\n\n", intentSet.OriginalPrompt))

		sb.WriteString("## Identified Intents\n")
		for _, intent := range intentSet.Intents {
			sb.WriteString(fmt.Sprintf("- %s (priority: %d)\n", intent.Type, intent.Priority))
		}
		sb.WriteString("\n")

		tasks := a.promptSystem.GetCurrentTasks()
		if len(tasks) > 0 {
			sb.WriteString("## Task Progress\n")
			for _, task := range tasks {
				sb.WriteString(fmt.Sprintf("⏳ %s (priority: %d)\n", task.Description, task.Priority))
			}
			sb.WriteString("\n")
		}

		var workingAreas []string
		for _, task := range tasks {
			if files, ok := task.Context["files_found"].([]string); ok {
				workingAreas = append(workingAreas, files...)
			}
		}
		if len(workingAreas) > 0 {
			sb.WriteString("## Working Areas\n")
			for _, f := range workingAreas {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
			sb.WriteString("\n")
		}

		var allContextNeeds []promptpkg.ContextNeed
		for _, task := range tasks {
			if needs, ok := task.Context["context_needs"].([]promptpkg.ContextNeed); ok {
				allContextNeeds = append(allContextNeeds, needs...)
			}
		}
		if len(allContextNeeds) > 0 {
			sb.WriteString("## Context Requirements\n")
			for _, need := range allContextNeeds {
				if need.InjectTiming != promptpkg.InjectTimingDeferred {
					sb.WriteString(fmt.Sprintf("- %s: %s\n", need.Key, need.Description))
				}
			}
			sb.WriteString("\n")
		}
	}

	// --- Skills availability ---
	sb.WriteString(skillsPromptBlock(a.skills))

	// --- Mode block (capability-mask specific guidance; see modes.go) ---
	if block := a.modePromptBlock(); block != "" {
		sb.WriteString(block)
		sb.WriteString("\n")
	}

	// --- Tool policy + per-tool documentation from the live registry ---
	sb.WriteString("## Tool Policy\nUse only the native tools exposed for this request. Do not invent unavailable tools. Read the current state of anything before you change it; never claim a change was made without a successful tool result.\n")
	model := ""
	if a.cfg != nil {
		model = a.cfg.Model
	}
	sb.WriteString(promptpkg.RenderToolSections(a.tools, model))

	// --- Engineering protocol (from the legacy rich sections) ---
	sb.WriteString(renderTaskProtocol())
	sb.WriteString(renderCollaborativeJudgment())
	sb.WriteString(renderEfficiencyProtocols(a.tools))

	// --- Safety ---
	sb.WriteString(renderSafetyProtocols())

	// --- Project context ---
	sb.WriteString(renderProjectContext(a.cfg, a.sess.Messages, a.ContextManager()))

	// --- Skill proximity hints based on recently touched files ---
	if hint := skillProximityBlock(a.skills, a.skillSnapshot()); hint != "" {
		sb.WriteString(hint)
		sb.WriteString("\n")
	}

	// --- Long history note ---
	if len(a.sess.Messages) > 15 {
		sb.WriteString("[Note: Conversation history is long. Focus on recent state and established plan.]\n\n")
	}

	return sb.String()
}
