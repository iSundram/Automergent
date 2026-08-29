package context

import (
	"fmt"

	"github.com/iSundram/Automergent/internal/shared"
)

// PhaseBudgetConfig holds token budget allocation for a phase.
type PhaseBudgetConfig struct {
	SystemPrompt int
	Messages     int
	Tools        int
	Files        int
	Buffer       int
}

// ContextWindow represents the assembled context for a model request.
type ContextWindow struct {
	SystemPrompt string
	Messages     []shared.Message
	ToolResults  []ToolResult
	FileContext  []AssemblyFileContext
	WorkingSet   []string
	TokenUsage   TokenUsage
	Phase        shared.AgentPhase
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	SystemPrompt int
	Messages     int
	Tools        int
	Files        int
	Total        int
	Limit        int
}

// ToolResult represents a tool execution result for context.
type ToolResult struct {
	ToolName   string
	CallID     string
	Output     string
	IsError    bool
	Tokens     int
	Truncated  bool
}

// FileContext represents a file included in context (assembly-specific, lighter weight).
type AssemblyFileContext struct {
	Path     string
	Content  string
	Tokens   int
	Priority float64
	Required bool
}

// ContextAssembler builds the context window for each phase.
type ContextAssembler struct {
	manager      *Manager
	budget       *TokenBudget
	ranker       *Ranker
	phaseBudgets map[shared.AgentPhase]PhaseBudgetConfig
}

// NewContextAssembler creates a new context assembler.
func NewContextAssembler(manager *Manager, budget *TokenBudget, ranker *Ranker) *ContextAssembler {
	return &ContextAssembler{
		manager:    manager,
		budget:     budget,
		ranker:     ranker,
		phaseBudgets: map[shared.AgentPhase]PhaseBudgetConfig{
			shared.PhaseInit: {
				SystemPrompt: 8000,
				Messages:     15000,
				Tools:        5000,
				Files:        10000,
				Buffer:       5000,
			},
			shared.PhaseExplore: {
				SystemPrompt: 8000,
				Messages:     20000,
				Tools:        15000,
				Files:        30000,
				Buffer:       10000,
			},
			shared.PhasePlan: {
				SystemPrompt: 8000,
				Messages:     25000,
				Tools:        10000,
				Files:        25000,
				Buffer:       10000,
			},
			shared.PhaseBuild: {
				SystemPrompt: 8000,
				Messages:     30000,
				Tools:        20000,
				Files:        35000,
				Buffer:       15000,
			},
		},
	}
}

// Assemble builds the context window for a phase.
func (a *ContextAssembler) Assemble(
	session *Session,
	phase shared.AgentPhase,
	model shared.ModelInfo,
	agentConfig *AgentConfig,
) (*ContextWindow, error) {
	budget := a.phaseBudgets[phase]
	modelLimit := model.ContextWindow
	if modelLimit <= 0 {
		modelLimit = 128000 // Default
	}
	
	// 1. System prompt will be composed by PromptComposer
	systemPrompt := "" // Will be filled by caller
	
	// 2. Rank and truncate messages
	messages := a.rankAndTruncateMessages(session.Messages, budget.Messages, phase)
	
	// 3. Get recent tool results
	toolResults := a.getRecentToolResults(session, budget.Tools)
	
	// 4. Get file context
	fileContext := a.getFileContext(session, budget.Files, phase)
	
	// 5. Get working set
	workingSet := a.getWorkingSet(session)
	
	// 6. Calculate token usage
	usage := a.calculateUsage(systemPrompt, messages, toolResults, fileContext, modelLimit, budget)
	
	return &ContextWindow{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		ToolResults:  toolResults,
		FileContext:  fileContext,
		WorkingSet:   workingSet,
		TokenUsage:   usage,
		Phase:        phase,
	}, nil
}

