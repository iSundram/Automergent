package recovery

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/compiler"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
	recoverypolicy "github.com/iSundram/Automergent/internal/recovery"
)

// Cause classifies the likely root cause of a diagnostic.
type Cause string

const (
	CauseUnknown     Cause = "unknown"
	CauseSyntax      Cause = "syntax"
	CauseImport      Cause = "import"
	CauseDependency  Cause = "dependency"
	CauseConfig      Cause = "config"
	CauseMissingFile Cause = "missing_file"
	CausePermission  Cause = "permission"
	CauseTransient   Cause = "transient"
)

// RetryPolicy describes retry behavior with jitter.
type RetryPolicy struct {
	Retryable    bool          `json:"retryable"`
	MaxAttempts  int           `json:"max_attempts"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
	Multiplier   float64       `json:"multiplier"`
	Jitter       float64       `json:"jitter"`
}

// Delay returns the delay for a retry attempt using exponential backoff and jitter.
func (p RetryPolicy) Delay(attempt int, rng *rand.Rand) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if p.InitialDelay <= 0 {
		p.InitialDelay = 250 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 5 * time.Second
	}
	if p.Multiplier <= 0 {
		p.Multiplier = 2
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 1
	}

	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt-1))
	if p.Jitter > 0 {
		if rng == nil {
			rng = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		span := delay * p.Jitter
		delay += (rng.Float64()*2 - 1) * span
	}

	if delay < float64(p.InitialDelay) {
		delay = float64(p.InitialDelay)
	}
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	return time.Duration(delay)
}

// AsRecoveryPolicy adapts diagnostic retry settings to the unified policy contract.
func (p RetryPolicy) AsRecoveryPolicy() recoverypolicy.Policy {
	return &recoverypolicy.ExponentialPolicy{
		MaxAttempts:  p.MaxAttempts + 1,
		InitialDelay: p.InitialDelay,
		MaxDelay:     p.MaxDelay,
		Multiplier:   p.Multiplier,
		Jitter:       p.Jitter,
		ShouldRetry: func(err error) (bool, string) {
			if p.Retryable {
				return true, "diagnostic-policy-retryable"
			}
			return false, "diagnostic-policy-non-retryable"
		},
	}
}

// Classification describes how to recover from a diagnostic.
type Classification struct {
	Diagnostic     types.Diagnostic `json:"diagnostic"`
	Cause          Cause            `json:"cause"`
	RootCauseHint  string           `json:"root_cause_hint"`
	FixSuggestions []string         `json:"fix_suggestions"`
	UserMessage    string           `json:"user_message"`
	Retry          RetryPolicy      `json:"retry"`
	Confidence     float64          `json:"confidence"`
}

// Report summarizes diagnostics into actionable recovery guidance.
type Report struct {
	Primary     Classification   `json:"primary"`
	Items       []Classification `json:"items"`
	Summary     string           `json:"summary"`
	UserMessage string           `json:"user_message"`
	ActionItems []string         `json:"action_items"`
	Retry       RetryPolicy      `json:"retry"`
}

// ClassifyDiagnostic turns a single diagnostic into a recovery classification.
func ClassifyDiagnostic(diag types.Diagnostic) Classification {
	msg := strings.ToLower(diag.Message)
	code := strings.ToLower(diag.Code)
	source := strings.ToLower(diag.Source)

	class := Classification{
		Diagnostic: diag,
		Cause:      CauseUnknown,
		Retry: RetryPolicy{
			Retryable:    false,
			MaxAttempts:  1,
			InitialDelay: 250 * time.Millisecond,
			MaxDelay:     5 * time.Second,
			Multiplier:   2,
			Jitter:       0.2,
		},
		Confidence: 0.35,
	}

	switch {
	case strings.Contains(code, "missing-package"):
		class.Cause = CauseSyntax
		class.RootCauseHint = "The file is missing a Go package declaration."
		class.FixSuggestions = []string{
			"Add `package <name>` at the top of the file.",
			"Keep the package name consistent with the rest of the directory.",
		}
		class.UserMessage = "Likely root cause: missing package declaration."
		class.Confidence = 0.95
	case strings.Contains(code, "json-syntax-error"), strings.Contains(code, "syntax-error"),
		strings.Contains(code, "missing-token"), strings.Contains(source, "tree-sitter-"), strings.Contains(msg, "syntax"):
		class.Cause = CauseSyntax
		class.RootCauseHint = "The parser found malformed source near the reported location."
		class.FixSuggestions = []string{
			"Check brackets, quotes, and delimiters near the reported line.",
			"Re-run the formatter or parser after fixing the syntax.",
		}
		class.UserMessage = "Likely root cause: syntax issue in the file."
		class.Confidence = 0.9
	case strings.Contains(code, "await-outside-async"):
		class.Cause = CauseSyntax
		class.RootCauseHint = "await is used outside an async function."
		class.FixSuggestions = []string{
			"Mark the function async or remove await.",
			"Verify the surrounding function scope.",
		}
		class.UserMessage = "Likely root cause: async/await misuse."
		class.Confidence = 0.92
	case strings.Contains(msg, "cannot find package") || strings.Contains(msg, "module not found") || strings.Contains(msg, "no module named") || strings.Contains(msg, "could not find crate"):
		class.Cause = CauseImport
		class.RootCauseHint = "The dependency or import path cannot be resolved."
		class.FixSuggestions = []string{
			"Verify the import path spelling.",
			"Install or declare the missing dependency.",
		}
		class.UserMessage = "Likely root cause: missing or incorrect dependency import."
		class.Confidence = 0.94
		class.Retry.Retryable = false
	case strings.Contains(msg, "permission denied"):
		class.Cause = CausePermission
		class.RootCauseHint = "The current process lacks permission to access the file."
		class.FixSuggestions = []string{
			"Check file ownership and permissions.",
			"Re-run with the required privileges if appropriate.",
		}
		class.UserMessage = "Likely root cause: permission problem."
		class.Confidence = 0.88
	case strings.Contains(msg, "no such file") || strings.Contains(msg, "file not found"):
		class.Cause = CauseMissingFile
		class.RootCauseHint = "A referenced file or path does not exist."
		class.FixSuggestions = []string{
			"Verify the file path.",
			"Create the missing file or update the reference.",
		}
		class.UserMessage = "Likely root cause: missing file."
		class.Confidence = 0.9
	case strings.Contains(msg, "temporarily") || strings.Contains(msg, "timeout") || strings.Contains(msg, "try again"):
		class.Cause = CauseTransient
		class.RootCauseHint = "The failure looks transient."
		class.FixSuggestions = []string{
			"Retry the operation after a short delay.",
			"Check for network or service instability.",
		}
		class.UserMessage = "Likely root cause: transient failure."
		class.Confidence = 0.6
		class.Retry.Retryable = true
		class.Retry.MaxAttempts = 3
		class.Retry.InitialDelay = 300 * time.Millisecond
		class.Retry.MaxDelay = 2 * time.Second
		class.Retry.Jitter = 0.25
	}

	if len(class.FixSuggestions) == 0 {
		class.FixSuggestions = []string{
			"Inspect the reported location and surrounding code.",
			"Apply the smallest fix and re-run diagnostics.",
		}
	}

	return class
}

// ClassifyCompilerDiagnostic converts compiler diagnostics into recovery guidance.
func ClassifyCompilerDiagnostic(diag compiler.CompilerDiagnostic) Classification {
	return ClassifyDiagnostic(types.Diagnostic{
		FilePath: diag.FilePath,
		Line:     diag.Line,
		Column:   diag.Column,
		Severity: diag.Severity,
		Code:     diag.ErrorCode,
		Message:  diag.Message,
		Source:   diag.Source,
	})
}

// Summarize converts diagnostics into a single actionable report.
func Summarize(diags []types.Diagnostic) Report {
	items := make([]Classification, 0, len(diags))
	for _, diag := range diags {
		items = append(items, ClassifyDiagnostic(diag))
	}
	return summarize(items)
}

// SummarizeCompiler converts compiler diagnostics into a single actionable report.
func SummarizeCompiler(diags []compiler.CompilerDiagnostic) Report {
	items := make([]Classification, 0, len(diags))
	for _, diag := range diags {
		items = append(items, ClassifyCompilerDiagnostic(diag))
	}
	return summarize(items)
}

func summarize(items []Classification) Report {
	if len(items) == 0 {
		return Report{}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		if items[i].Diagnostic.Line != items[j].Diagnostic.Line {
			return items[i].Diagnostic.Line < items[j].Diagnostic.Line
		}
		return items[i].Diagnostic.Column < items[j].Diagnostic.Column
	})

	primary := items[0]
	suggestions := dedupStrings(primary.FixSuggestions)
	for _, item := range items[1:] {
		suggestions = append(suggestions, item.FixSuggestions...)
	}
	suggestions = dedupStrings(suggestions)

	actionItems := make([]string, 0, len(suggestions))
	for _, s := range suggestions {
		actionItems = append(actionItems, "- "+s)
	}

	summary := primary.UserMessage
	if summary == "" {
		summary = "Likely root cause identified."
	}

	userMessage := summary
	if len(suggestions) > 0 {
		userMessage += "\nNext steps:\n" + strings.Join(actionItems, "\n")
	}
	if primary.Retry.Retryable {
		userMessage += fmt.Sprintf("\nRetry policy: %d attempts, start after %s with jitter %.0f%%.",
			primary.Retry.MaxAttempts, primary.Retry.InitialDelay, primary.Retry.Jitter*100)
	}

	return Report{
		Primary:     primary,
		Items:       items,
		Summary:     summary,
		UserMessage: userMessage,
		ActionItems: actionItems,
		Retry:       primary.Retry,
	}
}

func dedupStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

// Render formats the report for terminal-friendly user output.
func (r Report) Render() string {
	if r.Summary == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("[RECOVERY]\n")
	b.WriteString(r.UserMessage)
	b.WriteString("\n")
	return b.String()
}
