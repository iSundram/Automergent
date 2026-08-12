package errors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// AI Error Context
// ══════════════════════════════════════════════════════════════════════════════

// AIErrorContext contains structured information for AI-powered error analysis.
type AIErrorContext struct {
	// Error information
	Code      ErrorCode      `json:"code"`
	Category  Category       `json:"category"`
	Message   string         `json:"message"`
	Operation string         `json:"operation,omitempty"`
	Resource  string         `json:"resource,omitempty"`
	Cause     string         `json:"cause,omitempty"`
	Context   map[string]any `json:"context,omitempty"`

	// Environment context
	WorkingDir string `json:"working_dir,omitempty"`
	GitBranch  string `json:"git_branch,omitempty"`
	Language   string `json:"language,omitempty"`
	Framework  string `json:"framework,omitempty"`
	Runtime    string `json:"runtime,omitempty"`

	// Historical context
	SimilarErrors   []ErrorSummary `json:"similar_errors,omitempty"`
	RecentActions   []string       `json:"recent_actions,omitempty"`
	ResolutionHints []string       `json:"resolution_hints,omitempty"`
}

// ErrorSummary provides a brief summary of a past error.
type ErrorSummary struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Resolution string    `json:"resolution,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// PrepareForAI converts an AutomergentError to an AIErrorContext.
func PrepareForAI(err *AutomergentError, env *EnvironmentContext) AIErrorContext {
	ctx := AIErrorContext{
		Code:      err.Code,
		Category:  err.Category,
		Message:   err.Message,
		Operation: err.Operation,
		Resource:  err.Resource,
		Context:   err.Context,
	}

	if err.Err != nil {
		ctx.Cause = err.Err.Error()
	}

	if env != nil {
		ctx.WorkingDir = env.WorkingDir
		ctx.GitBranch = env.GitBranch
		ctx.Language = env.Language
		ctx.Framework = env.Framework
		ctx.Runtime = env.Runtime
	}

	return ctx
}

// ToJSON converts the context to a JSON string for AI consumption.
func (c AIErrorContext) ToJSON() string {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal context: %s"}`, err)
	}
	return string(data)
}

// ToPrompt generates a prompt section for AI error analysis.
func (c AIErrorContext) ToPrompt() string {
	var b strings.Builder

	b.WriteString("## Error Information\n\n")
	b.WriteString(fmt.Sprintf("**Error Code:** %s\n", c.Code))
	b.WriteString(fmt.Sprintf("**Category:** %s\n", c.Category))
	b.WriteString(fmt.Sprintf("**Message:** %s\n", c.Message))

	if c.Operation != "" {
		b.WriteString(fmt.Sprintf("**Operation:** %s\n", c.Operation))
	}
	if c.Resource != "" {
		b.WriteString(fmt.Sprintf("**Resource:** %s\n", c.Resource))
	}
	if c.Cause != "" {
		b.WriteString(fmt.Sprintf("**Underlying Cause:** %s\n", c.Cause))
	}

	if len(c.Context) > 0 {
		b.WriteString("\n### Additional Context\n\n")
		for k, v := range c.Context {
			b.WriteString(fmt.Sprintf("- **%s:** %v\n", k, v))
		}
	}

	if c.WorkingDir != "" || c.Language != "" {
		b.WriteString("\n### Environment\n\n")
		if c.WorkingDir != "" {
			b.WriteString(fmt.Sprintf("- **Working Directory:** %s\n", c.WorkingDir))
		}
		if c.GitBranch != "" {
			b.WriteString(fmt.Sprintf("- **Git Branch:** %s\n", c.GitBranch))
		}
		if c.Language != "" {
			b.WriteString(fmt.Sprintf("- **Language:** %s\n", c.Language))
		}
		if c.Framework != "" {
			b.WriteString(fmt.Sprintf("- **Framework:** %s\n", c.Framework))
		}
		if c.Runtime != "" {
			b.WriteString(fmt.Sprintf("- **Runtime:** %s\n", c.Runtime))
		}
	}

	if len(c.SimilarErrors) > 0 {
		b.WriteString("\n### Similar Past Errors\n\n")
		for _, e := range c.SimilarErrors {
			b.WriteString(fmt.Sprintf("- [%s] %s", e.Code, e.Message))
			if e.Resolution != "" {
				b.WriteString(fmt.Sprintf(" (resolved by: %s)", e.Resolution))
			}
			b.WriteString("\n")
		}
	}

	if len(c.RecentActions) > 0 {
		b.WriteString("\n### Recent Actions\n\n")
		for i, action := range c.RecentActions {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, action))
		}
	}

	if len(c.ResolutionHints) > 0 {
		b.WriteString("\n### Known Resolution Hints\n\n")
		for _, hint := range c.ResolutionHints {
			b.WriteString(fmt.Sprintf("- %s\n", hint))
		}
	}

	return b.String()
}

