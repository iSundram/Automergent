package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/cache"
	"github.com/iSundram/Automergent/internal/config"
	contextmgr "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/version"
)

// AgentPhase represents the current stage of the development lifecycle.
type AgentPhase string

const (
	PhaseInit     AgentPhase = "init"
	PhaseExplore  AgentPhase = "explore"
	PhasePlan     AgentPhase = "plan"
	PhaseBuild    AgentPhase = "build"
	PhaseVerify   AgentPhase = "verify"
)

// DetectPhase analyzes the current state to determine the agent phase.
func DetectPhase(messages []ai.Message) AgentPhase {
	if len(messages) == 0 {
		return PhaseInit
	}

	hasExplored := hasExploreToolResult(messages)
	hasPlanned := hasPlanFile()
	hasBuilt := hasBuildToolResult(messages)

	if hasBuilt {
		return PhaseVerify
	}
	if hasPlanned {
		return PhaseBuild
	}
	if hasExplored {
		return PhasePlan
	}

	return PhaseInit
}

func hasExploreToolResult(messages []ai.Message) bool {
	toolNameByCallID := make(map[string]string)
	for _, m := range messages {
		for _, tc := range m.ToolCallParts() {
			if tc.ID != "" {
				toolNameByCallID[tc.ID] = tc.Name
			}
		}
	}

	for _, m := range messages {
		if m.Role != ai.RoleTool {
			continue
		}
		metadataToolName := toolNameFromMetadata(m.Metadata)
		for _, p := range m.Content {
			if p.Type != ai.ContentTypeToolResult || p.ToolResult == nil {
				continue
			}
			toolName := toolNameByCallID[p.ToolResult.ToolCallID]
			if toolName == "" {
				toolName = metadataToolName
			}
			if isExploreToolName(toolName) {
				return true
			}
		}
	}
	return false
}
func toolNameFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range []string{"tool_name", "toolName", "tool_call_name", "toolCallName"} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		name, ok := raw.(string)
		if ok {
			return name
		}
	}
	return ""
}


func isExploreToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	return strings.HasPrefix(name, "grep") ||
		strings.HasPrefix(name, "glob") ||
		strings.HasPrefix(name, "read") ||
		name == "view" ||
		strings.Contains(name, "search")
}

func hasPlanFile() bool {
	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, "AUTOMERGENT_PLAN.md")); err == nil {
		return true
	}
	return false
}

func hasBuildToolResult(messages []ai.Message) bool {
	toolNameByCallID := make(map[string]string)
	for _, m := range messages {
		for _, tc := range m.ToolCallParts() {
			if tc.ID != "" {
				toolNameByCallID[tc.ID] = tc.Name
			}
		}
	}

	for _, m := range messages {
		if m.Role != ai.RoleTool {
			continue
		}
		metadataToolName := toolNameFromMetadata(m.Metadata)
		for _, p := range m.Content {
			if p.Type != ai.ContentTypeToolResult || p.ToolResult == nil {
				continue
			}
			toolName := toolNameByCallID[p.ToolResult.ToolCallID]
			if toolName == "" {
				toolName = metadataToolName
			}
			if isBuildToolName(toolName) {
				return true
			}
		}
	}
	return false
}

func isBuildToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	return strings.HasPrefix(name, "edit") ||
		strings.HasPrefix(name, "write") ||
		name == "bash"
}

type promptSection struct {
	name         string
	content      string
	classifyAs   cache.ContentClassification
	isCacheBreak bool
}

const (
	maxPromptContextFiles  = 4
	maxPromptContextTokens = 2000
	maxPromptContextChars  = 1600
)

func renderIdentity() string {
	return fmt.Sprintf("# Identity\nYou are Automergent %s, a senior lead software engineer and autonomous agent. You take full responsibility for the technical integrity, security, and maintainability of the workspace. You operate with precision, focusing on solving problems rather than just completing tickets.\n\n", version.Version)
}

