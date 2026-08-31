package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"github.com/iSundram/Automergent/internal/tui/tips"
)

func TestRecordAPIErrorRingBufferCaps(t *testing.T) {
	app := newTestApp(t)
	for i := 0; i < maxAPIErrors+50; i++ {
		app.recordAPIError(apiErrorRecord{Message: fmt.Sprintf("failure %d", i)})
	}
	if got := len(app.apiErrors); got != maxAPIErrors {
		t.Errorf("ring buffer holds %d entries, want %d", got, maxAPIErrors)
	}
	// The oldest entries are the ones evicted.
	if first := app.apiErrors[0].Message; !strings.Contains(first, "failure 50") {
		t.Errorf("oldest retained entry = %q, want failure 50", first)
	}
}

// TestAPIKeysNeverEnterTheLog: error text from a provider can embed the request
// URL, which carries the key. The log is user-visible and long-lived, so this
// must be scrubbed on the way in, not at display time.
func TestAPIKeysNeverEnterTheLog(t *testing.T) {
	app := newTestApp(t)
	app.recordAPIError(apiErrorRecord{
		Message:    "POST https://api.example.com/v1?key=SUPERSECRET123 failed",
		Suggestion: "retry https://api.example.com/v1?api_key=ALSOSECRET",
		Resource:   "https://api.example.com/v1?key=SUPERSECRET123",
	})
	rec := app.apiErrors[0]
	for field, value := range map[string]string{
		"Message":    rec.Message,
		"Suggestion": rec.Suggestion,
		"Resource":   rec.Resource,
	} {
		if strings.Contains(value, "SUPERSECRET123") || strings.Contains(value, "ALSOSECRET") {
			t.Errorf("%s leaked a credential: %q", field, value)
		}
		if !strings.Contains(value, "***") {
			t.Errorf("%s should be redacted, got %q", field, value)
		}
	}
}

func TestHandleRetryEventPopulatesFooterState(t *testing.T) {
	app := newTestApp(t)
	app.handleRetryEvent(ai.RetryInfo{
		Provider: "google", Model: "gemini-3.6-flash",
		Code: "SERVICE_UNAVAILABLE", Status: "529",
		Message: "The model is overloaded. Please try again later.",
		Attempt: 3, MaxAttempts: 10, Delay: 8 * time.Second,
	})

	if !app.retrying {
		t.Fatal("expected retrying state")
	}
	if app.retryAttempt != 3 || app.retryMax != 10 {
		t.Errorf("attempt %d/%d, want 3/10", app.retryAttempt, app.retryMax)
	}
	if app.retryCode != "529" {
		t.Errorf("retryCode = %q, want the transport status 529", app.retryCode)
	}
	if app.retryDetail != "overloaded" {
		t.Errorf("retryDetail = %q, want %q", app.retryDetail, "overloaded")
	}
	if status := app.statusBar.Status(); !strings.Contains(status, "3/10") {
		t.Errorf("footer activity should count attempts, got %q", status)
	}
	if app.uiState() != tips.StateRetrying {
		t.Errorf("uiState = %q, want %q", app.uiState(), tips.StateRetrying)
	}
}

func TestRetryInfoLineNamesCodeAndCountdown(t *testing.T) {
	app := sizedTestApp(t)
	app.handleRetryEvent(ai.RetryInfo{
		Status: "429", Message: "rate limit exceeded",
		Attempt: 2, MaxAttempts: 10, Delay: 4 * time.Second,
	})
	app.refreshChrome()

	line := app.infoLine.Text()
	for _, want := range []string{"429", "2/10", "/error"} {
		if !strings.Contains(line, want) {
			t.Errorf("info line %q missing %q", line, want)
		}
	}
}

func TestTokensClearRetryState(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.handleRetryEvent(ai.RetryInfo{Status: "529", Attempt: 1, MaxAttempts: 10})
	if !app.retrying {
		t.Fatal("expected retrying")
	}
	// A token arriving proves the request went through.
	app.handleAgentEvent(agent.Event{Type: agent.EventToken, Payload: "hello"})
	if app.retrying {
		t.Error("a streamed token should end the retrying state")
	}
}