// EnvironmentContext captures the current environment for error analysis.
type EnvironmentContext struct {
	WorkingDir string
	GitBranch  string
	Language   string
	Framework  string
	Runtime    string
}

// ══════════════════════════════════════════════════════════════════════════════
// AI Fix Suggestions
// ══════════════════════════════════════════════════════════════════════════════

// FixSuggestion represents an AI-generated fix suggestion.
type FixSuggestion struct {
	// Summary is a brief description of the fix
	Summary string `json:"summary"`

	// Explanation provides detailed reasoning
	Explanation string `json:"explanation,omitempty"`

	// Steps are the concrete actions to take
	Steps []FixStep `json:"steps,omitempty"`

	// Code is an optional code snippet to apply
	Code string `json:"code,omitempty"`

	// Confidence indicates how confident the AI is (0.0 to 1.0)
	Confidence float64 `json:"confidence"`

	// Impact describes the expected impact of the fix
	Impact string `json:"impact,omitempty"`

	// Risks describes potential risks of the fix
	Risks []string `json:"risks,omitempty"`
}

// FixStep represents a single step in a fix suggestion.
type FixStep struct {
	Order       int    `json:"order"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	CodeChange  string `json:"code_change,omitempty"`
}

// SuggestionGenerator is a function that generates fix suggestions.
type SuggestionGenerator func(ctx context.Context, errCtx AIErrorContext) ([]FixSuggestion, error)

// ══════════════════════════════════════════════════════════════════════════════
// Built-in Suggestions
// ══════════════════════════════════════════════════════════════════════════════

// builtInSuggestions maps error codes to known fix suggestions.
var builtInSuggestions = map[ErrorCode][]FixSuggestion{
	CodeFileNotFound: {
		{
			Summary:    "Verify the file path is correct",
			Confidence: 0.9,
			Steps: []FixStep{
				{Order: 1, Description: "Check for typos in the file path"},
				{Order: 2, Description: "Verify the file exists at the specified location"},
				{Order: 3, Description: "Check file permissions if the file should exist"},
			},
		},
	},
	CodePermissionDenied: {
		{
			Summary:    "Fix file permissions",
			Confidence: 0.85,
			Steps: []FixStep{
				{Order: 1, Description: "Check current permissions", Command: "ls -la <file>"},
				{Order: 2, Description: "Update permissions if needed", Command: "chmod u+rw <file>"},
				{Order: 3, Description: "Check file ownership and change if needed"},
			},
		},
	},
	CodeRateLimited: {
		{
			Summary:    "Wait and retry with backoff",
			Confidence: 0.95,
			Steps: []FixStep{
				{Order: 1, Description: "Wait for the suggested retry delay"},
				{Order: 2, Description: "Implement exponential backoff for retries"},
				{Order: 3, Description: "Consider caching responses to reduce API calls"},
			},
		},
	},
	CodeUnauthorized: {
		{
			Summary:    "Check authentication credentials",
			Confidence: 0.9,
			Steps: []FixStep{
				{Order: 1, Description: "Verify your API key is correct and not expired"},
				{Order: 2, Description: "Check environment variables are properly set"},
				{Order: 3, Description: "Regenerate credentials if needed"},
			},
		},
	},
	CodeConfigNotFound: {
		{
			Summary:    "Create or locate configuration file",
			Confidence: 0.85,
			Steps: []FixStep{
				{Order: 1, Description: "Check for configuration file in standard locations"},
				{Order: 2, Description: "Create a new configuration file if missing"},
				{Order: 3, Description: "Copy from template if available"},
			},
		},
	},
	CodeMissingEnvVar: {
		{
			Summary:    "Set the required environment variable",
			Confidence: 0.95,
			Steps: []FixStep{
				{Order: 1, Description: "Identify the required value for the variable"},
				{Order: 2, Description: "Set the variable in your shell", Command: "export VAR_NAME=value"},
				{Order: 3, Description: "Consider adding to .env or shell profile"},
			},
		},
	},
	CodeContextTooLong: {
		{
			Summary:    "Reduce context size",
			Confidence: 0.8,
			Steps: []FixStep{
				{Order: 1, Description: "Remove unnecessary context or files"},
				{Order: 2, Description: "Summarize long content before including"},
				{Order: 3, Description: "Use a model with larger context if available"},
			},
		},
	},
	CodeGitConflict: {
		{
			Summary:    "Resolve merge conflicts",
			Confidence: 0.9,
			Steps: []FixStep{
				{Order: 1, Description: "List conflicted files", Command: "git diff --name-only --diff-filter=U"},
				{Order: 2, Description: "Open each file and resolve conflict markers"},
				{Order: 3, Description: "Stage resolved files", Command: "git add <resolved_files>"},
				{Order: 4, Description: "Complete the merge", Command: "git commit"},
			},
		},
	},
	CodeCommandNotFound: {
		{
			Summary:    "Install the missing command",
			Confidence: 0.85,
			Steps: []FixStep{
				{Order: 1, Description: "Check if the command is installed", Command: "which <command>"},
				{Order: 2, Description: "Install using package manager"},
				{Order: 3, Description: "Verify PATH includes the installation directory"},
			},
		},
	},
}

// GetBuiltInSuggestions returns built-in suggestions for an error code.
func GetBuiltInSuggestions(code ErrorCode) []FixSuggestion {
	if suggestions, ok := builtInSuggestions[code]; ok {
		return suggestions
	}
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Error Learning System
// ══════════════════════════════════════════════════════════════════════════════

// ErrorResolution records how an error was resolved.
type ErrorResolution struct {
	ErrorCode    ErrorCode      `json:"error_code"`
	ErrorMessage string         `json:"error_message"`
	Operation    string         `json:"operation,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	Resolution   string         `json:"resolution"`
	Timestamp    time.Time      `json:"timestamp"`
	Successful   bool           `json:"successful"`
}

