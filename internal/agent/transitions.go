package agent

// Loop transition semantics — a minimal port of the reference agent's
// query/transitions.ts.
//
// Every continue-site in the phase loop writes the reason it continued into
// loopState.transition, and every exit has a TerminalReason. This makes the
// recovery paths first-class: tests assert `state.transition ==
// ContinueMaxOutputRecovery` instead of inspecting injected message text,
// and each bounded retry visibly owns exactly the counter that bounds it.
// A continue-site must reset the counters it owns and preserve the ones
// guarding against spirals (e.g. reactiveCompactions survives stop-hook and
// recovery retries — retrying after a compaction that already failed to
// free space reproduces the failure identically).

// ContinueReason is why the previous loop iteration continued instead of
// returning.
type ContinueReason string

const (
	// ContinueNextTurn: the model called tools; results were appended and
	// the conversation continues normally.
	ContinueNextTurn ContinueReason = "next_turn"
	// ContinueExploreCoverage: the explore exit gate found the task's files
	// unread; one nudge per phase run.
	ContinueExploreCoverage ContinueReason = "explore_coverage_nudge"
	// ContinueStallNudge: no tool calls while work remains open; bounded by
	// the stall-nudge budget (longrun.go).
	ContinueStallNudge ContinueReason = "stall_nudge"
	// ContinuePhaseStepWrapup: the phase step budget fired; the model gets
	// one wrap-up turn before the hard cutoff.
	ContinuePhaseStepWrapup ContinueReason = "phase_step_wrapup"
	// ContinueReactiveCompact: the provider rejected the prompt as too long;
	// compaction ran and the same request is retried once.
	ContinueReactiveCompact ContinueReason = "reactive_compact_retry"
	// ContinueQuotaRetry: the provider reported quota exhaustion and a
	// fallback provider was selected; the request is retried.
	ContinueQuotaRetry ContinueReason = "quota_retry"
	// ContinueMaxOutputEscalate: the response was truncated at the output
	// cap; the same request is retried once at the escalated cap.
	ContinueMaxOutputEscalate ContinueReason = "max_output_tokens_escalate"
	// ContinueMaxOutputRecovery: the response was truncated even at the
	// escalated cap; a resume-mid-thought nudge continues the turn.
	ContinueMaxOutputRecovery ContinueReason = "max_output_tokens_recovery"
	// ContinueTokenBudget: the user set a token budget for the turn and it
	// is not yet spent; a continuation nudge keeps the model working.
	ContinueTokenBudget ContinueReason = "token_budget_continuation"
)

// TerminalReason is why the phase loop exited.
type TerminalReason string

const (
	// TerminalCompleted: the model finished its response without tool calls.
	TerminalCompleted TerminalReason = "completed"
	// TerminalBlockingLimit: context exhausted past the blocking limit;
	// compaction could not free enough space.
	TerminalBlockingLimit TerminalReason = "blocking_limit"
	// TerminalMaxSteps: the phase exceeded twice its step budget; aborted to
	// protect the session from a runaway loop.
	TerminalMaxSteps TerminalReason = "max_steps"
	// TerminalModelError: a provider error the loop cannot recover from.
	TerminalModelError TerminalReason = "model_error"
	// TerminalAborted: the run's context was cancelled.
	TerminalAborted TerminalReason = "aborted"
)

// Terminal is the phase loop's exit value.
type Terminal struct {
	Reason    TerminalReason
	Err       error
	TurnCount int
}

// loopState carries the loop's mutable state across iterations. The counters
// bound the retry paths; the transition records why the previous iteration
// continued (empty on the first iteration).
type loopState struct {
	// turnCount is the number of provider calls made in this phase loop.
	turnCount int
	// transition is why the previous iteration continued.
	transition ContinueReason
	// maxOutputRecoveryCount bounds resume-mid-thought retries after output
	// truncation. Reset on ContinueNextTurn (a healthy turn resets it).
	maxOutputRecoveryCount int
	// maxOutputEscalated records that the one-time cap escalation fired.
	maxOutputEscalated bool
	// reactiveCompactions bounds context-overflow compaction retries.
	reactiveCompactions int
}

// Output-cap ladder for truncation recovery. The default request cap leaves
// headroom for the compaction summary; when the model actually needs more,
// escalate once before falling back to multi-turn recovery.
const (
	defaultMaxOutputTokens  = 8192
	escalatedMaxOutputTokens = 65536
	maxOutputRecoveryLimit  = 3
)

// truncatedRecoveryNudge is the ephemeral message injected when the model's
// output was cut by the token cap: the reference agent's wording, tuned to
// avoid the apology/recap reflex that wastes the recovery turn.
const truncatedRecoveryNudge = "Output token limit hit. Resume directly — no apology, no recap of what you were doing. Pick up mid-thought if that is where the cut happened. Break remaining work into smaller pieces."

// setLastTransition records the loop's last continue reason for
// observability (tests assert recovery paths without inspecting messages).
func (a *Agent) setLastTransition(r ContinueReason) {
	a.mu.Lock()
	a.lastTransition = r
	a.mu.Unlock()
}

// LastTransition returns why the phase loop's previous iteration continued
// ("" before the first iteration or when no loop has run).
func (a *Agent) LastTransition() ContinueReason {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastTransition
}
