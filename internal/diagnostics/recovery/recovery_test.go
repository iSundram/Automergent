package recovery

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

func TestClassifyDiagnosticSyntax(t *testing.T) {
	diag := types.Diagnostic{Code: "syntax-error", Message: "syntax error: unexpected }", Source: "tree-sitter-go"}
	got := ClassifyDiagnostic(diag)
	if got.Cause != CauseSyntax {
		t.Fatalf("cause = %s, want %s", got.Cause, CauseSyntax)
	}
	if got.Confidence < 0.8 {
		t.Fatalf("expected strong confidence, got %v", got.Confidence)
	}
	if len(got.FixSuggestions) == 0 {
		t.Fatal("expected fix suggestions")
	}
}

func TestClassifyDiagnosticTransientRetry(t *testing.T) {
	diag := types.Diagnostic{Code: "tool-timeout", Message: "request timeout, try again", Source: "tool"}
	got := ClassifyDiagnostic(diag)
	if !got.Retry.Retryable {
		t.Fatal("expected retryable transient diagnostic")
	}
	if got.Retry.MaxAttempts != 3 {
		t.Fatalf("max attempts = %d, want 3", got.Retry.MaxAttempts)
	}
}

func TestRetryPolicyDelayUsesJitter(t *testing.T) {
	policy := RetryPolicy{InitialDelay: time.Second, MaxDelay: 10 * time.Second, Multiplier: 2, Jitter: 0.5}
	rng := rand.New(rand.NewSource(1))
	delay := policy.Delay(2, rng)
	if delay < 500*time.Millisecond || delay > 3*time.Second {
		t.Fatalf("delay out of expected jitter range: %s", delay)
	}
}

func TestSummarizeBuildsUserMessage(t *testing.T) {
	report := Summarize([]types.Diagnostic{
		{Code: "missing-package", Message: "Go files must start with a package declaration", Source: "tree-sitter-go"},
		{Code: "syntax-error", Message: "syntax error", Source: "tree-sitter-go"},
	})
	if report.UserMessage == "" {
		t.Fatal("expected user message")
	}
	if !strings.Contains(report.UserMessage, "Next steps:") {
		t.Fatalf("missing next steps in %q", report.UserMessage)
	}
	if report.Primary.Cause != CauseSyntax {
		t.Fatalf("primary cause = %s, want %s", report.Primary.Cause, CauseSyntax)
	}
}

func TestRetryPolicyAdapterDecision(t *testing.T) {
	policy := RetryPolicy{
		Retryable:    true,
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
		Multiplier:   2,
	}

	adapted := policy.AsRecoveryPolicy()
	if d := adapted.Decide(1, errSentinel{}); !d.Retry {
		t.Fatalf("expected retry decision, got %+v", d)
	}
	if d := adapted.Decide(3, errSentinel{}); d.Retry {
		t.Fatalf("expected retry stop, got %+v", d)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "boom" }
