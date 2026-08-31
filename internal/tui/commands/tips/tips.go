// Package tips holds the per-command tip material shown in the TUI's info
// line, one Go file per command. Each command file registers its CommandTip in
// init(), so All() returns the full set in deterministic (sorted-filename)
// order with no parsing or embedding step at startup.
package tips

// CommandTip is the tip material for one command.
type CommandTip struct {
	Name string
	// Tip is the one-line infoline hint.
	Tip string
	// Personalized is a context-aware variant; {model}, {mode}, {artifacts}
	// and {workdir} are replaced with live values by the app.
	Personalized string
	// Body is the comprehensive guidance block.
	Body string
}

// InfolineTip resolves the text the info line should show for a command:
// the personalized variant when present, else the plain tip.
func (ct CommandTip) InfolineTip() string {
	if ct.Personalized != "" {
		return ct.Personalized
	}
	return ct.Tip
}

var (
	byName = map[string]CommandTip{}
	order  []string
)

// register records one command's tip material. Called from each per-command
// file's init(); a duplicate name panics at startup rather than silently
// shadowing an earlier entry.
func register(ct CommandTip) {
	if _, dup := byName[ct.Name]; dup {
		panic("tips: duplicate command tip for " + ct.Name)
	}
	byName[ct.Name] = ct
	order = append(order, ct.Name)
}

// For returns the tip material for a command name.
func For(name string) (CommandTip, bool) {
	ct, ok := byName[name]
	return ct, ok
}

// All returns every registered tip in registration order, which is the
// rotation order the idle info line follows.
func All() []CommandTip {
	out := make([]CommandTip, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}
