// Package errors provides a structured error system with AI-powered explanations
// and fix suggestions for Automergent.
package errors

// ErrorCode represents a unique typed error code.
type ErrorCode string

// Error codes grouped by category for easy reference.
const (
	// Validation errors
	CodeValidationFailed    ErrorCode = "VALIDATION_FAILED"
	CodeInvalidInput        ErrorCode = "INVALID_INPUT"
	CodeInvalidFormat       ErrorCode = "INVALID_FORMAT"
	CodeMissingRequired     ErrorCode = "MISSING_REQUIRED"
	CodeOutOfRange          ErrorCode = "OUT_OF_RANGE"
	CodeTypeMismatch        ErrorCode = "TYPE_MISMATCH"
	CodeConstraintViolation ErrorCode = "CONSTRAINT_VIOLATION"

	// IO errors
	CodeFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	CodeFileExists       ErrorCode = "FILE_EXISTS"
	CodePermissionDenied ErrorCode = "PERMISSION_DENIED"
	CodeDiskFull         ErrorCode = "DISK_FULL"
	CodeReadError        ErrorCode = "READ_ERROR"
	CodeWriteError       ErrorCode = "WRITE_ERROR"
	CodePathInvalid      ErrorCode = "PATH_INVALID"
	CodeSymlinkError     ErrorCode = "SYMLINK_ERROR"

	// Network errors
	CodeConnectionFailed  ErrorCode = "CONNECTION_FAILED"
	CodeConnectionTimeout ErrorCode = "CONNECTION_TIMEOUT"
	CodeDNSError          ErrorCode = "DNS_ERROR"
	CodeTLSError          ErrorCode = "TLS_ERROR"
	CodeHTTPError         ErrorCode = "HTTP_ERROR"
	CodeWebsocketError    ErrorCode = "WEBSOCKET_ERROR"

	// API errors
	CodeRateLimited        ErrorCode = "RATE_LIMITED"
	CodeQuotaExceeded      ErrorCode = "QUOTA_EXCEEDED"
	CodeUnauthorized       ErrorCode = "UNAUTHORIZED"
	CodeForbidden          ErrorCode = "FORBIDDEN"
	CodeNotFound           ErrorCode = "NOT_FOUND"
	CodeConflict           ErrorCode = "CONFLICT"
	CodeServerError        ErrorCode = "SERVER_ERROR"
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeBadGateway         ErrorCode = "BAD_GATEWAY"
	CodeGatewayTimeout     ErrorCode = "GATEWAY_TIMEOUT"

	// Configuration errors
	CodeConfigNotFound    ErrorCode = "CONFIG_NOT_FOUND"
	CodeConfigInvalid     ErrorCode = "CONFIG_INVALID"
	CodeConfigParseFailed ErrorCode = "CONFIG_PARSE_FAILED"
	CodeMissingEnvVar     ErrorCode = "MISSING_ENV_VAR"
	CodeInvalidEnvVar     ErrorCode = "INVALID_ENV_VAR"

	// Authentication errors
	CodeTokenExpired       ErrorCode = "TOKEN_EXPIRED"
	CodeTokenInvalid       ErrorCode = "TOKEN_INVALID"
	CodeCredentialsInvalid ErrorCode = "CREDENTIALS_INVALID"
	CodeSessionExpired     ErrorCode = "SESSION_EXPIRED"
	CodeMFARequired        ErrorCode = "MFA_REQUIRED"

	// Process errors
	CodeProcessFailed   ErrorCode = "PROCESS_FAILED"
	CodeProcessTimeout  ErrorCode = "PROCESS_TIMEOUT"
	CodeProcessKilled   ErrorCode = "PROCESS_KILLED"
	CodeCommandNotFound ErrorCode = "COMMAND_NOT_FOUND"

	// Git errors
	CodeGitNotRepo      ErrorCode = "GIT_NOT_REPO"
	CodeGitConflict     ErrorCode = "GIT_CONFLICT"
	CodeGitDetachedHead ErrorCode = "GIT_DETACHED_HEAD"
	CodeGitDirty        ErrorCode = "GIT_DIRTY"

	// AI/Provider errors
	CodeProviderError     ErrorCode = "PROVIDER_ERROR"
	CodeModelNotFound     ErrorCode = "MODEL_NOT_FOUND"
	CodeContextTooLong    ErrorCode = "CONTEXT_TOO_LONG"
	CodeContentFiltered   ErrorCode = "CONTENT_FILTERED"
	CodeStreamError       ErrorCode = "STREAM_ERROR"
	CodeMalformedResponse ErrorCode = "MALFORMED_RESPONSE"

	// Tool errors
	CodeToolNotFound   ErrorCode = "TOOL_NOT_FOUND"
	CodeToolExecFailed ErrorCode = "TOOL_EXEC_FAILED"
	CodeToolTimeout    ErrorCode = "TOOL_TIMEOUT"

	// Internal errors
	CodeInternal      ErrorCode = "INTERNAL"
	CodePanic         ErrorCode = "PANIC"
	CodeUnimplemented ErrorCode = "UNIMPLEMENTED"
	CodeDeprecated    ErrorCode = "DEPRECATED"
	CodeUnknown       ErrorCode = "UNKNOWN"
)

