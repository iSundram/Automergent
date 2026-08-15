package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/session"
)

// exportConversation writes a readable, deterministic Markdown transcript.
func (a *App) exportConversation(path string) error {
	if strings.TrimSpace(path) == "" {
		path = "conversation.md"
	}
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return fmt.Errorf("export path must be relative to the workspace")
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve export path: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("export path must stay inside the workspace")
	}
	var b strings.Builder
	b.WriteString("# Automergent Conversation\n\n")
	for _, message := range a.sess.Messages {
		b.WriteString("## ")
		b.WriteString(strings.Title(string(message.Role)))
		b.WriteString("\n\n")
		b.WriteString(message.TextContent())
		b.WriteString("\n\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func (a *App) searchWorkspace(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Usage: /search <query>"
	}
	var b strings.Builder
	count := 0
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if path != "." && (filepath.Base(path) == ".git" || filepath.Base(path) == "node_modules" || filepath.Base(path) == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 1<<20 || count >= 50 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || strings.IndexByte(string(data), 0) >= 0 {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				if count == 0 {
					b.WriteString("Search results for `" + query + "`:\n")
				}
				b.WriteString(fmt.Sprintf("%s:%d: %s\n", path, i+1, strings.TrimSpace(line)))
				count++
				if count >= 50 {
					break
				}
			}
		}
		return nil
	})
	if count == 0 {
		return fmt.Sprintf("No matches found for %q", query)
	}
	return b.String()
}

func (a *App) showSessions() {
	if a.storage != nil {
		if sessions, err := a.storage.List(); err == nil {
			a.sessionBrowser.SetSessions(sessions)
		} else {
			a.statusBar.SetStatus("Error listing sessions: " + err.Error())
			return
		}
	} else {
		a.sessionBrowser.SetSessions([]*session.Session{a.sess})
	}
	a.sessionBrowser.Show()
	a.layout()
}

func (a *App) resumeSession(id string) error {
	if a.storage == nil {
		return fmt.Errorf("session storage unavailable")
	}
	s, err := a.storage.Load(id)
	if err != nil {
		return err
	}
	a.sess = s
	a.ag.SetSession(s)
	a.conversation.Clear()
	for _, m := range s.Messages {
		a.conversation.AddMessage(string(m.Role), m.TextContent(), false)
	}
	a.stats.InputTokens, a.stats.OutputTokens = s.TotalInputTokens, s.TotalOutputTokens
	a.header.SetTokens(s.TotalInputTokens + s.TotalOutputTokens)
	if s.Provider != "" {
		_ = a.switchProvider(s.Provider, s.Model)
	}
	a.statusBar.SetStatus("Session resumed: " + s.ID)
	a.layout()
	return nil
}
