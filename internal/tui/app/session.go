package app

// Session lifecycle: runs, restore/replay, rewind, cancellation.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (a *App) startAgent(prompt string) tea.Cmd {
	// Checkpoint the pre-turn conversation so /rewind can return here.
	a.captureCheckpoint(prompt)
	prompt = a.expandPrompt(prompt)
	if strings.HasPrefix(prompt, "!") {
		return a.runShellPassthrough(prompt[1:])
	}
	a.thinking = true
	a.streamedReply = false
	a.runTokens = 0
	a.runStart = time.Now()
	a.tokRate = 0
	a.activeTool = ""
	a.runToolCount = 0
	a.lastOutcome = outcomeNone
	a.clearRetryState()
	a.spin.SetLabel("thinking")
	a.spin.Start()
	a.conversation.AddMessage("user", prompt, false)
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

func (a *App) runShellPassthrough(command string) tea.Cmd {
	a.conversation.AddMessage("user", "!"+command, false)
	return func() tea.Msg {
		cmd := exec.Command("bash", "-c", command)
		output, _ := cmd.CombinedOutput()
		content := string(output)
		if content == "" {
			content = "(no output)"
		}
		return agentEventMsg{ev: agent.Event{Type: agent.EventDone, Payload: content}}
	}
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
	a.sess = s
	if a.sess.WorkDir == "" {
		a.sess.WorkDir = a.workDir
	}
	a.ag.SetSession(s)
	if a.persist != nil {
		a.persist.SetSession(s)
	}
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
			providerErr = err
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

func (a *App) rewindConversation(args []string) {
	if a.thinking {
		a.conversation.AddMessage("system", "Cannot rewind while the agent is running.", true)
		return
	}
	userTurnIdx := []int{}
	for i, msg := range a.sess.Messages {
		if msg.Role == ai.RoleUser {
			userTurnIdx = append(userTurnIdx, i)
		}
	}
	if len(userTurnIdx) < 2 {
		a.conversation.AddMessage("system", "Nothing to rewind — fewer than two user turns.", false)
		return
	}
	target := len(userTurnIdx) - 2 // default: drop the last exchange
	if len(args) > 0 {
		var n int
		if _, err := fmt.Sscanf(args[0], "%d", &n); err == nil && n >= 1 && n <= len(userTurnIdx) {
			target = n - 1
		}
	}
	cut := userTurnIdx[target]
	a.sess.SetMessages(append([]ai.Message{}, a.sess.Messages[:cut]...))
	if a.storage != nil {
		if err := a.storage.Save(a.sess); err != nil {
			a.conversation.AddMessage("system", "rewind: persist failed: "+err.Error(), true)
		}
	}
	a.replayAll()
	a.statusBar.SetStatus(fmt.Sprintf("Rewound to turn %d", target+1))
	a.layout()
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
	a.spin.SetLabel("thinking")
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