func renderTaskProtocol() string {
	return `
# System-Level Engineering Protocol
You are a full-system autonomous engineer operating in large-scale codebases. You must execute true system-level engineering rather than shallow, file-level edits. Your workflow must rigorously adhere to the following lifecycle:

1. **Mandatory Exploration Phase (System Discovery):**
   - **Active Search & Symbol Lookup:** You MUST actively use grep, glob, and read tools to identify *all* related components before any modification. Do not guess.
   - **Component Mapping:** Trace definitions, usages, imports, initialization points, and integration layers across the entire repository.
   - **Relevance Filtering:** Filter your discoveries. Select the most critical structural files rather than acting on shallow matches.

2. **Deep Analysis & Blast Radius:**
   - Identify the root cause for bugs or full system requirements for features.
   - Map the "Blast Radius" of your intended change across all modules and dependencies.

3. **System-Aware Strategic Plan:**
   - For all non-trivial changes, present a structured plan before editing:
     ### 🎯 Objective: [System-level summary]
     ### 🔍 Findings: [Key definitions, usages, and integration points discovered]
     ### 🛠️ Proposed Changes: [Comprehensive list of all affected files to be updated]

4. **System Integration Guarantee (Execution):**
   - **No Partial Edits:** You MUST update ALL affected files across the codebase. Treat a single-file update as INCOMPLETE if multiple integration points, imports, or usages exist.
   - Maintain strict correctness and idiomatic consistency across all modified layers.
`
}

func renderSafetyProtocols() string {
	return `
# Safety & Blast Radius
Carefully consider the reversibility of your actions. 

- **Safe Actions:** Reading files, searching code, running local read-only tests. You may perform these freely.
- **Moderate Actions:** Creating or editing files, adding dependencies. Perform these surgically after planning.
- **Destructive/Irreversible Actions:** Deleting files, 'git push --force', 'rm -rf', dropping database tables, killing system processes.
  - **MANDATE:** You MUST explicitly describe the risk and wait for user confirmation before executing any destructive or hard-to-reverse action. 
  - Do not use destructive shortcuts (e.g., --no-verify) to bypass obstacles.
`
}

func renderCollaborativeJudgment() string {
	return `
# Collaborative Judgment
You are a collaborator, not a submissive executor. 
- **Challenge Assumptions:** If a user's request is based on a misconception or would introduce a bug/security flaw, you MUST point it out and suggest a better approach.
- **Adjacent Awareness:** If you spot a bug or a better way to refactor code *adjacent* to your current task, surface it to the user.
- **Architecture First:** Prioritize clean, idiomatic abstractions over "quick fixes." Don't design for hypothetical futures, but don't leave technical debt behind.
`
}

func renderEfficiencyProtocols(reg *tools.Registry) string {
	var sb strings.Builder
	sb.WriteString("\n# Efficiency & Context Management\n")
	sb.WriteString("- **Parallelism:** Execute independent tool calls (e.g., reading multiple files) in a single turn.\n")
	sb.WriteString("- **Grep with Context:** Use `grep -C` to understand code points in one turn, avoiding redundant `read_file` calls.\n")
	sb.WriteString("- **Trajectory Awareness:** Your <thought> blocks are preserved across tool loops. Use them to maintain your internal reasoning state.\n")

	if reg != nil {
		sb.WriteString("\n## Tool Protocols\n")
		sb.WriteString("- **Read Before Edit:** Always read a file's state before editing.\n")
		sb.WriteString("- **Dedicated Tools:** Use `edit_file` instead of `sed`. Use `create_file` for new paths.\n")
	}
	return sb.String()
}

func renderProjectContext(cfg *config.Config, messages []ai.Message, cm *contextmgr.Manager) string {
	var sb strings.Builder
	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("\n# Project Context\n- Working Directory: %s\n- Mode: %s\n", cwd, cfg.Mode))

	// Lightweight Context Compaction: Summarize if history is getting long
	if len(messages) > 15 {
		sb.WriteString("\n[Trajectory Note: The conversation history is long. Focus on the most recent state and your established plan.]\n")
	}

	// Load AUTOMERGENT.md if present
	if data, err := os.ReadFile(filepath.Join(cwd, "AUTOMERGENT.md")); err == nil {
		sb.WriteString("\n## Project Mandates (AUTOMERGENT.md)\n")
		sb.WriteString(string(data))
	}
	renderManagedContextSelection(&sb, cfg, messages, cwd, cm)

	return sb.String()
}

func assemblePromptSections(sections []promptSection) string {
	var staticSections []string
	var dynamicSections []string
	for _, section := range sections {
		if section.isCacheBreak {
			continue
		}
		content := strings.TrimSpace(section.content)
		if content == "" {
			continue
		}
		content += "\n\n"
		switch section.classifyAs {
		case cache.ClassificationDynamic, cache.ClassificationVolatile:
			dynamicSections = append(dynamicSections, content)
		default:
			staticSections = append(staticSections, content)
		}
	}

	staticPart := strings.TrimSpace(strings.Join(staticSections, ""))
	dynamicPart := strings.TrimSpace(strings.Join(dynamicSections, ""))
	return cache.InsertBoundaryMarker(staticPart, dynamicPart)
}

