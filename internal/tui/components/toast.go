package components

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// toastTTL is how long a toast stays visible.
const toastTTL = 6 * time.Second

const maxToasts = 3

// ToastsTickMsg drives toast expiry; apps forward it to Toasts.Update.
type ToastsTickMsg struct{}

// Toasts is a non-blocking notification stack rendered above the footer.
// Warnings/errors surface here instead of polluting the conversation log.
type Toasts struct {
	styles *themes.Styles
	items  []Toast
	width  int
}

// Toast is one notification card.
type Toast struct {
	Level string // "warn" | "error" | "info"
	Title string
	Msg   string
	Until time.Time
}

// NewToasts creates the toast stack.
func NewToasts(styles *themes.Styles) *Toasts {
	return &Toasts{styles: styles}
}

// SetSize updates the available width for alignment.
func (t *Toasts) SetSize(w int) { t.width = w }

// Push adds a toast (evicting the oldest beyond the cap) and starts the
// expiry ticker.
func (t *Toasts) Push(level, title, msg string) tea.Cmd {
	t.items = append(t.items, Toast{
		Level: level,
		Title: title,
		Msg:   msg,
		Until: time.Now().Add(toastTTL),
	})
	if len(t.items) > maxToasts {
		t.items = t.items[len(t.items)-maxToasts:]
	}
	return t.tick()
}

func (t *Toasts) tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return ToastsTickMsg{} })
}

// Update prunes expired toasts and keeps the ticker alive while visible.
func (t *Toasts) Update(msg tea.Msg) (*Toasts, tea.Cmd) {
	if _, ok := msg.(ToastsTickMsg); !ok {
		return t, nil
	}
	t.prune()
	if len(t.items) > 0 {
		return t, t.tick()
	}
	return t, nil
}

func (t *Toasts) prune() {
	now := time.Now()
	kept := t.items[:0]
	for _, item := range t.items {
		if now.Before(item.Until) {
			kept = append(kept, item)
		}
	}
	t.items = kept
}

// Active reports whether any toasts are showing.
func (t *Toasts) Active() bool { t.prune(); return len(t.items) > 0 }

// Dismiss clears all toasts.
func (t *Toasts) Dismiss() { t.items = nil }

// View renders the stacked cards, right-aligned, or "" when nothing active.
func (t *Toasts) View() string {
	t.prune()
	if len(t.items) == 0 {
		return ""
	}
	var cards []string
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		MaxWidth(t.width - 4)
	for _, item := range t.items {
		var accent = t.styles.T.Yellow
		switch item.Level {
		case "error":
			accent = t.styles.T.Red
		case "info":
			accent = t.styles.T.Blue
		}
		header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(item.Title)
		body := ""
		if item.Msg != "" && item.Msg != item.Title {
			body = "\n" + lipgloss.NewStyle().Foreground(t.styles.T.Subtext).Render(item.Msg)
		}
		card := cardStyle.Copy().
			BorderForeground(accent).
			Render(header + body)
		cards = append(cards, card)
	}
	joined := lipgloss.JoinVertical(lipgloss.Right, cards...)

	// Right-align within the terminal width.
	lines := strings.Split(joined, "\n")
	maxW := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}
	pad := t.width - maxW - 2
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	left := strings.Repeat(" ", pad)
	for i, l := range lines {
		b.WriteString(left + l)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
