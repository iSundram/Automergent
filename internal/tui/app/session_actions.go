package app

import (
	"fmt"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	roots := append([]string{"."}, a.extraSearchDirs...)
	var b strings.Builder
	count := 0
	for _, root := range roots {
		rootLabel := filepath.Join(a.workDir, root)
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
			display := path
			if root != "." && strings.HasPrefix(display, root+string(filepath.Separator)) {
				display = rootLabel + display[len(root):]
			}
			for i, line := range strings.Split(string(data), "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
					if count == 0 {
						b.WriteString("Search results for `" + query + "`:\n")
					}
					b.WriteString(fmt.Sprintf("%s:%d: %s\n", display, i+1, strings.TrimSpace(line)))
					count++
					if count >= 50 {
						break
					}
				}
			}
			return nil
		})
	}
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

// projectSessions filters the session list to those belonging to the current
// project (matching work directory). Sessions that predate work-dir tracking
// (no recorded workdir) are kept so they remain resumable.
func (a *App) projectSessions(all []*session.Session) []*session.Session {
	if a.workDir == "" || len(all) == 0 {
		return all
	}
	out := make([]*session.Session, 0, len(all))
	for _, s := range all {
		if s.WorkDir == "" || s.WorkDir == a.workDir {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) showSessions() {
	if a.storage != nil {
		if sessions, err := a.storage.List(); err == nil {
			a.sessionBrowser.SetSessions(a.projectSessions(sessions))
		} else {
			a.statusBar.SetStatus("Error listing sessions: " + err.Error())
			return
		}
	} else {
		a.sessionBrowser.SetSessions([]*session.Session{a.sess})
	}
	a.sessionBrowser.SetCurrent(a.sess.ID)
	a.sessionBrowser.Show()
	// The key event that triggered this command must not also be delivered to
	// the freshly shown browser (it would be interpreted as "select" and the
	// picker would close instantly, resuming the first session).
	a.swallowNextKey = true
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
	return a.restoreSession(s)
}

// newSession saves the current conversation and swaps in a fresh session.
// Session-scoped state (rewind checkpoints, API error history, usage stats)
// is reset too — none of it may leak across sessions.
func (a *App) newSession() {
	if a.storage != nil && a.sess != nil && len(a.sess.Messages) > 0 {
		_ = a.storage.Save(a.sess)
	}
	a.conversation.Clear()
	a.checkpoints = nil
	a.apiErrors = nil
	a.sess = session.New()
	a.sess.Provider, a.sess.Model = a.cfg.Provider, a.cfg.Model
	a.sess.WorkDir = a.workDir
	a.ag.SetSession(a.sess)
	if a.persist != nil {
		a.persist.SetSession(a.sess)
	}
	a.updateActiveTokens()
	a.stats.TotalCost = 0
	a.stats.InputTokens = 0
	a.stats.OutputTokens = 0
	a.header.SetTokens(0)
	a.statusBar.SetStatus("New session started")
}

// clearConversationView clears only the rendered conversation.
func (a *App) clearConversationView() {
	a.conversation.Clear()
	a.statusBar.SetStatus("Conversation cleared")
}

// resetSessionHistory clears both the view and the persisted message history.
func (a *App) resetSessionHistory() {
	a.conversation.Clear()
	a.sess.SetMessages(nil)
	a.statusBar.SetStatus("History reset")
}

// --- Conversation history: checkpoints, branching, context files ---

// maxCheckpoints bounds in-memory rewind points (oldest dropped first).
const maxCheckpoints = 50

type conversationCheckpoint struct {
	label    string
	at       time.Time
	messages []ai.Message
}

// captureCheckpoint snapshots the conversation before a new agent turn.
func (a *App) captureCheckpoint(prompt string) {
	if a.sess == nil || len(a.sess.Messages) == 0 {
		return
	}
	snapshot := make([]ai.Message, len(a.sess.Messages))
	copy(snapshot, a.sess.Messages)
	label := strings.TrimSpace(prompt)
	if idx := strings.IndexAny(label, "\n"); idx > 0 {
		label = label[:idx]
	}
	if len(label) > 80 {
		label = label[:77] + "..."
	}
	a.checkpoints = append(a.checkpoints, conversationCheckpoint{
		label:    label,
		at:       time.Now(),
		messages: snapshot,
	})
	if len(a.checkpoints) > maxCheckpoints {
		a.checkpoints = a.checkpoints[len(a.checkpoints)-maxCheckpoints:]
	}
}

// checkpointSummaries exposes the rewind points oldest-first.
func (a *App) checkpointSummaries() []commands.CheckpointInfo {
	out := make([]commands.CheckpointInfo, 0, len(a.checkpoints))
	for i, cp := range a.checkpoints {
		out = append(out, commands.CheckpointInfo{Index: i + 1, Label: cp.label, At: cp.at, Messages: len(cp.messages)})
	}
	return out
}

// rewindTo restores the conversation to the state captured before turn n
// (1-based). Future checkpoints are discarded; the session is persisted.
func (a *App) rewindTo(n int) error {
	if n < 1 || n > len(a.checkpoints) {
		return fmt.Errorf("checkpoint %d out of range (1-%d)", n, len(a.checkpoints))
	}
	cp := a.checkpoints[n-1]
	restored := make([]ai.Message, len(cp.messages))
	copy(restored, cp.messages)
	a.checkpoints = a.checkpoints[:n-1]
	a.sess.SetMessages(restored)
	a.conversation.Clear()
	for _, m := range restored {
		a.replayMessage(m)
	}
	a.updateActiveTokens()
	a.stats.TotalCost = 0
	if a.storage != nil {
		_ = a.storage.Save(a.sess)
	}
	a.statusBar.SetStatus(fmt.Sprintf("Rewound to checkpoint %d", n))
	return nil
}

// branchSession forks the current conversation into a fresh named session.
func (a *App) branchSession(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name must not be empty")
	}
	if a.storage != nil && a.sess != nil && len(a.sess.Messages) > 0 {
		_ = a.storage.Save(a.sess)
	}
	copied := make([]ai.Message, len(a.sess.Messages))
	copy(copied, a.sess.Messages)
	s := session.New()
	s.Provider, s.Model = a.cfg.Provider, a.cfg.Model
	s.WorkDir = a.workDir
	s.Title = "branch: " + name
	s.SetMessages(copied)
	if err := a.restoreSession(s); err != nil {
		return err
	}
	a.stats.TotalCost = 0
	a.statusBar.SetStatus("Branched into new session: " + name)
	return nil
}

// touchedFiles lists unique file paths seen in tool calls this session.
func (a *App) touchedFiles() []string {
	seen := map[string]bool{}
	var out []string
	for i := len(a.sess.Messages) - 1; i >= 0; i-- {
		for _, part := range a.sess.Messages[i].Content {
			if part.Type != ai.ContentTypeToolCall || part.ToolCall == nil {
				continue
			}
			for _, key := range []string{"path", "file_path", "file"} {
				if v, ok := part.ToolCall.Args[key].(string); ok && v != "" && !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
		}
		if len(out) >= 100 {
			break
		}
	}
	// Reverse so most recently touched come last? Keep discovery order stable:
	// collected newest-first; reverse for chronological output.
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// addSearchDir registers an extra read-only search root for /search.
func (a *App) addSearchDir(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("stat %s: %w", abs, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", abs)
	}
	for _, d := range a.extraSearchDirs {
		if d == abs {
			return fmt.Errorf("%s already added", abs)
		}
	}
	a.extraSearchDirs = append(a.extraSearchDirs, abs)
	return nil
}
