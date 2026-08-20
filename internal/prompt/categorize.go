package prompt

import (
	"strings"
)

// Categorizer categorizes user requests into types and determines execution strategy.
type Categorizer struct {
	config *PromptConfig
}

// NewCategorizer creates a new request categorizer.
func NewCategorizer(config *PromptConfig) *Categorizer {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &Categorizer{config: config}
}

// Categorize analyzes the user prompt and returns a categorized request.
func (c *Categorizer) Categorize(prompt string, workingDir string, files []string) *CategorizedRequest {
	lowerPrompt := strings.ToLower(prompt)

	category := c.detectCategory(lowerPrompt)
	complexity := c.detectComplexity(lowerPrompt, files)
	strategy := c.determineStrategy(category, complexity)
	tools := c.determineTools(category, complexity)
	workingAreas := c.detectWorkingAreas(lowerPrompt, files)
	requiresCoder := c.requiresCoder(category, complexity)
	todoItems := c.generateInitialTodos(category, complexity, prompt)
	contextNeeds := c.detectContextNeeds(category, workingDir, files)

	return &CategorizedRequest{
		Category:       category,
		Complexity:     complexity,
		Strategy:       strategy,
		AllowedTools:   tools,
		WorkingAreas:   workingAreas,
		OriginalPrompt: prompt,
		UserIntent:     c.extractIntent(prompt),
		RequiresCoder:  requiresCoder,
		TodoItems:      todoItems,
		ContextNeeds:   contextNeeds,
	}
}

func (c *Categorizer) detectCategory(prompt string) RequestCategory {
	// Inquiry phrasing takes precedence over verbs that may occur inside the
	// question (for example, "which files implement search?").
	if containsAny(prompt, []string{
		"tell me", "which files", "what files", "where is", "how does",
		"read files", "explain",
	}) {
		if containsAny(prompt, []string{"issue", "issues", "error", "broken", "not working"}) {
			return CategoryIssueSuspect
		}
		return CategoryUserAsking
	}
	// New feature keywords
	if containsAny(prompt, []string{
		"add", "create", "implement", "build", "new feature", "new function",
		"new component", "new module", "new api", "new endpoint", "add support",
	}) {
		return CategoryNewFeature
	}

	// Issue suspect keywords (must come before debug to catch "suspect" before "issue")
	if containsAny(prompt, []string{
		"suspect", "might be", "could be", "possibly", "think it's",
		"investigate", "check if", "verify if", "look into", "tell issues",
		"find issues", "identify issues", "related files", "root cause",
	}) {
		return CategoryIssueSuspect
	}

	// Debug keywords
	if containsAny(prompt, []string{
		"debug", "fix bug", "error", "crash", "fail", "not working",
		"broken", "issue", "problem", "exception", "stack trace",
	}) {
		return CategoryDebug
	}

	// Plan keywords
	if containsAny(prompt, []string{
		"plan", "design", "architecture", "approach", "strategy",
		"how to", "roadmap", "steps to", "proposal",
	}) {
		return CategoryPlan
	}

	// Verify work keywords
	if containsAny(prompt, []string{
		"verify", "review", "check", "validate", "audit", "inspect",
		"ensure", "confirm", "test", "quality",
	}) {
		return CategoryVerifyWork
	}

	// Direct/simple keywords
	if containsAny(prompt, []string{
		"show", "list", "get", "read", "display", "print", "find",
		"search", "grep", "where is", "what is",
	}) {
		return CategoryDirect
	}

	// Simple keywords
	if containsAny(prompt, []string{
		"simple", "quick", "small", "minor", "trivial", "easy",
		"just", "only", "one line", "single",
	}) {
		return CategorySimple
	}

	return CategoryUserAsking
}

func (c *Categorizer) detectComplexity(prompt string, files []string) TaskComplexity {
	// Check for complexity indicators in prompt
	complexIndicators := []string{
		"refactor", "redesign", "migrate", "rewrite", "overhaul",
		"multiple", "many", "several", "complex", "large", "extensive",
		"integration", "cross-cutting", "system-wide", "architectural",
	}

	moderateIndicators := []string{
		"add", "create", "implement", "modify", "update", "change",
		"new", "feature", "function", "component", "module", "api",
		"read files", "related files", "search", "grep", "analyze",
		"issues", "inspect", "understand", "compare",
	}

	if containsAny(prompt, complexIndicators) || len(files) > 10 {
		return ComplexityComplex
	}
	if containsAny(prompt, moderateIndicators) || len(files) > 3 {
		return ComplexityModerate
	}
	return ComplexitySimple
}

func (c *Categorizer) determineStrategy(category RequestCategory, complexity TaskComplexity) ExecutionStrategy {
	if complexity == ComplexityComplex {
		return StrategyParallel
	}
	if complexity == ComplexityModerate {
		return StrategyTodoWalkthrough
	}
	if category == CategoryNewFeature || category == CategoryDebug {
		return StrategyCoderAgent
	}
	return StrategyDirect
}

func (c *Categorizer) determineTools(category RequestCategory, complexity TaskComplexity) ToolSet {
	if category == CategorySimple || category == CategoryDirect {
		return ToolSetReadOnly
	}
	if complexity == ComplexityComplex {
		return ToolSetFull
	}
	if complexity == ComplexityModerate {
		return ToolSetModerate
	}
	return ToolSetBasic
}

