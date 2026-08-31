package commands

import (
	"strings"
	"testing"
)

func TestTipsCoverEveryCommand(t *testing.T) {
	r := Default()
	for _, cmd := range r.List() {
		ct, ok := r.Tip(cmd.Name)
		if !ok {
			t.Errorf("command %q has no registered tip (tips/%s.go)", cmd.Name, cmd.Name)
			continue
		}
		if strings.TrimSpace(ct.Tip) == "" {
			t.Errorf("tips/%s.go is missing its Tip field", cmd.Name)
		}
		if strings.TrimSpace(ct.Body) == "" {
			t.Errorf("tips/%s.go is missing its comprehensive Body", cmd.Name)
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

func TestInfolineTipPrefersPersonalized(t *testing.T) {
	ct, ok := Default().Tip("compact")
	if !ok {
		t.Fatal("compact has no tip")
	}
	if ct.Personalized == "" {
		t.Fatal("compact tip has no personalized variant to exercise")
	}
	if ct.InfolineTip() != ct.Personalized {
		t.Fatalf("InfolineTip must prefer personalized, got %q", ct.InfolineTip())
	}
	// A tip without a personalized variant falls back to the plain tip.
	ct2 := ct
	ct2.Personalized = ""
	if ct2.InfolineTip() != ct2.Tip {
		t.Fatal("InfolineTip must fall back to the plain tip")
	}
}