// ErrorLearner tracks error resolutions for learning.
type ErrorLearner struct {
	mu          sync.RWMutex
	resolutions []ErrorResolution
	maxHistory  int
}

// NewErrorLearner creates a new error learner.
func NewErrorLearner(maxHistory int) *ErrorLearner {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	return &ErrorLearner{
		resolutions: make([]ErrorResolution, 0, maxHistory),
		maxHistory:  maxHistory,
	}
}

// RecordResolution records how an error was resolved.
func (l *ErrorLearner) RecordResolution(err *AutomergentError, resolution string, successful bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	res := ErrorResolution{
		ErrorCode:    err.Code,
		ErrorMessage: err.Message,
		Operation:    err.Operation,
		Context:      err.Context,
		Resolution:   resolution,
		Timestamp:    time.Now(),
		Successful:   successful,
	}

	l.resolutions = append(l.resolutions, res)

	// Trim if exceeds max
	if len(l.resolutions) > l.maxHistory {
		l.resolutions = l.resolutions[len(l.resolutions)-l.maxHistory:]
	}
}

// FindSimilar finds similar past errors and their resolutions.
func (l *ErrorLearner) FindSimilar(err *AutomergentError, limit int) []ErrorSummary {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var similar []ErrorSummary
	for i := len(l.resolutions) - 1; i >= 0 && len(similar) < limit; i-- {
		res := l.resolutions[i]
		if res.ErrorCode == err.Code && res.Successful {
			similar = append(similar, ErrorSummary{
				Code:       res.ErrorCode,
				Message:    res.ErrorMessage,
				Resolution: res.Resolution,
				Timestamp:  res.Timestamp,
			})
		}
	}

	return similar
}

