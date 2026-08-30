package app

import (
	"strings"
	"testing"
)

// The continuation loop (goal.go): completion markers clear the goal,
// blocked streaks pause it, and the turn cap bounds runaway runs.

func newGoalTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.setGoal("make it work", 0)
	return app
}

func TestGoalCompletionMarkerClearsGoal(t *testing.T) {
	app := newGoalTestApp(t)
	cmd := app.maybeContinueGoal("all done\nGOAL_COMPLETE")
	if cmd != nil {
		t.Fatal("completion must stop the loop, not continue it")
	}
	if app.goal != nil {
		t.Fatal("GOAL_COMPLETE must clear the goal")
	}
}

func TestGoalContinuationAdvancesTurns(t *testing.T) {
	app := newGoalTestApp(t)
	cmd := app.maybeContinueGoal("progress: half done")
	if cmd == nil {
		t.Fatal("ordinary progress must drive a continuation turn")
	}
	if app.goal.turns != 1 {
		t.Fatalf("turns = %d, want 1", app.goal.turns)
	}
}

func TestGoalBlockedStreakPauses(t *testing.T) {
	app := newGoalTestApp(t)
	app.maybeContinueGoal("GOAL_BLOCKED: no access")
	app.maybeContinueGoal("GOAL_BLOCKED: still no access")
	if app.goal.paused {
		t.Fatal("two blocked turns must not pause yet")
	}
	app.maybeContinueGoal("GOAL_BLOCKED: giving up")
	if !app.goal.paused {
		t.Fatal("three consecutive blocked turns must pause the goal")
	}
	if app.goal.lastBlock != "no access" && app.goal.lastBlock != "still no access" && app.goal.lastBlock != "giving up" {
		t.Fatalf("lastBlock not recorded: %q", app.goal.lastBlock)
	}
	if cmd := app.maybeContinueGoal("GOAL_BLOCKED: more"); cmd != nil {
		t.Fatal("paused goal must not continue")
	}
}

func TestGoalProgressResetsBlockedStreak(t *testing.T) {
	app := newGoalTestApp(t)
	app.maybeContinueGoal("GOAL_BLOCKED: a")
	app.maybeContinueGoal("GOAL_BLOCKED: b")
	app.maybeContinueGoal("made real progress") // resets streak
	app.maybeContinueGoal("GOAL_BLOCKED: c")
	if app.goal.paused {
		t.Fatal("progress must reset the blocked streak before the third block")
	}
	if app.goal.blocked != 1 {
		t.Fatalf("blocked = %d, want 1 after reset", app.goal.blocked)
	}
}

func TestGoalTurnCapPauses(t *testing.T) {
	app := newGoalTestApp(t)
	app.goal.turns = goalMaxTurns
	if cmd := app.maybeContinueGoal("still going"); cmd != nil {
		t.Fatal("turn cap must stop the loop")
	}
	if !app.goal.paused {
		t.Fatal("turn cap must pause the goal")
	}
}

func TestGoalBudgetPauses(t *testing.T) {
	app := newGoalTestApp(t)
	app.goal.tokenBudget = 100
	// Token usage is the session-total delta since the goal was set; push
	// the baseline down so the delta reads as an exhausted budget.
	app.goal.tokenBaseline = -100
	if cmd := app.maybeContinueGoal("more work"); cmd != nil {
		t.Fatal("exhausted budget must stop the loop")
	}
	if !app.goal.paused {
		t.Fatal("exhausted budget must pause the goal")
	}
}

func TestGoalPausedAndClearedDoNotContinue(t *testing.T) {
	app := newGoalTestApp(t)
	app.goal.paused = true
	if cmd := app.maybeContinueGoal("work"); cmd != nil {
		t.Fatal("paused goal must not continue")
	}
	app.goal.paused = false
	app.goal = nil
	if cmd := app.maybeContinueGoal("work"); cmd != nil {
		t.Fatal("cleared goal must not continue")
	}
}

func TestGoalMarkerParsing(t *testing.T) {
	if m, _ := parseGoalMarker("text\nGOAL_COMPLETE"); m != goalMarkerComplete {
		t.Fatal("GOAL_COMPLETE line not detected")
	}
	if m, r := parseGoalMarker("GOAL_BLOCKED: missing API key"); m != goalMarkerBlocked || r != "missing API key" {
		t.Fatalf("blocked marker mis-parsed: %d %q", m, r)
	}
	if m, _ := parseGoalMarker("the goal complete word in a sentence"); m != goalMarkerNone {
		t.Fatal("marker must match whole trimmed lines only")
	}
}

func TestGoalContinuationPromptContent(t *testing.T) {
	app := newGoalTestApp(t)
	app.goal.turns = 7
	prompt := goalContinuationPrompt(app.goal)
	for _, want := range []string{"make it work", "goal-steering", "GOAL_COMPLETE", "GOAL_BLOCKED", "7 / 150"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation prompt missing %q", want)
		}
	}
}
