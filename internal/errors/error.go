package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// AutomergentError is the structured error type for Automergent with rich context,
// AI-powered explanations, and fix suggestions.
type AutomergentError struct {
	// Err is the underlying error (may be nil)
	Err error

	// Code is the typed error code for programmatic handling
	Code ErrorCode

	// Category groups related error types
	Category Category

	// Severity indicates how critical the error is
	Severity Severity

	// Message is the user-friendly error message
	Message string

	// Operation describes what was being attempted when the error occurred
	Operation string

	// Resource identifies the file, URL, or resource involved
	Resource string

	// Context holds additional key-value pairs for debugging
	Context map[string]any

	// Suggestion provides guidance on how to fix the error
	Suggestion string

	// Retriable indicates whether the operation can be retried
	Retriable bool

	// RetryAfter suggests how long to wait before retrying.
	// When unset, retry delay may still be derived from known context fields
	// (retry_after, retry_after_ms, retry_delay, ...).
	RetryAfter time.Duration

	// Stack contains the call stack at the point of error creation
	Stack []Frame

	// Timestamp records when the error occurred
	Timestamp time.Time

	// RequestID links the error to a specific request (if applicable)
	RequestID string
}

// Frame represents a single stack frame.
type Frame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// Error implements the error interface.
func (e *AutomergentError) Error() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s] ", e.Code))
	b.WriteString(e.Message)

	if e.Resource != "" {
		b.WriteString(fmt.Sprintf(" (resource: %s)", e.Resource))
	}
	if e.Operation != "" {
		b.WriteString(fmt.Sprintf(" (operation: %s)", e.Operation))
	}
	if e.Err != nil {
		b.WriteString(fmt.Sprintf(": %v", e.Err))
	}
	return b.String()
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *AutomergentError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is matching by code.
func (e *AutomergentError) Is(target error) bool {
	if te, ok := target.(*AutomergentError); ok {
		return e.Code == te.Code
	}
	return false
}

// WithContext adds a key-value pair to the error context.
func (e *AutomergentError) WithContext(key string, value any) *AutomergentError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// WithResource sets the resource associated with the error.
func (e *AutomergentError) WithResource(resource string) *AutomergentError {
	e.Resource = resource
	return e
}

// WithOperation sets the operation being performed.
func (e *AutomergentError) WithOperation(op string) *AutomergentError {
	e.Operation = op
	return e
}

// WithSuggestion sets a fix suggestion.
func (e *AutomergentError) WithSuggestion(suggestion string) *AutomergentError {
	e.Suggestion = suggestion
	return e
}

// WithRetry marks the error as retriable.
func (e *AutomergentError) WithRetry(after time.Duration) *AutomergentError {
	e.Retriable = true
	if after > 0 {
		e.RetryAfter = after
	} else {
		e.RetryAfter = 0
	}
	return e
}

// WithRequestID associates the error with a request.
func (e *AutomergentError) WithRequestID(id string) *AutomergentError {
	e.RequestID = id
	return e
}

// WithSeverity sets the error severity.
func (e *AutomergentError) WithSeverity(severity Severity) *AutomergentError {
	e.Severity = severity
	return e
}

// Wrap wraps an existing error with additional context.
func (e *AutomergentError) Wrap(err error) *AutomergentError {
	e.Err = err
	return e
}

// UserMessage returns a clean message suitable for end users.
func (e *AutomergentError) UserMessage() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s\n\nSuggestion: %s", e.Message, e.Suggestion)
	}
	return e.Message
}

// DebugString returns a detailed string for debugging.
func (e *AutomergentError) DebugString() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Error: %s\n", e.Message))
	b.WriteString(fmt.Sprintf("  Code: %s\n", e.Code))
	b.WriteString(fmt.Sprintf("  Category: %s\n", e.Category))
	b.WriteString(fmt.Sprintf("  Severity: %s\n", e.Severity))

	if e.Operation != "" {
		b.WriteString(fmt.Sprintf("  Operation: %s\n", e.Operation))
	}
	if e.Resource != "" {
		b.WriteString(fmt.Sprintf("  Resource: %s\n", e.Resource))
	}
	if e.RequestID != "" {
		b.WriteString(fmt.Sprintf("  Request ID: %s\n", e.RequestID))
	}
	if e.Suggestion != "" {
		b.WriteString(fmt.Sprintf("  Suggestion: %s\n", e.Suggestion))
	}
	if e.Retriable {
		b.WriteString(fmt.Sprintf("  Retriable: true (after %s)\n", e.RetryAfter))
	}
	if len(e.Context) > 0 {
		b.WriteString("  Context:\n")
		for k, v := range e.Context {
			b.WriteString(fmt.Sprintf("    %s: %v\n", k, v))
		}
	}
	if e.Err != nil {
		b.WriteString(fmt.Sprintf("  Underlying error: %v\n", e.Err))
	}
	if len(e.Stack) > 0 {
		b.WriteString("  Stack trace:\n")
		for _, frame := range e.Stack {
			b.WriteString(fmt.Sprintf("    %s\n      %s:%d\n", frame.Function, frame.File, frame.Line))
		}
	}
	return b.String()
}

