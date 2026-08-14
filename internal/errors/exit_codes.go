package errors

import (
	"context"
	stderrors "errors"
	"strings"
)

// ExitCode represents process exit codes for CLI automation.
type ExitCode int

const (
	ExitOK                 ExitCode = 0
	ExitGeneral            ExitCode = 1
	ExitInvalidArgs        ExitCode = 2
	ExitAuthFailed         ExitCode = 10
	ExitAPIError           ExitCode = 11
	ExitContextExceeded    ExitCode = 12
	ExitToolExecutionError ExitCode = 20
	ExitTimeout            ExitCode = 124
	ExitInterrupted        ExitCode = 130
)

var categoryToExitCode = map[Category]ExitCode{
	CategoryValidation: ExitInvalidArgs,
	CategoryConfig:     ExitInvalidArgs,
	CategoryAuth:       ExitAuthFailed,
	CategoryAPI:        ExitAPIError,
	CategoryNetwork:    ExitAPIError,
	CategoryAI:         ExitAPIError,
	CategoryTool:       ExitToolExecutionError,
	CategoryProcess:    ExitToolExecutionError,
}

// ExitCodeForCategory maps a typed error category to a CLI exit code.
func ExitCodeForCategory(category Category) ExitCode {
	if code, ok := categoryToExitCode[category]; ok {
		return code
	}
	return ExitGeneral
}

// ExitCodeForError maps an error to a CLI exit code with safe fallbacks.
func ExitCodeForError(err error) ExitCode {
	switch ErrorCategoryForError(err) {
	case "":
		return ExitOK
	case "invalid_args":
		return ExitInvalidArgs
	case "auth_failed":
		return ExitAuthFailed
	case "provider_error":
		return ExitAPIError
	case "tool_error":
		return ExitToolExecutionError
	case "context_limit":
		return ExitContextExceeded
	case "timeout":
		return ExitTimeout
	case "interrupted":
		return ExitInterrupted
	default:
		return ExitGeneral
	}
}

// ErrorCategoryForError returns a stable CLI-facing category key for an error.
// Categories: invalid_args, auth_failed, provider_error, tool_error,
// context_limit, internal_error, timeout, interrupted.
func ErrorCategoryForError(err error) string {
	if err == nil {
		return ""
	}

	if stderrors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if stderrors.Is(err, context.Canceled) {
		return "interrupted"
	}

	oce := GetAutomergentError(err)
	if oce != nil {
		return categoryFromAutomergentError(oce)
	}
	return categoryFromMessage(err.Error())
}

func categoryFromAutomergentError(oce *AutomergentError) string {
	switch oce.Code {
	case CodeContextTooLong:
		return "context_limit"
	case CodeUnauthorized, CodeForbidden, CodeTokenExpired, CodeTokenInvalid, CodeCredentialsInvalid, CodeSessionExpired, CodeMFARequired:
		return "auth_failed"
	}

	switch oce.Category {
	case CategoryValidation, CategoryConfig:
		return "invalid_args"
	case CategoryAuth:
		return "auth_failed"
	case CategoryAPI, CategoryNetwork, CategoryAI:
		return "provider_error"
	case CategoryTool, CategoryProcess:
		return "tool_error"
	default:
		return "internal_error"
	}
}

func categoryFromMessage(msg string) string {
	lower := strings.ToLower(msg)
	contains := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(lower, p) {
				return true
			}
		}
		return false
	}

	if contains(
		"unknown flag",
		"invalid argument",
		"requires at least",
		"accepts ",
		"prompt required",
		"unknown provider",
		"invalid output format",
		"decode config",
		"load session",
	) {
		return "invalid_args"
	}
	if contains(
		"unauthorized",
		"forbidden",
		"invalid api key",
		"api key not set",
		"token expired",
		"token invalid",
		"401",
		"403",
	) {
		return "auth_failed"
	}
	if contains(
		"context too long",
		"context window",
		"token limit",
		"max tokens",
		"context exceeded",
	) {
		return "context_limit"
	}
	if contains(
		"tool",
		"command failed",
		"process failed",
		"exit code",
		"agent: stream",
	) {
		return "tool_error"
	}
	if contains(
		"provider",
		"rate limit",
		"service unavailable",
		"server error",
		"gateway timeout",
		"bad gateway",
		"connection refused",
		"connection failed",
		"429",
		"5xx",
		"agent: complete",
	) {
		return "provider_error"
	}
	return "internal_error"
}