// GetResolutionHints returns hints based on past successful resolutions.
func (l *ErrorLearner) GetResolutionHints(code ErrorCode) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	hints := make(map[string]int)
	for _, res := range l.resolutions {
		if res.ErrorCode == code && res.Successful {
			hints[res.Resolution]++
		}
	}

	// Sort by frequency
	var result []string
	for hint := range hints {
		result = append(result, hint)
	}

	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// AI Explanation Generator
// ══════════════════════════════════════════════════════════════════════════════

// AIExplanation represents an AI-generated explanation.
type AIExplanation struct {
	Summary     string   `json:"summary"`
	Details     string   `json:"details"`
	RootCause   string   `json:"root_cause,omitempty"`
	Impact      string   `json:"impact,omitempty"`
	Prevention  string   `json:"prevention,omitempty"`
	RelatedDocs []string `json:"related_docs,omitempty"`
}

// ExplanationGenerator generates AI explanations for errors.
type ExplanationGenerator interface {
	Explain(ctx context.Context, errCtx AIErrorContext) (*AIExplanation, error)
	SuggestFixes(ctx context.Context, errCtx AIErrorContext) ([]FixSuggestion, error)
}

// DefaultExplanationGenerator provides built-in explanations without AI.
type DefaultExplanationGenerator struct {
	learner *ErrorLearner
}

// NewDefaultExplanationGenerator creates a new default generator.
func NewDefaultExplanationGenerator(learner *ErrorLearner) *DefaultExplanationGenerator {
	return &DefaultExplanationGenerator{learner: learner}
}

// Explain generates an explanation based on error code and context.
func (g *DefaultExplanationGenerator) Explain(_ context.Context, errCtx AIErrorContext) (*AIExplanation, error) {
	explanation := &AIExplanation{
		Summary: errCtx.Message,
	}

	// Add category-specific details
	switch errCtx.Category {
	case CategoryValidation:
		explanation.Details = "A validation error occurred while processing input data."
		explanation.Prevention = "Ensure input data meets the expected format and constraints before processing."

	case CategoryIO:
		explanation.Details = "An I/O operation failed while reading from or writing to the filesystem."
		explanation.Prevention = "Verify file paths, permissions, and available disk space before I/O operations."

	case CategoryNetwork:
		explanation.Details = "A network-related error occurred while communicating with an external service."
		explanation.Prevention = "Implement retry logic with exponential backoff for network operations."

	case CategoryAPI:
		explanation.Details = "An API request failed due to server-side issues or request limits."
		explanation.Prevention = "Implement rate limiting, caching, and proper error handling for API calls."

	case CategoryAuth:
		explanation.Details = "An authentication or authorization error occurred."
		explanation.Prevention = "Ensure credentials are valid and properly configured. Implement token refresh for expiring credentials."

	case CategoryConfig:
		explanation.Details = "A configuration error was detected."
		explanation.Prevention = "Validate configuration files at startup. Provide clear error messages for missing or invalid settings."

	case CategoryGit:
		explanation.Details = "A Git operation failed."
		explanation.Prevention = "Ensure clean working directory and resolve conflicts before Git operations."

	case CategoryAI:
		explanation.Details = "An error occurred while interacting with the AI provider."
		explanation.Prevention = "Handle rate limits gracefully and validate context length before requests."

	default:
		explanation.Details = "An unexpected error occurred."
		explanation.Prevention = "Review error logs and stack traces for debugging information."
	}

	// Add root cause analysis based on error code
	explanation.RootCause = getRootCause(errCtx.Code)

	return explanation, nil
}

