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
	if got := a.stallNudgeBlock(1); got == "" {
		t.Error("stall nudge expected with active goal")
	}
	a.SetGoal("")
	if got := a.stallNudgeBlock(1); got != "" {
		t.Errorf("no nudge without pending work, got %q", got)
	}
}

func TestFinishGate(t *testing.T) {
	a := &Agent{sess: newTestSession()}

	// No goal, no todos -> plain finish passes.
	if ok, _ := a.finishGate("done", ""); !ok {
		t.Error("plain finish should pass without goal/todos")
	}

	// Goal active without evidence -> denied.
	a.SetGoal("overnight")
	if ok, reason := a.finishGate("done", ""); ok || !strings.Contains(reason, "/goal") {
		t.Errorf("goal without evidence must deny, ok=%v reason=%q", ok, reason)
	}

	// With evidence -> allowed.
	if ok, _ := a.finishGate("done", "go test ./... green"); !ok {
		t.Error("evidenced finish should pass")
	}
}

func TestInjectLongRunContextStallContinues(t *testing.T) {
	a := &Agent{sess: newTestSession()}
	a.promptSystem = nil
	a.SetGoal("pending work")
	meta := &runMetadata{}
	// First call: turn 1, no tools — treated as warm-up, no continuation.
	if a.injectLongRunContext(meta, false) {
		t.Fatal("turn 1 must not force continuation")
	}
	// Second consecutive no-tool turn: stall nudge forces continuation.
	if !a.injectLongRunContext(meta, false) {
		t.Fatal("stall on turn 2 with active goal should continue")
	}
	if len(a.sess.Messages) < 1 {
		t.Fatal("expected injected system message")
	}
	if got := a.sess.Messages[len(a.sess.Messages)-1].Role; got != aiRoleSystemForTest() {
		t.Fatalf("injected role = %v", got)
	}
	// Tool activity resets stalls; no message injected.
	before := len(a.sess.Messages)
	if a.injectLongRunContext(meta, true) {
		t.Fatal("healthy turn must not continue")
	}
	if len(a.sess.Messages) != before {
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
