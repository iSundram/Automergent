package agent

import "testing"

func TestIsValidOnlyPlanAndEdit(t *testing.T) {
	if !IsValid("plan") {
		t.Fatalf("expected plan mode to be valid")
	}
	if !IsValid("edit") {
		t.Fatalf("expected edit mode to be valid")
	}
	if IsValid("suggest") {
		t.Fatalf("expected suggest mode to be invalid")
	}
}

func TestAllModesContainsCanonicalModes(t *testing.T) {
	modes := AllModes()
	// The canonical cycle: legacy "edit" is normalised to "manual" before it
	// ever reaches this list.
	want := []string{"manual", "accept-edits", "auto", "plan"}
	if len(modes) != len(want) {
		t.Fatalf("expected %d modes, got %d: %#v", len(want), len(modes), modes)
	}
	for i, m := range want {
		if modes[i] != m {
			t.Fatalf("unexpected modes: %#v", modes)
		}
	}
}
