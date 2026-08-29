package app

// API error history backing the /error command and the retry indicator.
//
// The provider already retries transient failures up to ten times
// (internal/ai/google/client.go), but before this those attempts were invisible:
// a request being retried looked identical to one that had hung, and a terminal
// failure left one throwaway line in the transcript with no code and no history.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
)

// maxAPIErrors bounds the error ring buffer. Enough to cover a long run's
// worth of rate limiting without growing without limit.
const maxAPIErrors = 100

// Sticky run outcomes shown in the footer until the next run starts.
const (
	outcomeNone        = ""
	outcomeInterrupted = "interrupted"
	outcomeCancelled   = "cancelled"
	outcomeError       = "error"
)

// apiErrorRecord is one observed API failure: either a retried attempt or a
// terminal failure.
type apiErrorRecord struct {
	At      time.Time
	Code    string
	Status  string
	Message string
	Detail  string

	// Suggestion is the provider's own remediation hint, when it supplied one.
	Suggestion string
	// RequestID and Resource help correlate a failure with provider-side logs.
	RequestID string
	Resource  string

	Provider string
	Model    string

	Attempt     int
	MaxAttempts int

	// Retrying distinguishes "this attempt failed, another follows" from
	// "this request is dead".
	Retrying bool
}

// displayCode returns the most specific identifier available for the failure,
// preferring the transport status because that is what users recognise ("429").
func (r apiErrorRecord) displayCode() string {
	switch {
	case r.Status != "":
		return r.Status
	case r.Code != "":
		return r.Code
	default:
		return "error"
	}
}

// recordAPIError appends to the ring buffer, evicting the oldest entry when
// full. Messages are sanitized on the way in so an API key embedded in a
// request URL can never be persisted or re-displayed.
func (a *App) recordAPIError(rec apiErrorRecord) {
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	rec.Message = sanitizeURLs(rec.Message)
	rec.Detail = sanitizeURLs(rec.Detail)
	rec.Suggestion = sanitizeURLs(rec.Suggestion)
	rec.Resource = sanitizeURLs(rec.Resource)

	a.apiErrors = append(a.apiErrors, rec)
	if len(a.apiErrors) > maxAPIErrors {
		a.apiErrors = a.apiErrors[len(a.apiErrors)-maxAPIErrors:]
	}
}

// latestAPIError returns the most recent record, if any.
func (a *App) latestAPIError() (apiErrorRecord, bool) {
	if len(a.apiErrors) == 0 {
		return apiErrorRecord{}, false
	}
	return a.apiErrors[len(a.apiErrors)-1], true
}

// handleRetryEvent records a retried attempt and puts the UI into the retrying
// state so the footer counts attempts instead of appearing to hang.
func (a *App) handleRetryEvent(info ai.RetryInfo) {
	detail := retryDetailFor(info)
	a.recordAPIError(apiErrorRecord{
		Code:        info.Code,
		Status:      info.Status,
		Message:     info.Message,
		Detail:      detail,
		Provider:    info.Provider,
		Model:       info.Model,
		Attempt:     info.Attempt,
		MaxAttempts: info.MaxAttempts,
		Retrying:    true,
	})

	a.retrying = true
	a.retryAttempt = info.Attempt
	a.retryMax = info.MaxAttempts
	a.retryCode = firstNonEmpty(info.Status, info.Code)
	a.retryDetail = detail
	a.retryDelay = info.Delay
	a.retryDelayAt = time.Now()

	label := fmt.Sprintf("retrying (%d/%d)", info.Attempt, info.MaxAttempts)
	a.statusBar.SetStatus(label)
	a.spin.SetLabel(label)
}

// clearRetryState leaves the retrying state, called when a request finally
// succeeds or fails for good.
func (a *App) clearRetryState() {
	a.retrying = false
	a.retryAttempt = 0
	a.retryMax = 0
	a.retryCode = ""
	a.retryDetail = ""
	a.retryDelay = 0
	a.retryDelayAt = time.Time{}
}

// retryDetailFor derives a short human-readable qualifier from a retry report,
// so the info line can say "529 overloaded" rather than just a bare number.
func retryDetailFor(info ai.RetryInfo) string {
	msg := strings.ToLower(info.Message)
	switch {
	case strings.Contains(msg, "overloaded"):
		return "overloaded"
	case strings.Contains(msg, "quota"):
		return "quota exceeded"
	case strings.Contains(msg, "rate limit"), info.Status == "429":
		return "rate limited"
	case strings.Contains(msg, "deadline"), strings.Contains(msg, "timeout"):
		return "timed out"
	case strings.Contains(msg, "connection"), strings.Contains(msg, "no such host"):
		return "connection failed"
	case strings.Contains(msg, "unavailable"), info.Status == "503":
		return "service unavailable"
	}
	switch info.Code {
	case "RATE_LIMITED":
		return "rate limited"
	case "QUOTA_EXCEEDED":
		return "quota exceeded"
	case "SERVICE_UNAVAILABLE":
		return "service unavailable"
	case "SERVER_ERROR":
		return "server error"
	}
	return ""
}

