// Package tips is the single source of truth for the keyboard hints and
// "what do I do now" info lines shown in the TUI's bottom chrome.
//
// Every hint string in the UI comes from here, keyed by the UI state the app
// is currently in. Nothing else in the TUI should carry hint literals: the
// footer's key row and the `└─` info line under the spinner are both derived
// from these tables, so a hint can never advertise a key that does nothing in
// the current state.
package tips

import (
	"fmt"
	"strings"
	"time"
)

// State identifies one distinct interaction state of the TUI. The app derives
// it from live model fields rather than storing it, so it cannot drift out of
// sync with what is actually on screen.
type State string

const (
	// StateIdle is the resting state: input focused, nothing running.
	StateIdle State = "idle"
	// StateRunning is an agent turn in flight with no tool executing.
	StateRunning State = "running"
	// StateRunningTool is an agent turn with a tool currently executing.
	StateRunningTool State = "running-tool"
	// StateAwaitingPermission is a tool permission prompt awaiting y/n.
	StateAwaitingPermission State = "awaiting-permission"
	// StateAwaitingAnswer is a structured ask_user questionnaire awaiting input.
	StateAwaitingAnswer State = "awaiting-answer"
	// StateInterrupted is the moment after the user interrupted a run.
	StateInterrupted State = "interrupted"
	// StateCancelled is a run that was cancelled and not yet superseded.
	StateCancelled State = "cancelled"
	// StateQueued is one or more messages waiting for the current run to end.
	StateQueued State = "queued"
	// StateRetrying is an API call being retried after a transient failure.
	StateRetrying State = "retrying"
	// StateError is a terminal API or agent failure.
	StateError State = "error"
	// StateBrowsing is scroll-focus in the conversation pane.
	StateBrowsing State = "browsing"
	// StateDockFocused is keyboard focus in the background-task dock.
	StateDockFocused State = "dock-focused"
	// StatePaletteOpen is the slash/file completion palette showing.
	StatePaletteOpen State = "palette-open"
	// StateDiffOpen is the fullscreen diff overlay showing.
	StateDiffOpen State = "diff-open"
	// StateReviewProposal is a pending edit proposal awaiting accept/reject.
	StateReviewProposal State = "review-proposal"
	// StateSessionBrowser is the inline session picker showing.
	StateSessionBrowser State = "session-browser"
	// StateConfirmClear is the armed state after a first ESC on dirty input.
	StateConfirmClear State = "confirm-clear"
	// StateConfirmQuit is the armed state after a first Ctrl+C when idle.
	StateConfirmQuit State = "confirm-quit"
	// StateStopFirst is Ctrl+C pressed again while the agent is still running.
	StateStopFirst State = "stop-first"
	// StateHelp is the help overlay showing.
	StateHelp State = "help"
)

// AllStates returns every state in a stable order. Tests iterate this to
// guarantee that adding a State without adding its tips fails the build's
// test suite rather than silently rendering an empty hint row.
func AllStates() []State {
	return []State{
		StateIdle,
		StateRunning,
		StateRunningTool,
		StateAwaitingPermission,
		StateAwaitingAnswer,
		StateInterrupted,
		StateCancelled,
		StateQueued,
		StateRetrying,
		StateError,
		StateBrowsing,
		StateDockFocused,
		StatePaletteOpen,
		StateDiffOpen,
		StateReviewProposal,
		StateSessionBrowser,
		StateConfirmClear,
		StateConfirmQuit,
		StateStopFirst,
		StateHelp,
	}
}

// Hint is one key→action pair rendered in the footer's centre row.
//
// Priority orders truncation when the terminal is too narrow: lower numbers
// survive longer. Priority 0 means "never drop this hint" — reserve it for the
// one key that is the only way out of a state.
type Hint struct {
	Key      string
	Action   string
	Priority int
}

// String renders a hint as it appears in the footer: "ESC cancel".
func (h Hint) String() string {
	if h.Action == "" {
		return h.Key
	}
	return h.Key + " " + h.Action
}

// Context carries the handful of live values an info line interpolates. All
// fields are optional; a zero Context still yields a usable (if less specific)
// line for every state.
type Context struct {
	// Queued is how many messages are waiting to be delivered.
	Queued int
	// Attempt and MaxAttempts describe retry progress, 1-based.
	Attempt     int
	MaxAttempts int
	// ErrCode is a provider or HTTP error code, e.g. "529" or "RATE_LIMITED".
	ErrCode string
	// Tool is the tool name currently executing or awaiting permission.
	Tool string
	// Detail is a short human-readable qualifier, e.g. "overloaded".
	Detail string
	// ToolsRun is how many tools completed in the run being described.
	ToolsRun int
	// Elapsed is how long the current activity has been running.
	Elapsed time.Duration
	// NextRetryIn is the delay before the next retry attempt.
	NextRetryIn time.Duration
}

