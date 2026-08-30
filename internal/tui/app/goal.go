package app

// Goal-driven autonomy.
//
// A goal is a per-session state machine: when the agent goes idle with an
// active, unpaused goal, a <goal-steering> continuation prompt is injected
// that drives the next turn. The model reports outcomes with protocol
// markers in its final reply (GOAL_COMPLETE / GOAL_BLOCKED) so no extra tool
// is needed; three consecutive blocked reports pause the loop, a token
// budget and a turn cap bound runaway runs, and /goal continue resets the
// turn counter.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Goal loop bounds.
const (
	goalMaxTurns          = 150
	goalBlockedThreshold  = 3
)

// goalState is the mutable per-session goal. Not persisted: a goal is a
// live-work contract, not history.
type goalState struct {
	objective   string
	paused      bool
	tokenBudget int // 0 = unlimited
	tokensUsed  int
	turns       int
	blocked     int
	lastBlock   string
	startedAt   time.Time
	// tokenBaseline is the session's cumulative token total when the goal
	// was set; per-turn usage is the delta against it.
	tokenBaseline int
}

// setGoal installs a new objective, replacing any previous goal.
func (a *App) setGoal(objective string, tokenBudget int) {
	a.goal = &goalState{
		objective:     objective,
		tokenBudget:   tokenBudget,
		startedAt:     time.Now(),
		tokenBaseline: a.sessionTokenTotal(),
	}
}

// sessionTokenTotal is the cumulative token count for the active session.
func (a *App) sessionTokenTotal() int {
	if a.sess == nil {
		return 0
	}
	return a.sess.TotalInputTokens + a.sess.TotalOutputTokens
}

// goalTokenUsage reconciles the goal's token accounting with the session's
// cumulative totals (the delta since the goal was set).
func (a *App) goalTokenUsage() int {
	if a.goal == nil {
		return 0
	}
	used := a.sessionTokenTotal() - a.goal.tokenBaseline
	if used < 0 {
		used = 0
	}
	a.goal.tokensUsed = used
	return used
}

// maybeContinueGoal runs after a turn completes with no queued user message.
// It inspects the final reply for goal protocol markers and either advances
// the loop (returning the continuation tea.Cmd) or terminates it.
func (a *App) maybeContinueGoal(finalText string) tea.Cmd {
	g := a.goal
	if g == nil || g.paused || g.objective == "" {
		return nil
	}

	switch marker, reason := parseGoalMarker(finalText); marker {
	case goalMarkerComplete:
		a.conversation.AddMessage("system", "✓ Goal complete — "+g.objective, false)
		a.statusBar.SetStatus("Goal complete")
		a.goal = nil
		return nil
	case goalMarkerBlocked:
		g.blocked++
		g.lastBlock = reason
		if g.blocked >= goalBlockedThreshold {
			g.paused = true
			a.conversation.AddMessage("system",
				fmt.Sprintf("Goal paused after %d blocked turns — last blocker: %s. Use /goal resume to retry or /goal clear to stop.",
					g.blocked, reason), false)
			a.statusBar.SetStatus("Goal paused (blocked)")
			return nil
		}
	case goalMarkerNone:
		g.blocked = 0 // real progress resets the blocked streak
	}

	if g.turns >= goalMaxTurns {
		g.paused = true
		a.conversation.AddMessage("system",
			fmt.Sprintf("Goal reached the %d-turn cap. Use /goal continue to reset the counter and keep going.", goalMaxTurns), false)
		a.statusBar.SetStatus("Goal paused (turn cap)")
		return nil
	}

	if g.tokenBudget > 0 && a.goalTokenUsage() >= g.tokenBudget {
		g.paused = true
		a.conversation.AddMessage("system",
			fmt.Sprintf("Goal token budget exhausted (%d/%d). Use /goal budget <n> to raise it, /goal resume to continue unbudgeted, or /goal clear to stop.",
				g.tokensUsed, g.tokenBudget), false)
		a.statusBar.SetStatus("Goal paused (budget)")
		return nil
	}

	g.turns++
	display := fmt.Sprintf("goal continuation · turn %d — %s", g.turns, g.objective)
	return a.startAgentCommand(display, goalContinuationPrompt(g), "goal")
}