// ToMap returns a map representation for JSON serialization.
func (e *AutomergentError) ToMap() map[string]any {
	m := map[string]any{
		"code":      string(e.Code),
		"category":  string(e.Category),
		"severity":  string(e.Severity),
		"message":   e.Message,
		"timestamp": e.Timestamp.Format(time.RFC3339),
		"retriable": e.Retriable,
	}
	if e.Operation != "" {
		m["operation"] = e.Operation
	}
	if e.Resource != "" {
		m["resource"] = e.Resource
	}
	if e.Suggestion != "" {
		m["suggestion"] = e.Suggestion
	}
	if e.RequestID != "" {
		m["request_id"] = e.RequestID
	}
	if e.Retriable && e.RetryAfter > 0 {
		m["retry_after_ms"] = e.RetryAfter.Milliseconds()
	}
	if len(e.Context) > 0 {
		m["context"] = e.Context
	}
	if e.Err != nil {
		m["cause"] = e.Err.Error()
	}
	return m
}

// captureStack captures the current call stack, skipping n frames.
func captureStack(skip int) []Frame {
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip+2, pcs) // +2 to skip Callers and captureStack
	if n == 0 {
		return nil
	}

	frames := make([]Frame, 0, n)
	callersFrames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := callersFrames.Next()
		// Skip runtime internals
		if !strings.Contains(frame.File, "runtime/") {
			frames = append(frames, Frame{
				Function: frame.Function,
				File:     frame.File,
				Line:     frame.Line,
			})
		}
		if !more {
			break
		}
	}
	return frames
}

// New creates a new AutomergentError with the given code and message.
func New(code ErrorCode, message string) *AutomergentError {
	return &AutomergentError{
		Code:      code,
		Category:  CategoryOf(code),
		Severity:  SeverityError,
		Message:   message,
		Context:   make(map[string]any),
		Stack:     captureStack(1),
		Timestamp: time.Now(),
	}
}

// Wrap wraps an existing error with a code and message.
func Wrap(err error, code ErrorCode, message string) *AutomergentError {
	if err == nil {
		return New(code, message)
	}

	// If wrapping another AutomergentError, preserve its context
	var oce *AutomergentError
	if errors.As(err, &oce) {
		newErr := New(code, message)
		newErr.Err = err
		// Merge context from wrapped error
		for k, v := range oce.Context {
			if _, exists := newErr.Context[k]; !exists {
				newErr.Context[k] = v
			}
		}
		return newErr
	}

	e := New(code, message)
	e.Err = err
	return e
}

// Wrapf wraps an error with a formatted message.
func Wrapf(err error, code ErrorCode, format string, args ...any) *AutomergentError {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// Is checks if an error matches a specific error code.
func Is(err error, code ErrorCode) bool {
	var oce *AutomergentError
	if errors.As(err, &oce) {
		return oce.Code == code
	}
	return false
}

// IsCategory checks if an error belongs to a specific category.
func IsCategory(err error, cat Category) bool {
	var oce *AutomergentError
	if errors.As(err, &oce) {
		return oce.Category == cat
	}
	return false
}

// GetCode extracts the error code from an error.
func GetCode(err error) ErrorCode {
	var oce *AutomergentError
	if errors.As(err, &oce) {
		return oce.Code
	}
	return CodeUnknown
}

// GetAutomergentError extracts an AutomergentError from any error.
func GetAutomergentError(err error) *AutomergentError {
	var oce *AutomergentError
	if errors.As(err, &oce) {
		return oce
	}
	return nil
}

// IsRetriable checks if an error can be retried.
func IsRetriable(err error) bool {
	var oce *AutomergentError
	if errors.As(err, &oce) {
		return oce.Retriable
	}
	return false
}

// GetRetryAfter returns how long to wait before retrying.
// It prefers RetryAfter and falls back to known retry delay context fields.
func GetRetryAfter(err error) time.Duration {
	var oce *AutomergentError
	if !errors.As(err, &oce) || !oce.Retriable {
		return 0
	}
	if oce.RetryAfter > 0 {
		return oce.RetryAfter
	}

	return parseRetryDelayFromContext(oce.Context)
}

func parseRetryDelayFromContext(ctx map[string]any) time.Duration {
	if len(ctx) == 0 {
		return 0
	}

	maxDelay := time.Duration(0)

	for _, key := range []string{"retry_after_ms", "retryAfterMs", "retry_delay_ms", "retryDelayMs"} {
		if value, ok := ctx[key]; ok {
			if delay := parseRetryDelayValue(value, time.Millisecond); delay > maxDelay {
				maxDelay = delay
			}
		}
	}

	for _, key := range []string{"retry_after", "retryAfter", "retry_after_seconds", "retryAfterSeconds", "retry_delay", "retryDelay", "retry_delay_seconds"} {
		if value, ok := ctx[key]; ok {
			if delay := parseRetryDelayValue(value, time.Second); delay > maxDelay {
				maxDelay = delay
			}
		}
	}

	return maxDelay
}

func parseRetryDelayValue(value any, unit time.Duration) time.Duration {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return time.Duration(v) * unit
		}
	case int32:
		if v > 0 {
			return time.Duration(v) * unit
		}
	case int64:
		if v > 0 {
			return time.Duration(v) * unit
		}
	case float32:
		if v > 0 {
			return time.Duration(float64(v) * float64(unit))
		}
	case float64:
		if v > 0 {
			return time.Duration(v * float64(unit))
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0
		}
		if numeric, err := strconv.ParseFloat(text, 64); err == nil && numeric > 0 {
			return time.Duration(numeric * float64(unit))
		}
		if duration, err := time.ParseDuration(text); err == nil && duration > 0 {
			return duration
		}
	}
	return 0
}
