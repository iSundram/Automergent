// compare.go implements DiagnosticDelta – a before/after comparison used by
// the edit_file tool to decide whether a proposed change is safe to apply.
package diagnostics

import "strings"

// DiagnosticDelta describes how diagnostics changed between two file versions.
type DiagnosticDelta struct {
	Fixed      []Diagnostic // Diagnostics present before but gone after.
	Introduced []Diagnostic // Diagnostics absent before but present after.
	Unchanged  []Diagnostic // Diagnostics present in both versions.

	FixedCount      int
	IntroducedCount int
	// Severity breakdown of Introduced, for callers that gate on severity
	// (e.g. block on errors but not warnings).
	IntroducedErrors   int
	IntroducedWarnings int
	// IsSafe is true when no new errors (severity "error") are introduced.
	IsSafe bool
}

// Compare computes the diagnostic delta between before and after slices.
//
// Diagnostics are matched by code, source, and message – deliberately
// position-insensitive. A pre-existing diagnostic that merely shifts lines
// (because the edit inserted or removed lines above it) must not be reported
// as newly introduced, or every edit to a file that already has problems
// would be blocked. Duplicate diagnostics with the same identity are matched
// with multiset semantics.
func Compare(before, after []Diagnostic) DiagnosticDelta {
	beforeMap := groupByIdentity(before)
	afterMap := groupByIdentity(after)

	var fixed, introduced, unchanged []Diagnostic
	for k, bs := range beforeMap {
		as := afterMap[k]
		if len(bs) > len(as) {
			unchanged = append(unchanged, as...)
			fixed = append(fixed, bs[len(as):]...)
		} else {
			unchanged = append(unchanged, bs...)
		}
	}
	for k, as := range afterMap {
		if extra := len(as) - len(beforeMap[k]); extra > 0 {
			introduced = append(introduced, as[len(as)-extra:]...)
		}
	}

	delta := DiagnosticDelta{
		Fixed:           fixed,
		Introduced:      introduced,
		Unchanged:       unchanged,
		FixedCount:      len(fixed),
		IntroducedCount: len(introduced),
	}
	for _, d := range introduced {
		switch d.Severity {
		case "error":
			delta.IntroducedErrors++
		case "warning":
			delta.IntroducedWarnings++
		}
	}
	delta.IsSafe = delta.IntroducedErrors == 0
	return delta
}

// identityKey identifies a diagnostic independent of its position, so edits
// that shift lines do not fabricate introductions.
func identityKey(d Diagnostic) string {
	return d.Code + "\x00" + d.Source + "\x00" + strings.TrimSpace(d.Message)
}

func groupByIdentity(diags []Diagnostic) map[string][]Diagnostic {
	m := make(map[string][]Diagnostic, len(diags))
	for _, d := range diags {
		k := identityKey(d)
		m[k] = append(m[k], d)
	}
	return m
}