type goalMarker int

const (
	goalMarkerNone goalMarker = iota
	goalMarkerComplete
	goalMarkerBlocked
)

// parseGoalMarker scans a final reply for the goal protocol markers. The
// model is instructed to end its reply with one of:
//
//	GOAL_COMPLETE
//	GOAL_BLOCKED: <reason>
func parseGoalMarker(text string) (goalMarker, string) {
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "GOAL_COMPLETE" {
			return goalMarkerComplete, ""
		}
		if strings.HasPrefix(t, "GOAL_BLOCKED") {
			reason := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, "GOAL_BLOCKED"), ":"))
			return goalMarkerBlocked, reason
		}
	}
	return goalMarkerNone, ""
}

// goalContinuationPrompt is the steering message injected to drive the next
// turn.
func goalContinuationPrompt(g *goalState) string {
	var sb strings.Builder
	sb.WriteString("<goal-steering type=\"continuation\">\n")
	sb.WriteString("You have an active goal to work on. Continue making progress.\n\n")
	sb.WriteString("## Active Goal\n")
	sb.WriteString(g.objective + "\n\n")

	sb.WriteString("## Status\n")
	fmt.Fprintf(&sb, "- Continuation turns executed: %d / %d\n", g.turns, goalMaxTurns)
	if g.tokenBudget > 0 {
		fmt.Fprintf(&sb, "- Tokens used: %d / %d (%d remaining)\n",
			g.tokensUsed, g.tokenBudget, maxInt(0, g.tokenBudget-g.tokensUsed))
	} else {
		fmt.Fprintf(&sb, "- Tokens used: %d (no budget set)\n", g.tokensUsed)
	}
	if g.lastBlock != "" {
		fmt.Fprintf(&sb, "- Last blocker: %s\n", g.lastBlock)
	}

	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString("Continue working towards the goal. Do NOT narrow the scope of the goal — even if you cannot complete everything in one turn, maintain the full objective and make as much progress as possible.\n\n")
	sb.WriteString("When you believe the goal is fully achieved, perform a strict Completion Audit — verify each requirement against actual evidence (files, tests, command output), not assumption — and end your reply with a line containing exactly:\nGOAL_COMPLETE\n\n")
	sb.WriteString("If you are genuinely unable to make progress (missing access, unsatisfiable requirement, hard dependency), end your reply with:\nGOAL_BLOCKED: <one-line reason>\n\n")
	sb.WriteString("Otherwise just continue working. Do not repeat this message back.\n")
	sb.WriteString("</goal-steering>\n")
	return sb.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// goalSnapshot renders the current goal state for /goal status.
func (a *App) goalSnapshot() string {
	g := a.goal
	if g == nil {
		return "No goal set. Use /goal <objective> to set one."
	}
	status := "active"
	if g.paused {
		status = "paused"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Goal (%s): %s\n", status, g.objective)
	fmt.Fprintf(&sb, "  Turns: %d / %d", g.turns, goalMaxTurns)
	if g.tokenBudget > 0 {
		fmt.Fprintf(&sb, " · Tokens: %d / %d", a.goalTokenUsage(), g.tokenBudget)
	} else {
		fmt.Fprintf(&sb, " · Tokens: %d (no budget)", a.goalTokenUsage())
	}
	if g.blocked > 0 {
		fmt.Fprintf(&sb, " · Blocked streak: %d/%d", g.blocked, goalBlockedThreshold)
	}
	fmt.Fprintf(&sb, "\n  Started: %s", g.startedAt.Format("15:04:05"))
	return sb.String()
}