// hintTable maps each state to its footer hints. Ordering within a state is
// the display order; Priority controls which survive a narrow terminal.
var hintTable = map[State][]Hint{
	StateIdle: {
		{Key: "ENTER", Action: "send", Priority: 0},
		{Key: "/", Action: "commands", Priority: 2},
		{Key: "@", Action: "files", Priority: 3},
		{Key: "SHIFT+TAB", Action: "mode", Priority: 2},
		{Key: "CTRL+C", Action: "exit", Priority: 4},
		{Key: "?", Action: "help", Priority: 5},
	},
	StateRunning: {
		{Key: "ESC", Action: "interrupt", Priority: 0},
		{Key: "ENTER", Action: "queue", Priority: 1},
		{Key: "CTRL+J", Action: "steer", Priority: 3},
		{Key: "CTRL+O", Action: "verbose", Priority: 5},
	},
	StateRunningTool: {
		{Key: "ESC", Action: "interrupt", Priority: 0},
		{Key: "ENTER", Action: "queue", Priority: 1},
		{Key: "CTRL+J", Action: "steer", Priority: 3},
	},
	StateAwaitingPermission: {
		{Key: "Y", Action: "allow", Priority: 0},
		{Key: "N", Action: "deny", Priority: 0},
		{Key: "A", Action: "always", Priority: 2},
		{Key: "ESC", Action: "deny", Priority: 3},
	},
	StateAwaitingAnswer: {
		{Key: "ENTER", Action: "answer", Priority: 0},
		{Key: "TAB", Action: "next", Priority: 2},
		{Key: "ESC", Action: "dismiss", Priority: 1},
	},
	StateInterrupted: {
		{Key: "ENTER", Action: "resume", Priority: 0},
		{Key: "/rewind", Action: "undo", Priority: 2},
		{Key: "SHIFT+TAB", Action: "mode", Priority: 4},
	},
	StateCancelled: {
		{Key: "ENTER", Action: "send", Priority: 0},
		{Key: "/rewind", Action: "undo", Priority: 2},
		{Key: "/error", Action: "log", Priority: 4},
	},
	StateQueued: {
		{Key: "ESC", Action: "clear queue", Priority: 0},
		{Key: "CTRL+J", Action: "send now", Priority: 1},
		{Key: "ENTER", Action: "queue more", Priority: 3},
	},
	StateRetrying: {
		{Key: "ESC", Action: "cancel", Priority: 0},
		{Key: "/error", Action: "details", Priority: 1},
	},
	StateError: {
		{Key: "ENTER", Action: "retry", Priority: 0},
		{Key: "/error", Action: "details", Priority: 1},
		{Key: "/doctor", Action: "diagnose", Priority: 3},
	},
	StateBrowsing: {
		{Key: "↑↓", Action: "scroll", Priority: 0},
		{Key: "PGUP/PGDN", Action: "page", Priority: 2},
		{Key: "TAB", Action: "input", Priority: 1},
		{Key: "CTRL+E", Action: "expand", Priority: 4},
	},
	StateDockFocused: {
		{Key: "↑", Action: "back", Priority: 0},
		{Key: "ENTER", Action: "inspect", Priority: 1},
		{Key: "J/K", Action: "move", Priority: 3},
		{Key: "ESC", Action: "back", Priority: 2},
	},
	StatePaletteOpen: {
		{Key: "ENTER", Action: "select", Priority: 0},
		{Key: "↑↓", Action: "move", Priority: 1},
		{Key: "ESC", Action: "close", Priority: 1},
		{Key: "TAB", Action: "complete", Priority: 3},
	},
	StateDiffOpen: {
		{Key: "ESC", Action: "close", Priority: 0},
		{Key: "↑↓", Action: "scroll", Priority: 2},
		{Key: "CTRL+R", Action: "review", Priority: 4},
	},
	StateReviewProposal: {
		{Key: "A", Action: "accept", Priority: 0},
		{Key: "R", Action: "reject", Priority: 0},
		{Key: "ESC", Action: "close", Priority: 2},
	},
	StateSessionBrowser: {
		{Key: "ENTER", Action: "resume", Priority: 0},
		{Key: "↑↓", Action: "move", Priority: 1},
		{Key: "ESC", Action: "close", Priority: 1},
	},
	StateConfirmClear: {
		{Key: "ESC", Action: "clear input", Priority: 0},
		{Key: "CTRL+P", Action: "restore", Priority: 2},
	},
	StateConfirmQuit: {
		{Key: "CTRL+C", Action: "exit", Priority: 0},
		{Key: "ANY", Action: "cancel", Priority: 2},
	},
	StateStopFirst: {
		{Key: "ESC", Action: "stop agent", Priority: 0},
		{Key: "CTRL+C", Action: "exit after stop", Priority: 2},
	},
	StateHelp: {
		{Key: "ESC", Action: "close", Priority: 0},
		{Key: "?", Action: "close", Priority: 3},
	},
}

