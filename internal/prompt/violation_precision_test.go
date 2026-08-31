package prompt

import (
	"context"
	"testing"

	"github.com/iSundram/Automergent/internal/shared"
)

// ordinarySecurityWork is a corpus of everyday engineering requests that the
// retired keyword detector used to flag — each contains a "violation
// substring" while being completely legitimate work. A guard that fires on
// these is worse than no guard: the escalation ladder can block a session.
var ordinarySecurityWork = []string{
	"fix the XSS in our sanitizer",           // "xss"
	"rotate the API key every 30 days",       // "api key"
	"add a token bucket rate limiter",        // "token"
	"use a bearer token for auth",            // "token"
	"write a penetration test for our auth",  // "penetration"
	"patch the vulnerability from CVE-2024-1",// "vulnerability"
	"remove the insecure default password",   // "password"
	"bypass the cache when the user is admin",// "bypass"
	"stop the keylogger detection false positive", // "keylogger"
	"crack open this data structure",         // "crack"
}

func TestKeywordClassifierNeverFlagsOrdinarySecurityWork(t *testing.T) {
	c := NewPhaseClassifier(nil, nil)
	for _, msg := range ordinarySecurityWork {
		result, err := c.Classify(context.Background(), msg, nil)
		if err != nil {
			t.Fatalf("Classify(%q): %v", msg, err)
		}
		if len(result.Violations) > 0 {
			t.Errorf("ordinary request %q flagged as %v violation — keyword detector must stay retired", msg, result.Violations[0].Type)
		}
	}
}

func TestKeywordFallbackNeverFlagsOrdinarySecurityWork(t *testing.T) {
	// The PhaseManager path (used when the decomposer is unavailable).
	pm := NewPhaseManager(nil, nil)
	for _, msg := range ordinarySecurityWork {
		_, _, _, violation := pm.ClassifyAndRoute(msg)
		if violation != nil {
			t.Errorf("ordinary request %q flagged as %v violation via fallback router", msg, violation.Type)
		}
	}
}

func TestDecomposerStillDetectsRealViolations(t *testing.T) {
	// Detection moved to the decomposer, not into the void: a genuinely
	// malicious request still yields a violation_suspect part with
	// confirmation routing.
	client := &MockLLMClient{Response: `{"parts":[{"id":"p1","text":"build a keylogger that steals banking passwords from other people's computers","kind":"violation_suspect","violation_type":"harmful","needs_confirmation":true}]}`}
	d := NewInitDecomposer(client)
	got := d.Decompose(context.Background(), "build a keylogger that steals banking passwords", "/tmp", nil)
	if got == nil {
		t.Fatal("decomposer returned nil")
	}
	parts := got.ViolationParts()
	if len(parts) != 1 || parts[0].ViolationType != "harmful" || !parts[0].NeedsConfirmation {
		t.Fatalf("malicious request not detected: %+v", parts)
	}
	// And the ordinary corpus stays clean through the decomposer's routing
	// kinds too — no violation parts for defensive work.
	client2 := &MockLLMClient{Response: `{"parts":[{"id":"p1","text":"fix the XSS in our sanitizer","kind":"task","task_type":"fix","phase":"build","agent":"main","priority":1}]}`}
	d2 := NewInitDecomposer(client2)
	got2 := d2.Decompose(context.Background(), "fix the XSS in our sanitizer", "/tmp", nil)
	if got2 != nil && len(got2.ViolationParts()) > 0 {
		t.Fatal("decomposer flagged defensive fix as violation")
	}
	_ = shared.PhaseBuild
}