// rankAndTruncateMessages ranks messages by relevance and truncates to budget.
func (a *ContextAssembler) rankAndTruncateMessages(messages []shared.Message, budget int, phase shared.AgentPhase) []shared.Message {
	if len(messages) == 0 {
		return messages
	}
	
	// Estimate tokens per message
	type msgWithTokens struct {
		msg    shared.Message
		tokens int
		score  float64
	}
	
	var msgTokens []msgWithTokens
	totalTokens := 0
	for i, msg := range messages {
		tokens := a.estimateMessageTokens(msg)
		// Simple relevance scoring: recent messages and user messages with tool results score higher
		score := float64(len(messages) - i) // Recency score
		if msg.Role == "user" {
			score += 10
		}
		if msg.Role == "tool" {
			score += 5
		}
		msgTokens = append(msgTokens, msgWithTokens{msg, tokens, score})
		totalTokens += tokens
	}
	
	// If under budget, return all
	if totalTokens <= budget {
		return messages
	}
	
	// Sort by score (descending) then by recency
	for i := 0; i < len(msgTokens)-1; i++ {
		for j := i + 1; j < len(msgTokens); j++ {
			if msgTokens[i].score < msgTokens[j].score {
				msgTokens[i], msgTokens[j] = msgTokens[j], msgTokens[i]
			}
		}
	}
	
	// Truncate to budget
	var result []shared.Message
	currentTokens := 0
	for _, rt := range msgTokens {
		if currentTokens+rt.tokens > budget {
			break
		}
		result = append(result, rt.msg)
		currentTokens += rt.tokens
	}
	
	// Reverse to maintain chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	
	return result
}

// getRecentToolResults returns recent tool results within budget.
func (a *ContextAssembler) getRecentToolResults(session *Session, budget int) []ToolResult {
	var results []ToolResult
	// This would pull from session's tool history
	// For now return empty - integration needed
	return results
}

// getFileContext returns relevant file context within budget.
func (a *ContextAssembler) getFileContext(session *Session, budget int, phase shared.AgentPhase) []AssemblyFileContext {
	var files []AssemblyFileContext
	
	// Get active/read files from session
	// This integrates with the context manager
	// For now return empty - integration needed
	
	return files
}

// getWorkingSet returns the current working set of files.
func (a *ContextAssembler) getWorkingSet(session *Session) []string {
	// Return files currently being worked on
	return session.ActiveFiles
}

// estimateMessageTokens estimates tokens for a message.
func (a *ContextAssembler) estimateMessageTokens(msg shared.Message) int {
	// Rough estimation: ~4 chars per token
	content := msg.Content
	if msg.Metadata != nil {
		for k, v := range msg.Metadata {
			content += k + fmt.Sprintf("%v", v)
		}
	}
	return len(content) / 4
}

// calculateUsage calculates token usage.
func (a *ContextAssembler) calculateUsage(
	systemPrompt string,
	messages []shared.Message,
	toolResults []ToolResult,
	fileContext []AssemblyFileContext,
	modelLimit int,
	budget PhaseBudgetConfig,
) TokenUsage {
	usage := TokenUsage{
		SystemPrompt: len(systemPrompt) / 4,
		Limit:        modelLimit,
	}
	
	for _, msg := range messages {
		usage.Messages += a.estimateMessageTokens(msg)
	}
	
	for _, tr := range toolResults {
		usage.Tools += tr.Tokens
	}
	
	for _, fc := range fileContext {
		usage.Files += fc.Tokens
	}
	
	usage.Total = usage.SystemPrompt + usage.Messages + usage.Tools + usage.Files
	return usage
}

// AgentConfig holds agent-specific configuration for context.
type AgentConfig struct {
	Name           string
	PhaseTools     map[shared.AgentPhase][]string
	PhaseMaxSteps  map[shared.AgentPhase]int
	ViolationPolicy shared.ViolationPolicy
}

// Session represents an agent session (simplified for context assembly).
type Session struct {
	Messages    []shared.Message
	ActiveFiles []string
	ToolHistory []ToolResult
}