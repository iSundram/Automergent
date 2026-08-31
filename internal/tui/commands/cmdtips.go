package commands

// Per-command tips: one markdown file per command under tips/, embedded at
// build time. Each file carries a one-line infoline tip, a personalized
// variant (with {placeholders} the app fills from live state), and a
// comprehensive body for the help overlay.

import (
	"embed"
	"strings"
)

//go:embed tips/*.md
var tipsFS embed.FS

// CommandTip is the parsed tip material for one command.
type CommandTip struct {
	Name string
	// Tip is the one-line infoline hint.
	Tip string
	// Personalized is a context-aware variant; {model}, {mode}, {sessions},
	// {artifacts} and {workdir} are replaced with live values.
	Personalized string
	// Body is the comprehensive guidance block.
	Body string
}

// parseTipFile parses the tip format:
//
//	tip: one line
//	personalized: one line with {placeholders}
//	---
//	comprehensive body
func parseTipFile(name, content string) CommandTip {
	ct := CommandTip{Name: name}
	rest := content
	if idx := strings.Index(rest, "\n---"); idx >= 0 {
		head, body := rest[:idx], rest[idx+4:]
		rest = head
		ct.Body = strings.TrimSpace(body)
	}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "tip:"):
			ct.Tip = strings.TrimSpace(strings.TrimPrefix(line, "tip:"))
		case strings.HasPrefix(line, "personalized:"):
			ct.Personalized = strings.TrimSpace(strings.TrimPrefix(line, "personalized:"))
		}
	}
	return ct
}

// commandTips holds every parsed tip, keyed by command name.
var commandTips = func() map[string]CommandTip {
	out := map[string]CommandTip{}
	entries, err := tipsFS.ReadDir("tips")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := tipsFS.ReadFile("tips/" + e.Name())
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		out[name] = parseTipFile(name, string(data))
	}
	return out
}()

// Tip returns the tip material for a command name (or alias).
func (r *Registry) Tip(name string) (CommandTip, bool) {
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	ct, ok := commandTips[name]
	return ct, ok
}

// InfolineTip resolves the text the info line should show for a command:
// the personalized variant when present, else the plain tip.
func (ct CommandTip) InfolineTip() string {
	if ct.Personalized != "" {
		return ct.Personalized
	}
	return ct.Tip
}
