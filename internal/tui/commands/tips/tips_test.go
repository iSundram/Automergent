package tips

import (
	"strings"
	"testing"
)

func TestAllTipsAreSingleLine(t *testing.T) {
	for _, ct := range All() {
		for field, val := range map[string]string{"Tip": ct.Tip, "Personalized": ct.Personalized} {
			if strings.ContainsAny(val, "\n\r") {
				t.Errorf("%s %s field must be one line", ct.Name, field)
			}
		}
	}
}

func TestAllFactsAreSingleLine(t *testing.T) {
	if len(Facts) == 0 {
		t.Fatal("Facts must not be empty")
	}
	for i, f := range Facts {
		if strings.TrimSpace(f) == "" {
			t.Errorf("Facts[%d] is empty", i)
		}
		if strings.ContainsAny(f, "\n\r") {
			t.Errorf("Facts[%d] must be one line", i)
		}
	}
}

func TestAllCoversEveryRegisteredName(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("no tips registered — the per-command files' init() must run")
	}
	seen := map[string]bool{}
	for _, ct := range all {
		if ct.Name == "" {
			t.Error("a registered tip has an empty Name")
		}
		if seen[ct.Name] {
			t.Errorf("tip %q registered more than once", ct.Name)
		}
		seen[ct.Name] = true
		if _, ok := For(ct.Name); !ok {
			t.Errorf("For(%q) missed a name returned by All()", ct.Name)
		}
	}
	if _, ok := For("definitely-not-a-command"); ok {
		t.Error("For() must report unknown commands as missing")
	}
}

func TestAllIsDeterministic(t *testing.T) {
	first := All()
	for i := 0; i < 3; i++ {
		again := All()
		if len(again) != len(first) {
			t.Fatalf("All() length changed between calls: %d vs %d", len(first), len(again))
		}
		for j := range first {
			if first[j].Name != again[j].Name {
				t.Fatalf("All() order changed at %d: %q vs %q", j, first[j].Name, again[j].Name)
			}
		}
	}
}