func (c *Categorizer) detectWorkingAreas(prompt string, files []string) []string {
	// In a real implementation, this would use codebase analysis
	// For now, return the provided files as working areas
	return files
}

func (c *Categorizer) requiresCoder(category RequestCategory, complexity TaskComplexity) bool {
	return category == CategoryNewFeature ||
		category == CategoryDebug ||
		complexity == ComplexityModerate ||
		complexity == ComplexityComplex
}

func (c *Categorizer) extractIntent(prompt string) string {
	// Extract the core intent from the prompt
	// This is a simplified version - in practice would use NLP
	return prompt
}

func (c *Categorizer) generateInitialTodos(category RequestCategory, complexity TaskComplexity, prompt string) []TodoItem {
	var todos []TodoItem

	switch category {
	case CategoryNewFeature:
		todos = append(todos,
			TodoItem{ID: "todo-1", Description: "Analyze requirements and design approach", Status: TodoStatusPending, Priority: 1},
			TodoItem{ID: "todo-2", Description: "Identify working areas and dependencies", Status: TodoStatusPending, Priority: 2},
			TodoItem{ID: "todo-3", Description: "Implement core functionality", Status: TodoStatusPending, Priority: 3, Tools: []string{"edit", "write", "bash"}},
			TodoItem{ID: "todo-4", Description: "Write tests", Status: TodoStatusPending, Priority: 4, Tools: []string{"write", "bash"}},
			TodoItem{ID: "todo-5", Description: "Verify implementation", Status: TodoStatusPending, Priority: 5, Tools: []string{"bash", "read"}},
		)
	case CategoryDebug:
		todos = append(todos,
			TodoItem{ID: "todo-1", Description: "Reproduce and understand the issue", Status: TodoStatusPending, Priority: 1, Tools: []string{"read", "bash"}},
			TodoItem{ID: "todo-2", Description: "Identify root cause", Status: TodoStatusPending, Priority: 2, Tools: []string{"read", "search"}},
			TodoItem{ID: "todo-3", Description: "Implement fix", Status: TodoStatusPending, Priority: 3, Tools: []string{"edit", "write"}},
			TodoItem{ID: "todo-4", Description: "Verify fix works", Status: TodoStatusPending, Priority: 4, Tools: []string{"bash", "read"}},
		)
	case CategoryPlan:
		todos = append(todos,
			TodoItem{ID: "todo-1", Description: "Gather requirements and constraints", Status: TodoStatusPending, Priority: 1},
			TodoItem{ID: "todo-2", Description: "Research existing patterns and solutions", Status: TodoStatusPending, Priority: 2, Tools: []string{"read", "search"}},
			TodoItem{ID: "todo-3", Description: "Create detailed plan", Status: TodoStatusPending, Priority: 3},
		)
	default:
		todos = append(todos,
			TodoItem{ID: "todo-1", Description: "Understand and address request", Status: TodoStatusPending, Priority: 1},
		)
	}

	// Adjust for complexity
	if complexity == ComplexitySimple {
		// Keep only first todo for simple tasks
		if len(todos) > 1 {
			todos = todos[:1]
		}
	}

	return todos
}

func (c *Categorizer) detectContextNeeds(category RequestCategory, workingDir string, files []string) []ContextNeed {
	var needs []ContextNeed

	// Always need working directory context
	needs = append(needs, ContextNeed{
		Key:          "working_dir",
		Description:  "Current working directory",
		Required:     true,
		Source:       ContextSourceWorkingDir,
		InjectTiming: InjectTimingImmediate,
	})

	// Need user prompt context
	needs = append(needs, ContextNeed{
		Key:          "user_prompt",
		Description:  "Original user request",
		Required:     true,
		Source:       ContextSourceUserPrompt,
		InjectTiming: InjectTimingImmediate,
	})

	// Category-specific context needs
	switch category {
	case CategoryNewFeature:
		needs = append(needs,
			ContextNeed{Key: "codebase_patterns", Description: "Existing code patterns to follow", Required: true, Source: ContextSourceCodebase, InjectTiming: InjectTimingDeferred},
			ContextNeed{Key: "dependencies", Description: "Project dependencies and imports", Required: true, Source: ContextSourceCodebase, InjectTiming: InjectTimingDeferred},
		)
	case CategoryDebug:
		needs = append(needs,
			ContextNeed{Key: "error_logs", Description: "Error logs and stack traces", Required: true, Source: ContextSourceCodebase, InjectTiming: InjectTimingImmediate},
			ContextNeed{Key: "related_code", Description: "Code related to the issue", Required: true, Source: ContextSourceCodebase, InjectTiming: InjectTimingDeferred},
		)
	case CategoryVerifyWork:
		needs = append(needs,
			ContextNeed{Key: "code_to_review", Description: "Code that needs review", Required: true, Source: ContextSourceCodebase, InjectTiming: InjectTimingImmediate},
		)
	}

	// Add file-specific context needs
	for _, f := range files {
		needs = append(needs, ContextNeed{
			Key:          "file:" + f,
			Description:  "Content of " + f,
			Required:     false,
			Source:       ContextSourceCodebase,
			InjectTiming: InjectTimingOnDemand,
		})
	}

	return needs
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
