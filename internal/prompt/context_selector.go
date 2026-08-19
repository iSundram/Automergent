package prompt

import (
	"context"
	"fmt"
	"os"
	"strings"

	contextpkg "github.com/iSundram/Automergent/internal/context"
)

// ContextSelector selects relevant context based on category profile and token budget.
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

// SelectContext selects relevant context for a categorized request.
func (cs *ContextSelector) SelectContext(ctx context.Context, req *CategorizedRequest) (string, error) {
	profile := GetContextProfile(req.Category)

	var parts []string
	usedTokens := 0

	// 1. Working directory context
	if profile.IncludeProjectContext {
		part := fmt.Sprintf("## Project Context\n- Working Directory: %s\n", cs.workingDir)
		partTokens := estimateTokens(part)
		if usedTokens+partTokens <= profile.TokenBudget {
			parts = append(parts, part)
			usedTokens += partTokens
		}
	}

	// 2. Working areas (always included if available)
	if len(req.WorkingAreas) > 0 {
		part := "## Working Areas\n"
		for _, f := range req.WorkingAreas {
			part += fmt.Sprintf("- %s\n", f)
		}
		partTokens := estimateTokens(part)
		if usedTokens+partTokens <= profile.TokenBudget {
			parts = append(parts, part)
			usedTokens += partTokens
		}
	}

	// 3. Select relevant files using ranking
	if profile.MaxFiles > 0 && cs.contextManager != nil {
		relevantFiles, err := cs.selectRelevantFiles(ctx, req, profile)
		if err == nil && len(relevantFiles) > 0 {
			fileContext := cs.buildFileContext(ctx, relevantFiles, profile)
			partTokens := estimateTokens(fileContext)
			if usedTokens+partTokens <= profile.TokenBudget {
				parts = append(parts, fileContext)
				usedTokens += partTokens
			}
		}
	}

	// 4. Stashed context (for resumption)
	if profile.IncludeStashedContext {
		stashed := cs.getStashedContext(profile)
		if stashed != "" {
			partTokens := estimateTokens(stashed)
			if usedTokens+partTokens <= profile.TokenBudget {
				parts = append(parts, stashed)
				usedTokens += partTokens
			}
		}
	}

	// 5. Constraints
	if len(req.ContextNeeds) > 0 {
		constraints := "## Constraints\n"
		for _, need := range req.ContextNeeds {
			if need.InjectTiming != InjectTimingDeferred {
				constraints += fmt.Sprintf("- %s: %s\n", need.Key, need.Description)
			}
		}
		partTokens := estimateTokens(constraints)
		if usedTokens+partTokens <= profile.TokenBudget {
			parts = append(parts, constraints)
			usedTokens += partTokens
		}
	}

	return strings.Join(parts, "\n"), nil
}

// selectRelevantFiles uses the ranking system to find relevant files.
func (cs *ContextSelector) selectRelevantFiles(ctx context.Context, req *CategorizedRequest, profile ContextProfile) ([]string, error) {
	// Get candidate files: recent + frequent + working areas
	var candidates []string
	seen := make(map[string]bool)

	// Add working areas first (highest priority)
	for _, f := range req.WorkingAreas {
		if !seen[f] {
			candidates = append(candidates, f)
			seen[f] = true
		}
	}

	// Add recent files
	if profile.IncludeRecentFiles {
		recent := cs.contextManager.RecentFiles(profile.MaxFiles * 2)
		for _, f := range recent {
			if !seen[f] {
				candidates = append(candidates, f)
				seen[f] = true
			}
		}
	}

	// Add frequent files
	if profile.IncludeFrequentFiles {
		frequent := cs.contextManager.FrequentFiles(profile.MaxFiles)
		for _, f := range frequent {
			if !seen[f] {
				candidates = append(candidates, f)
				seen[f] = true
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Rank files using the ranker
	scores, err := cs.contextManager.RankFiles(ctx, candidates, req.UserIntent, profile.MaxFiles)
	if err != nil {
		return nil, err
	}

	// Filter by minimum relevance score
	var result []string
	for _, score := range scores {
		if score.Score >= profile.MinRelevanceScore {
			result = append(result, score.Path)
		}
	}

	return result, nil
}

// buildFileContext builds context from selected files.
func (cs *ContextSelector) buildFileContext(ctx context.Context, files []string, profile ContextProfile) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Files\n")

	for i, f := range files {
		if i >= profile.MaxFiles {
			break
		}

		content, err := cs.readFileWithBudget(f, profile.TokenBudget/2)
		if err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n", f))
		if profile.IncludeSymbols {
			// In a real implementation, extract symbols
		}
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

	return content, nil
}

// getStashedContext retrieves relevant stashed context.
func (cs *ContextSelector) getStashedContext(profile ContextProfile) string {
	// This would retrieve from the prompt manager's stashed contexts
	// For now, return empty - will be filled by the manager
	return ""
}

// estimateTokens roughly estimates token count from text.
func estimateTokens(text string) int {
	// Rough estimate: 4 characters per token
	return len(text) / 4
}