// SuggestFixes generates fix suggestions based on built-in knowledge and past resolutions.
func (g *DefaultExplanationGenerator) SuggestFixes(_ context.Context, errCtx AIErrorContext) ([]FixSuggestion, error) {
	var suggestions []FixSuggestion

	// Get built-in suggestions
	if builtIn := GetBuiltInSuggestions(errCtx.Code); len(builtIn) > 0 {
		suggestions = append(suggestions, builtIn...)
	}

	// Add suggestions from error learner
	if g.learner != nil {
		hints := g.learner.GetResolutionHints(errCtx.Code)
		for _, hint := range hints {
			suggestions = append(suggestions, FixSuggestion{
				Summary:    hint,
				Confidence: 0.7, // Lower confidence for learned suggestions
			})
		}
	}

	return suggestions, nil
}

// getRootCause returns a likely root cause based on error code.
func getRootCause(code ErrorCode) string {
	causes := map[ErrorCode]string{
		CodeFileNotFound:       "The specified file does not exist at the given path",
		CodePermissionDenied:   "The process lacks the necessary permissions for the requested operation",
		CodeConnectionFailed:   "Unable to establish a network connection to the target host",
		CodeConnectionTimeout:  "The connection attempt exceeded the maximum allowed time",
		CodeRateLimited:        "Too many requests were made in a short period",
		CodeUnauthorized:       "The provided credentials are invalid or missing",
		CodeConfigNotFound:     "The required configuration file was not found in expected locations",
		CodeMissingEnvVar:      "A required environment variable is not set",
		CodeContextTooLong:     "The input context exceeds the model's maximum token limit",
		CodeGitConflict:        "Changes in the working directory conflict with the target branch",
		CodeCommandNotFound:    "The specified command is not installed or not in PATH",
		CodeTokenExpired:       "The authentication token has exceeded its validity period",
		CodeDiskFull:           "The storage device has no remaining free space",
		CodeServiceUnavailable: "The target service is temporarily unavailable",
	}

	if cause, ok := causes[code]; ok {
		return cause
	}
	return "The exact root cause could not be determined"
}

// ══════════════════════════════════════════════════════════════════════════════
// Integration Helpers
// ══════════════════════════════════════════════════════════════════════════════

// EnrichWithAI adds AI-generated suggestions to an error.
func EnrichWithAI(err *AutomergentError, generator ExplanationGenerator, env *EnvironmentContext) *AutomergentError {
	if err == nil || generator == nil {
		return err
	}

	ctx := context.Background()
	errCtx := PrepareForAI(err, env)

	// Get explanation
	explanation, explainErr := generator.Explain(ctx, errCtx)
	if explainErr == nil && explanation != nil {
		if err.Context == nil {
			err.Context = make(map[string]any)
		}
		err.Context["ai_explanation"] = explanation
	}

	// Get suggestions
	suggestions, suggestErr := generator.SuggestFixes(ctx, errCtx)
	if suggestErr == nil && len(suggestions) > 0 {
		// Use the highest confidence suggestion
		best := suggestions[0]
		for _, s := range suggestions[1:] {
			if s.Confidence > best.Confidence {
				best = s
			}
		}
		if err.Suggestion == "" {
			err.Suggestion = best.Summary
		}
		err.Context["ai_suggestions"] = suggestions
	}

	return err
}