// Category groups related error types for routing and handling.
type Category string

const (
	CategoryValidation Category = "validation"
	CategoryIO         Category = "io"
	CategoryNetwork    Category = "network"
	CategoryAPI        Category = "api"
	CategoryConfig     Category = "config"
	CategoryAuth       Category = "auth"
	CategoryProcess    Category = "process"
	CategoryGit        Category = "git"
	CategoryAI         Category = "ai"
	CategoryTool       Category = "tool"
	CategoryInternal   Category = "internal"
	CategoryUnknown    Category = "unknown"
)

// Severity indicates how critical an error is.
type Severity string

const (
	SeverityDebug    Severity = "debug"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// codeToCategory maps error codes to their categories.
var codeToCategory = map[ErrorCode]Category{
	// Validation
	CodeValidationFailed:    CategoryValidation,
	CodeInvalidInput:        CategoryValidation,
	CodeInvalidFormat:       CategoryValidation,
	CodeMissingRequired:     CategoryValidation,
	CodeOutOfRange:          CategoryValidation,
	CodeTypeMismatch:        CategoryValidation,
	CodeConstraintViolation: CategoryValidation,

	// IO
	CodeFileNotFound:     CategoryIO,
	CodeFileExists:       CategoryIO,
	CodePermissionDenied: CategoryIO,
	CodeDiskFull:         CategoryIO,
	CodeReadError:        CategoryIO,
	CodeWriteError:       CategoryIO,
	CodePathInvalid:      CategoryIO,
	CodeSymlinkError:     CategoryIO,

	// Network
	CodeConnectionFailed:  CategoryNetwork,
	CodeConnectionTimeout: CategoryNetwork,
	CodeDNSError:          CategoryNetwork,
	CodeTLSError:          CategoryNetwork,
	CodeHTTPError:         CategoryNetwork,
	CodeWebsocketError:    CategoryNetwork,

	// API
	CodeRateLimited:        CategoryAPI,
	CodeQuotaExceeded:      CategoryAPI,
	CodeUnauthorized:       CategoryAPI,
	CodeForbidden:          CategoryAPI,
	CodeNotFound:           CategoryAPI,
	CodeConflict:           CategoryAPI,
	CodeServerError:        CategoryAPI,
	CodeServiceUnavailable: CategoryAPI,
	CodeBadGateway:         CategoryAPI,
	CodeGatewayTimeout:     CategoryAPI,

	// Config
	CodeConfigNotFound:    CategoryConfig,
	CodeConfigInvalid:     CategoryConfig,
	CodeConfigParseFailed: CategoryConfig,
	CodeMissingEnvVar:     CategoryConfig,
	CodeInvalidEnvVar:     CategoryConfig,

	// Auth
	CodeTokenExpired:       CategoryAuth,
	CodeTokenInvalid:       CategoryAuth,
	CodeCredentialsInvalid: CategoryAuth,
	CodeSessionExpired:     CategoryAuth,
	CodeMFARequired:        CategoryAuth,

	// Process
	CodeProcessFailed:   CategoryProcess,
	CodeProcessTimeout:  CategoryProcess,
	CodeProcessKilled:   CategoryProcess,
	CodeCommandNotFound: CategoryProcess,

	// Git
	CodeGitNotRepo:      CategoryGit,
	CodeGitConflict:     CategoryGit,
	CodeGitDetachedHead: CategoryGit,
	CodeGitDirty:        CategoryGit,

	// AI
	CodeProviderError:     CategoryAI,
	CodeModelNotFound:     CategoryAI,
	CodeContextTooLong:    CategoryAI,
	CodeContentFiltered:   CategoryAI,
	CodeStreamError:       CategoryAI,
	CodeMalformedResponse: CategoryAI,

	// Tool
	CodeToolNotFound:   CategoryTool,
	CodeToolExecFailed: CategoryTool,
	CodeToolTimeout:    CategoryTool,

	// Internal
	CodeInternal:      CategoryInternal,
	CodePanic:         CategoryInternal,
	CodeUnimplemented: CategoryInternal,
	CodeDeprecated:    CategoryInternal,
	CodeUnknown:       CategoryUnknown,
}

// CategoryOf returns the category for a given error code.
func CategoryOf(code ErrorCode) Category {
	if cat, ok := codeToCategory[code]; ok {
		return cat
	}
	return CategoryUnknown
}

// String returns the string representation of the error code.
func (c ErrorCode) String() string {
	return string(c)
}

// String returns the string representation of the category.
func (c Category) String() string {
	return string(c)
}

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}
