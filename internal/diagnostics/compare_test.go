package diagnostics

import (
	"testing"
)

// Regression: an edit that merely shifts a pre-existing diagnostic's line
// must not be reported as an introduction. This is the bug that made
// edit_file refuse to repair any already-broken file.
func TestCompare_LineShiftIsNotIntroduction(t *testing.T) {
	before := "package main\n\nfunc f( {\n}\n"
	after := "\npackage main\n\nfunc f( {\n}\n"

	delta := Compare(Analyze("x.go", before), Analyze("x.go", after))

	if delta.IntroducedCount != 0 {
		t.Errorf("line-shift introduced %d diagnostics: %+v", delta.IntroducedCount, delta.Introduced)
	}
	if delta.FixedCount != 0 {
		t.Errorf("line-shift reported %d fixed diagnostics: %+v", delta.FixedCount, delta.Fixed)
	}
	if !delta.IsSafe {
		t.Error("line-shift must be safe")
	}
}

// A genuinely new error must still block.
func TestCompare_NewErrorIsIntroduction(t *testing.T) {
	before := "package main\n\nfunc f() {\n}\n"
	after := "package main\n\nfunc f( {\n}\n"

	delta := Compare(Analyze("x.go", before), Analyze("x.go", after))

	if delta.IntroducedErrors == 0 {
		t.Errorf("expected introduced errors, got %+v", delta.Introduced)
	}
	if delta.IsSafe {
		t.Error("a new syntax error must not be safe")
	}
}

// Severity breakdown: a new warning alone is not a new error.
func TestCompare_SeverityBreakdown(t *testing.T) {
	before := []Diagnostic{}
	after := []Diagnostic{
		{Line: 1, Severity: "warning", Code: "w", Message: "m", Source: "s"},
		{Line: 2, Severity: "info", Code: "i", Message: "m", Source: "s"},
	}

	delta := Compare(before, after)

	if delta.IntroducedErrors != 0 {
		t.Errorf("warnings/info must not count as errors, got %d", delta.IntroducedErrors)
	}
	if delta.IntroducedWarnings != 1 {
		t.Errorf("expected 1 warning, got %d", delta.IntroducedWarnings)
	}
	if !delta.IsSafe {
		t.Error("warning-only introduction is safe")
	}
}

// Two identical diagnostics before, one after: one is fixed, not all.
func TestCompare_DuplicateMultisetSemantics(t *testing.T) {
	before := []Diagnostic{
		{Line: 1, Severity: "error", Code: "c", Message: "m", Source: "s"},
		{Line: 5, Severity: "error", Code: "c", Message: "m", Source: "s"},
	}
	after := []Diagnostic{
		{Line: 9, Severity: "error", Code: "c", Message: "m", Source: "s"},
	}

	delta := Compare(before, after)

	if delta.FixedCount != 1 {
		t.Errorf("expected 1 fixed (of 2 duplicates), got %d", delta.FixedCount)
	}
	if delta.IntroducedCount != 0 {
		t.Errorf("expected 0 introduced, got %d", delta.IntroducedCount)
	}
}

// Position changes alone (same identity) are neither fixed nor introduced.
func TestCompare_PositionChangeIsNoop(t *testing.T) {
	before := []Diagnostic{{Line: 3, Column: 2, Severity: "error", Code: "c", Message: "m", Source: "s"}}
	after := []Diagnostic{{Line: 10, Column: 4, Severity: "error", Code: "c", Message: "m", Source: "s"}}

	delta := Compare(before, after)

	if delta.FixedCount != 0 || delta.IntroducedCount != 0 {
		t.Errorf("position-only change: fixed=%d introduced=%d", delta.FixedCount, delta.IntroducedCount)
	}
	if len(delta.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(delta.Unchanged))
	}
}

func TestCompare_EmptyInputs(t *testing.T) {
	delta := Compare(nil, nil)
	if !delta.IsSafe || delta.FixedCount != 0 || delta.IntroducedCount != 0 {
		t.Errorf("empty compare: %+v", delta)
	}
}
