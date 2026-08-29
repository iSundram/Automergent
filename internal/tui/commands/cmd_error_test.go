package commands

import (
	"strings"
	"testing"
	"time"
)

func TestErrorCommandWithNoHistory(t *testing.T) {
	host := NewMockHost()
	handleErrors(host, nil)

	if len(host.systemMessages) != 1 {
		t.Fatalf("expected one message, got %d", len(host.systemMessages))
	}
	if !strings.Contains(host.systemMessages[0], "No API errors") {
		t.Errorf("unexpected message: %q", host.systemMessages[0])
	}
}

func TestErrorCommandListsNewestFirstWithCounts(t *testing.T) {
	host := NewMockHost()
	host.apiErrors = []APIErrorInfo{
		{
			At: time.Now().Add(-30 * time.Second), Code: "429", Detail: "rate limited",
			Message: "rate limit exceeded", Attempt: 3, MaxAttempts: 10, Retrying: true,
		},
		{
			At: time.Now().Add(-90 * time.Second), Code: "500", Detail: "server error",
			Message: "internal error", Attempt: 10, MaxAttempts: 10, Retrying: false,
		},
	}

	handleErrors(host, nil)

	out := host.systemMessages[0]
	if !strings.Contains(out, "API errors this session: 2") {
		t.Errorf("missing total count: %q", out)
	}
	if !strings.Contains(out, "1 retried") || !strings.Contains(out, "1 final") {
		t.Errorf("missing retried/final split: %q", out)
	}
	if !strings.Contains(out, "retry 3/10") {
		t.Errorf("retried entry should show its attempt: %q", out)
	}
	if !strings.Contains(out, "failed after 10 attempts") {
		t.Errorf("terminal entry should show the attempt count: %q", out)
	}
	if !strings.Contains(out, "/error clear") {
		t.Errorf("footer hint missing: %q", out)
	}
}

func TestErrorCommandDetailView(t *testing.T) {
	host := NewMockHost()
	host.apiErrors = []APIErrorInfo{{
		At: time.Now(), Code: "529", Detail: "overloaded",
		Message:    "The model is overloaded. Please try again later.",
		Suggestion: "Wait a moment and retry",
		RequestID:  "req-abc123",
		Provider:   "google", Model: "gemini-3.6-flash",
		Attempt: 10, MaxAttempts: 10,
	}}

	handleErrors(host, []string{"1"})

	out := host.systemMessages[0]
	for _, want := range []string{
		"529", "overloaded", "google", "gemini-3.6-flash",
		"req-abc123", "Wait a moment and retry", "overloaded",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail view missing %q:\n%s", want, out)
		}
	}
}

func TestErrorCommandDetailOutOfRange(t *testing.T) {
	host := NewMockHost()
	host.apiErrors = []APIErrorInfo{{Code: "429"}}

	handleErrors(host, []string{"7"})

	if len(host.errorMessages) != 1 {
		t.Fatalf("expected an error message, got %v", host.errorMessages)
	}
	if !strings.Contains(host.errorMessages[0], "no error 7") {
		t.Errorf("unexpected error: %q", host.errorMessages[0])
	}
}

func TestErrorCommandRejectsNonNumericArg(t *testing.T) {
	host := NewMockHost()
	handleErrors(host, []string{"bogus"})

	if len(host.usageMessages) != 1 {
		t.Fatalf("expected a usage message, got %v", host.usageMessages)
	}
	if !strings.Contains(host.usageMessages[0], "/error") {
		t.Errorf("usage should name the command: %q", host.usageMessages[0])
	}
}

func TestErrorCommandClear(t *testing.T) {
	host := NewMockHost()
	host.apiErrors = []APIErrorInfo{{Code: "429"}, {Code: "500"}}

	handleErrors(host, []string{"clear"})

	if host.clearAPIErrorCalls != 1 {
		t.Errorf("ClearAPIErrors called %d times, want 1", host.clearAPIErrorCalls)
	}
	if len(host.apiErrors) != 0 {
		t.Error("history should be empty after clear")
	}
	if !strings.Contains(host.systemMessages[0], "cleared") {
		t.Errorf("unexpected message: %q", host.systemMessages[0])
	}
}

func TestErrorCommandIsRegistered(t *testing.T) {
	r := Default()
	cmd, ok := r.Lookup("error")
	if !ok {
		t.Fatal("/error is not registered")
	}
	if !cmd.Immediate {
		t.Error("/error should be Immediate: it takes no confirmation step")
	}
	if !cmd.SupportsHeadless {
		t.Error("/error should work headless")
	}
	if _, ok := r.Lookup("errors"); !ok {
		t.Error("the /errors alias is not registered")
	}
}

func TestHumanAge(t *testing.T) {
	cases := map[time.Duration]string{
		12 * time.Second: "12s",
		4 * time.Minute:  "4m",
		2 * time.Hour:    "2h",
	}
	for in, want := range cases {
		if got := humanAge(in); got != want {
			t.Errorf("humanAge(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("\n\n  real line  \nsecond"); got != "real line" {
		t.Errorf("firstLine() = %q", got)
	}
	if got := firstLine("   "); got != "" {
		t.Errorf("blank input should give empty, got %q", got)
	}
}

func TestModeCommandListsAllModes(t *testing.T) {
	host := NewMockHost()
	host.mode = "manual"

	handleMode(host, nil)

	out := host.systemMessages[0]
	for _, mode := range []string{"manual", "accept-edits", "auto", "plan"} {
		if !strings.Contains(out, mode) {
			t.Errorf("/mode listing missing %q:\n%s", mode, out)
		}
	}
	if !strings.Contains(out, "shift+tab") {
		t.Errorf("/mode should mention the shift+tab cycle:\n%s", out)
	}
}

func TestModeCommandAcceptsNewModes(t *testing.T) {
	for _, mode := range []string{"manual", "accept-edits", "auto", "plan"} {
		host := NewMockHost()
		handleMode(host, []string{mode})

		if len(host.errorMessages) != 0 {
			t.Errorf("mode %q was rejected: %v", mode, host.errorMessages)
			continue
		}
		if len(host.setModeCalls) != 1 || host.setModeCalls[0] != mode {
			t.Errorf("mode %q: SetMode calls = %v", mode, host.setModeCalls)
		}
	}
}

// TestModeCommandCanonicalisesLegacyEdit: "edit" still has to be accepted, but
// it must be stored under its current name so the approval gate agrees.
func TestModeCommandCanonicalisesLegacyEdit(t *testing.T) {
	host := NewMockHost()
	handleMode(host, []string{"edit"})

	if len(host.errorMessages) != 0 {
		t.Fatalf(`"edit" should still be accepted: %v`, host.errorMessages)
	}
	if len(host.setModeCalls) != 1 || host.setModeCalls[0] != "manual" {
		t.Errorf(`"edit" should canonicalise to "manual", got %v`, host.setModeCalls)
	}
}

func TestModeCommandRejectsUnknown(t *testing.T) {
	host := NewMockHost()
	handleMode(host, []string{"turbo"})

	if len(host.errorMessages) != 1 {
		t.Fatalf("expected a rejection, got %v", host.errorMessages)
	}
	if len(host.setModeCalls) != 0 {
		t.Error("an invalid mode must not be applied")
	}
}
