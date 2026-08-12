package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/version"
)

// AgentPhase represents the current stage of the development lifecycle.
type AgentPhase string

const (
	PhaseResearch AgentPhase = "research"
	PhasePlan     AgentPhase = "plan"
	PhaseExecute  AgentPhase = "execute"
)

// DetectPhase analyzes the current state to determine the agent phase.
func DetectPhase(messages []ai.Message) AgentPhase {
	if len(messages) == 0 {
		return PhaseResearch
	}

	hasResearched := false
	for _, m := range messages {
		if m.Role == ai.RoleTool {
			for _, p := range m.Content {
				if p.Type == ai.ContentTypeToolResult {
					name := p.ToolResult.ToolCallID
					if strings.Contains(name, "grep") || strings.Contains(name, "glob") || strings.Contains(name, "read") {
						hasResearched = true
					}
				}
			}
		}
	}

	if hasResearched {
		// Check if a plan file was created
		cwd, _ := os.Getwd()
		if _, err := os.Stat(filepath.Join(cwd, "AUTOMERGENT_PLAN.md")); err == nil {
			return PhaseExecute
		}
		return PhasePlan
	}

	return PhaseResearch
}

// buildSystemPrompt orchestrates the modular prompt construction.
func buildSystemPrompt(cfg *config.Config, reg *tools.Registry, messages []ai.Message) string {
	var sb strings.Builder

	// 1. Identity & Role
	sb.WriteString(renderIdentity())

	// 2. Core Task Protocol (Investigation-Driven Workflow)
	sb.WriteString(renderTaskProtocol())

	// 3. Safety & Blast Radius
	sb.WriteString(renderSafetyProtocols())

	// 4. Collaborative Judgment & Engineering Standards
	sb.WriteString(renderCollaborativeJudgment())

	// 5. Verification Gate (The "Contract")
	sb.WriteString(renderVerificationGate())

	// 6. Tool & Context Efficiency
	sb.WriteString(renderEfficiencyProtocols(reg))

	// 7. Dynamic Project Context
	sb.WriteString(renderProjectContext(cfg, messages))

	return sb.String()
}

func renderIdentity() string {
	return fmt.Sprintf("# Identity\nYou are Automergent %s, a senior lead software engineer and autonomous agent. You take full responsibility for the technical integrity, security, and maintainability of the workspace. You operate with precision, focusing on solving problems rather than just completing tickets.\n\n", version.Version)
}

func renderTaskProtocol() string {
	return `
# Core Task Protocol
Follow this unified, self-correcting workflow for every request:

1. **Classify:** Determine the task type:
   - **Inquiry:** Answer questions or analyze code. DO NOT modify files unless explicitly requested.
   - **Bug Fix:** Reproduce the issue first, identify the root cause, then fix.
   - **Feature:** Plan the implementation, add the feature, and write tests.
   - **Refactor:** Improve structure without changing behavior. Verify with existing tests.
2. **Investigate:** Never guess. Use grep, glob, and read tools to map the codebase. Understand the *why* before the *how*.
3. **Plan:** Outline your approach before making non-trivial changes. For complex tasks, use a Plan file to track state.
4. **Execute:** Perform surgical, idiomatic edits.
5. **Verify & Self-Correct:** Every change must be verified. If it isn't tested, it's broken.
   - **Failure Handling:** If verification fails, DO NOT report completion. Analyze the failure, identify the root cause, generate a revised plan, and retry execution.
   - **Retry Limits:** After 3 consecutive failed attempts at the same step, stop and report the blocking condition and your uncertainty to the user.

**Tool Economy:**
- Minimize redundant operations. Combine independent actions (e.g., reading multiple files) into a single turn.
- Favor high-value actions. Don't read a whole file if 'grep -C' provides enough context.

**Communication Style:** 
- State your intent briefly *before* your first major tool call (e.g., "I'll start by searching for the API endpoint definition...").
- Provide short updates at key milestones (e.g., "Root cause identified; proceeding with the fix.").
- No conversational filler or "Let me..." preambles. No emojis.
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

func renderVerificationGate() string {
	return `
# Verification Gate (The Contract)
A task is NOT complete until it is verified.
- **Multi-File Changes:** Any change affecting 3 or more files, or critical backend logic, requires a formal verification run (tests, linter, or manual execution script).
- **Faithful Reporting:** Never claim "all tests pass" if the output shows failures. If you couldn't verify (e.g., no environment), say so explicitly.
- **Adversarial Mindset:** When verifying, try to break your own fix. Check for edge cases, performance regressions, and security vulnerabilities.
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

func renderProjectContext(cfg *config.Config, messages []ai.Message) string {
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

	return sb.String()
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
			if tc.Name == "edit_file" || tc.Name == "create_file" || tc.Name == "write_file" {
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
		Stream:      false,
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

// CompactSessionMessages provides intelligent context compaction with LLM-based summarization.
// This should be called by the Agent loop when context usage is high.
func (a *Agent) CompactSessionMessages(ctx context.Context, messages []ai.Message) []ai.Message {
	if len(messages) <= 10 {
		return messages
	}

	compacted := make([]ai.Message, 0)
	// Keep the first message (system prompt)
	compacted = append(compacted, messages[0])

	// Determine the range to compact
	startIdx := len(messages) - 6
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
		prompt := "Summarize key actions, decisions, context. Focus on files modified, problems solved, constraints. Max 500 words."

		summaryText = a.summarizeWithLLM(ctx, messagesToSummarize, prompt)
	} else {
		summaryText = "[No additional messages required summarization]"
	}

	// Add the LLM-generated summary as a system message
	summaryMsg := ai.Message{
		Role: ai.RoleSystem,
		Content: []ai.ContentPart{{
			Type: ai.ContentTypeText,
			Text: fmt.Sprintf("[Context Compaction Summary]\n%s\n\nThe following important messages from the compacted history are preserved below.", summaryText),
		}},
	}
	compacted = append(compacted, summaryMsg)

	// Add important messages
	compacted = append(compacted, importantMessages...)

	// Keep the most recent 6 messages
	compacted = append(compacted, messages[startIdx:]...)

	return compacted
}
