package commands

import (
	"strings"
	"testing"
)

func TestCommitSendsPromptThroughAgent(t *testing.T) {
	m := NewMockHost()
	handleCommit(m, []string{"only", "the parser files"})
	if len(m.startAgentCalls) != 1 {
		t.Fatalf("expected one agent start, got %v", m.startAgentCalls)
	}
	prompt := m.startAgentCalls[0]
	for _, want := range []string{"git status", "Never push", "Focus: only the parser files"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("commit prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestReviewTargetsUncommittedChangesByDefault(t *testing.T) {
	m := NewMockHost()
	handleReview(m, nil)
	prompt := m.startAgentCalls[0]
	if !strings.Contains(prompt, "uncommitted changes") || !strings.Contains(prompt, "blocking") {
		t.Fatalf("unexpected default review prompt:\n%s", prompt)
	}
}

func TestReviewDetectsPullRequestRefs(t *testing.T) {
	m := NewMockHost()
	handleReview(m, []string{"#42"})
	if !strings.Contains(m.startAgentCalls[0], "pull request #42") {
		t.Fatalf("PR ref not detected:\n%s", m.startAgentCalls[0])
	}

	m = NewMockHost()
	handleReview(m, []string{"feature/branch"})
	if strings.Contains(m.startAgentCalls[0], "pull request") {
		t.Fatalf("branch ref mistaken for PR:\n%s", m.startAgentCalls[0])
	}
}

func TestRemoveCustomDropsOnlyCustoms(t *testing.T) {
	reg := Default()
	before := len(reg.List())
	custom := Command{Name: "deploy", Aliases: []string{"dp"}, Description: "d", Category: customCategory, Icon: customIcon}
	if err := reg.RegisterCustom(custom, func(h Host, a []string) Result { return Done(nil) }); err != nil {
		t.Fatal(err)
	}
	if cmd, ok := reg.Lookup("deploy"); !ok || cmd.Name != "deploy" {
		t.Fatal("custom not registered")
	}
	if _, ok := reg.Lookup("dp"); !ok {
		t.Fatal("custom alias not registered")
	}

	if got := reg.RemoveCustom(); got != 1 {
		t.Fatalf("RemoveCustom removed %d, want 1", got)
	}
	if len(reg.List()) != before {
		t.Fatalf("builtin count changed: %d -> %d", before, len(reg.List()))
	}
	if _, ok := reg.Lookup("deploy"); ok {
		t.Fatal("custom survived RemoveCustom")
	}
	if _, ok := reg.Lookup("dp"); ok {
		t.Fatal("custom alias survived RemoveCustom — stale alias map")
	}
	builtinModel, ok := reg.Lookup("model")
	if !ok || builtinModel.Category != "AI & Model" || !reg.HasHandler("model") {
		t.Fatal("builtins damaged by RemoveCustom")
	}

	// Re-registering the same custom after removal must work (no stale state).
	if err := reg.RegisterCustom(custom, func(h Host, a []string) Result { return Done(nil) }); err != nil {
		t.Fatalf("re-register after removal failed: %v", err)
	}
}