// recordTerminalAPIError files a run-ending failure in the history.
//
// The provider has already classified most failures into an AutomergentError
// with a code, a transport status and often a remediation suggestion — so the
// structured value is read first, and string inspection is only a fallback for
// errors raised outside the provider (context cancellation, tool plumbing).
func (a *App) recordTerminalAPIError(err error) {
	if err == nil {
		return
	}
	errStr := err.Error()
	rec := apiErrorRecord{
		Message:  errStr,
		Provider: a.cfg.Provider,
		Model:    a.cfg.Model,
		Retrying: false,
	}

	var oce *automergentErrors.AutomergentError
	if errors.As(err, &oce) && oce != nil {
		rec.Code = string(oce.Code)
		rec.Message = oce.Message
		rec.Suggestion = oce.Suggestion
		rec.RequestID = oce.RequestID
		rec.Resource = oce.Resource
		if status, ok := oce.Context["status_code"]; ok {
			rec.Status = fmt.Sprint(status)
		}
		if rec.Message == "" {
			rec.Message = errStr
		}
	}
	if rec.Status == "" {
		rec.Status = extractHTTPStatus(errStr)
	}
	rec.Detail = terminalDetailFor(errStr)
	if rec.Detail == "" && oce != nil {
		rec.Detail = detailForCode(string(oce.Code))
	}

	// Attribute the attempt count from the retry sequence that led here, so
	// "failed after 10 attempts" is accurate rather than assumed.
	if a.retryMax > 0 {
		rec.Attempt = a.retryMax
		rec.MaxAttempts = a.retryMax
	} else {
		rec.Attempt = 1
		rec.MaxAttempts = 1
	}
	a.recordAPIError(rec)
	a.checkFailureThreshold()
}

// detailForCode maps a classified error code to a short qualifier, for errors
// whose message text does not itself say what went wrong.
func detailForCode(code string) string {
	switch code {
	case "RATE_LIMITED":
		return "rate limited"
	case "QUOTA_EXCEEDED":
		return "quota exceeded"
	case "SERVICE_UNAVAILABLE":
		return "service unavailable"
	case "SERVER_ERROR", "BAD_GATEWAY", "GATEWAY_TIMEOUT":
		return "server error"
	case "UNAUTHORIZED":
		return "authentication failed"
	case "FORBIDDEN":
		return "forbidden"
	case "CONNECTION_FAILED", "CONNECTION_TIMEOUT":
		return "connection failed"
	case "STREAM_ERROR":
		return "stream interrupted"
	case "CONFIG_INVALID":
		return "provider misconfigured"
	case "INVALID_INPUT", "VALIDATION_FAILED":
		return "invalid request"
	}
	return ""
}

// extractHTTPStatus pulls a recognisable HTTP status out of an error string.
func extractHTTPStatus(errStr string) string {
	for _, code := range []string{"429", "529", "503", "502", "504", "500", "401", "403", "404", "400"} {
		if strings.Contains(errStr, code) {
			return code
		}
	}
	return ""
}

// terminalDetailFor summarises a terminal failure in a couple of words.
func terminalDetailFor(errStr string) string {
	s := strings.ToLower(errStr)
	switch {
	case strings.Contains(s, "authentication") || strings.Contains(s, "401"):
		return "authentication failed"
	case strings.Contains(s, "403"):
		return "forbidden"
	case strings.Contains(s, "quota"):
		return "quota exceeded"
	case strings.Contains(s, "429") || strings.Contains(s, "rate limit"):
		return "rate limited"
	case strings.Contains(s, "overloaded") || strings.Contains(s, "529"):
		return "overloaded"
	case strings.Contains(s, "deadline") || strings.Contains(s, "timeout"):
		return "timed out"
	case strings.Contains(s, "connection refused") || strings.Contains(s, "no such host"):
		return "connection failed"
	}
	return ""
}

// outcomeBadge maps the sticky run outcome onto the footer badge text.
func (a *App) outcomeBadge() string {
	switch a.lastOutcome {
	case outcomeInterrupted, outcomeCancelled:
		return components.OutcomeCancelled
	case outcomeError:
		return components.OutcomeError
	default:
		return ""
	}
}

// APIErrors implements commands.Host: the /error command's data source.
func (a *App) APIErrors() []commands.APIErrorInfo {
	out := make([]commands.APIErrorInfo, 0, len(a.apiErrors))
	// Newest first: when something just broke, that is what the user wants.
	for i := len(a.apiErrors) - 1; i >= 0; i-- {
		rec := a.apiErrors[i]
		out = append(out, commands.APIErrorInfo{
			At:          rec.At,
			Code:        rec.displayCode(),
			Detail:      rec.Detail,
			Message:     rec.Message,
			Suggestion:  rec.Suggestion,
			RequestID:   rec.RequestID,
			Provider:    rec.Provider,
			Model:       rec.Model,
			Attempt:     rec.Attempt,
			MaxAttempts: rec.MaxAttempts,
			Retrying:    rec.Retrying,
		})
	}
	return out
}

// ClearAPIErrors implements commands.Host: empties the error history.
func (a *App) ClearAPIErrors() {
	a.apiErrors = nil
	a.clearRetryState()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// failureThresholdForModel is the number of consecutive terminal failures
// on the same model that triggers the Model Hub failure alert.
const failureThresholdForModel = 5

// checkFailureThreshold counts consecutive terminal failures for the current
// model and triggers the Model Hub failure alert when the threshold is reached.
func (a *App) checkFailureThreshold() {
	if a.modelHub == nil || !a.modelHub.Visible() {
		return
	}
	provider := a.cfg.Provider
	model := a.cfg.Model
	if provider == "" || model == "" {
		return
	}
	// Walk backwards through the error log counting consecutive terminal
	// failures for this provider/model pair.
	consecutive := 0
	var lastCode string
	for i := len(a.apiErrors) - 1; i >= 0; i-- {
		rec := a.apiErrors[i]
		if rec.Provider != provider || rec.Model != model {
			break
		}
		if rec.Retrying {
			// A retrying record means the final outcome is still unknown;
			// count it but keep looking.
		}
		consecutive++
		lastCode = rec.displayCode()
	}
	if consecutive >= failureThresholdForModel {
		a.modelHub.ShowFailureAlert(model, provider, consecutive, lastCode)
	}
}