func renderManagedContextSelection(sb *strings.Builder, cfg *config.Config, messages []ai.Message, cwd string, cm *contextmgr.Manager) {
	activeFiles := resolveExistingContextFiles(cwd, cfg.ContextFiles)
	if len(activeFiles) == 0 {
		return
	}

	if cm == nil {
		return
	}
	manager := cm
	intent := latestUserIntent(messages)
	resp, err := manager.GetContext(context.Background(), contextmgr.ContextRequest{
		Intent:      intent,
		ActiveFiles: activeFiles,
		TokenBudget: maxPromptContextTokens,
		IncludeDeps: true,
	})
	if err != nil && resp == nil {
		return
	}

	items := resp.Items
	if len(items) == 0 {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Priority > items[j].Priority })
	if len(items) > maxPromptContextFiles {
		items = items[:maxPromptContextFiles]
	}

	sb.WriteString("\n## Selected File Context\n")
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("- %s (%d tokens)\n", item.Path, item.Tokens))
		sb.WriteString("```text\n")
		sb.WriteString(truncateContextContent(item.Content, maxPromptContextChars))
		sb.WriteString("\n```\n")
	}
}

func resolveExistingContextFiles(cwd string, configured []string) []string {
	seen := make(map[string]bool, len(configured))
	files := make([]string, 0, len(configured))
	for _, path := range configured {
		if strings.TrimSpace(path) == "" {
			continue
		}
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(cwd, resolved)
		}
		if seen[resolved] {
			continue
		}
		if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
			seen[resolved] = true
			files = append(files, resolved)
		}
	}
	return files
}

func latestUserIntent(messages []ai.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleUser {
			text := strings.TrimSpace(messages[i].TextContent())
			if text != "" {
				return text
			}
		}
	}
	return "current task"
}

func truncateContextContent(content string, maxChars int) string {
	content = strings.TrimSpace(content)
	if len(content) <= maxChars {
		return content
	}
	half := maxChars / 2
	return content[:half] + "\n... [truncated] ...\n" + content[len(content)-half:]
}

// isImportantMessage reports whether a message should be preserved during compaction.
func (a *Agent) isImportantMessage(msg ai.Message) bool {
	if msg.Role == ai.RoleUser {
		text := strings.ToLower(msg.TextContent())
		return strings.Contains(text, "confirm") ||
			strings.Contains(text, "constraint") ||
			strings.Contains(text, "must") ||
			strings.Contains(text, "should")
	}

	if msg.Role == ai.RoleTool {
		for _, part := range msg.Content {
			if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil && part.ToolResult.IsError {
				return true
			}
		}
		return false
	}

	if msg.Role == ai.RoleAssistant {
		text := strings.ToLower(msg.TextContent())
		if strings.Contains(text, "plan") || strings.Contains(text, "approach") || strings.Contains(text, "strategy") {
			return true
		}
		for _, tc := range msg.ToolCallParts() {
			if tc.Name == "edit_file" || tc.Name == "write_file" {
				return true
			}
		}
	}

	return false
}

// identifyImportantMessages filters messages that should be preserved during compaction.
func (a *Agent) identifyImportantMessages(messages []ai.Message) []ai.Message {
	important := make([]ai.Message, 0, len(messages))
	for _, msg := range messages {
		if a.isImportantMessage(msg) {
			important = append(important, msg)
		}
	}
	return important
}

// summarizeWithLLM uses the AI provider to generate a concise summary of messages.
// It formats the messages as text and calls the provider with a specialized summarization prompt.
func (a *Agent) summarizeWithLLM(ctx context.Context, messages []ai.Message, prompt string) string {
	// Format messages as text for summarization
	var sb strings.Builder
	for i, msg := range messages {
		sb.WriteString(fmt.Sprintf("=== Message %d (%s) ===\n", i+1, msg.Role))
		sb.WriteString(msg.PlaintextForHistory())
		sb.WriteString("\n\n")
	}

	// Create summarization request
	req := ai.CompletionRequest{
		Messages: []ai.Message{
			ai.NewTextMessage(ai.RoleUser, fmt.Sprintf("%s\n\nConversation history:\n%s", prompt, sb.String())),
		},
		System:      "You are a precise technical summarizer. Extract and condense key information without adding interpretation.",
		Temperature: 0.3,
		MaxTokens:   1000,
		Stream:      true,
	}

	// Call provider
	resp, err := a.provider.Complete(ctx, req)
	if err != nil {
		// Fallback to simple count if LLM fails
		return fmt.Sprintf("[LLM summarization unavailable: %v. Messages in compacted range: %d]", err, len(messages))
	}

	// Extract summary text from response
	var summary strings.Builder
	for chunk := range resp.Stream() {
		if chunk.Error != nil {
			return fmt.Sprintf("[Summarization error: %v]", chunk.Error)
		}
		summary.WriteString(chunk.Text)
	}

	return summary.String()
}

