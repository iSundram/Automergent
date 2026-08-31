package components

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Toasts is RETIRED as a visible surface. Errors already have two homes —
// the in-conversation error card and the /error log — and the rounded toast
// cards duplicated both while covering conversation content. The type stays
// (dozens of Push call sites across the app) but Push no longer stores
// anything and View always renders nothing, so the component costs zero
// rows and never appears.
type Toasts struct {
	styles *themes.Styles
}

// Toast is one notification card (kept for API compatibility).
type Toast struct {
	Level string // "warn" | "error" | "info"
	Title string
	Msg   string
	Until time.Time
}

// ToastsTickMsg drives toast expiry; apps forward it to Toasts.Update.
type ToastsTickMsg struct{}

// NewToasts creates the (inert) toast stack.
func NewToasts(styles *themes.Styles) *Toasts {
	return &Toasts{styles: styles}
}

// SetSize updates the available width (no-op retained for API compatibility).
func (t *Toasts) SetSize(w int) {}

// Push accepts a notification and discards it. Callers keep their signatures;
// errors reach the user through the error card and /error instead.
func (t *Toasts) Push(level, title, msg string) tea.Cmd {
	return nil
}

// Update is a no-op: nothing is ever stored, so there is nothing to expire.
func (t *Toasts) Update(msg tea.Msg) (*Toasts, tea.Cmd) {
	return t, nil
}

// Active always reports false: the stack never shows.
func (t *Toasts) Active() bool { return false }

// Dismiss is a no-op.
func (t *Toasts) Dismiss() {}

// View renders nothing, permanently.
func (t *Toasts) View() string { return "" }