// Hints returns the footer key hints for a state. Unknown states fall back to
// the idle hints so a missing entry degrades to something useful rather than
// an empty row.
func Hints(s State) []Hint {
	if hints, ok := hintTable[s]; ok {
		out := make([]Hint, len(hints))
		copy(out, hints)
		return out
	}
	return Hints(StateIdle)
}

// Info returns the `└─` info-line text for a state: what the user should do
// next, and why the UI is in this state. Context fills in live specifics.
func Info(s State, ctx Context) string {
	switch s {
	case StateIdle:
		return "enter sends · / for commands · @ for files · shift+tab switches mode"

	case StateRunning:
		return "esc interrupts · ctrl+c stops · enter queues a message for after this turn"

	case StateRunningTool:
		if ctx.Tool != "" {
			return fmt.Sprintf("running %s · esc interrupts · ctrl+j steers after this tool", ctx.Tool)
		}
		return "esc interrupts · ctrl+j steers after the current tool"

	case StateAwaitingPermission:
		if ctx.Tool != "" {
			return fmt.Sprintf("%s needs permission · y allows once, a always allows, n denies", ctx.Tool)
		}
		return "waiting for permission · y allows once, a always allows, n denies"

	case StateAwaitingAnswer:
		return "the agent asked a question · answer it, or esc to dismiss and let it continue"

	case StateInterrupted:
		msg := "interrupted"
		if ctx.ToolsRun > 0 {
			msg = fmt.Sprintf("interrupted after %s", plural(ctx.ToolsRun, "tool"))
		}
		return msg + " · enter resumes with a new message, /rewind undoes the turn"

	case StateCancelled:
		return "run cancelled · nothing was lost, enter sends a new message, /rewind undoes the turn"

	case StateQueued:
		base := fmt.Sprintf("%s queued", plural(ctx.Queued, "message"))
		return base + " · sends when this turn ends · ctrl+j sends after the current tool · esc clears"

	case StateRetrying:
		var b strings.Builder
		if ctx.ErrCode != "" {
			b.WriteString(ctx.ErrCode)
			if ctx.Detail != "" {
				b.WriteString(" " + ctx.Detail)
			}
		} else if ctx.Detail != "" {
			b.WriteString(ctx.Detail)
		} else {
			b.WriteString("request failed")
		}
		if ctx.Attempt > 0 && ctx.MaxAttempts > 0 {
			fmt.Fprintf(&b, " · retry %d/%d", ctx.Attempt, ctx.MaxAttempts)
		}
		if ctx.NextRetryIn > 0 {
			fmt.Fprintf(&b, " · next in %s", roundDuration(ctx.NextRetryIn))
		}
		b.WriteString(" · /error for details")
		return b.String()

	case StateError:
		var b strings.Builder
		if ctx.ErrCode != "" {
			fmt.Fprintf(&b, "failed with %s", ctx.ErrCode)
		} else {
			b.WriteString("request failed")
		}
		if ctx.MaxAttempts > 0 {
			fmt.Fprintf(&b, " after %s", plural(ctx.MaxAttempts, "attempt"))
		}
		b.WriteString(" · /error shows the full log · enter retries")
		return b.String()

	case StateBrowsing:
		return "browsing the transcript · ↑↓ scrolls, tab returns to the prompt"

	case StateDockFocused:
		return "background tasks · enter inspects, ↑ or esc returns to the prompt"

	case StatePaletteOpen:
		return "↑↓ moves · enter selects · esc closes the list and keeps what you typed"

	case StateDiffOpen:
		return "reviewing changes · ↑↓ scrolls, esc closes"

	case StateReviewProposal:
		return "this edit is proposed, not written · a accepts and applies, r rejects it"

	case StateSessionBrowser:
		return "pick a session to resume · esc closes and keeps the current one"

	case StateConfirmClear:
		return "press esc again to clear the prompt · ctrl+p brings it back afterwards"

	case StateConfirmQuit:
		return "press ctrl+c again to exit · any other key cancels"

	case StateStopFirst:
		return "the agent is still running — stop it first with esc, then ctrl+c twice to exit"

	case StateHelp:
		return "esc or ? closes this help"
	}
	return Info(StateIdle, ctx)
}

// plural renders "1 tool" / "3 tools" without a dedicated dependency.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// roundDuration renders a retry delay compactly: sub-second delays as
// milliseconds, everything else to the nearest second.
func roundDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}
