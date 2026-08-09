package keys

import "github.com/charmbracelet/bubbles/key"

// Bindings holds all key bindings for the TUI.
type Bindings struct {
	Submit      key.Binding
	Quit        key.Binding
	Cancel      key.Binding
	ToggleDiff  key.Binding
	ToggleLSP   key.Binding
	SessionList key.Binding
	NewSession  key.Binding
	ClearInput  key.Binding
	ScrollUp    key.Binding
	ScrollDown  key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Confirm     key.Binding
	Deny        key.Binding
}

// Default returns the default key bindings.
func Default() *Bindings {
	return &Bindings{
		Submit:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		Quit:        key.NewBinding(key.WithKeys("ctrl+c", "ctrl+q"), key.WithHelp("ctrl+c", "quit")),
		Cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		ToggleDiff:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "toggle diff")),
		ToggleLSP:   key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "toggle LSP")),
		SessionList: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "sessions")),
		NewSession:  key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new session")),
		ClearInput:  key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear input")),
		ScrollUp:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
		ScrollDown:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
		PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Confirm:     key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "confirm")),
		Deny:        key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "deny")),
	}
}
