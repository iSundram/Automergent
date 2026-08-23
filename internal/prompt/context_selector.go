package prompt

import (
	"context"
	"fmt"
	"os"
	"strings"

	contextpkg "github.com/iSundram/Automergent/internal/context"
)

// ContextSelector selects relevant context based on task specs and token budget.
type ContextSelector struct {
	contextManager *contextpkg.Manager
	workingDir     string
	config         *PromptConfig
}

// NewContextSelector creates a new context selector.
func NewContextSelector(mgr *contextpkg.Manager, workingDir string, config *PromptConfig) *ContextSelector {
	return &ContextSelector{
		contextManager: mgr,
		workingDir:     workingDir,
		config:         config,
	}
}

// SelectContextForTasks selects relevant context for a set of task specs.
func (cs *ContextSelector) SelectContextForTasks(ctx context.Context, tasks []TaskSpec, workingDir string, tokenBudget int) (string, error) {
	var parts []string
	usedTokens := 0

	// 1. Working directory context
	part := fmt.Sprintf("## Project Context\n- Working Directory: %s\n", workingDir)
	partTokens := estimateTokens(part)
	if usedTokens+partTokens <= tokenBudget {
		parts = append(parts, part)
		usedTokens += partTokens
	}

	// 2. Collect all files from tasks
	var allFiles []string
	seen := make(map[string]bool)
	for _, task := range tasks {
		if files, ok := task.Context["files"].([]string); ok {
			for _, f := range files {
				if !seen[f] {
					allFiles = append(allFiles, f)
					seen[f] = true
				}
			}
		}
		if files, ok := task.Context["files_found"].([]string); ok {
			for _, f := range files {
				if !seen[f] {
					allFiles = append(allFiles, f)
					seen[f] = true
				}
			}
		}
	}

	if len(allFiles) > 0 {
		part := "## Relevant Files (from tasks)\n"
		for _, f := range allFiles {
			part += fmt.Sprintf("- %s\n", f)
		}
		partTokens := estimateTokens(part)
		if usedTokens+partTokens <= tokenBudget {
			parts = append(parts, part)
			usedTokens += partTokens
		}
	}

	// 3. Select relevant files using ranking (if context manager available)
	if len(allFiles) > 0 && cs.contextManager != nil {
		relevantFiles, err := cs.selectRelevantFilesFromList(ctx, allFiles, tokenBudget-usedTokens)
		if err == nil && len(relevantFiles) > 0 {
			fileContext := cs.buildFileContext(ctx, relevantFiles, tokenBudget-usedTokens)
			partTokens := estimateTokens(fileContext)
			if usedTokens+partTokens <= tokenBudget {
				parts = append(parts, fileContext)
				usedTokens += partTokens
			}
		}
	}

	// 4. Code snippets from init results
	var allSnippets map[string]string
	for _, task := range tasks {
		if snippets, ok := task.Context["code_snippets"].(map[string]string); ok {
			if allSnippets == nil {
				allSnippets = make(map[string]string)
			}
			for k, v := range snippets {
				allSnippets[k] = v
			}
		}
	}

	if len(allSnippets) > 0 {
		var sb strings.Builder
		sb.WriteString("## Code Snippets\n")
		for path, code := range allSnippets {
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", path, code))
		}
		snippetContext := sb.String()
		partTokens := estimateTokens(snippetContext)
		if usedTokens+partTokens <= tokenBudget {
			parts = append(parts, snippetContext)
			usedTokens += partTokens
		}
	}

	// 5. Constraints from tasks
	var allConstraints []string
	for _, task := range tasks {
		if constraints, ok := task.Context["constraints"].([]string); ok {
			allConstraints = append(allConstraints, constraints...)
		}
	}

	if len(allConstraints) > 0 {
		constraints := "## Constraints\n"
		for _, c := range allConstraints {
			constraints += fmt.Sprintf("- %s\n", c)
		}
		partTokens := estimateTokens(constraints)
		if usedTokens+partTokens <= tokenBudget {
			parts = append(parts, constraints)
			usedTokens += partTokens
		}
	}

	return strings.Join(parts, "\n"), nil
}

// selectRelevantFilesFromList uses the ranking system to find relevant files from a given list.
func (cs *ContextSelector) selectRelevantFilesFromList(ctx context.Context, candidates []string, maxTokens int) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// Use a generic intent for ranking
	intent := "understand codebase structure and implementation details"

	// Rank files using the ranker
	scores, err := cs.contextManager.RankFiles(ctx, candidates, intent, len(candidates))
	if err != nil {
		return nil, err
	}

	// Filter by minimum relevance score and token budget
	var result []string
	usedTokens := 0
	for _, score := range scores {
		if score.Score >= 0.3 { // MinRelevanceScore
			// Estimate tokens for this file
			content, err := cs.readFileWithBudget(score.Path, maxTokens-usedTokens)
			if err != nil {
				continue
			}
			fileTokens := estimateTokens(content)
			if usedTokens+fileTokens <= maxTokens {
				result = append(result, score.Path)
				usedTokens += fileTokens
			}
		}
	}

	return result, nil
}

// buildFileContext builds context from selected files.
func (cs *ContextSelector) buildFileContext(ctx context.Context, files []string, tokenBudget int) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Files\n")

	for i, f := range files {
		if i >= 15 { // MaxFiles
			break
		}

		content, err := cs.readFileWithBudget(f, tokenBudget/2)
		if err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n", f))
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// readFileWithBudget reads a file with token budget consideration.
func (cs *ContextSelector) readFileWithBudget(path string, maxTokens int) (string, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(contentBytes)

	// Truncate if exceeds budget (rough estimate: 4 chars per token)
	maxChars := maxTokens * 4
	if len(content) > maxChars {
		content = content[:maxChars] + "\n... [truncated]"
	}

	return content, nil}

// estimateTokens roughly estimates token count from text.
func estimateTokens(text string) int {
	// Rough estimate: 4 characters per token
	return len(text) / 4
}