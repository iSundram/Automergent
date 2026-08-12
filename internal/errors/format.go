package errors

import (
	"fmt"
	"io"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════════
// Error Formatters
// ══════════════════════════════════════════════════════════════════════════════

// FormatOption controls how errors are formatted.
type FormatOption int

const (
	FormatBrief FormatOption = iota // One-line summary
	FormatFull                      // Full details including context
	FormatDebug                     // All details including stack trace
	FormatJSON                      // JSON format for machine processing
)

// Format formats an error according to the specified option.
func Format(err error, opt FormatOption) string {
	if err == nil {
		return ""
	}

	oce := GetAutomergentError(err)
	if oce == nil {
		// Fall back to standard error formatting
		return err.Error()
	}

	switch opt {
	case FormatBrief:
		return formatBrief(oce)
	case FormatFull:
		return formatFull(oce)
	case FormatDebug:
		return oce.DebugString()
	case FormatJSON:
		return formatJSON(oce)
	default:
		return oce.Error()
	}
}

func formatBrief(e *AutomergentError) string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func formatFull(e *AutomergentError) string {
	var b strings.Builder

	// Main error
	b.WriteString(fmt.Sprintf("Error [%s]: %s\n", e.Code, e.Message))

	// Category and severity
	b.WriteString(fmt.Sprintf("Category: %s | Severity: %s\n", e.Category, e.Severity))

	// Operation and resource
	if e.Operation != "" {
		b.WriteString(fmt.Sprintf("Operation: %s\n", e.Operation))
	}
	if e.Resource != "" {
		b.WriteString(fmt.Sprintf("Resource: %s\n", e.Resource))
	}

	// Suggestion
	if e.Suggestion != "" {
		b.WriteString(fmt.Sprintf("\nSuggestion: %s\n", e.Suggestion))
	}

	// Retry info
	if e.Retriable {
		b.WriteString(fmt.Sprintf("\nRetriable: yes (after %s)\n", e.RetryAfter))
	}

	// Underlying error
	if e.Err != nil {
		b.WriteString(fmt.Sprintf("\nCaused by: %v\n", e.Err))
	}

	return b.String()
}

func formatJSON(e *AutomergentError) string {
	m := e.ToMap()
	var b strings.Builder
	b.WriteString("{\n")
	i := 0
	for k, v := range m {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(fmt.Sprintf(`  "%s": %v`, k, jsonValue(v)))
		i++
	}
	b.WriteString("\n}")
	return b.String()
}

func jsonValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(val, `"`, `\"`))
	case bool:
		return fmt.Sprintf("%t", val)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf(`"%v"`, val)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Terminal Formatting (colored output)
// ══════════════════════════════════════════════════════════════════════════════

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// FormatTerminal formats an error with ANSI colors for terminal display.
func FormatTerminal(err error, useColor bool) string {
	if err == nil {
		return ""
	}

	oce := GetAutomergentError(err)
	if oce == nil {
		if useColor {
			return colorRed + "Error: " + err.Error() + colorReset
		}
		return "Error: " + err.Error()
	}

	var b strings.Builder

	// Color based on severity
	severityColor := colorRed
	switch oce.Severity {
	case SeverityWarning:
		severityColor = colorYellow
	case SeverityInfo:
		severityColor = colorBlue
	case SeverityDebug:
		severityColor = colorGray
	}

	if useColor {
		// Error header with color
		b.WriteString(severityColor + colorBold)
		b.WriteString(fmt.Sprintf("[%s]", oce.Code))
		b.WriteString(colorReset + " ")
		b.WriteString(oce.Message)
		b.WriteString("\n")

		// Details in dim
		if oce.Operation != "" || oce.Resource != "" {
			b.WriteString(colorDim)
			if oce.Operation != "" {
				b.WriteString(fmt.Sprintf("  Operation: %s\n", oce.Operation))
			}
			if oce.Resource != "" {
				b.WriteString(fmt.Sprintf("  Resource: %s\n", oce.Resource))
			}
			b.WriteString(colorReset)
		}

		// Suggestion in cyan
		if oce.Suggestion != "" {
			b.WriteString(colorCyan)
			b.WriteString(fmt.Sprintf("\n💡 %s\n", oce.Suggestion))
			b.WriteString(colorReset)
		}

		// Underlying error
		if oce.Err != nil {
			b.WriteString(colorGray)
			b.WriteString(fmt.Sprintf("\nCaused by: %v\n", oce.Err))
			b.WriteString(colorReset)
		}
	} else {
		// Plain text
		b.WriteString(fmt.Sprintf("[%s] %s\n", oce.Code, oce.Message))

		if oce.Operation != "" {
			b.WriteString(fmt.Sprintf("  Operation: %s\n", oce.Operation))
		}
		if oce.Resource != "" {
			b.WriteString(fmt.Sprintf("  Resource: %s\n", oce.Resource))
		}
		if oce.Suggestion != "" {
			b.WriteString(fmt.Sprintf("\nSuggestion: %s\n", oce.Suggestion))
		}
		if oce.Err != nil {
			b.WriteString(fmt.Sprintf("\nCaused by: %v\n", oce.Err))
		}
	}

	return b.String()
}