// GhostLargeOutputs truncates large tool results to save context space,
// replacing the full content with a summary and a hint for the model.
func (a *Agent) GhostLargeOutputs(messages []ai.Message) []ai.Message {
	maxChars := 32768
	if a.cfg != nil && a.cfg.MaxToolOutputChars > 0 {
		maxChars = a.cfg.MaxToolOutputChars
	}

	ghosted := make([]ai.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != ai.RoleTool {
			ghosted[i] = msg
			continue
		}

		newMsg := msg
		newMsg.Content = make([]ai.ContentPart, len(msg.Content))
		for j, part := range msg.Content {
			if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil && len(part.ToolResult.Content) > maxChars {
				// Ghost the result
				limit := 500
				if len(part.ToolResult.Content) < limit {
					limit = len(part.ToolResult.Content)
				}
				summary := fmt.Sprintf("[Output too large (%d chars). Truncated to first %d chars...]\n\n%s\n\n... [output continues for %d more chars. Use 'read_file' or specific 'grep' if you need more details.]",
					len(part.ToolResult.Content),
					limit,
					part.ToolResult.Content[:limit],
					len(part.ToolResult.Content)-limit)

				newResult := *part.ToolResult
				newResult.Content = summary
				newMsg.Content[j] = ai.ContentPart{
					Type:       ai.ContentTypeToolResult,
					ToolResult: &newResult,
				}
			} else {
				newMsg.Content[j] = part
			}
		}
		ghosted[i] = newMsg
	}
	return ghosted
}

// CompactSessionMessages provides intelligent context compaction with LLM-based summarization.
// This should be called by the Agent loop when context usage is high.
func (a *Agent) CompactSessionMessages(ctx context.Context, messages []ai.Message) []ai.Message {
	// 0. Pre-process large tool outputs
	messages = a.GhostLargeOutputs(messages)

	// Respect configured compaction thresholds
	keepRecent := 8
	if a.cfg != nil && a.cfg.CompressionKeepRecent > 0 {
		keepRecent = a.cfg.CompressionKeepRecent
	}

	if len(messages) <= 12 || len(messages) <= keepRecent+4 {
		return messages
	}

	compacted := make([]ai.Message, 0)
	// Keep the first message (Original Intent / System Prompt)
	compacted = append(compacted, messages[0])

	// Determine the range to compact
	startIdx := len(messages) - keepRecent
	if startIdx < 1 {
		startIdx = 1
	}

	// Extract messages to be compacted (middle section)
	messagesToCompact := messages[1:startIdx]

	// Identify important messages from the range to compact
	importantMessages := a.identifyImportantMessages(messagesToCompact)

	// Prepare remaining messages for summarization
	var messagesToSummarize []ai.Message
	for _, msg := range messagesToCompact {
		if !a.isImportantMessage(msg) {
			messagesToSummarize = append(messagesToSummarize, msg)
		}
	}

	// Generate LLM-based summary if there are messages to summarize
	var summaryText string
	if len(messagesToSummarize) > 0 {
		prompt := `Summarize this segment of the coding session history. 
Focus on:
1. The specific problem being addressed.
2. Key files investigated or modified.
3. Decisions made and rationale provided.
4. Any constraints or requirements identified.
5. Successful vs. failed attempts.

Keep the summary technical and concise (max 800 words).`

		summaryText = a.summarizeWithLLM(ctx, messagesToSummarize, prompt)
	} else {
		summaryText = "[No additional messages required summarization]"
	}

	// Add the LLM-generated summary as a system message with a "Neural" header
	summaryMsg := ai.Message{
		Role: ai.RoleSystem,
		Content: []ai.ContentPart{{
			Type: ai.ContentTypeText,
			Text: fmt.Sprintf("# Neural Context Summary\n\n> This is a compressed representation of the earlier conversation to maintain context efficiency.\n\n%s\n\n---", summaryText),
		}},
	}
	compacted = append(compacted, summaryMsg)

	// Add important messages (Preserved context)
	if len(importantMessages) > 0 {
		compacted = append(compacted, ai.Message{
			Role: ai.RoleSystem,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeText,
				Text: "## Preserved Key Context\nThe following high-signal messages from the compacted history have been preserved for reference:",
			}},
		})
		compacted = append(compacted, importantMessages...)
	}

	// Keep the most recent messages
	compacted = append(compacted, messages[startIdx:]...)

	return compacted
}
