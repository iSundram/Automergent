package app

// Session lifecycle: runs, restore/replay, rewind, cancellation.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"
	"github.com/iSundram/Automergent/internal/tui/render"
	"os"
	"strings"
	"time"
)

func (a *App) startAgent(prompt string) tea.Cmd {
	return a.runAgentTurn(prompt, prompt, "")
}

// startAgentCommand starts a run whose user turn is a prompt-command
// expansion: the conversation shows the expansion behind the "/command"
// chip, and the agent receives it wrapped in a provenance header. The turn
// must not ALSO be echoed as a plain user message — runAgentTurn renders
// the command bubble instead of the generic echo.
func (a *App) startAgentCommand(displayPrompt, agentPrompt, command string) tea.Cmd {
	return a.runAgentTurn(displayPrompt, agentPrompt, command)
}

func (a *App) runAgentTurn(displayPrompt, agentPrompt, command string) tea.Cmd {
	// Checkpoint the pre-turn conversation so /rewind can return here.
	a.captureCheckpoint(displayPrompt)
	prompt := a.expandPrompt(agentPrompt)
	if strings.HasPrefix(prompt, "!") {
		return a.runShellPassthrough(prompt[1:])
	}
	a.thinking = true
	a.streamedReply = false
	a.runChars = 0
	a.runStart = time.Now()
	a.spin.SetMeta("")
	a.lastRunSummary = ""
	a.activeTool = ""
	a.runToolCount = 0
	a.lastOutcome = outcomeNone
	a.clearRetryState()
	a.setSpinVerb("")
	a.spin.Start()
	if command != "" {
		a.conversation.AddUserCommandMessage(command, displayPrompt)
	} else {
		a.conversation.AddMessage("user", prompt, false)
	}
	a.updateActiveTokens()
	a.statusBar.SetStatus("Thinking…")
	a.refreshChrome()
	a.layout() // Adjust for thinking spinner
	go func() { _ = a.ag.Run(a.ctx, prompt) }()
	return a.waitForAgentEvent()
}

// scheduleStreamTick arms the coalesced streaming render timer.

func (a *App) expandPrompt(prompt string) string {
	words := strings.Fields(prompt)
	for i, word := range words {
		if strings.HasPrefix(word, "@") {
			path := word[1:]
			content, err := os.ReadFile(path)
			if err == nil {
				words[i] = fmt.Sprintf("\n--- %s ---\n%s\n", path, string(content))
			}
		}
	}
	return strings.Join(words, " ")
}

// passthroughInlineWindow is how long a "!command" from the input runs inline
// before it moves to the background dock. Commands that finish inside the
// window answer in place as a terminal card; anything still running past it
// becomes a dock row and keeps going until it completes.
const passthroughInlineWindow = 10 * time.Second

// passthroughDoneMsg reports an inline "!command" that finished within the
// window: its output becomes a terminal card in the conversation.
type passthroughDoneMsg struct {
	id       string
	command  string
	output   string
	exitCode int
	duration time.Duration
}

// passthroughBackgroundedMsg reports an inline "!command" that outlived the
// window: the session is re-marked as background and takes a dock row.
type passthroughBackgroundedMsg struct {
	id      string
	command string
}

