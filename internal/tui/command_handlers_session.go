package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// handleApprovalsCommand lists or revokes always-allow tool approvals.
// Usage: /approvals            -> list all approvals with indices
//
//	/approvals revoke <i> -> revoke approval at 1-based index
func (a *App) handleApprovalsCommand(args []string) {
	approvals := a.ag.Approvals()
	if len(args) > 0 && args[0] == "revoke" {
		if len(args) < 2 {
			a.conversation.AddMessage("system", "Usage: /approvals revoke <index>", false)
			return
		}
		idx, err := strconv.Atoi(args[1])
		if err != nil || idx < 1 || idx > len(approvals) {
			a.conversation.AddMessage("system", fmt.Sprintf("Invalid index. Use /approvals to list indices (1-%d).", len(approvals)), false)
			return
		}
		scope := approvals[idx-1].Scope
		if a.ag.RevokeApproval(scope) {
			if a.storage != nil {
				_ = a.storage.Save(a.sess)
			}
			a.conversation.AddMessage("system", "Revoked approval: "+formatApprovalScope(scope), false)
			a.statusBar.SetStatus("Approval revoked")
		} else {
			a.conversation.AddMessage("system", "Approval not found.", false)
		}
		return
	}

	if len(approvals) == 0 {
		a.conversation.AddMessage("system", "No always-allow approvals in this session. Approve a tool with 'a' (always) to grant one.", false)
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Always-allow approvals (%d):\n", len(approvals)))
	for i, ap := range approvals {
		fmt.Fprintf(&b, "%d. %s\n", i+1, formatApprovalScope(ap.Scope))
	}
	b.WriteString("\nRevoke with: /approvals revoke <index>")
	a.conversation.AddMessage("system", b.String(), false)
}

// formatApprovalScope renders a persisted scope key in a readable form,
// e.g. 'shell write (high risk) — /projects/alpha'.
func formatApprovalScope(scope string) string {
	project := ""
	if strings.HasPrefix(scope, "project=") {
		if idx := strings.Index(scope, ";name="); idx > 0 {
			project = strings.TrimPrefix(scope[7:idx], "/")
		}
	}
	name := "tool"
	if idx := strings.Index(scope, `name="`); idx >= 0 {
		rest := scope[idx+len(`name="`):]
		if end := strings.Index(rest, `"`); end > 0 {
			name = rest[:end]
		}
	}
	action := "read"
	if idx := strings.Index(scope, "action="); idx >= 0 {
		rest := scope[idx+len("action="):]
		if end := strings.Index(rest, ";"); end > 0 {
			action = rest[:end]
		}
	}
	risk := ""
	if idx := strings.Index(scope, "risk="); idx >= 0 {
		risk = scope[idx+len("risk="):]
	}
	label := fmt.Sprintf("%s %s", name, action)
	if risk != "" {
		label += fmt.Sprintf(" (%s risk)", risk)
	}
	if project != "" {
		label += " — " + project
	}
	return label
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
	var s *session.Session
	if a.persist != nil {
		// Prefer a crash-recovery point for this session (may contain more
		// recent messages than the last clean save), then fall back to disk.
		recovered, err := a.persist.ResumeSession(id, a.storage)
		if err != nil {
			return err
		}
		s = recovered
	} else {
		loaded, err := a.storage.Load(id)
		if err != nil {
			return err
		}
		s = loaded
	}
	a.sess = s
	a.ag.SetSession(s)
	if a.persist != nil {
		a.persist.SetSession(s)
	}
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
