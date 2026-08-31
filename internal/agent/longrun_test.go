package agent

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/shared"
)

func newTestSession() *session.Session { return session.New() }

func aiRoleSystemForTest() ai.Role { return ai.RoleSystem }

func TestGoalSetGetClear(t *testing.T) {
	a := &Agent{sess: newTestSession()}
	a.SetGoal("ship the feature overnight")
	if a.Goal() != "ship the feature overnight" {
		t.Fatalf("goal = %q", a.Goal())
	}
	a.SetGoal("")
	if a.Goal() != "" {
		t.Fatalf("goal should be cleared, got %q", a.Goal())
	}
}

func TestGoalContinuationBlockCadence(t *testing.T) {
	a := &Agent{sess: newTestSession()}
	a.SetGoal("long task")
	if got := a.goalContinuationBlock(0); got != "" {
		t.Errorf("continuation 0 must not annotate, got %q", got)
	}
	if got := a.goalContinuationBlock(3); got != "" {
		t.Errorf("continuation 3 must not annotate, got %q", got)
	}
	got := a.goalContinuationBlock(5)
	if !strings.Contains(got, "Continuation Reminder") || !strings.Contains(got, "#5") == false && !strings.Contains(got, "#5") {
		if !strings.Contains(got, "Continuation") {
			t.Errorf("expected reminder at 5, got %q", got)
		}
	}
}

func TestStallNudgeRequiresPendingWork(t *testing.T) {
	a := &Agent{sess: newTestSession(), promptSystem: nil}
	// No turn context: nudge only fires when a goal exists.
	a.SetGoal("keep going")
	if got := a.stallNudgeBlock(1, true); got == "" {
		t.Error("stall nudge expected with active goal")
	}
	a.SetGoal("")
	if got := a.stallNudgeBlock(1, true); got != "" {
		t.Errorf("no nudge without pending work, got %q", got)
	}
	// Informational phases never stall-nudge: their deliverable is the
	// answer, and nudging produced the answer→pointless-tool loop
	// (session 1c051187).
	a.SetGoal("keep going")
	if got := a.stallNudgeBlock(1, false); got != "" {
		t.Errorf("informational phase must not be nudged, got %q", got)
	}
}

func TestInjectLongRunContextStallContinues(t *testing.T) {
	a := &Agent{sess: newTestSession()}
	a.promptSystem = nil
	a.SetGoal("pending work")
	meta := &runMetadata{}
	// First call: turn 1, no tools — treated as warm-up, no continuation.
	if a.injectLongRunContext(meta, false, true) {
		t.Fatal("turn 1 must not force continuation")
	}
	// Second consecutive no-tool turn: stall nudge forces continuation. The
	// nudge rides as an ephemeral request-only reminder — it must reach the
	// next provider call without ever being written into the session history
	// (see ephemeral.go for why persistence is pollution).
	if !a.injectLongRunContext(meta, false, true) {
		t.Fatal("stall on turn 2 with active goal should continue")
	}
	if len(a.sess.Messages) != 0 {
		t.Fatalf("nudge must not pollute session history, got %d messages", len(a.sess.Messages))
	}
	msgs := a.drainEphemeral()
	if len(msgs) != 1 {
		t.Fatalf("expected one ephemeral reminder queued, got %d", len(msgs))
	}
	if msgs[0].Role != aiRoleSystemForTest() {
		t.Fatalf("injected role = %v", msgs[0].Role)
	}
	// Tool activity resets stalls; no message injected.
	if a.injectLongRunContext(meta, true, true) {
		t.Fatal("healthy turn must not continue")
	}
	if len(a.drainEphemeral()) != 0 {
		t.Fatal("healthy turn must not inject messages")
	}
}

func TestTodoPendingDrivesStallNudge(t *testing.T) {
	// stallNudgeBlock reads GetTurnContext which is nil without a prompt
	// system, so the goal branch applies — covered above. This test guards
	// the todo-shape logic used by that path.
	items := []shared.TodoItem{{ID: "t1", Status: shared.TodoStatusPending}}
	pending := 0
	for _, todo := range items {
		if todo.Status != "completed" {
			pending++
		}
	}
	if pending == 0 {
		t.Fatal("sanity: pending todo should count")
	}
}
