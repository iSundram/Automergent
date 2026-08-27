package app

// UI state derivation.
//
// The TUI's bottom chrome (mode chip, activity, key hints, info line) is
// rendered from a single derived state rather than from a stored enum. Deriving
// it means the chrome cannot disagree with what is actually on screen: there is
// no second place to forget to update when a pane opens or a run ends.
//
// The precedence order here deliberately mirrors the ESC chain in escape.go —
// whatever ESC would act on is what the hints describe.

import (
	"time"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/tips"
)

// uiState reports the interaction state the app is currently in.
func (a *App) uiState() tips.State {
	switch {
	// Modal takeovers, outermost first.
	case a.showHelp:
		return tips.StateHelp
	case a.selector.Visible():
		return tips.StateSessionBrowser
	case a.confirm.Visible():
		return tips.StateAwaitingPermission
	case a.questionnaire != nil && a.questionnaire.Visible():
		return tips.StateAwaitingAnswer
	case a.diffPane.Visible() && a.reviewingProposal != "":
		return tips.StateReviewProposal
	case a.diffPane.Visible():
		return tips.StateDiffOpen
	case a.sessionBrowser.Visible():
		return tips.StateSessionBrowser
	case a.palette.Visible():
		return tips.StatePaletteOpen

	// Armed confirmations: these describe a key the user just pressed, so they
	// outrank the ambient state they were pressed in. The ESC arm is gated on
	// its window so a lapsed hint never advertises a clear that won't happen.
	case a.ctrlCArmed && a.thinking:
		return tips.StateStopFirst
	case a.ctrlCArmed:
		return tips.StateConfirmQuit
	case a.escArmed && escArmActive(a.lastEscAt):
		return tips.StateConfirmClear

	// A legacy ask_user reply channel pending without the questionnaire pane:
	// Enter answers the agent directly here, which must not be advertised as
	// "enter queues a message".
	case a.askUserReplyCh != nil:
		return tips.StateAwaitingAnswer

	// Focus-owning panes.
	case a.dockFocusActive():
		return tips.StateDockFocused
	case a.browsing:
		return tips.StateBrowsing

	// Run activity.
	case a.retrying:
		return tips.StateRetrying
	case a.thinking && len(a.msgQueue) > 0:
		return tips.StateQueued
	case a.thinking && a.activeTool != "":
		return tips.StateRunningTool
	case a.thinking:
		return tips.StateRunning
	case len(a.msgQueue) > 0:
		return tips.StateQueued

	// Sticky outcomes from the last run.
	case a.lastOutcome == outcomeInterrupted:
		return tips.StateInterrupted
	case a.lastOutcome == outcomeCancelled:
		return tips.StateCancelled
	case a.lastOutcome == outcomeError:
		return tips.StateError
	}
	return tips.StateIdle
}

// tipContext gathers the live values the info line interpolates.
func (a *App) tipContext() tips.Context {
	ctx := tips.Context{
		Queued:   len(a.msgQueue),
		Tool:     a.activeTool,
		ToolsRun: a.runToolCount,
	}
	if !a.runStart.IsZero() && a.thinking {
		ctx.Elapsed = time.Since(a.runStart)
	}
	if a.retrying {
		ctx.Attempt = a.retryAttempt
		ctx.MaxAttempts = a.retryMax
		ctx.ErrCode = a.retryCode
		ctx.Detail = a.retryDetail
		// Compute remaining delay dynamically so the countdown ticks live.
		if !a.retryDelayAt.IsZero() && a.retryDelay > 0 {
			remaining := a.retryDelay - time.Since(a.retryDelayAt)
			if remaining < 0 {
				remaining = 0
			}
			ctx.NextRetryIn = remaining
		}
	} else if a.lastOutcome == outcomeError {
		if rec, ok := a.latestAPIError(); ok {
			ctx.ErrCode = rec.displayCode()
			ctx.Detail = rec.Detail
			ctx.MaxAttempts = rec.Attempt
		}
	}
	// A permission prompt names the tool it is asking about, which is not
	// necessarily the tool that was last seen executing.
	if a.confirm.Visible() && a.permissionTool != "" {
		ctx.Tool = a.permissionTool
	}
	return ctx
}

// displayMode returns the approval mode to show in the footer chip, resolving
// the legacy "edit" alias and defaulting an unset mode.
func (a *App) displayMode() string {
	mode := agent.CanonicalMode(a.cfg.Mode)
	if mode == "" {
		return "manual"
	}
	return mode
}

// refreshChrome recomputes the derived state and pushes it into the footer and
// info line. Called after anything that could change the state: key handling,
// agent events, pane toggles.
func (a *App) refreshChrome() {
	state := a.uiState()
	ctx := a.tipContext()

	a.statusBar.SetMode(a.displayMode())
	a.statusBar.SetQueued(len(a.msgQueue))
	a.statusBar.SetOutcome(a.outcomeBadge())
	a.infoLine.Set(state, ctx)
}
