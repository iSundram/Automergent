package render

// The shared vocabulary for background work.
//
// Three subsystems name the same states in different words. internal/tools/agent
// says explore | task | general-purpose | code-review; internal/agent/agentdef
// says general-purpose | explore | review | contexter | coordinator; the shell
// manager says running | completed | failed | cancelled; and the conversation's
// tool cards say running | error | done. Each of the three UI surfaces that
// displayed them — dock, board, tool card — mapped the words to marks and
// colours itself, with a different switch and a different set of defaults, so
// the same failed agent could be a red ✗ in one place and a muted ○ two rows
// below. One surface even coloured agent types by a vocabulary its own data
// source never emitted, so the colours simply never applied.
//
// This file is the single mapping. Every surface asks it, so a state has one
// mark and one colour everywhere in the program.

import (
	"image/color"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Status is the canonical lifecycle state of a background task.
type Status string

const (
	// StatusRunning is in flight and producing.
	StatusRunning Status = "running"
	// StatusIdle has started but not yet reported activity.
	StatusIdle Status = "idle"
	// StatusQueued is admitted but waiting for a slot or a dependency.
	StatusQueued Status = "queued"
	// StatusDone finished successfully.
	StatusDone Status = "done"
	// StatusFailed finished unsuccessfully.
	StatusFailed Status = "failed"
	// StatusStopped was cancelled or killed before finishing.
	StatusStopped Status = "stopped"
)

// CanonicalStatus folds any of the three backends' status words into one
// Status. An unrecognised word becomes StatusIdle rather than being passed
// through: a status the UI cannot map is a status it cannot colour, and
// inventing a row for it is worse than showing it as not-yet-started.
func CanonicalStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "running", "active", "in_progress", "in progress":
		return StatusRunning
	case "queued", "pending", "waiting", "blocked":
		return StatusQueued
	case "completed", "complete", "done", "ok", "success":
		return StatusDone
	case "failed", "fail", "error":
		return StatusFailed
	case "cancelled", "canceled", "killed", "stopped", "interrupted":
		return StatusStopped
	default:
		return StatusIdle
	}
}

// Terminal reports whether a status will not change again.
func (s Status) Terminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusStopped
}

// Mark returns the single glyph for a status, from the charter.
func (s Status) Mark() string {
	switch s {
	case StatusRunning:
		return GlyphRun
	case StatusQueued:
		return GlyphStopped
	case StatusDone:
		return GlyphOK
	case StatusFailed:
		return GlyphFail
	case StatusStopped:
		return GlyphStopped
	default:
		return GlyphIdle
	}
}

// Label returns the status word to print. These are the words the UI shows, as
// distinct from the many words the backends use.
func (s Status) Label() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusQueued:
		return "queued"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusStopped:
		return "stopped"
	default:
		return "starting"
	}
}

// Color returns the theme colour for a status. Running is the accent rather
// than yellow: yellow is the UI's warning colour everywhere else, and normal
// progress is not a warning.
func (s Status) Color(t *themes.Theme) color.Color {
	switch s {
	case StatusRunning:
		return t.Accent
	case StatusQueued:
		return t.Muted
	case StatusDone:
		return t.Green
	case StatusFailed:
		return t.Red
	case StatusStopped:
		return t.Muted
	default:
		return t.Subtext
	}
}

// AgentKind is the canonical short name for an agent's role, folded from both
// backend vocabularies so one name reaches the display.
type AgentKind string

const (
	KindGeneral     AgentKind = "general"
	KindExplore     AgentKind = "explore"
	KindReview      AgentKind = "review"
	KindContexter   AgentKind = "context"
	KindCoordinator AgentKind = "coord"
	KindCustom      AgentKind = "custom"
)

// CanonicalKind folds any agent-type spelling either backend produces into one
// AgentKind. A user-defined agent from .agents/*.md lands on KindCustom, which
// is honest: the UI knows it is an agent and does not know its role.
func CanonicalKind(s string) AgentKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "general", "general-purpose", "gen", "default", "task", "claude":
		return KindGeneral
	case "explore", "search", "find", "research":
		return KindExplore
	case "review", "code-review", "audit", "check", "security-review":
		return KindReview
	case "contexter", "context", "ctx", "compact":
		return KindContexter
	case "coordinator", "coord", "orchestrate", "orch", "swarm":
		return KindCoordinator
	default:
		return KindCustom
	}
}

// Color returns the theme colour for an agent kind. The palette is chosen so
// that a coordinator and its children are visually distinguishable at a glance
// in a nested list, not to be decorative.
func (k AgentKind) Color(t *themes.Theme) color.Color {
	switch k {
	case KindExplore:
		return t.Blue
	case KindReview:
		return t.Yellow
	case KindGeneral:
		return t.Green
	case KindContexter:
		return t.Cyan
	case KindCoordinator:
		return t.Magenta
	default:
		return t.Subtext
	}
}