// runShellPassthrough runs an "!command" from the input without blocking the
// TUI. The command is spawned through the shell manager (so stop/inspect work
// exactly like any background command), polled for the inline window, and
// either resolved to a terminal card or promoted to the dock. The output is
// never faked into an assistant message — the user typed shell, the reply is
// a terminal card.
func (a *App) runShellPassthrough(command string) tea.Cmd {
	a.conversation.AddMessage("user", "!"+command, false)
	id := toolsshell.GetManager().NextID()
	// The running card goes up immediately, so the transcript shows "$ command"
	// while it executes instead of only after the fact.
	a.conversation.AddToolLifecycleStart(id, "bash", fmt.Sprintf(`{"command":%q}`, command), "")
	a.passthroughCards[id] = true
	runner := toolsshell.NewAsyncRunnerTool(0)
	started := time.Now()

	return func() tea.Msg {
		res, err := runner.Execute(context.Background(), map[string]any{
			"command":  command,
			"mode":     "async",
			"shell_id": id,
		})
		if err != nil {
			return passthroughDoneMsg{id: id, command: command, output: err.Error(), exitCode: -1, duration: time.Since(started)}
		}
		if res.IsError {
			return passthroughDoneMsg{id: id, command: command, output: res.Content, exitCode: -1, duration: time.Since(started)}
		}
		// Keep the session out of the user-visible background set while it is
		// inside the inline window: no dock row, no completion toast. It is
		// re-marked only if the command outlives the window.
		toolsshell.GetManager().MarkBackground(id, false, false)

		for {
			session, ok := toolsshell.GetManager().Get(id)
			if !ok || session.IsCompleted() {
				break
			}
			if time.Since(started) >= passthroughInlineWindow {
				toolsshell.GetManager().MarkBackground(id, true, false)
				return passthroughBackgroundedMsg{id: id, command: command}
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Finished within the window: read the output and settle the record so
		// it can never linger in the history as a phantom "running" row.
		msg := passthroughDoneMsg{id: id, command: command, duration: time.Since(started)}
		if session, ok := toolsshell.GetManager().Get(id); ok {
			session.Lock()
			msg.output = session.Stdout.String()
			if session.Stderr.Len() > 0 {
				msg.output += "\n[stderr]\n" + session.Stderr.String()
			}
			msg.exitCode = session.ExitCode
			session.Unlock()
		}
		if rec, ok := toolsshell.GetManager().GetRecord(id); ok && rec.Status == toolsshell.SessionStatusRunning {
			status := toolsshell.SessionStatusCompleted
			if msg.exitCode != 0 {
				status = toolsshell.SessionStatusFailed
			}
			_ = toolsshell.GetManager().UpdateStatus(id, status, msg.exitCode, nil)
		}
		if msg.output == "" {
			msg.output = "(no output)"
		}
		return msg
	}
}

// handlePassthroughDone fills in the terminal card for an inline "!command".
func (a *App) handlePassthroughDone(m passthroughDoneMsg) tea.Cmd {
	delete(a.passthroughCards, m.id)
	result := tools.Result{Content: m.output}
	if m.exitCode != 0 {
		result.IsError = true
		if m.exitCode > 0 {
			result.Content = fmt.Sprintf("%s\n[exit %d]", m.output, m.exitCode)
		}
	}
	a.conversation.AddToolLifecycleDone(m.id, "bash", "", "", m.duration, result, a.conversation.ReviewMode())
	a.refreshDock()
	return nil
}

// handlePassthroughBackgrounded promotes a running "!command" to the dock.
// From here on it is an ordinary background command: it keeps its dock row
// while it runs, its completion toast fires, and the row leaves when done.
func (a *App) handlePassthroughBackgrounded(m passthroughBackgroundedMsg) tea.Cmd {
	delete(a.passthroughCards, m.id)
	a.refreshDock()
	return a.notice("info", "Moved to background", render.Clip(m.command, 60))
}

func (a *App) waitForAgentEvent() tea.Cmd {
	return func() tea.Msg {
		ev := <-a.ag.Events()
		return agentEventMsg{ev: ev}
	}
}

func (a *App) restoreSession(s *session.Session) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if a.thinking {
		return fmt.Errorf("agent is running — /cancel it before switching sessions")
	}
	// Session-scoped state from the previous conversation must not leak:
	// error history belongs to the session that produced it.
	a.apiErrors = nil
	a.clearRetryState()
	a.sess = s
	if a.sess.WorkDir == "" {
		a.sess.WorkDir = a.workDir
	}
	a.ag.SetSession(s)
	if a.persist != nil {
		a.persist.SetSession(s)
	}
	// Checkpoints must be derived from THIS session's history: the in-memory
	// list still holds the previously active conversation's rewind points,
	// and rewinding to one would splice foreign messages into this session.
	a.checkpoints = rebuildCheckpoints(s.Messages)
	// Same discipline for artifacts: reload this session's registry so the
	// previous session's artifacts never leak across.
	a.cancelPlanReview()
	a.loadArtifactsForSession()
	a.conversation.Clear()
	for _, sm := range s.Messages {
		a.replayMessage(sm)
	}
	a.stats.InputTokens = s.TotalInputTokens
	a.stats.OutputTokens = s.TotalOutputTokens
	a.header.SetTokens(s.TotalInputTokens + s.TotalOutputTokens)
	a.updateActiveTokens()
	if calc := a.ag.AdaptiveCalculator(); calc != nil {
		a.header.SetAdaptiveWeight(calc.Weight())
	}
	var providerErr error
	if s.Provider != "" {
		if err := a.switchProvider(s.Provider, s.Model); err != nil {
			providerErr = fmt.Errorf("session loaded, but provider switch failed: %w", err)
		}
	}
	a.statusBar.SetStatus("Session resumed: " + s.ID)
	a.layout()
	return providerErr
}

// replayMessage rebuilds the conversation view for one stored message so a
// resumed session looks exactly like it did while running (thoughts, tool
// calls, results and text are all restored, not just plain text).
func (a *App) replayMessage(sm ai.Message) {
	switch sm.Role {
	case ai.RoleTool:
		for _, p := range sm.Content {
			if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil {
				a.conversation.AddToolLifecycleDone(
					p.ToolResult.ToolCallID, "", "", "", 0,
					tools.Result{Content: p.ToolResult.Content, IsError: p.ToolResult.IsError},
					a.conversation.ReviewMode(),
				)
			}
		}
	case ai.RoleUser, ai.RoleSystem:
		// System messages are model/context plumbing (compaction summaries,
		// staged instructions), not user-facing conversation. They must not
		// leak into the transcript when a session is resumed.
		if sm.Role == ai.RoleSystem {
			return
		}
		if text := sm.TextContent(); text != "" {
			a.conversation.AddMessage(string(sm.Role), text, false)
		}
	case ai.RoleAssistant:
		var thought, text string
		for _, p := range sm.Content {
			switch p.Type {
			case ai.ContentTypeThought:
				thought += p.Thought
			case ai.ContentTypeText:
				text += p.Text
			}
		}
		if thought != "" || text != "" {
			a.conversation.AddMessageFull(string(sm.Role), text, thought, false)
		}
		for _, p := range sm.Content {
			if p.Type != ai.ContentTypeToolCall || p.ToolCall == nil {
				continue
			}
			argText := ""
			if len(p.ToolCall.Args) > 0 {
				if b, err := json.Marshal(p.ToolCall.Args); err == nil {
					argText = string(b)
				}
			}
			a.conversation.AddToolLifecycleStart(p.ToolCall.ID, p.ToolCall.Name, argText, extractToolContext(p.ToolCall.Name, p.ToolCall.Args))
		}
	}
}

// replayAll rebuilds the conversation pane from the session messages.
func (a *App) replayAll() {
	a.conversation.Clear()
	for _, sm := range a.sess.Messages {
		a.replayMessage(sm)
	}
}

func (a *App) cancelActiveRun(status string) {
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.thinking = false
	a.spin.Stop()
	a.setSpinVerb("")
	a.streamTickPending = false
	a.activeTool = ""
	a.clearRetryState()
	// A queued message belonged to the run being cancelled: delivering it into
	// the next, unrelated run would be surprising. Never do this silently —
	// say how many messages the interrupt took with it.
	if n := len(a.msgQueue); n > 0 {
		status = strings.TrimSpace(status + " · " + pluralMessages(n) + " queued discarded")
	}
	a.clearQueue()
	a.conversation.RenderIfDirty()

	// Unblock a structured ask_user session in flight
	if a.pendingAsk != nil {
		select {
		case <-a.pendingAsk.done:
		default:
			close(a.pendingAsk.done)
		}
		a.pendingAsk = nil
		a.questionnaire.Cancel()
	}

	// Clean up any pending ask_user channel to prevent agent deadlock
	if a.askUserReplyCh != nil {
		select {
		case a.askUserReplyCh <- "": // Send empty response to unblock
		default:
		}
		a.askUserReplyCh = nil
	}

	// Drain any pending events from the cancelled run
	for {
		select {
		case <-a.ag.Events():
		default:
			goto done
		}
	}
done:
	a.layout() // Reclaim space
	if status != "" {
		a.statusBar.SetStatus(status)
	}
}