// TestTerminalErrorUsesStructuredClassification: the provider already
// classified the failure, so the log must read the code off the error rather
// than re-deriving it by grepping the message text.
func TestTerminalErrorUsesStructuredClassification(t *testing.T) {
	app := newTestApp(t)
	provErr := automergentErrors.New(automergentErrors.CodeRateLimited, "quota exhausted for this key").
		WithContext("status_code", 429).
		WithSuggestion("Wait for the quota window to reset").
		WithResource("https://api.example.com/v1")
	wrapped := fmt.Errorf("agent: complete: %w", provErr)

	app.recordTerminalAPIError(wrapped)

	rec, ok := app.latestAPIError()
	if !ok {
		t.Fatal("no record was created")
	}
	if rec.Code != string(automergentErrors.CodeRateLimited) {
		t.Errorf("Code = %q, want %q", rec.Code, automergentErrors.CodeRateLimited)
	}
	if rec.Status != "429" {
		t.Errorf("Status = %q, want 429 from the error context", rec.Status)
	}
	if rec.Suggestion == "" {
		t.Error("the provider's suggestion should be carried into the record")
	}
	if rec.Retrying {
		t.Error("a terminal failure must not be marked as retrying")
	}
	// displayCode prefers the status because that is what users recognise.
	if got := rec.displayCode(); got != "429" {
		t.Errorf("displayCode() = %q, want 429", got)
	}
}

func TestTerminalErrorFallsBackToStringInspection(t *testing.T) {
	app := newTestApp(t)
	// An unclassified error, e.g. raised outside the provider.
	app.recordTerminalAPIError(errors.New("dial tcp: connection refused after 503"))

	rec, _ := app.latestAPIError()
	if rec.Status != "503" {
		t.Errorf("Status = %q, want 503 extracted from the text", rec.Status)
	}
	if rec.Detail != "connection failed" {
		t.Errorf("Detail = %q, want %q", rec.Detail, "connection failed")
	}
}

func TestTerminalErrorAttributesRetryAttempts(t *testing.T) {
	app := newTestApp(t)
	app.handleRetryEvent(ai.RetryInfo{Status: "529", Attempt: 9, MaxAttempts: 10})
	app.recordTerminalAPIError(errors.New("529 overloaded"))

	rec, _ := app.latestAPIError()
	if rec.MaxAttempts != 10 {
		t.Errorf("MaxAttempts = %d, want 10 from the retry sequence", rec.MaxAttempts)
	}
}

func TestRecordTerminalAPIErrorIgnoresNil(t *testing.T) {
	app := newTestApp(t)
	app.recordTerminalAPIError(nil)
	if len(app.apiErrors) != 0 {
		t.Error("a nil error should not create a record")
	}
}

func TestAPIErrorsReturnedNewestFirst(t *testing.T) {
	app := newTestApp(t)
	app.recordAPIError(apiErrorRecord{Message: "oldest"})
	app.recordAPIError(apiErrorRecord{Message: "newest"})

	got := app.APIErrors()
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if !strings.Contains(got[0].Message, "newest") {
		t.Errorf("first entry = %q, want the newest", got[0].Message)
	}
}

func TestClearAPIErrors(t *testing.T) {
	app := newTestApp(t)
	app.recordAPIError(apiErrorRecord{Message: "boom"})
	app.retrying = true

	app.ClearAPIErrors()

	if len(app.APIErrors()) != 0 {
		t.Error("history should be empty after ClearAPIErrors")
	}
	if app.retrying {
		t.Error("clearing should also reset live retry state")
	}
}

func TestErrorEventSetsStickyOutcome(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true

	app.handleAgentEvent(agent.Event{
		Type:    agent.EventError,
		Payload: errors.New("500 internal server error"),
	})

	if app.thinking {
		t.Error("an error should end the run")
	}
	if app.lastOutcome != outcomeError {
		t.Errorf("outcome = %q, want %q", app.lastOutcome, outcomeError)
	}
	if app.uiState() != tips.StateError {
		t.Errorf("uiState = %q, want %q", app.uiState(), tips.StateError)
	}
	if len(app.apiErrors) == 0 {
		t.Error("the failure should have been recorded for /error")
	}
}

