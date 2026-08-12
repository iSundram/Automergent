// compare.go implements DiagnosticDelta – a before/after comparison used by
// the edit_file tool to decide whether a proposed change is safe to apply.
package diagnostics

// DiagnosticDelta describes how diagnostics changed between two file versions.
type DiagnosticDelta struct {
	Fixed      []Diagnostic // Diagnostics present before but gone after.
	Introduced []Diagnostic // Diagnostics absent before but present after.
	Unchanged  []Diagnostic // Diagnostics present in both versions.

	FixedCount      int
	IntroducedCount int
	// IsSafe is true when no new errors are introduced.
	IsSafe bool
}

// Compare computes the diagnostic delta between before and after slices.
// Diagnostics are matched by their Key() value.
func Compare(before, after []Diagnostic) DiagnosticDelta {
	beforeMap := toMap(before)
	afterMap := toMap(after)

	fixed := subtract(beforeMap, afterMap)
	introduced := subtract(afterMap, beforeMap)
	unchanged := intersect(beforeMap, afterMap)

	return DiagnosticDelta{
		Fixed:           fixed,
		Introduced:      introduced,
		Unchanged:       unchanged,
		FixedCount:      len(fixed),
		IntroducedCount: len(introduced),
		IsSafe:          len(introduced) == 0,
	}
}

func toMap(diags []Diagnostic) map[string]Diagnostic {
	m := make(map[string]Diagnostic, len(diags))
	for _, d := range diags {
		m[d.Key()] = d
	}
	return m
}

// subtract returns diagnostics present in a but not in b.
func subtract(a, b map[string]Diagnostic) []Diagnostic {
	var out []Diagnostic
	for k, d := range a {
		if _, exists := b[k]; !exists {
			out = append(out, d)
		}
	}
	return out
}

// intersect returns diagnostics present in both a and b.
func intersect(a, b map[string]Diagnostic) []Diagnostic {
	var out []Diagnostic
	for k, d := range a {
		if _, exists := b[k]; exists {
			out = append(out, d)
		}
	}
	return out
}
