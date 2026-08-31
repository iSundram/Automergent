package commands

import (
	"strings"
	"testing"
)

func TestTipFilesCoverEveryCommand(t *testing.T) {
	r := Default()
	for _, cmd := range r.List() {
		ct, ok := r.Tip(cmd.Name)
		if !ok {
			t.Errorf("command %q has no tips file (tips/%s.md)", cmd.Name, cmd.Name)
			continue
		}
		if strings.TrimSpace(ct.Tip) == "" {
			t.Errorf("tips/%s.md is missing its tip: line", cmd.Name)
		}
		if strings.TrimSpace(ct.Body) == "" {
			t.Errorf("tips/%s.md is missing its comprehensive body", cmd.Name)
		}
	}
}

func TestTipResolvesAliases(t *testing.T) {
	r := Default()
	// "session" is an alias of "sessions"; "stop" of "cancel"; "artifacts" of "artifact".
	for alias, want := range map[string]string{"session": "sessions", "stop": "cancel", "artifacts": "artifact"} {
		ct, ok := r.Tip(alias)
		if !ok {
			t.Fatalf("alias %q did not resolve to a tip", alias)
		}
		if ct.Name != want {
			t.Fatalf("alias %q resolved to %q, want %q", alias, ct.Name, want)
		}
	}
}

func TestParseTipFile(t *testing.T) {
	ct := parseTipFile("demo", "tip: short hint\npersonalized: uses {model}\n---\n# Body\n\nGuidance here.\n")
	if ct.Tip != "short hint" {
		t.Fatalf("tip parsed wrong: %q", ct.Tip)
	}
	if ct.Personalized != "uses {model}" {
		t.Fatalf("personalized parsed wrong: %q", ct.Personalized)
	}
	if !strings.Contains(ct.Body, "Guidance here.") {
		t.Fatalf("body parsed wrong: %q", ct.Body)
	}

	// Personalized wins for the infoline; tip is the fallback.
	if ct.InfolineTip() != "uses {model}" {
		t.Fatalf("InfolineTip must prefer personalized, got %q", ct.InfolineTip())
	}
	if parseTipFile("x", "tip: only tip\n").InfolineTip() != "only tip" {
		t.Fatal("InfolineTip must fall back to the plain tip")
	}
}