// TestCancellationIsNotRecordedAsAPIError: the user cancelling is not a
// provider failure and must not pollute the error log.
func TestCancellationIsNotRecordedAsAPIError(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true

	app.handleAgentEvent(agent.Event{
		Type:    agent.EventError,
		Payload: errors.New("context canceled"),
	})

	if len(app.apiErrors) != 0 {
		t.Errorf("cancellation should not be logged as an API error, got %d entries", len(app.apiErrors))
	}
	if app.lastOutcome != outcomeCancelled {
		t.Errorf("outcome = %q, want %q", app.lastOutcome, outcomeCancelled)
	}
}

func TestDoneEventClearsStickyOutcome(t *testing.T) {
	app := sizedTestApp(t)
	app.lastOutcome = outcomeInterrupted
	app.thinking = true

	app.handleAgentEvent(agent.Event{Type: agent.EventDone, Payload: "all finished"})

	if app.lastOutcome != outcomeNone {
		t.Errorf("a completed run should clear the outcome badge, got %q", app.lastOutcome)
	}
}

func TestRetryDetailFor(t *testing.T) {
	cases := []struct {
		info ai.RetryInfo
		want string
	}{
		{ai.RetryInfo{Message: "The model is overloaded"}, "overloaded"},
		{ai.RetryInfo{Message: "quota exceeded for project"}, "quota exceeded"},
		{ai.RetryInfo{Status: "429"}, "rate limited"},
		{ai.RetryInfo{Message: "context deadline exceeded"}, "timed out"},
		{ai.RetryInfo{Code: "SERVICE_UNAVAILABLE"}, "service unavailable"},
		{ai.RetryInfo{Message: "something unrecognised"}, ""},
	}
	for _, c := range cases {
		if got := retryDetailFor(c.info); got != c.want {
			t.Errorf("retryDetailFor(%+v) = %q, want %q", c.info, got, c.want)
		}
	}
}

func TestOutcomeBadgeMapping(t *testing.T) {
	app := newTestApp(t)
	cases := map[string]string{
		outcomeNone:        "",
		outcomeInterrupted: "CANCELLED",
		outcomeCancelled:   "CANCELLED",
		outcomeError:       "ERROR",
	}
	for outcome, want := range cases {
		app.lastOutcome = outcome
		if got := app.outcomeBadge(); got != want {
			t.Errorf("outcome %q → badge %q, want %q", outcome, got, want)
		}
	}
}

func TestRecordAPIErrorDeduplicatesSameFailure(t *testing.T) {
	app := newTestApp(t)
	base := apiErrorRecord{Code: "RATE_LIMITED", Status: "429", Message: "quota exceeded"}

	// The same failure arrives via the retry observer (attempt 1), again at
	// attempt 2, and finally as the terminal error — it must read as one
	// entry with the latest attempt count.
	app.recordAPIError(apiErrorRecord{Code: base.Code, Status: base.Status, Message: base.Message, Attempt: 1, MaxAttempts: 10, Retrying: true})
	app.recordAPIError(apiErrorRecord{Code: base.Code, Status: base.Status, Message: base.Message, Attempt: 2, MaxAttempts: 10, Retrying: true})
	app.recordAPIError(apiErrorRecord{Code: base.Code, Status: base.Status, Message: base.Message, Attempt: 2, MaxAttempts: 10, Retrying: false})

	if len(app.apiErrors) != 1 {
		t.Fatalf("one failure must read as one entry, got %d", len(app.apiErrors))
	}
	if app.apiErrors[0].Attempt != 2 {
		t.Fatalf("attempt count must advance in place, got %d", app.apiErrors[0].Attempt)
	}

	// A different failure is a new entry.
	app.recordAPIError(apiErrorRecord{Code: "UNAVAILABLE", Message: "connection reset"})
	if len(app.apiErrors) != 2 {
		t.Fatalf("a distinct failure must append, got %d", len(app.apiErrors))
	}
}
