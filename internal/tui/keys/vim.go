package keys

// Vim returns vim-style key bindings.
func Vim() *Bindings {
	b := Default()
	b.ScrollUp.SetKeys("k", "up")
	b.ScrollDown.SetKeys("j", "down")
	b.PageUp.SetKeys("ctrl+u", "pgup")
	b.PageDown.SetKeys("ctrl+d", "pgdown")
	b.Submit.SetKeys("enter")
	b.Quit.SetKeys("q", "ctrl+c")
	b.Cancel.SetKeys("esc")
	return b
}
