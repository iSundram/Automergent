package errors

import (
	"fmt"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// Validation Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewValidationError creates a validation error.
func NewValidationError(message string) *AutomergentError {
	return New(CodeValidationFailed, message).WithSeverity(SeverityWarning)
}

// NewInvalidInputError creates an error for invalid input.
func NewInvalidInputError(field, value, reason string) *AutomergentError {
	return New(CodeInvalidInput, fmt.Sprintf("invalid value for '%s': %s", field, reason)).
		WithContext("field", field).
		WithContext("value", value).
		WithSuggestion(fmt.Sprintf("Provide a valid value for '%s'", field))
}

// NewMissingRequiredError creates an error for missing required fields.
func NewMissingRequiredError(field string) *AutomergentError {
	return New(CodeMissingRequired, fmt.Sprintf("required field '%s' is missing", field)).
		WithContext("field", field).
		WithSuggestion(fmt.Sprintf("Provide a value for '%s'", field))
}

// NewOutOfRangeError creates an error for values outside acceptable range.
func NewOutOfRangeError(field string, value, min, max any) *AutomergentError {
	return New(CodeOutOfRange, fmt.Sprintf("value %v for '%s' is out of range [%v, %v]", value, field, min, max)).
		WithContext("field", field).
		WithContext("value", value).
		WithContext("min", min).
		WithContext("max", max).
		WithSuggestion(fmt.Sprintf("Provide a value between %v and %v for '%s'", min, max, field))
}

// ══════════════════════════════════════════════════════════════════════════════
// IO Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewFileNotFoundError creates a file not found error.
func NewFileNotFoundError(path string) *AutomergentError {
	return New(CodeFileNotFound, fmt.Sprintf("file not found: %s", path)).
		WithResource(path).
		WithSuggestion("Check that the file path is correct and the file exists")
}

// NewFileExistsError creates an error when a file already exists.
func NewFileExistsError(path string) *AutomergentError {
	return New(CodeFileExists, fmt.Sprintf("file already exists: %s", path)).
		WithResource(path).
		WithSuggestion("Use a different filename or remove the existing file first")
}

// NewPermissionDeniedError creates a permission denied error.
func NewPermissionDeniedError(path, operation string) *AutomergentError {
	return New(CodePermissionDenied, fmt.Sprintf("permission denied: cannot %s '%s'", operation, path)).
		WithResource(path).
		WithOperation(operation).
		WithSuggestion("Check file permissions or run with appropriate privileges")
}

// NewDiskFullError creates a disk full error.
func NewDiskFullError(path string, required int64) *AutomergentError {
	return New(CodeDiskFull, "disk space is full").
		WithResource(path).
		WithContext("required_bytes", required).
		WithSuggestion("Free up disk space or write to a different location")
}

// NewIOError creates a general IO error.
func NewIOError(operation, resource string, err error) *AutomergentError {
	code := CodeReadError
	if operation == "write" || operation == "create" || operation == "delete" {
		code = CodeWriteError
	}
	return Wrap(err, code, fmt.Sprintf("IO error during %s", operation)).
		WithOperation(operation).
		WithResource(resource)
}

// NewPathInvalidError creates an invalid path error.
func NewPathInvalidError(path, reason string) *AutomergentError {
	return New(CodePathInvalid, fmt.Sprintf("invalid path '%s': %s", path, reason)).
		WithResource(path).
		WithContext("reason", reason)
}

// ══════════════════════════════════════════════════════════════════════════════
// Network Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewConnectionError creates a connection failure error.
func NewConnectionError(host string, err error) *AutomergentError {
	return Wrap(err, CodeConnectionFailed, fmt.Sprintf("failed to connect to %s", host)).
		WithResource(host).
		WithRetry(5 * time.Second).
		WithSuggestion("Check your network connection and verify the host is reachable")
}

// NewTimeoutError creates a connection timeout error.
func NewTimeoutError(host string, duration time.Duration) *AutomergentError {
	return New(CodeConnectionTimeout, fmt.Sprintf("connection to %s timed out after %s", host, duration)).
		WithResource(host).
		WithContext("timeout_duration", duration.String()).
		WithRetry(10 * time.Second).
		WithSuggestion("The server may be slow or overloaded. Try again later")
}

// NewDNSError creates a DNS resolution error.
func NewDNSError(host string, err error) *AutomergentError {
	return Wrap(err, CodeDNSError, fmt.Sprintf("failed to resolve DNS for %s", host)).
		WithResource(host).
		WithSuggestion("Check that the hostname is correct and DNS is working")
}

// NewTLSError creates a TLS/SSL error.
func NewTLSError(host string, err error) *AutomergentError {
	return Wrap(err, CodeTLSError, fmt.Sprintf("TLS handshake failed with %s", host)).
		WithResource(host).
		WithSuggestion("Check certificate validity or consider the server's SSL configuration")
}

// NewHTTPError creates an HTTP error with status code.
func NewHTTPError(status int, body string, url string) *AutomergentError {
	var code ErrorCode
	var retriable bool
	var retryAfter time.Duration

	switch {
	case status == 429:
		code = CodeRateLimited
		retriable = true
		retryAfter = 60 * time.Second
	case status == 401:
		code = CodeUnauthorized
	case status == 403:
		code = CodeForbidden
	case status == 404:
		code = CodeNotFound
	case status == 409:
		code = CodeConflict
	case status == 502:
		code = CodeBadGateway
		retriable = true
		retryAfter = 10 * time.Second
	case status == 503:
		code = CodeServiceUnavailable
		retriable = true
		retryAfter = 30 * time.Second
	case status == 504:
		code = CodeGatewayTimeout
		retriable = true
		retryAfter = 10 * time.Second
	case status >= 500:
		code = CodeServerError
		retriable = true
		retryAfter = 5 * time.Second
	default:
		code = CodeHTTPError
	}

	e := New(code, fmt.Sprintf("HTTP %d error", status)).
		WithResource(url).
		WithContext("status_code", status)

	if body != "" {
		// Truncate long bodies
		if len(body) > 500 {
			body = body[:500] + "..."
		}
		e.WithContext("response_body", body)
	}

	if retriable {
		e.WithRetry(retryAfter)
	}

	return e
}

// ══════════════════════════════════════════════════════════════════════════════
// API Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewRateLimitError creates a rate limit error.
func NewRateLimitError(service string, retryAfter time.Duration) *AutomergentError {
	return New(CodeRateLimited, fmt.Sprintf("rate limit exceeded for %s", service)).
		WithResource(service).
		WithRetry(retryAfter).
		WithSuggestion(fmt.Sprintf("Wait %s before making another request", retryAfter))
}

// NewQuotaExceededError creates a quota exceeded error.
func NewQuotaExceededError(service string, quota, used int64) *AutomergentError {
	return New(CodeQuotaExceeded, fmt.Sprintf("quota exceeded for %s (used %d of %d)", service, used, quota)).
		WithResource(service).
		WithContext("quota", quota).
		WithContext("used", used).
		WithSuggestion("Upgrade your plan or wait for the quota to reset")
}

// NewUnauthorizedError creates an unauthorized error.
func NewUnauthorizedError(service, reason string) *AutomergentError {
	return New(CodeUnauthorized, fmt.Sprintf("unauthorized access to %s: %s", service, reason)).
		WithResource(service).
		WithSuggestion("Check your API key or authentication credentials")
}

// NewForbiddenError creates a forbidden error.
func NewForbiddenError(resource, reason string) *AutomergentError {
	return New(CodeForbidden, fmt.Sprintf("access forbidden: %s", reason)).
		WithResource(resource).
		WithSuggestion("You may not have permission to access this resource")
}

// ══════════════════════════════════════════════════════════════════════════════
// Configuration Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewConfigNotFoundError creates a config not found error.
func NewConfigNotFoundError(path string) *AutomergentError {
	return New(CodeConfigNotFound, fmt.Sprintf("configuration file not found: %s", path)).
		WithResource(path).
		WithSuggestion("Create the configuration file or run the setup command")
}

// NewConfigInvalidError creates an invalid configuration error.
func NewConfigInvalidError(path string, field string, reason string) *AutomergentError {
	return New(CodeConfigInvalid, fmt.Sprintf("invalid configuration in %s: %s", path, reason)).
		WithResource(path).
		WithContext("field", field).
		WithContext("reason", reason).
		WithSuggestion(fmt.Sprintf("Fix the '%s' field in your configuration", field))
}

// NewConfigParseError creates a config parse error.
func NewConfigParseError(path string, err error) *AutomergentError {
	return Wrap(err, CodeConfigParseFailed, fmt.Sprintf("failed to parse configuration: %s", path)).
		WithResource(path).
		WithSuggestion("Check the configuration file syntax (YAML/JSON/TOML)")
}

// NewMissingEnvVarError creates a missing environment variable error.
func NewMissingEnvVarError(name string) *AutomergentError {
	return New(CodeMissingEnvVar, fmt.Sprintf("required environment variable '%s' is not set", name)).
		WithContext("env_var", name).
		WithSuggestion(fmt.Sprintf("Set the %s environment variable", name))
}

// NewInvalidEnvVarError creates an invalid environment variable error.
func NewInvalidEnvVarError(name, value, reason string) *AutomergentError {
	return New(CodeInvalidEnvVar, fmt.Sprintf("invalid value for environment variable '%s': %s", name, reason)).
		WithContext("env_var", name).
		WithContext("value", value).
		WithContext("reason", reason).
		WithSuggestion(fmt.Sprintf("Set %s to a valid value", name))
}

// ══════════════════════════════════════════════════════════════════════════════
// Authentication Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewTokenExpiredError creates a token expired error.
func NewTokenExpiredError(tokenType string) *AutomergentError {
	return New(CodeTokenExpired, fmt.Sprintf("%s token has expired", tokenType)).
		WithContext("token_type", tokenType).
		WithSuggestion("Re-authenticate to get a new token")
}

// NewTokenInvalidError creates an invalid token error.
func NewTokenInvalidError(tokenType, reason string) *AutomergentError {
	return New(CodeTokenInvalid, fmt.Sprintf("invalid %s token: %s", tokenType, reason)).
		WithContext("token_type", tokenType).
		WithContext("reason", reason).
		WithSuggestion("Check your token or re-authenticate")
}

// NewCredentialsInvalidError creates an invalid credentials error.
func NewCredentialsInvalidError() *AutomergentError {
	return New(CodeCredentialsInvalid, "invalid credentials").
		WithSuggestion("Check your username and password")
}

// NewSessionExpiredError creates a session expired error.
func NewSessionExpiredError() *AutomergentError {
	return New(CodeSessionExpired, "session has expired").
		WithSuggestion("Log in again to start a new session")
}

// ══════════════════════════════════════════════════════════════════════════════
// Process Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewProcessError creates a process execution error.
func NewProcessError(command string, exitCode int, stderr string) *AutomergentError {
	e := New(CodeProcessFailed, fmt.Sprintf("command '%s' failed with exit code %d", command, exitCode)).
		WithContext("command", command).
		WithContext("exit_code", exitCode)

	if stderr != "" {
		if len(stderr) > 1000 {
			stderr = stderr[:1000] + "..."
		}
		e.WithContext("stderr", stderr)
	}

	return e
}

// NewProcessTimeoutError creates a process timeout error.
func NewProcessTimeoutError(command string, timeout time.Duration) *AutomergentError {
	return New(CodeProcessTimeout, fmt.Sprintf("command '%s' timed out after %s", command, timeout)).
		WithContext("command", command).
		WithContext("timeout", timeout.String()).
		WithSuggestion("Increase the timeout or optimize the command")
}

// NewProcessKilledError creates a process killed error.
func NewProcessKilledError(command string, signal string) *AutomergentError {
	return New(CodeProcessKilled, fmt.Sprintf("command '%s' was killed by signal %s", command, signal)).
		WithContext("command", command).
		WithContext("signal", signal)
}

// NewCommandNotFoundError creates a command not found error.
func NewCommandNotFoundError(command string) *AutomergentError {
	return New(CodeCommandNotFound, fmt.Sprintf("command not found: %s", command)).
		WithContext("command", command).
		WithSuggestion(fmt.Sprintf("Install '%s' or check that it's in your PATH", command))
}

// ══════════════════════════════════════════════════════════════════════════════
// Git Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewGitNotRepoError creates a not a git repository error.
func NewGitNotRepoError(path string) *AutomergentError {
	return New(CodeGitNotRepo, fmt.Sprintf("'%s' is not a git repository", path)).
		WithResource(path).
		WithSuggestion("Initialize a git repository with 'git init'")
}

// NewGitConflictError creates a git conflict error.
func NewGitConflictError(files []string) *AutomergentError {
	return New(CodeGitConflict, fmt.Sprintf("merge conflict in %d file(s)", len(files))).
		WithContext("conflicted_files", files).
		WithSuggestion("Resolve the conflicts and commit the changes")
}

// NewGitDirtyError creates a dirty working directory error.
func NewGitDirtyError(path string, uncommitted int) *AutomergentError {
	return New(CodeGitDirty, fmt.Sprintf("working directory has %d uncommitted changes", uncommitted)).
		WithResource(path).
		WithContext("uncommitted_count", uncommitted).
		WithSuggestion("Commit or stash your changes before proceeding")
}

// ══════════════════════════════════════════════════════════════════════════════
// AI/Provider Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewProviderError creates an AI provider error.
func NewProviderError(provider string, err error) *AutomergentError {
	return Wrap(err, CodeProviderError, fmt.Sprintf("provider '%s' error", provider)).
		WithContext("provider", provider).
		WithRetry(5 * time.Second)
}

// NewModelNotFoundError creates a model not found error.
func NewModelNotFoundError(provider, model string) *AutomergentError {
	return New(CodeModelNotFound, fmt.Sprintf("model '%s' not found for provider '%s'", model, provider)).
		WithContext("provider", provider).
		WithContext("model", model).
		WithSuggestion("Check the model name or list available models")
}

// NewContextTooLongError creates a context too long error.
func NewContextTooLongError(tokenCount, maxTokens int) *AutomergentError {
	return New(CodeContextTooLong, fmt.Sprintf("context length %d exceeds maximum %d tokens", tokenCount, maxTokens)).
		WithContext("token_count", tokenCount).
		WithContext("max_tokens", maxTokens).
		WithSuggestion("Reduce the input size or use a model with a larger context window")
}

// NewContentFilteredError creates a content filtered error.
func NewContentFilteredError(reason string) *AutomergentError {
	return New(CodeContentFiltered, fmt.Sprintf("content was filtered: %s", reason)).
		WithContext("filter_reason", reason).
		WithSuggestion("Modify your input to comply with content policies")
}

// NewStreamError creates a streaming error.
func NewStreamError(provider string, err error) *AutomergentError {
	return Wrap(err, CodeStreamError, fmt.Sprintf("stream error from %s", provider)).
		WithContext("provider", provider).
		WithRetry(2 * time.Second)
}

// NewMalformedResponseError creates a malformed response error.
func NewMalformedResponseError(provider, reason string) *AutomergentError {
	return New(CodeMalformedResponse, fmt.Sprintf("malformed response from %s: %s", provider, reason)).
		WithContext("provider", provider).
		WithContext("reason", reason).
		WithRetry(1 * time.Second)
}

// ══════════════════════════════════════════════════════════════════════════════
// Tool Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewToolNotFoundError creates a tool not found error.
func NewToolNotFoundError(toolName string) *AutomergentError {
	return New(CodeToolNotFound, fmt.Sprintf("tool '%s' not found", toolName)).
		WithContext("tool", toolName).
		WithSuggestion("Check available tools or load the required tool")
}

// NewToolExecError creates a tool execution error.
func NewToolExecError(toolName string, err error) *AutomergentError {
	return Wrap(err, CodeToolExecFailed, fmt.Sprintf("tool '%s' execution failed", toolName)).
		WithContext("tool", toolName)
}

// NewToolTimeoutError creates a tool timeout error.
func NewToolTimeoutError(toolName string, timeout time.Duration) *AutomergentError {
	return New(CodeToolTimeout, fmt.Sprintf("tool '%s' timed out after %s", toolName, timeout)).
		WithContext("tool", toolName).
		WithContext("timeout", timeout.String()).
		WithSuggestion("Increase the tool timeout or simplify the operation")
}

// ══════════════════════════════════════════════════════════════════════════════
// Internal Errors
// ══════════════════════════════════════════════════════════════════════════════

// NewInternalError creates an internal error.
func NewInternalError(message string, err error) *AutomergentError {
	return Wrap(err, CodeInternal, message).
		WithSeverity(SeverityCritical)
}

// NewPanicError creates an error from a recovered panic.
func NewPanicError(recovered any) *AutomergentError {
	return New(CodePanic, fmt.Sprintf("panic recovered: %v", recovered)).
		WithSeverity(SeverityCritical).
		WithContext("panic_value", recovered)
}

// NewUnimplementedError creates an unimplemented feature error.
func NewUnimplementedError(feature string) *AutomergentError {
	return New(CodeUnimplemented, fmt.Sprintf("feature '%s' is not yet implemented", feature)).
		WithContext("feature", feature)
}

// NewDeprecatedError creates a deprecated feature error.
func NewDeprecatedError(feature, alternative string) *AutomergentError {
	return New(CodeDeprecated, fmt.Sprintf("feature '%s' is deprecated", feature)).
		WithContext("feature", feature).
		WithContext("alternative", alternative).
		WithSeverity(SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use '%s' instead", alternative))
}