// ══════════════════════════════════════════════════════════════════════════════
// Error Writing
// ══════════════════════════════════════════════════════════════════════════════

// WriteError writes a formatted error to the given writer.
func WriteError(w io.Writer, err error, opt FormatOption) (int, error) {
	if err == nil {
		return 0, nil
	}
	return fmt.Fprintln(w, Format(err, opt))
}

// WriteErrorTerminal writes a terminal-formatted error to the given writer.
func WriteErrorTerminal(w io.Writer, err error, useColor bool) (int, error) {
	if err == nil {
		return 0, nil
	}
	return fmt.Fprint(w, FormatTerminal(err, useColor))
}

// ══════════════════════════════════════════════════════════════════════════════
// Markdown Formatting
// ══════════════════════════════════════════════════════════════════════════════

// FormatMarkdown formats an error as markdown.
func FormatMarkdown(err error) string {
	if err == nil {
		return ""
	}

	oce := GetAutomergentError(err)
	if oce == nil {
		return fmt.Sprintf("**Error:** %s\n", err.Error())
	}

	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("## ❌ Error: %s\n\n", oce.Code))
	b.WriteString(fmt.Sprintf("**%s**\n\n", oce.Message))

	// Details table
	b.WriteString("| Property | Value |\n")
	b.WriteString("|----------|-------|\n")
	b.WriteString(fmt.Sprintf("| Category | %s |\n", oce.Category))
	b.WriteString(fmt.Sprintf("| Severity | %s |\n", oce.Severity))
	if oce.Operation != "" {
		b.WriteString(fmt.Sprintf("| Operation | %s |\n", oce.Operation))
	}
	if oce.Resource != "" {
		b.WriteString(fmt.Sprintf("| Resource | `%s` |\n", oce.Resource))
	}
	if oce.Retriable {
		b.WriteString(fmt.Sprintf("| Retriable | Yes (after %s) |\n", oce.RetryAfter))
	}

	b.WriteString("\n")

	// Suggestion
	if oce.Suggestion != "" {
		b.WriteString("### 💡 Suggestion\n\n")
		b.WriteString(oce.Suggestion)
		b.WriteString("\n\n")
	}

	// Context
	if len(oce.Context) > 0 {
		b.WriteString("### Context\n\n")
		b.WriteString("```json\n")
		for k, v := range oce.Context {
			b.WriteString(fmt.Sprintf("  \"%s\": %v\n", k, v))
		}
		b.WriteString("```\n\n")
	}

	// Underlying error
	if oce.Err != nil {
		b.WriteString("### Underlying Error\n\n")
		b.WriteString(fmt.Sprintf("```\n%v\n```\n", oce.Err))
	}

	return b.String()
}
