package keys

// Emacs returns Emacs-style key bindings.
func Emacs() *Bindings {
	b := Default()
	b.ScrollUp.SetKeys("ctrl+p", "up")
	b.ScrollDown.SetKeys("ctrl+n", "down")
	b.PageUp.SetKeys("alt+v", "pgup")
	b.PageDown.SetKeys("ctrl+v", "pgdown")
	b.ClearInput.SetKeys("ctrl+k")
	b.Cancel.SetKeys("ctrl+g", "esc")
	return b
}

// Get returns key bindings by name.
func Get(name string) *Bindings {
	switch name {
	case "vim":
		return Vim()
	case "emacs":
		return Emacs()
	default:
		return Default()
	}
}
