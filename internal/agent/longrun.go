package agent

import (
	"fmt"
	"strings"

	promptpkg "github.com/iSundram/Automergent/internal/prompt"
)

// Long-run machinery (the /goal pattern from both reference agents):
// - a user-marked goal forces continuation until deliverables have evidence
// - anti-stall nudges when the model produces no tool calls while work is pending
// - a finish gate refuses unevidenced completion

const (
	goalReminderEveryTurns = 5
	// maxAutoContinuations bounds FORCED stall-recovery continuations per
	// Run (goal reminders annotate but never force). 20 let an
	// answer→nudge→pointless-tool→answer cycle burn ~380K tokens on a
	// simple informational question (session 1c051187); a genuine stall
	// that needs more than a few nudges is a model problem, not a
	// try-harder problem.
	maxAutoContinuations = 4
	stallNudgeMaxPerRun  = 2
)

// goalMetadataKey marks the session-level goal in session metadata.
const goalMetadataKey = "goal"

// SetGoal records a long-run objective on the session.
func (a *Agent) SetGoal(goal string) {
	if a.sess == nil {
		return
	}
	if a.sess.Metadata == nil {
		a.sess.Metadata = make(map[string]string)
	}
	if strings.TrimSpace(goal) == "" {
		delete(a.sess.Metadata, goalMetadataKey)
		return
	}
	a.sess.Metadata[goalMetadataKey] = strings.TrimSpace(goal)
}

// Goal returns the active long-run objective, if any.
func (a *Agent) Goal() string {
	if a.sess == nil || a.sess.Metadata == nil {
		return ""
	}
	return a.sess.Metadata[goalMetadataKey]
}

// goalContinuationBlock builds the escalating audit reminder injected every
// N turns while a goal is active.
func (a *Agent) goalContinuationBlock(continuation int) string {
	goal := a.Goal()
	if goal == "" || continuation == 0 || continuation%goalReminderEveryTurns != 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n# Continuation Reminder\n")
	sb.WriteString(fmt.Sprintf("You are still working toward the user's marked goal. Do not stop until it is fully complete. Time spent so far marker: continuation #%d.\n", continuation))
	sb.WriteString("Before concluding, verify your work:\n")
	sb.WriteString("1. Re-read the original request and list every concrete deliverable.\n")
	sb.WriteString("2. Map each requirement to concrete evidence: file contents, test results, build logs.\n")
	sb.WriteString("3. Confirm via actual output — wanting to be done or having spent effort is not being done.\n")
	sb.WriteString("4. If evidence is missing, continue working with tools now.\n")
	return sb.String()
}

// stallNudgeBlock returns the injected message when the model replied without
// tool calls while todos remain open. Only IMPLEMENTATION phases (build)
// nudge: there, stopping without verified work is a real stall. Informational
// phases (explore/plan) deliver the answer itself — nudging them to "keep
// calling tools" after a complete findings report produced the go-test loop
// in session 1c051187. Returns "" when no nudge is warranted.
func (a *Agent) stallNudgeBlock(stallCount int, implementationPhase bool) string {
	if stallCount <= 0 || stallCount > stallNudgeMaxPerRun {
		return ""
	}
	if !implementationPhase {
		return ""
	}
	var turnCtx *promptpkg.TurnContext
	if a.promptSystem != nil {
		turnCtx = a.promptSystem.GetTurnContext()
	}
	pending := 0
	if turnCtx != nil {
		for _, todo := range turnCtx.TodoItems {
			if todo.Status != "completed" {
				pending++
			}
		}
	}
	if pending == 0 && a.Goal() == "" {
		return ""
	}
	return "\n[System note] You did not call any tools this turn, but implementation work remains incomplete or unverified. Continue with the next concrete step and verify with real command output; call `finish` only once you have verification evidence.\n"
}

// runMetadata carries per-Run counters (continuation index, stall count).
type runMetadata struct {
	turn          int
	stalls        int
	continuations int
}

// injectLongRunContext queues goal reminders and stall nudges as an
// ephemeral system reminder for the next provider turn (see ephemeral.go —
// nudges are request-only and never pollute the session history). It reports
// whether the run should CONTINUE instead of stopping: only stall recovery
// forces continuation; goal reminders annotate without forcing.
// implementationPhase selects stall-nudge semantics (build yes, informational
// no — see stallNudgeBlock).
func (a *Agent) injectLongRunContext(meta *runMetadata, hadToolCalls bool, implementationPhase bool) (continueTurn bool) {
	meta.turn++
	var block string

	if !hadToolCalls && meta.turn > 1 {
		meta.stalls++
		block = a.stallNudgeBlock(meta.stalls, implementationPhase)
	} else {
		meta.stalls = 0
	}

	forceContinue := block != "" &&
		meta.stalls <= stallNudgeMaxPerRun &&
		meta.continuations < maxAutoContinuations

	meta.continuations++
	block += a.goalContinuationBlock(meta.continuations)

	if strings.TrimSpace(block) == "" {
		return false
	}
	a.injectEphemeral(block)
	return forceContinue
}
