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

func TestAllModesContainsTwoModes(t *testing.T) {
	modes := AllModes()
	if len(modes) != 2 {
		t.Fatalf("expected 2 modes, got %d", len(modes))
	}
	if modes[0] != "edit" || modes[1] != "plan" {
		t.Fatalf("unexpected modes: %#v", modes)
	}
}
